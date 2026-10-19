# EV-146 — Bằng chứng

Ngày chạy: 2026-10-19. Windows 11, Docker Desktop 28.4.0, Go 1.26.9. Base: `feature/EV-137-signed-queue-token`.
**Chưa có bằng chứng CI (Linux):** mọi con số là chạy cục bộ.

| Kiểm | Kết quả |
|---|---|
| Unit `waitingroom`, `ticketing` (`-race`) | PASS |
| Tích hợp waitingroom (Redis 7): `test/soldout` (6 test), `test/queuetoken`, `test/leader` (hồi quy sau khi sửa `admit.lua`) | PASS |
| Tích hợp ticketing (Postgres 16 + Redis 7): `TestSoldOut*` (3 test), `TestPurchaseLimit*`, `TestReconciler*`, `TestNoOversell_WithConcurrentExpiry` | PASS |
| `go vet -tags=integration` cả hai service | PASS |
| Coverage `domain/` + `app/` (cổng 70%) | waitingroom 98,4%, ticketing 87,3% |
| Web `vitest run` toàn bộ | 109 file / 1945 test PASS (thêm 3 test phòng chờ) |
| CI | Bước tích hợp waitingroom (`./test/...`, mẫu `TestTichHop`) chạy cả `test/soldout`; bước ticketing chạy `./...` |
| Chưa chạy | Cổng oversell 10.000 goroutine (đường tạo hold không đổi); k6 spike; E2E trên stack thật (cần EV-104) |
