/** Dinh dang hien thi. Khong phu thuoc ICU/locale data de ket qua xac dinh tren moi may. */

/** ms -> "mm:ss", hoac "h:mm:ss" khi tu 1 gio tro len.
 *
 * Lam tron XUONG (floor) co y: con 1500ms phai hien "00:01", khong phai "00:02".
 * Khong bao gio duoc hien nhieu thoi gian hon so thuc con lai — day la dong ho giu ve,
 * hien thua mot giay la hua voi khach mot thu ho khong co.
 *
 * Gia tri am hoac khong hop le -> "00:00". Khong dem len, khong hien dau tru. */
export function formatClock(ms: number): string {
  if (!Number.isFinite(ms) || ms <= 0) return "00:00";

  const totalSeconds = Math.floor(ms / 1000);
  const hours = Math.floor(totalSeconds / 3600);
  const minutes = Math.floor((totalSeconds % 3600) / 60);
  const seconds = totalSeconds % 60;
  const pad = (value: number) => String(value).padStart(2, "0");

  return hours > 0 ? `${hours}:${pad(minutes)}:${pad(seconds)}` : `${pad(minutes)}:${pad(seconds)}`;
}

/** 1250000 -> "1.250.000 đ" (DESIGN.md muc Typography: dau cham phan cach nghin, hau to đ).
 *
 * Nhom chu so bang regex thay vi toLocaleString: khong phu thuoc ICU, nen output giong nhau
 * o Node, o browser va trong CI. Gia tri khong hop le tra "—" chu khong bao gio "NaN đ". */
export function formatVnd(amount: number): string {
  if (!Number.isFinite(amount)) return "—";

  const sign = amount < 0 ? "-" : "";
  const grouped = String(Math.abs(Math.trunc(amount))).replace(/\B(?=(\d{3})+(?!\d))/g, ".");

  return `${sign}${grouped} đ`;
}

/** ms -> "9 phút 30 giây", danh cho screen reader.
 *
 * Doc "09:30" theo tung ky tu la vo nghia voi screen reader, nen vung so duoc aria-hidden
 * va thay bang chuoi nay. Bo don vi bang 0 de khong doc "0 gio 9 phut 30 giay". */
export function spellDuration(ms: number): string {
  if (!Number.isFinite(ms) || ms <= 0) return "0 giây";

  const totalSeconds = Math.floor(ms / 1000);
  const hours = Math.floor(totalSeconds / 3600);
  const minutes = Math.floor((totalSeconds % 3600) / 60);
  const seconds = totalSeconds % 60;

  const parts: string[] = [];
  if (hours > 0) parts.push(`${hours} giờ`);
  if (minutes > 0) parts.push(`${minutes} phút`);
  if (seconds > 0 || parts.length === 0) parts.push(`${seconds} giây`);

  return parts.join(" ");
}

/** 4210 -> "4.210" (dau cham ngan nghin). Tinh tay bang regex, khong toLocaleString/ICU nen
 *  server va client render giong nhau (tranh loi hydration). Khong huu han -> "—". */
export function formatCount(n: number): string {
  if (!Number.isFinite(n) || Math.abs(n) > Number.MAX_SAFE_INTEGER) return "—";

  // Lam tron ve 0 (ke ca -0.5, -0) thi khong hien dau tru.
  const abs = Math.abs(Math.trunc(n));
  const sign = n < 0 && abs !== 0 ? "-" : "";
  return `${sign}${String(abs).replace(/\B(?=(\d{3})+(?!\d))/g, ".")}`;
}

/** 12.7 -> "12,7"; 1234.56 -> "1.234,6" (dau cham ngan nghin, dau phay thap phan).
 *  Lam tron `digits` chu so; -0 hoac ket qua lam tron ve 0 -> "0,0"; khong huu han -> "—". */
export function formatDecimal(n: number, digits = 1): string {
  if (!Number.isFinite(n) || Math.abs(n) > Number.MAX_SAFE_INTEGER) return "—";

  const fixed = Math.abs(n).toFixed(digits);
  const [intPart, fracPart] = fixed.split(".");
  const isZero = Number(fixed) === 0;
  const sign = n < 0 && !isZero ? "-" : "";
  const grouped = intPart.replace(/\B(?=(\d{3})+(?!\d))/g, ".");

  return fracPart ? `${sign}${grouped},${fracPart}` : `${sign}${grouped}`;
}

/** ISO -> "HH:MM:SS" theo gio Viet Nam (UTC+7) tinh tay, khong toLocale*, khong phu thuoc
 *  mui gio cua may. ISO hong hoac rong -> "--:--:--". */
export function formatTimeVn(iso: string): string {
  // Chi nhan ISO co Z hoac offset ±HH:MM; thieu thi Date.parse hieu theo mui gio may -> sai.
  if (!/(Z|[+-]\d{2}:\d{2})$/i.test(iso)) return "--:--:--";
  const ms = Date.parse(iso);
  if (Number.isNaN(ms)) return "--:--:--";

  const VN_OFFSET_MS = 7 * 60 * 60 * 1000;
  const DAY_MS = 24 * 60 * 60 * 1000;
  const msOfDay = (((ms + VN_OFFSET_MS) % DAY_MS) + DAY_MS) % DAY_MS;
  const totalSeconds = Math.floor(msOfDay / 1000);
  const pad = (value: number) => String(value).padStart(2, "0");

  return `${pad(Math.floor(totalSeconds / 3600))}:${pad(Math.floor((totalSeconds % 3600) / 60))}:${pad(totalSeconds % 60)}`;
}
