# EV-120 (bugfix) — Bằng chứng

Ngày chạy: 2026-10-18. Máy: Windows 11, Docker Desktop (engine 29.6.2). Go chạy trong `golang:1.26` (go1.26.9,
Debian — có gcc cho `-race`), mount Docker socket cho testcontainers. Nhánh `bugfix/EV-120-moderation-outbox-relay`
trên `feature/EV-157-ai-worker-consumers` @ `9065edb`.

**Chưa có bằng chứng CI (Linux):** nhánh chưa được đẩy.

## 1. Tái hiện trước khi sửa

| Lỗi | Cách tái hiện | Kết quả trên mã cũ |
|---|---|---|
| O4 | `TestOutboxRelay_Integration_KhongChamDongCuaServiceKhac`: dòng `event/moderation` chèn trước dòng `order` | **FAIL** sau 5 giây: "dong 'order' khong duoc phat: relay ticketing bi ket boi dong cua service khac" |
| O1 | Đột biến G1 (đặt lại key `ai.moderation.q`) trên mã mới | Đỏ ở `TestHTTP_AddTier_And_Submit_BR_E1` và `TestTichHop_NopDuyet_JobToiHangKiemDuyet` |
| O2, O5 | Mã cũ không có relay, không có `UpdateWithOutbox` (test không biên dịch được trên mã cũ) | Đột biến G2, G6 thay vai trò tái hiện — §3 |

## 2. Kết quả

| Kiểm | Kết quả |
|---|---|
| event: `go vet ./...`, `go vet -tags=integration ./...`, `go build ./...` | sạch |
| event: `go test -race ./...` | PASS — `adapter/http`, `app`, `domain`, `worker` |
| event: tích hợp (`-tags=integration`, Postgres 16 + RabbitMQ 3.13) | PASS — `TestTichHop_NopDuyet_JobToiHangKiemDuyet`, `TestTichHop_RoutingKeyLa_KhongChanDongSau`, `TestTichHop_UpdateWithOutbox_HongGiuaChungThiKhongGhiGi` |
| ticketing: `go test -race ./...` | PASS |
| ticketing: toàn bộ tích hợp `go test -tags=integration ./... -timeout=15m` | PASS — 9 package, kể cả `test/concurrency` |
| **EVF-39**: `go test -v -tags=integration -run TestNoOversell ./test/concurrency/... -count=20` | **40/40 PASS** (`UnderExtremeConcurrency` ×20, `WithConcurrentExpiry` ×20), 0 SKIP, 490 giây |
| Cổng coverage `domain/` + `app/` (`check-domain-coverage.sh … 70`) | event **78,7%**, ticketing **98,2%** |
| golangci-lint v2.12.2 (bản LF) | ticketing **0 issues**. event: 7 issues, **đều có sẵn trên `develop`** (4 `rows.Close`, 1 `ifElseChain`, gofmt `domain/event.go`, `domain/ticket_tier.go`); lỗi do slice này gây ra (`exitAfterDefer`, gofmt 3 file) đã sửa |
| `go.mod` event | dòng `go 1.25.0` giữ nguyên, không có `toolchain`; thêm `amqp091-go` v1.15.0 và phụ thuộc test của `libs/go/testx` |
| `.github/scripts/tests/test-workflows.sh` | 137 PASS, 0 FAIL |

## 3. Thử phá (đột biến)

| # | Đột biến | Kết quả | Test đỏ |
|---|---|---|---|
| G1 | Key về lại `ai.moderation.q` | Đỏ | `TestHTTP_AddTier_And_Submit_BR_E1` (và test tích hợp nộp duyệt) |
| G2 | `SubmitForReview` dùng `Update` thường (không ghi job) | Đỏ | `TestHTTP_AddTier_And_Submit_BR_E1` |
| G3 | Relay event bỏ lọc `aggregate_type` | **Sống sót lần đầu → siết test → Đỏ** | `TestTichHop_NopDuyet_JobToiHangKiemDuyet` |
| G4 | Key không có tuyến thì `break` thay vì bỏ qua | Đỏ | `TestTichHop_RoutingKeyLa_KhongChanDongSau` |
| G5 | Relay ticketing bỏ lọc `aggregate_type` | Đỏ | `TestOutboxRelay_Integration_KhongChamDongCuaServiceKhac` |
| G6 | Ghi outbox bằng `r.db` thay vì `tx` (ngoài transaction) | **Sống sót lần đầu → thêm test → Đỏ** | `TestTichHop_UpdateWithOutbox_HongGiuaChungThiKhongGhiGi` |

- **G3**: bảng định tuyến tình cờ là lớp lọc thứ hai (dòng `order` không có tuyến → bỏ qua), nên test đầu không
  thấy. Nhưng còn một lỗi thật: ≥ `Batch` dòng của service khác đứng trước thì mọi lô toàn dòng bị bỏ qua và job
  kiểm duyệt không bao giờ tới lượt. Test tích hợp giờ chèn 12 dòng `order` (> Batch 10) trước khi nộp duyệt.
- **G6**: đường thuận không phân biệt được. Test mới cho message thứ hai hỏng (payload không marshal được) và kiểm
  sự kiện vẫn `DRAFT`, không còn dòng outbox nào.

## 4. Giới hạn đã biết

- Chưa chạy event service trên compose (chưa có trong `docker-compose.yml` — X3). Đường đầu-cuối được chứng minh
  bằng test tích hợp trên Postgres + RabbitMQ thật; worker EV-157 đã được chạy thật ở evidence của EV-157.
- Dòng outbox có routing key không có tuyến nằm lại vĩnh viễn (có log lỗi). Đây là lỗi lập trình, không phải dữ
  liệu khách; chưa có metric riêng.
