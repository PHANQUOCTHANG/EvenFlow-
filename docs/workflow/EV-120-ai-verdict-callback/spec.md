# EV-120 (bugfix) — Kết quả kiểm duyệt AI không bao giờ về tới event service (Spec)

| Mục | Giá trị |
|---|---|
| Jira | EV-120 (mã backlog **EVF-23**) — "Đẩy job kiểm duyệt AI khi submit, nhận kết quả bất đồng bộ": chặng **nhận kết quả** |
| Branch | `bugfix/EV-120-ai-verdict-callback` từ `develop` |
| Phát hiện bởi | Chạy thật đầu-cuối qua gateway của EV-121 (JWT thật từ identity, toàn bộ stack compose) |
| Quy tắc | BR-E4 (kiểm duyệt AI), gateway là cổng ra ngoài duy nhất và chặn `/internal/` ([routes.go](../../../services/gateway/internal/proxy/routes.go)), cấu hình kiểm lúc khởi động |
| Tier | **Major**: mọi sự kiện nộp duyệt trên compose kẹt ở `PENDING_REVIEW` |

## 1. Hiện trạng (đã kiểm trên `develop` @ `5eeab92`, stack compose đầy đủ)

| # | Hiện trạng | Hệ quả |
|---|---|---|
| C1 | ai-worker gửi `PATCH {INTERNAL_API_BASE}/internal/v1/events/{id}/ai-verdict` với `INTERNAL_API_BASE = http://gateway:8080` (mặc định trong `config.py` và compose, từ EV-157) | Gateway chặn `/internal/` theo thiết kế → **404** |
| C2 | Runtime coi 4xx là lỗi vĩnh viễn → job vào `ai.dlq` ngay lần đầu | Sự kiện **kẹt mãi ở `PENDING_REVIEW`**, không vào hàng đợi Moderator (EV-121), không tự lên lịch; chỉ thấy khi đọc log ai-worker hoặc độ sâu `ai.dlq` |
| C3 | Cùng một biến `internal_api_base` dùng cho hai việc khác nhau: callback nội bộ của event service và `SystemTools` của chatbot (gọi API công khai của waitingroom, ticketing, event — phải qua gateway) | Không thể sửa bằng cách đổi giá trị chung: đổi sang `event:8084` thì công cụ chatbot gọi sai service |

Bằng chứng chạy thật (trước khi sửa): 3 sự kiện nộp duyệt, AI mock trả đúng `NEEDS_HUMAN` / `AUTO_REJECTED` /
`AUTO_APPROVED`, cả 3 callback `404 Not Found` → `chuyen vao DLQ` (`permanent`), cả 3 vẫn `PENDING_REVIEW`
sau 15 giây; hàng đợi Moderator rỗng.

## 2. Tiêu chí nghiệm thu

| AC | Nội dung |
|---|---|
| AC-1 | Callback kết quả kiểm duyệt gửi thẳng tới event service qua cấu hình riêng `EVENT_API_BASE` (mặc định `http://event:8084`), không qua gateway |
| AC-2 | `SystemTools` của chatbot vẫn dùng `INTERNAL_API_BASE` (gateway) — không đổi hành vi |
| AC-3 | Cấu hình hỏng thì không khởi động: `EVENT_API_BASE` phải là URL `http(s)` có host |
| AC-4 | Compose: `EVENT_API_BASE` của ai-worker trỏ tới service `event` đúng cổng `HTTP_ADDR` của nó — có test đọc file compose để không lệch lại |
| AC-5 | Chạy thật trên compose: sự kiện nộp duyệt được AI quyết trong vài giây (`SCHEDULED` / `REJECTED` / còn `PENDING_REVIEW` với review `NEEDS_HUMAN`), hàng đợi Moderator có hồ sơ, `ai.dlq` không tăng |

## 3. Thiết kế

- `app/config.py`: thêm `event_api_base: str = "http://event:8084"` cùng validator URL (AC-3). Giữ
  `internal_api_base` cho `SystemTools` (AC-2).
- `app/main.py`: tách `build_moderation_consumer(settings, gemini, limiter)` để lắp consumer bằng
  `event_api_base` — test được mà không cần dựng lifespan.
- `deploy/compose/docker-compose.yml`: `EVENT_API_BASE: "http://event:8084"`, chú thích lý do không đi qua gateway.
- Không đổi gateway (chặn `/internal/` là đúng), không đổi event service, không migration.

## 4. Ngoài phạm vi

- `SystemTools.event_info` / `event_detail` gọi `GET /v1/events/{id}` nhưng gateway chỉ có
  `GET /api/v1/events/{id}` → công cụ chatbot luôn trả "không lấy được thông tin". Lỗi riêng của EV-171 (trợ lý),
  cần bugfix riêng.
- Job đã vào `ai.dlq` trước bản sửa không tự chạy lại: replay theo runbook DLQ (EV-159).
