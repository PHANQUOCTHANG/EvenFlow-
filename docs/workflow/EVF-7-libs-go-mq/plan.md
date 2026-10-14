# EVF-7 — Kế hoạch thực thi (Plan)

Đầu vào: `spec.md` (AC-1..AC-7). Worktree `D:/Documents/Nam4-1/Project1/EvenFlow-mq`, base `develop` @ `51c4729`.
Lệnh Go chạy với `PATH="/c/Program Files/Go/bin:$PATH" GOTOOLCHAIN=local` (máy: Go 1.27.0; **không** để dòng `go` trong
`go.mod` bị nâng — giữ `go 1.25.0` nếu dependency cho phép, ngược lại tối đa `go 1.26.0` để khớp CI/Dockerfile).
Máy có Docker CLI nhưng **daemon không chạy**: test tích hợp chỉ chạy trong CI.

## 1. Hợp đồng chốt trước — package `github.com/eventflow/eventflow/libs/go/mq`

```go
// topology.go
type Exchange struct{ Name, Kind string; Durable bool }
type Queue struct{ Name string; Durable bool; Args map[string]any }   // Args dung kieu int64 cho so (khop JSON sau khi chuan hoa)
type Binding struct{ Source, Destination, RoutingKey string }
type Topology struct{ Exchanges []Exchange; Queues []Queue; Bindings []Binding }
func AITopology() Topology          // §2 cua spec
func TicketingTopology() Topology   // ticketing.events + ticketing.hold.released.q
func NotificationTopology() Topology
func Merge(ts ...Topology) Topology
// Declarer la phan cua *amqp091.Channel ma Declare can (de unit test bang fake).
type Declarer interface {
    ExchangeDeclare(name, kind string, durable, autoDelete, internal, noWait bool, args amqp091.Table) error
    QueueDeclare(name string, durable, autoDelete, exclusive, noWait bool, args amqp091.Table) (amqp091.Queue, error)
    QueueBind(name, key, exchange string, noWait bool, args amqp091.Table) error
}
func Declare(ch Declarer, t Topology) error   // thu tu: exchanges, queues, bindings; loi boc kem ten doi tuong

// publisher.go
type Publisher struct{ /* channel o che do confirm, mutex */ }
func NewPublisher(ch *amqp091.Channel) (*Publisher, error)   // ch.Confirm(false)
type Message struct{ Body []byte; ContentType string; Headers map[string]any; MessageID string }
func (p *Publisher) Publish(ctx context.Context, exchange, routingKey string, msg Message) error
// - otelx.InjectAMQP vao headers; span "mq.publish <exchange>/<key>" bang otelx.StartSpan
// - DeliveryMode persistent; cho DeferredConfirmation.WaitContext(ctx): ack -> nil, nack -> ErrNacked
var ErrNacked = errors.New("mq: broker nack")

// consumer.go
type Handler func(ctx context.Context, d Delivery) error
type Delivery struct{ Body []byte; Headers map[string]any; RoutingKey, MessageID string; Attempt int }
var ErrPermanent = errors.New("mq: permanent failure")   // errors.Is -> DLQ ngay
type ConsumerConfig struct {
    Queue string
    MaxAttempts int             // mac dinh 3
    RetryExchange, RetryKey string   // rong -> khong retry (lan loi dau di DLQ neu co DLX, khong co thi nack requeue=false)
    DeadLetterExchange, DeadLetterKey string
    Prefetch int                // mac dinh 10
}
const AttemptHeader = "x-ef-attempt"
// Quyet dinh thuan (unit test khong can broker):
type Action int   // ActionAck, ActionRetry, ActionDeadLetter
func decide(handlerErr error, attempt, maxAttempts int, canRetry bool) Action
func Consume(ctx context.Context, ch *amqp091.Channel, pub *Publisher, cfg ConsumerConfig, h Handler) error
// - Attempt doc tu AttemptHeader (thieu/hong -> 1)
// - otelx.ExtractAMQP + span "mq.consume <queue>" cha la span cua publisher
// - Retry/DLQ: pub.Publish ban sao (giu headers goc, AttemptHeader = attempt+1 cho retry, = attempt cho DLQ) roi Ack;
//   publish loi -> Nack(requeue=true) (AC-6)
// - ctx huy -> dung consume, tra ctx.Err(); kenh deliveries dong -> tra loi
```

