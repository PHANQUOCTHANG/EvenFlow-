# EV-171 — Bằng chứng

Ngày chạy: 2026-10-19. Máy: Windows 11, Docker Desktop 29.6.2, Python 3.13.15, uv 0.9.30.
Nhánh `feature/EV-171-assistant-api` từ `develop` @ `e1c911d`. Review và thử phá do người hiện thực tự làm — cần 1
reviewer khi mở PR.

## 1. Kết quả

| Kiểm | Kết quả |
|---|---|
| `uv run ruff check .` | All checks passed |
| `uv run pytest -q` | **169 passed** (baseline 151) |
| `uv run pytest -m integration -q` (Redis 7 + RabbitMQ 3.13) | **22 passed** (baseline 14) |
| p95 thời gian `POST` trả 202 (50 request, broker + Redis thật, ASGI trong process) | **5,8 – 12,1 ms** qua 3 lần đo (median 4,8 – 6,9 ms); ngưỡng Jira 50 ms |

## 2. Chạy thật trên compose

`docker compose up -d --build redis rabbitmq ai-worker`, gọi bằng `curl` (body UTF-8 từ file):

| Bước | Quan sát |
|---|---|
| `POST /v1/events/evt-1/assistant/messages` "Tôi tắt trình duyệt có mất chỗ không?" | 202, `job_id`, `poll_url`, `stream_url`, `quota.used = 1` |
| `GET /v1/assistant/messages/{job_id}` sau 1 giây | 200 `status: done`, câu trả lời FAQ |
| `GET …/stream` | `event: answer` + payload, đóng |
| `GET` với identity khác | 404 |
| Câu thứ 11 trong 15 phút | 200 `quota_exceeded`, `quota = {used: 10, max: 10, reset_at}` |
| Body không phải JSON | 400 `application/problem+json` |
| `/metrics` | `ai_assistant_requests_total{outcome="queued"}`, `{outcome="quota_exceeded"}` |
| Log worker | không có nội dung câu hỏi hay câu trả lời |

Lỗi chỉ lộ khi chạy thật, đã sửa: body hỏng trả `{"detail": …}` mặc định của FastAPI thay vì RFC 7807 — API giờ tự
đọc body (`test_Post_BodyHong_LoiRFC7807`). Nguyên nhân gốc lần gọi đầu là `curl` trên Git Bash/Windows làm hỏng body
tiếng Việt theo codepage của shell; gửi từ file UTF-8 thì đúng.

## 3. Thử phá

| # | Đột biến | Kết quả | Bắt bởi |
|---|---|---|---|
| F1 | Quota `>` thay `>=` (cho qua câu 11) | Đỏ | `Quota_10QuaCau11BiChan` |
| F2 | Bỏ kiểm chủ job | Đỏ | `Get_KhongPhaiChu_404` |
| F3 | SSE bỏ đọc kết quả đã có | Đỏ | `DauCuoi_PostRoiGetRoiStream` |
| F4 | Nấc OFF vẫn trừ quota | Đỏ | `Post_NacOFF_KhongXepHangKhongTruQuota` |
| F5 | Vượt quota vẫn xếp hàng | Đỏ | `Post_VuotQuota_TraFaqVaCSKH_KhongXepHang` |
| F6 | Gợi ý lộ template chưa điền `{…}` | Đỏ | `Post_VuotQuota_TraFaqVaCSKH_KhongXepHang` |
| F7 | Khoá quota không có TTL | **Sống sót lần đầu → thêm kiểm `PTTL` → Đỏ** | `Quota_10QuaCau11BiChan` |

## 4. Giới hạn

- Identity lấy từ `X-Identity-Id` cho tới khi gateway cấp JWT (giai đoạn 3) — cùng nợ với ticketing / waitingroom.
- `apps/web` vẫn mô phỏng; nối vào API thật khi gateway định tuyến được.
- Câu hỏi mở (không khớp FAQ) cần `tools.event_context` gọi gateway → hiện trả câu dự phòng (đã ghi ở EV-157).
