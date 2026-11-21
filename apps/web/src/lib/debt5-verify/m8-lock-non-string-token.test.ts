/** AC-5 (mutant M-8): a lock manager that resolves a truthy NON-string (e.g. `{}` or `42`) without running the callback is
 *  not a token -> NETWORK_ERROR, session kept, 0 POST, no retry with a bogus bearer. */
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { resetEnv, setupEnv } from "@/test-utils/fetch-mock";

import {
  installLocks,
  newTab,
  outcome,
  removeLocks,
  seedUser,
} from "../debt/harness";
import { apiServer, bearers, posts } from "../debt5/harness5";

beforeEach(() => {
  setupEnv();
  vi.useFakeTimers();
});
afterEach(() => {
  vi.useRealTimers();
  removeLocks();
  resetEnv();
});

describe("AC-5 lock result that is a truthy non-string", () => {
  for (const [label, value] of [
    ["{}", {}],
    ["42", 42],
  ] as const) {
    it(`request() resolves ${label} -> NETWORK_ERROR, session kept, 0 POST, only the original bearer sent`, async () => {
      installLocks({ request: () => Promise.resolve(value) });
      const s = apiServer();
      seedUser("u-1", "old", "r1");
      const A = await newTab();
      const a = outcome("A", A.api.apiFetch("/v1/orders/a"));
      await vi.advanceTimersByTimeAsync(100);
      expect(await a).toBe("A:NETWORK_ERROR");
      expect(A.sess.getSession()?.refreshToken).toBe("r1");
      expect(posts(s.calls)).toEqual([]);
      expect(bearers(s.calls)).toEqual(["Bearer old"]);
    });
  }
});
