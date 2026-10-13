# EVF-36 — Review kế hoạch (plan review + review bảo mật)

| Mục | Giá trị |
|---|---|
| Đầu vào | `spec.md` (AC-1..AC-9), `plan.md` — worktree `EvenFlow-outbox`, nhánh `feature/EVF-36-outbox-relay` trên EVF-7 (`407f599`) |
| Người review | Reviewer độc lập (context sạch), kiêm review bảo mật vì tier **High-risk** |
| Đã đối chiếu | `migrations/0001_core.sql`, `services/ticketing/**` (repository, hold_sweeper, ports, create_hold, cmd/worker, cmd/server, go.mod, test/), `libs/go/mq` (publisher, topology, trace, doc), `libs/go/otelx/amqp.go`, `deploy/compose/docker-compose.yml`, `deploy/docker/ticketing.Dockerfile`, `go.work`, `.github/workflows/{integration,ci}.yml`, `docs/02`, `docs/03 §7`, `docs/06`, `docs/09 W6`, mã nguồn `amqp091-go@v1.15.0`, module cache cục bộ |

## Kết luận: **FAIL** — cần plan v2 rồi review lại

Hướng tổng thể đúng (outbox cùng transaction, poll + `SKIP LOCKED` vì PgBouncer transaction mode, at-least-once, `MessageId = event_id`).
Nhưng có 2 blocker: (B1) thiết kế "relay lỗi kênh ⇒ worker thoát" làm **sweeper chết theo RabbitMQ** — vé bị khoá suốt
thời gian broker sập; (B2) migration — thứ người dùng phải duyệt — chưa được viết đầy đủ (Down chỉ là một dòng chú thích) và
kế hoạch rollback trái quy ước forward-only. Thêm 9 lỗi major về tính đúng của relay, dữ liệu cũ, và khả năng test.

## Độ phủ AC

| AC | Plan có cách làm | Test được? | Ghi chú |
|---|---|---|---|
| AC-1 | Có | **Chưa** | Ticketing chưa có harness Postgres (`test/concurrency/helpers.go:31-34` còn `t.Skip`); "hoặc fake" không chứng minh được *cùng transaction* → M8 |
| AC-2 | Có | Có | Row cũ (trước 0002) bị publish **không có phong bì** → M3 |
| AC-3 | Có | Có | Lỗi DB giữa lô làm tx bị huỷ nhưng vòng lặp vẫn publish → M1; tràn số backoff → m1 |
| AC-4 | Có | Một phần | "Giết" relay bằng huỷ ctx = rollback êm, chưa giống chết thật → m11. Kịch bản RabbitMQ stop/start mâu thuẫn với B1 |
| AC-5 | Có | Có | Header sau JSON round-trip phải toàn chuỗi → M4 |
| AC-6 | Có | Có | Đánh dấu published cho **mọi** key unroutable = mất event lặng lẽ → M5 |
| AC-7 | Có | Có | Fail-fast khi *dial* lỗi là sai (B1); metric thiếu nhãn/registry → m4 |
| AC-8 | Có | Một phần | "Down trả đúng schema 0001" cần so schema thật (vd. `pg_dump -s` / `information_schema`), không chỉ "chạy được"; Down chưa viết → B2 |
| AC-9 | Có | Có | Module mới `testcontainers .../modules/postgres` không có trong cache → M8; ảnh hưởng workspace → m8 |

## Phát hiện

### Blocker

**B1 — RabbitMQ sập kéo sweeper chết theo; vé bị khoá.**
Bằng chứng: plan §1.3 dòng 151-153 (worker dial AMQP lúc khởi động, "relay trả lỗi kênh ⇒ dừng worker"); `cmd/worker/main.go:99-108`
(goroutine đầu tiên trả về là `stop()` mọi thứ); sweeper là con đường **duy nhất** nhả hold hết hạn (`hold_sweeper.go:12-22`,
`docs/02:160-173`). Khi broker ngừng: worker thoát → compose `restart: unless-stopped` (`docker-compose.yml:189`) khởi động lại
→ dial thất bại → thoát lại (crash loop) → **không hold nào được nhả** trong suốt sự cố → hệ thống báo hết vé giả giữa đợt mở bán.
Đây đúng là sự phụ thuộc mà outbox sinh ra để cắt bỏ, và đi ngược kịch bản `docs/09:88`.
**Sửa plan:** relay tự giám sát kết nối: `RelayRunner` (trong ticketing hoặc `libs/go/outbox`) lặp `dial → mq.Declare →
mq.NewPublisher → relay.Run` cho tới khi `ErrClosed`/`NotifyClose`, đóng, chờ backoff có trần (vd. 1s→30s + jitter) rồi dial lại;
**không bao giờ** trả lỗi ra `run()` trừ khi ctx huỷ. Fail-fast chỉ cho cấu hình sai (thiếu/parse lỗi `AMQP_URL`). Thêm gauge
`outbox_relay_connected`. Giữ `depends_on: rabbitmq healthy` cho tiện khởi động nhưng không dựa vào nó. Sửa AC-7 trong spec tương
ứng và thêm test: broker dừng ⇒ sweeper vẫn nhả hold (unit với fake dialer luôn lỗi). Sửa luôn bảng rủi ro dòng 184.

