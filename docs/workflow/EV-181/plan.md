# EV-181 — Kế hoạch thực thi (Plan)

## 1. Thứ tự thực hiện

| Bước | Nội dung | Lý do / Ràng buộc |
|---|---|---|
| **1** | Hiện thực `src/lib/jitter.ts` & test `src/lib/jitter.test.ts` | Tầng tiện ích tính toán jitter 0..5s ngẫu nhiên (EVF-61, BR-Q5) độc lập, 0 side-effect |
| **2** | Hiện thực route `/api/time/route.ts` & test `src/app/api/time/route.test.ts` | Endpoint cấp giờ server chuẩn có header cache edge 1s |
| **3** | Hiện thực hook `src/hooks/use-server-time-sync.ts` & test | Đồng bộ giờ server và bù trừ RTT qua endpoint `/api/time` |
| **4** | Hiện thực `src/lib/event-service.ts` & test | Snapshot dữ liệu sự kiện ISR, ẩn số lượng tồn kho theo BR-A4 |
| **5** | Hiện thực các Event UI Components & tests | Component thông báo LOBBY (BR-Q1), danh sách vé, panel action đếm ngược + jitter |
| **6** | Hiện thực trang danh sách `events/page.tsx` & trang chi tiết ISR `events/[slug]/page.tsx` | Route ISR Next.js App Router, metadata SEO, fallback 404, cập nhật `nav-config.ts` |
| **7** | Chạy toàn bộ Gate kiểm tra (Lint, Typecheck, Test, Coverage, Build) | Thu thập raw logs cho `evidence.md` và `traceability.md` |
| **8** | AI Adversarial Review độc lập | Rà soát 4 quy tắc bất biến và lập biên bản `code-review.md` |

---

## 2. Danh sách File thay đổi

### Thêm mới
- `apps/web/src/lib/jitter.ts`
- `apps/web/src/lib/jitter.test.ts`
- `apps/web/src/app/api/time/route.ts`
- `apps/web/src/app/api/time/route.test.ts`
- `apps/web/src/hooks/use-server-time-sync.ts`
- `apps/web/src/hooks/use-server-time-sync.test.tsx`
- `apps/web/src/lib/event-service.ts`
- `apps/web/src/lib/event-service.test.ts`
- `apps/web/src/components/event/lobby-notice.tsx`
- `apps/web/src/components/event/lobby-notice.test.tsx`
- `apps/web/src/components/event/ticket-tier-list.tsx`
- `apps/web/src/components/event/ticket-tier-list.test.tsx`
- `apps/web/src/components/event/event-action-panel.tsx`
- `apps/web/src/components/event/event-action-panel.test.tsx`
- `apps/web/src/app/(marketing)/events/page.tsx`
- `apps/web/src/app/(marketing)/events/page.test.tsx`
- `apps/web/src/app/(marketing)/events/[slug]/page.tsx`
- `apps/web/src/app/(marketing)/events/[slug]/page.test.tsx`
- `docs/workflow/EV-181/baseline.md`
- `docs/workflow/EV-181/spec.md`
- `docs/workflow/EV-181/plan.md`
- `docs/workflow/EV-181/traceability.md`
- `docs/workflow/EV-181/evidence.md`
- `docs/workflow/EV-181/code-review.md`

### Sửa đổi
- `apps/web/src/components/layout/nav-config.ts` (Bật `ready: true` cho `/events`)

### TUYỆT ĐỐI CẤM SỬA
- Các test suite baseline đã có (44 test files trước đó).
- `vitest.config.ts`, `tsconfig.json` (không hạ ngưỡng coverage 70/70/60/70).
- Quy tắc an toàn: Không bypass Lua gate, không sửa DB ngoài transaction.

---

## 3. Hợp đồng API / Type Signatures

```typescript
// 1. Jitter Utility Contract
export interface JitterOptions {
  maxJitterMs?: number; // default: 5000ms
  minJitterMs?: number; // default: 0ms
}
export function calculateJitterMs(options?: JitterOptions): number;
export function sleepWithJitter(onProgress?: (remainingMs: number) => void, options?: JitterOptions): Promise<number>;

// 2. Server Time Contract (/api/time)
export interface ServerTimeResponse {
  server_time: number; // Epoch milliseconds
  rfc3339: string;     // ISO-8601 with timezone (e.g. 2026-10-06T14:00:00.000Z)
}

// 3. Server Time Sync Hook
export interface ServerTimeSyncResult {
  offsetMs: number;
  synced: boolean;
  isLoading: boolean;
  error: Error | null;
  serverNow: () => number;
}
export function useServerTimeSync(endpoint?: string): ServerTimeSyncResult;

// 4. Ticket Tier Availability (BR-A4)
export type TierAvailability = "AVAILABLE" | "FEW_LEFT" | "SOLD_OUT";

export interface TicketTierSnapshot {
  id: string;
  name: string;
  price: number;
  description?: string;
  availability: TierAvailability; // Chỉ hiển thị dạng khoảng, KHÔNG lộ quota chính xác
}

export interface EventDetailSnapshot {
  id: string;
  slug: string;
  title: string;
  description: string;
  venue: string;
  startsAt: string;     // ISO-8601 with TZ
  saleStartAt: string;  // T0: ISO-8601 with TZ
  state: "SCHEDULED" | "ON_SALE" | "SOLD_OUT" | "CLOSED" | "COMPLETED" | "CANCELLED";
  bannerUrl?: string;
  tiers: TicketTierSnapshot[];
}
```
