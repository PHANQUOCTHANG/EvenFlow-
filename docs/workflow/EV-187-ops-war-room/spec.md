# EV-187 — War room cho Ops (Spec)

| Mục | Giá trị |
|---|---|
| Jira | EV-187 (mã backlog **EVF-117**, issue 1808) — Sprint 5 |
| Branch | `feature/EV-187-ops-war-room` (từ `develop` @ `51c4729`) |
| Tier | **Standard** (10 bước), chỉ frontend `apps/web` |
| Thiết kế | `docs/08-stitch-ui-ux-handoff.md` §10.18; ảnh `apps/web/doc/design/stitch/page/image/Trung tâm điều hành Live Ops War Room.png` |

## 1. Bối cảnh và giới hạn

Jira: "Một màn hình thấy đủ phễu chuyển đổi, admit rate, tồn kho, AI lag; dữ liệu trễ không quá 5s".
Phụ thuộc EVF-55 (admit controller — **đã có** trong waitingroom), EVF-125 (Grafana — **chưa có**), và
liên kết tới EVF-60 (API ghi đè admit_rate — **chưa có**: `SetRateOverride` tồn tại trong
`services/waitingroom/internal/app/admit_controller.go:244` nhưng không có HTTP endpoint nào gọi).

Vì vậy slice này làm **màn hình + hợp đồng dữ liệu + dữ liệu mẫu**, theo đúng mẫu EV-186
(`lib/organizer-service.ts` mock, `hooks/use-organizer-sales.ts` polling). Khi có Ops API, chỉ thay phần
thân của `getOpsWarRoom`. Handoff §10.18 cấm "fabricated green health" và "animate demo metrics as if live":
dữ liệu mẫu phải được **gắn nhãn rõ** là dữ liệu mẫu.

## 2. Hợp đồng dữ liệu (chốt trước)

`apps/web/src/lib/ops-service.ts`:

```ts
export type DegradationMode = "NORMAL" | "SAVING" | "FAQ_ONLY" | "OFF";
export type SourceHealth = "ok" | "stale" | "unavailable";

export interface OpsTierInventory {
  tierId: string; name: string;
  quota: number; held: number; paid: number; issued: number; available: number;
}

export interface OpsWarRoomSnapshot {
  eventId: string; eventTitle: string;
  saleState: "SCHEDULED" | "ON_SALE" | "SOLD_OUT" | "CLOSED";
  generatedAt: string;                 // ISO, giờ server lúc chụp số liệu
  funnel: { lobby: number; queued: number; admitted: number; holding: number; paid: number; issued: number };
  admission: { ratePerSec: number; mode: "AUTO" | "MANUAL"; queueDepth: number };
  inventory: OpsTierInventory[];
  ai: { mode: DegradationMode; manualOverride: boolean; chatQueueLagSeconds: number; budgetUsedRatio: number };
  sources: { waitingroom: SourceHealth; ticketing: SourceHealth; ai: SourceHealth };
  isSample: boolean;                   // true = dữ liệu mẫu, UI phải gắn nhãn
}

export class OpsServiceError extends Error { status: number }   // 403 = không có quyền, 404 = không có sự kiện
export async function getOpsWarRoom(eventId: string): Promise<OpsWarRoomSnapshot>;
export async function listOpsEvents(): Promise<{ eventId: string; eventTitle: string; saleState: OpsWarRoomSnapshot["saleState"] }[]>;
```

Tên trường bám theo nguồn thật để thay được về sau: `admit_rate` / `ZCARD wr:{id}:queue` của waitingroom,
`waitingroom_admit_rate` / `waitingroom_queue_depth`, `ai_queue_lag_seconds` / `ai_degradation_mode` và
`GET /admin/mode` của ai-worker.

## 3. Tiêu chí nghiệm thu

**AC-1 — Một màn hình đủ số liệu.** Given snapshot hợp lệ, When Ops mở `/ops/events/{eventId}`, Then thấy
trên cùng một trang: tên sự kiện và trạng thái bán; phễu 6 bước (lobby → queued → admitted → holding → paid →
issued) với số tuyệt đối; admit rate (đơn vị "khách/giây"), chế độ AUTO/MANUAL, độ sâu hàng chờ; bảng tồn kho
theo hạng vé (quota, đang giữ, đã thanh toán, đã phát hành, còn lại); AI: nấc suy biến, độ trễ hàng đợi chat
(giây), % ngân sách token đã dùng. Mỗi chỉ số có đơn vị.

