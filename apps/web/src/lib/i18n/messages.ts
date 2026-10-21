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

  "eventLoader.loading": "Đang tải thông tin sự kiện...",
  "eventLoader.errorTitle": "Không tải được sự kiện",
  "eventLoader.errorBody": "Không thể kết nối tới máy chủ sự kiện. Vui lòng thử lại.",
  "eventLoader.retry": "Thử lại",

  "waiting.otpRequiredTitle": "Cần xác thực OTP",
  "waiting.otpRequiredBody": "Tài khoản của bạn chưa xác thực OTP nên chưa thể vào phòng chờ. Hãy hoàn tất xác thực OTP rồi đăng nhập lại.",
  "waiting.otpRequiredAction": "Xác thực OTP và đăng nhập lại",
  "waiting.deviceRequiredTitle": "Cần đăng nhập lại trên thiết bị này",
  "waiting.deviceRequiredBody": "Phiên đăng nhập hiện tại chưa gắn với thiết bị. Vui lòng đăng nhập lại để tiếp tục vào phòng chờ.",
  "waiting.deviceRequiredAction": "Đăng nhập lại",
  "waiting.signInRequired": "Bạn cần đăng nhập trước khi vào phòng chờ.",
  "waiting.signInAction": "Đăng nhập",

  "checkout.tokenInvalidTitle": "Lượt vào mua vé không còn hiệu lực",
  "checkout.tokenInvalidBody": "Mã xếp hàng của bạn đã hết hạn hoặc không hợp lệ. Vui lòng quay lại phòng chờ để lấy lượt mới.",
  "checkout.tokenInvalidAction": "Quay lại phòng chờ",
  "checkout.perIdentityLimit": "Bạn đã đạt giới hạn số vé cho sự kiện này.",
  "checkout.quantityNotAllowed": "Số lượng vé bạn chọn vượt quá giới hạn cho một đơn.",
  "checkout.forbidden": "Tài khoản của bạn không có quyền mua vé cho sự kiện này.",
  "checkout.authRequired": "Phiên đăng nhập đã hết hạn. Vui lòng đăng nhập lại.",
  "checkout.signInAction": "Đăng nhập",
  "checkout.availabilityIndicative": "Tình trạng vé chỉ mang tính tham khảo; kết quả giữ vé mới là chính xác.",

  "payment.methodsLoading": "Đang tải các cổng thanh toán...",
  "payment.methodsErrorTitle": "Không tải được cổng thanh toán",
  "payment.methodsErrorBody": "Chưa lấy được danh sách cổng thanh toán từ máy chủ.",
  "payment.methodsRetry": "Tải lại",
  "payment.methodsEmpty": "Hiện chưa có cổng thanh toán nào đang mở.",
  "payment.missingOrderTitle": "Thiếu thông tin đơn hàng",
  "payment.missingOrderBody": "Không tìm thấy mã đơn hoặc mã giữ vé. Vui lòng chọn vé lại.",
  "payment.backToCheckout": "Quay lại chọn vé",

  "result.missingOrderTitle": "Thiếu mã đơn hàng",
  "result.missingOrderBody": "Đường dẫn không có mã đơn hàng. Vui lòng kiểm tra lại email hoặc chọn vé lại.",
  "result.loadErrorTitle": "Không tải được đơn hàng",
  "result.loadErrorBody": "Chưa lấy được trạng thái đơn hàng. Đơn hàng của bạn không bị ảnh hưởng, vui lòng thử lại.",
  "result.notFoundTitle": "Không tìm thấy đơn hàng",
  "result.notFoundBody": "Đơn hàng không tồn tại hoặc không thuộc tài khoản đang đăng nhập.",
  "result.retry": "Thử lại",
  "result.ticketsPendingTitle": "Đang phát hành vé",
  "result.paidIssuingTitle": "Thanh toán thành công — đang phát hành vé",
  "result.paidIssuingBody": "Thanh toán đã được ghi nhận. Vé điện tử sẽ hiển thị tại đây ngay khi được phát hành.",
  "result.ticketsPendingBody": "Thanh toán đã được ghi nhận. Vé điện tử đang được phát hành, trang sẽ tự cập nhật.",
  "result.ticketsFailedTitle": "Chưa lấy được vé điện tử",
  "result.ticketsFailedBody": "Vé của bạn chưa sẵn sàng. Bạn không bị trừ tiền thêm, vui lòng thử lại sau ít phút.",
  "result.ticketsRetry": "Lấy vé lại",
  "result.qrLabel": "Mã QR vé {code}",
  "result.qrHint": "Xuất trình mã này tại cổng sự kiện để check-in",
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

  "eventLoader.loading": "Loading event...",
  "eventLoader.errorTitle": "Could not load the event",
  "eventLoader.errorBody": "Could not reach the event server. Please try again.",
  "eventLoader.retry": "Retry",

  "waiting.otpRequiredTitle": "OTP verification required",
  "waiting.otpRequiredBody": "Your account has not verified its OTP yet, so you cannot enter the waiting room. Finish OTP verification, then sign in again.",
  "waiting.otpRequiredAction": "Verify OTP and sign in again",
  "waiting.deviceRequiredTitle": "Sign in again on this device",
  "waiting.deviceRequiredBody": "Your current session is not bound to a device. Please sign in again to continue to the waiting room.",
  "waiting.deviceRequiredAction": "Sign in again",
  "waiting.signInRequired": "You need to sign in before joining the waiting room.",
  "waiting.signInAction": "Sign in",

  "checkout.tokenInvalidTitle": "Your turn to buy is no longer valid",
  "checkout.tokenInvalidBody": "Your queue token has expired or is invalid. Please go back to the waiting room to get a new turn.",
  "checkout.tokenInvalidAction": "Back to the waiting room",
  "checkout.perIdentityLimit": "You have reached the ticket limit for this event.",
  "checkout.quantityNotAllowed": "The quantity you chose exceeds the limit for one order.",
  "checkout.forbidden": "Your account is not allowed to buy tickets for this event.",
  "checkout.authRequired": "Your session has expired. Please sign in again.",
  "checkout.signInAction": "Sign in",
  "checkout.availabilityIndicative": "Ticket availability is indicative; the hold result is authoritative.",

  "payment.methodsLoading": "Loading payment methods...",
  "payment.methodsErrorTitle": "Could not load payment methods",
  "payment.methodsErrorBody": "The list of payment methods could not be fetched from the server.",
  "payment.methodsRetry": "Reload",
  "payment.methodsEmpty": "No payment method is available right now.",
  "payment.missingOrderTitle": "Order details missing",
  "payment.missingOrderBody": "The order or hold id is missing. Please choose your tickets again.",
  "payment.backToCheckout": "Back to ticket selection",

  "result.missingOrderTitle": "Order id missing",
  "result.missingOrderBody": "The link has no order id. Please check your email or choose your tickets again.",
  "result.loadErrorTitle": "Could not load the order",
  "result.loadErrorBody": "The order status could not be fetched. Your order is not affected, please try again.",
  "result.notFoundTitle": "Order not found",
  "result.notFoundBody": "The order does not exist or does not belong to the signed-in account.",
  "result.retry": "Retry",
  "result.ticketsPendingTitle": "Issuing your tickets",
  "result.paidIssuingTitle": "Payment successful — issuing your tickets",
  "result.paidIssuingBody": "Your payment is recorded. Your e-tickets will appear here as soon as they are issued.",
  "result.ticketsPendingBody": "Payment is recorded. Your e-tickets are being issued, this page will update by itself.",
  "result.ticketsFailedTitle": "E-tickets not available yet",
  "result.ticketsFailedBody": "Your tickets are not ready. You will not be charged again, please try again in a few minutes.",
  "result.ticketsRetry": "Fetch tickets again",
  "result.qrLabel": "Ticket QR code {code}",
  "result.qrHint": "Show this code at the event gate to check in",
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
