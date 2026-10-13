# EVF-36 — Kế hoạch thực thi (Plan)

Đầu vào: `spec.md` (AC-1..AC-9). Worktree `D:/Documents/Nam4-1/Project1/EvenFlow-outbox`, xếp trên EVF-7 (`407f599`).
Go: `export PATH="/c/Program Files/Go/bin:$PATH" GOTOOLCHAIN=local`. Không `go work sync`. Không nâng dòng `go` trong go.mod.

## 1. Hợp đồng chốt trước

### 1.1 Migration `migrations/0002_outbox_relay.sql` (goose)

```sql
-- +goose Up
ALTER TABLE outbox_events
  ADD COLUMN event_id        UUID        NOT NULL DEFAULT gen_random_uuid(),
  ADD COLUMN headers         JSONB       NOT NULL DEFAULT '{}'::jsonb,
  ADD COLUMN attempts        INT         NOT NULL DEFAULT 0,
  ADD COLUMN last_error      TEXT,
  ADD COLUMN next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT now();
ALTER TABLE outbox_events ADD CONSTRAINT outbox_events_event_id_key UNIQUE (event_id);
DROP INDEX IF EXISTS idx_outbox_unpublished;
CREATE INDEX idx_outbox_due ON outbox_events (next_attempt_at, id) WHERE published_at IS NULL;
-- +goose Down
-- nguoc lai: drop index moi, tao lai idx_outbox_unpublished, drop constraint va 5 cot
```
(`gen_random_uuid()` có sẵn từ PostgreSQL 13; compose dùng 16.)

### 1.2 Package `github.com/eventflow/eventflow/libs/go/outbox`

```go
// Execer la phan cua pgx.Tx ma Write can (de test bang fake, khong keo pgx vao API).
type Execer interface {
    Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}
type Event struct {
    AggregateType string
    AggregateID   string // UUID dang chuoi
    RoutingKey    string // <domain>.<entity>.<action>
    Data          any    // marshal JSON thanh truong "data"
}
const SchemaVersion = 1
// Write sinh event_id (UUID v4), dung phong bi (spec AC-2), luu headers trace (otelx.InjectAMQP tu ctx)
// va trace_id; tra ve event_id. Goi trong transaction nghiep vu.
func Write(ctx context.Context, tx Execer, e Event) (string, error)

// Envelope la body JSON duoc publish.
type Envelope struct {
    EventID string `json:"event_id"`; OccurredAt time.Time `json:"occurred_at"`; TraceID string `json:"trace_id,omitempty"`
    SchemaVersion int `json:"schema_version"`; Type string `json:"type"`; Data json.RawMessage `json:"data"`
}

// Relay
type DB interface { Begin(ctx context.Context) (pgx.Tx, error) }   // *pgxpool.Pool thoa man
type Publisher interface { Publish(ctx context.Context, exchange, key string, msg mq.Message) error } // *mq.Publisher
type RelayConfig struct {
    Exchange string        // bat buoc
    BatchSize int          // mac dinh 100
    Interval time.Duration // mac dinh 500ms (cho khi lo trong)
    BaseBackoff, MaxBackoff time.Duration // mac dinh 1s, 5m (lui = min(Base*2^(attempts-1), Max))
    PublishTimeout time.Duration // mac dinh 10s moi message
}
type Relay struct{ /* db, pub, cfg, metrics */ }
func NewRelay(db DB, pub Publisher, cfg RelayConfig) (*Relay, error) // cfg sai -> loi
func (r *Relay) Run(ctx context.Context) error   // vong lap toi khi ctx huy -> tra nil; loi DB lien tiep -> lui va thu tiep, khong thoat
func (r *Relay) RunOnce(ctx context.Context) (published int, err error) // mot lo; de test
```

