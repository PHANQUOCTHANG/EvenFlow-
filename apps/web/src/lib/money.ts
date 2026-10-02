import { formatVnd } from "./format";

/** Nhom chu so khong phu thuoc ICU, giong formatVnd. */
function group(amount: number): string {
  const sign = amount < 0 ? "-" : "";
  return `${sign}${String(Math.abs(Math.trunc(amount))).replace(/\B(?=(\d{3})+(?!\d))/g, ".")}`;
}

/** Dinh dang so tien THEO DUNG tien te duoc truyen.
 *
 * `formatVnd` hard-code hau to `đ`. Neu server tra `currency: "USD"` ma van in `đ` thi do la
 * SAI SO TIEN, khong phai sai dinh dang. Nen:
 *   - "VND" -> dung formatVnd (dinh dang DESIGN.md: 1.250.000 đ)
 *   - tien te khac -> so da nhom + MA tien te; khong bia ky hieu cho mot don vi minh khong biet
 *   - thieu currency -> "—". KHONG mac dinh VND: doan tien te cua mot so tien la dieu te nhat
 *     co the lam o day. */
export function formatMoney(amount: number, currency: string | undefined | null): string {
  if (!Number.isFinite(amount)) return "—";

  const code = typeof currency === "string" ? currency.trim().toUpperCase() : "";
  if (code === "") return "—";
  if (code === "VND") return formatVnd(amount);

  return `${group(amount)} ${code}`;
}
