# EV-114 — Bằng chứng

Ngày chạy: 2026-10-19. Máy: Windows 11, Go 1.26.9 (`GOTOOLCHAIN=local`).
Nhánh `feature/EV-114-event-route-authorization`, base `feature/gateway-auth-routing`.

**Chưa có bằng chứng CI (Linux):** nhánh chưa được đẩy. Mọi con số là chạy cục bộ.

| Kiểm | Kết quả |
|---|---|
| Unit `services/event` (`go test -race ./...`) | PASS — `TestAuthz*`: 7 test, 45 ca con trong ma trận quyền |
| `go vet` (có và không `-tags=integration`) | PASS |
| Coverage `domain/` + `app/` (cổng 70%) | 78,7% |
| `gofmt` file đã sửa | sạch |
| `golangci-lint` `internal/adapter/http` | sạch (ngoài cảnh báo CRLF của checkout Windows) |

Trước khi gắn quyền, 4 test HTTP cũ gọi không danh tính và nhận 401 — xác nhận route đã thực sự được bảo vệ; sau
đó cập nhật test để gửi danh tính như gateway.

`GOWORK=off go build -mod=readonly` của event **đã hỏng sẵn trên `develop`** (thiếu go.sum); CI và Dockerfile build ở
chế độ workspace nên không bị ảnh hưởng.
