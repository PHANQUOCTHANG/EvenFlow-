/**
 * Contract test: lib/api/auth.ts (AC-1, AC-2, AC-6, AC-8, AC-27).
 * Moi goi di qua gateway: `${NEXT_PUBLIC_API_BASE}/api/v1/auth/...`, khong con :8083 / NEXT_PUBLIC_API_URL.
 */
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import {
  fetchMe,
  loginUser,
  loginWithGoogle,
  logout,
  refreshSession,
  requestRegistrationOTP,
  requestResetPasswordOTP,
  resetPassword,
  verifyRegistrationOTP,
} from "@/lib/api/auth";
import { getDeviceFingerprint } from "@/lib/device";
import {
  BASE,
  expectNoIdentityOrCredentials,
  installFetch,
  json,
  noContent,
  problem,
  resetEnv,
  seedSession,
  setupEnv,
  type Call,
} from "@/test-utils/fetch-mock";

beforeEach(() => {
  setupEnv();
  vi.stubEnv("NEXT_PUBLIC_API_URL", "http://localhost:8083/api/v1"); // gia tri cu: phai bi bo qua
});
afterEach(resetEnv);

const TOKENS = {
  id: "u-1",
  email: "a@example.com",
  role: "buyer",
  created_at: "2026-01-01T00:00:00Z",
  access_token: "acc-new",
  token: "acc-new",
  token_type: "Bearer",
  expires_in: 900,
  refresh_token: "ref-new",
  refresh_expires_in: 604800,
};

describe("loginUser (AC-1)", () => {
  it("POST {base}/api/v1/auth/login {identifier, password, device_fingerprint}", async () => {
    const { calls } = installFetch(() => json(TOKENS));
    await loginUser("a@example.com", "pw123456");
    expect(calls).toHaveLength(1);
    expect(calls[0].url).toBe(`${BASE}/api/v1/auth/login`);
    expect(calls[0].method).toBe("POST");
    expect(calls[0].body).toEqual({
      identifier: "a@example.com",
      password: "pw123456",
      device_fingerprint: getDeviceFingerprint(),
    });
    expectNoIdentityOrCredentials(calls[0]);
  });

  it("khong goi :8083 hay NEXT_PUBLIC_API_URL", async () => {
    const { calls } = installFetch(() => json(TOKENS));
    await loginUser("a@example.com", "pw");
    for (const c of calls) {
      expect(c.url.startsWith(BASE)).toBe(true);
      expect(c.url).not.toContain("8083");
    }
  });

  it("luu access token, refresh token va {id, role, email} vao phien", async () => {
    installFetch(() => json(TOKENS));
    await loginUser("a@example.com", "pw");
    expect(localStorage.getItem("ef_token")).toBe("acc-new");
    expect(localStorage.getItem("ef_refresh")).toBe("ref-new");
    expect(JSON.parse(localStorage.getItem("ef_user") as string)).toEqual({
      id: "u-1",
      role: "buyer",
      email: "a@example.com",
    });
  });

  it("401 -> nem loi mang status/type cua problem va KHONG luu phien", async () => {
    installFetch(() => problem(401, "/problems/unauthorized", "sai mat khau"));
    await expect(loginUser("a@example.com", "bad")).rejects.toMatchObject({ status: 401 });
    expect(localStorage.getItem("ef_token")).toBeNull();
    expect(localStorage.getItem("ef_refresh")).toBeNull();
  });

  it("429 -> nem loi status 429", async () => {
    installFetch(() => problem(429, "/problems/rate-limited"));
    await expect(loginUser("a@example.com", "pw")).rejects.toMatchObject({ status: 429 });
  });
});

