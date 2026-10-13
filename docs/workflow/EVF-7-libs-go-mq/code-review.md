# EVF-7 — Review code (Code review)

| Mục | Giá trị |
|---|---|
| Người review | Reviewer context sạch (agent) — 2026-10-13 |
| Đầu vào | Thay đổi chưa commit trên `feature/EVF-7-libs-go-mq` so với `origin/develop` @ `51c4729`: `libs/go/mq/**`, `libs/go/go.{mod,sum}`, `deploy/rabbitmq/definitions.json`, `docs/02-kien-truc.md`, `.github/workflows/integration.yml` |
| Đối chiếu | `spec.md` (AC-1..AC-7), `plan.md` (§7 đè các mục trước), `plan-review.md`, `baseline.md`, `traceability.md`; mã nguồn `amqp091-go` v1.15.0 trong module cache |
| Kết luận | **FAIL** — 1 blocker, 2 major, 10 minor |

## 1. Đã kiểm gì, bằng cách nào

- Trong `libs/go` (Go 1.27.0, `GOTOOLCHAIN=local`): `go vet ./...` sạch; `go vet -tags=integration ./mq/...` sạch;
  `go test -race -count=1 ./...` **PASS** (`httpx`, `mq`, `otelx`); `golangci-lint` v2.12.2 `run ./mq/...` **0 issues**;
  thêm `--build-tags integration`: **1 issue** (xem m10).
- `go.mod`: dòng `go 1.25.0` giữ nguyên, không có `toolchain`; `git status` chỉ có các file được phép theo plan §4/§7
  (không đụng `go.work`, `go.work.sum`, `services/**`).
- Đọc mã `amqp091-go` v1.15.0: `channel.go:36-56` (`notifyAll`, timeout 5 s), `:511-515` (dispatch `basicReturn`
  đồng bộ trên goroutine đọc của connection, trước `basicAck` cùng message), `:249-250` (`shutdown` gọi `setClosed()`
  trước tiên), `:2205-2245` (`closeResources` **đóng** mọi kênh `returns` rồi mới `confirms.Close()`),
  `confirms.go:199-251` (`Close` đặt `ack=false`; `WaitContext` trả `(false, nil)`), `read.go:184-230` (giải mã số:
  `'B'`→`byte`, `'u'`→`uint16`, `'i'`→`uint32`, `'I'`→`int32`, `'l'`→`int64`).
- **Tái hiện blocker B1** trên bản sao trong scratchpad (không đụng worktree): test `Publisher{returns: <kênh đã đóng>}`
  gọi `wasReturned` → không trả về sau 2 s (`--- FAIL: TestSpinKhiReturnsDong (2.00s)`).
- Docker daemon không chạy → test tích hợp chỉ đọc, không chạy.

## 2. Đối chiếu AC và quyết định §7

