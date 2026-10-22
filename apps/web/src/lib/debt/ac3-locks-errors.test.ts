/** AC-3 (R4-2 / W-2): any non-ApiError from navigator.locks.request (rejection or synchronous throw) becomes an ApiError.
 *  InvalidStateError / NotSupportedError / AbortError / sync TypeError -> NETWORK_ERROR, 0 POST, session kept, no timer left.
 *  SecurityError -> in-tab single-flight refresh (exactly 1 POST, request succeeds). */
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { installFetch, json, problem, resetEnv, setupEnv } from "@/test-utils/fetch-mock";

import { installLocks, newTab, removeLocks, seedUser } from "./harness";

beforeEach(() => {
  setupEnv();
  vi.useFakeTimers();
});
afterEach(() => {
  vi.useRealTimers();
  removeLocks();
  resetEnv();
});

const isRefresh = (c: { path: string }) => c.path === "/api/v1/auth/refresh";

function api() {
  let valid = "acc-never";
  return installFetch((c) => {
    if (isRefresh(c)) {
      valid = "acc-2";
      return json({ id: "u-1", role: "buyer", access_token: "acc-2", refresh_token: "r2", expires_in: 900 });
    }
    return c.headers.get("Authorization") === `Bearer ${valid}` ? json({ ok: true }) : problem(401, "/problems/token-expired");
  });
}

type Mode = "reject" | "throw";
const make = (mode: Mode, name: string) => () => {
  const err = name === "TypeError" ? new TypeError("boom") : new DOMException("lock failure", name);
  if (mode === "throw") throw err;
  return Promise.reject(err);
};

const NETWORK_CASES: Array<[Mode, string]> = [
  ["reject", "InvalidStateError"],
  ["reject", "NotSupportedError"],
  ["reject", "AbortError"],
  ["reject", "TypeError"],
  ["throw", "InvalidStateError"],
  ["throw", "NotSupportedError"],
  ["throw", "TypeError"],
];

describe("AC-3 lock failures", () => {
  it.each(NETWORK_CASES)("%s %s -> ApiError NETWORK_ERROR, 0 POST, session kept, no pending timer", async (mode, name) => {
    installLocks({ request: make(mode, name) });
    const { calls } = api();
    seedUser("u-1", "old", "r1");
    const tab = await newTab();
    const timersBefore = vi.getTimerCount();
    const err = await tab.api.apiFetch("/v1/orders/o1").then(
      () => null,
      (e: unknown) => e,
    );
    expect(err).toBeInstanceOf(tab.api.ApiError);
    expect((err as { code: string }).code).toBe("NETWORK_ERROR");
    expect(calls.filter(isRefresh)).toHaveLength(0);
    expect(tab.sess.getSession()?.refreshToken).toBe("r1");
    expect(vi.getTimerCount()).toBeLessThanOrEqual(timersBefore);
  });

  it.each(["reject", "throw"] as const)("SecurityError (%s) -> in-tab single-flight: exactly 1 POST, request succeeds", async (mode) => {
    installLocks({ request: make(mode, "SecurityError") });
    const { calls } = api();
    seedUser("u-1", "old", "r1");
    const tab = await newTab();
    const timersBefore = vi.getTimerCount();
    const [a, b] = await Promise.all([tab.api.apiFetch("/v1/orders/o1"), tab.api.apiFetch("/v1/orders/o2")]);
    expect([a.status, b.status]).toEqual([200, 200]);
    expect(calls.filter(isRefresh)).toHaveLength(1);
    expect(tab.sess.getSession()?.refreshToken).toBe("r2");
    await vi.advanceTimersByTimeAsync(0);
    expect(vi.getTimerCount()).toBeLessThanOrEqual(timersBefore);
  });

  it("W-2: the lock error also reaches hold-client as a classified (AMBIGUOUS) error, not a raw DOMException", async () => {
    installLocks({ request: make("reject", "InvalidStateError") });
    api();
    seedUser("u-1", "old", "r1");
    await newTab();
    const hold = await import("@/lib/hold-client");
    const e = await hold
      .requestHold({ eventId: "e1", ticketTypeId: "t1", quantity: 1, queueToken: "q" })
      .then(
        () => null,
        (x: { type?: string }) => x,
      );
    expect(e).not.toBeInstanceOf(DOMException);
    expect(e?.type).toBe("AMBIGUOUS");
  });

  it("GUARD: a lock error does not clear buy-flow keys or the session", async () => {
    installLocks({ request: make("reject", "InvalidStateError") });
    api();
    seedUser("u-1", "old", "r1");
    sessionStorage.setItem("ef:queue:token:e1", "q-tok");
    const tab = await newTab();
    await tab.api.apiFetch("/v1/orders/o1").catch(() => undefined);
    expect(sessionStorage.getItem("ef:queue:token:e1")).toBe("q-tok");
    expect(tab.sess.getSession()).not.toBeNull();
  });
});
