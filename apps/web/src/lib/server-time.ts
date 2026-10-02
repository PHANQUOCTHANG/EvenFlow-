/** Lech giua dong ho client va dong ho server.
 *
 * Dong ho may khach co the lech hang phut (hoac bi dat sai co y). Moi moc thoi han trong
 * he thong la cua SERVER, nen truoc khi so sanh phai quy ve gio server.
 *
 * Module nay KHONG tu goi API lay gio server — offset do caller cung cap, vi nguon offset
 * (header `Date`, field trong response status, time API) tuy thuoc endpoint ma task nay
 * chua co contract. */

/** serverNowMs - clientNowMs. Dau vao khong hop le -> 0 (tin dong ho client). */
export function serverOffsetMs(serverNowMs: number, clientNowMs: number): number {
  if (!Number.isFinite(serverNowMs) || !Number.isFinite(clientNowMs)) return 0;
  return serverNowMs - clientNowMs;
}

/** "Bay gio" theo gio server. */
export function serverNow(offsetMs: number = 0): number {
  return Date.now() + (Number.isFinite(offsetMs) ? offsetMs : 0);
}

/** Chuan hoa moc thoi han: nhan epoch ms hoac chuoi ISO.
 *  Tra null khi khong co moc hoac moc khong doc duoc — "chua biet thoi han" KHAC "da het han". */
export function toEpochMs(value: number | string | null | undefined): number | null {
  if (value === null || value === undefined) return null;
  const ms = typeof value === "number" ? value : Date.parse(value);
  return Number.isFinite(ms) ? ms : null;
}
