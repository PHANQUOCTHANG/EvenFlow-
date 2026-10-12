# EV-186 — Kế hoạch thực thi (Plan)

## 1. Thứ tự thực hiện

| Bước | Nội dung | Lý do / Ràng buộc |
|---|---|---|
| **1** | Hiện thực `apps/web/src/lib/organizer-service.ts` + `organizer-service.test.ts` | Tầng Domain & Service: Types, validation BR-E1 (≥ 1 tier, quota > 0, price >= 0), BR-E2 (sale_start_at >= now + 30m), BR-E3 (cấm giảm quota khi ON_SALE), BR-E4 (AI scan 4 trục, điểm rủi ro), mock repository & realtime snapshot (≤ 5s) |
| **2** | Hiện thực `apps/web/src/hooks/use-organizer-sales.ts` + `use-organizer-sales.test.tsx` | Hook điều phối dữ liệu doanh số realtime: chu kỳ cập nhật ≤ 5s (mặc định 4s), tính toán freshness, toggle auto-refresh, manual refresh |
| **3** | Hiện thực `apps/web/src/components/organizer/event-list.tsx` + `event-list.test.tsx` | Component danh sách sự kiện: bộ lọc trạng thái, tìm kiếm, status badges, action links, empty state |
| **4** | Hiện thực `apps/web/src/components/organizer/event-wizard.tsx` + `event-wizard.test.tsx` | Component Stepper Wizard 4 bước: thông tin cơ bản, hạng vé & quota, lịch mở bán, review & submit kèm validation lỗi trực quan |
| **5** | Hiện thực `apps/web/src/components/organizer/moderation-status-card.tsx` + `moderation-status-card.test.tsx` | Component hiển thị trạng thái kiểm duyệt, AI scan 4 trục (BR-E4), lý do từ chối (BR-E5), audit log và nút nộp lại |
| **6** | Hiện thực `apps/web/src/components/organizer/sales-dashboard.tsx` + `sales-dashboard.test.tsx` + `index.ts` | Component Dashboard doanh số realtime: Metric cards (doanh thu, vé bán, đơn hàng, active holds), bảng chi tiết hạng vé, freshness indicator, báo cáo sau bán |
| **7** | Hiện thực các Route Pages trong `apps/web/src/app/(organizer)/` + tests tương ứng | Route `/organizer/events`, `/organizer/events/new`, `/organizer/events/[eventId]/sales`, `/organizer/events/[eventId]/review-status`, `/organizer/events/[eventId]/edit`, `/organizer` (redirect) |
| **8** | Cập nhật `ORGANIZER_NAV` trong `apps/web/src/components/layout/nav-config.ts` | Đổi `ready: true` cho `/organizer/events` và `/organizer/events/new` |
| **9** | Chạy Gate tự động (typecheck, lint, test, build) & thu thập `evidence.md`, `traceability.md` | Xác minh 100% test pass, không hồi quy |
| **10** | Adversarial Review (`code-review.md`) & Commit theo Conventional Commits | Đảm bảo 4 quy tắc bất biến và chuẩn chỉ mã nguồn |

---

## 2. Danh sách File thay đổi

### Thêm mới
- `apps/web/src/lib/organizer-service.ts`
- `apps/web/src/lib/organizer-service.test.ts`
- `apps/web/src/hooks/use-organizer-sales.ts`
- `apps/web/src/hooks/use-organizer-sales.test.tsx`
- `apps/web/src/components/organizer/event-list.tsx`
- `apps/web/src/components/organizer/event-list.test.tsx`
- `apps/web/src/components/organizer/event-wizard.tsx`
- `apps/web/src/components/organizer/event-wizard.test.tsx`
- `apps/web/src/components/organizer/moderation-status-card.tsx`
- `apps/web/src/components/organizer/moderation-status-card.test.tsx`
- `apps/web/src/components/organizer/sales-dashboard.tsx`
- `apps/web/src/components/organizer/sales-dashboard.test.tsx`
- `apps/web/src/components/organizer/index.ts`
- `apps/web/src/app/(organizer)/organizer/page.tsx`
- `apps/web/src/app/(organizer)/organizer/page.test.tsx`
- `apps/web/src/app/(organizer)/organizer/events/page.tsx`
- `apps/web/src/app/(organizer)/organizer/events/page.test.tsx`
- `apps/web/src/app/(organizer)/organizer/events/new/page.tsx`
- `apps/web/src/app/(organizer)/organizer/events/new/page.test.tsx`
- `apps/web/src/app/(organizer)/organizer/events/[eventId]/sales/page.tsx`
- `apps/web/src/app/(organizer)/organizer/events/[eventId]/sales/page.test.tsx`
- `apps/web/src/app/(organizer)/organizer/events/[eventId]/review-status/page.tsx`
- `apps/web/src/app/(organizer)/organizer/events/[eventId]/review-status/page.test.tsx`
- `apps/web/src/app/(organizer)/organizer/events/[eventId]/edit/page.tsx`
- `apps/web/src/app/(organizer)/organizer/events/[eventId]/edit/page.test.tsx`
- `docs/workflow/EV-186/baseline.md`
- `docs/workflow/EV-186/spec.md`
- `docs/workflow/EV-186/plan.md`
- `docs/workflow/EV-186/traceability.md`
- `docs/workflow/EV-186/evidence.md`
- `docs/workflow/EV-186/code-review.md`

