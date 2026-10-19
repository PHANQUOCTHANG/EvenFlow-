# EV-155 (EVF-75) — Baseline (Trạng thái trước khi thay đổi)

Chụp lúc: 2026-10-19 19:40 UTC+7, commit gốc `602cf5ef03d6ff616393186edd1db4649c4ce43c` (nhánh `develop` sau khi merge PR #79), working tree sạch trên nhánh `feature/EV-155-notification-confirmation-tickets`.

## 1. Kết quả chạy test hiện tại
- **Dịch vụ `services/notification`**:
  - Lệnh: `docker run --rm -v "${PWD}:/app" -w /app golang:1.26-alpine go test ./services/notification/...`
  - Kết quả: `? github.com/eventflow/eventflow/services/notification/cmd/server [no test files]`
  - Hiện trạng: Service mới chỉ có file khung `cmd/server/main.go` với endpoint `/healthz`, chưa có logic nghiệp vụ Hexagonal (Domain, Port, App, Adapter) và chưa có test nào.
- **Dịch vụ phụ thuộc liên quan (`services/payment/...`)**:
  - Lệnh: `docker run --rm -v "${PWD}:/app" -w /app golang:1.26-alpine go test ./services/payment/...`
  - Kết quả: **70 passed, 0 failed** trên toàn bộ các package của payment service (`gateway`, `http`, `metrics`, `postgres`, `app`, `domain`).
- **Thư viện dùng chung (`libs/go/...`)**:
  - `libs/go/mq`: Đã có `NotificationTopology` khai báo exchange `notification.direct` và queue `notification.email.q`.
  - Toàn bộ các package thư viện (`authx`, `httpx`, `idem`, `leader`, `loadsignal`, `mq`, `otelx`, `testx`) đều xanh.
- **Nhận định**: Toàn bộ codebase hiện tại không có test nào bị fail. Mọi test fail sau này là lỗi hồi quy và không được chấp nhận.

## 2. Các thành phần và sự kiện liên quan
| Thành phần / Nguồn sự kiện | Chi tiết | Ảnh hưởng dự kiến |
|---|---|---|
| `services/payment` (EV-151) | Phát ra outbox event `order.paid` (`OutboxOrderPaidPayload`) khi thanh toán thành công | Notification service cần lắng nghe event này để gửi email xác nhận thanh toán |
| `services/ticketing` (EV-133) | Phát ra outbox event `ticketing.ticket.issued` (`OutboxTicketIssuedPayload`) | Notification service cần lắng nghe event này để gửi email/SMS chứa vé kèm QR/JTI |
| `services/payment` (EV-154) | Phát ra outbox event `payment.refunded` (`OutboxPaymentRefundedPayload`) | Notification service tiếp nhận gửi email/SMS thông báo hoàn tiền |
| `libs/go/mq` | `ConsumerConfig`, `Consume`, `Topology`, `NotificationTopology` | Tận dụng thư viện để hiện thực AMQP consumer có retry, DLQ và idempotent handler |

## 3. Ngưỡng coverage yêu cầu
- Lines: >= 70%
- Functions/Methods: >= 70%
- Statements: >= 70%
- Tuân thủ nghiêm ngặt 4 Quy tắc Bất biến của EvenFlow.
