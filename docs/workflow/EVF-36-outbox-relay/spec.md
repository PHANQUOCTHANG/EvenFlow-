# EVF-36 — Transactional outbox + relay ra RabbitMQ (Spec)

| Mục | Giá trị |
|---|---|
| Jira | EV-131 (mã backlog **EVF-36**, issue 1307) — 5 SP — phụ thuộc EVF-7 (`libs/go/mq`, PR #52) |
| Branch | `feature/EVF-36-outbox-relay`, **xếp trên** `feature/EVF-7-libs-go-mq` (#52) |
| Tier | **High-risk** — có migration DB và đổi luồng ghi của ticketing. Cần: review bảo mật, challenge kỹ, **người dùng duyệt migration trước khi merge** |
| Môi trường | Go 1.27 trên máy (`GOTOOLCHAIN=local`), CI Go 1.26; Docker daemon trên máy không chạy → test tích hợp chạy ở CI |

## 1. Bối cảnh

Jira AC: (1) không mất event khi service chết giữa chừng — có test kill-restart; (2) event có `event_id` để consumer khử trùng lặp.
Phạm vi: bảng outbox ghi trong cùng transaction nghiệp vụ; relay đọc, publish, đánh dấu đã gửi; `libs/go/outbox` dùng chung.

### Hiện trạng (`develop` + EVF-7)

| # | Vấn đề | Bằng chứng |
|---|---|---|
| O1 | Không có relay: row `outbox_events` được ghi nhưng không ai publish (`logPublisher`, TODO EVF-36) | `services/ticketing/cmd/worker/main.go:4,86,111-118` |
| O2 | `ticketing.hold.released` publish **thẳng** sau commit, lỗi bị bỏ qua (`_ =`) → service chết giữa commit và publish là **mất event** | `internal/worker/hold_sweeper.go:102-117` |
| O3 | Bảng không có id duy nhất cho message, không có số lần thử / lỗi / lịch thử lại, `trace_id` không bao giờ được ghi | `migrations/0001_core.sql:186-196`, `adapter/postgres/repository.go:179-192` |
| O4 | Payload `ticketing.hold.created` dùng `event_id` = id **sự kiện hoà nhạc**, trái quy ước `event_id` = id message | `repository.go:189`, `docs/03-cau-truc-src.md:214` |
| O5 | `ticketing.hold.created` không có binding nào; `mq.Publish` là `mandatory` → mọi lần publish trả `ErrUnroutable` | `libs/go/mq/topology.go:90-98`, `deploy/rabbitmq/definitions.json` |
| O6 | Worker không có `AMQP_URL`; compose không cho `ticketing-worker` phụ thuộc RabbitMQ | `docker-compose.yml:174-189` |
| O7 | PgBouncer chế độ transaction → không dùng được `LISTEN/NOTIFY`; relay phải **poll** | `docker-compose.yml:37` |

## 2. Tiêu chí nghiệm thu

**AC-1 — Ghi cùng transaction.** Event được ghi vào `outbox_events` trong **cùng** transaction với thay đổi nghiệp vụ: hold
tạo xong ⇒ có row `ticketing.hold.created`; hold được giải phóng (sweeper) ⇒ có row `ticketing.hold.released`. Transaction
rollback ⇒ không có row. Sweeper **không** publish thẳng nữa (O2).

**AC-2 — Phong bì event có `event_id`.** Mỗi row có `event_id` UUID duy nhất. Body publish là JSON
`{ "event_id", "occurred_at", "trace_id", "schema_version": 1, "type": <routing key>, "data": {...} }`; AMQP `MessageId` =
`event_id`. Trường id sự kiện hoà nhạc trong `data` đổi tên thành `sale_event_id` (O4).

**AC-3 — Relay publish và đánh dấu.** Relay poll theo lô (`FOR UPDATE SKIP LOCKED`, an toàn khi chạy nhiều bản), publish qua
`mq.Publisher` (confirm), thành công ⇒ `published_at` được đặt. Lỗi tạm ⇒ tăng `attempts`, ghi `last_error`, lùi
`next_attempt_at` theo backoff có trần; không chặn các row khác.

**AC-4 — Không mất khi chết giữa chừng (kill-restart).** Test tích hợp (Postgres + RabbitMQ thật): N event đã commit; relay bị
"giết" sau khi publish một phần (kể cả sau publish mà trước commit đánh dấu); khởi động relay mới ⇒ **mọi** `event_id` tới
được hàng đợi ít nhất một lần, mọi row có `published_at`. Thêm: RabbitMQ ngừng rồi chạy lại trong lúc relay chạy ⇒ không mất
event `ticketing.hold.released` (`docs/09:87`).

**AC-5 — Trace xuyên suốt.** Trace context của request tạo event được lưu cùng row và được khôi phục khi publish: span phía
consumer cùng trace ID với span lúc ghi outbox.

**AC-6 — Không kẹt vì unroutable.** Event không có hàng đợi nào nhận (vd. `ticketing.hold.created` hiện chưa có consumer) **không**
làm relay thử lại vô hạn: đánh dấu `published_at` kèm `last_error = 'unroutable'`, tăng metric và log cảnh báo.

**AC-7 — Vận hành.** Worker đọc `AMQP_URL` (thiếu ⇒ fail fast), khai báo `mq.TicketingTopology()`, chạy relay cùng các vòng lặp
hiện có, dừng êm theo `ctx`. Metric: số event chờ, số publish lỗi, số unroutable, độ trễ publish. Compose: `ticketing-worker`
có `AMQP_URL` và `depends_on` RabbitMQ.

**AC-8 — Migration an toàn.** `0002_outbox_relay.sql` có Up và Down; Up chạy được trên dữ liệu đã có (row cũ nhận `event_id` sinh
tự động); Down trả lại đúng schema 0001. Test tích hợp chạy Up → Down → Up.

**AC-9 — Không hồi quy.** `go vet`, `golangci-lint`, `go test -race` cho `libs/go` và `services/ticketing` pass; mọi service build
trong workspace; CI `test-go`, `lint-go`, `integration` xanh.

## 3. Ngoài phạm vi

- Consumer của `ticketing.hold.created` (payment/notification), dọn row đã publish (phase 2 theo `docs/09:100`).
- Outbox cho service khác ngoài ticketing.
- Bỏ `mandatory` toàn cục của `mq.Publish`.
