# EVF-7 — `libs/go/mq`: publisher confirm, consumer retry/DLQ, topology bằng code (Spec)

| Mục | Giá trị |
|---|---|
| Jira | EV-107 (mã backlog **EVF-7**, issue 1007) — 5 SP — phụ thuộc EVF-3 (`libs/go/otelx`, đã có) |
| Branch | `feature/EVF-7-libs-go-mq` (từ `develop` @ `51c4729`) |
| Tier | **Standard** (10 bước). Đụng CI: thêm bước test tích hợp cho `libs/go` trong `integration.yml` |
| Quyết định người dùng (2026-10-13) | **Code là nguồn chuẩn** của topology; sửa `deploy/rabbitmq/definitions.json` cho khớp. Go cài trên máy: 1.27.0 (`GOTOOLCHAIN=local`), CI dùng 1.26 |

## 1. Bối cảnh

Jira AC: (1) message lỗi 3 lần đi vào DLQ, có test; (2) trace context được giữ qua publish/consume.
Phạm vi: publisher có publisher-confirm; consumer retry giới hạn số lần, quá thì DLQ; khai báo topology bằng code, khớp
`definitions.json`. `docs/09:242` để ngỏ việc chọn nguồn chuẩn — slice này chốt: **code**.

### Lỗi trong `definitions.json` hiện tại (khảo sát `develop` @ `51c4729`)

| # | Lỗi | Hậu quả |
|---|---|---|
| T1 | `ai.delay.*` dead-letter về `ai.direct` **không** đặt routing key; message mang key `delay.Xs`, không binding nào khớp | Message retry bị **mất**, không bao giờ quay lại hàng gốc |
| T2 | `ai.delay.30s` dùng chung cho `ai.moderation.q` và `ai.anomaly.q` | Kể cả sửa T1 bằng một key cố định, chỉ trả về được **một** trong hai hàng |
| T3 | `ai.dlx` là `direct` nhưng `ai.dlq` bind bằng `#` | `#` chỉ là ký tự wildcard ở `topic`; với `direct` không message nào tới DLQ |
| T4 | Không gì dead-letter vào `ai.dlx` | DLQ không bao giờ nhận message |
| T5 | `x-max-priority` trên quorum queue `ai.chat.q` | Quorum queue không hỗ trợ priority — khai báo bị broker từ chối |
| T6 | Hàng gốc có TTL dead-letter vào `ai.retry` | Câu trả lời chat hết hạn (60s, "cố ý vô giá trị") lại bị **retry** |
| T7 | Compose mount `definitions.json` nhưng không có `load_definitions` | File không được nạp — ghi nợ, ngoài phạm vi |

## 2. Thiết kế topology mới (code là chuẩn)

- Retry do **consumer** điều khiển (không dựa vào TTL của hàng gốc): lần xử lý lỗi thứ `n < MaxAttempts` → publish bản sao
  (giữ body + header, tăng `x-ef-attempt`) sang exchange retry với key riêng của hàng, rồi `ack` bản gốc. Lần thứ
  `MaxAttempts` (mặc định **3**) hoặc lỗi vĩnh viễn → publish sang exchange DLX, rồi `ack`.
- Mỗi hàng gốc AI có **một hàng chờ trễ riêng**, dead-letter về `ai.direct` với `x-dead-letter-routing-key` = key của chính
  nó (sửa T1, T2): `ai.chat.retry` (5s → `chat`), `ai.moderation.retry` (30s → `moderation`), `ai.anomaly.retry`
  (30s → `anomaly`), `ai.report.retry` (2m → `report`); bind từ `ai.retry` bằng key = tên công việc.
- `ai.dlx` giữ `direct`, `ai.dlq` bind bằng **từng** key `chat`, `moderation`, `anomaly`, `report` (sửa T3, T4).
- Hàng gốc AI bỏ `x-dead-letter-*` (sửa T6: hết TTL thì bỏ); bỏ `x-max-priority` (T5).
- `ticketing.*`, `notification.*` giữ nguyên (không retry trong slice này).
- Header thuộc thư viện: `x-ef-attempt` (int, bắt đầu 1). Trace context theo `otelx.InjectAMQP`/`ExtractAMQP`.

## 3. Tiêu chí nghiệm thu

**AC-1 — Publisher confirm.** `Publisher.Publish` chỉ trả `nil` khi broker **ack**; nack hoặc kênh đóng → lỗi; tôn trọng
`ctx` (huỷ → lỗi, không treo).

**AC-2 — Lỗi 3 lần vào DLQ.** Với `MaxAttempts = 3`, handler luôn lỗi: message được xử lý **đúng 3 lần** rồi nằm ở DLQ
với `x-ef-attempt = 3`, body nguyên vẹn; hàng gốc và hàng trễ rỗng. Có **test tích hợp với RabbitMQ thật**
(testcontainers, `//go:build integration`) và unit test cho logic quyết định.

**AC-3 — Thành công thì ack, lỗi vĩnh viễn đi thẳng DLQ.** Handler trả `nil` → ack, không retry. Lỗi bọc
`mq.ErrPermanent` → DLQ ngay ở lần đầu.

**AC-4 — Trace xuyên suốt.** Span của handler có **cùng trace ID** với span tạo lúc publish, kể cả sau khi đi qua hàng chờ
trễ (lần thứ 2, 3). Có test (unit với recorder của otelx; tích hợp qua broker thật).

**AC-5 — Topology bằng code khớp file.** `mq.Declare` khai báo đúng exchanges/queues/bindings và arguments; chạy hai lần
không lỗi (idempotent). Một unit test đọc `deploy/rabbitmq/definitions.json` và so với topology trong code — lệch là
fail (kể cả arguments). `definitions.json` được sửa theo §2.

**AC-6 — Không mất message khi publish retry/DLQ lỗi.** Nếu publish sang retry/DLX thất bại, bản gốc được `nack` với
requeue (không ack) để không mất.

**AC-7 — Chất lượng.** `go vet`, `golangci-lint` (cấu hình repo), `go test ./... -race` trong `libs/go` pass;
`go.mod`/`go.sum` của `libs/go` cập nhật, `go work sync` không làm bẩn module khác. CI: `test-go`, `lint-go` xanh;
`integration.yml` chạy test tích hợp của `libs/go/mq`.

## 4. Ngoài phạm vi

- Nối `mq` vào ticketing outbox (EVF-36), notification, ai-worker Python (aio-pika) — ghi rõ topology mới cho EVF-80/82.
- Phân loại lỗi tạm thời/vĩnh viễn theo nghiệp vụ, alert DLQ (EVF-82, EVF-125).
- Bật `load_definitions` trong compose (T7), user/permission RabbitMQ.
- Reconnect tự động khi mất kết nối (ghi nợ; consumer trả lỗi để process supervisor khởi động lại).
