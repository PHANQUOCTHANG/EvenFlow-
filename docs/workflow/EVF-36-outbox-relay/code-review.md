# EVF-36 — Code review + security review (High-risk: migration DB)

| Mục | Giá trị |
|---|---|
| Phạm vi | `git diff origin/feature/EVF-7-libs-go-mq...HEAD`, branch `feature/EVF-36-outbox-relay`, PR #55 (draft, base `develop`) |
| Commit được review | `6f29095` + `553c828` (commit sau chỉ ép kiểu `::text` cho snapshot catalog trong test AC-8) |
| Tài liệu đối chiếu | `spec.md`, `plan.md` (§6 + §7 thay cho §1–§5), `plan-review.md`, `baseline.md`, `traceability.md` |
| Reviewer | Reviewer độc lập (context sạch), kiêm review bảo mật |

## Kết luận: **PASS**

Không có blocker hay major. Có 9 minor/nit, không cái nào chặn merge. **Điều kiện merge vẫn giữ nguyên:** người dùng phải duyệt
migration `0002` và ghi vào `approval.md` (§7 r10). File này hiện chưa có, đúng như dự kiến.

## 1. Bằng chứng đã chạy

| Kiểm tra | Kết quả |
|---|---|
| `go vet ./...` + `go vet -tags=integration` (`libs/go`: outbox, testx; `services/ticketing`) | sạch |
| `go test -race -count=1` `libs/go/outbox/...`, `services/ticketing/...` (máy local, Go 1.27, `GOTOOLCHAIN=local`) | pass |
| `go mod tidy -diff` cho `libs/go`, `services/ticketing` | không có diff; `go 1.25.0`, không có dòng `toolchain`, `go.work` / `go.work.sum` không đổi |
| `go build ./...` cho mọi service | pass. Riêng `identity` không tải được dependency do proxy offline, module này không bị đổi |
| **CI PR #55 trên `553c828`** | **mọi check xanh**: `integration`, `test-go`, `lint-go`, `build`, trivy ×4, `trivy-gate`, `validate-deploy` |
| CI `integration` run 38344301364: `libs/go/outbox` | 11/11 `TestTichHop_*` PASS: AC3 (25 event, 0 trùng), AC4 hard-kill (30, 0), AC4-R2 (30, 1 trùng ≤ lô), AC4 broker stop/start (25, 0), AC5, AC6, hai relay song song (200, 0 trùng), AC8 ×4 (Up/Down/Up, lock_timeout `55P03` trong ≈5s, INSERT kiểu cũ, row cũ được bọc phong bì) |
| CI `integration`: `services/ticketing` | `internal/adapter/postgres` ok trong 8.1s (test PersistHold/ReleaseHold chạy thật trên Postgres 16) |
| Run trước đó (38343980004) | FAIL ở `TestTichHop_AC8_UpDownUpTraDungSchema0001`: cột catalog kiểu `sql_identifier`/`name` không scan được vào `string`. `553c828` đã sửa bằng `::text`, run sau xanh |

## 2. Đánh giá theo hạng mục

**Migration SQL (`migrations/0002_outbox_relay.sql`)**
- Goose (cả `goose-docker:3.22.0` trong compose) mặc định chạy **mỗi chiều Up/Down của một file trong một transaction**, trừ khi
  file có `-- +goose NO TRANSACTION`, và file này không có. Goose tách câu lệnh theo `;` rồi `ExecContext` từng câu trong cùng tx,
  nên `SET LOCAL lock_timeout = '5s'` có hiệu lực cho các `ALTER` phía sau. `pgtest.Apply` chạy cả phần migration trong một tx
  tường minh. Pgx v5 dùng simple protocol khi không có tham số, nên chạy được nhiều câu trong một lần gửi. Test `55P03` trên CI
  chứng minh `lock_timeout` thực sự áp dụng.
