# Gate report — bước 6 (không LLM)

Chạy trên worktree `fix/ci-cd-review`, base `c848afa`, ngày 2026-10-08. Output thô rút gọn.

| # | Gate | Lệnh | Kết quả |
|---|---|---|---|
| 1 | actionlint | `docker run rhysd/actionlint:1.7.7 -color=false` | `ACTIONLINT_EXIT=0`, **0 phát hiện** (baseline: 2 — SC2129 `cd-web.yml:48`, SC2034 `e2e-nightly.yml:23`) |
| 2a | test K1 | `bash .github/scripts/tests/test-run-and-report-tests.sh` | **hiện tại: `35 PASS, 0 FAIL`**, exit 0 (baseline `c848afa`: 0/35). *Lần chạy đầu (trước Challenge): 30/30, baseline 0/30* |
| 2b | test tĩnh workflow | `bash .github/scripts/tests/test-workflows.sh` | **hiện tại: `128 PASS, 0 FAIL`**, exit 0 (baseline: 38 pass / 90 fail). *Lần chạy đầu (trước Challenge): 105/105, baseline 34/71* |
| 2c | test trên Linux, **có và không có GNU make** | `ubuntu:24.04` (+ `apt install make`, GNU Make 4.3), bản sao worktree LF | worktree: 35/0 + 128/0 ở **cả hai**; base: 0/35 + 38/90 ở cả hai. Đột biến bỏ `-count=200` khỏi Makefile ⇒ 126/128, đúng 2 ca K2 FAIL. **Trước bản sửa `norm_cmd`**: có make ⇒ 126/128 trên worktree sạch (FAIL giả — Verification vòng 1 phát hiện; trước đó gate này chỉ từng chạy trên máy không có make) |
| 3 | manifest | kubeconform v0.8.0 `-strict` + `docker compose config` | manifest không đổi trong diff; 5/5 hợp lệ (lần chạy 2026-10-06) |
| 4 | `gates.sh` | `BASE_REF=c848afa bash .github/scripts/gates.sh` | **lần 1: FAIL** — `buoc 4/4 build that bai (exit 139)`: native SWC Windows không nạp được (`DLL initialization routine failed`), Next lùi về WASM rồi segfault. `apps/web` không đổi trong diff. Build riêng chạy lại: exit 0. **Lần 2 (trọn lượt): exit 0**, C4.2 / C4.3 / C4.5 OK, C4.4 bỏ qua (không có cluster) |
| 5 | phạm vi diff | `git status --short` so với plan §1 | chỉ file trong plan §1 (gồm mục 10 mở rộng) + `docs/workflow/ci-cd-review/` |
| 6 | golangci-lint | v2.12.2 qua Docker, bản export LF | `config verify` OK. `run`: libs/go 0, ticketing 7, waitingroom 3 (nợ có sẵn, spec §3) |
| 7 | AC-12 image Go | `docker build -f deploy/docker/{ticketing,waitingroom}.Dockerfile .` trên bản export sạch + Dockerfile đã sửa | cả hai **exit 0** (đo trên `origin/main` `4050da1`, Verification tái hiện trên `c848afa`: hai commit giống nhau ở `deploy/docker/`, `go.work`, workflow). Bản gốc: exit 1, `go: cannot load module ../gateway listed in go.work file`. Đối chứng `GOWORK=off`: exit 1, `missing go.sum entry … pgx/v5`. Bản sửa sau Code Review (chỉ copy `go.mod` của module khác): thả `.env` + `stripe.pem` giả ⇒ **0** file lọt vào build stage |
| 8 | actionlint + test sau vòng sửa Challenge | như 1, 2a, 2b | actionlint 0; 35/35; 128/128 |

## Ghi chú trung thực

- Gate 4 **không** xanh ngay lần đầu. Lần fail được xác định là lỗi môi trường Windows (SWC native),
  không phải diff; nhưng chỉ tính là đạt sau một lượt **trọn** exit 0.
- Gate 6 trải qua 3 lần chạy sai **do cách dựng môi trường test của tôi**, không phải do repo: lần 1
  đặt `GOFLAGS=-mod=mod` (Go cấm trong workspace mode); lần 2 mount repo read-only (Go cần ghi
  `go.work.sum`); lần 3 export bằng `git archive` khi `core.autocrlf=true` ⇒ file CRLF ⇒ gofmt báo sai
  ở `1:1` mọi file. Số liệu trên là lần 4, export với `core.autocrlf=false`.
- Test Design (agent riêng) tự sửa 1 regex của chính test sau lần chạy đầu (không cho dấu nháy đóng
  sau `.sh`), rồi chạy lại 63 đột biến để chắc ca đó vẫn bắt được lỗi. Implementer không sửa file test
  nào.
