# EVF-36 — Baseline của bộ test (trước khi implement)

| Mục | Giá trị |
|---|---|
| Người viết | Test designer (không phải implementer), viết từ `spec.md`, `plan.md` §6–§7 (ghi đè §1–§5), `plan-review.md` |
| Worktree | `D:/Documents/Nam4-1/Project1/EvenFlow-outbox`, nhánh `feature/EVF-36-outbox-relay` (trên EVF-7 `407f599`) |
| Ngày | 2026-10-13 |
| Môi trường | Go 1.27 local (`GOTOOLCHAIN=local`), Docker **không** chạy ⇒ test tích hợp chỉ được `go vet -tags=integration`, chạy thật ở CI |

## 1. File test đã thêm / sửa

| File | Loại | Nội dung |
|---|---|---|
| `libs/go/testx/pgtest/pgtest.go` | helper (tag `integration`) | Postgres `postgres:16-alpine` qua `testcontainers.GenericContainer` (không dùng `modules/postgres`), mỗi `New`/`NewEmpty` là một DATABASE mới; tách `-- +goose Up/Down`, áp từng phần trong **transaction tường minh**; không có Docker ⇒ `t.Fatalf` |
| `libs/go/outbox/fakes_test.go` | unit (fake) | `fakeDB` (DB/Tx/Rows ở mức SQL, scan bằng `pgtype.Map` thật), `fakePublisher`, `fakeDialer`, helper metric/trace |
| `libs/go/outbox/write_test.go` | unit | `Write`, kiểm tra đầu vào, `Envelope` |
| `libs/go/outbox/relay_test.go` | unit | `NewRelay`, `RunOnce`, `Run`, `backoff`, `sanitizeErr` |
| `libs/go/outbox/integration_test.go` | tích hợp | Postgres + RabbitMQ 3.13 thật: AC-3, AC-4 (2 kịch bản kill + stop/start broker), AC-5, AC-6, hai relay song song, AC-8 (4 test) |
| `services/ticketing/internal/adapter/postgres/repository_integration_test.go` | tích hợp | `PersistHold`/`ReleaseHold` ghi outbox cùng transaction (pgtest) |
| `services/ticketing/internal/worker/hold_sweeper_test.go` | unit (sửa) | Bỏ fake publisher; sweeper không còn publish thẳng; dùng chữ ký mới của `NewHoldSweeper` |
| `services/ticketing/cmd/worker/main_test.go` | unit | `loadConfig` (fail fast `AMQP_URL`), `relayConfig`, `newPoolConfig`, `amqpDialer`, `runLoops` (B1) |

Phụ thuộc: `libs/go/go.mod` thêm `github.com/jackc/pgx/v5 v5.9.2` (direct) và gián tiếp `pgpassfile`, `pgservicefile`,
`puddle/v2 v2.2.2`, `golang.org/x/sync v0.22.0` (lấy từ module cache, `GOPROXY=file://…`). Dòng `go 1.25.0` giữ nguyên, không
có dòng `toolchain`. **Không** sửa `services/ticketing/go.mod` (xem §4).

## 2. Symbol implementer phải cung cấp (chữ ký chính xác)

### 2.1 `libs/go/outbox` (export)

```go
type Execer interface {
    Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}
type Event struct {
    AggregateType string
    AggregateID   string // UUID
    RoutingKey    string // ^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*){2,}$
    Data          any    // JSON <= 64 KiB, lưu nguyên vào cột payload (CHỈ data)
}
const SchemaVersion = 1
func Write(ctx context.Context, tx Execer, e Event) (eventID string, err error)

type Envelope struct {
    EventID       string          `json:"event_id"`
    OccurredAt    time.Time       `json:"occurred_at"`
    TraceID       string          `json:"trace_id,omitempty"`
    SchemaVersion int             `json:"schema_version"`
    Type          string          `json:"type"`
    Data          json.RawMessage `json:"data"`
}

type DB interface { // *pgxpool.Pool phải thỏa
    Begin(ctx context.Context) (pgx.Tx, error)
    Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
    Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
    QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}
type Publisher interface { // *mq.Publisher phải thỏa
    Publish(ctx context.Context, exchange, routingKey string, msg mq.Message) error
}
type Dialer func(ctx context.Context) (Publisher, io.Closer, error)

type RelayConfig struct {
    Exchange       string                // bắt buộc
    BatchSize      int                   // 0 -> 100; 1..1000
    Interval       time.Duration         // 0 -> 500ms
    Lease          time.Duration         // 0 -> 60s
    BatchDeadline  time.Duration         // 0 -> 30s
    PublishTimeout time.Duration         // 0 -> 10s; PublishTimeout <= BatchDeadline < Lease
    BaseBackoff    time.Duration         // 0 -> 1s
    MaxBackoff     time.Duration         // 0 -> 5m; 0 < Base <= Max <= 1h
    MaxAttempts    int                   // 0 -> 20; âm -> lỗi
    DropUnroutable []string
    Registerer     prometheus.Registerer // bắt buộc
}
type Relay struct{ /* ... */ }
func NewRelay(db DB, dial Dialer, cfg RelayConfig) (*Relay, error) // db/dial nil -> lỗi
func (r *Relay) Run(ctx context.Context) error                     // ctx huỷ -> nil; không bao giờ trả lỗi vì broker
func (r *Relay) RunOnce(ctx context.Context) (published int, err error)
```

