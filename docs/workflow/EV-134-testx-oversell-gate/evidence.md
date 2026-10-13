# EV-134 — Bằng chứng

Ngày chạy: 2026-10-13. Máy: Windows 11, Docker Desktop 28.4.0 (14 CPU, 8 GB), Go 1.26.9 (bản tải về thư mục tạm,
`GOTOOLCHAIN=local`). Nhánh `feature/EV-134-testx-oversell-gate`, base `develop` @ `0108c5f`.

**Chưa có bằng chứng CI (Linux).** Token của phiên làm việc không có quyền `workflow_dispatch` và `createPullRequest`
(HTTP 403), nên chưa chạy được `oversell-gate` / `integration` trên runner. Mọi con số dưới đây là chạy cục bộ.

## 1. Kết quả chính

| Cấu hình | Test | Kết quả |
|---|---|---|
| Production (Redis timeout 200 ms) | `TestNoOversell_UnderExtremeConcurrency` | 1/5 PASS; 4/5 FAIL ở **bất biến 4** với 453–872 request nhận `i/o timeout` |
| Production | `TestNoOversell_WithConcurrentExpiry` | PASS (22,7 s) |
| Chẩn đoán (Redis timeout 5 s, không commit) | `TestNoOversell_UnderExtremeConcurrency` | 5/5 PASS, mỗi lần 2,6–3,1 s (1 lần 7,6 s); 3/3 PASS ở lần chạy baseline đột biến |
| Chẩn đoán | `TestNoOversell_WithConcurrentExpiry` | PASS (28,4 s) |

Ở **mọi** lần đỏ, bất biến 1–3 giữ nguyên: đúng 100 đơn, tồn kho Postgres = 0, Redis lệch Postgres = 0. Chỉ bất biến 4
("mọi người thua nhận `ErrSoldOut`, không phải lỗi hệ thống") đỏ. Lỗi là `redis gate: chay script hold: i/o timeout` và
`kiem tra admit: i/o timeout`, tức client Redis (200 ms, pool 256) không nhận được phản hồi trong hạn khi 10.000 request
cùng lao vào.

Diễn giải (của tôi, chưa kiểm trên Linux): đây là độ trễ dưới đợt 10.000 request đồng thời, không phải lỗi logic —
nới timeout lên 5 s thì 5/5 xanh. Docker Desktop trên Windows có thể làm độ trễ này xấu hơn runner Linux; **CI quyết định**.
Không nới timeout trong test (quyết định D4/D5 của plan).

## 2. Đột biến (AC-6)

Mỗi đột biến áp tạm, chạy với Redis timeout 5 s để lỗi latency không che kết quả, rồi `git checkout` lại. Không commit.

| # | Đột biến | Test | Kết quả | Đúng dự kiến? |
|---|---|---|---|---|
| M1 | `hold.lua`: bỏ `HINCRBY ... -qty` | 1 | 3/3 đỏ: `Redis lech Postgres 10000 ve` | Có |
| M2 | M1 + `repository.go`: bỏ `AND available >= $3` | 1 | 3/3 đỏ cùng thông báo; vẫn đúng 100 đơn nhờ `CHECK (available >= 0)` | Có |
| M3 | M2 + bỏ `CHECK (available >= 0)` ở migration | 1 | 3/3 đỏ: `ban duoc 10000 ve, ky vong dung 100` — **oversell thật bị bắt** | Có |
| M4 | `ReleaseHold`: bỏ `released_at IS NULL AND consumed_at IS NULL` | 2 | 2/2 đỏ: `KHO BI PHONG: tong = 168 / 170, ky vong 100` | Có |
| M5 | `release.lua`: bỏ `HDEL`/`ZREM` (hold không bị xoá, Redis bị cộng lại mỗi lần quét) | 2 | **2/2 PASS — sống sót** | Có (K5) |
| M6 | Bỏ đền bù `gate.Release` + `PersistHold` lỗi ngẫu nhiên ~10% | 1 | 3/3 đỏ: bán 86 / 90 / 95 vé, kỳ vọng 100 — **under-sell bị bắt** | Có |
| M7 | Sweeper gọi thêm `gate.Release` trước `repo.ReleaseHold` | 2 | 2/2 PASS — sống sót | Có thể (ghi nhận) |

Lớp 2 (Postgres) đỡ được lớp 1: M1 và M2 vẫn bán đúng 100 vé. Chỉ khi cả hai lớp cùng hỏng (M3) mới oversell.