| Mục | Kết luận | Ghi chú |
|---|---|---|
| AC-1 | **Không đạt** | Đường ack/nack/ctx đúng, nhưng kênh đóng khi đang chờ confirm → treo vĩnh viễn (B1). Nack thật chưa có test (m9) |
| AC-2 | Đạt (logic) | `decide` dùng `>=` (m4 ✔), `attemptOf` đọc `int32` sau round-trip ✔; rủi ro kiểu không dấu (M2) |
| AC-3 | Đạt | Ack khi `nil`; `ErrPermanent` → DLQ lần đầu với `x-ef-attempt = attempt` |
| AC-4 | Đạt | Span Producer/Consumer (m6 ✔); inject vào bản sao sau khi mở span (m3a ✔); bản retry publish trong ctx span consume → cùng trace |
| AC-5 | Đạt | Code ↔ `definitions.json` khớp (test xanh); test sync có kẽ hở nhỏ (m7) |
| AC-6 / M2 / M4 | Đạt khi không có B1 | `mandatory=true` + `NotifyReturn`; lỗi → `Nack(requeue=true)` rồi `Consume` trả lỗi |
| AC-7 | Đạt cục bộ | Thiếu lint cho file tích hợp (m10); CI chưa chạy |
| M1 | ✔ | Không `go work sync`; chỉ `libs/go/go.{mod,sum}` đổi |
| M3 | ✔ | `x-dead-letter-strategy: at-least-once` + `x-overflow: reject-publish` ở cả code, `definitions.json`, topology test tích hợp |
| M5 | ✔ | Thiếu `Queue`/`DeadLetterExchange`, `MaxAttempts < 0` bị từ chối trước khi chạm kênh |
| M6 | ✔ phần lớn | `docs/02` + `_comment` cập nhật; còn `docs/09:242` (m8) |
| m1 | ✔ một phần | `ErrClosed` riêng, không bọc `amqp091.ErrClosed` ở 2/3 nhánh (m5) |
| m2 | **Lệch có chủ đích** | Mutex bao cả chờ confirm — đúng và không deadlock (xem §4), nhưng chưa ghi nhận là lệch plan (m1) |
| m3b | ✔ | `copyHeaders` bỏ `x-death`, `x-first-death-*`, `x-last-death-*`, `x-delivery-count` — cũng tránh luôn cơ chế phát hiện chu trình dead-letter của broker |
| m5 (plan) | ✔ | `callHandler` recover panic thành lỗi thường |
| m7, m8 (plan) | ✔ | Bước CI có `go vet -tags=integration`, dùng `run-and-report-tests.sh` + `-v`, `cache-dependency-path` nhiều dòng, test `t.Fatalf` khi thiếu Docker |

## 3. Phát hiện

### Blocker

**B1 — Kênh publish đóng khi đang chờ confirm → `Publish` quay vòng vô hạn (100% CPU), giữ mutex, không bao giờ trả về.**
Bằng chứng: `libs/go/mq/publisher.go:113-125` (`wasReturned`) và `:102-111` (`drainStale`) dùng
`select { case r := <-p.returns: ... default: return }`. Khi channel đóng (broker `channel.close` 404 do exchange không
tồn tại, mất kết nối, …), `amqp091` `closeResources` (`channel.go:2219-2221`) **đóng** kênh `returns` rồi mới
`confirms.Close()`; `WaitContext` trả `(false, nil)` (`publisher.go:85`) → `wasReturned` nhận giá trị rỗng từ kênh đã
đóng ở mọi vòng, nhánh `default` không bao giờ chạy. `ctx` không được kiểm trong vòng lặp. Đã tái hiện (§1).
Hậu quả: (a) `TestTichHop_PublishExchangeKhongTonTaiTraLoi` (`integration_test.go:278-289`) treo tới `-timeout=10m`
→ panic, **cả package tích hợp fail** trong CI; (b) trên production, mất kết nối giữa lúc `Consume` forward bản
retry/DLQ → goroutine quay vòng mãi, `Consume` không trả lỗi, supervisor không khởi động lại (trái spec §4, M4);
mọi `Publish` khác trên publisher bị chặn ở mutex. `drainStale` cũng quay vòng nếu kênh đóng giữa `IsClosed()`
(`:67`) và `:70`.
Sửa: đọc bằng `r, ok := <-p.returns`; `!ok` → dừng vòng lặp (với `wasReturned` trả `false` để rơi xuống nhánh `!acked`
→ `IsClosed()` → `ErrClosed`, `shutdown` đã `setClosed()` trước nên nhánh này đúng). Thêm unit test như bản tái hiện
(kênh `returns` đã đóng → hàm trả về ngay).

### Major