**B2 — Migration chưa đủ để người dùng duyệt; rollback trái quy ước.**
Bằng chứng: plan dòng 87-88 Down chỉ là chú thích; spec dòng 7 yêu cầu người dùng duyệt migration; `docs/03 §7` ghi "Migration
`goose`, chỉ tiến (forward-only) trên production"; plan §5 dòng 188 lại dùng `goose down` làm rollback và không nói thứ tự
(app mới ghi cột `event_id`/`headers` → Down trước khi rút app là vỡ `PersistHold`).
**Sửa plan:** (a) viết **đầy đủ** SQL Up và Down trong plan (Down: `DROP INDEX IF EXISTS idx_outbox_due; CREATE INDEX
idx_outbox_unpublished ON outbox_events (id) WHERE published_at IS NULL; ALTER TABLE outbox_events DROP CONSTRAINT
outbox_events_event_id_key, DROP COLUMN next_attempt_at, DROP COLUMN last_error, DROP COLUMN attempts, DROP COLUMN headers,
DROP COLUMN event_id;`). (b) Rollback production = **chỉ rút app** (schema 0002 tương thích ngược: mọi cột mới có DEFAULT/NULL,
code cũ `INSERT` không liệt kê cột mới vẫn chạy); Down chỉ dùng dev/test, và nếu chạy thì *sau* khi đã rút app + dừng
`ticketing-worker`. (c) Thêm mốc **người dùng ký duyệt** rõ ràng: sau khi viết migration + test AC-8 xanh, trước khi mở PR
(ghi lại trong `docs/workflow/EVF-36-outbox-relay/` — ai, lúc nào, commit nào). (d) Thêm bước **diễn tập rollback**: trên DB đã có
dữ liệu, `goose up` 0002 → chạy đúng câu `INSERT` cũ của `repository.go:187-189` (code trước EVF-36) phải thành công → `goose down`
→ `goose up`, so schema.

### Major

**M1 — Lỗi DB giữa lô: transaction đã hỏng mà relay vẫn publish; huỷ ctx khi tắt làm mất toàn bộ đánh dấu.**
Bằng chứng: plan dòng 132-138 chỉ dừng lô với `ErrClosed`. Trong Postgres, một câu lỗi (vd. `UPDATE` vi phạm, lỗi mã hoá ở M2)
đưa tx vào trạng thái *aborted* (SQLSTATE 25P02): mọi `UPDATE` sau đều lỗi, `COMMIT` thành `ROLLBACK` → các message đã publish
không được đánh dấu → lô sau publish lại **cả lô** → lặp vô hạn nếu lỗi tái diễn. Ngoài ra khi SIGTERM, ctx của relay bị huỷ →
`UPDATE`/`COMMIT` bằng ctx đó thất bại → mất đánh dấu của các message đã gửi.
**Sửa plan:** lỗi DB bất kỳ trong lô ⇒ dừng publish ngay, rollback, trả lỗi (Run lùi rồi thử lại). `UPDATE`/`COMMIT` dùng
`context.WithoutCancel(ctx)` + timeout ngắn (vd. 5s). Lỗi `context.Canceled` khi publish do tắt máy **không** tính `attempts`.
Unit test: fake Tx trả lỗi ở UPDATE thứ 2 ⇒ không có Publish nào sau đó; ctx huỷ giữa lô ⇒ các row đã publish vẫn được commit.