- `gen_random_uuid()` là hàm volatile nên `ADD COLUMN event_id` **rewrite bảng** và giữ ACCESS EXCLUSIVE suốt quá trình
  rewrite cộng thời gian build index UNIQUE. `now()` là hàm STABLE nên `next_attempt_at` không gây rewrite. `lock_timeout` chỉ
  giới hạn thời gian **chờ** lấy khoá, không giới hạn thời gian giữ khoá. Điều này đã được ghi rõ trong docs/02 (kiểm
  `count(*)` trước, bảng lớn thì backfill theo lô), chấp nhận được.
- Down đúng thứ tự: bỏ index → dựng lại `idx_outbox_unpublished` y hệt 0001 → bỏ constraint → bỏ cột. Test AC-8 so cột, index và
  constraint qua `information_schema` / `pg_indexes` / `pg_constraint` với snapshot 0001, kết quả khớp. Up lần hai trên DB có dữ
  liệu cũng chạy được.
- Tương thích ngược: mọi cột mới đều có DEFAULT, nên INSERT kiểu cũ vẫn chạy (đã có test). Rollback production chỉ cần rollback
  ứng dụng.

**SQL của relay trên PG16 / pgx 5.9.2**: tất cả đã được kiểm bằng integration test thật trên CI.
- Claim `UPDATE … WHERE id IN (SELECT … ORDER BY next_attempt_at, id LIMIT $2 FOR UPDATE SKIP LOCKED) RETURNING …` hợp lệ. Trong
  READ COMMITTED, nếu một row vừa bị relay khác claim và commit, EvalPlanQual kiểm lại `next_attempt_at <= now()` và loại row đó.
  Test hai relay song song cho 0 bản trùng.
- `make_interval(secs => $1)` suy ra `$1` là `double precision`, `float64` encode đúng, kể cả ở `QueryExecModeCacheDescribe`.
  `= ANY($1)` suy ra `bigint[]`, `[]int64` encode đúng. Cột `jsonb` scan vào `*[]byte` cho ra JSON text (codec bỏ byte version).
  `'infinity'::timestamptz` bị loại bởi `<= now()`.
- Mọi câu đánh dấu đều có `AND published_at IS NULL` (`relay.go:222-226`). Câu trả lease cũng có điều kiện này.

**Đếm `attempts` so với §7 R1**: đúng cho nack, unroutable ngoài `DropUnroutable`, hết `PublishTimeout` khi kênh còn mở (mq
trả `cho confirm: context deadline exceeded` và chỉ trả `ErrClosed` khi `IsClosed()`), lỗi dựng message và panic. `ErrClosed`,
dial lỗi, huỷ ctx, hết `BatchDeadline` và lỗi DB khi đánh dấu đều trả lease, không tăng `attempts`.
- **Phần implementer thêm (nhánh `default` cũng tăng attempts, `relay.go:402`): chấp nhận.** Nếu không đếm, một row độc gây lỗi
  lạ sẽ bị gửi lại mãi mà không bao giờ bị đỗ, không có tín hiệu `outbox_dead`. Trong mq, lỗi kênh/kết nối đã được chuẩn hoá thành
  `ErrClosed` qua `p.ch.IsClosed()` (`publisher.go:124-149,196-199`), nên nhánh `default` thực tế chỉ còn lỗi gắn với từng
  message. Tuy vậy, docs chưa phản ánh nhánh này (xem m3).

**Lease, deadline, panic, backoff**
- Trả lease đúng: `resClosed` / `resCanceled` / hết deadline trả `rows[i:]`; lỗi đánh dấu trả `rows[i+1:]`, row `i` giữ lease
  rồi gửi lại khi lease hết hạn, đúng at-least-once.
- Đánh dấu và trả lease chạy dưới `WithoutCancel` với timeout 5s (`relay.go:460-465`), nên SIGTERM không làm mất dấu đã gửi.
- `recover` bao trọn `buildMessage` và `Publish` (`relay.go:374-378`).
- Backoff khớp r2: số mũ được kẹp trong [0, 20], có trần, jitter [0.8, 1.2], tối thiểu 1ns. Với MaxInt32 không bị tràn.
- Hạn chót của lô không bó publish đang chạy (xem m1).

