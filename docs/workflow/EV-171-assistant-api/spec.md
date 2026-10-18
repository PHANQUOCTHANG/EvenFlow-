# EV-171 — API trợ lý phía khách: 202 + job_id, SSE, quota 10 câu / 15 phút (Spec)

| Mục | Giá trị |
|---|---|
| Jira | EV-171 (mã backlog **EVF-94**) — 5 SP — P0 — Sprint 3 |
| Phụ thuộc | EV-157 (PR #67): consumer chat và hợp đồng kết quả `ai:job:{job_id}:result` + `PUBLISH ai:job:{job_id}` |
| Branch | `feature/EV-171-assistant-api` (từ `develop` @ `e1c911d`) |
| Quy tắc | BR-A1 (không chờ đồng bộ Gemini), BR-A2 (10 câu / 15 phút), BR-A7 (suy biến) |
| Tier | Standard |

## 1. Bài toán

Khách trong phòng chờ hỏi trợ lý. Hàng nghìn câu hỏi mỗi phút không được làm chậm hệ thống: API chỉ **nhận và xếp
hàng** (202 + `job_id`), câu trả lời tới sau qua SSE hoặc polling. Mỗi khách tối đa 10 câu / 15 phút; vượt thì nhận
câu trả lời FAQ và gợi ý CSKH chứ không phải lỗi.

## 2. Hiện trạng (đã kiểm trên `develop` @ `e1c911d`)

| # | Lỗ hổng | Hệ quả |
|---|---|---|
| H1 | Không có endpoint nào nhận câu hỏi; không ai publish vào `ai.chat.q` | Pipeline chat (EV-157) không có đầu vào |
| H2 | `apps/web/src/lib/assistant-client.ts` mô phỏng hoàn toàn: tự sinh `jobId`, stream giả, quota trong `localStorage` | Quota phía trình duyệt xoá được bằng một lệnh; bot bỏ qua hoàn toàn |
| H3 | Kết quả chat có trong Redis nhưng không ai đọc ra cho khách | — |

## 3. Tiêu chí nghiệm thu

| AC | Nội dung |
|---|---|
| AC-1 | *(Jira)* `POST` trả 202 trong < 50 ms (p95 trên broker thật), không bao giờ gọi Gemini đồng bộ |
| AC-2 | *(Jira)* Vượt 10 câu / 15 phút → 200 với câu trả lời FAQ + gợi ý CSKH, không publish job |
| AC-3 | Quota theo (sự kiện, identity), cửa sổ trượt 15 phút, nguyên tử trên Redis (nhiều replica không vượt) |
| AC-4 | `GET /v1/assistant/messages/{job_id}`: 200 + kết quả khi xong, 202 `pending` khi chưa; chỉ chủ job đọc được (người khác → 404, không lộ sự tồn tại) |
| AC-5 | `GET /v1/assistant/messages/{job_id}/stream` (SSE): gửi `event: answer` khi có kết quả rồi đóng; nhận được cả khi kết quả có **trước** lúc kết nối; quá 65 giây (TTL job chat 60 giây) → `event: timeout` kèm câu trả lời dự phòng — khách luôn nhận được câu trả lời |
| AC-6 | Nấc suy biến `OFF` → 200 `unavailable` + thông báo CSKH, không publish, không trừ quota |
| AC-7 | Thiếu identity → 401; câu hỏi rỗng / > 500 ký tự → 422 (RFC 7807) |
| AC-8 | Metric `ai_assistant_requests_total{outcome}` và `ai_assistant_accept_seconds`; log không chứa câu hỏi |

## 4. Thiết kế

- **Vị trí:** ai-worker (FastAPI sẵn có, đã có Redis + RabbitMQ). `app/api/assistant.py` là `APIRouter`; phụ thuộc
  (quota, publisher, Redis, chế độ suy biến) đọc từ `app.state` để test thay bằng fake.
- **Identity:** header `X-Identity-Id` — cùng quy ước tạm của ticketing / waitingroom cho tới khi gateway cấp JWT
  (giai đoạn 3). Nợ đã ghi ở kế hoạch tổng.
- **Quota** `app/api/quota.py` + `scripts/quota.lua`: ZSET `ai:quota:{<event_id>}:<identity>` lưu thời điểm từng câu;
  `ZREMRANGEBYSCORE` cửa sổ cũ → `ZCARD` → còn chỗ thì `ZADD` — một lệnh Lua nên hai replica không cùng lấy câu thứ 10.
  Trả `allowed`, `used`, `reset_at_ms`. Mọi câu đã nhận đều tính (BR-A2 "10 câu", không phân biệt FAQ hay câu mở).
- **Publish** `app/mq/publisher.py`: channel riêng `publisher_confirms=True`, `mandatory=True`, persistent,
  `message_id = job_id`, `timestamp` = lúc nhận (ai-worker đo lag). Body đúng `ChatJob` (`job_id`, `event_id`,
  `identity_id`, `question`, `queue_token`, `enqueued_at`). Ghi `ai:job:{<job_id>}:owner = identity` (TTL 300 s)
  **trước** khi publish để GET/SSE kiểm quyền.
- **SSE:** `SUBSCRIBE ai:job:{job_id}` trước, rồi `GET` khoá kết quả (tránh khe: kết quả tới giữa hai bước), heartbeat
  `: ping` mỗi 15 giây, hết 65 giây gửi `timeout`.

## 5. Ngoài phạm vi

- Nối `apps/web` vào API thật (thay mô phỏng) — việc của frontend sau khi gateway định tuyến được (giai đoạn 3).
- Tool-call số liệu thật (EV-163), chống injection (EV-169), tín hiệu suy biến thật (EV-164).