**M2 — `last_error` không được làm sạch: NUL / UTF-8 hỏng / dài vô hạn.**
Bằng chứng: plan dòng 136 `last_error = $err`. Chuỗi lỗi có byte `0x00` hoặc UTF-8 không hợp lệ làm Postgres từ chối
(`invalid byte sequence for encoding "UTF8"`) → kích hoạt M1. Không giới hạn độ dài → phình bảng.
**Sửa plan:** `sanitizeErr`: `strings.ToValidUTF8`, bỏ `\x00`, cắt ≤ 1 KiB; chỉ chứa `err.Error()` (không chứa payload/URL).
Unit test với chuỗi có NUL và byte rác.

**M3 — Row có sẵn trước 0002 bị publish sai định dạng và sai ngữ nghĩa `event_id`.**
Bằng chứng: plan dòng 106-107 dựng phong bì **lúc `Write`** và lưu vào `payload`; relay gửi `payload` làm body (dòng 134).
Row do code hiện tại ghi (`repository.go:181-190`) có payload thô, trong đó `"event_id"` = id **sự kiện hoà nhạc** (O4). Sau
migration các row này nhận `event_id` UUID mới ở cột nhưng body vẫn là dữ liệu thô: consumer khử trùng theo `event_id` trong
body sẽ gộp **mọi** hold của cùng một buổi diễn thành một.
**Sửa plan:** `payload` chỉ chứa `data`; relay **dựng phong bì lúc publish** từ cột (`event_id`, `created_at` → `occurred_at`,
`trace_id`, `routing_key` → `type`, `schema_version` hằng số, `payload` → `data`). Row cũ tự được bọc đúng. Thêm test tích hợp:
chèn row kiểu 0001 trước khi chạy Up 0002 → relay publish ra phong bì hợp lệ với `event_id` = cột.

**M4 — Header lưu JSONB: giá trị không phải chuỗi làm publish lỗi vĩnh viễn (poison row).**
Bằng chứng: `amqp091-go@v1.15.0/types.go:416-438` — `validateField` chỉ nhận `Table`/`[]any`/scalar, **không** nhận
`map[string]any` (đối tượng JSON lồng nhau sau `json.Unmarshal`), và `channel.go:1902` validate header trước khi gửi. Lỗi đó
không phải `ErrClosed`/`ErrUnroutable` → plan coi là lỗi tạm → thử lại mãi mãi mỗi 5 phút. (Số JSON thành `float64` thì AMQP
nhận được, nhưng đổi kiểu so với lúc ghi.)
**Sửa plan:** kiểu lưu là `map[string]string` (đúng thứ `TextMapCarrier` sinh ra); đọc ra bằng `map[string]string`, chuyển sang
`map[string]any` chỉ chứa chuỗi. Thêm chính sách poison: `attempts >= MaxAttempts` (vd. 20) ⇒ ngừng thử (vd. `next_attempt_at =
'infinity'`, giữ `published_at IS NULL`), metric `outbox_dead_total` + log ERROR để vận hành xử lý tay. Ghi rõ trong spec AC-3.

**M5 — "Unroutable ⇒ đánh dấu đã gửi" áp cho mọi routing key = mất event lặng lẽ.**
Bằng chứng: spec AC-6 dòng 49-50, plan dòng 135-136. Nếu binding `ticketing.hold.released` bị xoá/lệch (ops, đổi
`definitions.json`), mọi `hold.released` bị đánh dấu published và **mất** — trái AC-4. Hàng `ticketing.hold.released.q` hiện
cũng chưa có consumer (`topology.go:90-98`).
**Sửa plan:** `RelayConfig.DiscardUnroutable` = tập routing key *được phép bỏ* (ban đầu `{"ticketing.hold.created"}`); key khác
gặp `ErrUnroutable` ⇒ xử lý như lỗi tạm (backoff + `outbox_unroutable_total{routing_key}` + log WARN) để sửa binding là gửi lại
được. Test cả hai nhánh.

