# EV-151 — Kế hoạch thực thi (Plan)

## 1. Thứ tự thực hiện

| Bước | Nội dung | Lý do / Ràng buộc kiến trúc |
|---|---|---|
| **1** | Bổ sung Domain & Lỗi nghiệp vụ (`internal/domain`) | Thuần túy (pure Go), 0 external import. Định nghĩa lỗi `ErrInvalidSignature`, `ErrReplayDetected`, `ErrDuplicateWebhook`, struct `WebhookPayload`, `ProcessWebhookResult`, `OutboxMessage`. Viết unit test domain. |
| **2** | Bổ sung Ports (`internal/port`) | Nguyên tắc DIP. Mở rộng `PaymentRepository` với các phương thức giao dịch atomic: `ProcessPaymentSuccessTx(ctx, params)` và `ProcessPaymentFailedTx(ctx, params)`. Bổ sung trích xuất timestamp vào `PaymentGateway` / `CallbackResult`. |
| **3** | Cập nhật Gateway Adapters (`internal/adapter/gateway`) | Bổ sung thẩm định timestamp và chữ ký chống replay cho `SandboxGateway`, `VNPayGateway`, `MoMoGateway`, `StripeGateway`. Viết unit test kiểm tra chữ ký sai & replay time window cho từng cổng. |
| **4** | Hiện thực Use Case `ProcessWebhookUseCase` (`internal/app`) | Điều phối: Xác minh gateway → Kiểm tra cửa sổ timestamp chống replay → Kiểm tra trùng lặp giao dịch (BR-O6) → Kích hoạt DB Transaction (cập nhật `payments`, `orders` -> `PAID`, ghi `outbox_events` -> `order.paid`) → Trả về kết quả 200 idempotent. Viết unit test app use case. |
| **5** | Hiện thực Repository Transaction (`internal/adapter/postgres`) | Triển khai `ProcessPaymentSuccessTx` và `ProcessPaymentFailedTx` trên `PostgresRepository` (sử dụng `pgx.Tx` với row lock hoặc unique constraint) và `MemoryRepository` (sync.Mutex mô phỏng atomic transaction). |
| **6** | Bổ sung HTTP Webhook Handlers (`internal/adapter/http`) | Triển khai handler `POST /v1/payments/webhook/{provider}` và `GET /v1/payments/webhook/{provider}`. Viết `handler_test.go` bao gồm cả concurrency test 100 luồng đồng thời kiểm chứng exactly-once. |
| **7** | Khởi tạo Server & Đi dây tổng thể (`cmd/server/main.go`) | Đăng ký route webhook mới vào HTTP server, nạp config secret keys và graceful shutdown. |
| **8** | Chạy Gate tự động & Thu thập chứng cứ (`evidence.md`, `traceability.md`, `code-review.md`) | Chạy typecheck, `go vet`, `gofmt`, test coverage, và biên bản review độc lập theo 4 Quy tắc Bất biến. |

---

## 2. Danh sách File thay đổi

### Thêm mới
- `services/payment/internal/app/process_webhook.go`
- `services/payment/internal/app/process_webhook_test.go`
- `docs/workflow/EV-151/baseline.md`
- `docs/workflow/EV-151/spec.md`
- `docs/workflow/EV-151/plan.md`
- `docs/workflow/EV-151/traceability.md`
- `docs/workflow/EV-151/evidence.md`
- `docs/workflow/EV-151/code-review.md`

