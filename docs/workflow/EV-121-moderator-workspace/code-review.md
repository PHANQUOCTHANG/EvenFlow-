# EV-121 — Review code

| Mục | Giá trị |
|---|---|
| Người review | Người hiện thực tự review — **không phải reviewer context sạch**; cần 1 reviewer khi mở PR |
| Đầu vào | `feature/EV-121-moderator-workspace` so với `develop` @ `43375eb` |
| Kết luận | **PASS sau khi sửa** — 2 lỗi tìm thấy trong lúc làm đã sửa (R1, R2); còn lại ghi nhận |

| # | Mức | Phát hiện | Xử lý |
|---|---|---|---|
| R1 | Major (UX tranh chấp) | Moderator đến sau nhận 422 "invalid state transition" thay vì 409 | `alreadyDecided()`; 3 test app; chạy thật 200 / 409 |
| R2 | Major (UI) | `key={status}` trên `DecisionPanel` làm mất thông báo "Đã duyệt" sau khi quyết | Trạng thái "vừa quyết" giữ ở `ReviewDetail` |
| R3 | Ghi nhận | `GetModerationDetail` đọc 4 trục từ `audit_logs.metadata` — phụ thuộc định dạng audit của `HandleAiVerdict` | Có test; thêm cột `ai_axes` cần migration (spec §5) |
| R4 | Ghi nhận | Hàng đợi gọi `GetByID` từng sự kiện (N+1) | Hàng đợi nhỏ (chỉ hồ sơ chờ người); tối ưu khi có số đo |
| R5 | Ghi nhận | `apps/web` không sửa `lib/api-client.ts` (nhánh `frontend-debt-r4` đang sửa) — `moderation-service.ts` dùng lại `apiFetch` / `toApiError` | Tránh xung đột với đồng đội |

## Quy tắc dự án

- Hexagonal: `domain` không import hạ tầng; `app` chỉ qua `port` (`ModerationDecisionStore` mới); SQL chỉ ở `adapter/postgres`.
- Bất biến 1: quyết định + review + audit trong một transaction (AC-3); không còn `_ =` nuốt lỗi audit.
- RFC 7807: `type` riêng cho 422 lật ngược thiếu lý do và 409 đã quyết.
- Không migration; không đổi gateway.
