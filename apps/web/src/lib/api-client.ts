/** Wrapper fetch dung chung cho moi client goi qua gateway (AC-3, AC-4, AC-5).
 *
 * - URL = apiBase() + path. Co phien -> `Authorization: Bearer <access>`; khong co -> khong gui.
 * - Khong bao gio dung `credentials` va khong tu gui header danh tinh (gateway tu chen tu token).
 * - 401 `/problems/token-expired` -> refresh DUNG 1 lan (single-flight trong tab + Web Locks giua cac tab, khong timeout POST),
 *   roi retry request goc dung 1 lan. Refresh hong -> xoa phien, nem ApiError AUTH_REQUIRED.
 * - 401/403 cua queue token KHONG phai loi dang nhap -> khong refresh, tra nguyen Response.
 * - Truoc khi gui, neu exp cua access token <= 30s thi refresh chu dong (exp chi la goi y, khong tin de xac thuc). */

import { apiBase } from "@/lib/api-base";
import { getDeviceFingerprint } from "@/lib/device";
import {
  clearSession,
  getSession,
  getSessionEpoch,
  getSessionPersist,
  sessionFromIdentity,
  setSession,
  type IdentityTokens,
  type Session,
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
/** Lan thu refresh chu dong gan nhat cho TUNG CAP token (access+refresh), thanh cong hay that bai:
 *  tranh lap lai trong 30 s khi cung mot cap con trong storage. Cap khac (tab khac xoay) -> khong bi chan. */
let lastProactive: { pair: string; at: number } | null = null;

const HARD_DEADLINE_MS = 45_000;
const LOCK_NAME = "ef-auth-refresh";
/** SecurityError tu Web Locks: ca tab dung single-flight trong tab, khong thu lai navigator.locks. */
let locksUnusable = false;

function pairKey(s: { accessToken: string; refreshToken: string | null }): string {
  return JSON.stringify([s.accessToken, s.refreshToken ?? ""]);
}

const networkError = () => new ApiError("NETWORK_ERROR", 0, {}, "Khong ket noi duoc may chu");

async function doRefresh(usedRefresh: string | null): Promise<string> {
  // Doc lai storage ngay truoc khi refresh: tab khac co the da xoay cap token (refresh token cu bi thu hoi).
  const cur = getSession();
  if (!cur || !cur.refreshToken) {
    clearSession();
    throw new ApiError("AUTH_REQUIRED", 401, {}, "Phien dang nhap da het han");
  }
  if (usedRefresh && cur.refreshToken !== usedRefresh) return cur.accessToken;
  return refreshCore(cur, cur.refreshToken);
}

async function refreshCore(cur: Session, refreshToken: string): Promise<string> {
  const epoch = getSessionEpoch();
  let res: Response;
  try {
    res = await fetch(`${apiBase()}${REFRESH_PATH}`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ refresh_token: refreshToken, device_fingerprint: getDeviceFingerprint() }),
    });
  } catch {
    // Mat mang: giu phien, de lan sau thu lai.
    throw new ApiError("NETWORK_ERROR", 0, {}, "Khong ket noi duoc may chu");
  }
  if (!res.ok) {
    if (res.status === 400 || res.status === 401 || res.status === 403) {
      // Dang xuat/xoay trong luc cho: khong xoa phien cua nguoi khac.
      if (epoch === getSessionEpoch() && getSession()?.refreshToken === cur.refreshToken) clearSession();
      throw new ApiError("AUTH_REQUIRED", 401, await readProblem(res), "Phien dang nhap da het han");
    }
    throw await toApiError(res);
  }
  const body = (await res.json()) as IdentityTokens;
  // F-12: dang xuat (cung tab hoac tab khac) trong luc refresh dang bay -> bo ket qua, khong hoi sinh phien.
  const after = getSession();
  if (epoch !== getSessionEpoch() || !after) {
    throw new ApiError("AUTH_REQUIRED", 401, {}, "Phien dang nhap da ket thuc");
  }
  if (after.refreshToken !== cur.refreshToken) return after.accessToken; // tab khac da xoay cap moi
  const next = sessionFromIdentity(body, cur.user);
  if (!next || !next.refreshToken) {
    clearSession();
    throw new ApiError("AUTH_REQUIRED", 401, {}, "Phan hoi refresh khong hop le");
  }
  setSession(next, { persist: getSessionPersist() ?? "local" });
  return next.accessToken;
}