### Sửa đổi
- `apps/web/src/components/layout/nav-config.ts` (bật `ready: true` cho `ORGANIZER_NAV`)

### TUYỆT ĐỐI CẤM SỬA
- `apps/web/src/app/(organizer)/layout.test.tsx` (baseline test cấu trúc shell)
- `apps/web/src/components/layout/workspace-shell.test.tsx` (baseline test)
- `apps/web/src/components/layout/nav-config.test.ts` (không sửa các test assertion)
- `vitest.config.ts`, `package.json` (không hạ ngưỡng kiểm tra)
- Bất kỳ file backend nào trong Go services hoặc `services/ai-worker`

---

## 3. Hợp đồng API / Type Signatures

### 3.1 `apps/web/src/lib/organizer-service.ts`
```ts
export type EventLifecycleState =
  | "DRAFT"
  | "PENDING_REVIEW"
  | "SCHEDULED"
  | "ON_SALE"
  | "SOLD_OUT"
  | "CLOSED"
  | "REJECTED"
  | "CANCELLED";

export interface TicketTierConfig {
  id: string;
  name: string;
  price: number;
  quota: number;
  description?: string;
}

export type ModerationAxis =
  | "PROHIBITED_CONTENT"
  | "MISLEADING_SCAM"
  | "IMAGE_COPYRIGHT"
  | "MISSING_INFO";

export interface ModerationAxisResult {
  axis: ModerationAxis;
  title: string;
  riskScore: number; // 0.0 -> 1.0
  passed: boolean;
  notes?: string;
}

export type ModerationDecision = "AUTO_APPROVED" | "NEEDS_HUMAN" | "AUTO_REJECTED" | "MODERATOR_OVERRIDE";

export interface ModerationDetail {
  overallRiskScore: number;
  decision: ModerationDecision;
  scannedAt: string;
  reasons: string[];
  moderatorNotes?: string;
  axes: ModerationAxisResult[];
}

export interface OrganizerEvent {
  id: string;
  slug: string;
  title: string;
  description: string;
  venue: string;
  startsAt: string;
  saleStartAt: string;
  state: EventLifecycleState;
  organizerId: string;
  tiers: TicketTierConfig[];
  moderation?: ModerationDetail;
  createdAt: string;
  updatedAt: string;
}

export interface TierSalesDetail {
  tierId: string;
  name: string;
  price: number;
  quota: number;
  sold: number;
  activeHolds: number;
  revenue: number;
  fillRatePercentage: number;
}

export interface RealtimeSalesSnapshot {
  eventId: string;
  timestamp: string;
  grossRevenue: number;
  totalTicketsSold: number;
  totalQuota: number;
  overallFillRatePercentage: number;
  paidOrdersCount: number;
  activeHoldsCount: number;
  tierSales: TierSalesDetail[];
  postSaleReport?: {
    averageOrderValue: number;
    peakSalesVelocityTps: number;
    paymentBreakdown: {
      vnpay: number;
      momo: number;
      card: number;
    };
  };
}
```

### 3.2 `apps/web/src/hooks/use-organizer-sales.ts`
```ts
export interface UseOrganizerSalesOptions {
  eventId: string;
  pollingIntervalMs?: number; // mặc định 4000ms (<= 5s)
  initialAutoRefresh?: boolean;
}

export interface UseOrganizerSalesReturn {
  data: RealtimeSalesSnapshot | null;
  isLoading: boolean;
  isRefreshing: boolean;
  error: Error | null;
  lastUpdatedAt: Date | null;
  autoRefresh: boolean;
  setAutoRefresh: (enabled: boolean) => void;
  refresh: () => Promise<void>;
}
```
