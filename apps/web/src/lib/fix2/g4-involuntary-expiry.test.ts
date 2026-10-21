/** Regression G-4 (R2-2): involuntary session expiry keeps buy-flow keys; only explicit logout clears them. */
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { apiFetch } from "@/lib/api-client";
import { logout } from "@/lib/api/auth";
import { getSession } from "@/lib/session";
import { installFetch, noContent, problem, resetEnv, seedSession, setupEnv } from "@/test-utils/fetch-mock";

beforeEach(setupEnv);
afterEach(resetEnv);

const KEYS = ["ef:queue:token:e1", "evenflow_hold_e1", "ef:pay:o1", "ef:hold:e1:t1:2", "ef:issue:o1"];

describe("G-4", () => {
  it("refresh rejected (401) -> auth keys cleared, buy-flow keys kept", async () => {
    seedSession("old", "r1");
    for (const k of KEYS) sessionStorage.setItem(k, "x");
    installFetch((c) =>
      c.path === "/api/v1/auth/refresh" ? problem(401, "/problems/unauthorized") : problem(401, "/problems/token-expired"),
    );
    await expect(apiFetch("/v1/orders/o1")).rejects.toMatchObject({ code: "AUTH_REQUIRED" });
    expect(getSession()).toBeNull();
    for (const k of KEYS) expect(sessionStorage.getItem(k), k).toBe("x");
  });

  it("explicit logout still clears buy-flow keys", async () => {
    seedSession("a", "r");
    for (const k of KEYS) sessionStorage.setItem(k, "x");
    installFetch(() => noContent());
    await logout();
    for (const k of KEYS) expect(sessionStorage.getItem(k), k).toBeNull();
  });
});
