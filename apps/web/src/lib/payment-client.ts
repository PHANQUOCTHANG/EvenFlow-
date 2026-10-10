/** Client API khoi tao thanh toan va tra cuu trang thai don hang (EVF-114, EV-184, BR-O2, BR-O5, BR-O6, BR-O8).
 *
 * Tuan thu cac nguyen tac bat bien:
 * 1. BR-O5: Bat buoc header `Idempotency-Key` (UUID v4) cho moi thao tac POST thanh toan.
 * 2. BR-O6: Thanh toan va webhook bat dong bo; redirect tu cong khong phai bang chung thanh toan thanh cong.
 * 3. BR-O2: Khi hold het han giua chung -> server danh dau EXPIRED va tra ve kho.
 * 4. BR-O8: Don PAID tra ve danh sach ve phat hanh (IssuedTicket) co ma QR demo an toan.
 */

export type PaymentMethod = "vnpay" | "momo" | "sandbox";

export type OrderStatus = "PENDING" | "PAID" | "FAILED" | "EXPIRED" | "CANCELLED";

export interface InitiatePaymentParams {
  eventId: string;
  orderId: string;
  holdId: string;
  amount: number;
  method: PaymentMethod;
  buyerName: string;
  buyerEmail: string;
  buyerPhone: string;
  idempotencyKey?: string;
  apiBaseUrl?: string;
  returnUrl?: string;
}

export interface PaymentInitiationResult {
  paymentId: string;
  orderId: string;
  redirectUrl: string;
  status: "PENDING" | "PROCESSING";
  providerTxnId?: string;
}

export interface IssuedTicket {
  ticketId: string;
  ticketCode: string;
  tierName: string;
  attendeeName: string;
  qrPayload: string; // Payload visual demo an toan (BR-O8)
}

export interface OrderDetails {
  orderId: string;
  eventId: string;
  eventTitle: string;
  venue: string;
  tierName: string;
  quantity: number;
  totalAmount: number;
  status: OrderStatus;
  paidAt?: string;
  paymentMethod?: PaymentMethod;
  failureReason?: string;
  issuedTickets?: IssuedTicket[];
}

export type PaymentClientErrorType =
  | "MISSING_IDEMPOTENCY_KEY"
  | "HOLD_EXPIRED"
  | "INVALID_INPUT"
  | "AMBIGUOUS"
  | "NETWORK_ERROR"
  | "ORDER_NOT_FOUND"
  | "UNKNOWN";

export class PaymentClientError extends Error {
  readonly type: PaymentClientErrorType;
  readonly status?: number;

  constructor(message: string, type: PaymentClientErrorType, status?: number) {
    super(message);
    this.name = "PaymentClientError";
    this.type = type;
    this.status = status;
  }
}

/** Sinh Idempotency-Key UUID v4 an toan (BR-O5) */
export function generateIdempotencyKey(): string {
  if (typeof crypto !== "undefined" && typeof crypto.randomUUID === "function") {
    return crypto.randomUUID();
  }
  return "xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx".replace(/[xy]/g, (c) => {
    const r = (Math.random() * 16) | 0;
    const v = c === "x" ? r : (r & 0x3) | 0x8;
    return v.toString(16);
  });
}

const ORDER_STORAGE_PREFIX = "evenflow_order_";

/** Luu thong tin don hang cuc bo de ho tro phuc hoi tren trang ket qua */
export function saveRecentOrder(order: OrderDetails): void {
  if (typeof window === "undefined" || !window.sessionStorage) return;
  try {
    sessionStorage.setItem(`${ORDER_STORAGE_PREFIX}${order.orderId}`, JSON.stringify(order));
  } catch {
    // bo qua loi storage
  }
}

/** Lay thong tin don hang da luu tu sessionStorage */
export function getRecentOrder(orderId: string): OrderDetails | null {
  if (typeof window === "undefined" || !window.sessionStorage) return null;
  try {
    const raw = sessionStorage.getItem(`${ORDER_STORAGE_PREFIX}${orderId}`);
    if (!raw) return null;
    return JSON.parse(raw) as OrderDetails;
  } catch {
    return null;
  }
}

/** Xoa thong tin don hang khoi sessionStorage */
export function clearRecentOrder(orderId: string): void {
  if (typeof window === "undefined" || !window.sessionStorage) return;
  try {
    sessionStorage.removeItem(`${ORDER_STORAGE_PREFIX}${orderId}`);
  } catch {
    // bo qua
  }
}

