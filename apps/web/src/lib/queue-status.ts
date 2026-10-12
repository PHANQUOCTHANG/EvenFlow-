/** Mot lan hoi trang thai hang cho (EV-182, BR-Q4).
 *
 * Khac `watchQueue` trong queue-client.ts (mot vong lap kin), module nay chi lam MOT request va
 * tra ve ket qua da phan loai. Vong lap, SSE va trang thai ket noi do hook `use-queue-status`
 * dieu phoi — tach ra de UI biet duoc ETag, nhip poll va loi ma khong phai doan.
 *
 * Nguyen tac: nhip poll la cua SERVER. Module nay chi lam sach gia tri server gui ve sao cho
 * mot gia tri hong (0, am, NaN, thieu) KHONG BAO GIO bien thanh poll nhanh hon. */

import type { QueueState } from "@/components/queue/queue-status-panel";
import { QueueError } from "@/lib/queue-client";

/** Nhip du phong khi server khong noi gi. Bang `pollMid` phia server — khong nhanh hon. */
export const DEFAULT_POLL_MS = 10_000;
/** San an toan: mot bug phia server tra `poll_after_ms: 1` khong duoc phep bien 300.000 tab
 *  thanh mot cuoc DDoS tu gay. */
export const MIN_POLL_MS = 1_000;
/** 503 khong co Retry-After: lui 5s, khop queue-client.ts. */
export const DEFAULT_RETRY_AFTER_MS = 5_000;

export interface QueueSnapshot {
  state: QueueState;
  /** null khi server tra -1 (LOBBY) hoac gia tri khong hop le. */
  rank: number | null;
  /** null khi server tra -1 (chua uoc luong duoc). KHONG tu tinh tu rank. */
  etaSeconds: number | null;
  pollAfterMs: number;
  /** Han suat mua (epoch ms) do server cap. */
  expiresAt: number | null;
}

export type StatusResult =
  | { kind: "updated"; snapshot: QueueSnapshot; etag: string | null; pollAfterMs: number }
  | { kind: "not-modified"; etag: string | null; pollAfterMs: number }
  | { kind: "throttled"; retryAfterMs: number };

const KNOWN_STATES: ReadonlySet<QueueState> = new Set<QueueState>([
  "LOBBY",
  "QUEUED",
  "ADMITTED",
  "EXPIRED",
  "SOLD_OUT",
  "UNKNOWN",
]);

/** Trang thai ket thuc: khong con gi de poll nua. */
export function isTerminalState(state: QueueState): boolean {
  return state === "ADMITTED" || state === "EXPIRED" || state === "SOLD_OUT" || state === "DROPPED";
}

/** Lam sach `poll_after_ms`. Khong hop le -> `fallback`; hop le nhung qua nho -> kep len MIN_POLL_MS. */
export function sanitizePollAfterMs(value: unknown, fallback: number = DEFAULT_POLL_MS): number {
  const n = typeof value === "string" && value.trim() !== "" ? Number(value) : value;
  if (typeof n !== "number" || !Number.isFinite(n) || n <= 0) return fallback;
  return Math.max(MIN_POLL_MS, Math.floor(n));
}

/** `Retry-After` theo giay (RFC 9110). Dang HTTP-date hiem gap o day nen roi ve mac dinh. */
export function parseRetryAfterMs(header: string | null): number {
  if (header === null || header.trim() === "") return DEFAULT_RETRY_AFTER_MS;
  const seconds = Number(header);
  if (!Number.isFinite(seconds) || seconds <= 0) return DEFAULT_RETRY_AFTER_MS;
  return Math.max(MIN_POLL_MS, Math.floor(seconds * 1000));
}

function nonNegativeInt(value: unknown): number | null {
  return typeof value === "number" && Number.isInteger(value) && value >= 0 ? value : null;
}

/** Chuan hoa payload `domain.Position` cua waitingroom thanh snapshot an toan cho UI. */
export function normalizePosition(raw: unknown): QueueSnapshot {
  const obj = (typeof raw === "object" && raw !== null ? raw : {}) as Record<string, unknown>;
  const rawState = typeof obj.state === "string" ? (obj.state as QueueState) : "UNKNOWN";
  const state: QueueState = KNOWN_STATES.has(rawState) ? rawState : "UNKNOWN";

  const eta = obj.eta_seconds;
  const expiresAt = obj.expires_at;

  return {
    state,
    // BR-Q1: o LOBBY khong co rank, ke ca khi payload lo co so.
    rank: state === "LOBBY" ? null : nonNegativeInt(obj.rank),
    etaSeconds: typeof eta === "number" && Number.isFinite(eta) && eta >= 0 ? eta : null,
    pollAfterMs: sanitizePollAfterMs(obj.poll_after_ms),
    expiresAt:
      typeof expiresAt === "number" && Number.isFinite(expiresAt) && expiresAt > 0 ? expiresAt : null,
  };
}

async function safeDetail(res: Response): Promise<string> {
  try {
    const body = (await res.json()) as { detail?: string; title?: string };
    return body.detail ?? body.title ?? res.statusText;
  } catch {
    return res.statusText;
  }
}

export interface FetchQueueStatusOptions {
  etag?: string | null;
  signal?: AbortSignal;
  baseUrl?: string;
}

/** GET `/v1/events/{eventId}/queue/status` dung MOT lan.
 *
 * - 200 -> `updated` (kem ETag moi va nhip poll da lam sach)
 * - 304 -> `not-modified` (nhip lay tu `X-Poll-After-Ms`)
 * - 503 -> `throttled` (ton trong `Retry-After`)
 * - con lai -> nem `QueueError`; loi mang -> nem nguyen loi goc de hook phan biet. */
export async function fetchQueueStatus(
  eventId: string,
  token: string,
  opts: FetchQueueStatusOptions = {},
): Promise<StatusResult> {
  const base = opts.baseUrl ?? process.env.NEXT_PUBLIC_API_BASE ?? "";
  const headers: Record<string, string> = { "X-Queue-Token": token };
  if (opts.etag) headers["If-None-Match"] = opts.etag;

  const res = await fetch(`${base}/v1/events/${encodeURIComponent(eventId)}/queue/status`, {
    headers,
    credentials: "include",
    cache: "no-store",
    signal: opts.signal,
  });

  const etag = res.headers.get("ETag") ?? opts.etag ?? null;

  if (res.status === 304) {
    return {
      kind: "not-modified",
      etag,
      pollAfterMs: sanitizePollAfterMs(res.headers.get("X-Poll-After-Ms")),
    };
  }

  if (res.status === 503) {
    return { kind: "throttled", retryAfterMs: parseRetryAfterMs(res.headers.get("Retry-After")) };
  }

  if (!res.ok) {
    throw new QueueError(res.status, await safeDetail(res));
  }

  const snapshot = normalizePosition(await res.json());
  return { kind: "updated", snapshot, etag, pollAfterMs: snapshot.pollAfterMs };
}
