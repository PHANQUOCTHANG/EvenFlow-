/** Wrapper fetch dung chung cho moi client goi qua gateway (AC-3, AC-4, AC-5).
 *
 * - URL = apiBase() + path. Co phien -> `Authorization: Bearer <access>`; khong co -> khong gui.
 * - Khong bao gio dung `credentials` va khong tu gui header danh tinh (gateway tu chen tu token).
 * - 401 `/problems/token-expired` -> refresh DUNG 1 lan (single-flight trong tab + Web Locks giua cac tab; POST bi huy sau 120 s),
 *   roi retry request goc dung 1 lan. Refresh hong -> xoa phien, nem ApiError AUTH_REQUIRED.
 * - 401/403 cua queue token KHONG phai loi dang nhap -> khong refresh, tra nguyen Response.
 * - Truoc khi gui, neu exp cua access token <= 30s thi refresh chu dong (exp chi la goi y, khong tin de xac thuc). */

import { apiBase } from "@/lib/api-base";
import { getDeviceFingerprint } from "@/lib/device";
import {
  clearSession,
  getSession,
  peekSession,
  getSessionEpoch,
  getSessionPersist,
  sessionFromIdentity,
  setSession,
  type IdentityTokens,
  type Session,
} from "@/lib/session";

// Lop mang dung peekSession (khong ghi gi); khi nap module ghi nhan chu so huu luong mua ve cua tab (ef:lastuid) mot lan.
if (typeof window !== "undefined") getSession();

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

/** Refresh dang bay, khoa theo user da gui request (khong chia se giua cac user khac nhau trong cung tab). */
const inflight = new Map<string, Promise<string>>();
/** Lan thu refresh chu dong gan nhat cho TUNG CAP token (access+refresh), thanh cong hay that bai:
 *  tranh lap lai trong 30 s khi cung mot cap con trong storage. Cap khac (tab khac xoay) -> khong bi chan. */
let lastProactive: { pair: string; at: number } | null = null;

const HARD_DEADLINE_MS = 45_000;
/** POST /refresh bi huy sau 120 s (server co the da xoay token -> khong bao gio POST lai token cu). */
export const REFRESH_POST_TIMEOUT_MS = 120_000;
const LOCK_NAME = "ef-auth-refresh";
/** SecurityError tu Web Locks: ca tab dung single-flight trong tab, khong thu lai navigator.locks. */
let locksUnusable = false;

function pairKey(s: { accessToken: string; refreshToken: string | null }): string {
  return JSON.stringify([s.accessToken, s.refreshToken ?? ""]);
}

const networkError = () => new ApiError("NETWORK_ERROR", 0, {}, "Khong ket noi duoc may chu");

const userChanged = () => new ApiError("AUTH_REQUIRED", 401, {}, "Phien dang nhap da chuyen sang nguoi dung khac");

async function doRefresh(usedRefresh: string | null, sentUserId: string | null): Promise<string> {
  // Doc lai storage ngay truoc khi refresh: tab khac co the da xoay cap token (refresh token cu bi thu hoi).
  const cur = peekSession();
  // Request goc gui bang user A; tab khac da doi sang user B -> khong dung token cua B, khong xoa phien cua B, khong POST.
  // Kiem tra user TRUOC nhanh "khong co refresh token" (nhanh do xoa phien).
  if (cur && sentUserId !== null && cur.user.id !== sentUserId) throw userChanged();
  if (!cur || !cur.refreshToken) {
    clearSession();
    throw new ApiError("AUTH_REQUIRED", 401, {}, "Phien dang nhap da het han");
  }
  if (usedRefresh && cur.refreshToken !== usedRefresh) return cur.accessToken;
  return refreshCore(cur, cur.refreshToken);
}

/** Chi xoa phien khi no van la phien da POST: cung epoch, cung refresh token VA cung user id. */
function isSamePostedSession(epoch: number, cur: Session): boolean {
  const stored = peekSession();
  return epoch === getSessionEpoch() && stored?.refreshToken === cur.refreshToken && stored?.user.id === cur.user.id;
}

async function refreshCore(cur: Session, refreshToken: string): Promise<string> {
  const epoch = getSessionEpoch();
  const controller = new AbortController();
  const abortTimer = setTimeout(() => controller.abort(), REFRESH_POST_TIMEOUT_MS);
  try {
    return await refreshPost(cur, refreshToken, epoch, controller);
  } catch (err) {
    if (controller.signal.aborted) {
      // Server co the da xoay token roi: khong bao gio POST lai. Chi xoa neu phien van la phien da POST.
      if (isSamePostedSession(epoch, cur)) clearSession();
      throw new ApiError("AUTH_REQUIRED", 401, {}, "Phien dang nhap da het han");
    }
    if (err instanceof ApiError) throw err;
    throw networkError();
  } finally {
    clearTimeout(abortTimer);
  }
}