Một lô (`RunOnce`): `BEGIN` → `SELECT id, event_id, routing_key, payload, headers, created_at, attempts FROM outbox_events WHERE
published_at IS NULL AND next_attempt_at <= now() ORDER BY id LIMIT $1 FOR UPDATE SKIP LOCKED` → với mỗi row: khôi phục ctx từ
`headers` (`otelx.ExtractAMQP`), `Publish` với `MessageID = event_id`, `ContentType = application/json`, `Type = routing_key`,
`Timestamp = created_at`, `Headers = headers` → thành công: `UPDATE published_at = now(), last_error = NULL`; `errors.Is(err,
mq.ErrUnroutable)`: `UPDATE published_at = now(), last_error = 'unroutable'` + metric; lỗi khác: `UPDATE attempts = attempts+1,
last_error = $err, next_attempt_at = now() + backoff`; nếu lỗi là `mq.ErrClosed` thì dừng lô (phần còn lại để lô sau) và trả lỗi
để worker dựng lại kênh → `COMMIT`. Publish **trước** commit đánh dấu ⇒ chết giữa chừng chỉ gây gửi trùng (at-least-once), không mất.

Metric (Prometheus, đăng ký qua registry truyền vào hoặc mặc định): `outbox_pending` (gauge, đếm lô gần nhất hoặc
`SELECT count(*)` định kỳ), `outbox_published_total`, `outbox_publish_errors_total`, `outbox_unroutable_total`,
`outbox_publish_lag_seconds` (histogram `now - created_at`).

### 1.3 Ticketing

- `adapter/postgres/repository.go` `PersistHold`: thay INSERT thô bằng `outbox.Write(ctx, tx, Event{AggregateType:"order",
  AggregateID: order_id, RoutingKey:"ticketing.hold.created", Data:{order_id, sale_event_id, hold_id?, expires_at, quantity}})`.
- `ReleaseHold`: trong cùng tx khi giải phóng hold, `outbox.Write(... "ticketing.hold.released", Data:{sale_event_id, hold_id,
  released_at})`.
- `worker/hold_sweeper.go`: bỏ publish thẳng; `port.Publisher` và `logPublisher` bị xoá nếu không còn ai dùng.
- `cmd/worker/main.go`: đọc `AMQP_URL` (thiếu ⇒ thoát với lỗi rõ), dial, mở **hai** kênh (topology + publish), `mq.Declare(ch,
  mq.TicketingTopology())`, `mq.NewPublisher`, chạy `relay.Run` cùng sweeper/seeder/metrics (errs cap 4); relay trả lỗi kênh ⇒
  dừng worker (supervisor khởi động lại — đúng tinh thần `libs/go/mq/doc.go`).
- `deploy/compose/docker-compose.yml`: `ticketing-worker` thêm `AMQP_URL` + `depends_on: rabbitmq` (healthy).

## 2. Test

- `libs/go/outbox`: unit `Write` (fake Execer: SQL + tham số, event_id UUID v4, phong bì, headers trace khi có span), backoff,
  `NewRelay` validate; `RunOnce` với fake DB/Tx + fake Publisher: thành công / unroutable / lỗi tạm / ErrClosed dừng lô.
- Tích hợp `//go:build integration` (testcontainers `modules/postgres` v0.44.0 với `postgres:16-alpine` + RabbitMQ 3.13; chạy
  phần Up của `0001` và `0002` trực tiếp bằng SQL): AC-1 rollback không có row; AC-3; AC-4 kill-restart (Publisher bọc "chết"
  sau k lần, relay mới); RabbitMQ stop/start (`rabbitmqctl stop_app/start_app`); AC-5 trace; AC-6 unroutable; AC-8 Up→Down→Up;
  hai relay song song không publish trùng trong cùng thời điểm (SKIP LOCKED).
- `services/ticketing`: unit/integration cho `PersistHold`/`ReleaseHold` ghi outbox trong tx (dùng container Postgres ở trên hoặc
  fake), sweeper không còn publish.
- CI `integration.yml`: thêm bước cho `libs/go/outbox` giống `mq`.

## 3. File được phép đổi

