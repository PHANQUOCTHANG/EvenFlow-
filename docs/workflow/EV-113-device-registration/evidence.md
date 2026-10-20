# EV-113 — Bằng chứng

Ngày chạy: 2026-10-19. Windows 11, Docker Desktop 28.4.0, Go 1.26.9. Base: `feature/EV-111-otp-verified-gate`.
**Chưa có bằng chứng CI (Linux):** mọi con số là chạy cục bộ.

| Kiểm | Kết quả |
|---|---|
| Unit `libs/go/authx`, `identity` (`-race`), `gateway` (`-race`) | PASS |
| Tích hợp identity (Postgres 16 thật): `TestDeviceRepo*`, `TestRefreshTokenRepo*`, `TestIdentityRepo*` | PASS — 10 test |
| Đồng thời: 20 đăng ký thiết bị mới cùng lúc, giới hạn 5 | 5 thành công, 15 `ErrTooManyDevices`, 5 dòng trong DB |
| Coverage `domain/` + `app/` identity (cổng 70%) | 80,3% |
| `GOWORK=off go build -mod=readonly` identity | PASS |
| CI | `integration.yml` đếm thêm `TestDeviceRepo` |