**M6 — Giữ transaction suốt lúc publish qua mạng: thời lượng không chặn trên.**
Bằng chứng: plan dòng 121-124 (mặc định 100 row × `PublishTimeout` 10s = tối đa ~1000s một tx). `mq.Publisher` giữ mutex và chờ
confirm tuần tự (`publisher.go:56-62,121`); khi broker bật *flow control/connection.blocked*, lệnh ghi của amqp091 có thể chặn
không theo ctx. Hệ quả: tx dài giữ xmin horizon → VACUUM không dọn được `seat_holds`/`ticket_inventory`/`orders` (bảng nóng
nhất đợt mở bán), ghim một server connection PgBouncer, và chặn migration (M7).
**Sửa plan:** chọn một: (a) giữ thiết kế nhưng thêm **hạn cho cả lô** (vd. 15s: hết hạn thì dừng publish, commit phần đã xong),
`BatchSize` mặc định 50, `SET LOCAL idle_in_transaction_session_timeout = '60s'` và `statement_timeout` trong tx (hợp lệ ở
PgBouncer transaction mode); hoặc (b) *lease*: tx ngắn `UPDATE ... SET next_attempt_at = now() + lease ... WHERE id IN (SELECT
... FOR UPDATE SKIP LOCKED) RETURNING ...`, publish ngoài tx, tx ngắn đánh dấu. Ghi lựa chọn vào plan và test "lô bị cắt theo
hạn".

**M7 — Khoá khi migration và hàng đợi khoá.**
Bằng chứng: plan dòng 78-86. Trên PG16: `DEFAULT gen_random_uuid()` là hàm *volatile* ⇒ `ALTER TABLE` **ghi lại toàn bảng** (và
dựng lại index) dưới khoá `ACCESS EXCLUSIVE` (`now()` là *stable* nên một mình nó chỉ là fast default; nhưng chung câu ALTER thì
cả câu vẫn rewrite). `ADD CONSTRAINT UNIQUE`, `DROP INDEX` cũng `ACCESS EXCLUSIVE`; goose chạy cả Up trong **một** tx nên khoá
giữ tới commit. `outbox_events` không bao giờ được dọn (`docs/09:100` để GĐ 2) nên càng về sau càng lớn. Nếu có relay đang giữ
`FOR UPDATE` (M6), migration xếp hàng sau nó và **mọi** `PersistHold` xếp hàng sau migration ⇒ ngừng tạo hold.
**Sửa plan:** đầu Up/Down thêm `SET LOCAL lock_timeout = '5s';` (fail nhanh, chạy lại); ghi thứ tự triển khai: dừng
`ticketing-worker` trước khi Down/Up lại; ghi rõ chấp nhận rewrite vì bảng nhỏ ở môi trường hiện tại (kèm cách kiểm tra
`SELECT count(*)` trước khi chạy), và phương án cho bảng lớn (cột nullable → backfill theo lô → `SET DEFAULT` → `CHECK NOT VALID`
+ `VALIDATE` → `CREATE UNIQUE INDEX CONCURRENTLY` với `-- +goose NO TRANSACTION`) — chưa cần làm nhưng phải có trong rủi ro.

**M8 — Hạ tầng test cho AC-1/AC-8 chưa khả thi như viết.**
Bằng chứng: `testcontainers-go/modules/postgres v0.44.0` **không** có trong module cache cục bộ (chỉ có `modules/rabbitmq`), và
proxy đang lỗi DNS ⇒ không `go get` được trên máy; `libs/go/go.mod` chưa có module này. Ticketing không có harness Postgres
(`test/concurrency/helpers.go:31-34`) và không dùng lại được helper nằm trong `libs/go/outbox` (package test). Plan dòng 164-165
"dùng container ở trên **hoặc fake**" — fake không chứng minh được "cùng transaction / rollback không có row".
**Sửa plan:** dùng `testcontainers.GenericContainer` lõi (đã có) với `postgres:16-alpine` + `wait.ForLog("database system is ready
to accept connections").WithOccurrence(2)`; đặt helper khởi động Postgres + áp migration ở nơi cả hai dùng được (vd.
`libs/go/outbox/outboxtest` không có build tag ẩn, hoặc chép tối thiểu sang `services/ticketing/internal/adapter/postgres`
với `//go:build integration`). Bắt buộc test tích hợp ticketing: `PersistHold` thành công ⇒ đúng 1 row `ticketing.hold.created`
(`data.sale_event_id`, không có `identity_id`); vi phạm `uq_active_order_per_identity_event` ⇒ 0 row; `ReleaseHold` hai lần ⇒
đúng 1 row `hold.released`.

