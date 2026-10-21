/** Regression F-14 (X-10) / H-1: a stuck Web Lock must not hang apiFetch forever, and must never lead to a POST. */
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { apiFetch } from "@/lib/api-client";
import { getSession } from "@/lib/session";
import { installFetch, json, problem, resetEnv, seedSession, setupEnv } from "@/test-utils/fetch-mock";

beforeEach(setupEnv);
afterEach(() => {
  vi.useRealTimers();
  Reflect.deleteProperty(navigator, "locks");
  resetEnv();
});

describe("F-14", () => {
  it("lock never granted -> after the deadline NETWORK_ERROR, no POST /refresh, session kept (H-1)", async () => {
    vi.useFakeTimers();
    const request = (_n: string, opts: { signal?: AbortSignal }) =>
      new Promise((_, reject) => {
        opts.signal?.addEventListener("abort", () => reject(new DOMException("aborted", "AbortError")));
      });
    Object.defineProperty(navigator, "locks", { value: { request }, configurable: true });
    const { calls } = installFetch((c) =>
      c.path === "/api/v1/auth/refresh" ? json({}) : problem(401, "/problems/token-expired"),
    );
    seedSession("old", "r1");
    const p = apiFetch("/v1/orders/o1").then(
      () => "ok",
      (e: { code?: string }) => e.code,
    );
    await vi.advanceTimersByTimeAsync(46_000);
    expect(await p).toBe("NETWORK_ERROR");
    expect(calls.filter((c) => c.path === "/api/v1/auth/refresh")).toHaveLength(0);
    expect(getSession()?.refreshToken).toBe("r1");
  });
});
