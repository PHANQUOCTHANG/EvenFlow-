# EV-134 — Truy vết AC → test

"Unit" chạy bằng `go test ./...` (không cần Docker). "Tích hợp" cần `-tags=integration` và Docker.
Kết quả là **chạy cục bộ ngày 2026-10-13**; cột CI để trống cho tới khi có run trên Linux (xem `evidence.md`).

| AC | Nội dung | Test / kiểm | Loại | Kết quả cục bộ | CI |
|---|---|---|---|---|---|
| AC-1 | Tách phần Up của migration, từ chối file thiếu marker / Up rỗng | `TestGooseUp_*` (4), `TestMigrationFiles_*` (2) | Unit | PASS | — |
| AC-1 | Migration thật của repo tách được, không lọt phần Down | `TestWriteUpScripts_MigrationThatCuaRepo` | Unit | PASS | — |
| AC-1 | Postgres + Redis thật khởi động, chia sẻ cả tiến trình, thiếu Docker thì fail | `setupEnv` (qua hai test oversell) | Tích hợp | PASS | |
| AC-2 | Không còn `t.Skip` / `panic("TODO")` trong `helpers.go` | `grep` trong evidence + test chạy thật | — | PASS | |
| AC-3 | Đúng 100 đơn, tồn kho 0, drift 0, 9.900 lần `ErrSoldOut` | `TestNoOversell_UnderExtremeConcurrency` | Tích hợp | **1/5 ở timeout 200 ms; 5/5 ở 5 s** (bất biến 1–3 luôn giữ) | |
| AC-4 | Hai sweeper song song, tổng = quota | `TestNoOversell_WithConcurrentExpiry` | Tích hợp | PASS | |
| AC-5 | 200 PASS / 0 FAIL / 0 SKIP trên một run `oversell-gate` | `make test-oversell` qua `run-and-report-tests.sh` | CI | Chưa chạy | |
| AC-6 | Cổng đỏ khi hệ thống bị phá | Bảng M1–M7 ở `evidence.md` §2 | Đột biến | M1–M4, M6 đỏ đúng; M5, M7 sống sót | — |
| AC-7 | 0 PASS ⇒ exit 1 khi `REQUIRE_PASS=1`; hành vi cũ giữ nguyên khi không đặt | `test-run-and-report-tests.sh` T36–T40 | Unit (script) | 40/40 PASS | — |
| AC-7 | Workflow đặt `REQUIRE_PASS`, có step cho hold hết hạn, Makefile tách target | `test-workflows.sh` (7 check AC-4 mới) | Unit (script) | 133/133 PASS; đột biến làm 2 check FAIL | — |
| AC-8 | gofmt, vet, `-race`, golangci-lint, build `-mod=readonly` | `evidence.md` §3 | — | PASS (image/Trivy: xem evidence) | |