**M9 — Thứ tự không được định nghĩa; câu SELECT không khớp index.**
Bằng chứng: plan dòng 132-133 `ORDER BY id` + backoff theo từng row ⇒ `hold.released` của một order có thể đi **trước**
`hold.created` của chính order đó (created đang backoff). `id` BIGSERIAL cũng không phải thứ tự commit. Index mới `(next_attempt_at,
id)` không phục vụ `ORDER BY id` ⇒ khi tồn đọng lớn (sau sự cố broker) mỗi lô phải sort toàn bộ row đến hạn.
**Sửa plan:** spec ghi rõ "không bảo đảm thứ tự, kể cả theo aggregate; consumer dùng `occurred_at`/trạng thái aggregate" (hoặc
chọn chặn theo aggregate — đắt hơn, không khuyến nghị lúc này). Đổi thành `ORDER BY next_attempt_at, id` cho khớp index.

### Minor

- **m1 — Tràn số backoff.** `Base*2^(attempts-1)` với Base=1s tràn `int64` ns từ `attempts ≥ 35` → âm → `min` trả âm → hot loop.
  Kẹp số mũ (vd. `if attempts > 30 → Max`), thêm jitter; test `attempts` = 0, 1, 35, 64, `MaxInt32`.
- **m2 — Hợp đồng dữ liệu chưa chốt.** `ReleaseHold(holdID)` không biết `sale_event_id` (`repository.go:222-226`): lấy qua
  `UPDATE seat_holds h ... FROM orders o WHERE o.id = h.order_id ... RETURNING o.event_id`. Chốt `AggregateType/ID` cho
  `hold.released` (đề xuất `order`/`order_id` cho nhất quán) và bỏ dấu `hold_id?` ở dòng 147. Cấm `identity_id`/PII trong `data`.
- **m3 — Ngữ nghĩa `hold.released` thay đổi.** Trước: chỉ phát khi cả Postgres và Redis xong (`hold_sweeper.go:107-117`). Sau:
  phát khi Postgres commit, kể cả khi Redis nhả lỗi. Cửa sổ crash an toàn (lần quét sau `ReleaseHold` trả `false` nên không ghi trùng
  event; Redis được nhả ở lần quét sau) nhưng phải ghi vào `docs/02:173` rằng event nghĩa là "đã nhả ở nguồn sự thật".
- **m4 — Metric.** Bắt buộc truyền `prometheus.Registerer` (thư viện không tự đăng ký vào `DefaultRegisterer` → panic trùng đăng ký
  trong test/hai relay); nhãn chỉ `routing_key` (tập đóng), **không** dùng chuỗi lỗi/`event_id`; `outbox_pending` đếm định kỳ (vd.
  15s) chứ không mỗi lô; ghi bucket cho histogram lag.
- **m5 — Bí mật trong log.** Không log `AMQP_URL`/`DATABASE_URL`; nếu cần log đích dùng `(*url.URL).Redacted()`. `last_error` không chứa
  URL/payload (xem M2).
- **m6 — Kiểm tra đầu vào / chống DoS.** `Write`: routing key theo `^[a-z]+\.[a-z_]+\.[a-z_]+$`, `AggregateID` là UUID, giới hạn
  kích thước `data` (vd. 256 KiB) để lỗi ngay lúc ghi thay vì thành poison. `NewRelay`: `1 ≤ BatchSize ≤ 1000`, các duration > 0,
  `Base ≤ Max`. SQL đều tham số hoá — không thấy bề mặt injection.
- **m7 — Danh sách file được phép thiếu.** Thêm `docs/06-Thiet_ke_co_ban.md` (sơ đồ ER `outbox_events`, dòng 184-190) và
  `go.work.sum` (nếu Go đòi checksum mới). `docs/03:71` đặt relay ở `worker/outbox_relay.go` trong khi plan đặt ở `libs/go/outbox` —
  ghi rõ phần cập nhật `docs/03 §2/§3`.
- **m8 — `libs/go` thêm phụ thuộc pgx.** `Execer` đã trả `pgconn.CommandTag` nên câu "không kéo pgx vào API" (dòng 95) không đúng — sửa
  chú thích. Cả 8 service phụ thuộc `libs/go` (workspace `go.work`) thấy pgx trong đồ thị module; pgx v5.9.2 có trong cache. Cổng
  kiểm: vòng `go vet` 9 module như `ci.yml:93-100` + `docker build` ticketing và waitingroom (Dockerfile copy cả `libs/`).
