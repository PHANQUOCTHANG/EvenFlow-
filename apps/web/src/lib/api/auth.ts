/** Client dinh danh qua gateway: `${NEXT_PUBLIC_API_BASE}/api/v1/auth/...` (AC-1, AC-2, AC-6, AC-8, AC-27).
 *
 * Moi loi HTTP duoc nem thanh ApiError (lib/api-client) mang `status`, `type`, `detail` cua problem. */

import { apiFetch, refreshAccessToken, toApiError } from "@/lib/api-client";
import { getDeviceFingerprint } from "@/lib/device";
import {
  clearSession,
  getSession,
  getSessionPersist,
  getStoredRefreshToken,
  sessionFromIdentity,
  setSession,
  type Session,
  type SessionPersist,
} from "@/lib/session";

export interface IdentityResponse {
  id: string;
  email?: string;
  role: string;
  created_at: string;
  token?: string;
  access_token?: string;
  token_type?: string;
  expires_in?: number;
  refresh_token?: string;
  refresh_expires_in?: number;
}

export interface LoginOptions {
  /** "ghi nho toi" tat -> "session" (sessionStorage). Mac dinh "local". */
  persist?: SessionPersist;
}

const AUTH = "/api/v1/auth";

async function postJson(path: string, body: unknown, auth = false): Promise<Response> {
  return apiFetch(`${AUTH}${path}`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
    auth,
  });
}

async function expectOk(res: Response): Promise<Response> {
  if (!res.ok) throw await toApiError(res);
  return res;
}

function storeFromIdentity(data: IdentityResponse, persist?: SessionPersist): Session {
  const session = sessionFromIdentity(data);
  if (!session) throw new Error("Phan hoi dang nhap khong co access token");
  setSession(session, { persist });
  return session;
}

export async function requestRegistrationOTP(identifier: string): Promise<void> {
  await expectOk(await postJson("/register/request-otp", { identifier }));
}

/** Dang ky: verify tra 201 KHONG co token -> dang nhap tiep voi cung identifier/password (AC-2). */
export async function verifyRegistrationOTP(
  identifier: string,
  password: string,
  otp: string,
  opts: LoginOptions = {},
): Promise<IdentityResponse> {
  await expectOk(await postJson("/register/verify", { identifier, password, otp }));
  return loginUser(identifier, password, opts);
}

export async function loginUser(identifier: string, password: string, opts: LoginOptions = {}): Promise<IdentityResponse> {
  // Backend gan phien voi thiet bi; phong cho yeu cau thiet bi moi cho xep hang (EV-113 / EV-137).
  const res = await expectOk(await postJson("/login", { identifier, password, device_fingerprint: getDeviceFingerprint() }));
  const data = (await res.json()) as IdentityResponse;
  storeFromIdentity(data, opts.persist);
  return data;
}

export async function requestResetPasswordOTP(identifier: string): Promise<void> {
  await expectOk(await postJson("/forgot-password/request-otp", { identifier }));
}

export async function resetPassword(identifier: string, otp: string, newPassword: string): Promise<void> {
  await expectOk(await postJson("/forgot-password/reset", { identifier, otp, newPassword }));
}

/** Google khong gan thiet bi -> goi /devices roi /refresh de lay cap token co `did` (AC-8). */
export async function loginWithGoogle(credential: string, opts: LoginOptions = {}): Promise<IdentityResponse> {
  const res = await expectOk(await postJson("/google", { credential }));
  const data = (await res.json()) as IdentityResponse;
  const googleSession = storeFromIdentity(data, opts.persist);

  try {
    await expectOk(await postJson("/devices", { fingerprint: getDeviceFingerprint() }, true));
    await refreshSession();
  } catch {
    // Gan thiet bi that bai: giu phien tu dang nhap Google (join queue se bao device-required).
    setSession(googleSession, { persist: opts.persist ?? getSessionPersist() ?? "local" });
  }
  return data;
}

/** Doi refresh token lay cap token moi (AC-4). Reject neu bi tu choi. */
export async function refreshSession(): Promise<Session | null> {
  await refreshAccessToken();
  return getSession();
}

/** Dang xuat: bao server thu hoi refresh token, nhung LUON xoa phien du loi (AC-6). */
export async function logout(): Promise<void> {
  // Doc truc tiep tu storage: ef_user hong van khong duoc bo qua viec thu hoi refresh token (F-15).
  const refreshToken = getStoredRefreshToken();
  try {
    if (refreshToken) await postJson("/logout", { refresh_token: refreshToken });
  } catch {
    /* mat mang / 5xx: van dang xuat o client */
  } finally {
    clearSession();
  }
}

export async function fetchMe(): Promise<{ id: string; role: string }> {
  const res = await expectOk(await apiFetch(`${AUTH}/me`, { method: "GET" }));
  return (await res.json()) as { id: string; role: string };
}
