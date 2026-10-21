# EV-118 — Bằng chứng

Ngày chạy: 2026-10-21. Windows 11, Docker Desktop 28.4.0, Go 1.26.9, Python 3.12 (container uv). Base: `feature/EV-131-post-payment-chain`.
**Chưa có bằng chứng CI (Linux):** mọi con số là chạy cục bộ.

| Kiểm | Kết quả |
|---|---|
| `go vet` + `go vet -tags=integration`, unit `-race` cả 9 module Go | PASS |
| Tích hợp waitingroom (`test/leader`, `test/queuetoken`, `test/soldout`) | PASS |
| Tích hợp ticketing `internal/...` (Postgres 16 + Redis 7) | PASS |
| Tích hợp event `adapter/postgres` (`ListPublic`, `OpenDueSales`) | PASS |
| Tích hợp `libs/go/activeevents`, `libs/go/leader` | PASS |
| ai-worker `pytest` (trừ `tests/integration`) | 203 PASS, gồm test đối chiếu `definitions.json` |
| Chưa chạy | Cổng oversell 10.000 goroutine (đường giữ ghế không đổi); k6 |

## Kịch bản organizer trên compose thật (mọi request qua gateway)

| Giờ | Bước | Kết quả |
|---|---|---|
| 21:18:32 | Đăng nhập organizer / moderator / demo (tài khoản seed) | 200 cả ba |
| 21:18:32 | Organizer tạo sự kiện, T0 = +90 giây; thêm hạng vé 200 vé | 201 / 201 |
| 21:18:33 | Nộp duyệt | AI `AUTO_APPROVED` (0.100) sau 1 giây → `SCHEDULED` |
| 21:18:34 | `GET /api/v1/events/public` ẩn danh | Có sự kiện mới |
| 21:18:34 | Chờ phòng chờ/ticketing nhận | Sau 2 giây: trong `wr:active_events`, có `sale_start_ms`, kho Redis đã nạp. Controller: "bat dau dieu khien", leader epoch 1, "cho toi gio mo ban 1m21s" |
| 21:18:38 | Khách join trước T0 | `LOBBY` |
| 21:20:02 | T0 | Controller "lottery hoan tat, so_nguoi=1"; sự kiện tự `ON_SALE` |
| 21:20:04 | Khách | `ADMITTED`; giữ 1 vé → 200, Postgres còn 199 |
| — | Organizer huỷ sự kiện | 200; vòng sau rời `wr:active_events`, controller "su kien roi danh sach, dung dieu khien" |

Hồi quy cùng stack: E2E thanh toán của EV-131 (vé tự phát, 3 email, QR verify) PASS; smoke 20/20 PASS sau khi sửa `seed.sh RESET`.
