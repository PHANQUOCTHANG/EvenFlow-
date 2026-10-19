# EV-112 — Bằng chứng

Ngày chạy: 2026-10-19. Máy: Windows 11, Docker Desktop 28.4.0, Go 1.26.9 (`GOTOOLCHAIN=local`).
Nhánh `feature/EV-112-refresh-token-rotation`, base `develop` @ `f2154d8`.

**Chưa có bằng chứng CI (Linux):** nhánh chưa được đẩy. Mọi con số là chạy cục bộ.

## 1. Kết quả

| Kiểm | Kết quả |
|---|---|
| Unit `services/identity` (`go test -race ./...`) | PASS |
| Unit `libs/go/authx`, `libs/go/httpx` (`-race`) | PASS |
| Tích hợp `TestRefreshTokenRepo*`, Postgres 16 thật (testcontainers, chạy cả migration `0003`) | PASS — 4 test |
| `go vet` identity (có và không `-tags=integration`) | PASS |
| `GOWORK=off go build -mod=readonly ./...` (identity) | PASS |
| Coverage `domain/` + `app/` identity (cổng 70%) | 80,4% |
| `gofmt` file mới | sạch. Các file identity đã sửa vốn chưa gofmt trên `develop` (CRLF + khoảng trắng thừa); không format lại để diff gọn |

Đồng thời: 20 goroutine cùng `Rotate` một token → **1 thành công, 19 `ErrRefreshTokenReused`, 0 lỗi khác**, họ
token không còn token sống nào sau đó.

## 2. CI

Thêm bước `go vet/test -tags=integration (identity)` vào `.github/workflows/integration.yml`, dùng
`run-and-report-tests.sh TestRefreshTokenRepo` (fail nếu 0 test PASS).