`migrations/0002_outbox_relay.sql` (mới), `libs/go/outbox/**`, `libs/go/go.mod`, `libs/go/go.sum`, `services/ticketing/**`
(repository, worker, cmd/worker, port, test liên quan, go.mod/go.sum nếu cần), `deploy/compose/docker-compose.yml` (chỉ
`ticketing-worker`), `.github/workflows/integration.yml`, `docs/02-kien-truc.md`, `docs/03-cau-truc-src.md`, `docs/09-*.md`
(dòng W6), `docs/workflow/EVF-36-outbox-relay/**`.

## 4. Rủi ro

| Rủi ro | Giảm thiểu |
|---|---|
| Migration lỗi trên dữ liệu thật | Chỉ ADD COLUMN có DEFAULT + index; test Up→Down→Up trên DB có sẵn row |
| Relay giữ khoá lâu (publish chậm trong tx) | `PublishTimeout` mỗi message, `BatchSize` vừa phải; SKIP LOCKED cho bản khác chạy |
| Gửi trùng | Thiết kế at-least-once có chủ đích; `MessageId = event_id` để consumer khử trùng |
| Đổi tên trường payload | Chưa có consumer nào; ghi rõ trong docs |
| `mandatory` gây kẹt | AC-6 đánh dấu unroutable |
| Mất kết nối RabbitMQ | ErrClosed ⇒ worker thoát, supervisor (compose `restart`) khởi động lại; row chưa đánh dấu sẽ được gửi lại |

## 5. Rollback

Revert merge commit; chạy `goose down` một bước (Down của 0002) — các cột mới bị bỏ, dữ liệu outbox cũ giữ nguyên.
(Đã thay bằng §6.1: production chỉ rollback ứng dụng.)

## 6. Kế hoạch v2 — sửa sau plan review vòng 1 (`plan-review.md`: FAIL, 2 blocker, 9 major)

Mục này **thay thế** §1.1–§1.3, §4, §5 ở những chỗ khác nhau.

### 6.1 Migration `0002_outbox_relay.sql` — bản đầy đủ (B2, M7)

```sql
-- +goose Up
-- Bang outbox_events nho (row cu duoc don o phase 2) nen rewrite do default volatile chap nhan duoc;
-- lock_timeout de migration bo cuoc thay vi xep hang sau relay / PersistHold va chan ca he thong.
SET LOCAL lock_timeout = '5s';
ALTER TABLE outbox_events
  ADD COLUMN event_id        UUID        NOT NULL DEFAULT gen_random_uuid(),
  ADD COLUMN headers         JSONB       NOT NULL DEFAULT '{}'::jsonb,
  ADD COLUMN attempts        INT         NOT NULL DEFAULT 0,
  ADD COLUMN last_error      TEXT,
  ADD COLUMN next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT now();
ALTER TABLE outbox_events ADD CONSTRAINT outbox_events_event_id_key UNIQUE (event_id);
DROP INDEX IF EXISTS idx_outbox_unpublished;
CREATE INDEX idx_outbox_due ON outbox_events (next_attempt_at, id) WHERE published_at IS NULL;

-- +goose Down
SET LOCAL lock_timeout = '5s';
DROP INDEX IF EXISTS idx_outbox_due;
CREATE INDEX IF NOT EXISTS idx_outbox_unpublished ON outbox_events (id) WHERE published_at IS NULL;
ALTER TABLE outbox_events DROP CONSTRAINT IF EXISTS outbox_events_event_id_key;
ALTER TABLE outbox_events
  DROP COLUMN IF EXISTS next_attempt_at,
  DROP COLUMN IF EXISTS last_error,
  DROP COLUMN IF EXISTS attempts,
  DROP COLUMN IF EXISTS headers,
  DROP COLUMN IF EXISTS event_id;
```

- **Tương thích ngược:** mọi cột mới có DEFAULT ⇒ câu INSERT cũ (`repository.go:187-189`) vẫn chạy trên schema 0002. Vì vậy
  **rollback trên production chỉ là rollback ứng dụng** (giữ schema 0002, đúng quy tắc forward-only ở `docs/03:215`). Down chỉ
  dùng ở dev/test.
