/** F-3 (R-2): the user guard is an explicit null check, so an empty-string user id cannot bypass it. */
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { installFetch, json, problem, resetEnv, setupEnv } from "@/test-utils/fetch-mock";

import { fakeLocks, installLocks, newTab, outcome, removeLocks, seedUser } from "./harness";

beforeEach(setupEnv);
afterEach(() => {
  vi.useRealTimers();
  removeLocks();
  resetEnv();
});

describe("F-3 empty user id", () => {
  it("request sent by user id \"\"; another tab signs in u-2: AUTH_REQUIRED, never retried with u-2's token", async () => {
    vi.useFakeTimers();
    const locks = fakeLocks();
    installLocks(locks);
    let release!: () => void;
    locks.request("ef-auth-refresh", () => new Promise<void>((r) => (release = r))).catch(() => undefined);
    const { calls } = installFetch((c) => {
      if (c.path === "/api/v1/auth/refresh") return json({});
      return c.headers.get("Authorization") === "Bearer acc-u2" ? json({ ok: true }) : problem(401, "/problems/token-expired");
    });
    seedUser("", "old", "r1");
    const tab = await newTab();
    const p = outcome("A", tab.api.apiFetch("/v1/orders/o1"));
    await vi.advanceTimersByTimeAsync(3_000);
    seedUser("u-2", "acc-u2", "r-u2");
    release();
    await vi.advanceTimersByTimeAsync(2_000);
    expect(await p).toBe("A:AUTH_REQUIRED");
    expect(calls.some((c) => c.headers.get("Authorization") === "Bearer acc-u2")).toBe(false);
    expect(localStorage.getItem("ef_token")).toBe("acc-u2");
  });
});
