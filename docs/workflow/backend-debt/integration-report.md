# Integration report — P4 (gates trên kết quả đã gộp)

Nhánh `fix/backend-debt`. 4 WP gộp: WP-A (Go, Integrator), WP-B (Python, Integrator), WP-C (test
ticketing, agent riêng), WP-D (test waitingroom, agent riêng). WP-C/WP-D chỉ tạo `*_test.go` — đã kiểm
`git status`/`git diff` của worktree từng agent trước khi chép.

Chạy trên bản copy LF của worktree (giống checkout trên runner), 2026-10-08.

## 1. Kết quả

| # | Gate | Kết quả |
|---|---|---|
| 1 | golangci-lint v2.12.2 — `libs/go`, `ticketing`, `waitingroom` | **0 / 0 / 0 issues** (trước: 0 / 7 / 3) |
| 2 | `go vet && go build && go test -race` — 9 module, `golang:1.25-alpine` (go1.25.14) | 9/9 rc 0 (waitingroom 2 gói ok, ticketing 3 gói ok) |
| 3 | Cổng coverage domain/app ≥ 70% (`check-domain-coverage.sh`) | ticketing **97.5%**, waitingroom **100%**; 7 module khác bỏ qua (không có gói domain/app) (trước: 0% / 0%) |
| 4 | `docker build` ticketing + waitingroom | rc 0 / rc 0 |
| 5 | Trivy `image --severity CRITICAL --ignore-unfixed` | **0 / 0** (trước: 3 / 1) |
| 6 | ruff 0.16.10 | `All checks passed!` (trước: 6) |
| 7 | pytest | rc 5 `no tests ran` — CI chấp nhận (nợ ngoài phạm vi) |
| 8 | `import app.main` | OK |
| 9 | actionlint 1.7.7 (trên worktree) | rc 0 |
| 10 | test của slice CI: K1 + workflow | 35 / 0, 128 / 0 |
| 11 | `gates.sh` | xem §4 |

## 2. Hai lỗi do chính Integrator gây ra, tìm thấy ở P4

- **`go 1.25` vs `go 1.25.0`.** `go get pgx@v5.9.2` tự đổi `services/ticketing/go.mod` thành
  `go 1.25.0`, trong khi `go.work` và 8 `go.mod` khác ghi `go 1.25`. Trong thứ tự phiên bản của Go,
  `1.25` < `1.25.0`, nên mọi lệnh ở workspace mode lỗi `module ../ticketing listed in go.work file
  requires go >= 1.25.0, but go.work lists go 1.25`. Sửa: cả 10 file ghi `go 1.25.0`.
- **Lỗi đó làm lộ một gate fail-open có sẵn.** Với `go.work` lệch như trên, `check-domain-coverage.sh`
  in "chua co goi internal/domain hoac internal/app, bo qua" và **exit 0**: `go list ./... 2>/dev/null
  | grep … || true` nuốt lỗi nạp module thành danh sách rỗng. Tức một lỗi build làm cổng coverage
  **xanh**. Sửa script: tách lỗi `go list` (exit 1, in lỗi) khỏi trường hợp "không có gói"; giữ output
  khi test fail (bản cũ `>/dev/null`). Kiểm cả hai chiều: `go.work` lệch ⇒ exit 1 kèm thông báo thật;
  bản đúng ⇒ 97.5% / 100%.

## 3. Bug nghiệp vụ tìm thấy bởi test (KHÔNG sửa trong slice này)

Repro đặt trong `*_bug_test.go` với `//go:build bugrepro` — không chạy mặc định, không làm CI đỏ, không
dùng `t.Skip`. Integrator **tự chạy lại** cả ba và xác nhận fail:

```
go test -tags bugrepro -race -count=1 ./internal/app/      (ticketing)
  --- FAIL: TestBug_BRO3_TaoHoldMoiPhaiHuyHoldCuVaTraKho
  --- FAIL: TestBug_NhieuKhachGiuGheDongThoi_KhongDuocDataRace     (11 x WARNING: DATA RACE)
go test -tags bugrepro -count=1 ./internal/app/ -run BugRepro  (waitingroom)
  --- FAIL: TestBugRepro_OverrideIsRespectedOnNextTicks
  --- FAIL: TestBugRepro_OverrideZeroPausesAdmission
```