**M1 — Tương quan return chỉ bằng `(exchange, routingKey, MessageId)`, `MessageId` thường rỗng → gán nhầm return cũ.**
Bằng chứng: `publisher.go:119`. Kịch bản: lần publish A bị huỷ ctx khi đang chờ (`:85-87`) và **thực sự** không định
tuyến được; return của A có thể tới **sau** `drainStale` của lần B (`:70`) nhưng trước ack của B (ack trên một kênh đi
theo thứ tự) → `wasReturned` của B thấy return của A; cùng exchange/key và `MessageId` rỗng (mặc định của `Message`,
và của bản forward khi message gốc không có id — `consumer.go:228`) → B trả `ErrUnroutable` dù đã vào queue →
consumer nack requeue + dừng → **bản trùng**. Ngược lại một return của B không bao giờ bị bỏ sót (đúng thứ tự
return-trước-ack), nên lỗi là dương tính giả chứ không mất message — vẫn đáng sửa vì plan-review M2 đã chỉ cách.
Sửa: gắn header duy nhất cho mỗi lần publish (vd. `x-ef-publish-id` = uuid/bộ đếm nguyên tử) và so khớp
`r.Headers["x-ef-publish-id"]`; giữ `drainStale` cho return mồ côi.

**M2 — `attemptOf` không đọc kiểu không dấu → `x-ef-attempt` thành 1 → retry vô hạn, không bao giờ vào DLQ.**
Bằng chứng: `consumer.go:98-124` chỉ xử lý `int, int8, int16, int32, int64, string`; `amqp091` giải mã `'B'`→`byte`,
`'u'`→`uint16`, `'i'`→`uint32` (`read.go:184-230`). Header này là **giao thức chung** với worker Python EVF-80/82
(`docs/02-kien-truc.md` mục L5); client nào ghi kiểu không dấu (hoặc `float`) làm mọi lần lỗi bị coi là lần 1 →
vòng retry không dừng, đúng loại lỗi AC-2 cấm. Test `asInt64` của bộ test đã xử lý `uint*` nhưng mã chính thì không.
Sửa: thêm `uint8/uint16/uint32/uint64` (chặn tràn) vào `attemptOf` + case trong `TestAttemptOf_DocHeader`; ghi trong
`docs/02` rằng `x-ef-attempt` là số nguyên có dấu 32-bit (`I`).

### Minor

**m1 — Mutex bao trọn publish→confirm→drain lệch plan m2 nhưng chưa ghi nhận.** `publisher.go:37-39,65-66`. Phân tích
(§4): đúng và không deadlock, nhưng mỗi `Publish` tốn trọn một RTT confirm và tuần tự hoá mọi publish trên publisher
(ảnh hưởng adapter outbox EVF-36). Sửa: ghi lệch + lý do vào `traceability.md`/doc của `Publisher`; khi đã có
`x-ef-publish-id` (M1) có thể bỏ mutex bằng một goroutine phân phối return theo id.

**m2 — Sau khi `Consume` trả về, consumer AMQP không bị huỷ.** `consumer.go:166-182`: không `ch.Cancel(tag)`; broker
tiếp tục đẩy tối đa `Prefetch` message vào bộ đệm của thư viện, chúng nằm **unacked** tới khi kênh đóng — kể cả
message vừa `Nack(requeue=true)` ở đường M4 (test `integration_test.go:463-466` phải đóng hẳn connection để chứng minh
message quay lại). Sửa: dùng consumer tag riêng, `defer ch.Cancel(tag, false)`; doc comment ghi "người gọi phải đóng
`ch` sau khi `Consume` trả về".

**m3 — Bản retry/DLQ làm mất thuộc tính AMQP.** `consumer.go:224-229` chỉ giữ `Body`, `ContentType`, `MessageId`,
headers; mất `CorrelationId`, `ReplyTo`, `ContentEncoding`, `Type`, `AppId`, `Timestamp`, `Priority`. `ContentEncoding`
mất có thể làm body nén không đọc được; `ReplyTo`/`CorrelationId` cần cho mô hình RPC của chat. `docs/02` lại ghi
"giữ body + header". Sửa: thêm các trường vào `Message` và chép sang bản forward, hoặc ghi rõ giới hạn trong docs.

**m4 — `Delivery.Headers` là bảng sống của delivery.** `consumer.go:193`: handler sửa map → thay đổi lọt vào bản
retry/DLQ (`copyHeaders(d.Headers)` ở `:222`). Sửa: truyền `copyHeaders(d.Headers)` (hoặc ghi doc "chỉ đọc").

