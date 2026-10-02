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