Tên mã nguồn và comment theo phong cách `libs/go` (comment tiếng Việt không dấu, `fmt.Errorf("...: %w")`, test chỉ dùng
`testing` + `otelx` recorder, không testify).

## 2. Test

- `mq/topology_test.go`: topology AI đúng §2 spec (hàng trễ riêng, return key, DLQ bindings, không priority, không DLX trên
  hàng gốc); `Declare` gọi đúng thứ tự/đối số với fake `Declarer`, lỗi được bọc; **so khớp `definitions.json`** (đọc file
  qua đường dẫn tương đối từ thư mục test, chuẩn hoá số JSON sang int64).
- `mq/consumer_test.go`: bảng `decide` (nil→Ack; lỗi + attempt<max + canRetry→Retry; attempt==max→DeadLetter;
  ErrPermanent→DeadLetter; không retry exchange→DeadLetter); đọc attempt từ header (thiếu, int32/int64/string, hỏng).
- `mq/trace_test.go`: header sau `InjectAMQP` qua `Delivery` → span consume cùng trace ID (dùng recorder như
  `otelx/otelx_test.go:28-42`).
- `mq/integration_test.go` (`//go:build integration`, testcontainers `modules/rabbitmq`, ảnh `rabbitmq:3.13-management-alpine`):
  topology test với hàng trễ TTL ngắn (vd. 200 ms); handler luôn lỗi → sau đúng 3 lần xử lý message nằm ở DLQ với
  `x-ef-attempt=3`; trace ID giống nhau ở cả 3 lần; publisher confirm ack; `Declare` hai lần không lỗi; `Declare(AITopology())`
  trên broker thật thành công (bắt T5).

## 3. CI

`integration.yml`: thêm bước `go test -tags=integration ./mq/...` với `working-directory: libs/go` (Go 1.26, cache
`libs/go/go.sum`). Không sửa `ci.yml` (`test-go` đã chạy `libs/go` không tag).

## 4. File được phép đổi

`libs/go/mq/**`, `libs/go/go.mod`, `libs/go/go.sum`, `go.work.sum` (nếu `go work sync` đổi), `deploy/rabbitmq/definitions.json`,
`.github/workflows/integration.yml`, `docs/workflow/EVF-7-libs-go-mq/**`, `docs/09-*.md` dòng quyết định nguồn chuẩn (242),
`docs/03-cau-truc-src.md` mục `mq/` nếu mô tả sai. **Không** đụng `services/**`.

## 5. Rủi ro

| Rủi ro | Giảm thiểu |
|---|---|
| Không chạy được test tích hợp trên máy | Unit test phủ logic; tích hợp chạy CI, link run vào evidence |
| `go.mod` bị nâng dòng `go`/thêm `toolchain` | `GOTOOLCHAIN=local`; kiểm `git diff libs/go/go.mod` trước commit |
| Dependency testcontainers kéo nhiều module, ảnh hưởng `go.work`/service | Chỉ dùng trong file `//go:build integration`; chạy `go work sync`, build mọi service bằng `go build ./...` từ gốc workspace |
| ai-worker Python dựa vào topology cũ | Chưa có code dùng (aio-pika chưa được gọi); ghi topology mới vào docs cho EVF-80/82 |
| Lệch kiểu số khi so JSON (float64 vs int64) | Chuẩn hoá trước khi so |

## 6. Rollback

Revert merge commit. Chưa service nào dùng `mq`, topology chưa được nạp ở môi trường nào (T7).

## 7. Sửa sau plan review vòng 1 (`plan-review.md`: PASS WITH CHANGES)

Giả định môi trường: **RabbitMQ 3.13** (ảnh compose). Thiết kế ghi rõ điều này; 4.x đổi `delivery-limit` mặc định thành 20.