- **Thứ tự deploy:** chạy migration → deploy worker/server mới. Trước khi migrate kiểm `SELECT count(*) FROM outbox_events`
  (ghi trong docs); bảng lớn (hàng triệu row) thì dùng cách thay thế: thêm cột nullable, backfill theo lô, rồi `SET NOT NULL` —
  ghi trong docs, không làm trong slice này.
- **Diễn tập rollback (test tích hợp):** trên schema 0002, câu INSERT kiểu cũ thành công và nhận `event_id` tự sinh; chạy Down rồi
  Up lại trên DB có dữ liệu ⇒ thành công.
- **Điểm duyệt của người dùng:** PR ghi rõ SQL của migration, tác động khoá, thứ tự deploy, cách rollback; **không merge khi người
  dùng chưa duyệt**.

### 6.2 Thiết kế relay: lease, publish ngoài transaction (M6, M1, M9)

Mỗi lô gồm ba bước, không bước nào giữ transaction trong lúc gọi mạng:
1. **Claim** (một transaction ngắn, `SET LOCAL statement_timeout = '5s'`):
   `UPDATE outbox_events SET next_attempt_at = now() + $lease WHERE id IN (SELECT id FROM outbox_events WHERE published_at IS NULL
   AND next_attempt_at <= now() ORDER BY next_attempt_at, id LIMIT $batch FOR UPDATE SKIP LOCKED) RETURNING id, event_id,
   routing_key, payload, headers, trace_id, created_at, attempts` (lease mặc định 60s). Relay khác bỏ qua các row đã claim cho tới
   khi lease hết hạn.
2. **Publish** từng row ngoài transaction, có `PublishTimeout` và hạn chót cả lô.
3. **Đánh dấu** từng kết quả bằng câu UPDATE riêng (autocommit), chạy dưới `context.WithoutCancel(ctx)` + timeout 5s để SIGTERM
   không làm mất dấu đã gửi. Lỗi DB ở bước này ⇒ **dừng lô** (row còn lại hết lease thì được gửi lại — at-least-once).
- Chết sau publish mà trước đánh dấu ⇒ gửi lại sau khi lease hết hạn (trùng, không mất); số bản trùng ≤ kích thước lô.
- **Thứ tự không được bảo đảm** (kể cả giữa `hold.created` và `hold.released` cùng đơn) — ghi vào spec; consumer dựa vào
  `occurred_at` và `event_id`.

### 6.3 Kết nối RabbitMQ thuộc về relay (B1)

- Relay tự quản lý kết nối: dial, mở kênh, `mq.Declare(TicketingTopology)`, `mq.NewPublisher`; gặp `mq.ErrClosed` hoặc dial lỗi ⇒
  đóng, chờ backoff có trần (1s → 30s, có jitter) rồi dial lại. **Worker không thoát** khi broker sập — sweeper tiếp tục giải phóng
  hold, event tích lại trong outbox.
- Worker chỉ fail fast khi `AMQP_URL` thiếu hoặc sai cú pháp (`amqp091.ParseURI`).
- Gauge `outbox_relay_connected` (0/1). AC-7 cập nhật theo.
- API: `outbox.NewRelay(db DB, dialer Dialer, cfg RelayConfig)` với `type Dialer func(ctx context.Context) (Publisher, io.Closer, error)`
  — worker truyền dialer thật (amqp091 + mq); test truyền dialer giả hoặc dialer tới container.

### 6.4 Payload, headers, giới hạn (M2, M3, M4, M5, m1, m2, m6)

- `payload` chỉ lưu **`data`**; relay dựng phong bì từ cột (`event_id`, `created_at` → `occurred_at`, `trace_id`, `routing_key` →
  `type`, `schema_version = 1`). Row cũ trước 0002 cũng được bọc đúng phong bì.
- `headers` lưu `map[string]string` (chỉ khoá trace W3C: `traceparent`, `tracestate`, `baggage`); khi publish chuyển thành
  `amqp091.Table` giá trị string ⇒ luôn hợp lệ.