**m5 — `ErrClosed` không bọc `amqp091.ErrClosed`.** `publisher.go:68,95` chỉ bọc lỗi riêng; `:81` bọc cả hai. Người gọi
kiểm `errors.Is(err, amqp.ErrClosed)` sẽ trượt ở hai nhánh. Sửa: `fmt.Errorf("...: %w: %w", ErrClosed, amqp.ErrClosed)`
nhất quán.

**m6 — `validate` không bắt cấu hình retry nửa vời.** `consumer.go:55-72`: `RetryKey` có mà `RetryExchange` rỗng →
lặng lẽ không retry (lỗi đầu vào DLQ); `RetryExchange` có mà `RetryKey` rỗng → mọi retry `ErrUnroutable` → consumer
dừng ở message lỗi đầu tiên. Sửa: yêu cầu hai trường cùng rỗng hoặc cùng có.

**m7 — Test đồng bộ `definitions.json` có kẽ hở.** `topology_test.go:513-571`: (a) so qua `fmt.Sprintf("%v")` nên chuỗi
`"5000"` trong file trùng với số `5000` trong code (broker sẽ từ chối TTL kiểu chuỗi) — `normArgs` giữ nguyên giá trị
không phải số; (b) gom vào map theo tên nên bản ghi **trùng** (cùng queue/binding hai lần, args khác nhau) không bị
phát hiện; (c) không so `auto_delete`/`internal`/`arguments` của exchange và `arguments` của binding (hiện file không
có, code luôn `false`/`nil` — chấp nhận được nếu ghi chú). Đã kiểm có so `type` exchange và `destination_type`
binding. Sửa: so `%#v` hoặc `reflect.DeepEqual` sau chuẩn hoá; báo lỗi khi tên trùng.

**m8 — Tài liệu chưa nhất quán.** `docs/09-ke-hoach-hoan-thien-backend.md:241` vẫn đề xuất "`definitions.json` — nguồn
duy nhất", trái quyết định spec ("code là nguồn chuẩn"); plan §4 cho phép sửa dòng này. `docs/03-cau-truc-src.md:98`
ghi "publisher có outbox" — `mq` không làm outbox (EVF-36). `_comment` của `definitions.json` ghi "Co alert" cho DLQ
nhưng chưa có alert (EVF-125). Sửa: cập nhật ba chỗ.

**m9 — Khoảng trống test AC-1 vẫn mở.** Không có test nack thật → `ErrNacked`, kênh đóng giữa lúc chờ → `ErrClosed`
(chỉ kiểm "có lỗi"), huỷ ctx khi đang chờ. B1 lọt chính vì thiếu loại test này ở mức unit. Sửa: unit test cho
`wasReturned`/`drainStale` với kênh đóng (B1); test tích hợp queue `x-max-length: 0` + `x-overflow: reject-publish`
→ `errors.Is(err, ErrNacked)`; test exchange không tồn tại kiểm `errors.Is(err, ErrClosed)`.

**m10 — File tích hợp không qua lint.** `golangci-lint run --build-tags integration ./mq/...` báo
`integration_test.go:91:16: Error return value of ch.Close is not checked (errcheck)`. CI `lint-go` không dùng tag
nên không bắt. Sửa: `defer func() { _ = ch.Close() }()`.

## 4. Phân tích publisher confirm + mandatory (yêu cầu riêng)

- **Thứ tự return/ack**: broker gửi `basic.return` trước `basic.ack` của cùng message; `amqp091` xử lý cả hai tuần tự
  trên goroutine đọc của connection, `notifyAll` gửi return vào kênh **trước** khi `confirms.One` đóng `done`
  (`channel.go:511-523`). Vậy khi `WaitContext` trả `acked=true`, return (nếu có) đã nằm trong bộ đệm → đọc không chặn
  ở `wasReturned` là **đáng tin** (không bỏ sót). Lỗi duy nhất là dương tính giả từ return cũ (M1).
