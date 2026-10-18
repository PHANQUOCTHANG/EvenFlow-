# EV-157 — Khung ai-worker: 4 consumer RabbitMQ tách biệt (Spec)

| Mục | Giá trị |
|---|---|
| Jira | EV-157 (mã backlog **EVF-80**) — 8 SP — P0 — Sprint 3 |
| Kế hoạch tổng | [09-ke-hoach-hoan-thien-backend.md](../../09-ke-hoach-hoan-thien-backend.md) — Giai đoạn 7, việc "Khởi động consumer" và "Gemini thật" |
| Branch | `feature/EV-157-ai-worker-consumers` (từ `develop` @ `c241ea1`) |
| Quy tắc nghiệp vụ | BR-A1, BR-A3, BR-A5, BR-A7 — [01-nghiep-vu.md](../../01-nghiep-vu.md) |
| Kiến trúc | [02-kien-truc.md](../../02-kien-truc.md) §AI, lớp L5–L7; giao thức retry dùng chung với `libs/go/mq` (EVF-7) |
| Tier | Standard (không chạm tồn kho / tiền / danh tính) |

## 1. Bài toán

Phòng chờ có thể có 30.000 người hỏi trợ lý cùng lúc, trong khi cùng một worker còn phải chạy kiểm duyệt sự kiện,
phát hiện bất thường và báo cáo sau bán. Một job báo cáo chạy 2 phút **không được** làm khách đang chờ phải đợi thêm
2 phút cho câu trả lời (BR-A3). Vì vậy mỗi loại việc có hàng đợi riêng và **pool consumer riêng**: chat bị nghẽn chỉ
vì report là lỗi thiết kế, không phải lỗi tải.

## 2. Hiện trạng (đã kiểm trên `develop` @ `c241ea1`)

| # | Lỗ hổng | Hệ quả |
|---|---|---|
| H1 | `app/main.py` không khởi động consumer nào (`TODO(EVF-80)`), `_state["consumers"] = []` | Mọi job AI nằm yên trong hàng đợi; trợ lý và kiểm duyệt AI chưa bao giờ chạy |
| H2 | Không ai khai báo topology AI lúc chạy: `definitions.json` chỉ được mount, RabbitMQ không nạp (EVF-7 plan-review T7); chỉ ticketing gọi `mq.Declare` cho topology của nó | Từ volume trống, publish vào `ai.direct` không có queue nào nhận → message mất |
| H3 | `GeminiClient.generate` nhánh thật là `NotImplementedError` | Tắt `GEMINI_MOCK` là worker hỏng |
| H4 | Không có embedder → không dựng được `SemanticCache` → không dựng được `ChatConsumer` | Pipeline chat đã viết nhưng không lắp được |
| H5 | `ChatConsumer` nhận `publish_result` nhưng chưa có nơi nhận kết quả | EV-171 (API trợ lý, SSE) không có hợp đồng để đọc |
| H6 | Gauge `ai_queue_lag_seconds` khai báo nhưng không bao giờ được ghi | AC Jira thứ 2 không đo được; suy biến (EV-164) không có tín hiệu |
| H7 | Consumer chưa theo giao thức retry/DLQ của `libs/go/mq` | Lỗi sẽ nack-requeue vòng nóng, hoặc mất message |
| H8 | Log dạng text, không có `trace_id` / `event_id` | Trái quy tắc bất biến 4 ([07 §5](../../07-Quy_trinh_lam_viec_nhom.md)) |
| H9 | `httpx` được import trực tiếp (`consumers/moderation.py`, `pipeline/tools.py`) nhưng không khai trong `pyproject.toml` (chỉ có qua `google-genai`) | Đổi phiên bản `google-genai` có thể làm worker vỡ import |

Phát hiện **ngoài phạm vi** (ghi lại để mở issue riêng, không sửa trong slice này):