describe("registration (AC-2)", () => {
  it("requestRegistrationOTP: POST /api/v1/auth/register/request-otp {identifier}", async () => {
    const { calls } = installFetch(() => json({}));
    await requestRegistrationOTP("a@example.com");
    expect(calls[0].url).toBe(`${BASE}/api/v1/auth/register/request-otp`);
    expect(calls[0].method).toBe("POST");
    expect(calls[0].body).toEqual({ identifier: "a@example.com" });
  });

  it("verify 201 khong co token -> dang nhap tiep voi cung identifier/password va luu phien", async () => {
    const { calls } = installFetch((c) =>
      c.path === "/api/v1/auth/register/verify"
        ? json({ id: "u-1", email: "a@example.com", role: "buyer", created_at: "2026-01-01T00:00:00Z" }, 201)
        : json(TOKENS),
    );
    await verifyRegistrationOTP("a@example.com", "pw123456", "123456");
    expect(calls.map((c) => c.path)).toEqual(["/api/v1/auth/register/verify", "/api/v1/auth/login"]);
    expect(calls[0].body).toEqual({ identifier: "a@example.com", password: "pw123456", otp: "123456" });
    expect(calls[1].body).toEqual({
      identifier: "a@example.com",
      password: "pw123456",
      device_fingerprint: getDeviceFingerprint(),
    });
    expect(localStorage.getItem("ef_token")).toBe("acc-new");
    expect(localStorage.getItem("ef_refresh")).toBe("ref-new");
  });

  it("verify that bai (409) -> khong goi login, khong luu phien, nem loi", async () => {
    const { calls } = installFetch(() => problem(409, "/problems/conflict"));
    await expect(verifyRegistrationOTP("a@example.com", "pw", "000000")).rejects.toMatchObject({ status: 409 });
    expect(calls).toHaveLength(1);
    expect(localStorage.getItem("ef_token")).toBeNull();
  });
});

describe("forgot password (giu nguyen, doi sang gateway)", () => {
  it("request-otp va reset dung URL gateway", async () => {
    const { calls } = installFetch(() => json({}));
    await requestResetPasswordOTP("a@example.com");
    await resetPassword("a@example.com", "123456", "NewPw123");
    expect(calls[0].url).toBe(`${BASE}/api/v1/auth/forgot-password/request-otp`);
    expect(calls[0].body).toEqual({ identifier: "a@example.com" });
    expect(calls[1].url).toBe(`${BASE}/api/v1/auth/forgot-password/reset`);
    expect(calls[1].body).toEqual({ identifier: "a@example.com", otp: "123456", newPassword: "NewPw123" });
  });
});

describe("loginWithGoogle (AC-8)", () => {
  const GOOGLE = { ...TOKENS, access_token: "g-acc", token: "g-acc", refresh_token: "g-ref" };
  const REFRESHED = { ...TOKENS, access_token: "r-acc", token: "r-acc", refresh_token: "r-ref" };

  function route(c: Call) {
    switch (c.path) {
      case "/api/v1/auth/google":
        return json(GOOGLE);
      case "/api/v1/auth/devices":
        return json({ device_id: "d-1" });
      case "/api/v1/auth/refresh":
        return json(REFRESHED);
      default:
        return problem(404, "/problems/not-found");
    }
  }

  it("goi google -> devices {fingerprint} (Bearer access google) -> refresh {refresh_token, device_fingerprint}", async () => {
    const { calls } = installFetch(route);
    await loginWithGoogle("google-credential");
    expect(calls.map((c) => c.path)).toEqual([
      "/api/v1/auth/google",
      "/api/v1/auth/devices",
      "/api/v1/auth/refresh",
    ]);
    expect(calls[0].body).toEqual({ credential: "google-credential" });
    expect(calls[1].method).toBe("POST");
    expect(calls[1].body).toEqual({ fingerprint: getDeviceFingerprint() });
    expect(calls[1].headers.get("Authorization")).toBe("Bearer g-acc");
    expect(calls[2].body).toEqual({ refresh_token: "g-ref", device_fingerprint: getDeviceFingerprint() });
    for (const c of calls) expectNoIdentityOrCredentials(c);
  });

  it("luu cap token DA REFRESH (da gan thiet bi), khong phai cap google ban dau", async () => {
    installFetch(route);
    await loginWithGoogle("google-credential");
    expect(localStorage.getItem("ef_token")).toBe("r-acc");
    expect(localStorage.getItem("ef_refresh")).toBe("r-ref");
  });

  it("goi devices that bai -> van luu phien tu dang nhap google va khong nem", async () => {
    installFetch((c) => (c.path === "/api/v1/auth/devices" ? problem(500, "about:blank") : route(c)));
    await expect(loginWithGoogle("google-credential")).resolves.toBeDefined();
    expect(localStorage.getItem("ef_token")).toBe("g-acc");
    expect(localStorage.getItem("ef_refresh")).toBe("g-ref");
  });

  it("google 401 -> nem loi va khong luu phien", async () => {
    installFetch(() => problem(401, "/problems/unauthorized"));
    await expect(loginWithGoogle("bad")).rejects.toMatchObject({ status: 401 });
    expect(localStorage.getItem("ef_token")).toBeNull();
  });
});

