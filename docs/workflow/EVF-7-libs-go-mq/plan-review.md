# EVF-7 — Review kế hoạch (Plan review)

| Mục | Giá trị |
|---|---|
| Người review | Reviewer context sạch (agent) — 2026-10-13 |
| Đầu vào | `spec.md`, `plan.md` (chưa commit) trên `feature/EVF-7-libs-go-mq` = `develop` @ `51c4729` |
| Kết luận | **PASS WITH CHANGES** — 0 blocker, 6 major, 9 minor |

## 1. Đã kiểm gì, bằng cách nào

- Đọc: `deploy/rabbitmq/definitions.json`, `deploy/compose/docker-compose.yml:65-78`, `libs/go/{go.mod,otelx/amqp.go,otelx/span.go,otelx/otelx_test.go}`,
  `go.work`, `.github/workflows/{integration.yml,ci.yml}`, `.github/scripts/{run-and-report-tests.sh,check-domain-coverage.sh}`,
  `.golangci.yml`, `deploy/docker/ticketing.Dockerfile`, `docs/02-kien-truc.md:215-226`, `docs/03-cau-truc-src.md:94-104`,
  `docs/09-ke-hoach-hoan-thien-backend.md:242`, `services/ticketing/internal/port/ports.go:139-141`,
  `services/ai-worker/.../consumers/chat.py`.
- Proxy: `amqp091-go` mới nhất **v1.15.0** (`go 1.20`, chỉ cần `goleak`); `testcontainers-go` và `modules/rabbitmq`
  mới nhất **v0.44.0** (`go 1.25.0`, `toolchain go1.25.9`) → tương thích `go 1.25.0` của `libs/go` và Go 1.26 của CI.
- **Thực nghiệm** trên bản sao (`git archive` vào scratchpad, không đụng worktree), Go 1.27.0 `GOTOOLCHAIN=local`:
  - `go get amqp091-go@v1.15.0 modules/rabbitmq@v0.44.0` + `go mod tidy` trong `libs/go`: dòng `go 1.25.0` **giữ nguyên**;
    `go.mod` +42 dòng, `go.sum` +97 dòng; `go.work.sum` **không đổi**.
  - `var _ Declarer = (*amqp091.Channel)(nil)` với đúng interface trong plan §1 **biên dịch được** → chữ ký khớp chính xác.
  - `PublishWithDeferredConfirmWithContext` + `DeferredConfirmation.WaitContext` và `rabbitmq.Run(ctx, "rabbitmq:3.13-management-alpine")`
    + `AmqpURL(ctx)` biên dịch được (`go vet -tags=integration ./mq/` sạch).
  - Workspace mode: `go build ./...` + `go vet ./...` của **cả 8 service** đều qua sau khi thêm dependency; module mode
    (`GOWORK=off`) của `ticketing`, `gateway` cũng qua, **không** cần sửa `go.sum` của service.
  - `go work sync`: xem M1.
  - `go test -race` chạy được trên máy (gcc ucrt64 có sẵn). `golangci-lint` **không** cài trên máy. Docker CLI có nhưng
    daemon không chạy.

## 2. Kiểm chứng T1–T7 (RabbitMQ 3.13)