/** Refresh luon nam trong Web Lock (H-1). Khoa do trinh duyet tha khi tab dong/crash nen khoa dang giu luon cua tab
 *  song: ben trong khoa doc lai storage (cap khac -> dung luon, khong POST), neu khong POST /refresh dung 1 lan,
 *  KHONG timeout/abort POST (abort sau khi server da xoay se gay reuse -> thu hoi ca ho token).
 *  Cho khoa co han cung HARD_DEADLINE_MS (AbortSignal): het han -> NETWORK_ERROR, khong clearSession, khong POST;
 *  yeu cau da bo khong bao gio duoc cap khoa de POST. */
async function withRefreshLock(usedRefresh: string | null): Promise<string> {
  const locks = typeof navigator !== "undefined" ? (navigator as Navigator & { locks?: LockManager }).locks : undefined;
  if (locksUnusable || !(locks && typeof locks.request === "function")) return doRefresh(usedRefresh);

  let abandoned = false;
  let granted = false;
  const ac = typeof AbortController !== "undefined" ? new AbortController() : null;
  let deadlineTimer: ReturnType<typeof setTimeout> | undefined;
  const deadline = new Promise<never>((_, reject) => {
    deadlineTimer = setTimeout(() => {
      if (granted) return; // da co khoa: refresh cua chinh minh chay den het, khong bo giua chung
      abandoned = true;
      ac?.abort();
      reject(networkError());
    }, HARD_DEADLINE_MS);
  });
  deadline.catch(() => undefined);

  // Loi tu Web Locks (reject hoac nem dong bo): khong bao gio lo ra loi tho cho caller (AC-3).
  const onLockError = async (err: unknown): Promise<string> => {
    if (err instanceof ApiError) throw err;
    const name = (err as { name?: string } | null)?.name;
    if (name === "SecurityError" && !granted && !abandoned) {
      // Khong tab nao khac co the chung khoa trong ngu canh nay: single-flight trong tab. Go han cho khoa truoc khi POST.
      granted = true;
      clearTimeout(deadlineTimer);
      locksUnusable = true;
      return doRefresh(usedRefresh);
    }
    throw networkError();
  };

  let locked: Promise<string>;
  try {
    locked = Promise.resolve(
      locks.request(LOCK_NAME, ac ? { signal: ac.signal } : {}, () => {
        if (abandoned) throw networkError();
        granted = true;
        clearTimeout(deadlineTimer);
        return doRefresh(usedRefresh);
      }),
    ).catch(onLockError) as Promise<string>;
  } catch (err) {
    locked = onLockError(err);
  }
  locked.catch(() => undefined);

  try {
    return await Promise.race([locked, deadline]);
  } finally {
    clearTimeout(deadlineTimer);
  }
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

/** Han access token (ms, dong ho may khach): uu tien `expiresAt` luu luc nhan (Date.now()+expires_in), chi dung exp JWT khi thieu. */
function expiryMs(s: { accessToken: string; expiresAt?: number }): number | null {
  if (typeof s.expiresAt === "number") return s.expiresAt;
  const exp = tokenExp(s.accessToken);
  return exp === null ? null : exp * 1000;
}

function shouldRefreshEarly(s: { accessToken: string; refreshToken: string | null; expiresAt?: number }): boolean {
  const exp = expiryMs(s);
  if (exp === null) return false;
  if (exp - Date.now() > SKEW_SECONDS * 1000) return false;
  if (lastProactive && lastProactive.pair === pairKey(s) && Date.now() - lastProactive.at < SKEW_SECONDS * 1000) {
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
  if (session?.refreshToken && shouldRefreshEarly(session)) {
    lastProactive = { pair: pairKey(session), at: Date.now() };
    try {
      await refreshAccessToken(session.refreshToken);
    } catch (err) {
      // Loi tam thoi (mang/5xx/429): han chi la goi y -> van gui request voi token hien tai, 401 se duoc xu ly sau.
      if (err instanceof ApiError && err.code === "AUTH_REQUIRED") throw err;
    }
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
