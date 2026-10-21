/** Client API khoi tao thanh toan va tra cuu trang thai don hang (EVF-114, EV-184, BR-O2, BR-O5, BR-O6, BR-O8).
 *
 * Tuan thu cac nguyen tac bat bien:
 * 1. BR-O5: Bat buoc header `Idempotency-Key` (UUID v4) cho moi thao tac POST thanh toan.
 * 2. BR-O6: Thanh toan va webhook bat dong bo; redirect tu cong khong phai bang chung thanh toan thanh cong.
 * 3. BR-O2: Khi hold het han giua chung -> server danh dau EXPIRED va tra ve kho.
 * 4. BR-O8: Ma QR chi den tu `qr_payload` that do `issueTickets` tra ve; khong bao gio co QR demo.
 * 5. AC-12/AC-14: khong bao gio gia thanh cong, khong bia gia tri mac dinh, khong phuc vu cache nhu du lieu song.
 */

import { ApiError, apiFetch, readProblem, type ApiFetchInit } from "@/lib/api-client";
import {
  forgetKey,
  IDEMPOTENCY_IN_PROGRESS,
  IN_PROGRESS_MAX_ATTEMPTS,
  isDefinitiveStatus,
  retryAfterMs,
  sleep,
  stableKey,
} from "@/lib/idempotency";

export type PaymentMethod = "vnpay" | "momo" | "sandbox" | "stripe";

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
  /** @deprecated Bo qua: goc API luon lay tu `apiBase()` qua apiFetch. Giu de khong vo caller cu. */
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

/** Ve ghi nhan tren don (GET /v1/orders/{id}); backend KHONG tra QR payload o day. */
export interface IssuedTicket {
  ticketId: string;
  ticketCode: string;
  tierName: string;
  attendeeName: string;
  status?: string;
}

/** Ve kem payload QR da ky, lay tu POST /v1/orders/{id}/issue-tickets (AC-15). */
export interface IssuedTicketQR {
  ticketId: string;
  ticketTypeId: string;
  status: string;
  qrPayload: string;
}

export interface OrderItem {
  ticketTypeId: string;
  tierName: string;
  quantity: number;
  unitCents: number;
}

export interface PaymentMethodInfo {
  code: PaymentMethod;
  name: string;
  description: string;
  enabled: boolean;
  icon: string;
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
  /** Han giu cho do server cap (RFC 3339). */
  expiresAt?: string;
  holdId?: string;
  items?: OrderItem[];
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
  | "NOT_FOUND"
  | "AUTH_REQUIRED"
  | "FORBIDDEN"
  | "UNKNOWN";

export class PaymentClientError extends Error {
  readonly type: PaymentClientErrorType;
  /** Cung gia tri voi `type`, de goi chung voi ApiError (`code`). */
  readonly code: PaymentClientErrorType;
  readonly status?: number;

