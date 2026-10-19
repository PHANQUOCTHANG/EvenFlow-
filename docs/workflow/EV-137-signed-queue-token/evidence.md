# EV-137 — Bằng chứng

Ngày chạy: 2026-10-19. Windows 11, Docker Desktop 28.4.0, Go 1.26.9. Base: `feature/EV-113-device-registration`.
**Chưa có bằng chứng CI (Linux):** mọi con số là chạy cục bộ.

| Kiểm | Kết quả |
|---|---|
| Unit `libs/go/authx`, `waitingroom` (`-race`), `ticketing` (`-race`) | PASS |
| Tích hợp waitingroom (Redis 7 thật): `TestTichHop_QueueToken*` | PASS — 3 test, khớp mẫu `TestTichHop` của bước CI hiện có |
| `go vet -tags=integration` waitingroom, ticketing | PASS |
| Coverage `domain/` + `app/` (cổng 70%) | waitingroom 98,3%, ticketing 87,3% |
| Web `vitest run` toàn bộ | 109 file / 1942 test PASS (gồm `device.test.ts`) |
| Chưa chạy | Cổng oversell 10.000 goroutine; k6 spike (script đã sửa nhưng chưa chạy tải) |