/** Khoi tao phien thanh toan (POST /v1/events/{id}/payments) kem header Idempotency-Key (BR-O5) */
export async function initiatePayment(
  params: InitiatePaymentParams,
): Promise<PaymentInitiationResult> {
  const {
    eventId,
    orderId,
    holdId,
    amount,
    method,
    buyerName,
    buyerEmail,
    buyerPhone,
    idempotencyKey = generateIdempotencyKey(),
    apiBaseUrl = "",
    returnUrl,
  } = params;

  if (!idempotencyKey) {
    throw new PaymentClientError(
      "Bắt buộc có Idempotency-Key khi thanh toán (BR-O5)",
      "MISSING_IDEMPOTENCY_KEY",
      400,
    );
  }

  const effectiveReturnUrl =
    returnUrl || `/checkout/${encodeURIComponent(eventId)}/result?orderId=${encodeURIComponent(orderId)}`;

  const url = `${apiBaseUrl}/v1/events/${encodeURIComponent(eventId)}/payments`;

  try {
    const response = await fetch(url, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        "Idempotency-Key": idempotencyKey,
      },
      body: JSON.stringify({
        order_id: orderId,
        hold_id: holdId,
        amount,
        method,
        buyer_name: buyerName,
        buyer_email: buyerEmail,
        buyer_phone: buyerPhone,
        return_url: effectiveReturnUrl,
      }),
    });

    if (response.ok) {
      const data = await response.json();
      return {
        paymentId: data.payment_id || `pay_${generateIdempotencyKey().slice(0, 8)}`,
        orderId: data.order_id || orderId,
        redirectUrl: data.redirect_url || effectiveReturnUrl,
        status: data.status || "PENDING",
        providerTxnId: data.provider_txn_id,
      };
    }

    if (response.status === 409) {
      throw new PaymentClientError(
        "Phiên giữ vé đã hết hạn trước khi thanh toán (BR-O2)",
        "HOLD_EXPIRED",
        409,
      );
    }

    if (response.status === 422 || response.status === 400) {
      const errBody = await response.json().catch(() => null);
      throw new PaymentClientError(
        errBody?.detail || "Thông tin thanh toán không hợp lệ",
        "INVALID_INPUT",
        response.status,
      );
    }

    if (response.status >= 500) {
      throw new PaymentClientError(
        "Hệ thống thanh toán đang xử lý hoặc phản hồi chậm",
        "AMBIGUOUS",
        response.status,
      );
    }

    throw new PaymentClientError(
      `Lỗi thanh toán HTTP ${response.status}`,
      "UNKNOWN",
      response.status,
    );
  } catch (err) {
    if (err instanceof PaymentClientError) {
      throw err;
    }

    // Neu khong co backend hoac mock fallback
    if (method === "sandbox" || (!apiBaseUrl && process.env.NODE_ENV !== "production")) {
      const paymentId = `mock_pay_${Date.now()}`;
      return {
        paymentId,
        orderId,
        redirectUrl: effectiveReturnUrl,
        status: "PENDING",
        providerTxnId: `txn_${Date.now()}`,
      };
    }

    throw new PaymentClientError(
      "Không thể kết nối đến máy chủ thanh toán (BR-O5)",
      "NETWORK_ERROR",
    );
  }
}

/** Tra cuu trang thai don hang tu backend (GET /v1/events/{id}/orders/{orderId}) */
export async function getOrderDetails(
  orderId: string,
  eventId: string,
  apiBaseUrl = "",
): Promise<OrderDetails> {
  const url = `${apiBaseUrl}/v1/events/${encodeURIComponent(eventId)}/orders/${encodeURIComponent(orderId)}`;

  try {
    const res = await fetch(url, {
      method: "GET",
      headers: {
        Accept: "application/json",
      },
    });

    if (res.ok) {
      const data = await res.json();
      const details: OrderDetails = {
        orderId: data.order_id || orderId,
        eventId: data.event_id || eventId,
        eventTitle: data.event_title || "Sự kiện EvenFlow",
        venue: data.venue || "Địa điểm tổ chức",
        tierName: data.tier_name || "Vé Tiêu Chuẩn",
        quantity: data.quantity || 1,
        totalAmount: data.total_amount || 0,
        status: (data.status as OrderStatus) || "PENDING",
        paidAt: data.paid_at,
        paymentMethod: data.payment_method,
        failureReason: data.failure_reason,
        issuedTickets: data.issued_tickets || [],
      };
      saveRecentOrder(details);
      return details;
    }

    if (res.status === 404) {
      const cached = getRecentOrder(orderId);
      if (cached) return cached;
      throw new PaymentClientError("Không tìm thấy đơn hàng", "ORDER_NOT_FOUND", 404);
    }

    throw new PaymentClientError(`Lỗi máy chủ HTTP ${res.status}`, "UNKNOWN", res.status);
  } catch (err) {
    if (err instanceof PaymentClientError) {
      throw err;
    }

    // Fallback lay tu storage neu mat mang hoac local dev
    const cached = getRecentOrder(orderId);
    if (cached) return cached;

    throw new PaymentClientError("Không thể kết nối máy chủ để kiểm tra đơn hàng", "NETWORK_ERROR");
  }
}
