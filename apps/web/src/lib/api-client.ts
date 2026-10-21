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

const LOCK_WAIT_MS = 10_000;

function pairKey(s: { accessToken: string; refreshToken: string | null }): string {
  return JSON.stringify([s.accessToken, s.refreshToken ?? ""]);
}

/** Danh dau "dang refresh" cheo tab (G-1): tab dang POST /refresh ghi danh dau truoc khi gui; tab khac cuop khoa
 *  thay danh dau con moi cho cung refresh token thi cho ket qua thay vi POST lan hai (tranh reuse -> thu hoi ca ho token). */
const MARKER_KEY = "ef_refresh_inflight";
const MARKER_TTL_MS = 30_000;
const POLL_MS = 250;
const HARD_DEADLINE_MS = 45_000;

function writeMarker(rt: string): string {
  const id = Math.random().toString(36).slice(2);
  try {
    localStorage.setItem(MARKER_KEY, JSON.stringify({ rt, at: Date.now(), id }));
  } catch {
    /* bo qua */
  }
  return id;
}

function clearMarker(id: string): void {
  try {
    const raw = localStorage.getItem(MARKER_KEY);
    if (raw && (JSON.parse(raw) as { id?: string }).id === id) localStorage.removeItem(MARKER_KEY);
  } catch {
    /* bo qua */
  }
}

function markerFresh(rt: string): boolean {
  try {
    const raw = localStorage.getItem(MARKER_KEY);
    if (!raw) return false;
    const m = JSON.parse(raw) as { rt?: string; at?: number };
    return m.rt === rt && typeof m.at === "number" && Date.now() - m.at < MARKER_TTL_MS;
  } catch {
    return false;
  }
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
  const markerId = writeMarker(cur.refreshToken);
  try {
    return await refreshCore(cur, cur.refreshToken);
  } finally {
    clearMarker(markerId);
  }
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

const TIMED_OUT = Symbol("lock-timeout");
const isAbort = (err: unknown): boolean => (err as { name?: string } | null)?.name === "AbortError";
const LOCK_NAME = "ef-auth-refresh";

/** Cho cap token doi (tab khac xoay xong) khi danh dau "dang refresh" con moi. Tra access token moi, hoac null
 *  (danh dau het/het han, phien mat, hoac qua han) -> nguoi goi quyet dinh. */
async function waitForRotation(usedRefresh: string | null, abandoned: () => boolean): Promise<string | null> {
  while (!abandoned()) {
    const cur = getSession();
    if (!cur || !cur.refreshToken) return null;
    if (usedRefresh && cur.refreshToken !== usedRefresh) return cur.accessToken;
    if (!markerFresh(cur.refreshToken)) return null;
    await new Promise((r) => setTimeout(r, POLL_MS));
  }
  return null;
}

/** Refresh luon nam trong Web Lock (G-1). Qua LOCK_WAIT_MS ma chua co khoa: doc lai storage (cap doi -> dung),
 *  khong thi cuop khoa (`steal`) va chi POST khi khong tab nao dang refresh cung token; qua HARD_DEADLINE_MS
 *  -> NETWORK_ERROR (khong xoa phien). */
async function withRefreshLock(usedRefresh: string | null): Promise<string> {
  const locks = typeof navigator !== "undefined" ? (navigator as Navigator & { locks?: LockManager }).locks : undefined;
  if (!(locks && typeof locks.request === "function")) return doRefresh(usedRefresh);

  let abandoned = false;
  let granted = false;
  const ac = typeof AbortController !== "undefined" ? new AbortController() : null;
  let waitTimer: ReturnType<typeof setTimeout> | undefined;
  let deadlineTimer: ReturnType<typeof setTimeout> | undefined;
  const deadline = new Promise<never>((_, reject) => {
    deadlineTimer = setTimeout(() => {
      abandoned = true;
      ac?.abort();
      reject(networkError());
    }, HARD_DEADLINE_MS);
  });
  deadline.catch(() => undefined);
  const timeout = new Promise<typeof TIMED_OUT>((resolve) => {
    waitTimer = setTimeout(() => {
      if (granted) return;
      ac?.abort();
      resolve(TIMED_OUT);
    }, LOCK_WAIT_MS);
  });

  const run = async (): Promise<string> => {
    let mine: Promise<string> | null = null;
    const locked = Promise.resolve(
      locks.request(LOCK_NAME, ac ? { signal: ac.signal } : {}, () => {
        granted = true;
        clearTimeout(waitTimer);
        if (abandoned) return Promise.reject(networkError());
        mine = doRefresh(usedRefresh);
        return mine;
      }),
    ).catch(async (err: unknown): Promise<string | typeof TIMED_OUT> => {
      if (isAbort(err)) {
        if (!granted) return TIMED_OUT;
        // Khoa bi cuop khi dang refresh: refresh cua chinh minh van chay tiep, cho no xong.
        if (mine) return mine;
      }
      throw err;
    });
    const out = await Promise.race([locked as Promise<string | typeof TIMED_OUT>, timeout]);
    if (out !== TIMED_OUT) return out;

    // Het han cho khoa: tab khac co the da xoay cap token.
    const cur = getSession();
    if (cur?.refreshToken && usedRefresh && cur.refreshToken !== usedRefresh) return cur.accessToken;
    try {
      return await Promise.resolve(
        locks.request(LOCK_NAME, { steal: true }, async () => {
          if (abandoned) throw networkError();
          const rotated = await waitForRotation(usedRefresh, () => abandoned);
          if (rotated) return rotated;
          if (abandoned) throw networkError();
          return doRefresh(usedRefresh);
        }),
      );
    } catch (err) {
      if (isAbort(err)) {
        // Bi tab khac cuop khoa: cho no xoay xong thay vi POST them.
        const rotated = await waitForRotation(usedRefresh, () => abandoned);
        if (rotated) return rotated;
        throw networkError();
      }
      throw err;
    }
  };

  try {
    return await Promise.race([run(), deadline]);
  } finally {
    clearTimeout(waitTimer);
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
