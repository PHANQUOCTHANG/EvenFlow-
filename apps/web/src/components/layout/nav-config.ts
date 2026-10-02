/** Danh sach nav theo vai tro.
 *
 * DAY LA CAU HINH HIEN THI, KHONG PHAI MA TRAN QUYEN.
 * Phan quyen that thuoc identity service. Dung dung file nay de quyet dinh ai duoc lam gi —
 * an mot muc nav khong he ngan duoc request toi route do.
 * (handoff muc 11.1: "Do not invent permission links; annotate role-specific nav as configurable")
 *
 * `ready` phan anh hien trang THAT cua repo: ngoai trang chu, chua route nao ton tai.
 * Item `ready: false` khong duoc render thanh <a> — bam vao se ra 404, te hon la noi thang
 * "Sap co" (handoff muc 5 nguyen tac 3: No fake certainty). */

export interface NavItem {
  href: string;
  label: string;
  /** false = route chua ton tai trong repo -> KHONG render <a>. */
  ready: boolean;
}

/** Nguon: docs/08-stitch-ui-ux-handoff.md muc 4 (Information architecture). */
export const PUBLIC_NAV: NavItem[] = [
  { href: "/", label: "Trang chủ", ready: true },
  { href: "/events", label: "Sự kiện", ready: false },
];

export const ORGANIZER_NAV: NavItem[] = [
  { href: "/organizer/events", label: "Sự kiện của tôi", ready: false },
  { href: "/organizer/events/new", label: "Tạo sự kiện", ready: false },
];

/** `/ops/events` la route index DE XUAT, chua co trong handoff muc 4 — o do Ops chi co
 *  `/ops/events/[eventId]` (war room) va `/ops/ai`. Mot muc nav khong the tro vao route co
 *  tham so, nen can mot trang danh sach. Giu `ready: false` cho den khi Product chot. */
export const OPS_NAV: NavItem[] = [
  { href: "/ops/events", label: "War room", ready: false },
  { href: "/ops/ai", label: "Hạ tầng AI", ready: false },
];

/** Route goc khop CHINH XAC. Neu dung startsWith cho "/" thi moi pathname deu bat dau bang
 *  "/" nen muc Trang chu se LUON active — do la bug kinh dien cua nav. */
export function isActive(pathname: string, href: string): boolean {
  if (href === "/") {
    return pathname === "/";
  }
  return pathname === href || pathname.startsWith(`${href}/`);
}

/** Item nao thuc su la "trang hien tai" trong MOT danh sach nav.
 *
 * `isActive` tinh cho tung item roi la khong du khi danh sach co item long tien to nhau.
 * Vi du that: ORGANIZER_NAV co ca `/organizer/events` va `/organizer/events/new`. O pathname
 * `/organizer/events/new` thi `isActive` tra true cho CA HAI -> hai muc cung mang
 * aria-current="page", screen reader bao hai "trang hien tai" trong cung mot nav.
 *
 * Nen trang hien tai duoc chon theo khop DAI NHAT, va tinh o cap danh sach. */
export function activeHref(items: NavItem[], pathname: string): string | null {
  let best: string | null = null;
  for (const item of items) {
    if (!isActive(pathname, item.href)) continue;
    if (best === null || item.href.length > best.length) {
      best = item.href;
    }
  }
  return best;
}
