# EV-155 (EVF-75) — Adversarial Code Review

## 1. Tổng quan Đánh giá (Review Summary)
- **Reviewer Role**: Senior Tech Lead & Security Auditor
- **Target Branch**: `feature/EV-155-notification-confirmation-tickets`
- **Scope**: Toàn bộ thay đổi mã nguồn triển khai dịch vụ thông báo xác nhận thanh toán và gửi vé điện tử kèm mã QR (`services/notification`).
- **Trạng thái**: **APPROVED** (100% Pass)

---

## 2. Checklist 4 Quy tắc Bất biến của EvenFlow

| Quy tắc | Yêu cầu | Thực tế kiểm tra trong mã nguồn | Đánh giá |
|---|---|---|---|
| **Quy tắc 1: DB Transaction** | Mọi thay đổi dữ liệu phụ thuộc lẫn nhau phải trong 1 transaction an toàn; không có thao tác ngoài transaction gây bất nhất | `NotificationStore` quản lý trạng thái thong bao (`PENDING`, `SENT`, `FAILED`) và cơ chế Deduplication bằng thread-safe mutex lock và atomic check-or-set. Không có goroutine ngầm làm rò rỉ trạng thái. | **ĐẠT** |
| **Quy tắc 2: Sync API** | Không gọi API bên thứ ba (Email SMTP / SMS / LLM) chặn luồng HTTP thanh toán / mua vé; xử lý bất đồng bộ qua RabbitMQ | Toàn bộ việc gửi thông báo được kích hoạt bất đồng bộ qua AMQP consumer (`NotificationEventConsumer`) lắng nghe message từ RabbitMQ (`order.paid`, `ticketing.ticket.issued`, `payment.refunded`). Các cổng SMTP và SMS đều có context timeout 5s–15s nghiêm ngặt. Luồng HTTP chính của mua vé hoàn toàn không bị ảnh hưởng. | **ĐẠT** |
| **Quy tắc 3: Data Integrity & Invariants** | Không oversell, không bypass Lua gate hay rule kho vé | Dịch vụ thông báo hoàn toàn tách biệt, không can thiệp vào Redis ZSET hay logic cấp phát kho vé của ticketing service. | **ĐẠT** |
| **Quy tắc 4: Observability & Security** | Structured JSON logging có `trace_id`, `event_id`, không log thông tin nhạy cảm PII | Mọi log entry đều sử dụng `slog.NewJSONHandler`, ghi nhận `trace_id`, `order_id`, `event_type`, `attempt`. Không ghi log mật khẩu SMTP, token hay thông tin thẻ tín dụng của khách. | **ĐẠT** |

---

## 3. Phân tích Kỹ thuật & Rà soát Lỗ hổng Tiềm ẩn

### 3.1. Phân tích Kiến trúc Hexagonal-lite
- **Domain Layer (`domain/notification.go`, `domain/errors.go`)**:
  - Độc lập 100%, không import bất kỳ package bên ngoài nào (0 external dependencies).
  - Validation chặt chẽ định dạng email và số điện thoại Việt Nam (`emailRegex`, `phoneRegex`).
  - Hàm tiện ích định dạng tiền tệ tiếng Việt `FormatCurrencyVND` xử lý chính xác cả số âm và số lớn.
- **Port Layer (`port/sender.go`, `port/renderer.go`, `port/store.go`)**:
  - Giao diện sạch sẽ, tuân thủ nguyên lý Dependency Inversion Principle (DIP).
- **Adapter Layer (`adapter/template`, `adapter/sender`, `adapter/store`, `adapter/rabbitmq`, `adapter/http`)**:
  - `template`: Go template renderer tiếng Việt chuẩn hoá, hỗ trợ cả HTML giàu thẩm mỹ và Plain text ngắn gọn cho SMS.
  - `sender`: Tách bạch rõ ràng giữa MockSender (cho testing/local dev) và SMTPEmailSender / WebhookSMSSender (cho production).
  - `store`: `MemoryNotificationStore` bảo vệ chống race condition bằng `sync.RWMutex`.
  - `rabbitmq`: Phân phối routing key linh hoạt (`order.paid`, `ticket.issued`, `payment.refunded`), xử lý poison pill bằng `mq.ErrPermanent` để đẩy vào DLQ, cho phép retry khi gặp lỗi tạm thời.
  - `http`: REST endpoints đầy đủ (`GET /healthz`, `POST /v1/notifications/send`, `GET /v1/notifications/history/{order_id}`).

### 3.2. Khả năng Chịu tải & Chống Lặp (Idempotency & Deduplication)
- **At-least-once Delivery**: Trong môi trường phân tán chịu tải cao, tin nhắn từ RabbitMQ có thể bị gửi lặp lại.
- **Giải pháp**: Cơ chế sinh `DedupKey = fmt.Sprintf("%s:%s:%s", eventType, orderID, channel)` kết hợp với `CheckOrSetDedup`. Nếu bản ghi đã tồn tại với trạng thái `SENT` hoặc `PENDING`, consumer sẽ bỏ qua việc gửi lại (`res.EmailSkipped = true`, `res.SMSSkipped = true`) và trả về `nil` để ACK message, triệt tiêu hoàn toàn nguy cơ spam khách hàng.

---

## 4. Các phát hiện trong quá trình Review
- **BLOCKER**: Không có.
- **MAJOR**: Không có.
- **MINOR**:
  - *Phát hiện*: Cần đảm bảo nếu sự kiện gửi thiếu cả email lẫn số điện thoại thì không retry vô tận.
  - *Xử lý*: Đã bổ sung kiểm tra trong `consumer.go`, nếu gặp `ErrInvalidEventPayload` hoặc `ErrInvalidRecipient` thì bọc ngay bằng `mq.ErrPermanent` để chuyển vào DLQ, ngăn chặn hiện tượng tắc nghẽn hàng đợi (poison pill).

---

## 5. Kết luận Review
Toàn bộ mã nguồn đạt chất lượng xuất sắc, vượt qua toàn bộ 3 Gates kiểm tra tự động (Format, Vet, Test 100% pass với coverage từ 71.1% đến 97.7%, Build thành công), tuân thủ 100% chuẩn mực kỹ thuật của EvenFlow.

**Quyết định: APPROVED FOR COMMIT & PR**
