/** Plan P-3 (mutant M-12): wiping a SHADOWED store (not the one that decides the session) must not bump the epoch.
 *  Live u-2 in localStorage is refreshing; the used u-1/r1 copy in sessionStorage is wiped meanwhile -> u-2's 2xx
 *  result is stored and used, not discarded. */
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { json, resetEnv, setupEnv } from "@/test-utils/fetch-mock";

import {
  fakeLocks,
  installLocks,
  newTab,
  outcome,
  removeLocks,
  seedUser,
} from "../debt/harness";
import { apiServer, bearers, IDENT, posts } from "../debt5/harness5";

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

describe("P-3 shadowed-store wipe during a live refresh", () => {
  it("GUARD: u-2 refreshing, used u-1/r1 sessionStorage copy wiped meanwhile -> u-2's 2xx result stored, request retried 200", async () => {
    const s = apiServer(() => "new-u2");
    s.controlled();
    seedUser("u-1", "old1", "r1", sessionStorage);
    seedUser("u-2", "acc-u2", "r-u2", localStorage);
    const A = await newTab();
    const epoch0 = A.sess.getSessionEpoch();
    const a = outcome("A", A.api.apiFetch("/v1/orders/a"));
    await vi.advanceTimersByTimeAsync(50);
    expect(posts(s.calls)).toEqual(["r-u2"]);
    A.sess.clearSessionHoldingRefresh("r1");
    expect(sessionStorage.getItem("ef_refresh")).toBeNull();
    expect(A.sess.getSessionEpoch()).toBe(epoch0);
    s.answer(
      json(IDENT({ id: "u-2", access_token: "new-u2", refresh_token: "r-u3" })),
    );
    await vi.advanceTimersByTimeAsync(50);
    expect(await a).toBe("A:200");
    expect(localStorage.getItem("ef_refresh")).toBe("r-u3");
    expect(A.sess.getSession()?.user.id).toBe("u-2");
    expect(bearers(s.calls)).toEqual(["Bearer acc-u2", "Bearer new-u2"]);
  });
});
