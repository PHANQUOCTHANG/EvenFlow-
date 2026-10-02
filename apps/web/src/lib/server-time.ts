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

/** Chuoi phai mang mui gio tuong minh: ket thuc bang `Z` hoac `+07:00` / `-0500`. */
const HAS_EXPLICIT_OFFSET = /(?:Z|[+-]\d{2}:?\d{2})$/;

/** Chuan hoa moc thoi han: nhan epoch ms hoac chuoi ISO-8601 CO mui gio.
 *
 * Tra null khi khong co moc hoac moc khong doc duoc — "chua biet thoi han" KHAC "da het han".
 *
 * Chuoi KHONG mang mui gio bi tu choi, khong phai de kho tinh: ECMA quy dinh dang chi co ngay
 * ("2026-10-03") duoc hieu la UTC, con dang co gio nhung khong co offset
 * ("2026-10-03T09:00:00") duoc hieu la gio DIA PHUONG. Hai quy tac trai nguoc nhau trong cung
 * mot ham `Date.parse`. Hau qua cu the: `expiresAt="2026-10-03"` se dem nguoc toi 07:00 gio
 * Viet Nam thay vi 00:00 — lech 7 tieng dung vao moc mo ban. Tu choi tai day an toan hon la
 * doan nham mui gio cua mot moc lien quan den tien. */
export function toEpochMs(value: number | string | null | undefined): number | null {
  if (value === null || value === undefined) return null;

  if (typeof value === "number") {
    return Number.isFinite(value) ? value : null;
  }

  const trimmed = value.trim();
  if (!HAS_EXPLICIT_OFFSET.test(trimmed)) {
    // Tra null la an toan, nhung im lang thi mot loi backend bien thanh UI dung im: khach
    // thay "--:--" vo thoi han va khong ai biet tai sao. Canh bao o dev/CI de bat som.
    if (process.env.NODE_ENV !== "production") {
      console.warn(
        `[server-time] moc thoi han "${trimmed}" khong co mui gio nen bi bo qua. ` +
          `Can ISO-8601 co offset, vi du "2026-10-03T09:00:00+07:00" hoac "...Z".`,
      );
    }
    return null;
  }

  const ms = Date.parse(trimmed);
  return Number.isFinite(ms) ? ms : null;
}