- **m9 — PgBouncer.** Test tích hợp nối thẳng Postgres nên không phát hiện vấn đề PgBouncer. pgx mặc định
  `QueryExecModeCacheStatement` (prepared statement có tên) đi qua `edoburu/pgbouncer:latest` (không pin) phụ thuộc
  `max_prepared_statements`; transaction tường minh của relay giữ mọi câu trên một server connection nên không phát sinh rủi ro mới —
  ghi giả định này vào plan; tuỳ chọn smoke qua compose.
- **m10 — Bước CI.** Đặt tên test `TestTichHop*` để dùng `run-and-report-tests.sh` như bước `mq` (`integration.yml:47-55`), thêm
  `go vet -tags=integration ./outbox/...`. Cân nhắc chạy goose thật (`kukymbr/goose-docker:3.22.0`, cùng ảnh compose dòng 115)
  up→down→up để kiểm chú thích goose — test tự tách file SQL không chứng minh goose đọc đúng.
- **m11 — Độ trung thực của kill-restart.** Huỷ ctx = rollback êm. Thêm biến thể đóng cứng kết nối (vd. `conn.PgConn().Close` /
  `pg_terminate_backend`) giữa publish và commit; khẳng định mỗi `event_id` tới ≥ 1 lần và số bản trùng ≤ kích thước lô.
- **m12 — Thông lượng.** `mq.Publisher` chỉ một publish đang chờ confirm (mutex) ⇒ vài trăm–nghìn msg/s mỗi relay; đợt mở bán có thể
  tồn đọng. Ghi vào rủi ro, theo dõi bằng histogram lag.
- **m13 — `ticketing.hold.released.q` chưa có consumer** ⇒ hàng quorum phình mãi. Ngoài phạm vi, nhưng ghi vào rủi ro/việc sau.

## Việc cần làm cho plan v2

1. B1: runner tự dial lại; sweeper độc lập với broker; sửa AC-7.
2. B2: SQL Up/Down đầy đủ, rollback = rút app, mốc ký duyệt + diễn tập rollback.
3. M1–M9 như trên; cập nhật spec (AC-3 poison/MaxAttempts, AC-6 allowlist, thứ tự không bảo đảm).
4. Minor: đưa vào plan hoặc ghi lý do bỏ qua.

## Vòng 2 (2026-10-13) — review `plan.md` §6 "Kế hoạch v2" (dòng 125–235)

### Kết luận: **PASS WITH CHANGES**

§6 xử lý được cả hai blocker và cả chín major của vòng 1. Còn lại 2 major mới/sót, đều là chỗ đặc tả chưa đủ chứ không phải sai thiết
kế, và một số minor. Sửa thẳng trong plan. Không cần review vòng 3 nếu chỉ sửa đúng các mục dưới; bước review code sẽ đối chiếu lại.

### Đối chiếu phát hiện vòng 1

| Mã | Trạng thái | Ghi chú (dòng `plan.md`) |
|---|---|---|
| B1 | Đã sửa | 186–193: relay giữ kết nối, redial 1s→30s + jitter, worker không thoát, fail fast chỉ khi URL sai. Thiếu test → r6 |
| B2 | Đã sửa | 129–168: SQL Up/Down đầy đủ, `lock_timeout`, rollback = rút app, diễn tập, điểm duyệt. Ghi nhận duyệt → r10 |
| M1 | Đã sửa | 179–181: đánh dấu autocommit dưới `WithoutCancel` + 5s, lỗi DB ⇒ dừng lô |
| M2 | Đã sửa | 201 |
| M3 | Đã sửa | 197–198: phong bì dựng từ cột |
| M4 | Đã sửa | 199–203: `map[string]string`, `MaxAttempts` + đỗ row. Chưa nói lỗi nào được tính `attempts` → **R1** |
| M5 | Đã sửa | 204–205: `DropUnroutable` theo danh sách |
| M6 | Đã sửa | 170–180: lease, publish ngoài transaction. Bất biến thời gian → r1 |
| M7 | Đã sửa | 135, 147, 162–164 |
| M8 | Đã sửa | 225–228: `GenericContainer`, test ticketing thật. Vị trí harness → r5 |
| M9 | Đã sửa | 175 `ORDER BY next_attempt_at, id` khớp index; 182–183 thứ tự không bảo đảm |
| m1 | Đã sửa | 206 (còn số mũ âm → r2) |
| m2 | Đã sửa | 209–211 (dạng UUID của hold_id → r3) |
| m3–m7, m12, m13 | Đã sửa | 213–221, 235 |
| m8 | Đã sửa | 234 |
| m9 | Ghi nhận | 232–233; nặng hơn vì đánh dấu giờ là autocommit → r8 |
| m10 | Một phần | 231 có `TestTichHop` + vet; không chạy goose thật → r9 (chấp nhận được) |
| m11 | Một phần | 229–230 chỉ giết phía AMQP, không phủ "publish xong mà chưa đánh dấu" → **R2** |