M5 sống sót vì `TestNoOversell_WithConcurrentExpiry` chỉ cộng tồn kho **Postgres** (`TotalAvailable + TotalHeldOrSold`) và
không kiểm `InventoryDrift`. Lỗi trả kho nhiều lần ở **Redis** — đúng loại BR-O7 muốn chặn — không làm test đỏ.
Đề xuất (cần trưởng nhóm duyệt, vì chạm `oversell_test.go`): thêm `InventoryDrift == 0` cuối test 2. Đây là thêm phép kiểm,
không nới lỏng. Chưa làm trong PR này.

M7 sống sót là hợp lý: đảo thứ tự chỉ nguy hiểm khi tiến trình chết giữa hai bước, thứ test này không mô phỏng.

## 3. Cổng chất lượng (AC-8)

| Kiểm | Kết quả |
|---|---|
| `gofmt` trên file đã sửa/thêm | sạch |
| `go vet` và `go vet -tags=integration` ở `libs/go`, `services/ticketing` | sạch |
| `go test -race -count=1 ./...` ở hai module | PASS (có 6 test mới trong `testx`) |
| `golangci-lint v2.12.2 --build-tags integration` | 0 lỗi ngoài `gofmt`; `gofmt` chỉ báo file cũ do working copy Windows checkout CRLF (index là LF) |
| `go build -mod=readonly ./...` ở cả 9 module | PASS |
| `test-run-and-report-tests.sh` | 40 PASS (5 ca mới T36–T40) |
| `test-workflows.sh` | 133 PASS; bỏ `REQUIRE_PASS` hoặc đổi tên target thì 2 check mới FAIL |
| Build image `ticketing` với Dockerfile hiện tại | **FAIL** `go.work requires go >= 1.26.0 (running go 1.25.14; GOTOOLCHAIN=local)`. Có sẵn trên `develop`, không do EV-134: `go.work` lên 1.26.0 ở commit `95650da` (EV-110) còn Dockerfile vẫn `golang:1.25-alpine`; PR #52 đổi sang `golang:1.26-alpine` |
| Build image `ticketing` với base `golang:1.26-alpine` (Dockerfile tạm ngoài repo) | PASS, 69,3 MB — thay đổi `go.mod`/`go.sum` không làm hỏng image |
| Trivy | chưa chạy cục bộ; `testcontainers` chỉ được import từ file gắn tag `integration`, không vào binary |

## 4. Thay đổi phụ cần biết

- `go-redis` nhảy `v9.7.0` → `v9.7.3` ở `libs/go` và `services/ticketing` vì module redis của testcontainers yêu cầu.
- `testcontainers-go` ghim `v0.44.0` cùng phiên bản với PR #52 để lúc rebase `go.mod`/`go.sum` ít xung đột.
- `setupEnv` thử lại `NewGate` tối đa 5 lần: ngay sau lần chạy nặng, `SCRIPT LOAD` với timeout 200 ms có thể trượt.
  Đã gặp 2/5 lần trước khi sửa; không còn gặp sau sửa. Đây là nhiễu dựng hạ tầng, không phải hành vi cần đo.

## 5. Việc còn lại

| Việc | Ai | Trạng thái |
|---|---|---|
| Merge PR #52 trước (hoặc rebase lên nó): Trivy/build image của nhánh này đỏ vì lỗi Dockerfile có sẵn ở trên | Nhóm | Chờ |
| Mở PR vào `develop`, đọc kết quả job `integration` trên Linux | Người có quyền (token phiên này bị 403) | Chờ — nhánh đã đẩy; nội dung PR để sẵn |
| Quyết định nếu test 1 đỏ trên CI: sửa hệ thống (timeout 200 ms / hàng đợi đường hold) hay đổi mô hình tải của test | Trưởng nhóm | Chờ số liệu CI |
| Chạy `oversell-gate` đủ 200 lần, ghi PASS/FAIL/SKIP và thời lượng; chỉnh `timeout-minutes` | Người có quyền dispatch | Chờ |
| Thêm `InventoryDrift == 0` vào test 2 (giết M5) | Trưởng nhóm duyệt | Đề xuất |
| Đặt `oversell-gate` thành required check trên `main` | Trưởng nhóm | Cấu hình repo |
| `testx.RabbitMQ` | EV-131 (outbox), sau khi PR #52 merge | Hoãn |
| Mở issue cho: Redis 200 ms dưới đợt 10.000 request (nếu tái hiện trên CI); điểm mù M5 | — | Chưa mở |
