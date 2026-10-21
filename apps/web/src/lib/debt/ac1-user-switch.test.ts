/** AC-1 (R4-4): a request sent by user A is never retried with user B's token after a cross-tab user switch. */
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { installFetch, json, problem, resetEnv, setupEnv } from "@/test-utils/fetch-mock";

import { fakeLocks, installLocks, newTab, outcome, removeLocks, seedUser } from "./harness";

beforeEach(setupEnv);
afterEach(() => {
  vi.useRealTimers();
  removeLocks();
  resetEnv();
});

/** Tab T holds a pending request of u-1 behind a lock held elsewhere; returns controls. */
async function waitingRequest() {
  vi.useFakeTimers();
  const locks = fakeLocks();
  installLocks(locks);
  let release!: () => void;
  locks.request("ef-auth-refresh", () => new Promise<void>((r) => (release = r))).catch(() => undefined);
  const { calls } = installFetch((c) => {
    if (c.path === "/api/v1/auth/refresh") {
      return json({ id: "u-x", role: "buyer", access_token: "acc-new", refresh_token: "r-new" });
    }
    const auth = c.headers.get("Authorization");
    return auth === "Bearer acc-rotated" || auth === "Bearer acc-u2"
      ? json({ ok: true })
      : problem(401, "/problems/token-expired");
  });
  seedUser("u-1", "old", "r1");
  const tab = await newTab();
  const p = outcome("A", tab.api.apiFetch("/v1/orders/o1"));
  await vi.advanceTimersByTimeAsync(3_000);
  return { tab, p, calls, release };
}

describe("AC-1 session ownership during the lock wait", () => {
  it("another tab signs out u-1 and signs in u-2: AUTH_REQUIRED, u-2 session untouched, 0 POST, no retry with u-2 token", async () => {
    const { p, calls, release } = await waitingRequest();
    seedUser("u-2", "acc-u2", "r-u2");
    release();
    await vi.advanceTimersByTimeAsync(2_000);
    expect(await p).toBe("A:AUTH_REQUIRED");
    expect(calls.filter((c) => c.path === "/api/v1/auth/refresh")).toHaveLength(0);
    expect(calls.some((c) => c.headers.get("Authorization") === "Bearer acc-u2")).toBe(false);
    expect(localStorage.getItem("ef_token")).toBe("acc-u2");
    expect(localStorage.getItem("ef_refresh")).toBe("r-u2");
    expect(JSON.parse(localStorage.getItem("ef_user")!).id).toBe("u-2");
  });

  it("u-2 holds the SAME refresh string but a different user id: still AUTH_REQUIRED (identity decides, not token)", async () => {
    const { p, calls, release } = await waitingRequest();
    seedUser("u-2", "acc-u2", "r1");
    release();
    await vi.advanceTimersByTimeAsync(2_000);
    expect(await p).toBe("A:AUTH_REQUIRED");
    expect(calls.filter((c) => c.path === "/api/v1/auth/refresh")).toHaveLength(0);
    expect(localStorage.getItem("ef_token")).toBe("acc-u2");
  });

  it("GUARD: the other tab rotated the pair for the SAME user: reused, request succeeds, no POST", async () => {
    const { p, calls, release } = await waitingRequest();
    seedUser("u-1", "acc-rotated", "r2");
    release();
    await vi.advanceTimersByTimeAsync(2_000);
    expect(await p).toBe("A:200");
    expect(calls.filter((c) => c.path === "/api/v1/auth/refresh")).toHaveLength(0);
    expect(localStorage.getItem("ef_refresh")).toBe("r2");
  });

  it("GUARD: the other tab signed out and nobody signed in: AUTH_REQUIRED, 0 POST", async () => {
    const { p, calls, release } = await waitingRequest();
    localStorage.clear();
    release();
    await vi.advanceTimersByTimeAsync(2_000);
    expect(await p).toBe("A:AUTH_REQUIRED");
    expect(calls.filter((c) => c.path === "/api/v1/auth/refresh")).toHaveLength(0);
  });

  it("u-1 -> u-2 -> u-1 within one wait: same user id again, so reuse is allowed (no POST)", async () => {
    const { p, calls, release } = await waitingRequest();
    seedUser("u-2", "acc-u2", "r-u2");
    seedUser("u-1", "acc-rotated", "r-back");
    release();
    await vi.advanceTimersByTimeAsync(2_000);
    expect(await p).toBe("A:200");
    expect(calls.filter((c) => c.path === "/api/v1/auth/refresh")).toHaveLength(0);
  });
});

describe("AC-1 user switch WHILE the refresh POST is in flight (plan M2-2)", () => {
  it("other tab signs in u-2 during u-1's POST, POST then succeeds: request NOT retried with u-2 token, AUTH_REQUIRED, u-2 session intact", async () => {
    vi.useFakeTimers();
    installLocks(fakeLocks());
    let answer!: () => void;
    const { calls } = installFetch(async (c) => {
      if (c.path === "/api/v1/auth/refresh") {
        await new Promise<void>((r) => (answer = r));
        return json({ id: "u-1", role: "buyer", access_token: "acc-u1-new", refresh_token: "r1b", expires_in: 900 });
      }
      const auth = c.headers.get("Authorization");
      return auth === "Bearer acc-u2" || auth === "Bearer acc-u1-new"
        ? json({ ok: true })
        : problem(401, "/problems/token-expired");
    });
    seedUser("u-1", "old", "r1");
    const tab = await newTab();
    const p = outcome("A", tab.api.apiFetch("/v1/orders/o1"));
    await vi.advanceTimersByTimeAsync(3_000);
    seedUser("u-2", "acc-u2", "r-u2");
    answer();
    await vi.advanceTimersByTimeAsync(2_000);
    expect(await p).toBe("A:AUTH_REQUIRED");
    expect(calls.some((c) => c.headers.get("Authorization") === "Bearer acc-u2")).toBe(false);
    expect(calls.filter((c) => c.path === "/api/v1/auth/refresh")).toHaveLength(1);
    expect(localStorage.getItem("ef_token")).toBe("acc-u2");
    expect(localStorage.getItem("ef_refresh")).toBe("r-u2");
    expect(JSON.parse(localStorage.getItem("ef_user")!).id).toBe("u-2");
  });
});