async function refreshPost(cur: Session, refreshToken: string, epoch: number, controller: AbortController): Promise<string> {
  let res: Response;
  try {
    res = await fetch(`${apiBase()}${REFRESH_PATH}`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ refresh_token: refreshToken, device_fingerprint: getDeviceFingerprint() }),
      signal: controller.signal,
    });
  } catch {
    // Mat mang: giu phien, de lan sau thu lai.
    throw networkError();
  }
  if (!res.ok) {
    if (res.status === 400 || res.status === 401 || res.status === 403) {
      // Dang xuat/xoay trong luc cho: khong xoa phien cua nguoi khac.
      if (isSamePostedSession(epoch, cur)) clearSession();
      throw new ApiError("AUTH_REQUIRED", 401, await readProblem(res), "Phien dang nhap da het han");
    }
    throw await toApiError(res);
  }
  const body = (await res.json()) as IdentityTokens;
  // F-12: dang xuat (cung tab hoac tab khac) trong luc refresh dang bay -> bo ket qua, khong hoi sinh phien.
  const after = peekSession();
  if (epoch !== getSessionEpoch() || !after) {
    throw new ApiError("AUTH_REQUIRED", 401, {}, "Phien dang nhap da ket thuc");
  }
  if (after.user.id !== cur.user.id) throw userChanged(); // tab khac doi user trong luc POST: khong tra token cua nguoi khac
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
 *  POST chi bi huy sau REFRESH_POST_TIMEOUT_MS va khi do KHONG bao gio POST lai token cu (reuse -> thu hoi ca ho token).
 *  Cho khoa co han cung HARD_DEADLINE_MS (AbortSignal): het han -> NETWORK_ERROR, khong clearSession, khong POST;
 *  yeu cau da bo khong bao gio duoc cap khoa de POST. */
async function withRefreshLock(usedRefresh: string | null, sentUserId: string | null): Promise<string> {
  const locks = typeof navigator !== "undefined" ? (navigator as Navigator & { locks?: LockManager }).locks : undefined;
  if (locksUnusable || !(locks && typeof locks.request === "function")) return doRefresh(usedRefresh, sentUserId);

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
      return doRefresh(usedRefresh, sentUserId);
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
        return doRefresh(usedRefresh, sentUserId);
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

/** Nguoi tham gia (khong so huu POST) cho refresh dang bay toi da HARD_DEADLINE_MS: het han -> NETWORK_ERROR, giu phien.
 *  Promise dung chung khong bi anh huong (inflight chi xoa khi no tu settle), nen khong co POST thu hai. */
function joinInflight(p: Promise<string>): Promise<string> {
  return new Promise<string>((resolve, reject) => {
    const timer = setTimeout(() => reject(networkError()), HARD_DEADLINE_MS);
    p.then(
      (v) => {
        clearTimeout(timer);
        resolve(v);
      },
      (e: unknown) => {
        clearTimeout(timer);
        reject(e);
      },
    );
  });
}

/** Refresh (hoac dung lai cap token tab khac vua xoay). Single-flight trong tab. Tra access token moi.
 *  Nguoi goi dau tien so huu POST (cho toi da REFRESH_POST_TIMEOUT_MS); nguoi vao sau cho toi da 45 s. */
export function refreshAccessToken(
  usedRefresh: string | null = peekSession()?.refreshToken ?? null,
  userId: string | null = peekSession()?.user.id ?? null,
): Promise<string> {
  const key = userId ?? "";
  const existing = inflight.get(key);
  if (existing) return joinInflight(existing);
  const p: Promise<string> = withRefreshLock(usedRefresh, userId).finally(() => {
    if (inflight.get(key) === p) inflight.delete(key);
  });
  inflight.set(key, p);
  return p;
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

  let session = peekSession();
  const startUserId = session?.user.id ?? null;
  if (session?.refreshToken && shouldRefreshEarly(session)) {
    lastProactive = { pair: pairKey(session), at: Date.now() };
    try {
      await refreshAccessToken(session.refreshToken, session.user.id);
    } catch (err) {
      // Loi tam thoi (mang/5xx/429): han chi la goi y -> van gui request voi token hien tai, 401 se duoc xu ly sau.
      if (err instanceof ApiError && err.code === "AUTH_REQUIRED") throw err;
    }
    session = peekSession();
    // Doi user trong luc refresh (ke ca khi loi tam thoi bi nuot): khong gui request cua A bang token cua B.
    if (session && session.user.id !== startUserId) throw userChanged();
  }
  const sentAccess = session?.accessToken ?? null;
  const sentRefresh = session?.refreshToken ?? null;
  const sentUserId = session?.user.id ?? null;
  const base: RequestInit = session ? { cache: "no-store", ...rest } : rest;
  const res = await fetch(url, withBearer(base, sentAccess));

  if (res.status !== 401 || !sentAccess) return res;
  if ((await readProblem(res)).type !== TOKEN_EXPIRED) return res;

  const fresh = await refreshAccessToken(sentRefresh, sentUserId);
  // Retry dung 1 lan; neu van 401 thi tra nguyen Response (khong refresh lan hai).
  return fetch(url, withBearer(base, fresh));
}