describe("refreshSession", () => {
  it("POST /api/v1/auth/refresh voi refresh token dang luu va luu cap moi", async () => {
    seedSession("old", "r1");
    const { calls } = installFetch(() => json({ ...TOKENS, access_token: "n1", refresh_token: "n2" }));
    await refreshSession();
    expect(calls[0].url).toBe(`${BASE}/api/v1/auth/refresh`);
    expect(calls[0].body).toEqual({ refresh_token: "r1", device_fingerprint: getDeviceFingerprint() });
    expect(localStorage.getItem("ef_token")).toBe("n1");
    expect(localStorage.getItem("ef_refresh")).toBe("n2");
  });

  it("refresh bi tu choi (401) -> reject", async () => {
    seedSession("old", "r1");
    installFetch(() => problem(401, "/problems/unauthorized"));
    await expect(refreshSession()).rejects.toBeDefined();
  });
});

describe("logout (AC-6)", () => {
  it("POST /api/v1/auth/logout {refresh_token} roi xoa phien", async () => {
    seedSession("acc", "r1");
    const { calls } = installFetch(() => noContent());
    await logout();
    expect(calls[0].url).toBe(`${BASE}/api/v1/auth/logout`);
    expect(calls[0].method).toBe("POST");
    expect(calls[0].body).toEqual({ refresh_token: "r1" });
    expect(localStorage.getItem("ef_token")).toBeNull();
    expect(localStorage.getItem("ef_refresh")).toBeNull();
    expect(localStorage.getItem("ef_user")).toBeNull();
  });

  it("server loi 500 -> van xoa phien", async () => {
    seedSession("acc", "r1");
    installFetch(() => problem(500, "about:blank"));
    await logout().catch(() => undefined);
    expect(localStorage.getItem("ef_token")).toBeNull();
    expect(localStorage.getItem("ef_refresh")).toBeNull();
  });

  it("mat mang -> van xoa phien", async () => {
    seedSession("acc", "r1");
    installFetch(() => {
      throw new TypeError("Failed to fetch");
    });
    await logout().catch(() => undefined);
    expect(localStorage.getItem("ef_token")).toBeNull();
    expect(localStorage.getItem("ef_refresh")).toBeNull();
  });

  it("khong co phien -> khong nem loi", async () => {
    installFetch(() => noContent());
    await expect(logout()).resolves.not.toThrow();
  });
});

describe("fetchMe", () => {
  it("GET /api/v1/auth/me voi Bearer, tra {id, role}", async () => {
    seedSession("acc-9", "r1");
    const { calls } = installFetch(() => json({ id: "u-1", role: "organizer" }));
    const me = await fetchMe();
    expect(calls[0].url).toBe(`${BASE}/api/v1/auth/me`);
    expect(calls[0].method).toBe("GET");
    expect(calls[0].headers.get("Authorization")).toBe("Bearer acc-9");
    expect(me).toMatchObject({ id: "u-1", role: "organizer" });
  });
});
