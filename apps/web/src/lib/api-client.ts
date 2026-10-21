/** Wrapper fetch dung chung cho moi client goi qua gateway (AC-3, AC-4, AC-5).
 *
 * - URL = apiBase() + path. Co phien -> `Authorization: Bearer <access>`; khong co -> khong gui.
 * - Khong bao gio dung `credentials` va khong tu gui header danh tinh (gateway tu chen tu token).
 * - 401 `/problems/token-expired` -> refresh DUNG 1 lan (single-flight trong tab + Web Locks giua cac tab),
 *   roi retry request goc dung 1 lan. Refresh hong -> xoa phien, nem ApiError AUTH_REQUIRED.
 * - 401/403 cua queue token KHONG phai loi dang nhap -> khong refresh, tra nguyen Response.
 * - Truoc khi gui, neu exp cua access token <= 30s thi refresh chu dong (exp chi la goi y, khong tin de xac thuc). */

import { apiBase } from "@/lib/api-base";
import { getDeviceFingerprint } from "@/lib/device";
import {
  clearSession,
  getSession,
  getSessionPersist,
  sessionFromIdentity,
  setSession,
  type IdentityTokens,
} from "@/lib/session";

export interface Problem {
  type?: string;
  title?: string;
  status: number;
  detail?: string;
}

/** Loi API. `code` de nhanh logic (AUTH_REQUIRED, hoac `type` cua problem, hoac HTTP_<status>). */
export class ApiError extends Error {
  readonly code: string;
  readonly status: number;
  readonly type?: string;
  readonly title?: string;
  readonly detail?: string;

  constructor(code: string, status: number, problem: Partial<Problem> = {}, message?: string) {
    super(message ?? problem.detail ?? problem.title ?? code);
    this.name = "ApiError";
    this.code = code;
    this.status = status;
    this.type = problem.type;
    this.title = problem.title;
    this.detail = problem.detail;
  }
}

/** Doc RFC 7807 tu Response (khong tieu thu body cua ban goc). Body khong phai JSON -> chi co status. */
export async function readProblem(res: Response): Promise<Problem> {
  try {
    const text = await res.clone().text();
    const data = JSON.parse(text) as Record<string, unknown> | null;
    if (data && typeof data === "object") {
      return {
        type: typeof data.type === "string" ? data.type : undefined,
        title: typeof data.title === "string" ? data.title : undefined,
        status: typeof data.status === "number" ? data.status : res.status,
        detail: typeof data.detail === "string" ? data.detail : undefined,
      };
    }
  } catch {
    /* khong phai JSON */
  }
  return { status: res.status };
}

/** Chuyen Response loi thanh ApiError (code = problem type neu co). */
export async function toApiError(res: Response): Promise<ApiError> {
  const p = await readProblem(res);
  return new ApiError(p.type ?? `HTTP_${res.status}`, res.status, p);
}

export interface ApiFetchInit extends RequestInit {
  /** false: khong gui Bearer va khong refresh (login, register, du lieu cong khai). */
  auth?: boolean;
}

const TOKEN_EXPIRED = "/problems/token-expired";
const REFRESH_PATH = "/api/v1/auth/refresh";
const SKEW_SECONDS = 30;

function tokenExp(token: string): number | null {
  try {
    const parts = token.split(".");
    if (parts.length !== 3) return null;
    const b64 = parts[1].replace(/-/g, "+").replace(/_/g, "/");
    const payload = JSON.parse(atob(b64)) as { exp?: unknown };
    return typeof payload.exp === "number" ? payload.exp : null;
  } catch {
    return null;
  }
}

let inflight: Promise<string> | null = null;
/** Cap access token vua duoc refresh trong tab nay (de khong refresh chu dong lap lai khi dong ho lech). */
let lastRefreshed: { access: string; at: number } | null = null;