| Finding | Quyết định (thay §1–§5 khi khác) |
|---|---|
| M1 | **Không** chạy `go work sync`. Chỉ `go get` + `go mod tidy` trong `libs/go`; kiểm `go build ./... && go vet ./...` từng module ở chế độ workspace. Bỏ `go.work.sum` khỏi danh sách file được phép; `git diff` chỉ được có `libs/go/go.mod`, `libs/go/go.sum` ngoài `mq/`. |
| M2 | `Publish` dùng `mandatory = true`, theo dõi `NotifyReturn`; message bị trả về → `ErrUnroutable` (kể cả broker vẫn ack). Consumer coi publish retry/DLQ lỗi như AC-6. |
| M3 | Hàng chờ trễ AI thêm `x-dead-letter-strategy: at-least-once` và `x-overflow: reject-publish`. Test topology + `definitions.json` có hai tham số này. |
| M4 | Publish retry/DLQ lỗi → `Nack(requeue=true)` **rồi `Consume` trả lỗi** (dừng, để supervisor khởi động lại) — không vòng lặp nóng. |
| M5 | `Consume` trả lỗi ngay khi `cfg.DeadLetterExchange` rỗng (không cho cấu hình làm mất message). `MaxAttempts < 1` → lỗi. |
| M6 | Được sửa `docs/02-kien-truc.md` (mục topology AI, dòng ~224) theo topology mới + giao thức retry (`x-ef-attempt`, hàng trễ riêng, DLQ) cho EVF-80/82; cập nhật `_comment` trong `definitions.json`. |
| m1 | Kênh đóng → `ErrClosed` (kiểm `IsClosed()`), không trả nhầm `ErrNacked`. Ghi chú: `ctx` có hiệu lực ở bước chờ confirm. |
| m2 | `Publisher` không bọc mutex quanh việc chờ confirm (thư viện đã an toàn theo kênh). Khuyến nghị (doc comment + test tích hợp): kênh publish **tách** kênh consume. `Consume` nhận `*Publisher` riêng. |
| m3 | Header publish là **bản sao** (map nil an toàn), inject trace sau khi mở span. Khi chép sang retry/DLQ: bỏ `x-death`, `x-first-death-*`, `x-last-death-*`, `x-delivery-count`. Giá trị header chỉ kiểu phẳng (string/số/bool), không map lồng. |
| m4 | `decide`: `attempt >= maxAttempts` → DeadLetter. |
| m5 | Handler panic → `recover` và coi là lỗi thường (đi retry/DLQ). Doc comment: handler phải idempotent (at-least-once, có thể trùng lần thử). |
| m6 | Span publish `SpanKindProducer`, consume `SpanKindConsumer` (dùng tracer `otel.Tracer("libs/go/mq")` trực tiếp + `otelx.InjectAMQP/ExtractAMQP`). Test tự chép helper recorder (không export từ otelx), không `t.Parallel`. |
| m7 | Bước CI chạy thêm `go vet -tags=integration ./mq/...`. |
| m8 | Test tích hợp **fail** (không `t.Skip`) khi không có Docker; bước CI dùng `.github/scripts/run-and-report-tests.sh` với `-v` như các bước test khác; `cache-dependency-path` liệt kê `libs/go/go.sum` và `services/ticketing/go.sum`. |
| m9 | Chạy golangci-lint v2.12.2 trên máy qua `go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.12.2 run ./...` trong `libs/go`. |

## 8. Sửa sau code review vòng 1 (`code-review.md`: FAIL — 1 blocker, 2 major)

