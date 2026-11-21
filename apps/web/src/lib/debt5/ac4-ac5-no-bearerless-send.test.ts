/** AC-4 (X-S1): swallowed proactive refresh + session gone -> AUTH_REQUIRED, nothing sent.
 *  AC-5 (X-S2): lock manager resolves without running the callback -> NETWORK_ERROR, session kept, no POST, no bearer-less retry. */
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { resetEnv, setupEnv } from "@/test-utils/fetch-mock";

import { fakeLocks, installLocks, newTab, outcome, removeLocks, seedUser } from "../debt/harness";
import { apiCalls, apiServer, bearers, jwtExpIn, posts } from "./harness5";

beforeEach(() => {
  setupEnv();
  vi.useFakeTimers();
});
afterEach(() => {
  vi.useRealTimers();
  removeLocks();
  resetEnv();
});

describe("AC-4 no unauthenticated send after a swallowed proactive refresh", () => {
  it("X-S1: lock never granted, another tab signs out at 2 s -> AUTH_REQUIRED at 45 s, no request at all, no POST", async () => {
    const locks = fakeLocks();
    installLocks(locks);
    void locks.request("ef-auth-refresh", () => new Promise(() => {}));
    const s = apiServer();
    seedUser("u-1", jwtExpIn(10), "r1");
    const A = await newTab();
    const a = outcome("A", A.api.apiFetch("/v1/orders/a"));
    await vi.advanceTimersByTimeAsync(2_000);
    localStorage.clear();
    await vi.advanceTimersByTimeAsync(44_000);
    expect(await a).toBe("A:AUTH_REQUIRED");
    expect(apiCalls(s.calls)).toHaveLength(0);
    expect(posts(s.calls)).toEqual([]);
  });

  it("GUARD: transient proactive failure but the session is still there -> request sent WITH the current bearer", async () => {
    const locks = fakeLocks();
    installLocks(locks);
    void locks.request("ef-auth-refresh", () => new Promise(() => {}));
    const tok = jwtExpIn(10);
    const s = apiServer(() => tok);
    seedUser("u-1", tok, "r1");
    const A = await newTab();
    const a = outcome("A", A.api.apiFetch("/v1/orders/a"));
    await vi.advanceTimersByTimeAsync(46_000);
    expect(await a).toBe("A:200");
    expect(bearers(s.calls)).toEqual([`Bearer ${tok}`]);
  });

  it("GUARD: a call that starts without any session is still sent without a bearer (public behaviour)", async () => {
    installLocks(fakeLocks());
    const s = apiServer();
    const A = await newTab();
    const a = outcome("A", A.api.apiFetch("/v1/orders/a"));
    await vi.advanceTimersByTimeAsync(10);
    expect(await a).toBe("A:401");
    expect(bearers(s.calls)).toEqual([null]);
  });
});

describe("AC-5 lock result that is not a token", () => {
  it("X-S2: request() resolves undefined -> NETWORK_ERROR, session kept, 0 POST, only the original bearer ever sent, timers cleaned", async () => {
    installLocks({ request: () => Promise.resolve(undefined) });
    const s = apiServer();
    seedUser("u-1", "old", "r1");
    const A = await newTab();
    const a = outcome("A", A.api.apiFetch("/v1/orders/a"));
    await vi.advanceTimersByTimeAsync(100);
    expect(await a).toBe("A:NETWORK_ERROR");
    expect(A.sess.getSession()?.refreshToken).toBe("r1");
    expect(posts(s.calls)).toEqual([]);
    expect(bearers(s.calls)).toEqual(["Bearer old"]);
    expect(vi.getTimerCount()).toBe(0);
  });

  it("refreshAccessToken() itself rejects NETWORK_ERROR (not resolves undefined)", async () => {
    installLocks({ request: () => Promise.resolve(undefined) });
    apiServer();
    seedUser("u-1", "old", "r1");
    const A = await newTab();
    const p = A.api.refreshAccessToken().then(
      (v: unknown) => `resolved:${String(v)}`,
      (e: { code?: string }) => `rejected:${e.code}`,
    );
    await vi.advanceTimersByTimeAsync(100);
    expect(await p).toBe("rejected:NETWORK_ERROR");
  });

  it("GUARD: a real lock that runs the callback still refreshes once", async () => {
    installLocks(fakeLocks());
    const s = apiServer(() => "new");
    s.setRefresh(() =>
      Promise.resolve(
        new Response(JSON.stringify({ id: "u-1", role: "buyer", access_token: "new", refresh_token: "r2", expires_in: 900 }), {
          status: 200,
        }),
      ),
    );
    seedUser("u-1", "old", "r1");
    const A = await newTab();
    const a = outcome("A", A.api.apiFetch("/v1/orders/a"));
    await vi.advanceTimersByTimeAsync(100);
    expect(await a).toBe("A:200");
    expect(posts(s.calls)).toEqual(["r1"]);
  });
});
