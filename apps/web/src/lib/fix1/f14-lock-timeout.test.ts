/** Regression F-14 (X-10): a stuck Web Lock must not hang apiFetch forever. */
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
  it("lock never granted -> after the timeout the tab refreshes on its own", async () => {
    vi.useFakeTimers();
    Object.defineProperty(navigator, "locks", { value: { request: () => new Promise(() => {}) }, configurable: true });
    let refreshed = false;
    installFetch((c) => {
      if (c.path === "/api/v1/auth/refresh") {
        refreshed = true;
        return json({ id: "u-1", role: "buyer", access_token: "n", refresh_token: "r2", expires_in: 900 });
      }
      return refreshed ? json({ ok: true }) : problem(401, "/problems/token-expired");
    });
    seedSession("old", "r1");
    let settled = false;
    const p = apiFetch("/v1/orders/o1").then((r) => {
      settled = true;
      return r;
    });
    await vi.advanceTimersByTimeAsync(11_000);
    expect(settled).toBe(true);
    expect((await p).status).toBe(200);
    expect(getSession()?.accessToken).toBe("n");
  });
});
