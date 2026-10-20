# EV-111 — Bằng chứng

Ngày chạy: 2026-10-19. Windows 11, Docker Desktop 28.4.0, Go 1.26.9. Nhánh base `develop` @ `1d4b358`.
**Chưa có bằng chứng CI (Linux):** mọi con số là chạy cục bộ.

| Kiểm | Kết quả |
|---|---|
| Unit `libs/go`, `identity`, `gateway`, `waitingroom` (`-race`) | PASS |
| Tích hợp identity (Postgres 16 thật): `TestIdentityRepo*`, `TestRefreshTokenRepo*` | PASS — 7 test |
| CI `integration.yml` | regex đổi thành `TestRefreshTokenRepo\|TestIdentityRepo` để đếm cả test mới |
