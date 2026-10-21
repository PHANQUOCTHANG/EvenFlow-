# EV-103 — Bằng chứng

Ngày chạy: 2026-10-21. Windows 11, Docker Desktop 28.4.0, Go 1.26.9. **Chưa có bằng chứng CI (Linux).**

| Kiểm | Kết quả |
|---|---|
| Unit `libs/go/otelx`, gateway (`-race`) | PASS |
| `go vet` + `-tags=integration` payment; tích hợp relay payment (Postgres 16 thật) | PASS |
| Jaeger sau `make demo` | Trace `407c17a9…`: **gateway, payment, ticketing-worker, notification** (span: webhook, mq.publish payment.events/order.paid, mq.consume ticketing.order-paid.q, mq.publish ticketing.events/ticketing.ticket.issued, mq.consume notification.events.q) |
| `make demo` | 37/37, 99 s; services trong Jaeger: gateway, identity, event, payment, waitingroom, ticketing, ticketing-worker, notification |

Trước thay đổi: trace nhiều service nhất tìm được là gateway + event (2 service), vì gateway không chuyển `traceparent`.