  constructor(message: string, type: PaymentClientErrorType, status?: number) {
    super(message);
    this.name = "PaymentClientError";
    this.type = type;
    this.code = type;
    this.status = status;
  }
}

const FORBIDDEN_PROBLEM = "/problems/forbidden";

/** 403 `/problems/forbidden` (sai vai tro / khong du quyen): loi rieng, khong thu lai (AC-6). Cac 403 khac giu nguyen. */
async function forbiddenError(res: Response): Promise<PaymentClientError | null> {
  if (res.status !== 403) return null;
  if ((await readProblem(res)).type !== FORBIDDEN_PROBLEM) return null;
  return new PaymentClientError("Không đủ quyền thực hiện thao tác này", "FORBIDDEN", 403);
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

const PAY_KEY_PREFIX = "ef:pay:";

/** Bam FNV-1a 32 bit (chi de phan biet noi dung yeu cau trong khoa sessionStorage, khong phai bao mat). */
function bodyHash(parts: ReadonlyArray<string | number>): string {
  const text = JSON.stringify(parts);
  let h = 0x811c9dc5;
  for (let i = 0; i < text.length; i++) {
    h ^= text.charCodeAt(i);
    h = Math.imul(h, 0x01000193) >>> 0;
  }
  return h.toString(16);
}
const ORDER_STORAGE_PREFIX = "evenflow_order_";

/** Luu thong tin don hang cuc bo (chi de debug / tuong thich); KHONG bao gio duoc doc lai nhu du lieu song. */
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

/** Goi apiFetch va doi loi ha tang thanh PaymentClientError (khong bao gio nuot loi thanh thanh cong). */
async function paymentFetch(path: string, init: ApiFetchInit, networkMessage: string): Promise<Response> {
  try {
    const res = await apiFetch(path, init);
    // 401 con lai sau khi apiFetch da thu refresh (hoac khong co phien): can dang nhap lai (F-5).
    if (res.status === 401 && init.auth !== false) {
      throw new PaymentClientError("Phiên đăng nhập đã hết hạn", "AUTH_REQUIRED", 401);
    }
    return res;
  } catch (err) {
    if (err instanceof PaymentClientError) throw err;
    if (err instanceof ApiError && err.code === "AUTH_REQUIRED") {
      throw new PaymentClientError("Phiên đăng nhập đã hết hạn", "AUTH_REQUIRED", 401);
    }
    throw new PaymentClientError(networkMessage, "NETWORK_ERROR");
  }
}

/** Khoi tao phien thanh toan (POST /v1/events/{id}/payments) kem header Idempotency-Key (BR-O5).
 *  Moi loi (non-2xx, mang, phan hoi thieu payment_id) deu reject: khong bao gio gia thanh cong (AC-12). */
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
    returnUrl,
  } = params;
  // Khoa on dinh theo don (F-8): thu lai sau NETWORK_ERROR dung lai khoa cu; xoa khi thanh cong / loi dinh doat.
  // G-5: khoa gan voi noi dung yeu cau (phuong thuc, nguoi mua, so tien): doi noi dung -> khoa moi; thu lai y het -> khoa cu.
  const keyStorage = `${PAY_KEY_PREFIX}${orderId}:${bodyHash([method, buyerName, buyerEmail, buyerPhone, amount, holdId])}`;
  const idempotencyKey = params.idempotencyKey ?? stableKey(keyStorage);

  if (!idempotencyKey) {
    throw new PaymentClientError(
      "Bắt buộc có Idempotency-Key khi thanh toán (BR-O5)",
      "MISSING_IDEMPOTENCY_KEY",
      400,
    );
  }

  const effectiveReturnUrl =
    returnUrl || `/checkout/${encodeURIComponent(eventId)}/result?orderId=${encodeURIComponent(orderId)}`;

  const send = () =>
    paymentFetch(
      `/v1/events/${encodeURIComponent(eventId)}/payments`,
      {
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
      },
      "Không thể kết nối đến máy chủ thanh toán (BR-O5)",
    );

  let response = await send();
  // 409 "cung khoa dang duoc xu ly": cho Retry-After roi gui lai CUNG khoa (toi da 3 lan gui).
  let inProgress = false;
  for (let attempt = 1; response.status === 409; attempt++) {
    inProgress = (await readProblem(response)).type === IDEMPOTENCY_IN_PROGRESS;
    if (!inProgress || attempt >= IN_PROGRESS_MAX_ATTEMPTS) break;
    await sleep(retryAfterMs(response));
    response = await send();
  }
  // G-6: co "dang xu ly" chi tinh theo phan hoi CUOI; 409 roi 4xx dinh doat thi van phai xoa khoa.
  if (response.status !== 409) inProgress = false;

  if (!params.idempotencyKey && (response.ok || (isDefinitiveStatus(response.status) && !inProgress))) {
    forgetKey(keyStorage);
  }

  if (inProgress && response.status === 409) {
    throw new PaymentClientError("Yêu cầu thanh toán đang được xử lý, vui lòng chờ giây lát", "AMBIGUOUS", 409);
  }

  if (response.ok) {
    const data = (await response.json().catch(() => null)) as Record<string, unknown> | null;
    if (!data || typeof data.payment_id !== "string" || !data.payment_id) {
      throw new PaymentClientError("Phản hồi thanh toán không hợp lệ", "UNKNOWN", response.status);
    }
    return {
      paymentId: data.payment_id,
      orderId: typeof data.order_id === "string" ? data.order_id : orderId,
      redirectUrl:
        typeof data.redirect_url === "string" && data.redirect_url ? data.redirect_url : effectiveReturnUrl,
      status: data.status === "PROCESSING" ? "PROCESSING" : "PENDING",
      providerTxnId: typeof data.provider_txn_id === "string" ? data.provider_txn_id : undefined,
    };
  }