- `last_error`: `strings.ToValidUTF8`, bỏ `\x00`, cắt 1 KiB.
- `MaxAttempts` (mặc định 20): vượt ⇒ **đỗ** row: `next_attempt_at = 'infinity'`, giữ `last_error`, metric `outbox_dead_total`
  + log lỗi. Không cần cột mới.
- **Unroutable:** chỉ routing key trong `RelayConfig.DropUnroutable` (ticketing: `ticketing.hold.created`) mới được đánh dấu
  `published_at` + `last_error = 'unroutable'`; key khác ⇒ lỗi tạm, thử lại có backoff (rồi đỗ khi quá `MaxAttempts`).
- Backoff: `min(Base * 2^min(attempts-1, 20), Max)` + jitter ±20%.
- `Write` kiểm: routing key khớp `^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*){2,}$`, `AggregateID` là UUID, `data` JSON ≤ 64 KiB; vi phạm ⇒
  lỗi. `BatchSize` ≤ 1000. Không đưa PII vào `data` (ghi trong doc).
- `ReleaseHold` lấy `sale_event_id` bằng `UPDATE … FROM orders … RETURNING`; aggregate của `hold.released` là `hold`
  (`aggregate_id = hold_id`); `hold.created` giữ `order`, `data` gồm `order_id`, `hold_id`, `sale_event_id`, `expires_at`,
  `quantity`.

### 6.5 Metric, log, tài liệu (m3, m4, m5, m7, m12, m13)

- `NewRelay` nhận `prometheus.Registerer` bắt buộc (trong `RelayConfig`); nhãn duy nhất `routing_key` (tập hữu hạn). Gauge
  `outbox_pending` cập nhật mỗi 15s (`count(*) WHERE published_at IS NULL AND next_attempt_at <> 'infinity'`).
- Không bao giờ log `AMQP_URL`/`DATABASE_URL`.
- `docs/02:173`: `hold.released` được phát sau khi commit Postgres, kể cả khi bước giải phóng Redis lỗi (cửa sổ crash đã phân tích:
  an toàn). `docs/06`: cập nhật sơ đồ bảng outbox. `docs/03:71`: vị trí relay (`libs/go/outbox`, nối trong `cmd/worker`).
- Rủi ro ghi nhận: một `Publisher` chỉ một message đang bay ⇒ giới hạn thông lượng relay; `ticketing.hold.released.q` chưa có
  consumer ⇒ hàng đợi tăng (ngoài phạm vi).

### 6.6 Test và môi trường (M8, m8, m9, m10, m11)

- Postgres tích hợp dùng **`testcontainers.GenericContainer`** (core, đã có trong go.sum) với ảnh `postgres:16-alpine` (khớp
  compose), chạy SQL Up của 0001 + 0002 trực tiếp. **Không** thêm `modules/postgres`.
- Ticketing có test tích hợp **thật** cho `PersistHold`/`ReleaseHold` ghi outbox trong tx (rollback ⇒ không row) — dựng harness
  Postgres dùng chung (không đụng `test/concurrency` của EVF-39).
- Kill-restart: đóng **cứng** kết nối AMQP giữa lô (không chỉ huỷ ctx), relay mới chạy sau khi lease hết hạn (lease ngắn trong
  test); assert mọi `event_id` tới hàng đợi, số bản trùng ≤ batch.
- Tên test tích hợp bắt đầu `TestTichHop`; CI thêm `go vet -tags=integration` + chạy cho `libs/go/outbox` và `services/ticketing`.
- Giả định: test tích hợp không đi qua PgBouncer; pgx mặc định cache prepared statement — ghi rủi ro với `pgbouncer:latest`,
  kiểm ở bước smoke compose khi có Docker.
- Gate: `go build ./... && go vet ./...` cho **mọi** module trong workspace; image Docker được build bởi job Trivy trên CI.
- File được phép thêm: `docs/06-Thiet_ke_co_ban.md` (phần outbox), `go.work.sum` chỉ khi lệnh `go` tự cập nhật (giải thích trong PR).

