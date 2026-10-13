# EV-134 — Kế hoạch thực thi (Plan)

Đầu vào: `spec.md` (AC-1..AC-8, phát hiện K1..K9). Base `develop` @ `0108c5f`.
Thời lượng trong tài liệu này là **ước tính từ đọc code**, chưa đo; bước 3 đo lại và ghi vào `baseline.md`.

## 0. Quyết định cần chốt trước khi code

Kế hoạch bên dưới viết theo cột "Đề xuất". Đổi quyết định nào thì sửa mục tương ứng.

| # | Câu hỏi | Đề xuất | Lý do |
|---|---|---|---|
| D1 | Cổng 200 lần không vừa 30 phút (K1). Xử lý thế nào? | **Tách theo test:** test 10.000 goroutine × 200, test hết hạn × 20 | AC của Jira nói về test 10.000 goroutine. Test hết hạn tốn ≥ 15 giây ngủ mỗi lần; 200 lần là ~1 giờ cho một cổng chạy trên mọi PR vào `main` |
| | Phương án khác | Giữ nguyên lệnh, nâng `-timeout` lên ~100 phút | Không đụng phạm vi test, nhưng cổng `main` dài ~80 phút |
| D2 | 0 PASS ở cổng oversell: cảnh báo hay fail? (K8) | **Fail cứng** | Lý do hoãn trước đây ("chặn mọi PR vào `main` tới khi EVF-39 xong") hết hiệu lực khi slice này merge |
| D3 | Thứ tự với PR #52 (K7) | **Merge #52 trước**, rồi mới tách nhánh | Tránh xung đột `go.mod`/`go.sum`/`integration.yml`; `testx.RabbitMQ` dùng lại module rabbitmq #52 đã thêm |
| | Nếu #52 chưa merge kịp | Tách từ `develop`, hoãn `testx.RabbitMQ` sang EV-131 | Cổng oversell chỉ cần Postgres + Redis |
| D4 | Pool trong test: giống production hay nới rộng? (K6) | **Giống `cmd/server/main.go`** (pgx `MaxConns` 25; Redis `PoolSize` 256, timeout 200 ms) | Bất biến 4 là cam kết nghiệp vụ. Timeout dưới tải là phát hiện cần báo, không phải nhiễu cần giấu |
| D5 | Sửa lỗi thật nếu test bắt được? | **Không sửa trong slice này**; mở issue, cổng đỏ cho tới khi sửa | Giữ slice nhỏ; đúng nguyên tắc ở `docs/09` mục 7 |

## 1. Hợp đồng — package `github.com/eventflow/eventflow/libs/go/testx`