  const forbidden = await forbiddenError(response);
  if (forbidden) throw forbidden;
  if (response.status === 409) {
    throw new PaymentClientError("Phiên giữ vé đã hết hạn trước khi thanh toán (BR-O2)", "HOLD_EXPIRED", 409);
  }
  if (response.status === 422 || response.status === 400) {
    const p = await readProblem(response);
    throw new PaymentClientError(p.detail || "Thông tin thanh toán không hợp lệ", "INVALID_INPUT", response.status);
  }
  if (response.status >= 500) {
    throw new PaymentClientError("Hệ thống thanh toán đang xử lý hoặc phản hồi chậm", "AMBIGUOUS", response.status);
  }
  throw new PaymentClientError(`Lỗi thanh toán HTTP ${response.status}`, "UNKNOWN", response.status);
}

const ORDER_STATUSES: ReadonlySet<string> = new Set(["PENDING", "PAID", "FAILED", "EXPIRED", "CANCELLED"]);

interface WireOrderItem {
  ticket_type_id?: string;
  tier_name?: string;
  quantity?: number;
  unit_cents?: number;
}

interface WireOrder {
  order_id?: string;
  event_id?: string;
  event_title?: string;
  venue?: string;
  tier_name?: string;
  quantity?: number;
  total_amount?: number;
  total_cents?: number;
  status?: string;
  expires_at?: string;
  hold_id?: string;
  items?: WireOrderItem[] | null;
  issued_tickets?: IssuedTicket[] | null;
  paid_at?: string;
  payment_method?: PaymentMethod;
  failure_reason?: string;
}

/** Tra cuu don hang that tu backend (GET /v1/orders/{orderId}, AC-14). Don chi hien cho chu don.
 *  404 -> NOT_FOUND. Khong dung cache, khong bia quantity / ten hang ve / ten su kien. */
export async function getOrderDetails(orderId: string, eventId: string): Promise<OrderDetails> {
  const res = await paymentFetch(
    `/v1/orders/${encodeURIComponent(orderId)}`,
    { method: "GET", headers: { Accept: "application/json" } },
    "Không thể kết nối máy chủ để kiểm tra đơn hàng",
  );

  if (res.status === 404) {
    throw new PaymentClientError("Không tìm thấy đơn hàng", "NOT_FOUND", 404);
  }
  const forbidden = await forbiddenError(res);
  if (forbidden) throw forbidden;
  if (!res.ok) {
    throw new PaymentClientError(`Lỗi máy chủ HTTP ${res.status}`, "UNKNOWN", res.status);
  }

  const data = (await res.json().catch(() => null)) as WireOrder | null;
  if (!data || typeof data.status !== "string" || !ORDER_STATUSES.has(data.status)) {
    throw new PaymentClientError("Phản hồi đơn hàng không hợp lệ", "UNKNOWN", res.status);
  }

  const items: OrderItem[] = Array.isArray(data.items)
    ? data.items.map((it) => ({
        ticketTypeId: String(it.ticket_type_id ?? ""),
        tierName: String(it.tier_name ?? ""),
        quantity: Number(it.quantity ?? 0),
        unitCents: Number(it.unit_cents ?? 0),
      }))
    : [];

  const details: OrderDetails = {
    orderId: data.order_id || orderId,
    eventId: data.event_id || eventId,
    eventTitle: data.event_title ?? "",
    venue: data.venue ?? "",
    tierName: data.tier_name ?? items[0]?.tierName ?? "",
    quantity: typeof data.quantity === "number" ? data.quantity : items.reduce((n, it) => n + it.quantity, 0),
    totalAmount: typeof data.total_amount === "number" ? data.total_amount : Number(data.total_cents ?? 0),
    status: data.status as OrderStatus,
    expiresAt: data.expires_at,
    holdId: data.hold_id,
    items,
    paidAt: data.paid_at,
    paymentMethod: data.payment_method,
    failureReason: data.failure_reason,
    issuedTickets: Array.isArray(data.issued_tickets) ? data.issued_tickets : [],
  };
  saveRecentOrder(details);
  return details;
}

