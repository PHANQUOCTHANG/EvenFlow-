/** Plan P-2 (mutant M-11): the used check runs BEFORE the "different pair -> reuse" branch. A request sent with r2;
 *  another tab clears localStorage; an old, already-used u-1/r1 copy surfaces from sessionStorage -> AUTH_REQUIRED,
 *  0 POST, the r1 copy wiped, the stale r1 access token is never sent, no raw 401. */
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import {
  installFetch,
  json,
  problem,
  resetEnv,
  setupEnv,
} from "@/test-utils/fetch-mock";

import {
  fakeLocks,
  installLocks,
  newTab,
  outcome,
  removeLocks,
  seedUser,
} from "../debt/harness";
import { bearers, isRefresh, posts } from "../debt5/harness5";

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

describe("P-2 used r1 copy surfacing from sessionStorage", () => {
  it("request sent with r2, other tab clears localStorage, used r1 copy in sessionStorage -> AUTH_REQUIRED, 0 POST, copy wiped", async () => {
    const s = installFetch(async (c) => {
      if (isRefresh(c)) return json({});
      const auth = c.headers.get("Authorization");
      if (auth === "Bearer acc2") {
        localStorage.clear(); // another tab signs out while this request is in flight
        return problem(401, "/problems/token-expired");
      }
      return auth === "Bearer old1"
        ? json({ ok: true })
        : problem(401, "/problems/token-expired");
    });
    seedUser("u-1", "old1", "r1", sessionStorage);
    seedUser("u-1", "acc2", "r2", localStorage);
    const A = await newTab();
    A.sess.markRefreshUsed("r1");
    const a = outcome("A", A.api.apiFetch("/v1/orders/a"));
    await vi.advanceTimersByTimeAsync(200);
    expect(await a).toBe("A:AUTH_REQUIRED");
    expect(posts(s.calls)).toEqual([]);
    expect(bearers(s.calls)).toEqual(["Bearer acc2"]);
    expect(sessionStorage.getItem("ef_refresh")).toBeNull();
    expect(sessionStorage.getItem("ef_token")).toBeNull();
    expect(A.sess.getSession()).toBeNull();
  });
});
