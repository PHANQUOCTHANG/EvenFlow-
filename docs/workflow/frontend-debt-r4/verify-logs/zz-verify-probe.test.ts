/** VERIFIER SCRATCH PROBE (not committed): bounded variants of the AC-2 tests that never await a pending promise,
 *  so a mutant surfaces as an assertion failure instead of a 5 s test timeout. */
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { installFetch, json, problem, resetEnv, setupEnv } from "@/test-utils/fetch-mock";

import { fakeLocks, hangUntilAborted, installLocks, newTab, outcome, removeLocks, seedUser } from "./harness";

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

const isRefresh = (c: { path: string }) => c.path === "/api/v1/auth/refresh";
function server() {
  return installFetch(async (c) => {
    if (isRefresh(c)) return hangUntilAborted(c.init);
    return c.headers.get("Authorization") === "Bearer acc-never" ? json({ ok: true }) : problem(401, "/problems/token-expired");
  });
}
function track(p: Promise<string>) {
  const s: { v: string | null } = { v: null };
  void p.then((v) => (s.v = v));
  return s;
}

describe("VERIFY probe (bounded)", () => {
  it("P-2: same-tab joiner started at 1 s has settled NETWORK_ERROR by 47 s (no await on pending)", async () => {
    server();
    seedUser("u-1", "old", "r1");
    const tab = await newTab();
    void tab.api.apiFetch("/v1/orders/o1").catch(() => undefined);
    await vi.advanceTimersByTimeAsync(1_000);
    const j = track(outcome("J", tab.api.apiFetch("/v1/orders/o1b")));
    await vi.advanceTimersByTimeAsync(46_000);
    expect(j.v).toBe("J:NETWORK_ERROR");
    await vi.advanceTimersByTimeAsync(200_000); // drain
  });

  it("P-6: no locks, never-answering POST: joiner times out at 91 s, call at 100 s does NOT start a 2nd POST", async () => {
    removeLocks();
    const srv = server();
    seedUser("u-1", "old", "r1");
    const tab = await newTab();
    const owner = track(outcome("owner", tab.api.apiFetch("/v1/orders/o1")));
    await vi.advanceTimersByTimeAsync(46_000);
    void outcome("j1", tab.api.apiFetch("/v1/orders/o2"));
    await vi.advanceTimersByTimeAsync(54_000); // t = 100 s
    const j2 = track(outcome("j2", tab.api.apiFetch("/v1/orders/o3")));
    await vi.advanceTimersByTimeAsync(1_000);
    expect(srv.calls.filter(isRefresh)).toHaveLength(1);
    await vi.advanceTimersByTimeAsync(20_000); // t = 121 s
    expect(owner.v).toBe("owner:AUTH_REQUIRED");
    expect(j2.v).toBe("j2:AUTH_REQUIRED");
    await vi.advanceTimersByTimeAsync(200_000); // drain
  });
});
