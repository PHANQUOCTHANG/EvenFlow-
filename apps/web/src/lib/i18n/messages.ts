/* Tu dien i18n vi/en (EV-188). `vi` la nguon, `en` phai du khoa (TypeScript chan thieu/thua).
 *
 * Pham vi dich gon: khung chung (skip link, header, footer, theme/language toggle) va
 * EventActionPanel. Chuoi con lai giu tieng Viet (no, xem docs/workflow/EV-188-a11y-i18n). */

export type Locale = "vi" | "en";
export const LOCALES: readonly Locale[] = ["vi", "en"];
export const DEFAULT_LOCALE: Locale = "vi";
export const LOCALE_STORAGE_KEY = "eventflow-locale";

/** Gia tri vi TRUNG NGUYEN VAN chuoi dang dung trong component (test hien co khong doi). */
export const vi = {
  "skipLink.label": "Bỏ qua điều hướng, tới nội dung chính",

  "header.menuToggle": "Mở menu điều hướng",
  "header.navLabel": "Điều hướng chính",

  "footer.tagline": "EventFlow — nền tảng bán vé sự kiện chịu tải đột biến.",
  "footer.disclaimer": "Dự án học tập. Mọi số liệu trong giao diện là dữ liệu mẫu.",

  "nav.home": "Trang chủ",
  "nav.events": "Sự kiện",
  "nav.comingSoon": "Sắp có",

  "theme.toggle": "Chuyển giao diện sáng/tối",
  "theme.light": "Sáng",
  "theme.dark": "Tối",

  "language.group": "Ngôn ngữ",

  "eventAction.soldOut": "Đã hết vé",
  "eventAction.cancelled": "Sự kiện đã huỷ",
  "eventAction.closed": "Đã đóng bán",
  "eventAction.dispatching": "Đang điều phối lượt vào phòng chờ...",
  "eventAction.keepScreen": "Vui lòng giữ nguyên màn hình...",
  "eventAction.jitterRemaining": "Còn {seconds}s (giảm tải đột biến)",
  "eventAction.saleStartsAt": "Thời gian mở bán",
  "eventAction.notYetOpen": "Chưa tới giờ mở bán",
  "eventAction.joinQueue": "Vào phòng chờ mua vé",
} as const;

export type MessageKey = keyof typeof vi;

export const en: Record<MessageKey, string> = {
  "skipLink.label": "Skip to main content",

  "header.menuToggle": "Open navigation menu",
  "header.navLabel": "Main navigation",

  "footer.tagline": "EventFlow — a ticketing platform built for sudden traffic surges.",
  "footer.disclaimer": "A student project. All figures in the interface are sample data.",

  "nav.home": "Home",
  "nav.events": "Events",
  "nav.comingSoon": "Coming soon",

  "theme.toggle": "Toggle light/dark theme",
  "theme.light": "Light",
  "theme.dark": "Dark",

  "language.group": "Language",

  "eventAction.soldOut": "Sold out",
  "eventAction.cancelled": "Event cancelled",
  "eventAction.closed": "Sale closed",
  "eventAction.dispatching": "Arranging your place in the waiting room...",
  "eventAction.keepScreen": "Please keep this screen open...",
  "eventAction.jitterRemaining": "{seconds}s left (smoothing traffic spikes)",
  "eventAction.saleStartsAt": "Sale starts",
  "eventAction.notYetOpen": "Sale not open yet",
  "eventAction.joinQueue": "Join the waiting room",
};

export const MESSAGES: Record<Locale, Record<MessageKey, string>> = { vi, en };

export function isLocale(value: unknown): value is Locale {
  return value === "vi" || value === "en";
}

/** Thay `{ten}` bang params.ten. Thieu khoa o en -> roi ve vi; locale la -> vi. */
export function translate(
  locale: Locale,
  key: MessageKey,
  params?: Record<string, string | number>,
): string {
  const dict = MESSAGES[isLocale(locale) ? locale : DEFAULT_LOCALE];
  const template = dict[key] ?? vi[key];
  if (!params) return template;
  return template.replace(/\{(\w+)\}/g, (whole, name: string) =>
    Object.hasOwn(params, name) ? String(params[name]) : whole,
  );
}