### 2.2 `libs/go/outbox` (seam không export)

```go
func backoff(attempts int, base, max time.Duration) time.Duration // min(base*2^clamp(attempts-1,0,20), max) * [0.8,1.2]
func sanitizeErr(err error) string                                 // ToValidUTF8, bỏ \x00, cắt <= 1024 byte (không cắt giữa rune)
```

### 2.3 Metric Prometheus (tên được test đọc)

`outbox_relay_connected` (gauge 0/1), `outbox_published_total{routing_key}`, `outbox_unroutable_total{routing_key}`.
Các metric khác của plan (`outbox_pending`, `outbox_dead`, `outbox_publish_errors_total`, `outbox_publish_lag_seconds`)
không bị test đọc nhưng không được trùng tên với ba metric trên.

### 2.4 Hợp đồng hành vi mà unit test dựa vào (`fakes_test.go`)

- **Claim:** một câu chứa `FOR UPDATE SKIP LOCKED`, `ORDER BY next_attempt_at, id`, lọc `published_at IS NULL`, đọc bằng
  `Query`/`QueryRow` (không `Exec`), cột lấy qua `RETURNING` (thứ tự tuỳ ý, cho phép `::text`). Chỉ claim khi đã kết nối.
- **Đánh dấu:** mỗi kết quả là một `UPDATE outbox_events …` riêng qua `DB.Exec` (không `SendBatch`), điều kiện `id = $n` hoặc
  `id = ANY($n)`, **luôn có `AND published_at IS NULL`**, chạy dưới `context.WithoutCancel` (fake trả `ctx.Err()` khi ctx đã huỷ):
  - đã gửi: `published_at = now()`; unroutable được phép bỏ: thêm `last_error = 'unroutable'` (hằng hoặc tham số);
  - thử lại: `attempts = attempts + 1, last_error = $x, next_attempt_at = now() + …` (không chứa `'infinity'`);
  - đỗ: `next_attempt_at = 'infinity'` (hằng trong SQL; quyết định đỗ ở Go: lần thất bại làm `attempts + 1 >= MaxAttempts`);
  - trả lease: `next_attempt_at = now()` (không đụng attempts).
- **Phân loại lỗi (R1):** tăng attempts — `mq.ErrNacked`, hết `PublishTimeout` khi kênh còn mở, `mq.ErrUnroutable` với key
  ngoài `DropUnroutable`, panic khi xử lý row (recover). Không tăng — `mq.ErrClosed` (đóng kết nối, trả lease phần còn lại,
  `RunOnce` trả lỗi bọc `mq.ErrClosed`, lần sau dial lại), ctx huỷ (đánh dấu phần đã gửi, trả lease phần còn lại, trả lỗi bọc
  `context.Canceled`), hết `BatchDeadline` (trả lease, không publish tiếp), dial lỗi (không claim).
- **Lỗi DB khi đánh dấu ⇒ dừng lô** (không publish tiếp), `RunOnce` trả lỗi.
- **Publish:** `MessageID = event_id`, `ContentType = application/json`, `Type = routing_key`, `Timestamp = created_at`,
  `Headers` = headers của row (mọi giá trị là `string`), ctx được khôi phục bằng `otelx.ExtractAMQP` từ headers; body =
  `Envelope` dựng từ cột (row cũ trước 0002 cũng vậy).
- **Write:** một câu `INSERT INTO outbox_events (<cột>) VALUES ($1, $2::uuid, …)`, mọi giá trị là placeholder; cột bắt buộc
  `event_id` (UUID v4 sinh ở Go, trùng giá trị trả về), `aggregate_type`, `aggregate_id`, `routing_key`, `payload`
  (`[]byte`/`string`/`json.RawMessage` hoặc giá trị JSON-hoá được), `headers` (`map[string]string` hoặc JSON của nó, chỉ khoá
  W3C), `trace_id` (bắt buộc khi ctx có span). Đầu vào sai ⇒ lỗi, **không** gọi `Exec`.