| # | Kết luận | Ghi chú |
|---|---|---|
| T1 | **Đúng** | Khi dead-letter có `x-dead-letter-routing-key`, routing key của message bị **thay** bằng key đó (`delay.Xs`). Hàng trễ dead-letter về `ai.direct` không đặt key → dùng key hiện tại `delay.Xs` → không binding nào khớp → bị bỏ. |
| T2 | **Đúng** | Hệ quả trực tiếp của T1 + hàng dùng chung. |
| T3 | **Đúng** (diễn đạt cần chỉnh) | Với `direct`, `#` là chuỗi so khớp nguyên văn: chỉ message có routing key đúng bằng `"#"` mới tới DLQ. "Không message nào" đúng trên thực tế, nên viết "trừ key literal `#`". |
| T4 | **Đúng** | Không queue nào có `x-dead-letter-exchange: ai.dlx`. |
| T5 | **Đúng** cho 3.13 | 3.13 từ chối `x-max-priority` trên quorum queue (`PRECONDITION_FAILED - invalid arg 'x-max-priority' ... rabbit_quorum_queue`). Import definitions cũng hỏng. (4.x hỗ trợ priority cho quorum theo kiểu khác; không áp dụng image `3.13` đang ghim.) Test tích hợp `Declare(AITopology())` bắt được — tốt. |
| T6 | **Đúng** | TTL hết hạn là một lý do dead-letter (`expired`) → đi `ai.retry`. |
| T7 | **Đúng** | Compose chỉ mount file, không có `RABBITMQ_SERVER_ADDITIONAL_ERL_ARGS`/`load_definitions` (`docker-compose.yml:65-78`); `deploy/k8s` không nhắc RabbitMQ. |

Topology mới (spec §2) về cơ bản **đúng**: hàng trễ riêng + `x-dead-letter-routing-key` = key của hàng gốc sửa T1/T2;
DLQ bind từng key sửa T3/T4; bỏ `x-dead-letter-*` khỏi hàng gốc sửa T6 (hết TTL ở quorum queue không có DLX thì bị bỏ);
bỏ priority sửa T5. Quorum queue hỗ trợ `x-message-ttl` từ 3.10. **`x-delivery-limit`: 3.13 không có giá trị mặc định
(không giới hạn)**; từ 4.0 mặc định là 20 — xem M4.

## 3. Phát hiện

### Major

**M1 — `go work sync` làm bẩn mọi module ngay cả trên `develop`; plan tự mâu thuẫn.**
Bằng chứng: chạy `go work sync` trên bản sao **chưa** thêm dependency nào (`51c4729`) đã sửa `go.work`, `libs/go/go.mod`
(`go 1.25.0` → `1.26.0`), `services/{identity,ticketing,waitingroom}/go.{mod,sum}` (dòng `go` → 1.26.0; `x/sys` 0.47→0.48,
`x/text` 0.41→0.42, `x/sync` 0.22→0.23; identity nâng `google.golang.org/api` v0.81→v0.264). Thêm amqp/testcontainers
không đổi gì thêm. Plan §5 "chạy `go work sync`" vi phạm §4 (không đụng `services/**`) và mục tiêu giữ `go 1.25.0`; AC-7
"`go work sync` không làm bẩn module khác" không thể đạt như đang viết.
Sửa: bỏ `go work sync` khỏi plan. Thay bằng: `go mod tidy` chỉ trong `libs/go`; kiểm `git status` chỉ có
`libs/go/go.{mod,sum}` đổi; chạy `go build ./... && go vet ./...` cho từng module trong vòng lặp như `ci.yml` (workspace
mode) — đó là chế độ CI và Dockerfile dùng (`ticketing.Dockerfile:15-28` copy `go.work` + toàn bộ `libs`). Sửa AC-7
tương ứng; ghi `go work sync` drift là nợ có sẵn (ngoài phạm vi). Bỏ `go.work.sum` khỏi danh sách file (thực nghiệm:
không đổi).

**M2 — Message không định tuyến được bị broker *ack* → retry/DLQ mất im lặng, AC-6 không bảo vệ.**
Bằng chứng: publish với `mandatory=false` tới exchange tồn tại nhưng không binding khớp → broker bỏ message và vẫn gửi
`basic.ack` (confirm chỉ nói "broker đã nhận trách nhiệm", không nói "đã vào queue"). Plan §1 không nhắc `mandatory`;
consumer sẽ `Ack` bản gốc sau khi `Publish` trả `nil` → mất message. Đây chính là loại lỗi T1–T4 (thiếu binding).
Sửa: `Publish` đặt `mandatory=true`, đăng ký `NotifyReturn` trên kênh của publisher và tương quan return với lần publish
(vd. header `x-ef-publish-id` duy nhất, hoặc `MessageId`), trả `ErrUnroutable` khi bị return (broker gửi `basic.return`
**trước** `basic.ack` của cùng message). Thêm test tích hợp: retry key không có binding → `Publish` lỗi → bản gốc
`Nack(requeue=true)`. Tối thiểu (nếu không làm): ghi rõ trong spec là rủi ro chấp nhận và chọn `fanout` cho `ai.dlx`.

