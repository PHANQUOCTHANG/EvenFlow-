/** Regression H-2 (Z-4, R3-4): a different user signing in must not inherit buy-flow keys; the same user keeps them. */
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { apiFetch } from "@/lib/api-client";
import { loginUser } from "@/lib/api/auth";
import { getSession } from "@/lib/session";
import { installFetch, json, problem, resetEnv, seedSession, setupEnv } from "@/test-utils/fetch-mock";

beforeEach(setupEnv);
afterEach(resetEnv);

const KEYS = ["ef:queue:token:e1", "evenflow_hold_e1", "evenflow_order_o1", "ef:issue:o1", "ef:pay:o1", "ef:hold:e1:t1:2"];

function handler(uid: string) {
  return (c: { path: string }) => {
    if (c.path === "/api/v1/auth/refresh") return problem(401, "/problems/unauthorized");
    if (c.path === "/api/v1/auth/login")
      return json({ id: uid, role: "buyer", access_token: "acc-B", refresh_token: "r-B", expires_in: 900 });
    return problem(401, "/problems/token-expired");
  };
}

async function expireThenLogin(uid: string) {
  seedSession("old", "r1"); // user u-1
  for (const k of KEYS) sessionStorage.setItem(k, "x");
  // establish the "last user" the way a real sign-in would
  const { setSession } = await import("@/lib/session");
  setSession({ accessToken: "old", refreshToken: "r1", user: { id: "u-1", role: "buyer" } });
  for (const k of KEYS) sessionStorage.setItem(k, "x");
  installFetch(handler(uid));
  await expect(apiFetch("/v1/orders/o1")).rejects.toMatchObject({ code: "AUTH_REQUIRED" });
  expect(getSession()).toBeNull();
  for (const k of KEYS) expect(sessionStorage.getItem(k), k).toBe("x"); // G-4: kept on involuntary expiry
  await loginUser("a@b.co", "pw");
}

describe("H-2", () => {
  it("different user signs in after involuntary expiry: buy-flow keys cleared", async () => {
    await expireThenLogin("u-2");
    expect(getSession()?.user.id).toBe("u-2");
    for (const k of KEYS) expect(sessionStorage.getItem(k), k).toBeNull();
  });

  it("same user signs in again: buy-flow keys kept", async () => {
    await expireThenLogin("u-1");
    for (const k of KEYS) expect(sessionStorage.getItem(k), k).toBe("x");
  });

  it("different user signs in while another is still signed in: buy-flow keys cleared", async () => {
    seedSession("old", "r1");
    for (const k of KEYS) sessionStorage.setItem(k, "x");
    installFetch(handler("u-2"));
    await loginUser("a@b.co", "pw");
    for (const k of KEYS) expect(sessionStorage.getItem(k), k).toBeNull();
  });
});