/** Danh sach cong thanh toan tu backend (GET /v1/payments/methods, public, AC-13).
 *  Reject khi loi: UI hien loi + thu lai, khong co danh sach cung. */
export async function listPaymentMethods(): Promise<PaymentMethodInfo[]> {
  const res = await paymentFetch(
    "/v1/payments/methods",
    { method: "GET", auth: false, headers: { Accept: "application/json" } },
    "Không thể tải danh sách cổng thanh toán",
  );
  if (!res.ok) {
    throw new PaymentClientError(
      `Không thể tải danh sách cổng thanh toán (HTTP ${res.status})`,
      "UNKNOWN",
      res.status,
    );
  }
  const data = (await res.json().catch(() => null)) as { methods?: unknown } | null;
  if (!data || !Array.isArray(data.methods)) {
    throw new PaymentClientError("Phản hồi danh sách cổng thanh toán không hợp lệ", "UNKNOWN", res.status);
  }
  return (data.methods as Array<Record<string, unknown>>).map((m) => ({
    code: String(m.code) as PaymentMethod,
    name: String(m.name ?? m.code ?? ""),
    description: String(m.description ?? ""),
    enabled: m.enabled === true,
    icon: String(m.icon ?? ""),
  }));
}

const ISSUE_KEY_PREFIX = "ef:issue:";

/** Idempotency-Key on dinh theo don (sessionStorage `ef:issue:{orderId}`): reload khong tao yeu cau moi (AC-15). */
function issueKeyFor(orderId: string): string {
  const storageKey = `${ISSUE_KEY_PREFIX}${orderId}`;
  try {
    const existing = sessionStorage.getItem(storageKey);
    if (existing) return existing;
    const fresh = generateIdempotencyKey();
    sessionStorage.setItem(storageKey, fresh);
    return fresh;
  } catch {
    return generateIdempotencyKey();
  }
}

export function forgetIssueKey(orderId: string): void {
  try {
    sessionStorage.removeItem(`${ISSUE_KEY_PREFIX}${orderId}`);
  } catch {
    // bo qua
  }
}

/** Lay ve kem QR payload da ky (POST /v1/orders/{id}/issue-tickets, idempotent).
 *  Chi tra ve cac ve co `qr_payload` that; khong bao gio dung payload gia. */
export async function issueTickets(orderId: string): Promise<IssuedTicketQR[]> {
  const res = await paymentFetch(
    `/v1/orders/${encodeURIComponent(orderId)}/issue-tickets`,
    { method: "POST", headers: { "Idempotency-Key": issueKeyFor(orderId) } },
    "Không thể kết nối máy chủ để lấy vé",
  );
  if (!res.ok) {
    // Phan hoi loi HTTP co the bi middleware idempotency ghi nho theo key: lan thu lai dung key moi.
    forgetIssueKey(orderId);
    const forbidden = await forbiddenError(res);
    if (forbidden) throw forbidden;
    const code: PaymentClientErrorType = res.status === 404 ? "NOT_FOUND" : "UNKNOWN";
    throw new PaymentClientError(`Chưa thể phát hành vé (HTTP ${res.status})`, code, res.status);
  }
  const data = (await res.json().catch(() => null)) as { tickets?: unknown } | null;
  const list = Array.isArray(data?.tickets) ? (data.tickets as Array<Record<string, unknown>>) : [];
  const tickets = list
    .filter((t) => typeof t.qr_payload === "string" && t.qr_payload !== "")
    .map((t) => ({
      ticketId: String(t.id),
      ticketTypeId: String(t.ticket_type_id ?? ""),
      status: String(t.status ?? ""),
      qrPayload: t.qr_payload as string,
    }));
  // 2xx nhung chua co ve QR: middleware idempotency se phat lai dung phan hoi rong cho cung khoa -> doi khoa (F-13).
  if (tickets.length === 0) forgetIssueKey(orderId);
  return tickets;
}
