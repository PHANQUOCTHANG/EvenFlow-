# EVF-7 — Baseline (trạng thái trước khi hiện thực)

| Mục | Giá trị |
|---|---|
| Chụp lúc | 2026-10-13, `feature/EVF-7-libs-go-mq` = `develop` @ `51c4729`, chưa commit |
| Người viết | Test designer (agent) — viết test từ `spec.md`, `plan.md` (§7 đè các mục trước), `plan-review.md`; **không** đọc/viết code hiện thực |
| Môi trường | Go 1.27.0, `GOTOOLCHAIN=local`; Docker CLI có, daemon **không chạy** → test tích hợp chỉ chạy được ở CI |

## 1. Thay đổi của bước test

| File | Nội dung |
|---|---|
| `libs/go/mq/topology_test.go` | 15 test: `AITopology` (exchanges, danh sách queue, hàng gốc không DLX/priority, hàng trễ riêng + at-least-once/reject-publish, DLQ, bindings, không binding mồ côi), Ticketing/Notification giữ nguyên, `Merge`, `Declare` với `Declarer` giả (thứ tự, đối số, lỗi bọc tên + dừng, gọi hai lần), **so khớp `deploy/rabbitmq/definitions.json`** |
| `libs/go/mq/consumer_test.go` | 10 test: `AttemptHeader`, bảng `decide`, `attemptOf`, `copyHeaders` (bỏ header broker, bản sao, nil an toàn), `ConsumerConfig.validate` (mặc định 3/10, từ chối thiếu DLX/Queue, `MaxAttempts < 1`), `Consume` từ chối cấu hình thiếu DLX trước khi dùng kênh, `callHandler` (panic → lỗi thường) |
| `libs/go/mq/trace_test.go` | 3 test: publish → consume cùng trace ID, kind Producer/Consumer, span consume là con của span publish, header người gọi không bị sửa, header nil an toàn, trace giữ qua 3 lần retry. Chép helper `newRecorder` từ `otelx/otelx_test.go:28-42` |
| `libs/go/mq/integration_test.go` | `//go:build integration`, testcontainers `modules/rabbitmq` ảnh `rabbitmq:3.13-management-alpine`, **fail** khi không có Docker. 9 test (xem `traceability.md`) |
| `libs/go/go.mod`, `libs/go/go.sum` | `go get github.com/rabbitmq/amqp091-go@v1.15.0`, `github.com/testcontainers/testcontainers-go@v0.44.0`, `.../modules/rabbitmq@v0.44.0` + `go mod tidy` trong `libs/go`. Dòng `go 1.25.0` giữ nguyên, **không** có dòng `toolchain`. **Không** chạy `go work sync`. go.mod +42 dòng, go.sum +97 dòng (khớp số liệu thực nghiệm của plan review) |

Không có file Go không phải test nào trong `libs/go/mq`.

## 2. Kết quả chạy hiện tại (trong `libs/go`)

| Lệnh | Kết quả | Lý do |
|---|---|---|
| `go vet ./...` | **FAIL** (biên dịch) | `mq`: `undefined: Topology` … — package `mq` chưa có mã nguồn |
| `go vet -tags=integration ./...` | **FAIL** (biên dịch) | như trên, thêm các tên của publisher/consumer |
| `go test ./...` | `httpx` ok, `otelx` ok, `redisx` không có test, `mq` **build failed** | như trên |
| `go test ./httpx/... ./otelx/... ./redisx/...` | **PASS** | không hồi quy ở package có sẵn |

Biên dịch với `-gcflags=-e` cho thấy **mọi** lỗi đều là `undefined: <tên>` (danh sách §3) — không có lỗi kiểu nào khác
trong file test.

### Kiểm tra chéo bằng bản hiện thực tham chiếu (ngoài worktree)

Để chắc test không sai/không quá chặt, đã chép `libs/go` + `deploy/rabbitmq/definitions.json` sang scratchpad và viết
một bản hiện thực tối thiểu đúng hợp đồng (Publisher/Consume chỉ là stub). Kết quả (`GOWORK=off`):

- `go vet ./mq` và `go vet -tags=integration ./mq`: sạch (file tích hợp biên dịch đúng với hợp đồng).
- `go test ./mq`: **mọi test unit pass, trừ `TestDefinitionsJSON_KhopTopologyCode`** — đúng kỳ vọng.
- Sau khi sửa `definitions.json` (bản sao) theo spec §2 + M3: `go test -race ./mq` **PASS** toàn bộ (61 test/subtest).

Bản tham chiếu **không** được đưa vào worktree; implementer tự viết.

### Test đồng bộ `definitions.json` hiện **phải fail** (AC-5)

