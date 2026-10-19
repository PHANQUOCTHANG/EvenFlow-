# EV-NEW-2 — Bằng chứng

Ngày chạy: 2026-10-19. Windows 11, Docker Desktop 28.4.0, Go 1.26.9. Base: `develop` @ `1d4b358`.
**Chưa có bằng chứng CI (Linux):** mọi con số là chạy cục bộ.

| Kiểm | Kết quả |
|---|---|
| Unit `ticketing` (`-race`), `gateway` | PASS |
| Tích hợp `TestOrderAPI*` (Postgres 16 + Redis 7 thật) | PASS — 7 test; 20 yêu cầu huỷ đồng thời → đúng 1 lần trả kho |
| `go vet -tags=integration` | PASS |
| Coverage `domain/` + `app/` ticketing (cổng 70%) | 88,7% |
| Web: `payment-client.test.ts`, `use-order-status.test.tsx` | 15 test PASS |
| Chưa chạy | Cổng oversell 10.000 goroutine (không đổi đường tạo hold); E2E Playwright trên stack thật (cần EV-104) |

## Lỗi bắt được nhờ test tích hợp

Lần chạy đầu: `Redis lệch Postgres -3 sau khi huỷ` — huỷ hold dùng id UUID có gạch ngang của Postgres để gọi `Gate.Release`,
nhưng Redis lưu hex 32 ký tự nên không tìm thấy hold. Đã sửa bằng `redisHoldID` và thêm test đơn vị cho việc đổi id.
