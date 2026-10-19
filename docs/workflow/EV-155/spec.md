# EV-155 (EVF-75) — Email/SMS Xác Nhận + Đính Kèm Vé qua Notification Service (Spec)

| Thuộc tính | Chi tiết |
|---|---|
| Jira Ticket | **EV-155** (Mã backlog: **EVF-75**) |
| Epic / Module | E6 – Thanh toán & Vận hành / Notification Service (`services/notification`) |
| Story Points | **3 SP** — Priority: **P1 / Sprint 5** |
| Tier | **Critical / Customer Communication & Asynchronous Reliability** (Bảo đảm 100% người mua nhận được email/SMS xác nhận thanh toán và vé điện tử chứa QR code/JTI, không gửi trùng lặp, có retry khi lỗi mạng) |
| Base Commit | `602cf5ef03d6ff616393186edd1db4649c4ce43c` |
| Branch | `feature/EV-155-notification-confirmation-tickets` |
| Nghiệp vụ liên quan | **BR-O6 / Quy tắc 2** (Xử lý bất đồng bộ, không gọi sync API chặn luồng), **BR-O5** (Idempotency & Deduplication), **Quy tắc 4** (Structured Logging & Bảo vệ PII), **libs/go/mq** (AMQP retry & DLQ) |

---

## 1. Bối cảnh & Hiện trạng

