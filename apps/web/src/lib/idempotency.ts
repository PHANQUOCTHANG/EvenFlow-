/** Tien ich Idempotency-Key cho POST ghi (BR-O5): khoa on dinh trong sessionStorage + cho Retry-After khi 409 dang xu ly. */

export const IDEMPOTENCY_IN_PROGRESS = "urn:eventflow:problem:idempotency-in-progress";
/** So lan gui toi da (1 lan dau + 2 lan thu lai) khi server tra 409 idempotency-in-progress. */
export const IN_PROGRESS_MAX_ATTEMPTS = 3;
const DEFAULT_RETRY_AFTER_MS = 1000;
const MAX_RETRY_AFTER_MS = 5000;

export function newIdempotencyKey(): string {
  if (typeof crypto !== "undefined" && typeof crypto.randomUUID === "function") return crypto.randomUUID();
  return "xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx".replace(/[xy]/g, (c) => {
    const r = (Math.random() * 16) | 0;
    return (c === "x" ? r : (r & 0x3) | 0x8).toString(16);
  });
}

/** Lay (hoac tao va luu) khoa theo `storageKey`; sessionStorage khong dung duoc -> khoa moi moi lan. */
export function stableKey(storageKey: string): string {
  try {
    const existing = sessionStorage.getItem(storageKey);
    if (existing) return existing;
    const fresh = newIdempotencyKey();
    sessionStorage.setItem(storageKey, fresh);
    return fresh;
  } catch {
    return newIdempotencyKey();
  }
}

export function forgetKey(storageKey: string): void {
  try {
    sessionStorage.removeItem(storageKey);
  } catch {
    /* bo qua */
  }
}

/** Retry-After (giay) -> ms; thieu/khong hop le -> 1 s; toi da 5 s. */
export function retryAfterMs(res: Response): number {
  const raw = res.headers.get("Retry-After");
  const secs = raw === null ? NaN : Number(raw);
  if (!Number.isFinite(secs) || secs < 0) return DEFAULT_RETRY_AFTER_MS;
  return Math.min(Math.round(secs * 1000), MAX_RETRY_AFTER_MS);
}

export function sleep(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

/** Loi la dinh doat (4xx tru 408/429): khoa cu khong con y nghia, lan sau dung khoa moi. */
export function isDefinitiveStatus(status: number): boolean {
  return status >= 400 && status < 500 && status !== 408 && status !== 429;
}