| # | Phát hiện | Đề xuất |
|---|---|---|
| O1 | `services/event/internal/app/enqueue_moderation.go` ghi outbox với routing key `ai.moderation.q`, nhưng binding của `ai.direct` là `moderation` | **Đã sửa** ở `bugfix/EV-120-moderation-outbox-relay` — xem [EV-120-moderation-outbox-relay](../EV-120-moderation-outbox-relay/spec.md) |
| O2 | Event service chưa có outbox relay (`cmd/server/main.go` chỉ tạo `OutboxRepository`) | **Đã sửa** cùng nhánh trên; kèm O4 (relay ticketing lấy cả dòng của service khác) và O5 (ghi outbox ngoài transaction, nuốt lỗi) |
| O3 | `GeminiClient.scan_moderation` nhánh thật gọi `generate` không qua limiter | **Đã kéo vào slice** (2026-10-18): slice này bật nhánh Gemini thật (AC-7), để lỗ này lại là phá bất biến "không bao giờ chạm trần". Xem AC-12 |

## 3. Tiêu chí nghiệm thu

| AC | Nội dung |
|---|---|
| AC-1 | *(Jira)* Job report nặng không làm tăng lag của `ai.chat.q` quá 5 giây |
| AC-2 | *(Jira)* Metric `ai_queue_lag_seconds{queue}` cho từng queue trong 4 queue |
| AC-3 | Mỗi queue có pool riêng: channel riêng, `prefetch = số consumer`, số handler chạy song song đúng cấu hình; một handler treo hoặc lỗi không chặn pool khác |
| AC-4 | Khởi động khai báo topology AI từ `deploy/rabbitmq/definitions.json` (bản sao đã được test so khớp với `libs/go/mq.AITopology`), idempotent; topology lệch tham số → worker **không khởi động** |
| AC-5 | Giao thức retry giống `libs/go/mq`: thành công → ack; lỗi tạm và `attempt < 3` → bản sao vào `ai.retry` key = tên việc, `x-ef-attempt = attempt + 1` (int32), giữ body và property, bỏ header của broker; lỗi vĩnh viễn hoặc hết lượt → `ai.dlx`; publish chuyển tiếp lỗi → nack requeue; đang tắt → nack requeue |
| AC-6 | Kết quả chat ghi vào Redis `ai:job:{<job_id>}:result` (TTL 300 giây) và `PUBLISH ai:job:{<job_id>}` — hợp đồng cho EV-171 |
| AC-7 | Gemini thật qua `google-genai`: timeout cứng, trả số token thật để hoàn permit; 429 → tăng `ai_gemini_429_total` và đi retry (không trả lỗi cho khách); embedder cho cache (mock tất định khi `GEMINI_MOCK=1`) |
| AC-8 | Tắt êm: ngừng nhận message mới, chờ handler đang chạy tối đa `SHUTDOWN_GRACE_SECONDS`; message chưa ack quay lại hàng |
| AC-9 | `anomaly` và `report` chưa có handler (EV-167, EV-168): message vào DLQ với `x-ef-error`, không mất, không vòng nóng |
| AC-10 | Cấu hình validate lúc khởi động: `CHAT_CONSUMERS ≥ 1`, `MODERATION_CONSUMERS ≥ 1`, `ANOMALY/REPORT_CONSUMERS ≥ 0`, timeout > 0; sai thì dừng |
| AC-11 | Log JSON (kể cả log của uvicorn từ dòng đầu tiên), mỗi dòng xử lý message có `queue`, `job_id`, `event_id`, `trace_id` (lấy từ header `traceparent` do `otelx` gắn), không log nội dung câu hỏi |
| AC-12 | Mọi lần gọi Gemini thật — chat **và kiểm duyệt** — đi qua một đường duy nhất `app.gemini.guarded`: xin phép limiter → gọi → hoàn phần thừa. Kiểm duyệt bị limiter từ chối / 429 → thử lại; lần cuối vẫn không gọi được → verdict `NEEDS_HUMAN` cho Moderator, sự kiện không bị kẹt ở `PENDING_REVIEW` |

## 4. Thiết kế

### 4.1. Bố cục

Giữ đúng khung [03 §5](../../03-cau-truc-src.md), thêm một package hạ tầng:

