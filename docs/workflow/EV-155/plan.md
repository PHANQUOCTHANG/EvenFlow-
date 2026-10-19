# EV-155 (EVF-75) — Kế hoạch thực thi (Plan)

| Thuộc tính | Chi tiết |
|---|---|
| Task | **EV-155** (Mã backlog: **EVF-75**) |
| Tiêu đề | Email/SMS xác nhận + đính kèm vé qua notification-svc |
| Phương pháp | **Disciplined Vibe Coding (6-Phase Vibe Loop)** |
| Kiến trúc | **Hexagonal Architecture (Domain -> Port -> App -> Adapter)** |

---

## 1. Thứ tự thực hiện

| Bước | Tầng | Nội dung thực hiện | Lý do & Ràng buộc |
|---|---|---|---|
| **1** | **Domain** (`internal/domain`) | Định nghĩa các models: `Notification`, `EmailMessage`, `SMSMessage`, `TicketDetail`, `OrderPaidEvent`, `TicketIssuedEvent`, `PaymentRefundedEvent`, `Channel`, `Status`. Định nghĩa `errors.go` (Domain errors). Viết unit tests cho domain. | Domain độc lập 100%, 0 external dependencies (không DB, không HTTP, không network). |
| **2** | **Port** (`internal/port`) | Định nghĩa các interfaces: `EmailSender`, `SMSSender`, `TemplateRenderer`, `NotificationStore`. | Hợp đồng giao tiếp trừu tượng phục vụ usecase và adapters. |
| **3** | **Template Engine** (`internal/adapter/template`) | Hiện thực `GoTemplateRenderer`: Mẫu email xác nhận đơn hàng, email đính kèm vé và QR/JTI, email hoàn tiền, mẫu tin nhắn SMS tiếng Việt chuẩn. Viết unit tests. | Đảm bảo khách hàng nhận nội dung tiếng Việt chuẩn xác, rõ ràng, thẩm mỹ. |
| **4** | **Sender Adapters** (`internal/adapter/sender`) | Hiện thực `MockEmailSender`, `SMTPEmailSender`, `MockSMSSender`, `WebhookSMSSender`. Hỗ trợ mô phỏng lỗi tạm thời và lỗi vĩnh viễn. Viết unit tests. | Đa dạng hóa bộ gửi, hỗ trợ cả môi trường test, local, và production. |
| **5** | **Store Adapter** (`internal/adapter/store`) | Hiện thực `MemoryNotificationStore` (thread-safe sync.RWMutex) cho dedup & history logging. Viết unit tests. | Chống gửi trùng lặp (idempotency) và truy vết lịch sử gửi tin. |
| **6** | **App Use Cases** (`internal/app`) | Hiện thực các use case: `ProcessOrderPaidUseCase`, `ProcessTicketIssuedUseCase`, `ProcessRefundUseCase`, `GetNotificationHistoryUseCase`. Viết unit tests (TDD). | Điều phối toàn bộ luồng nghiệp vụ thông báo. |
| **7** | **RabbitMQ Consumer** (`internal/adapter/rabbitmq`) | Tích hợp với `libs/go/mq` để tiếp nhận và phân phối message từ queue theo routing keys (`order.paid`, `ticketing.ticket.issued`, `payment.refunded`). Xử lý retry và bọc `mq.ErrPermanent`. Viết unit tests. | Đảm bảo tính bất đồng bộ (Quy tắc 2) và tin cậy (at-least-once with retry). |
| **8** | **HTTP & Server Wiring** (`internal/adapter/http`, `cmd/server`) | Xây dựng REST handler (`/healthz`, `/v1/notifications/send`, `/v1/notifications/history/{order_id}`). Đấu nối dependencies trong `cmd/server/main.go`. Viết unit tests. | Giao diện điều khiển và giám sát cho Ops và kiểm thử tự động. |
| **9** | **Chạy Gates & Review** | Chạy toàn bộ test suite (`go test -v -race ./...`), đo coverage, biên soạn `evidence.md` và thực hiện review đối kháng vào `code-review.md`. | Tuân thủ 100% quy chuẩn Vibe Coding của EvenFlow. |

---

## 2. Danh sách File thay đổi