### 2.5 `services/ticketing`

```go
// internal/worker
func NewHoldSweeper(gate port.InventoryGate, repo port.Repository, cfg SweeperConfig, log *slog.Logger) *HoldSweeper
// (port.Publisher và logPublisher bị xoá nếu không còn ai dùng)

// cmd/worker (package main) — tách khỏi run()
type config struct{ AMQPURL string /* + các trường khác tuỳ ý */ }
func loadConfig(getenv func(string) string) (config, error)       // thiếu/sai AMQP_URL -> lỗi nêu "AMQP_URL", không lộ URL/mật khẩu
func amqpDialer(url string) outbox.Dialer                         // không dial lúc tạo; Declare(mq.TicketingTopology()) + NewPublisher
func relayConfig(reg prometheus.Registerer) outbox.RelayConfig    // Exchange "ticketing.events", DropUnroutable {"ticketing.hold.created"}
func newPoolConfig(dsn string) (*pgxpool.Config, error)           // DefaultQueryExecMode = QueryExecModeCacheDescribe
func runLoops(ctx context.Context, loops ...func(context.Context) error) error // vòng đầu tiên trả về -> huỷ ctx chung, chờ tất cả
```

`Repository.PersistHold` / `ReleaseHold` giữ chữ ký; hành vi mới: xem `traceability.md` (AC-1, AC-2).
Migration `migrations/0002_outbox_relay.sql` đúng như plan §6.1 (test so schema sau Down với 0001, cần `lock_timeout` có hiệu lực
trong transaction ⇒ lỗi `55P03`).

## 3. Trạng thái hiện tại (trước implement)

| Lệnh | Kết quả |
|---|---|
| `libs/go`: `go vet ./...` | **FAIL** — `outbox`: `undefined: Publisher, RelayConfig, Relay, DB, Dialer, Execer, …` (package chưa có file non-test). Các package khác sạch |
| `libs/go`: `go vet -tags=integration ./...` | **FAIL** — như trên (`outbox/integration_test.go: undefined: Publisher …`); `testx/pgtest` vet sạch |
| `libs/go`: `go test ./mq/... ./otelx/... ./httpx/...` | PASS (đổi go.mod không làm hỏng gì) |
| `services/ticketing`: `go vet ./...` | **FAIL** — `libs/go/outbox: build constraints exclude all Go files`; `internal/worker`: `not enough arguments in call to worker.NewHoldSweeper` (muốn 4 tham số); `cmd/worker`: `undefined: loadConfig, amqpDialer, relayConfig, newPoolConfig, runLoops, outbox.*` |
| `services/ticketing`: `go vet -tags=integration ./...` | **FAIL** — như trên; `adapter/postgres` integration test compile được khi có `outbox` (dùng workspace) |

Kiểm chứng chính bộ test (đã xoá sau khi kiểm): với một implementation tham chiếu tạm thời theo plan, `go test ./outbox/` (unit)
và `go test ./cmd/worker/ ./internal/worker/` **PASS**; `go vet -tags=integration` cho cả hai module sạch. 15 mutant đã bị bắt
(đánh dấu dùng ctx đã huỷ, bỏ `AND published_at IS NULL`, bỏ mọi unroutable, ErrClosed tăng attempts, không đỗ, không recover,
không kẹp số mũ, số mũ âm, gauge sai, lỗi DB không dừng lô, bỏ hạn lô, không khôi phục trace, `ORDER BY id`, không giới hạn
64 KiB, không bỏ NUL). Test tích hợp **chưa chạy** (không có Docker).

## 4. Lưu ý cho implementer

1. `services/ticketing/go.mod` cần thêm require `github.com/testcontainers/testcontainers-go v0.44.0` (và go.sum tương ứng)
   vì test tích hợp ticketing import `libs/go/testx/pgtest`. Trong workspace (`go.work`) đã build được; `GOWORK=off` thì lỗi
   `missing go.sum entry`. Test designer không sửa go.mod của ticketing theo chỉ dẫn.
2. `go mod tidy` của `libs/go` sẽ đưa `github.com/prometheus/client_model` lên direct (test import `dto`).
3. CI `integration.yml` cần bước `go vet -tags=integration` + `go test -tags=integration` cho `./outbox/...` (tên `TestTichHop`),
   ticketing đã có bước `./...`.
4. Test `TestTichHop_AC4_RabbitMQDungRoiChayLaiKhongMatHoldReleased` chờ tối đa 120s cho relay dial lại (backoff 1s→30s).