- **Deadlock / chặn connection**: chỉ publisher này publish trên kênh, mutex bảo đảm mỗi lúc tối đa ~1–2 return chưa
  đọc, bộ đệm 16 không thể đầy → goroutine đọc của connection không bị chặn. Ngoài ra v1.15.0 `notifyAll` có timeout
  5 s (`channel.go:36-56`) nên kể cả bộ đệm đầy cũng không treo connection vĩnh viễn — nhưng khi đó return bị **bỏ**
  → `Publish` báo thành công cho message không định tuyến được; giữ nguyên thiết kế "một publisher sở hữu kênh" và ghi
  vào doc là đủ. Mutex không giữ trong lúc gọi handler nên không có vòng chờ với `Consume`.
- **ctx huỷ giữa lúc chờ**: `WaitContext` trả `ctx.Err()`, mutex nhả, kết quả không rõ (đã ghi doc `:51`). `Consume`
  nack requeue → có thể trùng (at-least-once, chấp nhận). Lưu ý `PublishWithDeferredConfirmWithContext` không tôn trọng
  ctx khi bị chặn ghi socket (broker `connection.blocked`) — giới hạn của thư viện, nên ghi doc.
- **Kênh đóng giữa lúc chờ**: `shutdown` gọi `setClosed()` trước (`channel.go:250`), `confirms.Close()` → `(false,nil)`;
  sau khi sửa B1 nhánh `!acked && IsClosed()` sẽ trả đúng `ErrClosed` (m1 của plan-review).
- **Rò rỉ**: không tạo goroutine; kênh `returns` do thư viện đóng khi kênh đóng. Không rò rỉ (ngoài B1).

## 5. Topology, CI, tài liệu

- `AITopology()` (`topology.go:288-331`) khớp spec §2 + M3 từng tham số; `definitions.json` khớp code, `ai.dlq` bind
  từng key, hàng gốc không `x-dead-letter-*`/`x-max-priority`. Quorum queue 3.13 hỗ trợ `x-message-ttl` (hết hạn ở đầu
  hàng — mọi message trong hàng trễ cùng TTL nên FIFO đúng) và at-least-once yêu cầu `reject-publish` — đã đặt.
- Test tích hợp hợp lý cho broker thật: hàng trễ TTL 200 ms cũng đặt `at-least-once` + `reject-publish`
  (`integration_test.go:138-145`), `eventually` 15–30 s, một `Sleep(3×TTL)` duy nhất; container dùng chung qua
  `sync.Once`, dọn ở `TestMain`; thiếu Docker → `t.Fatalf` mọi test. Tất cả 9 test tên `TestTichHop_*` nên pattern
  `TestTichHop` của `run-and-report-tests.sh` khớp (script đếm theo regex không neo). **Nhưng** với B1, test exchange
  không tồn tại sẽ treo tới hết `-timeout=10m`.
- `integration.yml`: `working-directory: libs/go`, `go vet -tags=integration`, `go test -v` bọc đúng
  `run-and-report-tests.sh <pattern> <cmd...>`, `cache-dependency-path` nhiều dòng — đúng. Bước chạy sau bước
  ticketing; nếu bước ticketing fail thì bước `mq` bị bỏ qua (mặc định GitHub) — chấp nhận được.
- `docs/02` (L5) mô tả đúng giao thức mới; thiếu kiểu của `x-ef-attempt` (M2) và danh sách thuộc tính được giữ (m3).
- Phong cách: comment không dấu, `fmt.Errorf("...: %w")`, test chỉ `testing` + recorder chép — khớp `libs/go`.

## 6. Việc phải làm trước khi PASS

1. B1: sửa `wasReturned`/`drainStale` xử lý kênh đóng + unit test.
2. M1: `x-ef-publish-id` để tương quan return.
3. M2: `attemptOf` nhận số không dấu + test; ghi kiểu header trong `docs/02`.
4. Minor: xử lý hoặc ghi lý do không làm vào `traceability.md`.
