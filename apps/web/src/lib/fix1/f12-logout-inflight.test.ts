/** Regression F-12 (C-8): logout during an in-flight refresh must not resurrect the session. */
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { apiFetch, ApiError } from "@/lib/api-client";
import { logout } from "@/lib/api/auth";
import { getSession } from "@/lib/session";
import { installFetch, json, noContent, problem, rejection, resetEnv, seedSession, setupEnv } from "@/test-utils/fetch-mock";

beforeEach(setupEnv);
afterEach(resetEnv);

describe("F-12", () => {
  it("logout while refresh is pending: refresh result is discarded", async () => {
    let release!: () => void;
    const gate = new Promise<void>((r) => (release = r));
    installFetch(async (c) => {
      if (c.path === "/api/v1/auth/refresh") {
        await gate;
        return json({ id: "u-1", role: "buyer", access_token: "new", refresh_token: "r2", expires_in: 900 });
      }
      if (c.path === "/api/v1/auth/logout") return noContent();
      return problem(401, "/problems/token-expired");
    });
    seedSession("old", "r1");
    const pending = rejection(apiFetch("/v1/orders/o1"));
    await new Promise((r) => setTimeout(r, 0));
    await logout();
    release();
    const err = (await pending) as ApiError;
    expect(err.code).toBe("AUTH_REQUIRED");
    expect(getSession()).toBeNull();
    expect(localStorage.getItem("ef_refresh")).toBeNull();
  });
});
