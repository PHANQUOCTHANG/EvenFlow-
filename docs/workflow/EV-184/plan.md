# EV-184 — Kế hoạch thực thi (Plan)

## 1. Thứ tự thực hiện

| Bước | Nội dung | Lý do / Ràng buộc |
|---|---|---|
| **1** | Hiện thực `apps/web/src/lib/payment-client.ts` + `payment-client.test.ts` | Tầng API client khởi tạo phiên thanh toán (BR-O5 Idempotency-Key), adapter cổng thanh toán (VNPay/MoMo/Sandbox) và polling đơn hàng |
| **2** | Hiện thực `apps/web/src/hooks/use-payment-session.ts` + `use-payment-session.test.tsx` | Hook quản lý form thanh toán, theo dõi hold expiry (BR-O2), xử lý trạng thái submitting/redirecting và chống duplicate |
| **3** | Hiện thực `apps/web/src/hooks/use-order-status.ts` + `use-order-status.test.tsx` | Hook polling trạng thái đơn hàng (BR-O6, webhook chậm hiển thị PENDING, adaptive backoff) |
| **4** | Hiện thực `apps/web/src/components/checkout/payment-handoff-view.tsx` + `payment-handoff-view.test.tsx` | Màn hình chọn phương thức thanh toán, tóm tắt đơn, đồng hồ giữ vé, khoá khi hold expired |
| **5** | Hiện thực `apps/web/src/components/checkout/order-result-view.tsx` + `order-result-view.test.tsx` | Màn hình kết quả đơn hàng: PENDING (đang xác nhận), PAID (vé điện tử + QR demo), FAILED, EXPIRED, CANCELLED |
| **6** | Hiện thực Page routes Next.js: `payment/page.tsx` và `result/page.tsx` + page tests | Routes dưới route group `(checkout)` |
| **7** | Chạy Gate tự động (typecheck, lint, test, build) & thu thập `evidence.md`, `traceability.md` | Xác minh 100% test pass, đạt ngưỡng coverage |
| **8** | Adversarial Review (`code-review.md`) & Commit theo Conventional Commits | Đảm bảo 4 quy tắc bất biến và chuẩn chỉ mã nguồn |

## 2. Danh sách File thay đổi

### Thêm mới
- `apps/web/src/lib/payment-client.ts`
- `apps/web/src/lib/payment-client.test.ts`
- `apps/web/src/hooks/use-payment-session.ts`
- `apps/web/src/hooks/use-payment-session.test.tsx`
- `apps/web/src/hooks/use-order-status.ts`
- `apps/web/src/hooks/use-order-status.test.tsx`
- `apps/web/src/components/checkout/payment-handoff-view.tsx`
- `apps/web/src/components/checkout/payment-handoff-view.test.tsx`
- `apps/web/src/components/checkout/order-result-view.tsx`
- `apps/web/src/components/checkout/order-result-view.test.tsx`
- `apps/web/src/app/(checkout)/checkout/[eventId]/payment/page.tsx`
- `apps/web/src/app/(checkout)/checkout/[eventId]/payment/page.test.tsx`
- `apps/web/src/app/(checkout)/checkout/[eventId]/result/page.tsx`
- `apps/web/src/app/(checkout)/checkout/[eventId]/result/page.test.tsx`
- `docs/workflow/EV-184/baseline.md`
- `docs/workflow/EV-184/spec.md`
- `docs/workflow/EV-184/plan.md`
- `docs/workflow/EV-184/traceability.md`
- `docs/workflow/EV-184/evidence.md`
- `docs/workflow/EV-184/code-review.md`

### Sửa đổi (nếu cần thiết)
- `apps/web/src/components/checkout/index.ts` (nếu cần export các component mới)
- `apps/web/src/lib/index.ts` (nếu có)

### TUYỆT ĐỐI CẤM SỬA
- `apps/web/src/components/checkout/order-summary.test.tsx` (baseline test)
- `apps/web/src/components/checkout/ticket-tier-card.test.tsx` (baseline test)
- `apps/web/src/components/checkout/checkout-view.test.tsx` (baseline test)
- `apps/web/src/hooks/use-server-countdown.test.tsx` (baseline test)
- `vitest.config.ts`, `package.json` (không hạ ngưỡng coverage)
- Bất kỳ code backend nào trong `services/ticketing` hay `services/waitingroom`

## 3. Hợp đồng API / Type Signatures

### 3.1 `payment-client.ts`
```ts
export type PaymentMethod = "vnpay" | "momo" | "sandbox";

export interface InitiatePaymentParams {
  eventId: string;
  orderId: string;
  holdId: string;
  amount: number;
  method: PaymentMethod;
  buyerName: string;
  buyerEmail: string;
  buyerPhone: string;
  idempotencyKey?: string; // BR-O5
  apiBaseUrl?: string;
  returnUrl?: string;
}

export interface PaymentInitiationResult {
  paymentId: string;
  orderId: string;
  redirectUrl: string;
  status: "PENDING" | "PROCESSING";
}

export type OrderStatus = "PENDING" | "PAID" | "FAILED" | "EXPIRED" | "CANCELLED";

export interface IssuedTicket {
  ticketId: string;
  ticketCode: string;
  tierName: string;
  attendeeName: string;
  qrPayload: string; // Demo payload
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

export async function initiatePayment(params: InitiatePaymentParams): Promise<PaymentInitiationResult>;
export async function getOrderDetails(orderId: string, eventId: string, apiBaseUrl?: string): Promise<OrderDetails>;
```

### 3.2 `use-payment-session.ts`
```ts
export interface UsePaymentSessionOptions {
  eventId: string;
  orderId: string;
  holdId: string;
  amount: number;
  expiresAt?: string;
  offsetMs?: number;
  apiBaseUrl?: string;
}

export interface UsePaymentSessionReturn {
  submitting: boolean;
  holdExpired: boolean;
  error: string | null;
  submitPayment: (formData: PaymentFormData) => Promise<PaymentInitiationResult | null>;
}
```

### 3.3 `use-order-status.ts`
```ts
export interface UseOrderStatusOptions {
  orderId: string;
  eventId: string;
  initialDetails?: OrderDetails | null;
  pollIntervalMs?: number;
  maxPollAttempts?: number;
}

export interface UseOrderStatusReturn {
  order: OrderDetails | null;
  status: OrderStatus;
  loading: boolean;
  pollAttempts: number;
  isPendingWebhook: boolean;
  refresh: () => Promise<void>;
}
```
