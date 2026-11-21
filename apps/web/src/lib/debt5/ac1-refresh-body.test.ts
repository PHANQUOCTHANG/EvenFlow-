/** AC-1 (R-3): refresh answers 2xx but the body is unusable -> AUTH_REQUIRED (not NETWORK_ERROR); session cleared only
 *  if it is still the posted session; abort while reading the body keeps the abort outcome. */
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { json, resetEnv, setupEnv } from "@/test-utils/fetch-mock";

import { fakeLocks, installLocks, newTab, outcome, removeLocks, seedUser } from "../debt/harness";
import { apiServer, posts } from "./harness5";

beforeEach(() => {
  setupEnv();
  vi.useFakeTimers();
  installLocks(fakeLocks());
});
afterEach(() => {
  vi.useRealTimers();
  removeLocks();
  resetEnv();
});

const broken = () => new Response(new ReadableStream({ start: (c) => c.error(new Error("connection dropped")) }), { status: 200 });

describe("AC-1 unusable refresh body", () => {
  const cases: Array<[string, () => Response]> = [
    ["not JSON", () => new Response("<html>oops</html>", { status: 200 })],
    ["body stream errors", broken],
    ["JSON null", () => json(null)],
    ["JSON string", () => json("hello")],
    ["JSON number", () => json(5)],
  ];
  it.each(cases)("200 + %s -> AUTH_REQUIRED, session cleared, 1 POST", async (_n, mk) => {
    const s = apiServer();
    s.setRefresh(mk);
    seedUser("u-1", "old", "r1");
    const A = await newTab();
    const a = outcome("A", A.api.apiFetch("/v1/orders/a"));
    await vi.advanceTimersByTimeAsync(100);
    expect(await a).toBe("A:AUTH_REQUIRED");
    expect(A.sess.getSession()).toBeNull();
    expect(posts(s.calls)).toEqual(["r1"]);
  });

  it("GUARD: JSON array body already ended in AUTH_REQUIRED (stays so)", async () => {
    const s = apiServer();
    s.setRefresh(() => json([]));
    seedUser("u-1", "old", "r1");
    const A = await newTab();
    const a = outcome("A", A.api.apiFetch("/v1/orders/a"));
    await vi.advanceTimersByTimeAsync(100);
    expect(await a).toBe("A:AUTH_REQUIRED");
  });

  it("GUARD: sign-out in this tab (epoch changed) then another user signs in; bad body -> AUTH_REQUIRED, other session untouched", async () => {
    const s = apiServer();
    s.controlled();
    seedUser("u-1", "old", "r1");
    const A = await newTab();
    const a = outcome("A", A.api.apiFetch("/v1/orders/a"));
    await vi.advanceTimersByTimeAsync(1_000);
    A.sess.clearSession();
    seedUser("u-2", "acc-u2", "r-u2");
    s.answer(new Response("garbage", { status: 200 }));
    await vi.advanceTimersByTimeAsync(100);
    expect(await a).toBe("A:AUTH_REQUIRED");
    expect(A.sess.getSession()?.user.id).toBe("u-2");
    expect(A.sess.getSession()?.refreshToken).toBe("r-u2");
  });

  it("GUARD: abort at 120 s while the body is being read keeps the abort outcome (AUTH_REQUIRED, cleared, 1 POST)", async () => {
    const s = apiServer();
    s.setRefresh((c) => {
      const sig = c.init.signal as AbortSignal;
      return Promise.resolve(
        new Response(
          new ReadableStream({
            start(ctl) {
              sig.addEventListener("abort", () => ctl.error(sig.reason ?? new DOMException("aborted", "AbortError")));
            },
          }),
          { status: 200 },
        ),
      );
    });
    seedUser("u-1", "old", "r1");
    const A = await newTab();
    const a = outcome("A", A.api.apiFetch("/v1/orders/a"));
    await vi.advanceTimersByTimeAsync(119_000);
    await vi.advanceTimersByTimeAsync(2_000);
    expect(await a).toBe("A:AUTH_REQUIRED");
    expect(A.sess.getSession()).toBeNull();
    expect(posts(s.calls)).toEqual(["r1"]);
  });
});
