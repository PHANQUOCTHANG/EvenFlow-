# EV-120 (bugfix) — Bằng chứng

Ngày chạy: 2026-10-22. Máy: Windows 11, Docker Desktop. Python 3.13 qua `uv`. Nhánh
`bugfix/EV-120-ai-verdict-callback` từ `develop` @ `5eeab92`. Review và thử phá do người hiện thực tự làm — cần 1
reviewer khi mở PR.

## 1. Kết quả

| Kiểm | Kết quả |
|---|---|
| `services/ai-worker`: `uv run ruff check .` | sạch |
| `services/ai-worker`: `uv run pytest` | **210 passed** (baseline 202; +8 test mới) |
| `services/ai-worker`: `uv run pytest -m integration` (RabbitMQ + Redis thật) | **22 passed** |
| `docker compose -f deploy/compose/docker-compose.yml config -q` | hợp lệ |

## 2. Chạy thật (toàn bộ stack compose, qua gateway `:8080`, JWT thật từ identity)

Đăng ký organizer + 2 moderator bằng OTP (đọc từ mailpit), đổi vai trò trong `identities`, đăng nhập lấy
`access_token`. Organizer tạo + thêm hạng vé + nộp 3 sự kiện; AI chạy ở chế độ mock (luật từ khoá).

| Bước | Trước sửa (`develop`) | Sau sửa |
|---|---|---|
| Log ai-worker | `PATCH http://gateway:8080/internal/v1/events/{id}/ai-verdict "404 Not Found"` → `chuyen vao DLQ` (`permanent`) cho cả 3 | callback tới `http://event:8084`, không lỗi |
| Trạng thái sau vài giây | cả 3 `PENDING_REVIEW`, không có review | "nghi van" → `PENDING_REVIEW` + review `NEEDS_HUMAN`; "vu khi" → `REJECTED` + `AUTO_REJECTED` |
| `GET /api/v1/moderation/queue` (Moderator) | `[]` | có cả 2 hồ sơ |
| Độ sâu `ai.dlq` | 0 → **9** (3 lần chạy thử) | giữ nguyên 9 — không thêm job hỏng |
| `PATCH /internal/...` từ ngoài qua gateway | 404 | 404 — gateway vẫn chặn đúng thiết kế |

Đi tiếp luồng Moderator (EV-121) trên chính các hồ sơ này, qua gateway:

| Bước | Kết quả |
|---|---|
| Organizer xem hàng đợi | 403 |
| Lật ngược sự kiện AI đã từ chối, lý do rỗng | 422 `/problems/override-reason-required` |
| Lật ngược có lý do | 200 → `SCHEDULED`, audit `event.auto_rejected, event.moderator_overrode_ai` |
| Hai Moderator cùng duyệt hồ sơ `NEEDS_HUMAN` | **200 và 409**, audit `event.needs_human_review, event.moderator_approved` (đúng 1 quyết định) |
| Hàng đợi sau quyết định | không còn 2 hồ sơ |

## 3. Thử phá

| # | Đột biến | Kết quả | Bắt bởi |
|---|---|---|---|
| M1 | `build_moderation_consumer` dùng lại `internal_api_base` | Đỏ | `test_ConsumerKiemDuyetGoiEventApiBase_KhongQuaGateway` |
| M2 | Mặc định `event_api_base` = gateway | Đỏ | `test_CallbackKiemDuyet_MacDinhDiThangEventService` |
| M3 | Bỏ validator URL | Đỏ | `test_EventApiBaseSaiBiTuChoi` (4 ca) |
| M4 | Compose trỏ `EVENT_API_BASE` sai cổng (`event:8080`) | Đỏ | `test_CallbackKiemDuyetTroThangEventService_DungCong` |
| M5 | Compose đổi `INTERNAL_API_BASE` sang `event` (sửa "gọn" bằng một biến) | Đỏ | `test_SystemToolsVanDiQuaGateway` |

## 4. Phát hiện mới khi chạy thật (ngoài phạm vi, cần bugfix riêng)

| # | Mức | Phát hiện |
|---|---|---|
| F1 | Major | Một lần `POST /api/v1/events/{id}/submit` trả **500**: `pq: bind message has 15 result formats but query has 8 columns (08P01)`. Event service dùng `lib/pq` qua PgBouncer `POOL_MODE: transaction`; câu lệnh chuẩn bị (unnamed prepared statement) của hai kết nối khách lẫn nhau trên cùng kết nối server khi relay outbox chạy song song. Lỗi ngắt quãng, sự kiện ở lại `DRAFT` |
| F2 | Minor | `SystemTools` của trợ lý gọi `GET /v1/events/{id}` — gateway chỉ có `GET /api/v1/events/{id}` (404 trong log gateway) — xem spec §4 |
| F3 | Ghi nhận | 9 job trong `ai.dlq` từ trước bản sửa cần replay theo runbook DLQ (EV-159) |