### Bối cảnh nghiệp vụ
Theo quy định trong kiến trúc tổng thể [docs/02-kien-truc.md](file:///c:/Users/ADMIN/OneDrive/Documents/GitHub/EvenFlow-/docs/02-kien-truc.md) và backlog [docs/04-jira-backlog.md](file:///c:/Users/ADMIN/OneDrive/Documents/GitHub/EvenFlow-/docs/04-jira-backlog.md):
- Khách hàng sau khi hoàn tất thanh toán thành công (`order.paid`) và nhận vé điện tử (`ticketing.ticket.issued`) cần nhận ngay thư điện tử (Email) và/hoặc tin nhắn (SMS) xác nhận kèm thông tin chi tiết về vé, mã bảo mật JTI, và chữ ký QR để check-in tại sự kiện.
- Khi có sự kiện bị hủy hoặc hoàn tiền (`payment.refunded` - EV-154), hệ thống cần gửi thông báo hoàn tiền rõ ràng cho khách hàng.
- **Quy tắc 2 của EvenFlow**: Tuyệt đối không gửi email/SMS đồng bộ (Sync Call) chặn luồng xử lý Webhook hoặc luồng mua vé. Việc gửi thông báo là trách nhiệm chuyên biệt của dịch vụ `notification-svc`, được kích hoạt thông qua việc tiêu thụ message từ RabbitMQ.

### Hiện trạng mã nguồn trước EV-155
- `services/notification`: Mới chỉ là khung service sơ khai (`cmd/server/main.go`) với endpoint `/healthz`. Chưa có kiến trúc Hexagonal (Domain, Port, App, Adapter) và chưa có bất kỳ logic nghiệp vụ nào.
- `services/payment` và `services/ticketing` đã triển khai Transactional Outbox phát ra các sự kiện:
  - `order.paid` (`OutboxOrderPaidPayload` trong `services/payment/internal/domain/payment.go`)
  - `ticketing.ticket.issued` (`OutboxTicketIssuedPayload` trong `services/ticketing/internal/adapter/postgres/ticket_repository.go`)
  - `payment.refunded` (`OutboxPaymentRefundedPayload` trong `services/payment/internal/domain/refund.go`)
- `libs/go/mq`: Đã khai báo sẵn `NotificationTopology` với exchange `notification.direct` và queue `notification.email.q`.
- **Gaps cần giải quyết trong EV-155**:
  1. Xây dựng domain models và template engine chuẩn tiếng Việt cho Email & SMS.
  2. Xây dựng use cases: xử lý xác nhận thanh toán (`order.paid`), phát hành vé (`ticket.issued`), và thông báo hoàn tiền (`payment.refunded`).
  3. Xây dựng cổng trừu tượng (`port`) và adapter cho việc gửi Email (SMTP & Mock) và SMS (Provider & Mock).
  4. Cơ chế chống gửi lặp tin nhắn (Idempotency / Deduplication store) để đảm bảo không spam khách hàng khi message bị giao lặp lại (at-least-once).
  5. Cơ chế Retry khi gặp sự cố tạm thời (transient errors) và chuyển DLQ khi gặp lỗi vĩnh viễn (permanent errors) thông qua `libs/go/mq`.
  6. Endpoint HTTP REST API phục vụ kiểm thử, giám sát lịch sử thông báo và `/healthz`.

---

## 2. Tiêu chí nghiệm thu (Acceptance Criteria - AC)

### AC-1 — Tiếp nhận và phân tích sự kiện (Message Ingestion & Parsing)
- **Điều kiện**: Consumer nhận message từ RabbitMQ queue hoặc qua REST API kích hoạt.
- **Hành vi**:
  - Deserialize chính xác payload JSON của các sự kiện:
    - `order.paid`: `order_id`, `event_id`, `identity_id`, `amount_cents`, `currency`, `provider`, `provider_txn_id`, `paid_at`.
    - `ticketing.ticket.issued` / `ticket.issued`: `order_id`, `event_id`, `identity_id`, `ticket_count`, danh sách `tickets` (`id`, `ticket_type_id`, `jti`, `qr_sig`, `status`).
    - `payment.refunded`: `order_id`, `event_id`, `identity_id`, `amount_cents`, `currency`, `refunded_at`, `reason`.
  - Validate tính hợp lệ của dữ liệu (bắt buộc có `order_id`, `identity_id`, email hoặc số điện thoại người nhận hợp lệ).
  - Trả về `ErrPermanent` nếu JSON hỏng hoặc thiếu trường dữ liệu cốt lõi (poison pill) để không bị lặp vô tận.

### AC-2 — Template Engine Tiếng Việt chuẩn hóa (Vietnamese Notification Templates)
- **Email xác nhận đơn hàng (`order_confirmation`)**:
  - Tiêu đề: `[EvenFlow] Xác nhận đơn hàng #{order_id} thành công`
  - Nội dung HTML & Plain text: Tên sự kiện, Mã đơn hàng, Tổng tiền thanh toán (định dạng tiền tệ VND có dấu phân cách, ví dụ: `500.000 VND`), Cổng thanh toán, Thời gian thanh toán.
- **Email đính kèm vé điện tử (`ticket_issuance`)**:
  - Tiêu đề: `[EvenFlow] Vé điện tử của bạn cho đơn hàng #{order_id}`
  - Nội dung HTML & Plain text: Liệt kê chi tiết từng vé gồm: Mã vé (`ticket_id`), Hạng vé, Mã định danh duy nhất (`JTI`), Chuỗi chữ ký QR (`qr_sig`), Hướng dẫn check-in tại cổng sự kiện.
- **SMS thông báo ngắn gọn (`sms_order_paid` / `sms_ticket_issued`)**:
  - Nội dung tiếng Việt không dấu hoặc có dấu ngắn gọn dưới 160 ký tự: `EvenFlow: Don hang {order_id} thanh toan thanh cong {amount} VND. {count} ve da duoc xuat. Chi tiet da gui ve email cua ban.`
- **Email thông báo hoàn tiền (`order_refunded`)**:
  - Tiêu đề: `[EvenFlow] Thông báo hoàn tiền cho đơn hàng #{order_id}`
  - Nội dung: Số tiền hoàn lại, Lý do hoàn tiền, Thời gian hoàn tất.

### AC-3 — Chống gửi trùng lặp tuyệt đối (Idempotency & Deduplication)
- **Điều kiện**: Khi RabbitMQ giao lại cùng một message (do worker restart hoặc timeout ack) hoặc client gọi lại API.
- **Hành vi**:
  - Sinh khoá định danh duy nhất (Dedup Key):
    `dedup_key = fmt.Sprintf("%s:%s:%s", eventType, orderID, channel)`
    Ví dụ: `order.paid:ord-12345:email`, `ticket.issued:ord-12345:sms`.
  - Tra cứu trạng thái trong `IdempotencyStore`:
    - Nếu trạng thái đã là `COMPLETED` / `SENT`: Ghi nhận đã xử lý, bỏ qua việc gửi lại (`AlreadySent = true`), trả về `nil` (Ack message thành công).
    - Nếu chưa có: Tạo trạng thái `PENDING`, tiến hành gửi qua `EmailSender` / `SMSSender`, sau đó chuyển sang `SENT` kèm `sent_at`.
    - Đảm bảo thread-safe (chống race condition khi có nhiều luồng xử lý song song).

### AC-4 — Trừu tượng hóa Sender & Đa kênh (Sender Abstraction: Email & SMS)
- **Email Sender**:
  - `EmailSender` interface gồm phương thức `SendEmail(ctx context.Context, msg EmailMessage) error`.
  - `MockEmailSender`: Lưu trữ danh sách email gửi thành công, hỗ trợ kiểm tra assertions trong unit/integration test, có cờ mô phỏng lỗi tạm thời (`SimulateTransientError`) và lỗi vĩnh viễn (`SimulatePermanentError`).
  - `SMTPEmailSender`: Hiện thực giao thức SMTP tiêu chuẩn qua `net/smtp` với TLS/STARTTLS, cấu hình qua biến môi trường (`SMTP_HOST`, `SMTP_PORT`, `SMTP_USER`, `SMTP_PASSWORD`, `SMTP_FROM`).
- **SMS Sender**:
  - `SMSSender` interface gồm phương thức `SendSMS(ctx context.Context, msg SMSMessage) error`.
  - `MockSMSSender`: Lưu trữ danh sách SMS gửi thành công, hỗ trợ assertions trong unit tests.
  - `WebhookSMSSender`: Adapter gửi SMS qua HTTP webhook/SMS gateway của nhà mạng.

### AC-5 — Cơ chế Thử lại (Retry) & Phục hồi lỗi (Resilience)
- **Điều kiện**: Gặp lỗi trong quá trình gửi tin (mạng chập chờn, timeout SMTP).
- **Hành vi**:
  - Lỗi tạm thời (Transient error): Trả về lỗi thường để `libs/go/mq` thử lại qua retry exchange theo cấu hình `MaxAttempts` (mặc định 3 lần).
  - Lỗi vĩnh viễn (Permanent error, ví dụ: địa chỉ email/số điện thoại sai định dạng nghiêm trọng): Bọc lỗi bằng `mq.ErrPermanent` để đẩy ngay vào Dead Letter Queue (DLQ), không retry vô ích làm nghẽn hệ thống.
  - Ghi log có cấu trúc (Structured JSON Logging với `log/slog`): Đầy đủ `trace_id`, `order_id`, `event_type`, `channel`, `attempt`, `duration_ms`.
  - Tuân thủ Quy tắc 4: Không log nội dung nhạy cảm, mật khẩu hoặc token PII.

### AC-6 — HTTP REST API & Vận hành
- `GET /healthz`: Kiểm tra tính khả dụng của service (HTTP 200).
- `POST /v1/notifications/send`: Cho phép Ops kích hoạt gửi thông báo thủ công (hỗ trợ trường hợp khách cần gửi lại vé).
- `GET /v1/notifications/history/{order_id}`: Tra cứu lịch sử các thông báo đã gửi cho đơn hàng, phục vụ đối soát và audit.

---

## 3. Ranh giới (Scope)

### In Scope
- Thiết kế trọn vẹn kiến trúc Hexagonal-lite cho `services/notification`:
  - `domain/`: notification entities, channels, template definitions, validation.
  - `port/`: `EmailSender`, `SMSSender`, `IdempotencyStore`, `TemplateRenderer`, `NotificationLogRepository`.
  - `app/`: `SendOrderConfirmationUseCase`, `SendTicketIssuedUseCase`, `SendRefundNotificationUseCase`, `GetNotificationHistoryUseCase`.
  - `adapter/`:
    - `sender/`: `MockEmailSender`, `SMTPEmailSender`, `MockSMSSender`, `WebhookSMSSender`.
    - `store/`: `MemoryNotificationStore` (in-memory thread-safe dedup & history store cho test & standalone runtime), và `PostgresNotificationStore`.
    - `template/`: `GoTemplateRenderer` (chuẩn hoá tiếng Việt HTML & Plain text).
    - `rabbitmq/`: AMQP consumer tích hợp `libs/go/mq` lắng nghe các event `order.paid`, `ticketing.ticket.issued`, `payment.refunded`.
    - `http/`: REST API handlers (`/healthz`, `/v1/notifications/send`, `/v1/notifications/history/{order_id}`).
- Unit tests & TDD đầy đủ đạt coverage >= 70% trên mọi package cốt lõi.

### Out of Scope
- Trực tiếp xử lý thanh toán hay phát hành vé trong DB (thuộc `payment-svc` và `ticketing-svc`).
- Tích hợp push notification di động qua APNs/FCM trong phạm vi task này (dành cho epic mobile).
