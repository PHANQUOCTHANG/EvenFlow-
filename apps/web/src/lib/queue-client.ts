/**
 * Client phong cho.
 *
 * Hai quyet dinh thiet ke o day co anh huong truc tiep toi kha nang song sot
 * cua backend tai giay mo ban:
 *
 * 1. JITTER (Tang 2 trong docs/02). Client hoan viec goi join mot khoang ngau
 *    nhien 0..5s sau T0. Nho co lottery (BR-Q1), hoan KHONG lam giam co hoi cua
 *    khach — nen day khong phai la mot su hy sinh ma khach phai chap nhan, va
 *    cung khong ai co dong co "lach" bang cach bo qua no.
 *    Ket qua: dinh 300k rps trong 1 giay bi trai thanh ~60k rps trong 5 giay.
 *
 * 2. NHIP POLL DO SERVER QUYET DINH (BR-Q4). Client KHONG tu chon chu ky poll.
 *    Moi phan hoi deu kem poll_after_ms, va client tuan thu. Neu de client tu
 *    chon, moi tab se poll moi giay va 300.000 nguoi cho se tao ra 300k rps
 *    lien tuc suot 30 phut — gap nhieu lan chinh cai dinh tai luc mo ban.
 *    Phan poll nam o `lib/queue-status.ts` (mot request) + `hooks/use-queue-status.ts`
 *    (vong lap, backoff, dung han khi gap 4xx).
 */

import { apiFetch, readProblem } from "@/lib/api-client";

export type QueueState = "LOBBY" | "QUEUED" | "ADMITTED" | "EXPIRED" | "SOLD_OUT" | "UNKNOWN";

export interface JoinResponse {
  state: QueueState;
  /** Rong khi state = SOLD_OUT: het ve han thi khach moi khong duoc ghi danh nen khong co token (EV-146). */
  queue_token: string;
  rank: number;
  is_new: boolean;
  poll_after_ms: number;
}

const MAX_JITTER_MS = 5000;

const sleep = (ms: number) => new Promise((r) => setTimeout(r, ms));

/** Hoan ngau nhien 0..5s de trai phang dinh tai tai T0. */
export function jitterDelay(maxMs = MAX_JITTER_MS): number {
  return Math.floor(Math.random() * maxMs);
}

/**
 * Ghi danh vao phong cho.
 *
 * Idempotent phia server (BR-Q3): goi lai tu tab khac tra ve dung token va rank
 * cu, nen client co the retry thoai mai ma khong so mat cho.
 */
export async function joinQueue(
  eventId: string,
  signals: Record<string, unknown>,
  opts: { jitter?: boolean; signal?: AbortSignal } = {},
): Promise<JoinResponse> {
  if (opts.jitter !== false) {
    await sleep(jitterDelay());
  }

  // Di qua apiFetch: Bearer tu phien dang nhap (AC-3, AC-9); khong cookie.
  const res = await apiFetch(`/v1/events/${encodeURIComponent(eventId)}/queue/join`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ signals }),
    signal: opts.signal,
  });

  if (!res.ok) {
    throw await queueErrorFrom(res);
  }
  return res.json();
}

/** Ma loi cua QueueError. 403 otp-required / device-required duoc tach rieng de UI huong dan nguoi dung (AC-9). */
export type QueueErrorCode = "OTP_REQUIRED" | "DEVICE_REQUIRED" | "FORBIDDEN" | "UNAUTHORIZED" | string;

const PROBLEM_OTP_REQUIRED = "/problems/otp-required";
const PROBLEM_DEVICE_REQUIRED = "/problems/device-required";

/** Doi Response loi thanh QueueError (status + detail + code theo `type` cua RFC 7807). */
export async function queueErrorFrom(res: Response): Promise<QueueError> {
  const p = await readProblem(res);
  const detail = p.detail ?? p.title ?? res.statusText;
  let code: QueueErrorCode = p.type ?? `HTTP_${res.status}`;
  if (res.status === 403) {
    if (p.type === PROBLEM_OTP_REQUIRED) code = "OTP_REQUIRED";
    else if (p.type === PROBLEM_DEVICE_REQUIRED) code = "DEVICE_REQUIRED";
    else code = "FORBIDDEN";
  } else if (res.status === 401 && !p.type) {
    code = "UNAUTHORIZED";
  }
  return new QueueError(res.status, detail, code);
}

export class QueueError extends Error {
  constructor(
    public readonly status: number,
    message: string,
    public readonly code: QueueErrorCode = `HTTP_${status}`,
  ) {
    super(message);
    this.name = "QueueError";
  }
}
