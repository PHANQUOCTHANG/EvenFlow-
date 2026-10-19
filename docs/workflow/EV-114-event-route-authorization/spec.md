# EV-114 — Gắn phân quyền vào route event, lấy organizer_id từ token (Spec)

| Mục | Giá trị |
|---|---|
| Jira | EV-114 (mã backlog **EVF-14**, RBAC) — 3 SP — Sprint A, việc #3 |
| Kế hoạch tổng | [10-ke-hoach-hoan-thien-du-an.md](../../10-ke-hoach-hoan-thien-du-an.md) — làn bảo mật #1 → #2 → **#3** → #9 |
| Branch | `feature/EV-114-event-route-authorization`, chồng trên `feature/gateway-auth-routing` (cần danh tính do gateway gắn) |
| Ma trận quyền | [01-nghiep-vu.md](../../01-nghiep-vu.md) mục 3 |

Nhánh remote `feature/EV-114-rbac-authorization` (đã merge) làm phần middleware **trong identity**; slice này gắn quyền
vào **event-svc**, nơi chưa route nào được bảo vệ.

## 1. Bài toán (`develop` @ `f2154d8`)

| # | Lỗ hổng | Hệ quả |
|---|---|---|
| L1 | Không route nào của event kiểm danh tính | Ai cũng gọi được `approve` / `reject` / `publish` |
| L2 | `organizer_id` lấy từ **body** (tạo, sửa, thêm hạng vé, đổi quota, nộp duyệt, huỷ) và từ query (danh sách) | Gửi `organizer_id` của người khác là sửa/huỷ được sự kiện của họ |
| L3 | `moderator_id` lấy từ body, không có thì đi nhánh không audit | Duyệt giả danh moderator khác hoặc không để lại dấu vết |
| L4 | `GET /events/{id}` trả cả sự kiện DRAFT/PENDING_REVIEW/REJECTED | Lộ sự kiện chưa duyệt |
| L5 | Lỗi 500 trả nguyên `err.Error()` | Lộ chi tiết SQL/host ra client |

## 2. Thiết kế

Danh tính đọc từ header gateway gắn sau khi xác minh token (`authx.PrincipalFromHeaders`; header hỏng = không có).
`h.require(handler, roles...)` bọc từng route: thiếu danh tính → 401; sai role → 403 + `audit_logs`
`ACCESS_DENIED` (cùng hằng số với identity).

| Route | Role |
|---|---|
| `GET /api/v1/events/{id}`, `GET /api/v1/events/slug/{slug}` | Công khai; sự kiện chưa duyệt chỉ chủ sự kiện / moderator / ops thấy, người khác 404 |
| `POST /api/v1/events`, `PUT /{id}`, `POST /{id}/tiers`, `PUT /{id}/tiers/{tierId}`, `POST /{id}/submit`, `POST /{id}/cancel` | organizer (`organizer_id` = token; use case kiểm sở hữu) |
| `GET /api/v1/events` | organizer (của chính mình); moderator/ops (bắt buộc `?organizer_id=`) |
| `POST /{id}/approve`, `POST /{id}/reject`, `GET /api/v1/moderation/queue` | moderator (`moderator_id` = token, luôn qua luồng có audit) |
| `POST /{id}/publish` | ops — mở bán là thao tác vận hành, tới khi có scheduler |
| `PATCH /internal/v1/events/{id}/ai-verdict` | Nội bộ; gateway chặn từ ngoài |

Trường `organizer_id` / `moderator_id` trong body và query giữ lại cho client cũ nhưng **chỉ để đối chiếu**: rỗng
thì bỏ qua, lệch với token → 403 + audit `ACCESS_DENIED_CROSS_TENANT`. Organizer đụng sự kiện không phải của mình
(`ErrUnauthorizedOrganizer`) cũng được audit cross-tenant. Body của `submit`/`cancel`/`approve`/`reject` thành tuỳ
chọn. Lỗi 500 trả câu chung, chi tiết vào log.

## 3. Tiêu chí nghiệm thu

| AC | Nội dung |
|---|---|
| AC-1 | Ma trận role × route khớp docs/01: role không được phép → 403, không đổi trạng thái, có audit; không danh tính → 401 |
| AC-2 | Organizer không sửa/thêm vé/huỷ được sự kiện của người khác → 403 + audit cross-tenant, dữ liệu không đổi |
| AC-3 | `organizer_id` / `moderator_id` lấy từ token; body lệch token → 403 + audit; không gửi → dùng token |
| AC-4 | Quyết định duyệt ghi đúng moderator từ token |
| AC-5 | Sự kiện chưa duyệt chỉ chủ/moderator/ops xem được (cả theo id và slug); đã duyệt thì công khai |
| AC-6 | Danh sách: organizer chỉ thấy của mình; moderator/ops chỉ định organizer; buyer 403 |
| AC-7 | Header danh tính hỏng → 401; lỗi 500 không lộ chi tiết |

## 4. Giới hạn

- **Tin header vì sau gateway.** Gọi thẳng cổng 8084 vẫn giả danh được: EV-104 không được publish cổng event.
- **Web:** trang organizer/moderator phải gửi `Authorization` qua gateway; body không cần `organizer_id`/`moderator_id`
  nữa (gửi đúng của mình thì vẫn chạy).
- Báo cáo sau bán (Organizer chỉ xem sự kiện của mình) chưa có route — áp dụng cùng mẫu khi làm.
