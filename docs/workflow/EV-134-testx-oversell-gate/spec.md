# EV-134 — `libs/go/testx` + cổng chống oversell chạy thật (Spec)

| Mục | Giá trị |
|---|---|
| Jira | EV-134 (mã backlog **EVF-39**, issue 1310) — 5 SP — Highest — phụ thuộc EVF-32, EVF-34 (đã có code) |
| Kế hoạch tổng | [09-ke-hoach-hoan-thien-backend.md](../../09-ke-hoach-hoan-thien-backend.md) — Giai đoạn 1, việc W3 |
| Branch | `feature/EV-134-testx-oversell-gate` (từ `develop`) |
| Quy tắc nghiệp vụ | BR-O1 (không oversell), BR-O7 (trả kho idempotent) — [01-nghiep-vu.md](../../01-nghiep-vu.md) |
| Khảo sát | `develop` @ `0108c5f`, ngày 2026-10-13 |

## 1. Bối cảnh

Jira AC: (1) chạy 200 lần trong CI, mỗi lần đúng 100 đơn thành công; (2) 0 oversell, 0 under-sell;
(3) test fail thì chặn merge và chặn release.

Hiện trạng: cổng `Oversell Gate (EVF-39)` **xanh mà không kiểm gì**. `setupEnv` trong
`services/ticketing/test/concurrency/helpers.go:33` gọi `t.Skip`, nên 200 lần chạy cho ra 400 SKIP, 0 PASS
(đã ghi ở `docs/workflow/ci-cd-review/spec.md` mục M1). `libs/go/testx/` rỗng. Bảy helper còn lại là `panic("TODO")`.

Slice này làm hai việc: dựng `libs/go/testx` (hạ tầng thật qua testcontainers, dùng chung cho các việc W4–W6),
và hiện thực `helpers.go` để hai test `TestNoOversell_*` chạy thật.

## 2. Phát hiện khi khảo sát

Các điểm này quyết định thiết kế; chi tiết xử lý ở `plan.md`.

| # | Phát hiện | Hệ quả |
|---|---|---|
| K1 | `TestNoOversell_WithConcurrentExpiry` có `time.Sleep(3s)` × 5 vòng viết cứng → tối thiểu 15 giây mỗi lần. Lệnh cổng hiện tại (`-run TestNoOversell -count=200 -timeout=30m`) khớp **cả hai** test | 200 lần riêng test hết hạn đã ≥ 50 phút, vượt `-timeout=30m` và `timeout-minutes: 40`. Cổng **không thể xanh** với cấu hình hiện tại, kể cả khi code đúng |
| K2 | `setupEnv` được gọi mỗi lần test chạy; `-count=200` là 400 lần gọi | Khởi động container mỗi lần gọi là hàng chục phút chỉ để dựng hạ tầng → container phải dùng chung trong một tiến trình, cô lập bằng `event_id` mới |
| K3 | `SeedIdentity` và `AdmitToken` được gọi **trong** 10.000 goroutine, trước `<-start` | `t.Fatal` không được gọi ngoài goroutine của test → hai helper này không được làm I/O có thể lỗi; phải nạp sẵn hàng loạt |
| K4 | Phép kiểm cuối của test hết hạn đọc `TotalAvailable` rồi `TotalHeldOrSold` bằng hai truy vấn, trong khi sweeper còn chạy | Sweeper trả kho giữa hai truy vấn → tổng < quota → đỏ giả. Với chu kỳ quét 1 giây và TTL 2 giây, đợt quét cuối rơi đúng quanh mốc 3 giây |
| K5 | Test hết hạn chỉ kiểm tổng trong **Postgres**, không kiểm lệch Redis | Trả kho hai lần ở Redis (hỏng `release.lua`) không làm test đỏ — điểm mù, xem AC-6 |
| K6 | `cmd/server/main.go` đặt Redis `ReadTimeout` 200 ms. Script `hold` đã chạy trên server nhưng client timeout → `CreateHold` trả lỗi, không đền bù | Dưới tải, có thể mất vé ở Redis cho tới khi có job đối soát (EVF-37, chưa có). Test có thể đỏ **vì lỗi thật** ở bất biến 1, 3 hoặc 4 |
| K7 | PR #52 (`feature/EVF-7-libs-go-mq`, đang mở) đã thêm `testcontainers-go v0.44.0` vào `libs/go/go.mod` và sửa `integration.yml` | Hai nhánh đụng cùng `go.mod`, `go.sum`, `integration.yml` → cần chốt thứ tự merge |
| K8 | `run-and-report-tests.sh` chỉ **cảnh báo** khi 0 PASS, vẫn exit 0 | Đổi tên test hoặc sai `-run` là cổng lại xanh câm. Quyết định "biến thành fail cứng" trước đây để chờ EVF-39 |
| K9 | Máy khảo sát không có Go trong PATH, Docker daemon không chạy | Không kiểm được tại chỗ cho tới khi cài; bằng chứng chính lấy từ CI |