### Phát hiện còn lại / mới

#### Major

**R1 — Chưa định nghĩa kết quả nào tăng `attempts`; row bị đỗ thì không có cách đưa lại.**
Bằng chứng: `plan.md:202–205` (`MaxAttempts` 20 ⇒ `next_attempt_at = 'infinity'`), `plan.md:188–189` (ErrClosed ⇒ dial lại), §1.2
dòng 70 ("lỗi khác ⇒ attempts+1"). Với backoff 1s·2^n trần 5m, 20 lần ≈ 1 giờ. Nếu các trường hợp sau đều bị tính `attempts`: ErrClosed,
huỷ ctx khi SIGTERM, row chưa kịp gửi khi lô hết hạn, timeout confirm lúc kết nối nửa sống, thì một sự cố broker/mạng khoảng 1 giờ sẽ
đỗ vĩnh viễn các event `hold.released`. Với consumer, như vậy là **mất event** (trái AC-4). `outbox_dead_total` là counter, về 0 khi
khởi động lại, nên không dùng làm alert bền được.
**Sửa plan:** (a) Chỉ tăng `attempts` với `ErrNacked`, `ErrUnroutable` (key ngoài danh sách), timeout confirm khi kênh vẫn mở, và lỗi
validate. **Không** tăng với `ErrClosed`, `context.Canceled`, lỗi dial, hay row chưa xử lý khi lô dừng sớm. (b) Khi lô dừng sớm (vì
ErrClosed, hết hạn hoặc lỗi DB), cố gắng trả lease các row chưa xử lý: `UPDATE … SET next_attempt_at = now() WHERE id = ANY($1) AND
published_at IS NULL`. (c) Relay chỉ claim khi đang có kết nối. (d) Thêm gauge `outbox_dead` (đếm row `'infinity'`, cập nhật cùng
`outbox_pending`), và ghi runbook đưa lại vào docs: `UPDATE outbox_events SET attempts = 0, next_attempt_at = now() WHERE
next_attempt_at = 'infinity' AND published_at IS NULL [AND routing_key = …]`. Unit test cho từng lớp lỗi.

**R2 — AC-4 "publish xong mà chưa đánh dấu" chưa có test.**
Bằng chứng: spec AC-4 dòng 41–43 ("kể cả sau publish mà trước commit đánh dấu"). `plan.md:229–230` chỉ đóng cứng kết nối **AMQP**:
khi đó publish thất bại, nên test không bao giờ đạt trạng thái "broker đã nhận nhưng DB chưa ghi". Bản §1.2 cũ có "Publisher bọc chết
sau k lần", nhưng §6 đã thay thế nó.
**Sửa plan:** thêm kịch bản phía DB. Sau k lần publish thành công, chạy `pg_terminate_backend` trên các kết nối của pool relay (hoặc dùng
DB bọc làm câu `UPDATE` đánh dấu trả lỗi), rồi dừng relay. Chạy relay mới với lease ngắn và khẳng định: mọi `event_id` tới queue ≥ 1
lần, mọi row có `published_at`, số bản trùng ≤ kích thước lô.

#### Minor

- **r1 — Bất biến thời gian của lease.** Hạn chót cả lô chưa có giá trị (`plan.md:178`). Cần `NewRelay` kiểm `PublishTimeout ≤
  BatchDeadline < Lease` (vd. 10s ≤ 30s < 60s), nếu không relay khác có thể claim lại trong khi bản cũ vẫn đang gửi. Câu đánh dấu thêm
  `AND published_at IS NULL` để đánh dấu lỗi đến muộn không ghi `attempts`/`last_error` lên row đã gửi.