Với `definitions.json` hiện tại, `TestDefinitionsJSON_KhopTopologyCode` báo (trích):

```
queue ai.chat.q lech:
 file durable=true args=map[x-dead-letter-exchange:ai.retry x-dead-letter-routing-key:delay.5s x-max-priority:5 x-message-ttl:60000 x-queue-type:quorum]
 code durable=true args=map[x-message-ttl:60000 x-queue-type:quorum]
queue ai.chat.retry co trong code nhung thieu trong definitions.json
queue ai.delay.5s / ai.delay.30s / ai.delay.2m co trong definitions.json nhung khong co trong code
queue ai.moderation.q / ai.anomaly.q / ai.report.q lech (x-dead-letter-* thua)
binding ai.dlx -> ai.dlq [#] co trong definitions.json nhung khong co trong code
binding ai.dlx -> ai.dlq [chat|moderation|anomaly|report] co trong code nhung thieu trong definitions.json
binding ai.retry -> ai.delay.* [delay.*] thua; ai.retry -> ai.*.retry [<job>] thieu
```

Tức là bắt đúng T1–T6. Implementer phải sửa `definitions.json` theo spec §2 + plan §7 M3 (và `_comment`, M6) để test xanh.
Exchanges và phần `ticketing.*`/`notification.*` đã khớp.

## 3. Ký hiệu implementer phải cung cấp (package `mq`)

Ký hiệu export — theo plan §1/§7, test dựa đúng các tên này:

```go
// topology.go
type Exchange struct{ Name, Kind string; Durable bool }
type Queue struct{ Name string; Durable bool; Args map[string]any }      // so: int64 (test chuan hoa moi kieu so nguyen)
type Binding struct{ Source, Destination, RoutingKey string }            // phai so sanh duoc bang ==
type Topology struct{ Exchanges []Exchange; Queues []Queue; Bindings []Binding }
func AITopology() Topology
func TicketingTopology() Topology
func NotificationTopology() Topology
func Merge(ts ...Topology) Topology                                       // noi theo thu tu, khong loai trung
type Declarer interface {                                                 // *amqp091.Channel phai thoa
    ExchangeDeclare(name, kind string, durable, autoDelete, internal, noWait bool, args amqp091.Table) error
    QueueDeclare(name string, durable, autoDelete, exclusive, noWait bool, args amqp091.Table) (amqp091.Queue, error)
    QueueBind(name, key, exchange string, noWait bool, args amqp091.Table) error
}
func Declare(ch Declarer, t Topology) error
// Declare: exchanges -> queues -> bindings theo dung thu tu trong Topology; autoDelete/internal/exclusive/noWait = false;
// QueueDeclare nhan Args cua Queue; QueueBind(Destination, RoutingKey, Source, false, nil);
// loi: boc %w va chua ten doi tuong (exchange/queue; binding chua ten queue dich hoac exchange nguon), dung ngay.

// publisher.go
type Publisher struct{ /* ... */ }
func NewPublisher(ch *amqp091.Channel) (*Publisher, error)
type Message struct{ Body []byte; ContentType string; Headers map[string]any; MessageID string }
func (p *Publisher) Publish(ctx context.Context, exchange, routingKey string, msg Message) error
var ErrNacked = errors.New(...)       // plan §1 (test hien khong ep duoc nack that)
var ErrUnroutable = errors.New(...)   // M2: mandatory=true + NotifyReturn

// consumer.go
type Handler func(ctx context.Context, d Delivery) error
type Delivery struct{ Body []byte; Headers map[string]any; RoutingKey, MessageID string; Attempt int }
var ErrPermanent = errors.New(...)
type ConsumerConfig struct {
    Queue string
    MaxAttempts int
    RetryExchange, RetryKey string
    DeadLetterExchange, DeadLetterKey string
    Prefetch int
}
const AttemptHeader = "x-ef-attempt"
func Consume(ctx context.Context, ch *amqp091.Channel, pub *Publisher, cfg ConsumerConfig, h Handler) error
```

Ký hiệu **không export** test giả định (đặt tên theo plan khi plan có tên; còn lại do test designer đặt):