Gói thường trong module `libs/go` (không tách module riêng: #52 đã đưa testcontainers vào `go.mod` này).
File **không** gắn build tag: chỉ được import từ file `//go:build integration`, nên không vào binary, nhưng vẫn
được `go vet` và golangci-lint quét trong job `lint-go`.

```go
// postgres.go
// Postgres khoi dong (mot lan cho ca tien trinh) postgres:16-alpine, chay moi migration va tra ve DSN.
// Khong co Docker -> t.Fatalf (khong skip, hop dong K1 cua CI).
func Postgres(t testing.TB, opts ...PostgresOption) string
func WithMigrations(dir string) PostgresOption // mac dinh: <goc repo>/migrations

// redis.go
func Redis(t testing.TB) string // "host:port" cua redis:7-alpine

// rabbitmq.go (phu thuoc D3)
func RabbitMQ(t testing.TB) string // AMQP URL cua rabbitmq:3.13-management-alpine

// testx.go
func RepoRoot(t testing.TB) string // di nguoc tu thu muc test toi thu muc chua go.work
func Shutdown()                    // go container; goi trong TestMain sau m.Run()

// goose.go -- thuan, unit test khong can Docker
func gooseUp(sql string) (string, error) // phan giua "-- +goose Up" va "-- +goose Down"; thieu Up -> loi
func migrationFiles(dir string) ([]string, error) // *.sql, sap theo ten
```

- Mỗi loại container một `sync.Once`; lỗi khởi động được nhớ lại để mọi test sau fail ngay, không chờ lại.
- Phiên bản: `testcontainers-go v0.44.0` cùng `modules/postgres`, `modules/redis`, `modules/rabbitmq` (khớp #52).
- Không dùng thư viện goose: chỉ cần phần Up của file SQL thuần, tránh thêm dependency.
- Phong cách theo `libs/go`: comment tiếng Việt không dấu, `fmt.Errorf("...: %w")`, test chỉ dùng `testing`.

## 2. Hiện thực `services/ticketing/test/concurrency/helpers.go`

`TestEnv` thêm trường không export: `pool *pgxpool.Pool`, `rdb *redis.Client`, `sweepers sync.WaitGroup`.
`Repo` và `Gate` là adapter **thật** (`postgresadapter.NewRepository`, `redisadapter.NewGate`) — script Lua được
nạp bằng chính đường production.

| Helper | Cách làm | Ghi chú |
|---|---|---|
| `setupEnv` | `testx.Postgres` + `testx.Redis`; tạo pool và client theo D4; nạp 10.000 identity một lần (`generate_series`, `ON CONFLICT DO NOTHING`); `t.Cleanup` đóng pool/client **sau khi** `sweepers.Wait()` | Container dùng chung cả tiến trình (K2) |
| `SeedEvent` | `event_id` ngẫu nhiên, `slug` duy nhất, trạng thái `ON_SALE`; hạng vé `quota`, 32 bucket chia theo công thức của `deploy/scripts/seed.sh`; nạp Redis qua `repo.ListInventory` → `gate.SeedInventory`; `ZADD` sẵn 10.000 token admit (hạn 1 giờ) bằng một pipeline | Cô lập giữa 200 lần chạy bằng `event_id`, không truncate |
| `SeedIdentity(n)` | Trả UUID xác định `00000000-0000-4000-8000-<n 12 chữ số>`; **không I/O** | K3 |
| `AdmitToken` | Trả token xác định theo identity; **không I/O** (đã admit ở `SeedEvent`) | K3; goroutine tới `<-start` gần như cùng lúc |
| `TotalAvailable` | `SUM(available)` của `ticket_inventory` theo hạng vé | |
| `TotalHeldOrSold` | `SUM(quantity)` của `seat_holds` có `released_at IS NULL` | Đang giữ + đã consume |
| `InventoryDrift` | Tổng `HVALS inv:{ev}:tt:<tt>` trừ `SUM(available)` Postgres | Có dấu: dương = Redis phồng |
| `SetHoldTTL` | `UPDATE events SET hold_ttl_seconds` | `GetSaleConfig` đọc lại mỗi request |
| `RunSweeper` | `worker.NewHoldSweeper` thật, publisher no-op, `Interval` **100 ms**, `Batch` 500; đăng ký vào `sweepers` | Xử lý K4: đợt quét cuối xong ~0,5 giây trước phép kiểm |

Thêm `main_test.go` (cùng build tag) với `TestMain` gọi `testx.Shutdown()`.

Helper chạy trong goroutine của test dùng `t.Fatalf`; không helper nào gọi `t.Fatal` từ goroutine con.

## 3. Trình tự làm việc

| Bước | Việc | Kết thúc khi |
|---|---|---|
| 0 | Chốt D1–D5. Cài Go 1.26 và bật Docker Desktop trên máy dev (K9), hoặc chấp nhận kiểm qua CI. Kiểm trạng thái PR #52 | Có môi trường chạy `go test -tags=integration` |
| 1 | `testx`: `goose.go` + unit test trước (không cần Docker), rồi `postgres.go`, `redis.go`, `rabbitmq.go`. `go get` + `go mod tidy` trong `libs/go` | Unit test `testx` xanh; một test tích hợp nhỏ ping được ba container |
| 2 | `helpers.go` + `main_test.go`. `go mod tidy` trong `services/ticketing` | Hai test chạy thật một lần (xanh **hoặc** đỏ có lý do) |
| 3 | Đo: 1 lần, rồi 20 lần mỗi test, ghi thời gian và số PASS vào `baseline.md` | Có số đo để xác nhận hoặc đổi D1 |
| 4 | Nếu đỏ: phân loại *lỗi hạ tầng test* (sửa helper) hay *lỗi thật* (D5: mở issue, ghi `evidence.md`) | Mọi lần đỏ đều có nguyên nhân ghi lại |
| 5 | CI — xem §4 | `oversell-gate` chạy bằng `workflow_dispatch` trên nhánh: 200 PASS |
| 6 | Đột biến — xem §6 | Bảng đột biến điền đủ kết quả |
| 7 | Cổng chất lượng AC-8; tài liệu: `evidence.md`, `traceability.md`, cập nhật `phase-1/report.md` và dòng trạng thái ở `docs/09` | PR mở vào `develop` |

Commit theo từng bước, dạng `feat(testx): ... (EV-134)`, `test(ticketing): ... (EV-134)`, `ci: ... (EV-134)`.

## 4. Thay đổi CI

| File | Thay đổi |
|---|---|
| `Makefile` (`test-oversell`) | Theo D1: hai lệnh — `-run 'TestNoOversell_UnderExtremeConcurrency$' -count=200 -timeout=30m`, rồi `-run 'TestNoOversell_WithConcurrentExpiry$' -count=20 -timeout=15m`. Giữ `$(TESTFLAGS)` |
| `.github/workflows/oversell-gate.yml` | Go `1.26` (khớp `go.work` và #52); `cache-dependency-path` thêm `libs/go/go.sum`; bật `REQUIRE_PASS`; xoá đoạn "LƯU Ý: test thật còn là TODO"; chỉnh `timeout-minutes` theo số đo ở bước 3 |
| `.github/workflows/integration.yml` | Bước smoke 20 lần bật `REQUIRE_PASS`. Nếu số đo vượt 15 phút: giảm còn 5 lần cho test hết hạn |
| `.github/scripts/run-and-report-tests.sh` | Biến môi trường `REQUIRE_PASS=1`: lệnh exit 0 mà 0 PASS → exit 1 (D2). Mặc định giữ hành vi cũ |
| `.github/scripts/tests/test-run-and-report-tests.sh` | Thêm ca: `REQUIRE_PASS=1` + toàn SKIP → exit 1; + có PASS → exit 0; không đặt biến → như cũ |
| `.github/scripts/tests/test-workflows.sh` | Cập nhật `ORIG_OVERSELL` và các phép kiểm AC-4 theo recipe mới; thêm phép kiểm hai workflow có đặt `REQUIRE_PASS` |

Thứ tự bắt buộc: bật `REQUIRE_PASS` trong **cùng PR** đã bỏ `t.Skip`, nếu không `develop` đỏ.

## 5. File được phép đổi

`libs/go/testx/**`, `libs/go/go.mod`, `libs/go/go.sum`, `services/ticketing/test/concurrency/**`,
`services/ticketing/go.mod`, `services/ticketing/go.sum`, `go.work.sum` (chỉ khi build `-mod=readonly` đòi),
các file ở §4, `docs/workflow/EV-134-testx-oversell-gate/**`, `docs/workflow/phase-1/report.md`,
`docs/09-ke-hoach-hoan-thien-backend.md` (dòng trạng thái), `docs/03-cau-truc-src.md` (mô tả `testx` nếu sai).

**Không** đụng `services/ticketing/internal/**`, `migrations/**`, `deploy/**`. Trong `oversell_test.go` chỉ được sửa
comment; đổi bất cứ dòng lệnh nào cần trưởng nhóm duyệt và ghi lý do vào `evidence.md`.

## 6. Đột biến — chứng minh cổng cắn (AC-6)

Cổng này chưa từng đỏ lần nào, nên phải thấy nó đỏ trước khi tin nó xanh. Mỗi đột biến áp tạm trên máy, chạy
test tương ứng 5 lần, ghi kết quả, rồi `git checkout` lại.

| # | Đột biến | Test | Dự kiến |
|---|---|---|---|
| M1 | `hold.lua`: bỏ `HINCRBY ... -qty` | Test 1 | **Đỏ ở bất biến 3** (Redis phồng). Bất biến 1 vẫn giữ nhờ Postgres — chứng minh lớp 2 |
| M2 | M1 + `repository.go`: bỏ `AND available >= $3` | Test 1 | Vẫn đúng 100 đơn nhờ `CHECK (available >= 0)`; đỏ ở bất biến 3 |
| M3 | M2 + bỏ constraint `CHECK (available >= 0)` | Test 1 | **Đỏ ở bất biến 1** (oversell thật) |
| M4 | `repository.go` `ReleaseHold`: bỏ `released_at IS NULL` | Test 2 | Đỏ: tổng > 100, hoặc lỗi `inv_available_le_capacity` |
| M5 | `release.lua`: không xoá hold trước khi cộng kho | Test 2 | **Dự kiến sống sót** (K5: test 2 không kiểm Redis) |
| M6 | `create_hold.go`: bỏ đền bù `gate.Release`, và ép `PersistHold` lỗi ngẫu nhiên ~10% (rollback) | Test 1 | **Đỏ ở bất biến 1 theo chiều under-sell** (< 100 đơn, vé kẹt ở Redis) |
| M7 | `hold_sweeper.go`: đảo thứ tự, Redis trước Postgres | Test 2 | Ghi nhận kết quả; có thể sống sót |

Nếu M5 sống sót đúng như dự kiến: đề xuất thêm phép kiểm `InventoryDrift == 0` vào cuối test 2 — là **thêm** phép
kiểm, không nới lỏng — và trình trưởng nhóm duyệt như một thay đổi riêng.

## 7. Rủi ro

| Rủi ro | Khả năng | Giảm thiểu |
|---|---|---|
| Test đỏ vì lỗi thật của hệ thống (K6: Redis timeout làm mất vé) | Trung bình | D5: ghi bug kèm log; không nới timeout để che. Cổng đỏ là đúng chức năng của nó |
| Đỏ giả ở test 2 do đọc hai truy vấn không cùng snapshot (K4) | Thấp sau khi đặt `Interval` 100 ms | Nếu vẫn xảy ra trong 200 lần: đề xuất một helper đọc cả hai số trong một câu SQL (sửa test, cần duyệt) |
| Under-sell giả do `hold.lua` chỉ quét 3/32 bucket | Rất thấp với 10.000 request cho 100 vé | Nếu gặp: đó là hành vi "báo hết vé sớm" của thiết kế — ghi thành phát hiện, không tăng số goroutine cho xanh |
| Vượt ngân sách thời gian CI | Trung bình | Đo ở bước 3 trước khi chốt số lần; đòn bẩy: Postgres `synchronous_commit=off` trong container test (không đổi ngữ nghĩa khoá) |
| Runner 2 vCPU chậm làm 10.000 goroutine nghẽn pool | Trung bình | pgx chờ theo `ctx` (không timeout); theo dõi `other` trong log; so runner trước khi kết luận |
| `go.mod` bị nâng dòng `go` hoặc thêm `toolchain` | Thấp | `GOTOOLCHAIN=local` khi `go get`; kiểm `git diff` trước commit |
| Dependency testcontainers làm hỏng build image / Trivy | Thấp | Không vào binary; kiểm `docker build` hai image Go và Trivy trước khi mở PR |
| Xung đột với PR #52 | Cao nếu làm song song | D3 |
| `close(start)` không chờ 10.000 goroutine sẵn sàng → đỉnh tải kém sắc | Đã giảm | Helper trước `<-start` không I/O; ghi nhận, không sửa test |
| Docker Hub giới hạn pull trên runner | Thấp | Ba ảnh nhỏ, kéo một lần mỗi job |

## 8. Rollback

Revert merge commit. Slice không đổi code production, schema hay hạ tầng chạy thật. Sau revert, cổng oversell
quay về trạng thái skip **và** `REQUIRE_PASS` biến mất cùng lúc (cùng PR), nên `develop` không đỏ.

## 9. Sản phẩm bàn giao

- `libs/go/testx/` + unit test cho phần tách goose.
- `helpers.go` hoàn chỉnh, `main_test.go`.
- CI theo §4; một lần chạy `oversell-gate` xanh với 200 PASS (link vào `evidence.md`).
- `baseline.md` (số đo bước 3), `evidence.md` (lệnh + kết quả + bảng đột biến), `traceability.md` (AC → test).
- Danh sách issue mở cho mọi lỗi thật và điểm mù tìm thấy (K5, K6, M5, M7).
- Việc tay cho trưởng nhóm: đặt `oversell-gate` thành required check trên `main`.
