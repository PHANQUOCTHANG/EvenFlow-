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

export type QueueState = "LOBBY" | "QUEUED" | "ADMITTED" | "EXPIRED" | "UNKNOWN";

export interface JoinResponse {
  state: QueueState;
  queue_token: string;
  rank: number;
  is_new: boolean;
  poll_after_ms: number;
}

const API_BASE = process.env.NEXT_PUBLIC_API_BASE ?? "";
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

  const res = await fetch(`${API_BASE}/v1/events/${eventId}/queue/join`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ signals }),
    credentials: "include",
    signal: opts.signal,
  });

  if (!res.ok) {
    throw new QueueError(res.status, await safeDetail(res));
  }
  return res.json();
}

export class QueueError extends Error {
  constructor(
    public readonly status: number,
    message: string,
  ) {
    super(message);
    this.name = "QueueError";
  }
}

async function safeDetail(res: Response): Promise<string> {
  try {
    const body = await res.json();
    return body.detail ?? body.title ?? res.statusText;
  } catch {
    return res.statusText;
  }
}
