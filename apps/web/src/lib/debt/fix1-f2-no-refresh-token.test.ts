/** F-2 (R-1): the user check runs before the "no refresh token -> clearSession" branch; user B is never wiped. */
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { installFetch, json, problem, resetEnv, setupEnv } from "@/test-utils/fetch-mock";

import { fakeLocks, hangUntilAborted, installLocks, newTab, outcome, removeLocks, seedUser } from "./harness";

beforeEach(setupEnv);
afterEach(() => {
  vi.useRealTimers();
  removeLocks();
  resetEnv();
});

describe("F-2 doRefresh with another user's session lacking a refresh token", () => {
  it("u-1 request waits for the lock; another tab stores u-2 WITHOUT refresh token: AUTH_REQUIRED, u-2 intact, 0 POST", async () => {
    vi.useFakeTimers();
    const locks = fakeLocks();
    installLocks(locks);
    let release!: () => void;
    locks.request("ef-auth-refresh", () => new Promise<void>((r) => (release = r))).catch(() => undefined);
    const { calls } = installFetch((c) =>
      c.path === "/api/v1/auth/refresh" ? json({}) : problem(401, "/problems/token-expired"),
    );
    seedUser("u-1", "old", "r1");
    const tab = await newTab();
    const p = outcome("A", tab.api.apiFetch("/v1/orders/o1"));
    await vi.advanceTimersByTimeAsync(3_000);
    seedUser("u-2", "acc-u2", null);
    release();
    await vi.advanceTimersByTimeAsync(2_000);
    expect(await p).toBe("A:AUTH_REQUIRED");
    expect(calls.filter((c) => c.path === "/api/v1/auth/refresh")).toHaveLength(0);
    expect(localStorage.getItem("ef_token")).toBe("acc-u2");
    expect(JSON.parse(localStorage.getItem("ef_user")!).id).toBe("u-2");
  });
});