```
app/
├── config.py                  # + validate, prefetch, timeout, grace
├── logging.py                 # JSON formatter + trace_id tu traceparent
├── main.py                    # lifespan: Redis -> topology -> runtime 4 pool
├── mq/
│   ├── topology.py            # doc definitions.json, khai bao idempotent
│   ├── protocol.py            # THUAN: attempt_of, decide, forward_headers (khop libs/go/mq)
│   └── runtime.py             # QueuePool / ConsumerRuntime tren aio-pika
├── consumers/
│   ├── chat.py                # + adapter handler(message) -> ChatConsumer.handle
│   ├── moderation.py
│   └── unsupported.py         # anomaly/report -> PermanentError
├── gemini/client.py           # + nhanh that, embed, Rate429
└── pipeline/results.py        # RedisResultSink (AC-6)
```

`mq/protocol.py` không import aio-pika: test được không cần broker, giống `decide`/`attemptOf` của Go.

### 4.2. Pool tách biệt (AC-1, AC-3)

| Cách | Chọn? | Vì sao |
|---|---|---|
| Một prefetch chung cho mọi queue (`basic.qos global`) | ✗ | 1 report chiếm slot → chat chờ; đúng lỗi BR-A3 cấm. Thực tế quorum queue của RabbitMQ 3.13 **từ chối** global qos (`540 NOT_IMPLEMENTED`, xem `challenge.md` M18) |
| Một process mỗi queue | ✗ | Ops mất chỗ điều khiển suy biến chung, metric phân tán (lý do đã ghi đầu `main.py`) |
| **Mỗi queue một channel + consumer `prefetch = N` + N task, chung event loop** | ✔ | RabbitMQ áp `basic.qos` (global = false) cho **từng consumer**, nên report không thể lấy slot của chat. Channel riêng thêm cách ly lỗi: publish chuyển tiếp hỏng làm đóng channel thì chỉ pool đó bị ảnh hưởng |

*Sửa sau thử phá:* bản đầu ghi "broker giới hạn theo từng channel". Đúng ra giới hạn theo từng consumer; dùng chung
channel vẫn cách ly được (đột biến M17 sống sót vì lý do này). Giữ channel riêng vì cách ly lỗi, không phải vì prefetch.

Hệ quả bắt buộc: handler **phải async và không chặn event loop**. Việc nặng CPU (tổng hợp số liệu báo cáo) chạy qua
`asyncio.to_thread`. Test AC-1 có cả trường hợp report chặn đồng bộ qua `to_thread` để chứng minh chat không bị ảnh hưởng;
gọi `time.sleep` trực tiếp trong handler thì test đỏ (đột biến M19) — đây là rủi ro thật còn lại của AC-1.

Mỗi pool: `QueuePool(queue, key, concurrency, handler)` mở channel `publisher_confirms=True, on_return_raises=True`,
`set_qos(prefetch_count=concurrency)`, `queue.consume(cb)`; `cb` đưa message vào `asyncio.Queue(maxsize=concurrency)`,
N task worker lấy ra xử lý. Pool có `concurrency = 0` thì không mở.

### 4.3. Giao thức retry (AC-5, AC-9)

Hàm thuần khớp `libs/go/mq/consumer.go`:

| Python | Go | Hành vi |
|---|---|---|
| `attempt_of(headers)` | `attemptOf` | Thiếu, hỏng, < 1 hoặc > int32 → 1 |
| `decide(err, attempt, max=3)` | `decide` | `None` → ACK; `PermanentError` hoặc `attempt ≥ max` → DEAD_LETTER; còn lại → RETRY |
| `forward_headers(headers, attempt)` | `copyHeaders` + `forwardMessage` | Bỏ `x-death`, `x-delivery-count`, `x-first-death-*`, `x-last-death-*`, `x-ef-publish-id`; đặt `x-ef-attempt` |

`Retry(delay_ms)` của `ChatConsumer` (limiter chưa cấp phép) được coi là lỗi tạm → đi `ai.chat.retry` (5 giây cố định
theo topology). Không ngủ trong handler. Phân loại lỗi chi tiết theo từng handler và alert khi DLQ có message thuộc
**EV-159**; slice này chỉ dựng đường đi của message.

### 4.4. Khai báo topology (AC-4)