- **r2 — Số mũ âm trong backoff.** `2^min(attempts-1, 20)` với `attempts = 0` cho số mũ -1, và phép dịch bit với số âm trong Go
  **panic**. Dùng `max(attempts-1, 0)`, kiểm `0 < Base ≤ Max ≤ 1h` (để Base·2^20 không tràn), test `attempts` = 0, 1, 21, `MaxInt32`.
- **r3 — Dạng UUID của `hold_id`.** HoldID của Redis có thể là 32 hex không gạch (`repository.go:275–280`, hàm `toUUID`); nếu `Write`
  kiểm UUID bằng regex có gạch thì `hold.released` sẽ lỗi. Dùng `RETURNING h.id::text` (dạng chuẩn) cho cả `aggregate_id` và
  `data.hold_id`, khớp `PersistHold` (`out.HoldID` cũng lấy từ `RETURNING id::text`).
- **r4 — AC-8 chưa so schema.** `plan.md:165–166` chỉ kiểm "Down rồi Up thành công", trong khi spec đòi "Down trả lại đúng schema 0001".
  Cần so `information_schema.columns` + `pg_indexes` + `pg_constraint` của `outbox_events` trước Up và sau Down. Trong test, chạy Up/Down
  trong transaction tường minh như goose; `SET LOCAL` ngoài transaction chỉ sinh WARNING và không có tác dụng.
- **r5 — Chưa chốt vị trí harness Postgres dùng chung** (`plan.md:227–228`). Service không import được package `internal` của `libs/go`.
  Đề xuất `libs/go/testx/pgtest` (đúng tên `testx` trong `docs/09` W3), chỉ import từ file `_test`. Khi đó `go.mod` của ticketing thêm
  require testcontainers (được phép theo §3).
- **r6 — Thiếu test cho B1.** Unit test: dialer luôn lỗi ⇒ `Relay.Run` không trả về cho tới khi ctx huỷ, gauge `connected = 0`, không
  claim row. Thêm test `run()` của worker (hoặc tách hàm) để chắc rằng relay không bao giờ kéo sweeper dừng theo.
- **r7 — Row làm relay chết (panic) bị claim lại mãi.** Câu claim không tăng `attempts` (`plan.md:174–176`), nên row gây crash được claim
  lại sau mỗi lease và không bao giờ bị đỗ. Tuỳ chọn: tăng `attempts` ngay trong câu claim, hoặc `recover` quanh xử lý từng row và coi đó
  là lỗi validate.
- **r8 — PgBouncer + autocommit.** Các câu đánh dấu giờ chạy ngoài transaction, có thể rơi vào server connection khác với lúc prepare.
  `QueryExecModeCacheStatement` của pgx đi qua transaction mode chỉ an toàn khi `max_prepared_statements > 0`; ảnh
  `edoburu/pgbouncer:latest` không pin và compose không đặt giá trị này. Rủi ro vốn đã có ở server, nhưng nên đặt
  `cfg.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeCacheDescribe` cho pool của worker, hoặc ghi rõ phụ thuộc kèm bước smoke.
- **r9 — Không chạy goose thật.** Chấp nhận được: 0002 chỉ có câu lệnh đơn, không cần `StatementBegin`. Ghi vào rủi ro; smoke compose
  (`migrate` dùng `goose-docker:3.22.0`) sẽ phủ khi có Docker.
- **r10 — Ghi nhận duyệt.** Ngoài mô tả PR, lưu bằng chứng duyệt (ai, lúc nào, SHA commit của migration) trong
  `docs/workflow/EVF-36-outbox-relay/` để truy vết theo quy trình High-risk.
- **r11 — Row do code cũ ghi** (trước 0002, hoặc trong lúc rollback app) có khoá `event_id` = id buổi diễn **bên trong `data`**. Phong
  bì vẫn đúng, nhưng docs cần ghi rõ consumer chỉ đọc `event_id` ở phong bì.

### Bảo mật (vòng 2)

Không có phát hiện mới. Mọi SQL đều tham số hoá; giá trị ghép thẳng vào câu duy nhất là hằng `'infinity'`. Header chỉ chứa khoá trace
W3C. `last_error` đã được làm sạch và giới hạn độ dài. `data` ≤ 64 KiB và không có PII. `BatchSize` ≤ 1000. Không log URL chứa bí mật.
Khi `AMQP_URL` sai cú pháp: lỗi của `amqp091.ParseURI` không chứa mật khẩu, nhưng lúc bọc lỗi không được kèm chuỗi URL.