```go
type Action int
const ( ActionAck Action = iota; ActionRetry; ActionDeadLetter )          // plan §1
func decide(handlerErr error, attempt, maxAttempts int, canRetry bool) Action   // plan §1; attempt >= max -> DeadLetter (m4)
func attemptOf(headers amqp091.Table) int
    // doc AttemptHeader: int/int8/int16/int32/int64 va chuoi so thap phan; thieu, hong, kieu la, <1 -> 1
func copyHeaders(in amqp091.Table) amqp091.Table
    // ban sao MOI, khong bao gio nil; bo x-death, x-first-death-*, x-last-death-*, x-delivery-count (m3)
func (c ConsumerConfig) validate() (ConsumerConfig, error)
    // Queue rong hoac DeadLetterExchange rong -> loi (M5); MaxAttempts < 0 -> loi; 0 -> 3; Prefetch 0 -> 10
func callHandler(ctx context.Context, h Handler, d Delivery) error
    // goi handler, recover panic thanh loi thuong (khong boc ErrPermanent) (m5)
func startPublishSpan(ctx context.Context, exchange, routingKey string, headers amqp091.Table) (context.Context, trace.Span, amqp091.Table)
    // span SpanKindProducer ten "mq.publish <exchange>/<routingKey>"; tra BAN SAO headers (nil an toan)
    // da otelx.InjectAMQP SAU khi mo span (m3, m6). Publish phai dung ham nay.
func startConsumeSpan(ctx context.Context, queue string, headers amqp091.Table) (context.Context, trace.Span)
    // otelx.ExtractAMQP roi mo span SpanKindConsumer ten "mq.consume <queue>". Consume phai dung ham nay.
```

Ghi chú tương thích: `attemptOf`/`copyHeaders`/`startPublishSpan`/`startConsumeSpan` có thể nhận/trả `map[string]any`
thay vì `amqp091.Table` — test vẫn biên dịch (hai kiểu gán được cho nhau). `MaxAttempts = 0` hiểu là "dùng mặc định 3";
chỉ giá trị âm bị từ chối. `TestConsume_TuChoiCauHinhThieuDLX` gọi `Consume(ctx, nil, nil, cfg, h)` — `Consume` phải
validate **trước** khi chạm vào kênh/publisher.

## 4. Hành vi tích hợp test giả định (ngoài AC chữ)

- `Consume` trả `context.Canceled` (bọc được, kiểm bằng `errors.Is`) khi ctx bị huỷ.
- Publish retry/DLQ lỗi → `Nack(requeue=true)` rồi `Consume` trả lỗi khác `context.Canceled` (M4).
- Bản ở DLQ giữ `ContentType`, body, `x-ef-attempt` = lần cuối (3 khi lỗi 3 lần, 1 khi `ErrPermanent`).
- `Publish` đặt `DeliveryMode = Persistent`, giữ `ContentType`, `MessageID`, header của người gọi, thêm `traceparent`.
- Sau một lần `ErrUnroutable`, `Publisher` vẫn dùng tiếp được.
- Publish tới exchange không tồn tại trả lỗi trước khi ctx (10 s) hết hạn (kênh bị broker đóng → không treo, m1).

## 5. Vòng 2 — test bổ sung sau code review (plan §8)

Chỉ sửa file test. Seam/hàm **không export** mới mà test giả định (implementer phải thêm đúng tên, chữ ký):

```go
// publisher.go — seam de unit test Publisher bang kenh gia (B1, M1, m5)
type confirmation interface {
    WaitContext(ctx context.Context) (bool, error)          // *amqp091.DeferredConfirmation thoa
}
type publishChannel interface {
    IsClosed() bool
    publish(ctx context.Context, exchange, key string, mandatory bool, msg amqp091.Publishing) (confirmation, error)
}
func newPublisherWith(ch publishChannel, returns <-chan amqp091.Return) *Publisher
// NewPublisher: Confirm(false) + NotifyReturn roi goi newPublisherWith(adapter{ch}, returns); adapter.publish goi
// PublishWithDeferredConfirmWithContext(ctx, exchange, key, mandatory, false, msg). Truong Publisher.ch doi sang publishChannel.

// consumer.go (m4, m3)
func newDelivery(d amqp091.Delivery) Delivery               // Delivery cho handler; Headers la BAN SAO; Attempt = attemptOf
func forwardMessage(d amqp091.Delivery, attempt int) Message // ban retry/DLQ: copyHeaders + AttemptHeader=attempt + giu property
```

`handleDelivery`/`forward` nên dùng hai hàm này (test chỉ phủ hàm, không phủ chỗ gọi).

Kiểm tra trên bản sao trong scratchpad (mã đang sửa của implementer + seam tham chiếu tối thiểu): `go vet` cả hai tag
sạch, `go test -race ./mq` PASS. Đột biến về hành vi cũ (đọc `returns` không kiểm `ok`, so return không theo id) làm
`TestPublisher_KenhDongKhiChoConfirmKhongQuayVong`, `TestPublisher_ReturnsDaDongTruocKhiPublishKhongQuayVong` (timeout
2 s) và `TestPublisher_ReturnCuKhacIDBiBoQua` fail — test bắt đúng B1 và M1. golangci-lint `--build-tags integration` trên
bản sao: không có lỗi nào trong file test.