Nguồn chuẩn vẫn là `libs/go/mq.AITopology()`; Python đọc `definitions.json` (đã có test Go so khớp từng tham số) và
chỉ lấy phần `ai.*`. Khai báo cùng tham số là idempotent; khác tham số thì RabbitMQ trả `PRECONDITION_FAILED` → worker
dừng ngay, đúng nguyên tắc fail fast. Đường dẫn file qua `AI_TOPOLOGY_FILE` (Dockerfile copy file vào image).

### 4.5. Lag (AC-2)

`ai_queue_lag_seconds{queue}` = tuổi của message vừa nhận: `now − timestamp` (property AMQP), không có thì dùng
`enqueued_at` trong body. Một task lấy mẫu mỗi 5 giây khai báo passive từng queue: `message_count == 0` và không
message nào đang xử lý → đặt lag = 0 (nếu không, gauge đứng ở giá trị cũ mãi). Thêm `ai_queue_depth{queue}`,
`ai_jobs_total{queue,outcome}` (`ack`, `retry`, `dead_letter`, `expired`, `requeue`) và
`ai_job_duration_seconds{queue}`.

### 4.6. Kết quả chat (AC-6)

`RedisResultSink.publish(job_id, payload)`: `SET ai:job:{<job_id>}:result <json> EX 300` rồi `PUBLISH ai:job:{<job_id>} <json>`.
SET trước để client SSE (EV-171) đăng ký muộn vẫn đọc được — cùng lý do `singleflight.wait` poll thay vì chỉ pub/sub.
Hash tag `{job_id}` theo quy ước Redis key ([03 §7](../../03-cau-truc-src.md)).

### 4.7. Gemini thật (AC-7)

- `genai.Client(api_key).aio.models.generate_content(model, contents, config=GenerateContentConfig(max_output_tokens))`
  trong `asyncio.wait_for(timeout=GEMINI_TIMEOUT_SECONDS)` (mặc định 10 giây, ngắn hơn TTL singleflight 30 giây).
- Số token thật = `usage_metadata.total_token_count`; thiếu thì coi là số đã đặt chỗ (không hoàn nhầm).
- `errors.ClientError` có `code == 429` → tăng `ai_gemini_429_total`, ném `Retry`. Đây là chỉ số phải bằng 0 ở EV-158;
  đếm nó là điều kiện để chứng minh.
- `embed(text)`: thật dùng `embed_content` (model `GEMINI_EMBED_MODEL`); mock là vector băm tất định 64 chiều trên câu
  đã `normalize`, để cache ngữ nghĩa chạy được ở local và test.
- SDK được tiêm qua constructor để unit test không gọi mạng.

### 4.8. Tắt êm (AC-8)

Huỷ consumer tag của mọi pool trước (broker ngừng giao), chờ hàng nội bộ cạn và handler xong trong
`SHUTDOWN_GRACE_SECONDS` (mặc định 20), quá hạn thì huỷ task; message chưa ack được broker trả lại khi đóng channel.
Khớp nhánh "đang tắt → nack requeue" của Go.

## 5. Ngoài phạm vi

- API trợ lý phía khách, quota 10 câu / 15 phút (EV-171); tool-call số liệu thật đầy đủ (EV-163).
- Phân loại lỗi theo handler + alert DLQ (EV-159); tín hiệu suy biến thật (EV-164).
- Handler anomaly / report (EV-167, EV-168); structured output 4 trục cho kiểm duyệt (EV-166).
- Sửa phía event service (O1, O2).
- Vector search thay quét tuyến tính (EV-161).

## 6. Kiểm chứng dự kiến

| Loại | Lệnh |
|---|---|
| Unit | `uv run pytest -q` (mặc định bỏ marker `integration`) |
| Tích hợp | `uv run pytest -m integration -q` — testcontainers RabbitMQ 3.13 + Redis 7; thêm bước vào `.github/workflows/integration.yml` |
| Lint | `uv run ruff check .` |
| Chạy thật | `docker compose up ai-worker` từ volume trống: topology xuất hiện, publish một job chat thì kết quả có trong Redis, `/metrics` có lag 4 queue |