### Sửa đổi
- `services/payment/internal/domain/errors.go`: Thêm các lỗi `ErrInvalidSignature`, `ErrReplayDetected`, `ErrDuplicateWebhook`.
- `services/payment/internal/domain/payment.go`: Thêm model kết quả xử lý webhook và outbox payload.
- `services/payment/internal/domain/payment_test.go`: Bổ sung test kiểm tra các hàm/lỗi mới.
- `services/payment/internal/port/gateway.go`: Mở rộng `CallbackResult` chứa `Timestamp` và `Payload`.
- `services/payment/internal/port/repository.go`: Mở rộng interface `PaymentRepository` với các hàm transaction atomic cho webhook.
- `services/payment/internal/adapter/gateway/sandbox.go`: Bổ sung kiểm tra timestamp chống replay và verify logic.
- `services/payment/internal/adapter/gateway/sandbox_test.go`: Bổ sung test timestamp replay.
- `services/payment/internal/adapter/gateway/vnpay.go`: Hỗ trợ parse `vnp_PayDate` và verify chữ ký.
- `services/payment/internal/adapter/gateway/vnpay_test.go`: Bổ sung test chữ ký sai và timestamp quá hạn.
- `services/payment/internal/adapter/gateway/momo.go`: Hỗ trợ parse `responseTime` và verify chữ ký.
- `services/payment/internal/adapter/gateway/momo_test.go`: Bổ sung test chữ ký sai và timestamp quá hạn.
- `services/payment/internal/adapter/gateway/stripe.go`: Hỗ trợ parse `created` timestamp và verify webhook.
- `services/payment/internal/adapter/gateway/stripe_test.go`: Bổ sung test chữ ký sai và timestamp quá hạn.
- `services/payment/internal/adapter/postgres/repository.go`: Hiện thực `ProcessPaymentSuccessTx` và `ProcessPaymentFailedTx`.
- `services/payment/internal/adapter/postgres/memory_repository.go`: Hiện thực in-memory atomic processing cho unit & integration tests.
- `services/payment/internal/adapter/http/handler.go`: Bổ sung routes webhook `POST` & `GET /v1/payments/webhook/{provider}`.
- `services/payment/internal/adapter/http/handler_test.go`: Bổ sung test webhook endpoints, test chữ ký sai, test replay, và test 100 requests concurrent.
- `services/payment/cmd/server/main.go`: Khởi tạo use case webhook và đăng ký route.

### TUYỆT ĐỐI CẤM SỬA
- `migrations/0001_core.sql`: Giữ nguyên schema gốc (bảng `orders`, `payments`, `outbox_events`, `payment_inbox` đã đầy đủ).
- `services/ticketing/**`: Không can thiệp mã nguồn ticketing.
- `libs/go/httpx`, `libs/go/idem`: Giữ nguyên thư viện dùng chung.

---

## 3. Hợp đồng API / Type Signatures

### Domain & Port
```go
package domain

type WebhookProcessResult struct {
    Success       bool
    AlreadyDone   bool
    OrderID       string
    PaymentID     string
    ProviderTxnID string
    Status        PaymentStatus
    Message       string
}

type ProcessWebhookParams struct {
    Provider      Provider
    ProviderTxnID string
    OrderID       string
    PaymentID     string
    AmountCents   int64
    Currency      string
    Status        PaymentStatus
    RawPayload    map[string]any
    TraceID       string
    Timestamp     time.Time
}
```

```go
package port

type PaymentRepository interface {
    GetOrder(ctx context.Context, orderID string) (domain.OrderView, error)
    CreatePaymentAndMarkPending(ctx context.Context, payment domain.Payment) error
    GetPayment(ctx context.Context, paymentID string) (domain.Payment, error)
    GetPaymentByOrder(ctx context.Context, orderID string) (domain.Payment, error)

    // Bổ sung cho EV-151 (Quy tắc bất biến 1 - Atomic DB Transaction)
    ProcessPaymentSuccessTx(ctx context.Context, params domain.ProcessWebhookParams) (domain.WebhookProcessResult, error)
    ProcessPaymentFailedTx(ctx context.Context, params domain.ProcessWebhookParams) (domain.WebhookProcessResult, error)
}
```

### HTTP Endpoints
- `POST /v1/payments/webhook/{provider}`: Nhận JSON payload hoặc form data từ cổng thanh toán.
- `GET /v1/payments/webhook/{provider}`: Nhận query string từ cổng thanh toán (dành riêng cho VNPay IPN/redirect callback).
- Phản hồi:
  - Thành công: HTTP 200 OK `{"status": "ok", "order_id": "...", "status": "SUCCEEDED"}`
  - Trùng lặp (Duplicate): HTTP 200 OK `{"status": "ok", "message": "duplicate webhook ignored", "already_processed": true}` (BR-O6)
  - Chữ ký sai: HTTP 400 Bad Request `{"error": "chữ ký không hợp lệ"}`
  - Replay attack (quá hạn thời gian): HTTP 400 Bad Request `{"error": "webhook quá cửa sổ thời gian cho phép"}`
