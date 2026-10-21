# EV-131 / EV-133 / EV-155 — Bằng chứng

Ngày chạy: 2026-10-21. Windows 11, Docker Desktop 28.4.0, Go 1.26.9. Base: `feature/EV-104-compose-all-services`.
**Chưa có bằng chứng CI (Linux):** mọi con số là chạy cục bộ.

| Kiểm | Kết quả |
|---|---|
| Unit `libs/go`, `payment`, `ticketing`, `notification`, `identity` (`-race`) | PASS |
| `go vet` + `go vet -tags=integration` các module đã sửa | PASS |
| Tích hợp `libs/go/mq` (RabbitMQ 3.13 thật), gồm `TopologySuKien_RetryVeDungHangGiuKeyGoc` | PASS |
| Tích hợp payment relay (Postgres 16 thật) | PASS (2 test) |
| Tích hợp ticketing `internal/worker`, `internal/adapter/postgres` (hồi quy relay, phát vé) | PASS |
| Chưa chạy | Cổng oversell 10.000 goroutine (đường giữ ghế không đổi); k6 |

## E2E trên compose thật

`docker compose up -d --build`, mọi request qua gateway `localhost:8080`:

| Bước | Kết quả |
|---|---|
| Đăng ký → OTP (đọc từ Mailpit) → xác minh → đăng nhập | 200 / 201 / 200 |
| Xếp hàng → `ADMITTED` → tình trạng vé → giữ 1 vé → xem đơn | 200, `AVAILABLE` |
| Thanh toán sandbox → webhook | 200, đơn `PAID` |
| **Không gọi `issue-tickets`**, chờ | Đơn `ISSUED` ở lần kiểm đầu tiên (≤ 3 giây sau webhook), 1 vé |
| Outbox | Không còn dòng `order.paid` / `ticketing.ticket.issued` chưa publish |
| Mailpit của khách | 3 email: mã OTP, "Xác nhận thanh toán thành công", "Vé điện tử chính thức" |
| Gọi lại `issue-tickets` | `already_issued=true`, DB vẫn 1 vé |
| `POST /v1/tickets/verify` với QR do worker ký | 200 `valid: true`; QR sửa 1 ký tự → 400 `/problems/tampered-qr` |
| Broker sau E2E | `ticketing.dlq` = 0, `notification.dlq` = 0; mỗi hàng consumer có 1 consumer |

Lưu ý khi nâng cấp môi trường đã chạy: topology mới chỉ **thêm** exchange/queue, không đổi tham số của cái cũ, nên không cần xoá volume RabbitMQ.