## 3. Tiêu chí nghiệm thu

**AC-1 — `testx` dựng hạ tầng thật.** `testx.Postgres`, `testx.Redis`, `testx.RabbitMQ` khởi động container bằng
đúng ảnh của `deploy/compose/docker-compose.yml` (`postgres:16-alpine`, `redis:7-alpine`,
`rabbitmq:3.13-management-alpine`), mỗi loại một lần cho cả tiến trình test. Postgres đã chạy mọi file trong
`migrations/` (phần `-- +goose Up`). Không có Docker thì test **FAIL**, không skip.

**AC-2 — Không còn TODO, không sửa phép kiểm.** `helpers.go` không còn `t.Skip` hay `panic("TODO")`. Các phép
kiểm (bốn bất biến của test 1, phép kiểm tổng của test 2), hằng `quota = 100`, `concurrency = 10_000` trong
`oversell_test.go` giữ nguyên. Nếu test đúng nghiệp vụ mà đỏ: báo bug, không sửa test cho xanh.

**AC-3 — Không oversell, không under-sell.** `TestNoOversell_UnderExtremeConcurrency` PASS: đúng 100 lần thành
công; tồn kho Postgres = 0; Redis lệch Postgres = 0; 9.900 lần còn lại đều nhận `ErrSoldOut`.

**AC-4 — Trả kho idempotent khi có hai sweeper.** `TestNoOversell_WithConcurrentExpiry` PASS với hai sweeper song
song qua 5 vòng giữ-và-nhả: tồn kho + đang giữ = 100.

**AC-5 — 200 lần trong CI.** Bảng tổng kết của `run-and-report-tests.sh` trên một lần chạy `oversell-gate`:
test 10.000 goroutine **200 PASS, 0 FAIL, 0 SKIP**. Test hết hạn chạy số lần theo quyết định D1.

**AC-6 — Cổng thực sự cắn.** Mỗi đột biến trong bảng ở `plan.md` §6 làm test đỏ ở đúng bất biến dự kiến; đột biến
nào sống sót được ghi thành phát hiện kèm đề xuất. Không commit đột biến.

**AC-7 — Fail thì chặn.** 0 PASS ở cổng oversell là **exit khác 0** (không còn chỉ cảnh báo). Workflow
`oversell-gate` đỏ khi test đỏ. Việc đặt nó thành required check trên `main` là cấu hình repo, do trưởng nhóm làm.

**AC-8 — Chất lượng.** `gofmt`, `go vet`, `go test -race` (không tag) pass ở `libs/go` và `services/ticketing`;
golangci-lint 0 lỗi kể cả với `--build-tags integration`; 4 binary build được với `-mod=readonly`; hai image Go
build được; Trivy không CRITICAL; `test-workflows.sh` và `test-run-and-report-tests.sh` pass.

## 4. Ngoài phạm vi

- Job đối soát tồn kho và metric `inventory_drift` (EVF-37) — K6 nếu xảy ra thì ghi bug, sửa ở đó.
- Sửa lõi `hold.lua`, `release.lua`, `repository.go`, `hold_sweeper.go`. Slice này chỉ **đo**; lỗi tìm thấy mở issue riêng.
- Chuyển test tích hợp của `libs/go/mq` sang dùng `testx.RabbitMQ` (làm cùng EVF-36).
- `idem` (EV-128), `outbox` (EV-131).
- Mở cổng coverage 70% cho `libs/go`: `testx` cần Docker nên không đo được trong job `test-go`.
- Test tải qua HTTP (k6) — giai đoạn 9.
