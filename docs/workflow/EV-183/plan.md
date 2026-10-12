# EV-183 — Kế hoạch thực thi (Plan)

## 1. Thứ tự thực hiện

| Bước | Nội dung | Lý do / Ràng buộc |
|---|---|---|
| **1** | Hiện thực `apps/web/src/lib/hold-client.ts` + `hold-client.test.ts` | Tầng API client gửi `Idempotency-Key` (BR-O5), `X-Queue-Token`, `X-Identity-Id`, chuẩn hoá các mã lỗi 403, 409, 422, ambiguous |
| **2** | Hiện thực `apps/web/src/hooks/use-hold-timer.ts` + `use-hold-timer.test.tsx` | Hook điều phối vòng đời hold, đếm ngược `expires_at` (BR-O2), lưu storage, chống đổi giờ máy khách |
| **3** | Hiện thực `apps/web/src/components/checkout/checkout-view.tsx` + `checkout-view.test.tsx` | Component hiển thị danh sách `TicketTierCard`, giỏ hàng, `OrderSummary`, xử lý cảnh báo BR-O3 |
| **4** | Hiện thực `apps/web/src/app/(checkout)/checkout/[eventId]/page.tsx` + `page.test.tsx` | Route page hoàn chỉnh, tích hợp với `getEventById`, kiểm tra queue token guard (AC-6) |
| **5** | Chạy Gate tự động (typecheck, lint, test, build) & thu thập `evidence.md`, `traceability.md` | Xác minh 100% test pass, không có lỗi hồi quy |
| **6** | Adversarial Review (`code-review.md`) & Commit theo Conventional Commits | Đảm bảo 4 quy tắc bất biến và chất lượng mã nguồn |

## 2. Danh sách File thay đổi

### Thêm mới
- `apps/web/src/lib/hold-client.ts`
- `apps/web/src/lib/hold-client.test.ts`
- `apps/web/src/hooks/use-hold-timer.ts`
- `apps/web/src/hooks/use-hold-timer.test.tsx`
- `apps/web/src/components/checkout/checkout-view.tsx`
- `apps/web/src/components/checkout/checkout-view.test.tsx`
- `apps/web/src/app/(checkout)/checkout/[eventId]/page.tsx`
- `apps/web/src/app/(checkout)/checkout/[eventId]/page.test.tsx`
- `docs/workflow/EV-183/baseline.md`
- `docs/workflow/EV-183/spec.md`
- `docs/workflow/EV-183/plan.md`
- `docs/workflow/EV-183/traceability.md`
- `docs/workflow/EV-183/evidence.md`
- `docs/workflow/EV-183/code-review.md`

### Sửa đổi (nếu cần thiết)
- Không sửa file cũ nếu không bắt buộc.

### TUYỆT ĐỐI CẤM SỬA
- `apps/web/src/components/checkout/order-summary.test.tsx` (baseline test)
- `apps/web/src/components/checkout/ticket-tier-card.test.tsx` (baseline test)
- `apps/web/src/hooks/use-server-countdown.test.tsx` (baseline test)
- `vitest.config.ts`, `package.json` (không hạ ngưỡng coverage)
- Bất kỳ logic backend nào trong `services/ticketing`

## 3. Hợp đồng API / Type Signatures

### 3.1 `hold-client.ts`
```ts
export interface CreateHoldParams {
  eventId: string;
  ticketTypeId: string;
  quantity: number;
  queueToken: string;
  identityId?: string;
  idempotencyKey?: string;
  apiBaseUrl?: string;
}

export interface HoldResult {
  orderId: string;
  holdId: string;
  expiresAt: string; // ISO-8601 string from server (BR-O2)
  ticketTypeId: string;
  quantity: number;
}

export type HoldClientErrorType =
  | "MISSING_IDEMPOTENCY_KEY"
  | "NOT_ADMITTED"        // 403 (ErrNotAdmitted)
  | "SOLD_OUT"            // 409 (ErrSoldOut)
  | "LIMIT_EXCEEDED"       // 422 (ErrPerIdentityLimit / ErrQuantityNotAllowed)
  | "AMBIGUOUS"           // Mạng đứt / timeout / 503 (BR-O5)
  | "UNKNOWN";

export class HoldClientError extends Error {
  readonly type: HoldClientErrorType;
  readonly status?: number;
  constructor(message: string, type: HoldClientErrorType, status?: number);
}

export async function requestHold(params: CreateHoldParams): Promise<HoldResult>;
```

### 3.2 `use-hold-timer.ts`
```ts
export interface UseHoldTimerOptions {
  eventId: string;
  offsetMs?: number;
  apiBaseUrl?: string;
}

export interface UseHoldTimerReturn {
  hold: HoldResult | null;
  holdState: HoldState; // "idle" | "creating" | "active" | "expired" | "sold_out" | "ambiguous"
  remainingMs: number;
  expired: boolean;
  error: HoldClientError | null;
  createHold: (ticketTypeId: string, quantity: number) => Promise<boolean>;
  resetHold: () => void;
}

export function useHoldTimer(options: UseHoldTimerOptions): UseHoldTimerReturn;
```
