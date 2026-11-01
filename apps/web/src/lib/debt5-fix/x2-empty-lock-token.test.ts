/** X-2 (AC-5): a lock result of "" (empty string) is not a token -> NETWORK_ERROR, never a bearer-less retry. */
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { resetEnv, setupEnv } from "@/test-utils/fetch-mock";

import { installLocks, newTab, outcome, removeLocks, seedUser } from "../debt/harness";
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

describe("X-2 empty-string lock result", () => {
  it("request() resolves '' -> NETWORK_ERROR, session kept, 0 POST, only the original bearer ever sent", async () => {
    installLocks({ request: () => Promise.resolve("") });
    const s = apiServer();
    seedUser("u-1", "old", "r1");
    const A = await newTab();
    const a = outcome("A", A.api.apiFetch("/v1/orders/a"));
    await vi.advanceTimersByTimeAsync(100);
    expect(await a).toBe("A:NETWORK_ERROR");
    expect(bearers(s.calls)).toEqual(["Bearer old"]);
    expect(posts(s.calls)).toEqual([]);
    expect(A.sess.getSession()?.refreshToken).toBe("r1");
  });

  it("refreshAccessToken() rejects NETWORK_ERROR when the lock resolves ''", async () => {
    installLocks({ request: () => Promise.resolve("") });
    apiServer();
    seedUser("u-1", "old", "r1");
    const A = await newTab();
    const p = A.api.refreshAccessToken();
    const settled = p.then(
      (v) => ({ ok: true as const, v }),
      (e: unknown) => ({ ok: false as const, code: (e as { code?: string }).code }),
    );
    await vi.advanceTimersByTimeAsync(100);
    expect(await settled).toEqual({ ok: false, code: "NETWORK_ERROR" });
  });
});
