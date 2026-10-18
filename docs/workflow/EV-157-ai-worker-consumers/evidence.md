# EV-157 — Bằng chứng

Ngày chạy: 2026-10-17. Máy: Windows 11, Docker Desktop (engine 29.6.2), Python 3.13.15, uv 0.9.30.
Nhánh `feature/EV-157-ai-worker-consumers`, base `develop` @ `c241ea1`, thay đổi chưa commit.

**Chưa có bằng chứng CI (Linux):** nhánh chưa được đẩy. Mọi con số là chạy cục bộ.

## 1. Kết quả

| Kiểm | Kết quả |
|---|---|
| `uv sync --all-extras --locked` (sau `uv lock`: `uv.lock` +2 dòng cho `httpx`) | OK |
| `uv run ruff check .` | **All checks passed** |
| `uv run pytest -q` (unit, bỏ marker `integration`) | **113 passed** (baseline 4) — chạy lại 2026-10-18 sau B2 + log uvicorn |
| `uv run pytest -m integration -q` (RabbitMQ `3.13-management-alpine` + Redis `7-alpine` qua testcontainers) | **13 passed**, 3 lần ngày 2026-10-17 + 1 lần ngày 2026-10-18 sau B2, không chập chờn (~63 giây / lần) |
| Log trên compose sau `--log-config` | 0 dòng không phải JSON, kể cả `Started server process` của uvicorn |
| `bash .github/scripts/tests/test-workflows.sh` (sau khi thêm bước pytest vào `integration.yml`) | **137 PASS, 0 FAIL** |
| Thử phá (25 đột biến) | 24 đỏ, 1 tương đương có giải thích — `challenge.md` |

## 2. Chạy thật trên compose từ volume trống

`docker compose up -d --build redis rabbitmq ai-worker` sau khi xoá volume `rabbitdata`, `redisdata`.

| Bước | Quan sát |
|---|---|
| Khởi động | `/readyz` = `ready`; log JSON `pool san sang` cho 4 queue (chat 20, moderation 5, anomaly 1, report 1) |
| Topology | `rabbitmqctl list_queues`: đủ 9 queue `ai.*` — worker tự khai báo (trước slice: không có queue nào) |
| Chat câu FAQ (`Tôi tắt trình duyệt có mất chỗ không?`) | `ai:job:{smoke-1}:result` có `source: faq`, `latency_ms: 6`, TTL 296 giây |
| Chat câu hỏi mở | `source: fallback` sau ~2 giây: `tools.event_context` không kết nối được `gateway:8080` (gateway chưa có — giai đoạn 3). Khách vẫn nhận câu trả lời (BR-A1) |
| Job report | Vào `ai.dlq`; log `chuyen vao DLQ … cong viec 'report' chua co handler (EV-168)` |
| Body không phải JSON | Vào `ai.dlq` ngay lần 1 (`PermanentError: job chat hong: JSONDecodeError`) |
| Log | Mỗi dòng xử lý có `queue`, `job_id`, `event_id`, `trace_id = 4bf92f35…` lấy từ `traceparent`, `attempt`; không có nội dung câu hỏi |
| `/metrics` | `ai_queue_lag_seconds` và `ai_queue_depth` cho đủ 4 queue, về 0 khi hàng trống; `ai_jobs_total{outcome=ack|dead_letter}` đúng số job |
| `docker compose stop -t 30 ai-worker` | `Waiting for application shutdown` → `ai-worker dang tat` → `Application shutdown complete` |

## 3. Lỗi chỉ lộ ra khi chạy thật, đã sửa

Lần build đầu worker chết lúc import: `config.py` tính đường dẫn mặc định bằng `Path(__file__).parents[3]`; trong image
file nằm ở `/app/app/config.py` nên chỉ có 3 cấp cha → `IndexError`. Unit test chạy từ repo không thấy. Sửa bằng
`default_topology_file()` có nhánh lùi, thêm test `TopologyMacDinh_TrongImageKhongVoLucImport`.

## 4. Đánh đổi và giới hạn đã biết

- **Gemini thật chưa được gọi**: không có API key trên máy. Nhánh thật chỉ được kiểm bằng SDK giả đúng chữ ký
  google-genai 2.29.0. Cần một lần chạy với `GEMINI_MOCK=0` trước EV-158.
- **Retry của chat luôn chờ 5 giây** (TTL cố định của `ai.chat.retry`), không theo `wait_ms` của limiter. Lần thử thứ 3
  vẫn bị từ chối thì khách nhận câu trả lời dự phòng (B1 trong `code-review.md`).
- **Job kiểm duyệt chưa tới được worker trên stack thật**: event service ghi outbox với key `ai.moderation.q` (binding
  là `moderation`) và chưa có relay — O1, O2 trong spec, sửa ở nhánh bugfix riêng xếp chồng lên nhánh này.
- **Embedding không qua limiter** (quota tách riêng với generate) — đo ở EV-158.
- **Lần chạy compose đầu tiên đã dùng `docker compose down -v`**, xoá mọi volume của stack local (kể cả `pgdata`).
  Dữ liệu dev dựng lại được bằng `make migrate && make seed`.