| # | Service | Bug | Mức | Bằng chứng | Cần quyết |
|---|---|---|---|---|---|
| 1 | ticketing | **Data race**: mọi request dùng chung một `*math/rand.Rand` (`create_hold.go:31,40,88`, `uc.rng.Intn`). `math/rand.Rand` không an toàn đồng thời; `Execute` chạy song song từ HTTP handler — đúng lúc mở bán | **cao** | 50 khách đồng thời ⇒ `DATA RACE` dưới `-race`; không `-race` thì pass (số lượng vẫn đúng ⇒ không oversell trực tiếp, nhưng có thể hỏng trạng thái RNG / panic) | sửa nhỏ (vd `math/rand/v2` hàm cấp gói, an toàn đồng thời) — chờ duyệt |
| 2 | ticketing | **BR-O3 mâu thuẫn với code**: BR "tạo hold mới → hold cũ bị huỷ và trả kho ngay"; code (`create_hold.go:107-110`, `case port.HoldAlreadyHolding`) trả lại hold cũ, có comment nói là chủ ý ("khách bấm lại không nên bị phạt") | trung | test repro | **sửa code hay sửa BR** — quyết định nghiệp vụ |
| 3 | waitingroom | **BR-Q5 `manual_override` không được giữ**: `SetRateOverride` (`admit_controller.go:197-199`) chỉ đặt rate; tick sau `adjust` (181-190) tính lại từ đó. Ops đặt 42/s ⇒ tick sau 52/s; Ops đặt **0 để dừng khẩn cấp** ⇒ 3 tick sau vẫn thả 60 người | **cao** | test repro | cần chốt nghĩa của `manual_override` (giữ tới khi Ops bỏ, hay chỉ là điểm xuất phát mới). Ngoài ra **không có endpoint Ops nào gọi `SetRateOverride`** |

Nghi vấn chưa xác nhận (cần DB/Redis thật — ngoài tầng domain/app):
- `CountPurchased` (`adapter/postgres/repository.go:47-58`) đếm cả đơn `HELD`/`PAYMENT_PENDING`, trong
  khi BR-O4 ghi "gộp trên mọi đơn đã PAID".
- BR-O5 (Idempotency-Key): handler chỉ kiểm header có mặt; không thấy chỗ lưu/trả lại response cũ 24h.

## 3b. P5 — Review + Challenge (agent riêng, chỉ đọc): ĐẠT, 9 minor

Đột biến mã nguồn trên bản copy, `golang:1.25`: **ticketing 20/22 bị bắt, waitingroom 23/31**. Không
đột biến nào lọt ở bất biến chính (không oversell, trả kho bù khi Postgres từ chối, Postgres trước
Redis, TTL 10/15 phút, BR-O4, admit, T0 + lottery, clamp, backpressure). `TestRun_*` ×30 và toàn bộ
waitingroom ×20: không flaky. Agent tự khởi động lại Docker Desktop khi nó bị tắt giữa chừng, và bỏ
một lượt đột biến bị nhiễu (hai lượt cùng ghi một thư mục) rồi chạy lại sạch.

| # | Phát hiện | Xử lý |
|---|---|---|
| 1 | `services/ticketing/go.mod` chưa tidy ở module mode (thiếu 4 `// indirect`); workspace mode vẫn build | **Chưa sửa**: `GOWORK=off go mod tidy` trong Docker không tải được module (`lookup proxy.golang.org: no such host`). Ghi nợ cùng `go.sum` |
| 2 | Không track `go.sum`/`go.work.sum` | ngoài phạm vi (spec §4) |
| 3 | `check-domain-coverage.sh` ngoài bảng WP | **Sửa**: ghi vào WP-A là mở rộng phạm vi |
| 4 | spec còn ghi 1.24 / `go 1.25` | **Sửa** câu chữ |
| 5 | test `bugrepro` không gate nào biên dịch ⇒ hỏng âm thầm khi port/fake đổi; C4.2 của `gates.sh` không soi Go (và regex của nó không khớp `t.Skip` viết hoa) | **Sửa phần đầu**: step `go vet -tags bugrepro ./internal/...` trong `test-go`. Phần C4.2 cho Go: ghi nợ |
| 6 | test batch của sweeper tự so với chính nó (đột biến Batch 500→1 lọt) | ghi nhận (file của WP-C; batch không phải BR) |
| 7 | BR-Q4 chỉ ghim 2 đầu 3s / 30s; tầng giữa và ngưỡng không ghim | chấp nhận — BR chỉ quy định 2 đầu |
| 8 | `semantic_cache.put` log traceback mỗi lần embedder lỗi ⇒ có thể ngập log khi embedder sập lúc mở bán | ghi nhận; cân nhắc rate-limit log |
| 9 | pgx 5.9 bỏ Describe Portal với statement đã cache; compose đi qua PgBouncer `transaction` | chưa kiểm với PgBouncer thật — để integration / oversell / E2E |

## 4. `gates.sh`

`npm ci` rồi `BASE_REF=origin/develop bash .github/scripts/gates.sh` trên worktree:

```
OK buoc 1/4 lint | 2/4 typecheck | 3/4 test:coverage | 4/4 build
OK: khong co dong them nao chua .skip/.only/it.todo
OK: /api/healthz, /api/readyz khong nam trong prerender manifest
BO QUA validate k8s: khong co cluster
OK: co readiness co trong ca hai bundle, nhanh 503 con nguyen, healthz khong co 503
TAT CA GATE DAT.   GATES_EXIT=0
```

Ghi chú: bước dựng bản copy LF cho §1 báo `tar: Cannot stat` cho ~40 ảnh thiết kế tên tiếng Việt dưới
`apps/web/doc/design/stitch/` (git in tên dạng escape bát phân). Các file đó không tham gia gate nào ở
§1; `gates.sh` chạy trực tiếp trên worktree nên không bị ảnh hưởng.
