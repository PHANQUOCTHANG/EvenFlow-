# EV-NEW-1 (gateway) — Bằng chứng

Ngày chạy: 2026-10-19. Máy: Windows 11, Go 1.26.9 (`GOTOOLCHAIN=local`), golangci-lint v2.6.2 (CI dùng v2.12.2).
Nhánh `feature/gateway-auth-routing`, base `feature/EV-112-refresh-token-rotation`.

**Chưa có bằng chứng CI (Linux):** nhánh chưa được đẩy. Mọi con số là chạy cục bộ.

| Kiểm | Kết quả |
|---|---|
| Unit `services/gateway` (`go test -race ./...`) | PASS — config, proxy (11 test), ratelimit |
| Unit `libs/go/authx` | PASS |
| `golangci-lint run ./...` (gateway) | 0 issue |
| `GOWORK=off go build -mod=readonly ./...` (gateway) | PASS |

CI: thêm `services/gateway` vào bước `golangci-lint` của `ci.yml`. Gateway không có `internal/domain|app` nên cổng
coverage 70% bỏ qua module này (giống `libs/go`).

Chưa kiểm: chạy end-to-end qua compose (web → gateway → service) — cần EV-104.