**AC-2 — Độ tươi ≤ 5s.** Dữ liệu được làm mới tự động mỗi **2s**. Trang hiển thị "cập nhật lúc …" và tuổi dữ
liệu tính từ `generatedAt`. Khi tuổi dữ liệu > 5s (hoặc lần làm mới gần nhất lỗi), trang hiện cảnh báo
**dữ liệu cũ** (role `alert` hoặc `status`) và **không** xoá số liệu cuối cùng.

**AC-3 — Không tô xanh giả.** Khi `isSample = true`, trang hiện nhãn "Dữ liệu mẫu" dễ thấy ở đầu trang.
Một nguồn có `sources.x = "unavailable"` thì các ô thuộc nguồn đó hiện "Không có dữ liệu" thay vì số;
`"stale"` thì ô đó gắn nhãn "cũ". Không có chỉ báo "hệ thống bình thường" nào khi một nguồn không `ok`.

**AC-4 — Bất biến tồn kho (BR-O1).** Hạng vé nào có `held + paid + issued > quota` hoặc `available < 0`
thì hàng đó được đánh dấu lỗi rõ ràng (chữ, không chỉ màu) và trang có cảnh báo "lệch tồn kho".

**AC-5 — Nấc suy biến AI.** Hiển thị đúng một trong `NORMAL / SAVING / FAQ_ONLY / OFF` kèm câu mô tả tác động
tới người mua bằng ngôn ngữ thường; `manualOverride = true` thì ghi rõ "Ops đặt tay".

**AC-6 — Thao tác ghi đè tách khỏi phần giám sát.** Khu vực "Điều khiển" tách riêng; nút ghi đè admit rate /
tạm dừng **bị vô hiệu** với giải thích "Chưa có API ghi đè (EVF-60)". Không có thao tác nào gửi request.

**AC-7 — Trạng thái đặc biệt.** `getOpsWarRoom` ném 403 → màn hình "Không có quyền"; 404 → "Không tìm thấy
sự kiện"; lỗi khác ở lần tải đầu → thông báo lỗi kèm nút thử lại; `saleState` ≠ `ON_SALE` → nhãn "Không có
đợt bán đang diễn ra" (vẫn hiện số liệu nếu có).

**AC-8 — Trang danh sách + điều hướng.** `/ops/events` liệt kê sự kiện từ `listOpsEvents`, mỗi mục là link
tới war room của nó. Mục "War room" trong `OPS_NAV` (`nav-config.ts`) chuyển `ready: true`; test hiện có của
`(ops)/layout` vẫn xanh (có thể cập nhật phần kỳ vọng `ready:false` nếu test đó khẳng định War room chưa sẵn).

**AC-9 — Polling an toàn.** Hook dừng timer khi unmount; không chồng request (lần làm mới mới không bắt đầu
khi lần trước chưa xong); có nút "Làm mới" thủ công; lỗi giữ lại snapshot cuối.

**AC-10 — Không hồi quy, có E2E.** `apps/web` lint/typecheck/test/build pass; thêm 1 kịch bản Playwright
`tests/e2e/specs/ops-war-room.spec.ts` mở `/ops/events` → vào war room → thấy phễu và nhãn "Dữ liệu mẫu"
(dữ liệu mẫu chạy trong server component, không cần giả lập API).

**AC-11 — Truy cập được.** Bảng tồn kho là `<table>` có `<caption>`/header; trạng thái nguồn và cảnh báo có
chữ, không chỉ màu; cập nhật tự động không cướp focus.

## 4. Ngoài phạm vi

- Ops API thật, ghi đè admit rate (EVF-60), audit log, Grafana (EVF-125).
- Biểu đồ xu hướng (repo chưa có thư viện chart; không có dữ liệu chuỗi thời gian) — ghi nợ.
- p99 latency, DB pool, error rate, webhook (handoff §10.18 chỉ yêu cầu khi có số liệu; hợp đồng chưa có nguồn).
- Chặn quyền bằng `RoleGuard` cho cả nhóm `(ops)`: `RoleGuard` chưa được dùng ở đâu, kể cả `(organizer)`;
  bật riêng cho ops sẽ chặn mọi demo không đăng nhập. Ghi nợ chung cho EV-114. Slice này chỉ có trạng thái
  "Không có quyền" khi dịch vụ trả 403.