async function doRefresh(usedRefresh: string | null): Promise<string> {
  // Doc lai storage ngay truoc khi refresh: tab khac co the da xoay cap token (refresh token cu bi thu hoi).
  const cur = getSession();
  if (!cur || !cur.refreshToken) {
    clearSession();
    throw new ApiError("AUTH_REQUIRED", 401, {}, "Phien dang nhap da het han");
  }
  if (usedRefresh && cur.refreshToken !== usedRefresh) return cur.accessToken;

  let res: Response;
  try {
    res = await fetch(`${apiBase()}${REFRESH_PATH}`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ refresh_token: cur.refreshToken, device_fingerprint: getDeviceFingerprint() }),
    });
  } catch {
    // Mat mang: giu phien, de lan sau thu lai.
    throw new ApiError("NETWORK_ERROR", 0, {}, "Khong ket noi duoc may chu");
  }
  if (!res.ok) {
    if (res.status === 400 || res.status === 401 || res.status === 403) {
      clearSession();
      throw new ApiError("AUTH_REQUIRED", 401, await readProblem(res), "Phien dang nhap da het han");
    }
    throw await toApiError(res);
  }
  const next = sessionFromIdentity((await res.json()) as IdentityTokens, cur.user);
  if (!next || !next.refreshToken) {
    clearSession();
    throw new ApiError("AUTH_REQUIRED", 401, {}, "Phan hoi refresh khong hop le");
  }
  setSession(next, { persist: getSessionPersist() ?? "local" });
  lastRefreshed = { access: next.accessToken, at: Date.now() };
  return next.accessToken;
}

async function withRefreshLock(usedRefresh: string | null): Promise<string> {
  const locks = typeof navigator !== "undefined" ? (navigator as Navigator & { locks?: LockManager }).locks : undefined;
  if (locks && typeof locks.request === "function") {
    return locks.request("ef-auth-refresh", () => doRefresh(usedRefresh)) as Promise<string>;
  }
  return doRefresh(usedRefresh);
}

/** Refresh (hoac dung lai cap token tab khac vua xoay). Single-flight trong tab. Tra access token moi. */
export function refreshAccessToken(usedRefresh: string | null = getSession()?.refreshToken ?? null): Promise<string> {
  if (!inflight) {
    inflight = withRefreshLock(usedRefresh).finally(() => {
      inflight = null;
    });
  }
  return inflight;
}

function shouldRefreshEarly(access: string): boolean {
  const exp = tokenExp(access);
  if (exp === null) return false;
  if (exp * 1000 - Date.now() > SKEW_SECONDS * 1000) return false;
  if (lastRefreshed && lastRefreshed.access === access && Date.now() - lastRefreshed.at < SKEW_SECONDS * 1000) {
    return false;
  }
  return true;
}

function withBearer(init: RequestInit, token: string | null): RequestInit {
  const headers = new Headers(init.headers);
  if (token) headers.set("Authorization", `Bearer ${token}`);
  else headers.delete("Authorization");
  return { ...init, headers };
}

export async function apiFetch(path: string, init: ApiFetchInit = {}): Promise<Response> {
  const { auth = true, ...rest } = init;
  const url = `${apiBase()}${path}`;
  if (!auth) return fetch(url, rest);

  let session = getSession();
  if (session?.refreshToken && shouldRefreshEarly(session.accessToken)) {
    await refreshAccessToken(session.refreshToken);
    session = getSession();
  }
  const sentAccess = session?.accessToken ?? null;
  const sentRefresh = session?.refreshToken ?? null;
  const base: RequestInit = session ? { cache: "no-store", ...rest } : rest;
  const res = await fetch(url, withBearer(base, sentAccess));

  if (res.status !== 401 || !sentAccess) return res;
  if ((await readProblem(res)).type !== TOKEN_EXPIRED) return res;

  const fresh = await refreshAccessToken(sentRefresh);
  // Retry dung 1 lan; neu van 401 thi tra nguyen Response (khong refresh lan hai).
  return fetch(url, withBearer(base, fresh));
}