**Kết nối và metric**
- Relay chỉ claim khi `r.pub != nil`. Redial lùi 1s→30s. Gauge `connected` về 0 khi dial lỗi hoặc gặp ErrClosed (xem m4 cho
  trường hợp kết nối chết lúc rảnh). `Run` chỉ trả `nil` khi ctx huỷ, và lúc đó đóng kết nối.
- Metric đăng ký qua `Registerer` bắt buộc, có gỡ đăng ký khi lỗi giữa chừng. Nhãn duy nhất là `routing_key`, tập hữu hạn vì
  `Write` kiểm regex. Gauge `pending` / `dead` đếm 15s một lần qua partial index.

**Write, phong bì, trace**
- `Write` kiểm `AggregateType` khác rỗng, `AggregateID` là UUID, routing key theo regex, `data` ≤ 64 KiB. `event_id` là UUIDv4 từ
  `crypto/rand`. Headers chỉ chứa `traceparent` / `tracestate` / `baggage`.
- Relay chuyển `map[string]string` thành `map[string]any` chỉ gồm giá trị string, nên luôn là `amqp.Table` hợp lệ.
- Phong bì dựng từ cột: `MessageId = event_id`, `Type = routing_key`, `Timestamp = created_at`.
- Trace: `ExtractAMQP` lấy context từ headers, rồi mq tạo span producer con và inject traceparent mới. AC5 trên CI chứng minh
  consumer có cùng trace ID.

**Ticketing**
- `PersistHold`: `hold_id` lấy từ `RETURNING id::text` (dạng chuẩn), `data` không có PII. `ReleaseHold`:
  `UPDATE seat_holds h … FROM orders o … RETURNING h.id::text, …, o.event_id::text` hợp lệ. Outbox được ghi trong cùng tx trước
  `Commit`. Lần gọi lặp lại trả `ErrNoRows`, nên không ghi thêm row. Sweeper đã bỏ publish thẳng và `port.Publisher` đã bị xoá.
- Worker:
  - `loadConfig` fail fast khi thiếu `AMQP_URL` hoặc URL sai cú pháp; thông báo lỗi không chứa URL. Lỗi từ `newPoolConfig`
    cũng không chứa DSN.
  - `redactErr` thay cả URL lẫn mật khẩu.
  - Dialer: lỗi ở bất kỳ bước nào đều đóng `conn`. Kênh topology đóng sau khi `Declare`. Kênh publish được tách riêng.
  - `runLoops`: vòng lặp đầu tiên trả về sẽ huỷ ctx chung, sau đó chờ tất cả các vòng lặp dừng.
  - Pool dùng `QueryExecModeCacheDescribe`, có comment giải thích lý do.
  - Khi tắt, `pool.Close` chỉ chạy sau khi mọi vòng lặp đã dừng.
- Compose: `ticketing-worker` có `AMQP_URL` và `depends_on: rabbitmq` với `service_healthy`.
- CI: đã thêm `go vet -tags=integration` cho ticketing và outbox, test outbox bọc bằng `run-and-report-tests.sh`.

**Bảo mật**
- Không có injection: mọi SQL đều dùng tham số. Tên DB trong `pgtest` được tạo từ pid và số thứ tự, đi qua
  `pgx.Identifier.Sanitize`.
- Không lộ `AMQP_URL` / `DATABASE_URL` trong log hay lỗi. `last_error` chỉ chứa thông điệp lỗi của mq (exchange và routing key),
  không có payload, đã được làm sạch UTF-8 / NUL và cắt ở 1 KiB.
