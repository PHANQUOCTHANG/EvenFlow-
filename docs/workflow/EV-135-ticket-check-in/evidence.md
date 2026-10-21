# EV-135 — Bằng chứng

Ngày chạy: 2026-10-21. Windows 11, Docker Desktop 28.4.0, Go 1.26.9. **Chưa có bằng chứng CI (Linux).**

| Kiểm | Kết quả |
|---|---|
| Unit ticketing `internal/app`, `internal/adapter/http` (`-race`), gateway `internal/proxy` | PASS |
| `go vet` và `go vet -tags=integration` ticketing | PASS |
| Tích hợp Postgres: 50 goroutine check-in một vé | PASS (1 thắng, 49 bị từ chối, vé thứ hai không đổi) |
| `make demo` trên compose thật (có check-in) | 36/36, 94 s |