**M3 — Chặng hàng trễ → hàng gốc là at-most-once.**
Bằng chứng: dead-letter từ quorum queue mặc định `x-dead-letter-strategy: at-most-once`; nếu queue đích không nhận
(leader đổi, đầy, cluster 3 node ở prod — `docs/02-kien-truc.md:129,337`) message bị bỏ. Publisher confirm chỉ phủ chặng
consumer → `ai.retry`. Lời hứa "không mất ngoài at-least-once" chưa đúng cho cả đường đi.
Sửa: hàng `ai.*.retry` khai `x-dead-letter-strategy: at-least-once` **và** `x-overflow: reject-publish` (bắt buộc đi kèm),
cả trong code lẫn `definitions.json`; test topology so cả hai arg. Hoặc ghi rõ trong spec §2 là chấp nhận at-most-once
cho chặng này.

**M4 — `Nack(requeue=true)` khi publish lỗi: vòng lặp nóng, và với RabbitMQ ≥ 4.0 thành mất message.**
Bằng chứng: plan §1 "publish lỗi → Nack(requeue=true)" rồi tiếp tục vòng consume. Nếu lỗi bền (kênh publisher đã đóng
do 404 exchange, broker alarm) thì mọi message bị lấy ra – nack – lấy lại liên tục. 3.13 không có delivery-limit mặc định
(vô hạn); 4.0+ mặc định 20 và vì hàng gốc **không còn DLX** (spec §2) message bị **bỏ** sau 20 lần.
Sửa: khi publish retry/DLX lỗi → `Nack(requeue=true)` rồi **trả lỗi khỏi `Consume`** (khớp spec §4 "consumer trả lỗi để
supervisor khởi động lại"). Ghi vào spec: topology giả định 3.13 (image ghim `3.13-management-alpine`), nâng 4.x phải xem
lại `x-delivery-limit` (đặt tường minh, vd. `-1`/không giới hạn hoặc giá trị lớn).

**M5 — Cấu hình không có DLX → `nack requeue=false` = bỏ message.**
Bằng chứng: plan §1 `ConsumerConfig`: "không có [DLX] thì nack requeue=false". Hàng gốc mới không có `x-dead-letter-*`
nên `requeue=false` là xoá. Trái tinh thần AC-6.
Sửa: `Consume` từ chối cấu hình thiếu `DeadLetterExchange` (trả lỗi trước khi consume), hoặc khi thiếu thì
`Nack(requeue=true)` + trả lỗi. Thêm hàng trong bảng test `decide`/validate config.

**M6 — Thiếu phạm vi tài liệu: `docs/02` mô tả topology cũ, "ghi topology mới cho EVF-80/82" không có file đích.**
Bằng chứng: `docs/02-kien-truc.md:224` ("mỗi queue có `x-dead-letter-exchange` trỏ `ai.retry` với các delay-queue
5s / 30s / 2m") sẽ sai sau slice; spec §4 và plan §5 hứa ghi topology cho ai-worker nhưng §4 plan không cho sửa file nào
ngoài `docs/03`, `docs/09`. Hợp đồng consumer (header `x-ef-attempt`, publish sang `ai.retry`/`ai.dlx` theo key, ack sau
confirm) là thứ ai-worker Python **bắt buộc** phải theo — `chat.py:40-45` hiện còn mô hình "nack với delay_ms" khác hẳn.
Sửa: thêm `docs/02-kien-truc.md` (đoạn L5, dòng ~215-226) vào file được phép; ghi topology mới + giao thức retry ở đó.
Cập nhật luôn `_comment` trong `definitions.json` (đang mô tả `delay.5s/30s/2m`).

### Minor

**m1 — Kênh đóng hiện ra thành `ErrNacked`.** `deferredConfirmations.Close()` (amqp091 v1.15.0 `confirms.go`) đặt
`ack=false` cho mọi confirm đang chờ, `WaitContext` trả `(false, nil)`. Đạt AC-1 (vẫn lỗi) nhưng chẩn đoán sai. Sửa: khi
`!ack` kiểm `ch.IsClosed()` → trả lỗi bọc `amqp091.ErrClosed`; giữ `ErrNacked` cho nack thật. Lưu ý thêm:
`PublishWithDeferredConfirmWithContext` chỉ kiểm `ctx` **trước** khi gửi; huỷ giữa chừng chỉ có tác dụng ở `WaitContext`.

**m2 — Mutex trong `Publisher`.** `PublishWithDeferredConfirm` đã giữ `ch.m` → an toàn đồng thời trên một kênh. Nếu mutex
của plan bao cả `WaitContext` thì mỗi publish phải chờ một RTT → tuần tự hoá. Sửa: không dùng mutex, hoặc chỉ bao lời gọi
publish, không bao chờ confirm. Ghi rõ: publisher và consumer dùng **kênh riêng** (khuyến nghị RabbitMQ; flow control
publish không chặn ack).

**m3 — Header khi chép bản sao.** (a) `Inject` phải ghi vào **bản copy** của `msg.Headers` (map `nil` → panic; không sửa
map của caller) và gọi **sau** khi mở span publish để consumer là con của span publish. (b) Bỏ `x-death`,
`x-first-death-*`, `x-last-death-*`, `x-delivery-count` khi chép header sang bản retry/DLQ (header do broker quản lý).
(c) `amqp091.Table.Validate` **không** nhận `map[string]any` lồng nhau (chỉ `amqp091.Table`) → chuẩn hoá hoặc ghi rõ trong
doc của `Message.Headers`.

**m4 — `decide` phải dùng `attempt >= maxAttempts`**, không phải `==` (header bị sửa, hoặc `MaxAttempts` hạ xuống khi
đang có message trong hàng trễ). Thêm case `attempt > max` vào bảng test.

**m5 — Trùng lặp và crash.** Publish-rồi-ack: confirm xong nhưng ack lỗi (kênh đóng) → bản gốc giao lại với **cùng**
`x-ef-attempt` → có hai bản (một trong hàng trễ). Handler crash/panic trước ack → giao lại mãi với cùng attempt (3.13
không giới hạn). Sửa: ghi vào spec "at-least-once, handler phải idempotent"; `Consume` `recover()` panic của handler →
coi như lỗi; cân nhắc tính attempt = max(header, `x-delivery-count`+1) mà quorum queue gắn khi giao lại.

**m6 — Span kind.** `otelx.StartSpan` cố định `SpanKindInternal` (`otelx/span.go:18-19`); span publish/consume nên là
`Producer`/`Consumer`. Không chặn AC-4 (trace ID vẫn chung). Sửa: dùng `otel.Tracer(...).Start` với kind đúng trong `mq`,
hoặc chấp nhận và ghi chú. Recorder: `newRecorder` là hàm không export trong `otelx_test.go` → `mq` phải chép helper;
test đổi provider toàn cục nên không `t.Parallel()`.

**m7 — Gate CI cho file definitions.** Filter `go` của `ci.yml:49-61` không có `deploy/rabbitmq/**` → PR chỉ sửa
`definitions.json` không chạy `test-go` của `libs/go`. `integration.yml` chạy mọi PR và `go test -tags=integration ./mq/...`
cũng chạy test không gắn tag, nên vẫn bắt được — ghi rõ điều này trong plan, hoặc thêm `deploy/rabbitmq/**` vào filter
`go` (khi đó thêm `ci.yml` vào danh sách file). `lint-go`/`go vet` không dùng tag `integration` → file tích hợp không
được lint; thêm `go vet -tags=integration ./mq/...` vào bước mới của `integration.yml`.

**m8 — Bước CI tích hợp.** Theo hợp đồng K1 (`run-and-report-tests.sh`): test tích hợp **không** được `t.Skip` khi thiếu
Docker (phải `t.Fatal`), và bọc bước bằng `run-and-report-tests.sh` với `go test -v`. `cache-dependency-path` của
`setup-go` đổi thành danh sách nhiều dòng (`services/ticketing/go.sum` và `libs/go/go.sum`) thay vì thay thế. Đây là lần
đầu repo dùng testcontainers (ticketing vẫn `t.Skip`, `helpers.go:31-35`) — ghi chú Ryuk cần Docker socket (có trên
`ubuntu-latest`).

**m9 — Môi trường máy.** `golangci-lint` chưa cài: AC-7 cần chạy được cục bộ — dùng
`go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.12.2 run ./...` trong `libs/go` (khớp bản ghim ở
`ci.yml`), hoặc ghi rõ chỉ kiểm ở CI. Docker Desktop **có cài** (CLI trong PATH) nhưng daemon tắt — "máy không có
Docker" nên sửa thành "daemon không chạy; có thể bật để chạy test tích hợp cục bộ".

## 4. Các điểm đã xác nhận đúng (không cần đổi)

- `Declarer` khớp chính xác `*amqp091.Channel` v1.15.0 (đã biên dịch assertion).
- `PublishWithDeferredConfirmWithContext(ctx, exchange, key, mandatory, immediate, msg) (*DeferredConfirmation, error)` và
  `(*DeferredConfirmation).WaitContext(ctx) (bool, error)` tồn tại đúng chữ ký; `Confirm(false)` trả `nil` DC nếu chưa bật
  confirm → `NewPublisher` phải gọi `Confirm` trước (plan có).
- `otelx.AMQPHeaders` là `map[string]any` → chuyển kiểu trực tiếp từ/đến `amqp091.Table`; `traceparent` giải mã ra
  `string` nên `Get` đọc được. Bản retry được `Inject` trong ctx của span consume → trace ID giữ nguyên qua hàng trễ (AC-4).
- `testcontainers-go/modules/rabbitmq` v0.44.0: `rabbitmq.Run(ctx, img, opts...)`, `(*RabbitMQContainer).AmqpURL(ctx)`;
  yêu cầu Go 1.25.0 — hợp với `go 1.25.0` và CI 1.26. Dependency này kéo `testify` gián tiếp (không ảnh hưởng quy ước
  "test không dùng testify").
- Thêm dependency vào `libs/go` **không** buộc sửa `go.sum` của service nào (workspace mode gộp `go.sum`; module mode của
  service vẫn build vì đồ thị được cắt tỉa). Dockerfile copy cả `libs` nên có `libs/go/go.sum`.
- Coverage gate bỏ qua `libs/go` (không có `internal/domain|app`) — không cản.
- `ports.go:139-141` (`Publish(ctx, routingKey, payload any)`) khác chữ ký `mq.Publisher` — đúng là việc của adapter
  EVF-36, không xung đột.

## 5. Thay đổi yêu cầu trước khi code

1. M1: bỏ `go work sync`, sửa AC-7 và §5 plan; bỏ `go.work.sum` khỏi file được phép.
2. M2: `mandatory=true` + xử lý return (hoặc ghi rủi ro chấp nhận rõ ràng).
3. M3: thêm `x-dead-letter-strategy: at-least-once` + `x-overflow: reject-publish` cho hàng trễ (hoặc ghi chấp nhận).
4. M4, M5: lỗi publish → nack requeue + trả lỗi khỏi `Consume`; cấu hình thiếu DLX bị từ chối.
5. M6: thêm `docs/02-kien-truc.md` vào file được phép; cập nhật `_comment` của `definitions.json`.
6. Minor: xử lý trong lúc code, ghi vào `review` nếu chọn không làm.
