# EVF-36 — Challenge (người phản biện thứ hai, ngữ cảnh sạch)

Commit: `cb4f598` (branch `feature/EVF-36-outbox-relay`, PR #55). Phạm vi tấn công: `migrations/0002_outbox_relay.sql`, `libs/go/outbox/*`,
`services/ticketing` (`repository.go`, `worker/hold_sweeper.go`, `cmd/worker/main.go`), compose, `integration.yml`.
Postgres/RabbitMQ thật không chạy cục bộ (Docker tắt); kết quả thật lấy từ CI run 38346290492 (29/29 `TestTichHop` pass, theo lời người giao việc, không đọc lại log).

## 1. Verdict: PASS (không có blocker, không có major)

Không tìm được kịch bản mất event, trùng lặp vượt cận `BatchSize`, hay làm relay/worker chết do dữ liệu. Có 4 finding minor và
một số lỗ hổng test (mutant sống) ở §3; không cái nào chặn merge. Điều kiện merge vẫn là người dùng duyệt migration (r10).

`go test -race -count=20 ./outbox/...` (libs/go): `ok ... 47.4s` (20 lần, không race, không flake).

## 2. Findings

| # | Mức | Mô tả | Repro / lập luận | Kết quả |
|---|---|---|---|---|
| F1 | minor | `markRetrySQL`/`markParkSQL` chỉ có `AND published_at IS NULL`, không có `next_attempt_at <> 'infinity'`. Nếu relay A bị đứng quá lease, relay B claim lại và đỗ row, rồi A mark retry ⇒ gỡ đỗ nhầm và `attempts` cộng hai lần. | Lập luận: `relay.go` hằng `markRetrySQL`/`markParkSQL` so với `releaseSQL` (đã có guard). Không tái hiện bằng fake (fake không mô phỏng hai relay). | Khó xảy ra vì bất biến `BatchDeadline + PublishTimeout + 2*5s < Lease`; hậu quả chỉ là thêm vài lần thử. Đề xuất thêm guard cho retry. |
| F2 | minor | `Write` không từ chối NUL trong `data`: `json.Marshal("x\x00y")` ra `"x\u0000y"`, mà PostgreSQL `jsonb` không lưu được `\u0000` ⇒ INSERT lỗi ⇒ cả transaction nghiệp vụ (`PersistHold`/`ReleaseHold`) lỗi. | Throwaway test: `json.Marshal(map[string]any{"a":"x\x00y"})` in `{"a":"x\u0000y"}`. Phần hành vi jsonb của PG16 là lập luận (không chạy được PG cục bộ). | Hiện không khai thác được: `data` của ticketing chỉ có UUID, số, thời gian. Bẫy cho người dùng `Write` sau này; đề xuất kiểm `\u0000` hoặc ghi chú trong doc.go. |
| F3 | minor | Histogram `outbox_publish_lag_seconds` dùng `time.Since(row.createdAt)`: `createdAt` là đồng hồ DB, `time.Now()` là đồng hồ Go ⇒ lệch đồng hồ cho lag sai/âm (rơi vào bucket thấp nhất, không báo). | `relay.go` (`r.m.lag.Observe(time.Since(row.createdAt).Seconds())`). Lease/backoff đều dùng `now()` của DB nên không bị ảnh hưởng. | Chỉ ảnh hưởng số đo, không ảnh hưởng đúng đắn. |
| F4 | minor | Mutant sống (§3) cho thấy hành vi chưa có test khẳng định: trả lease khi hết hạn lô/huỷ ctx giữa chừng; "huỷ ctx / hết hạn lô không tính attempt" (R1); backoff dial lại; đúng 64 KiB; `AggregateType == ""`; danh sách khoá header. | Mutation, bảng §3. | Code hiện đúng (xem §4) nhưng sửa nhầm sau này sẽ không bị test bắt. Đề xuất bổ sung 5-6 test nhỏ. |

Không phải finding: `connDead` kiểm `IsClosed` của publisher là nhánh chết trong production (`*mq.Publisher` không có `IsClosed`;
`*amqp091.Connection` qua `closer` là nhánh thật), nên mutant M11 là tương đương.

## 3. Bảng mutant

Cách chạy: sửa file bằng perl/sed, `go build ./outbox/`, `go test -count=1 ./outbox/` (unit, fakes), rồi `git checkout -- <file>`.
Mutant NOOP/BUILDFAIL được viết lại hoặc loại; chỉ đếm mutant biên dịch được và thực sự đổi file.

| ID | Mutation | Kết quả |
|---|---|---|
| M1 | park `next >= MaxAttempts` thành `>` | killed |
| M2 | backoff bỏ clamp số mũ 20 | killed |
| M3 | backoff bỏ trần `ceiling` | killed |
| M4 | backoff `attempts-1` thành `attempts` | killed |
| M5 | `releaseSQL` bỏ guard `<> 'infinity'` | killed |
| M6 | `releaseSQL` bỏ guard `published_at IS NULL` | killed |
| M7, M8, M9 | `markPublished/Retry/Park` bỏ `AND published_at IS NULL` | killed (cả ba) |
| M10 | bỏ kiểm `connDead` trước claim | killed |
| M11 | `connDead` bỏ nhánh `r.pub.IsClosed` | survived (tương đương trong production) |
| M12 | `connDead` bỏ nhánh `r.closer.IsClosed` | killed |
| M13 | không cap publish theo hạn chót lô | killed |
| M14 | allowlist bypass (`if true`) | killed |
| M15 | bỏ counter `outbox_unroutable_total` | killed |
| M16 | sanitize bỏ `ToValidUTF8` | killed |
| M17 | sanitize bỏ loại `\x00` | killed |
| M18 | sanitize bỏ cắt 1 KiB | killed |
| M19 | sanitize cắt giữa rune (`RuneStart` vô hiệu) | killed |
| M20 | lỗi mark: trả lease `rows[i:]` thay vì `rows[i+1:]` | survived (row lỗi mark được gửi lại ngay thay vì sau lease; vẫn at-least-once) |
| M21 | `ErrClosed` tính attempt | killed |
| M22 | huỷ ctx giữa publish tính attempt | survived (không test khẳng định R1) |
| M23 | hết hạn lô giữa publish tính attempt | survived |
| M24 | mark dùng `ctx` thay vì `WithoutCancel` | killed |
| M25 | metric published ghi trước mark | killed |
| M26 | bỏ `recover` | killed |
| M27 | bỏ kiểm hết hạn lô | killed |
| M28 | claim bỏ sắp xếp theo id | survived (thứ tự không được bảo đảm theo spec) |
| M29 | `outbox_pending` đếm cả row đỗ | survived (fake không kiểm SQL gauge) |
| M30 | backoff dial lại cố định 1s | survived |
| M31 | `fails` không reset khi lô rỗng | survived |
| M32 | bỏ kiểm `BatchDeadline+PublishTimeout+10s < Lease` | killed |
| M33 | bỏ trần `BatchSize <= 1000` | killed |
| M34 | bỏ trần `MaxBackoff <= 1h` | killed |
| M35 | `newMetrics` không `Unregister` khi đăng ký lỗi | survived |
| M36 | bỏ `SET LOCAL statement_timeout` | survived (fake không kiểm) |
| M37 | dialer trả publisher nil không đóng closer | survived |
| M38 | bỏ `trace_id` trong phong bì | killed |
| M39 | bỏ `MessageID` | killed |
| M40 | bỏ `ExtractAMQP` (mất liên kết trace) | killed |
| M41 | `closeConn` không đặt gauge connected=0 | killed |
| M42 | bỏ `lag.Observe` | survived |
| M43 | bỏ counter `publish_errors_total` | survived |
| M44 | bỏ giải mã headers | killed |
| M45 | bỏ `json.Valid(payload)` | survived (tương đương: cột `jsonb` luôn hợp lệ) |
| M47 | `resBatchEnd` không trả lease | survived |
| M48 | `resCanceled` không trả lease | survived (còn nhánh `ctx.Err()` đầu vòng che) |
| M49 | `resClosed` không trả lease | killed |
| W1 | `Write`: `> 64 KiB` thành `>=` | survived (không test đúng 65536 byte) |
| W2 | bỏ kiểm kích thước | killed |
| W3 | bỏ kiểm UUID | killed |
| W4 | bỏ kiểm routing key | killed |
| W5 | regexp `{2,}` thành `{1,}` | killed |
| W6 | bỏ kiểm `AggregateType == ""` | survived |
| W7 | chỉ copy `traceparent` (bỏ `tracestate`, `baggage`) | survived |
| W8 | không lưu `trace_id` | killed |
| W9, W10 | UUID v4 sai bit version/variant | killed (cả hai) |

Điểm mutant: 39 killed / 58 hợp lệ = 67%; loại 2 mutant tương đương (M11, M45): 39/56 = 70%. 19 mutant sống; các mutant
liên quan bất biến "không mất event" (M6-M9, M21, M24, M27, M49) đều bị giết. Mutant sống đáng chú ý nhất: M22, M23, M47, M48 (R1 và trả lease).

## 4. Các tấn công không phá được

- Chết giữa claim và publish / giữa publish và mark: lease dùng `now()` của DB; row chỉ quay lại sau lease, trùng tối đa `BatchSize`, không mất (CI kill-restart và biến thể TCP-only xanh).
- Mark `UPDATE` khớp 0 row: chỉ khi row đã `published_at`, an toàn nhờ `AND published_at IS NULL`.
- Give-back đua với relay khác: `releaseSQL` có `published_at IS NULL` và `<> 'infinity'` (M5, M6 bị giết).
- Lệch đồng hồ app/DB: lease, backoff, `published_at`, `released_at` đều dùng `now()` của DB; deadline Go chỉ là khoảng tương đối (F3 chỉ ảnh hưởng metric).
- Confirm trễ sau timeout: `mq.Publisher` giữ mutex suốt publish và gắn `x-ef-publish-id`; amqp091 theo dõi `DeferredConfirmation` theo delivery tag nên ack muộn không bị gán nhầm cho message kế tiếp.
- Đỗ/gỡ đỗ: `'infinity'` bị loại khỏi claim (`<= now()`), khỏi `outbox_pending` và khỏi release; `attempts` chỉ tăng đến `MaxAttempts` rồi đỗ, không tràn INT.
- Allowlist unroutable: key ngoài `DropUnroutable` thì retry, không bị đánh dấu đã gửi (M14 killed); `hold.released` không bao giờ bị bỏ.
- Write validation: regexp neo `^...$`, UUID chỉ dạng có gạch, `data` kiểm sau Marshal; `data=nil` lưu `null` (JSON hợp lệ).
- Header poisoning/trace: relay chỉ phát header string từ cột `headers` (giải mã lỗi thì tính attempt rồi đỗ, không kẹt); `traceparent` sai bị OTel bỏ qua (mất liên kết trace, không panic); baggage bị OTel giới hạn 8 KiB nên không làm header frame vượt frame-max.
- Nhãn metric: chỉ `routing_key` từ tập hữu hạn đã validate.
- Redial hot-spin: dialer "kết nối chết ngay", Interval 10 ms cho 49 dial/s, tức một dial mỗi hai chu kỳ `Interval`; với `Interval` mặc định 500 ms là khoảng 1 dial/s, không quay nóng. Lỗi dial/`ErrClosed` dùng backoff 1s-30s.
- Rò goroutine trong Run: `gaugeLoop` được `wg.Wait()` trong defer; `sleep` huỷ được theo ctx.
- SIGTERM giữa lô: mark chạy dưới `WithoutCancel` (M24 killed), phần chưa xử lý được trả lease; `runLoops` huỷ ctx chung và chờ mọi vòng lặp.
- Panic một row: `recover` trong `publishRow` (M26 killed). Panic ngoài đó làm chết cả process worker (không riêng sweeper); chấp nhận được vì orchestrator khởi động lại và outbox bền.
- Rò thông tin xác thực: `loadConfig` trả lỗi cố định không chứa URL; `redactErr` thay URL và mật khẩu; `newPoolConfig` trả thông báo cố định.
- ReleaseHold: `UPDATE ... FROM orders ... RETURNING` và `ErrNoRows` trả `(false, nil)` nên không ghi outbox cho hold không thực sự được giải phóng; id canonical; ghi outbox lỗi thì rollback cả giải phóng. PersistHold: `outbox.Write` trước `Commit`, `defer Rollback` nên rollback không để lại row.
- Migration: `SET LOCAL lock_timeout` trong transaction của goose; ADD COLUMN default volatile ghi lại bảng nhỏ (đã tài liệu hoá); INSERT kiểu cũ vẫn chạy (mọi cột mới có DEFAULT); `CREATE INDEX` không CONCURRENTLY chặn ghi trong lúc build trên bảng nhỏ.

## 5. Dọn dẹp

Mọi mutation đã hoàn tác bằng `git checkout`; test throwaway đã xoá; `git status` chỉ còn file này.
