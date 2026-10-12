# Spec — trả nợ backend mà CI đã sửa làm lộ ra

**Yêu cầu (người dùng, 2026-10-08):** sửa nợ trước rồi mới merge PR CI; nâng Go (ban đầu chọn
**1.24**, sau đổi sang **1.25** — xem AC-1); cổng coverage thì **viết test thật** (không hạ ngưỡng).

**Tier: Standard.** Chế độ song song §7 — 4 work package, file tách rời.

**Base:** nhánh `fix/backend-debt` = `fix/ci-cd-review` (`9550a06`) + merge `origin/develop`
(`4050da1`). Xếp chồng lên nhánh CI vì CI cũ không thể xanh (`lint-go` action v6 + `latest`, B1 của
`docs/workflow/ci-cd-review/spec.md`), nên nhánh nợ phải chạy với CI đã sửa.

## 1. Nợ đã đo (tại chỗ, không lấy số của agent)

| Job | Nợ | Bằng chứng |
|---|---|---|
| `lint-go` | 10 lỗi golangci-lint v2.12.2: `libs/go` 0, `ticketing` 7 (5 gofmt, 2 errcheck), `waitingroom` 3 (1 gofmt, 2 errcheck) | Docker, bản export LF |
| `test-go` | Cổng coverage domain/app ≥ 70%: `ticketing` 0%, `waitingroom` 0% — **không có file `_test.go` nào** ở `internal/domain`, `internal/app` | `check-domain-coverage.sh`; ~340 dòng code / 4 file |
| `lint-python` | ruff 0.16.10: 6 lỗi thật (SIM102, RUF023, RUF100, I001, UP035, BLE001). `ruff>=0.7` không pin, không có `uv.lock` ⇒ bản ruff trôi theo thời gian | `uv run ruff check` trong `ghcr.io/astral-sh/uv:python3.12-bookworm-slim`. `EXE002` là nhiễu do mount NTFS (git: `100644`) |
| `test-python` | Không có test nào (`no tests ran`, exit 5 — CI chấp nhận) | ghi nợ, **ngoài phạm vi** |
| Trivy | ticketing: 3 CRITICAL (`pgx/v5` v5.7.1 ×2 + stdlib); waitingroom: 1 CRITICAL (stdlib Go 1.23.12, CVE-2025-68121, sửa từ 1.24.13) | `aquasec/trivy:0.66.0 image` trên image đã build |

## 2. Acceptance criteria

**AC-1 — Go 1.25.** Mọi `go.mod` (9) và `go.work` khai **`go 1.25.0`** (không phải `go 1.25`:
trong thứ tự phiên bản của Go `1.25` < `1.25.0`, và `go get pgx@v5.9.x` tự ghi `1.25.0` — để lệch là
mọi lệnh workspace lỗi, xem integration-report §2); mọi Dockerfile Go dùng
`golang:1.25-alpine`; `ci.yml` `GO_VERSION`, `integration.yml`, `oversell-gate.yml` dùng `1.25`.
*Then* `go build ./...` và `go test ./...` xanh ở cả 9 module trong `golang:1.25`.

*Đổi từ 1.24 sang 1.25 ngày 2026-10-08, người dùng quyết.* Lý do: `go get pgx/v5@v5.9.0` trong
`golang:1.24` ⇒ `requires go >= 1.25.0 (running go 1.24.13)`. Ràng buộc đã kiểm bằng `go.mod` từng tag:
pgx v5.7.6 cần 1.23, v5.8.0 cần 1.24 — **cả hai chưa vá**; chỉ v5.9.0+ vá, và cần 1.25. Không có tổ hợp
Go 1.24 + pgx đã vá.

**AC-2 — pgx ≥ 5.9.0** ở `services/ticketing/go.mod`. Không đổi API gọi.

**AC-3 — `lint-go` sạch.** golangci-lint v2.12.2 trên `libs/go`, `services/ticketing`,
`services/waitingroom` ⇒ **0 issues**. errcheck sửa bằng cách xử lý lỗi thật (log / trả về), không
`//nolint` trừ khi có lý do ghi rõ tại chỗ.

**AC-4 — Trivy sạch CRITICAL.** Image ticketing, waitingroom build được (giữ AC-12 của slice CI) và
`trivy image --severity CRITICAL --ignore-unfixed` ⇒ 0.

**AC-5 — Coverage domain/app ≥ 70%** ở `ticketing` và `waitingroom`, bằng test viết **từ nghiệp vụ**
(`docs/01-nghiep-vu.md`) và chữ ký public — không viết lại code thành assert. Test **không** được sửa
mã nguồn; nếu một test đúng nghiệp vụ mà fail ⇒ **báo bug**, không sửa test cho xanh.

**AC-6 — `lint-python` sạch và ổn định.** ruff được **pin phiên bản cụ thể**; `ruff check` ⇒ 0 (trên
Linux, không tính `EXE002` do mount). BLE001 sửa bằng log kèm traceback, không chỉ `noqa`.

**AC-7 — Không hồi quy.** `gates.sh` exit 0; test của slice CI (35 + 128) vẫn xanh; `import app.main`
OK.

## 3. Work package (§7)

| WP | Sở hữu | AC |
|---|---|---|
| **WP-A** Go toolchain + deps + lint (Integrator) | mọi `go.mod`, `go.work`, Dockerfile Go (dòng `FROM`), `GO_VERSION` / `go-version` trong 3 workflow, file **không phải test** trong `services/{ticketing,waitingroom}` (gofmt, errcheck). *Mở rộng trong P4/P5:* `.github/scripts/check-domain-coverage.sh` (fail-open khi `go list` lỗi — integration-report §2) và step biên dịch `bugrepro` trong `ci.yml` (review #5) | AC-1..4 |
| **WP-B** Python (Integrator) | `services/ai-worker/**` | AC-6 |
| **WP-C** test ticketing (agent riêng) | **chỉ** `services/ticketing/internal/**/*_test.go` | AC-5 |
| **WP-D** test waitingroom (agent riêng) | **chỉ** `services/waitingroom/internal/**/*_test.go` | AC-5 |

WP-C/WP-D mỗi agent một worktree riêng từ cùng base; Integrator gộp bằng cách chép file test (đường dẫn
không giao nhau).

## 4. Ngoài phạm vi

- `test-python` không có test; không có `uv.lock` cho các dependency khác ngoài ruff.
- Repo không track `go.sum`.
- Biến oversell-skip thành fail cứng (EVF-39).
