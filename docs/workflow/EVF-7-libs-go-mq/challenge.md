# EVF-7 — Thử phá (Adversarial challenge)

| Mục | Giá trị |
|---|---|
| Người thực hiện | Challenger context sạch (agent), 2026-10-13 |
| Đối tượng | `feature/EVF-7-libs-go-mq` @ `e8ee62b` (PR #52 draft), worktree `EvenFlow-mq` |
| Đối chiếu | `spec.md`, `plan.md` (§7, §8 đè), `code-review.md`, `traceability.md`; mã `amqp091-go` v1.15.0 trong module cache; log CI run `38333640434` |
| Môi trường | Go 1.27.0 (`GOTOOLCHAIN=local`), Windows; Docker daemon **không** chạy → test tích hợp không chạy được ở máy, chỉ suy luận + đọc log CI |
| Kết luận | **PASS**: không có blocker hay major. Có 1 lỗi hành vi mức Thấp (F1, điều kiện hẹp) và một số chỗ test còn hở (F2–F6) |

## 1. Đã làm gì

- `go test -race -count=20 ./mq/...`: **PASS** (4.7 s). `go vet -tags=integration ./mq/...`: sạch.
- Thử 67 mutant, mỗi lần sửa một chỗ (script Python trong scratchpad: thay chuỗi đúng 1 lần → `go test -count=1 ./mq/...` →
  trả file về như cũ trong `finally`). Có 3 mutant ban đầu không biên dịch được (P3, P15, P17); đã viết lại cho
  biên dịch được rồi chạy lại.
- Viết test tạm `mq/zz_challenge_test.go` (X1–X4, **đã xoá**) để tái hiện F1, chứng minh F3 bắt được bằng unit test,
  và kiểm header có phình ra hay goroutine có rò không.
- Lần theo mã amqp091 v1.15.0 cho return/ack, đóng kênh, `basic.cancel`, `Table.Validate`, recovery.
- Đọc log CI `38333640434`: bước `go test -tags=integration (libs/go/mq)` có 11/11 `TestTichHop_*` PASS, **không có
  SKIP**, `ok ... libs/go/mq 20.320s`.

## 2. Phát hiện

| # | Mức | Tóm tắt |
|---|---|---|
| F1 | **Thấp** (lỗi hành vi tiềm ẩn) | Bộ đệm `NotifyReturn` (16) đầy → amqp091 bỏ `basic.return` sau 5 s → `Publish` trả `nil` cho message không định tuyến được (mất im lặng), goroutine đọc của connection bị kẹt 5 s cho mỗi return dư |
| F2 | Nhỏ (test hở) | Không test nào kiểm `Publish` có gửi đúng `CorrelationId/ReplyTo/Type/AppId/Priority/Timestamp/ContentEncoding` → cam kết plan §8 m3 không được bảo vệ đầu-cuối |
| F3 | Nhỏ (test hở) | Bỏ mutex của `Publisher` thì không test nào fail; hai publish đồng thời khiến một `ErrUnroutable` bị nuốt |
| F4 | Nhỏ (test hở) | Phần nối dây của `Consume` chưa có test: kênh deliveries đóng (`basic.cancel`), đường `RetryExchange` rỗng, `Qos`, `Cancel` khi thoát |
| F5 | Thấp (test hở) | `TestDeclare_LoiBocTenDoiTuongVaDungLai` không bắt được trường hợp `Declare` nuốt lỗi khai báo queue |
| F6 | Thấp (test hở) | Test đồng bộ `definitions.json` bỏ qua `policies`/`parameters`, trường lạ và khoá JSON trùng |
| F7 | Thông tin | Nếu bật `amqp091.Config.Recovery` thì ngữ nghĩa `ErrClosed`/`ErrNacked`/deliveries khác hẳn; doc chưa cấm |
| F8 | Thông tin | Log CI: bước "oversell smoke" cùng job có `0 PASS, 0 FAIL, 40 SKIP` (có sẵn từ trước, không thuộc EVF-7) |
| F9 | Thông tin | `x-ef-publish-id` lộ ra cho handler trong `Delivery.Headers`; `attemptOf` không nhận số thực (`'d'`) |

### F1 — Return bị bỏ khi bộ đệm đầy → `Publish` báo thành công cho message không định tuyến được (Thấp)

**Lần theo mã amqp091 v1.15.0:**
- `channel.go:511-515`: `basicReturn` được gửi tới listener qua `notifyAll` **ngay trên goroutine đọc** của connection.
- `channel.go:28,36-55`: `notifyAll` chờ tối đa `notifyTimeout = 5s` cho mỗi listener. Hết giờ thì **bỏ** giá trị đó.
- `publisher.go` (`NewPublisher`): `ch.NotifyReturn(make(chan amqp.Return, 16))`. `collect` chỉ chạy lúc bắt đầu
  `Publish` (pre-drain) và sau khi `WaitContext` trả về. Trong lúc chờ confirm thì không ai đọc kênh returns.
- `code-review.md` §4 cho rằng nhờ mutex chỉ có khoảng 1–2 return chưa đọc nên bộ đệm 16 "không thể đầy". Điều này
  **sai** khi đang có các publish bị huỷ ctx nhưng vẫn còn trên đường truyền. Khi broker bị `connection.blocked` (memory
  alarm), frame nhỏ vẫn ghi được vào bộ đệm TCP và `WaitContext` trả `ctx.Err()`. Chính
  `TestTichHop_PublishHuyCtxKhiDangChoConfirm` cho thấy đường này xảy ra thật trên broker 3.13. Caller dùng timeout cho
  mỗi lần gọi và thử lại (kiểu outbox relay) có thể để ≥16 publish không định tuyến được còn treo. Khi alarm tắt, broker
  trả hết return và ack. Lúc đó một `Publish` mới đã pre-drain xong và đang chờ confirm: 16 return cũ làm đầy bộ đệm,
  return của **chính lần này** bị bỏ sau 5 s, rồi ack tới → `Publish` trả `nil`.
- Hệ quả phụ: mỗi return vượt bộ đệm làm goroutine đọc kẹt 5 s. Mọi kênh trên connection (kể cả kênh consume) đứng
  theo trong thời gian đó.

**Tái hiện (test tạm X1, đã xoá).** Kênh giả mô phỏng đúng `notifyAll`, timeout rút từ 5 s xuống 50 ms:

```go
f.onPublish = func(f *fakePubChannel, ex, key string, msg amqp.Publishing) (confirmation, error) {
    for i := 0; i < 16; i++ { notifyAllSim(f.returns, amqp.Return{Headers: amqp.Table{hdrPublishID: "cu"}}, 50*time.Millisecond) }
    dropped = !notifyAllSim(f.returns, amqp.Return{Headers: amqp.Table{hdrPublishID: msg.Headers[hdrPublishID]}}, 50*time.Millisecond)
    return fakeConfirm{ack: true}, nil
}
err := p.Publish(ctx, "ai.retry", "khong-co-binding", Message{})
```

**Kết quả quan sát:** `return cua lan nay bi bo: true; Publish tra: <nil>`, rồi
`--- FAIL: TestChallenge_ReturnBiBoKhiBoDemDay`.

**Điều kiện:** cần ≥16 publish không định tuyến được bị bỏ dở. Đường `Consume` không chạm tới vì khi ctx huỷ thì
`Consume` dừng. Nên mức là **Thấp**.

**Gợi ý sửa:** trong lúc chờ confirm, dùng `select` trên `p.returns`, `dc.Done()` và `ctx.Done()` để vừa chờ vừa đọc
return. Như vậy bộ đệm không đầy trong lúc chờ và return của chính lần này không bị bỏ. Ghi rõ giới hạn này trong
`doc.go`.

### F2 — Thuộc tính AMQP qua `Publish` không được kiểm (Nhỏ)

Các mutant P9–P13 bỏ từng trường `CorrelationId`, `ReplyTo`, `Type`/`AppId`/`Timestamp`, `Priority`, `ContentEncoding`
khỏi `amqp.Publishing` trong `Publish`. Kết quả: **unit vẫn xanh**, và test tích hợp cũng không bắt được vì chỉ kiểm
Body/ContentType/MessageId/DeliveryMode. `TestForwardMessage_GiuThuocTinh` chỉ kiểm struct `Message`, chưa kiểm cái
thực sự gửi lên kênh.

**Sửa:** trong `TestPublisher_AckTraNilVaDatThuocTinh`, đặt đủ mọi trường của `Message` rồi so với `f.lastPublished`.

### F3 — Tính đồng thời của `Publisher` không được test (Nhỏ)

Mutant P14 (bỏ `p.mu.Lock()/Unlock()`): **unit và tích hợp đều xanh**. Test tạm X2 cho hai goroutine dùng chung một
`Publisher`. A publish key không định tuyến được và chờ confirm 200 ms. Trong lúc đó B publish key hợp lệ, và
pre-drain của B nuốt return của A.

**Kết quả quan sát:** code hiện tại **PASS**. Với mutant P14: `A: <nil>, muon ErrUnroutable (return bi publish khac
nuot)` → FAIL.

**Sửa:** thêm X2 vào bộ test chính thức để khoá quyết định "giữ mutex" của plan §8 m1.

### F4 — Phần nối dây của `Consume` chưa có test (Nhỏ)

`Consume` chỉ được test qua broker thật, nên các mutant sau sống sót ở **cả hai** bộ test:
- C27: kênh deliveries đóng → `return nil`. Không test nào mô phỏng `basic.cancel`.
- C20: `canRetry` luôn `true`. Không test tích hợp nào dùng `RetryExchange` rỗng. Khi đó bản retry sẽ đi tới default
  exchange với key rỗng → `ErrUnroutable` → Consume dừng, thay vì đi thẳng DLQ như hợp đồng.
- C25: bỏ prefetch. C26: bỏ `Cancel` khi thoát.

Hành vi hiện tại vẫn đúng theo lần đọc mã. Broker gửi `basic.cancel` (vd. queue bị xoá) → `channel.go:501-509` →
`consumers.cancel` đóng kênh vào (`consumers.go:148-158`) → goroutine `buffer` đẩy nốt các message còn lại rồi đóng
`out` → `Consume` trả lỗi "kenh deliveries da dong". Nhưng không có gì chặn hồi quy.

**Sửa:** tách vòng lặp `Consume` để nhận `<-chan amqp.Delivery` và một `Acknowledger` giả (`amqp.Delivery.Acknowledger`
là interface), hoặc thêm test tích hợp xoá queue khi đang consume và chạy với `RetryExchange` rỗng.

### F5 — Test lỗi `Declare` không phân biệt được lỗi khai báo queue (Thấp)

Mutant T3 thay `return fmt.Errorf("mq: declare queue ...")` bằng `_ = err` (nuốt lỗi): test **vẫn xanh**. Lý do:
`fakeDeclarer.QueueBind` cũng fail khi `name == failOn`, nên lỗi binding phía sau vẫn mang sentinel và tên queue.

**Sửa:** fake chỉ fail đúng thao tác được chọn (thêm trường `failOp`), rồi kiểm lần gọi cuối có `op == "queue"`.

### F6 — Phạm vi test đồng bộ `definitions.json` (Thấp)

Ba mutant trên file JSON sống sót:
- D5: thêm `"policies"` đặt `dead-letter-exchange` + `delivery-limit` cho `^ai\.chat\.q$`. Policy này **đưa lại T6**
  (hết TTL thì đi retry) mà test không thấy.
- D6: khoá trùng trong một object (`"x-message-ttl": 1, "x-message-ttl": 60000`). Go lấy giá trị cuối.
- D7: trường lạ `"type": "classic"` trên queue.

**Sửa:** fail khi có `policies`, `parameters` hoặc `global_parameters` nhắm vào `ai.*` (hoặc cấm hẳn), và dùng
`dec.DisallowUnknownFields()` với danh sách trường cho phép.

### F7–F9 — Thông tin

- **F7:** amqp091 v1.15.0 có recovery tuỳ chọn. Khi bật, `closeChannel` không giải phóng kênh (`connection.go:1141`),
  `shutdown` không gọi `closeResources` (`channel.go:312-317`), còn `confirms.reset` đặt `ack=false`. Hệ quả: publish tới
  exchange không tồn tại sẽ ra `ErrNacked` thay vì `ErrClosed`. Nên ghi vào `doc.go`: "không bật `Config.Recovery`".
- **F8:** log CI dòng 375: `##[warning]0 lan PASS, 0 lan FAIL, 40 lan SKIP` ở bước oversell smoke (ticketing). Bước
  này không kiểm gì. Đây là nợ có sẵn, ngoài EVF-7. Ngoài ra có cảnh báo Node 20 bị deprecate.
- **F9:** `newDelivery` không bỏ `x-ef-publish-id` khỏi header đưa cho handler (chỉ là chuyện hiển thị). `attemptOf`
  trả 1 cho `float64`, nhưng chỉ làm thừa vài lần retry có giới hạn, vì bản retry đầu tiên do thư viện viết lại dùng
  `int32`.

## 3. Mutant

Có 67 mutant: 37 bị unit giết, 30 sống. Trong 30 mutant sống:
- 8 là **tương đương** (không đổi hành vi với amqp091 v1.15.0).
- 7 bị test tích hợp giết. Kết luận này là suy luận từ test đã PASS trong CI, không chạy lại được ở máy.
- 15 **thực sự sống** (không test nào trong cả hai bộ bắt được).

| ID | Mutant | Unit | Tích hợp (suy luận) | Ghi chú |
|---|---|---|---|---|
| C1 | `decide` `>=` → `>` | **Giết** | — | |
| C2 | `ErrPermanent` không unwrap | **Giết** | — | |
| C3 | bỏ `!canRetry` | **Giết** | — | |
| C4 | `attemptOf` bỏ chặn `n<1` | **Giết** | — | |
| C5 | `attemptOf` bỏ chặn tràn | **Giết** | — | |
| C6 | bỏ kiểm tràn `uint64` | Sống | — | Tương đương (giá trị bị chặn ở kiểm tra cuối) |
| C7 | bỏ case `uint32` | **Giết** | — | |
| C8–C11 | `copyHeaders` giữ `x-delivery-count` / `x-ef-publish-id` / `x-first-death-*` / `x-death` | **Giết** (4) | — | |
| C12 | `cloneHeaders` trả map gốc | **Giết** | — | |
| C13 | bỏ `recover` | **Giết** | — | |
| C14–C16 | `validate` bỏ kiểm DLX / cặp Retry / mặc định Prefetch | **Giết** (3) | — | |
| C17 | bỏ nhánh "ctx huỷ → nack" | Sống | Sống | Tương đương: `Publish` kiểm ctx trước → nack requeue → lỗi bọc `Canceled` |
| C18 | retry không tăng attempt | Sống | **Giết** (`LoiBaLan`: attempts [1 2 3]) | |
| C19 | DLQ `attempt+1` | Sống | **Giết** (DLQ `x-ef-attempt=3`/`1`) | |
| C20 | `canRetry` luôn true | Sống | **Sống** | F4 |
| C21 | forward `Nack(requeue=false)` | Sống | **Giết** (`PublishRetryLoi`: hàng gốc phải còn 1) | |
| C22 | forward lỗi → ack | Sống | **Giết** (Consume phải trả lỗi) | |
| C23 | handler nhận ctx không có span | Sống | **Giết** (trace ID handler) | |
| C24 | publish retry bằng ctx không có span | Sống | **Giết** (trace lần 2, 3) | |
| C25 | `Qos(0)` | Sống | **Sống** | F4 |
| C26 | bỏ `Cancel` khi thoát | Sống | **Sống** | F4 |
| C27 | deliveries đóng → `nil` | Sống | **Sống** | F4 |
| C28 | ctx huỷ → `Consume` trả `nil` | Sống | **Giết** (`errors.Is(Canceled)`) | |
| C29, C30, C33 | `Attempt` luôn 1 / `forwardMessage` bỏ CorrelationID / bỏ Timestamp | **Giết** (3) | — | |
| C31 | `x-ef-attempt` int64 thay int32 | Sống | Sống | Tương đương trong thư viện (hợp đồng `docs/02` ghi int32 nhưng không có test) |
| C32 | `Ack(multiple=true)` | Sống | Sống | Tương đương vì xử lý tuần tự |
| P1, P2, P3, P8 | mandatory=false / không gắn publish-id / mọi return đều tính / bỏ persistent | **Giết** (4) | — | |
| P4 | bỏ pre-drain | Sống | Sống | Gần tương đương (`collect(id)` sau vẫn đọc hết) |
| P5 | `!acked` chỉ dựa vào returns đóng | Sống | Sống | Tương đương: `closeResources` đóng returns (`channel.go:2219`) trước `confirms.Close` (`:2240`) |
| P6 | lỗi publish khi kênh đóng không map `ErrClosed` | Sống | **Sống** | Chỉ có cửa sổ race; Thông tin |
| P7 | bỏ kiểm ctx đầu `Publish` | Sống | Sống | Tương đương: `PublishWithDeferredConfirmWithContext` kiểm ctx (`channel.go:1956-1962`) |
| P9–P13 | `Publish` bỏ CorrelationId / Timestamp+Type+AppId / ReplyTo / Priority / ContentEncoding | Sống (5) | **Sống** (5) | F2 |
| P14 | bỏ mutex | Sống | **Sống** | F3 (X2 giết được) |
| P15–P18 | publish-id cố định / `ErrClosed` không bọc / bỏ kiểm return / nack → nil | **Giết** (4) | — | |
| R1, R2 | không inject trace / span consume sai kind | **Giết** (2) | — | |
| T1, T2, T4, T5, T6 | exchange autoDelete / bỏ args queue / TTL lệch / bỏ at-least-once / DLQ bind `#` | **Giết** (5) | — | |
| T3 | `Declare` nuốt lỗi queue | Sống | Sống | F5 |
| T7 | TTL int32 | Sống | Sống | Tương đương |
| D1–D4 | JSON: TTL là chuỗi / bỏ at-least-once / `60000.0` / exchange `auto_delete` | **Giết** (4) | — | |
| D5–D7 | JSON: thêm policy / khoá trùng / trường lạ | Sống (3) | — | F6 |

**Điểm:**

| Cách tính | Kết quả |
|---|---|
| Chỉ unit | 37/67 = **55%** |
| Unit + tích hợp (suy luận) | 44/67 = **66%** |
| Bỏ 8 mutant tương đương | 44/59 = **75%** |

## 4. Các đòn không phá được

- **Race:** `-race -count=20` sạch.
- **Rò goroutine:** package không tạo goroutine nào (không có `go` trong file nguồn). X4 chạy 200 `Publish` bị huỷ ctx
  khi đang chờ confirm, số goroutine không tăng. Mutex được nhả sau khi huỷ (có cả test sẵn).
- **Header phình qua retry:** X3 forward 10 lần, mỗi lần thêm `x-death`/`x-delivery-count` giả. Số header luôn là 4
  (`tenant`, `x-ef-attempt`, `traceparent`, `x-ef-publish-id`). Header không lớn dần nên không vượt `frame_max`.
- **Message độc do kiểu header:** mọi kiểu amqp091 giải mã được (`read.go`: `t B b s I l u i f d D S A T F x V`) đều qua
  được `validateField` (`types.go:416-439`). Bản forward không bao giờ bị `Table.Validate` từ chối.
- **Thứ tự return/ack:** return được dispatch tuần tự trước ack trên cùng goroutine (`channel.go:511-523`). Khi bộ đệm
  chưa đầy thì đọc không chặn sau `WaitContext` là đáng tin. Return cũ có id khác bị bỏ qua (P3 bị giết).
- **Kênh đóng khi đang chờ (B1 cũ):** `r, ok := <-p.returns` thoát ngay. Có 2 unit test với giới hạn 2 s, và CI
  `PublishExchangeKhongTonTai` chạy 0.00 s.
- **`basic.cancel` khi queue bị xoá:** theo mã, `Consume` trả lỗi chứ không treo (xem F4: đúng nhưng chưa có test).
- **Biên và tràn của `decide`/`attemptOf`:** đã giết mọi mutant không tương đương.
- **`definitions.json`:** đổi kiểu số thành chuỗi, `60000.0`, bỏ một arg, `auto_delete`, binding trùng (`uniqueIndex`
  có test) đều bị bắt.
- **Cờ redelivered:** bị bỏ qua có chủ đích. Attempt lấy từ header nên redelivery sau crash không làm tăng đếm. Message
  có thể trùng (at-least-once, đã ghi trong `doc.go`), không mất.
- **CI:** bước `libs/go/mq` có 11/11 `TestTichHop_*` PASS trên RabbitMQ 3.13 thật, không SKIP. `go vet -tags=integration`
  sạch.

## 5. Dọn dẹp

Mọi file bị mutate đều được trả lại (script ghi lại nội dung gốc trong `finally`; P14 chạy tay thì chép lại từ bản sao).
Đã xoá `mq/zz_challenge_test.go`. `git status` chỉ còn file này (`challenge.md`). Không commit, không push.