- Không có PII trong `data`, test kiểm điều này bằng `assertKeys`.
- Giới hạn chống DoS: `data` ≤ 64 KiB, `BatchSize` ≤ 1000 (bộ nhớ tối đa khoảng 64 MiB mỗi lô), `MaxBackoff` ≤ 1h.
- Mật khẩu `eventflow` trong compose chỉ là credential dev, cùng kiểu với cấu hình đã có.

**Test**: đếm được 18 unit test cho relay/write và 8 cho worker, chạy trên fake. Fake DB có khớp chuỗi SQL, đây là change-detector
nhưng được integration thật bù lại. Không thấy tautology: các assert về attempts, lease, mark guard đều quan sát trạng thái của fake
chứ không quan sát code dưới test. 11 test tích hợp outbox cùng test tích hợp ticketing đều chạy thật trên CI. Timing của
integration ổn: dùng `eventually` (60–120s), lease 3s, chỉ có một `Sleep(3s)` mang tính tiền đề.

## 3. Findings

| # | Mức | Bằng chứng | Vấn đề | Sửa đề xuất |
|---|---|---|---|---|
| m1 | minor | `relay.go:104`, `248-259`, `384` | `BatchDeadline` chỉ được kiểm **giữa** các row; publish đang chạy dùng `PublishTimeout` riêng, chưa kể thêm đánh dấu / trả lease (5s mỗi việc). Một lô có thể kéo dài `BatchDeadline + PublishTimeout + 10s`, nhưng `NewRelay` chỉ kiểm `PublishTimeout ≤ BatchDeadline < Lease`. Một cấu hình hợp lệ như `Lease=31s, BatchDeadline=30s, PublishTimeout=10s` có thể làm lô vượt lease: relay khác claim lại sẽ gây trùng; nếu relay khác đã đỗ row thì `releaseSQL` của relay chậm có thể đặt lại `next_attempt_at = now()`, tức là gỡ đỗ ngoài ý muốn. Cấu hình mặc định (45s < 60s) và cấu hình ticketing an toàn. | Bó ctx publish bằng `min(PublishTimeout, deadline)` (`context.WithDeadline`), và/hoặc kiểm `BatchDeadline + PublishTimeout + 2*markTimeout < Lease` trong `applyDefaults`. |
| m2 | minor | `relay.go:400-403`; `cmd/worker/main.go` `amqpDialer` | Khi RabbitMQ bật alarm bộ nhớ/đĩa (`connection.blocked`) hoặc TCP bị blackhole trước khi heartbeat phát hiện, mọi publish đều hết timeout và bị **tính attempts** (đúng R1). Nếu alarm kéo dài khoảng ≥1h thì `hold.released` bị **đỗ** hàng loạt và phải gỡ tay theo runbook. Event không mất, nhưng phải can thiệp vận hành. | Đăng ký `conn.NotifyBlocked` trong dialer; khi đang bị blocked thì coi như chưa kết nối (không claim) hoặc phân loại timeout thành `resClosed`. Tối thiểu, ghi rủi ro này vào docs/02 cạnh runbook. |
| m3 | minor | `relay.go:402`; `doc.go:24-27`; `docs/02-kien-truc.md` (mục "Lỗi và đỗ row"); `plan.md` §7 R1 | Nhánh `default` (lỗi publish không phân loại được) tăng `attempts`. Hướng này đúng (tránh vòng lặp vô hạn không có tín hiệu), nhưng doc.go và docs/02 nói "**chỉ**" nack / unroutable / timeout / lỗi dựng message làm tăng attempts, tức là lệch với code. Chưa có unit test riêng cho một lỗi không phân loại (test timeout có đi qua nhánh này nhưng với lý do khác). | Cập nhật doc.go và docs/02 (thêm "lỗi publish khác khi kênh còn mở"). Ghi lệch so với R1 vào traceability. Thêm test lỗi tuỳ ý ⇒ `attempts+1`, không đóng kết nối. |
| m4 | minor | `relay.go:302-303`, `307-313` | `outbox_relay_connected` chỉ về 0 khi dial lỗi hoặc publish gặp ErrClosed. Nếu kết nối chết lúc relay đang rảnh (không có row), gauge vẫn là 1 cho tới lô tiếp theo có row, khiến cảnh báo theo gauge bị trễ hoặc sai. | Trong dialer, lắng nghe `conn.NotifyClose` (truyền vào qua `Closer` hoặc callback) để đặt gauge về 0 và xoá `pub`. Cách khác: kiểm `IsClosed()` trước khi claim. |
| m5 | minor | `integration_test.go:427-431` | Test "đóng cứng AMQP giữa lô" gọi `killTCP()` rồi `cancel1()` ngay trong cùng callback. Row tiếp theo vì thế thấy ctx đã huỷ trước khi publish, nên nhánh `ErrClosed` → `closeConn` → trả lease không được test này kiểm (plan §6.6: "không chỉ huỷ ctx"). Nhánh đó vẫn được phủ bởi test `stop_app` và unit test. | Thêm biến thể chỉ `killTCP()` không huỷ ctx: assert relay tự dial lại, `attempts` = 0, mọi `event_id` tới queue. |
| m6 | minor | `.github/workflows/integration.yml:37-39` | Bước `go test -tags=integration` của ticketing (có test tích hợp mới của EVF-36) không có `-v` và không bọc bằng `run-and-report-tests.sh`, nên nếu toàn bộ test bị skip thì bước này vẫn xanh. Hiện `pgtest` dùng `t.Fatal` khi thiếu Docker nên rủi ro thấp; package mất 8.1s cho thấy test chạy thật. | Bọc riêng `./internal/adapter/postgres/...` bằng `run-and-report-tests.sh TestTichHop` (`-v`). |
| m7 | nit | `relay.go:211-218` | Mẫu `WHERE id IN (SELECT … LIMIT … FOR UPDATE SKIP LOCKED)` phụ thuộc vào việc planner chỉ đánh giá subquery một lần. Hiện đúng như vậy (subquery có LIMIT / FOR UPDATE không bị kéo lên), CI không thấy trùng. | Có thể làm chắc hơn bằng `WITH c AS (SELECT … FOR UPDATE SKIP LOCKED) UPDATE … FROM c WHERE o.id = c.id`. Không bắt buộc. |
| m8 | nit | `repository.go:268`; `relay.go:264-266` | `released_at` trong `data` lấy theo đồng hồ Go, không phải `now()` của transaction (lệch nhỏ so với `seat_holds.released_at`). Metric `published` / `lag` được ghi nhận trước khi biết đánh dấu thành công hay không (lỗi đánh dấu sẽ gửi lại và đếm hai lần). | Lấy `released_at` qua `RETURNING h.released_at`. Ghi metric sau khi `markErr == nil`. |
| m9 | info | `migrations/0002_outbox_relay.sql`; `plan.md` §7 r9, r10; `cmd/server` | (a) CI không chạy binary goose thật: rủi ro đã ghi ở r9, cần kiểm bằng `make migrate` khi có Docker (smoke compose). (b) Thiếu `approval.md`: **không merge** khi người dùng chưa duyệt (r10). (c) Pool của `cmd/server` (giờ cũng gọi `outbox.Write` qua PgBouncer) vẫn dùng chế độ exec mặc định. Đây là hiện trạng từ trước, ngoài phạm vi, nên kiểm trong smoke compose với `pgbouncer:latest`. | Theo dõi khi smoke và duyệt. |

## 4. Ghi chú cho bước tiếp theo

- m1 và m3 nên sửa trước khi merge vì rẻ và đúng tinh thần r1 / R1. m2 và m4 có thể tách ticket vận hành.
- Sau khi người dùng duyệt migration: tạo `approval.md` (ai, khi nào, SHA), chạy smoke compose (`make migrate` bằng goose thật,
  worker qua PgBouncer với `CacheDescribe`).
