/** Chinh sach lui khi loi mang / 5xx (EV-182 AC-2).
 *
 * Dung chung cho polling, SSE va join de ca phong cho chi co MOT nhip lui. Neu moi noi tu
 * chon nhip rieng thi khi mang chap chon, 300.000 tab se dong loat retry theo nhieu nhip
 * khac nhau va tao thanh nhung con song request chong len nhau.
 *
 * Gia tri mac dinh khop voi `watchQueue` trong queue-client.ts: 1s, 2s, 4s ... tran 30s,
 * cong jitter 0..500ms de cac client khong retry cung mot mili giay sau khi mang co lai. */

export interface BackoffOptions {
  /** Do tre cua lan thu dau tien. */
  baseMs?: number;
  /** Tran cua phan luy thua (chua tinh jitter). */
  maxMs?: number;
  /** Bien do jitter cong them, phan bo deu trong [0, jitterMs). */
  jitterMs?: number;
  /** Inject de test xac dinh. */
  random?: () => number;
}

export const DEFAULT_BACKOFF_BASE_MS = 1_000;
export const DEFAULT_BACKOFF_MAX_MS = 30_000;
export const DEFAULT_BACKOFF_JITTER_MS = 500;

/** Do tre truoc lan thu thu `attempt` (bat dau tu 1).
 *
 * `attempt` khong hop le (<= 0, NaN) duoc coi nhu lan 1: lui it nhat mot nhip co ban, khong
 * bao gio tra 0 — tra 0 nghia la retry ngay, chinh la cai bao request ma ham nay sinh ra de chan. */
export function backoffDelay(attempt: number, options: BackoffOptions = {}): number {
  const baseMs = options.baseMs ?? DEFAULT_BACKOFF_BASE_MS;
  const maxMs = options.maxMs ?? DEFAULT_BACKOFF_MAX_MS;
  const jitterMs = options.jitterMs ?? DEFAULT_BACKOFF_JITTER_MS;
  const random = options.random ?? Math.random;

  const n = Number.isFinite(attempt) && attempt >= 1 ? Math.floor(attempt) : 1;
  // Gioi han so mu de 2 ** n khong tran thanh Infinity voi attempt rat lon.
  const exp = Math.min(n - 1, 30);
  const capped = Math.min(baseMs * 2 ** exp, maxMs);

  return Math.floor(capped + random() * jitterMs);
}
