# EV-121 — Khu làm việc Moderator: hàng đợi, thẩm định, duyệt / từ chối / lật ngược AI (Spec)

| Mục | Giá trị |
|---|---|
| Jira | EV-121 (mã backlog **EVF-24**) — 5 SP — P1 — Sprint 4 |
| Phụ thuộc | EV-120 (job kiểm duyệt AI, PR #59 + bugfix #68), EV-114 (RBAC) |
| Branch | `feature/EV-121-moderator-workspace` (từ `develop` @ `43375eb`) |
| Quy tắc | BR-E4 (ngưỡng AI), BR-E5 (mọi quyết định AI ghi audit và **luôn lật ngược được**) — [01-nghiep-vu.md](../../01-nghiep-vu.md) |
| Thiết kế | [08-stitch-ui-ux-handoff.md](../../08-stitch-ui-ux-handoff.md) §4 Moderator, §10.15, §10.16; ảnh `apps/web/doc/design/stitch/page/image/Hàng đợi Kiểm duyệt Sự kiện…png`, `Thẩm Định Hồ Sơ Sự Kiện…png` |
| Tier | Critical ở backend (audit + quyết định không đảo được), Standard ở frontend |

## 1. Bài toán

Jira: *"Là Moderator, tôi muốn danh sách sự kiện cần xem xét cùng lý do AI để ra quyết định nhanh và chính xác."*
Phạm vi: **API + UI** danh sách `NEEDS_HUMAN` kèm điểm 4 trục và lý do AI; duyệt, từ chối, lật ngược quyết định tự
động; ghi `audit_log`. AC: (1) mọi lần lật ngược quyết định AI ghi audit kèm **lý do bắt buộc**; (2) chỉ vai trò
Moderator thao tác được.

Story đã có phần API ở commit `f5614fe` nhưng **không có UI nào** — vai trò Moderator chưa có route trong `apps/web`.

## 2. Hiện trạng (đã kiểm trên `develop` @ `43375eb`)

| # | Lỗ hổng | Hệ quả |
|---|---|---|
| H1 | `apps/web` không có `/moderator/*`, không có client nào gọi `moderation/queue`, `approve`, `reject` | Moderator không làm được việc qua giao diện; sự kiện `NEEDS_HUMAN` nằm ở `PENDING_REVIEW` |
| H2 | `ModeratorActionUseCase.Approve` cho lật ngược `REJECTED → SCHEDULED` với `note` **tuỳ chọn** | Trái AC-1 Jira (lý do bắt buộc khi lật ngược) |
| H3 | Ghi review (`SetHumanDecision`) và audit dùng `_ = …` (nuốt lỗi), ghi ngoài transaction cập nhật sự kiện | Audit có thể mất mà không ai biết — trái AC-1 và bất biến 1 ([07 §5](../../07-Quy_trinh_lam_viec_nhom.md)) |
| H4 | Hàng đợi chỉ trả `NEEDS_HUMAN`; sự kiện AI tự từ chối không hiện ở đâu cho Moderator | BR-E5 "luôn có thể lật ngược" không có đường vào |
| H5 | Không có API đọc chi tiết hồ sơ cho Moderator (review + audit); điểm 4 trục chỉ nằm trong `audit_logs.metadata` của AI | UI không hiện được "kèm điểm 4 trục" và lịch sử audit |

## 3. Tiêu chí nghiệm thu

| AC | Nội dung |
|---|---|
| AC-1 | *(Jira)* Mọi lần lật ngược quyết định AI ghi `audit_log` kèm lý do **bắt buộc**: duyệt sự kiện AI đã từ chối mà thiếu lý do → 422 `override-reason-required`; từ chối luôn cần lý do |
| AC-2 | *(Jira)* Chỉ vai trò Moderator thao tác được: API 403 cho vai trò khác (đã có); UI chặn bằng `RoleGuard` và không hiện nút hành động khi API trả 403 |
| AC-3 | Quyết định của Moderator (trạng thái sự kiện, `human_verdict` của review, audit) nằm trong **một transaction**; lỗi bất kỳ bước nào → không ghi gì và trả lỗi |
| AC-9 | Hai Moderator quyết cùng một hồ sơ: đúng một người thắng; người kia nhận **409** `/problems/already-decided` — cả khi hai request chạy song song tới DB (điều kiện `status = trạng thái cũ`) lẫn khi người sau đến sau lúc người đầu đã commit |
| AC-4 | Hàng đợi gồm `NEEDS_HUMAN` chưa quyết **và** `AUTO_REJECTED` chưa có quyết định người (được đánh dấu "AI đã từ chối — có thể lật ngược") |
| AC-5 | `GET /api/v1/moderation/events/{id}`: sự kiện + hạng vé, review AI (điểm, lý do, verdict, quyết định người nếu có), 4 trục lấy từ audit AI mới nhất, lịch sử audit theo thời gian |
| AC-6 | UI `/moderator/reviews`: thẻ chỉ số (chờ duyệt, AI đã từ chối, thời gian chờ lâu nhất), lọc (tất cả / cần người duyệt / AI đã từ chối), tìm theo tên, trạng thái đang tải / rỗng / lỗi có thử lại |
| AC-7 | UI `/moderator/reviews/[eventId]`: tách **nội dung gốc của BTC** khỏi **phân tích AI**; 4 trục khi có; lịch sử audit; Duyệt / Từ chối có hộp xác nhận, lý do bắt buộc khi từ chối hoặc lật ngược; trạng thái đã quyết / khoá, không tìm thấy, không có quyền, lỗi gửi |
| AC-8 | AI không bao giờ là người quyết: UI ghi rõ khuyến nghị AI chỉ để tham khảo; không có nút "chấp nhận theo AI" một chạm |

## 4. Thiết kế

### 4.1. Backend (event service, hexagonal)

| Tầng | Thay đổi |
|---|---|
| `domain` | `ErrOverrideReasonRequired`; hàm thuần `IsAIOverride(evt)` (= sự kiện đang `REJECTED`) |
| `port` | `ModerationDecisionStore.ApplyDecision(ctx, evt, reviewID, human, verdict, note, audit) error` — một transaction; `ModerationRepository.ListPendingHuman` (thay `ListNeedsHuman`); `AuditLogRepository.ListBySubject` (đã có) |
| `app` | `Approve` bắt buộc lý do khi `IsAIOverride`; `Approve`/`Reject` gọi `ApplyDecision`, không nuốt lỗi; `GetModerationDetail` gom sự kiện + review + audit + 4 trục |
| `adapter/postgres` | `ApplyDecision` trong `BeginTx`; truy vấn hàng đợi mới |
| `adapter/http` | `GET /api/v1/moderation/events/{id}` (moderator); map `ErrOverrideReasonRequired` → 422 `type: /problems/override-reason-required` |

Không migration (4 trục đọc từ `audit_logs.metadata.axes` của bản ghi AI). Gateway đã định tuyến `/api/v1/moderation/`
với vai trò `moderator` — không đổi.

### 4.2. Frontend (`apps/web`)

- `lib/moderation-service.ts`: kiểu wire + hàm gọi qua `apiFetch` (không sửa `lib/api-client.ts` — nhánh
  `frontend-debt-r4` của đồng đội đang sửa file đó).
- `app/(moderator)/layout.tsx` (`WorkspaceShell` + `MODERATOR_NAV`), `moderator/page.tsx` chuyển hướng,
  `moderator/reviews/page.tsx`, `moderator/reviews/[eventId]/page.tsx`, bọc `RoleGuard allowedRoles={["moderator"]}`.
- `components/moderator/`: `review-queue`, `review-detail`, `decision-panel` (dùng `ConfirmDialog` / `Badge` / state
  pattern sẵn có).
- `MODERATOR_NAV` trong `nav-config.ts`; `/moderator/appeals` để `ready: false` (anti-bot EV-177 chưa có).

### 4.3. Quyết định chốt trong lúc làm

- **Không khoá cứng sau khi đã có người quyết.** Test có sẵn `TestHTTP_ModeratorApprove_And_Reject` và chú thích
  `ApproveOverride` coi "duyệt lại sự kiện Moderator khác đã từ chối, kèm lý do" là hợp lệ. Giữ hành vi đó: duyệt sự
  kiện `REJECTED` (do AI **hoặc** người) luôn cần lý do; audit phân biệt `event.moderator_overrode_ai` và
  `event.moderator_overrode_decision` (metadata `previous_human_verdict`). Tranh chấp thật (cùng lúc, cùng trạng thái
  cũ) vẫn bị chặn bởi AC-9.
- **Người đến sau là 409, không phải 422.** Chạy thật phát hiện: Moderator thứ hai đọc thấy sự kiện đã `SCHEDULED` và
  rơi vào lỗi chuyển trạng thái chung (422 "invalid state transition"). `alreadyDecided()` đổi các trường hợp đó thành
  `ErrAlreadyDecided`; `DRAFT`, `CANCELLED`… vẫn là lỗi trạng thái.

## 5. Ngoài phạm vi

- `/moderator/appeals` (khiếu nại anti-bot) — cần EV-177.
- Lật ngược sự kiện AI đã **duyệt** (`SCHEDULED → REJECTED`) — máy trạng thái chưa cho; ghi nợ.
- Cột `ai_axes` trong `moderation_reviews` (cần migration, chờ đội duyệt).
- Gán hồ sơ cho từng Moderator, SLA, phân trang server-side.
- **Phát hiện khi chạy thật, có sẵn trên `develop`:** tạo sự kiện trùng slug (slug sinh từ tiêu đề) trả **500**
  (`events_slug_key` 23505) thay vì 409 hoặc tự thêm hậu tố — nên mở bugfix cho EV-117.