| Finding | Quyết định |
|---|---|
| B1 | Mọi lần đọc `p.returns` dùng `r, ok := <-p.returns`; `!ok` (kênh đã đóng) → thoát ngay với `ErrClosed`. Vòng chờ confirm kiểm `ctx.Done()` và trạng thái đóng ở mỗi vòng; không có nhánh nào lặp không chặn. Unit test: returns đã đóng + confirm trả `(false, nil)` → `Publish` trả lỗi trong thời gian ngắn, không treo. |
| M1 | Mỗi lần publish gắn header `x-ef-publish-id` (giá trị duy nhất, vd. UUID/bộ đếm + ngẫu nhiên); return chỉ được tính cho đúng publish có id đó; return cũ khác id bị bỏ qua. Header này **không** chép sang retry/DLQ (strip khi copy) và được ghi vào doc. |
| M2 | `attemptOf` nhận mọi kiểu số nguyên amqp091 có thể giải mã (int8/16/32/64, int, uint8/byte, uint16, uint32, uint64) và string; số ≤ 0 hoặc tràn → 1. Doc (`docs/02`) ghi header là int32 có dấu. |
| m1 | Giữ mutex quanh publish→confirm (đúng và an toàn theo review); ghi rõ lệch so với §7 m2 và lý do (gán `basic.return` đúng publish) trong doc comment và mục này. Hệ quả: một publish cùng lúc mỗi `Publisher`; cần thông lượng cao thì tạo nhiều `Publisher` trên nhiều kênh. |
| m2 | `Consume` gọi `ch.Cancel(consumerTag, false)` khi thoát; doc: caller đóng kênh. |
| m3 | Bản sao retry/DLQ giữ các property `ContentType`, `ContentEncoding`, `CorrelationId`, `ReplyTo`, `Type`, `Timestamp`, `MessageId`, `Priority`, `AppId`, `DeliveryMode` (persistent). Sửa `docs/02` cho đúng. |
| m4 | `Delivery.Headers` là **bản sao**; sửa trong handler không ảnh hưởng bản chuyển tiếp. |
| m5 | `ErrClosed` bọc `amqp091.ErrClosed` (`errors.Is` cả hai). |
| m6 | `validate`: `RetryExchange` và `RetryKey` phải cùng có hoặc cùng rỗng. |
| m7 | Test so khớp JSON so **kiểu và giá trị** (không `%v`), và báo lỗi khi có phần tử trùng. |
| m8 | Sửa `docs/09` (dòng ~241: nguồn chuẩn là code, `definitions.json` là bản sao được test so khớp), `docs/03` (dòng ~98 mô tả `mq/`), `_comment` của `definitions.json` (bỏ "Co alert" — alert là EVF-82/125). |
| m9 | Test tích hợp thêm: nack thật → `ErrNacked` (hàng `x-max-length: 0` + `x-overflow: reject-publish`); kênh đóng giữa chừng → `errors.Is(err, ErrClosed)`; ctx huỷ khi đang chờ confirm → lỗi bọc `context.Canceled`. |
| m10 | `integration_test.go`: kiểm lỗi `ch.Close()` (errcheck). CI `lint-go` không dùng tag integration — chạy golangci với `--build-tags integration` ở máy trong bước gates. |
| File được phép thêm | `docs/09-*.md`, `docs/03-cau-truc-src.md` (chỉ mô tả `mq`). |

## 9. Sửa sau challenge (vòng 2; `challenge.md`: PASS, F1 là lỗi thật mức thấp)

| Finding | Quyết định |
|---|---|
| F1 | Trong lúc chờ confirm, `Publish` **đọc returns đồng thời** (`select` trên `returns`, `dc.Done()`, `ctx.Done()`), không chỉ sau khi confirm xong; return khác `x-ef-publish-id` bị bỏ ngay, nên bộ đệm `NotifyReturn` không đầy và `notifyAll` của amqp091 không phải chờ 5s. Khi `Publish` trả vì `ctx` huỷ, không để lại return chưa đọc của chính nó gây đầy bộ đệm cho lần sau (return đến muộn bị bỏ qua ở lần publish kế tiếp — đã có test M1). Unit test: >16 publish bị huỷ giữa chừng rồi một publish unroutable vẫn nhận `ErrUnroutable`, không treo. |
| F2 | Test (unit, fake channel): `Publish` chuyển đủ `CorrelationId`, `ReplyTo`, `Type`, `AppId`, `Timestamp`, `Priority`, `ContentEncoding`, `ContentType`, `MessageId`, persistent. |
| F3 | Test: hai `Publish` đồng thời trên một `Publisher`, một unroutable → đúng lần đó nhận `ErrUnroutable`, lần kia `nil` (giết mutant bỏ mutex). |
| F4 | Tách phần nối dây của `Consume` để test được bằng fake (vd. interface `consumeChannel` với `Qos`, `Consume`, `Cancel`): deliveries đóng → `Consume` trả **lỗi** (không `nil`); `Qos(prefetch)` được gọi; `Cancel(tag)` khi thoát; `RetryExchange` rỗng → lỗi đầu tiên đi thẳng DLQ. |
| F5 | Fake `Declarer` cho phép lỗi chỉ ở `QueueDeclare` mà bind sau đó vẫn thành công → `Declare` phải trả lỗi. |
| F6 | Test sync JSON: `policies` phải rỗng (một policy có thể gắn lại DLX), từ chối khoá lạ ở mức gốc/phần tử, phát hiện khoá JSON trùng (decoder dạng token). |
| F7 | `doc.go`: không bật `amqp091.Config.Recovery` (đổi ngữ nghĩa lỗi); supervisor khởi động lại khi `Consume` trả lỗi. |
| F8 | Ghi nhận: bước "oversell smoke" của ticketing trong `integration.yml` 0 PASS / 40 SKIP — có từ trước (EVF-39), không thuộc slice này. |
| F9 | Doc: `x-ef-publish-id` có thể thấy trong `Delivery.Headers`; `attemptOf` coi float là 1 (an toàn: thư viện ghi lại int32). |