## 7. Bổ sung sau plan review vòng 2 (`plan-review.md`: PASS WITH CHANGES)

| Finding | Quyết định |
|---|---|
| R1 | **Chỉ** các lỗi sau làm tăng `attempts`: `mq.ErrNacked`; `mq.ErrUnroutable` với key **không** thuộc `DropUnroutable`; hết `PublishTimeout` khi kênh vẫn mở; lỗi dựng message (phong bì/headers không hợp lệ). **Không** tính: `mq.ErrClosed`, dial lỗi, `ctx` huỷ, row chưa xử lý khi lô dừng sớm — những row này được **trả lease** ngay (`UPDATE … SET next_attempt_at = now() WHERE id = ANY($1) AND published_at IS NULL`). Relay **chỉ claim khi đang kết nối**. Gauge `outbox_dead` (đếm định kỳ row `next_attempt_at = 'infinity'`) thay cho counter; runbook trong `docs/02` có SQL gỡ đỗ (`UPDATE outbox_events SET next_attempt_at = now(), attempts = 0 WHERE … AND next_attempt_at = 'infinity'`). |
| R2 | Test tích hợp "đã publish, chưa đánh dấu": sau k publish thành công, làm bước đánh dấu thất bại (cắt kết nối DB của relay bằng `pg_terminate_backend` hoặc DB bọc lỗi), relay chết; relay mới với lease ngắn ⇒ mọi `event_id` tới hàng đợi ≥ 1 lần, mọi row có `published_at`, số bản trùng ≤ batch. |
| r1 | `RelayConfig.BatchDeadline` (mặc định 30s). `NewRelay` kiểm `PublishTimeout ≤ BatchDeadline < Lease`. Mọi UPDATE đánh dấu có `AND published_at IS NULL`. |
| r2 | Backoff: số mũ `clamp(attempts-1, 0, 20)`; `NewRelay` kiểm `0 < BaseBackoff ≤ MaxBackoff ≤ 1h`. Test attempts = 0, 1, 21, MaxInt32. |
| r3 | `ReleaseHold` dùng `RETURNING h.id::text` (dạng chuẩn có gạch) cho `aggregate_id` và `data.hold_id`. |
| r4 | Test AC-8: Up/Down chạy trong transaction tường minh (để `SET LOCAL` có hiệu lực); sau Down so cột, index, constraint của `outbox_events` với schema của 0001 (truy vấn `information_schema`/`pg_indexes`). |
| r5 | Harness Postgres dùng chung: `libs/go/testx/pgtest` (chỉ được import từ file `_test`/`integration`), áp dụng migration theo thứ tự file. Được phép thêm `libs/go/testx/**`. |
| r6 | Test B1: dialer luôn lỗi ⇒ `Run` không trả về tới khi `ctx` huỷ, gauge `connected` = 0, không row nào bị claim. Test worker: hàm `run()` của `cmd/worker` không dừng sweeper khi relay không kết nối được (tách hàm để test được). |
| r7 | Mỗi row được xử lý trong `recover`; panic ⇒ coi là lỗi dựng message (tăng `attempts`, ghi `last_error`). |
| r8 | Pool của worker dùng `pgx.QueryExecModeCacheDescribe` (an toàn với PgBouncer transaction mode bất kể `max_prepared_statements`); ghi lý do trong comment. |
| r9 | Ghi rủi ro: CI không chạy binary goose thật; 0002 chỉ gồm câu lệnh thường, test áp SQL trực tiếp. |
| r10 | Sự duyệt migration của người dùng (ai, khi nào, SHA commit) được ghi vào `docs/workflow/EVF-36-outbox-relay/approval.md` khi người dùng duyệt. |
| r11 | Docs: consumer đọc `event_id` **chỉ** từ phong bì; trong `data` của row cũ `event_id` là id sự kiện hoà nhạc. |