### Thêm mới
- `services/notification/internal/domain/notification.go`
- `services/notification/internal/domain/errors.go`
- `services/notification/internal/domain/notification_test.go`
- `services/notification/internal/port/sender.go`
- `services/notification/internal/port/renderer.go`
- `services/notification/internal/port/store.go`
- `services/notification/internal/adapter/template/renderer.go`
- `services/notification/internal/adapter/template/renderer_test.go`
- `services/notification/internal/adapter/sender/mock.go`
- `services/notification/internal/adapter/sender/smtp.go`
- `services/notification/internal/adapter/sender/sms.go`
- `services/notification/internal/adapter/sender/sender_test.go`
- `services/notification/internal/adapter/store/memory_store.go`
- `services/notification/internal/adapter/store/memory_store_test.go`
- `services/notification/internal/app/process_order_paid.go`
- `services/notification/internal/app/process_order_paid_test.go`
- `services/notification/internal/app/process_ticket_issued.go`
- `services/notification/internal/app/process_ticket_issued_test.go`
- `services/notification/internal/app/process_refund.go`
- `services/notification/internal/app/process_refund_test.go`
- `services/notification/internal/app/get_history.go`
- `services/notification/internal/app/get_history_test.go`
- `services/notification/internal/adapter/rabbitmq/consumer.go`
- `services/notification/internal/adapter/rabbitmq/consumer_test.go`
- `services/notification/internal/adapter/http/handler.go`
- `services/notification/internal/adapter/http/handler_test.go`
- `docs/workflow/EV-155/baseline.md`
- `docs/workflow/EV-155/spec.md`
- `docs/workflow/EV-155/plan.md`
- `docs/workflow/EV-155/traceability.md`
- `docs/workflow/EV-155/evidence.md`
- `docs/workflow/EV-155/code-review.md`

### Sửa đổi
- `services/notification/cmd/server/main.go`
- `services/notification/go.mod`

### TUYỆT ĐỐI CẤM SỬA
- `services/ticketing/internal/domain/purchase_limit.go` & `inventory.go` (Không can thiệp logic chống oversell)
- `migrations/0001_core.sql`
- `libs/go/mq/topology.go` (Topology `notification.direct` / `notification.email.q` đã chuẩn)
- Các file test baseline hiện có của các service khác

---

## 3. Hợp đồng API / Type Signatures

### Domain Types
```go
type Channel string
const (
    ChannelEmail Channel = "email"
    ChannelSMS   Channel = "sms"
)

type NotificationStatus string
const (
    StatusPending NotificationStatus = "PENDING"
    StatusSent    NotificationStatus = "SENT"
    StatusFailed  NotificationStatus = "FAILED"
    StatusSkipped NotificationStatus = "SKIPPED"
)

type Notification struct {
    ID          string             `json:"id"`
    DedupKey    string             `json:"dedup_key"`
    EventType   string             `json:"event_type"`
    OrderID     string             `json:"order_id"`
    Recipient   string             `json:"recipient"`
    Channel     Channel            `json:"channel"`
    Subject     string             `json:"subject,omitempty"`
    Status      NotificationStatus `json:"status"`
    ErrorReason string             `json:"error_reason,omitempty"`
    SentAt      *time.Time         `json:"sent_at,omitempty"`
    CreatedAt   time.Time          `json:"created_at"`
}
```

### Port Interfaces
```go
type EmailSender interface {
    SendEmail(ctx context.Context, msg EmailMessage) error
}

type SMSSender interface {
    SendSMS(ctx context.Context, msg SMSMessage) error
}

type TemplateRenderer interface {
    RenderOrderPaidEmail(evt OrderPaidEvent) (subject, htmlBody, textBody string, err error)
    RenderTicketIssuedEmail(evt TicketIssuedEvent) (subject, htmlBody, textBody string, err error)
    RenderRefundEmail(evt PaymentRefundedEvent) (subject, htmlBody, textBody string, err error)
    RenderSMS(eventType string, data any) (body string, err error)
}

type NotificationStore interface {
    CheckOrSetDedup(ctx context.Context, dedupKey string, notif Notification) (alreadyProcessed bool, err error)
    MarkSent(ctx context.Context, dedupKey string, notifID string, sentAt time.Time) error
    MarkFailed(ctx context.Context, dedupKey string, notifID string, reason string) error
    GetHistoryByOrderID(ctx context.Context, orderID string) ([]Notification, error)
}
```