## 8. Sửa sau code review (`code-review.md`: PASS, chỉ minor)

| Finding | Quyết định |
|---|---|
| m1 | ctx của mỗi lần publish bị chặn bởi hạn chót lô (không vượt `BatchDeadline` tính từ lúc claim). `NewRelay` kiểm `BatchDeadline + PublishTimeout + 2×markTimeout < Lease` (markTimeout = 5s). Trả lease và đánh dấu đỗ đều có `AND published_at IS NULL AND next_attempt_at <> 'infinity'` khi trả lease, để không gỡ đỗ nhầm. |
| m2 | **Nợ, không sửa ở slice này:** broker chặn kết nối (cảnh báo bộ nhớ/đĩa) làm publish hết hạn và bị tính là lần thử; cảnh báo kéo dài ~1 giờ có thể đỗ `hold.released`. Ghi trong `doc.go`, `docs/02` (runbook gỡ đỗ) và đề xuất ticket vận hành theo dõi `NotifyBlocked`. |
| m3 | Sửa tài liệu cho khớp code (đã chấp nhận): lỗi publish **không phân loại** cũng tính là lần thử (chống row độc thử mãi); `ErrClosed`, dial lỗi, `ctx` huỷ, row chưa xử lý thì không. Cập nhật `doc.go`, `docs/02`, §7 R1 → trỏ sang mục này. Unit test cho lỗi không phân loại. |
| m4 | Trước mỗi lần claim, relay kiểm kết nối còn sống (closer/publisher hỗ trợ `IsClosed()` hoặc theo dõi `NotifyClose`); chết ⇒ gauge `outbox_relay_connected` = 0, đóng và dial lại, không claim. Unit test. |
| m5 | Test tích hợp thêm biến thể **chỉ** cắt TCP (không huỷ ctx) để đi đúng nhánh `ErrClosed`. |
| m6 | Bước tích hợp của ticketing trong `integration.yml` chạy với `-v` qua `run-and-report-tests.sh` (bắt trường hợp toàn bộ test bị skip). |
| m7 | Không đổi (tuỳ chọn). |
| m8 | Metric published/lag chỉ ghi **sau** khi đánh dấu thành công. `released_at` của `hold.released` lấy từ DB (`now()` trong `RETURNING`) thay vì đồng hồ Go. |
| m9 | Ghi rủi ro: CI không chạy goose thật; pool của `cmd/server` vẫn dùng exec mode mặc định (có từ trước, ngoài phạm vi) — kiểm ở smoke compose. |

## 9. Sửa sau challenge (vòng 2; `challenge.md`: PASS, chỉ minor)

| Finding | Quyết định |
|---|---|
| F1 | `markRetrySQL` và `markParkSQL` thêm `AND next_attempt_at <> 'infinity'` (giống `releaseSQL`): relay đã quá lease không được gỡ đỗ hay đếm lần thử hai lần cho row mà relay khác đã đỗ. Unit test với fake: row đã đỗ không bị các UPDATE này đụng tới. |
| F2 | `Write` từ chối `data` mà JSON sau khi marshal chứa `\u0000` (jsonb của PostgreSQL không nhận) — trả lỗi validation trước khi chạm DB. Unit test. |
| F3 | Độ trễ publish tính theo đồng hồ DB: câu claim trả thêm `now()` của DB; lag = `db_now_at_claim - created_at` + thời gian trôi trong Go từ lúc claim; giá trị âm bị kẹp về 0. Unit test. |
| F4 | Bổ sung test giết các mutant còn sống: trả lease khi lô kết thúc/ctx huỷ (M47, M48); huỷ ctx / hết hạn lô không tính lần thử (M22, M23); backoff dial lại (M30, M31 — có thể qua seam cấu hình được trong test); biên đúng 64 KiB (W1); `AggregateType` rỗng (W6); danh sách khoá header cho phép (W7); SQL gauge và `statement_timeout` (M29, M36); metric lag và `publish_errors_total` (M42, M43). |
