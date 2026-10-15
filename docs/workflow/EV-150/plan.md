# EV-150 — Kế hoạch thực thi (Plan)

## 1. Thứ tự thực hiện

| Bước | Nội dung | Lý do / Ràng buộc kiến trúc |
|---|---|---|
| **1** | Định nghĩa Domain & Config (`internal/domain`, `internal/config`) | Thuần túy (pure Go), 0 external import. Định nghĩa Entity `Payment`, `Provider`, `PaymentStatus`, lỗi nghiệp vụ (`ErrHoldExpired`, `ErrAmountMismatch`, `ErrOrderNotHeld`,...). Viết unit test domain chạy tức thì. |
| **2** | Định nghĩa Ports (`internal/port`) | Nguyên tắc DIP (Dependency Inversion). Khai báo interface `PaymentGateway`, `PaymentRepository`, `GatewayRegistry`, `Clock`, `IDGenerator`. |
| **3** | Hiện thực Gateway Adapters (`internal/adapter/gateway`) | Xây dựng 4 adapter: `SandboxMockGateway` (local 100%), `VNPayGateway` (HMAC-SHA512), `MoMoGateway` (HMAC-SHA256), `StripeGateway` (Checkout Session mockable) và `GatewayRegistry` (Factory). Viết đầy đủ unit tests cho từng cổng. |
| **4** | Hiện thực Application Use Cases (`internal/app`) | Use case `InitiatePaymentUseCase`: Kiểm tra `HELD`, chặn nếu quá hạn `expires_at` (BR-O2), cập nhật trạng thái đơn sang `PAYMENT_PENDING` trong transaction, ghi nhận `payments` và gọi adapter cổng. Use case `GetPaymentUseCase`, `ListMethodsUseCase`. Viết unit test use case với mock repo & gateway. |
| **5** | Hiện thực Repository Adapter (`internal/adapter/postgres`) | Triển khai `PostgresRepository` trên bảng `orders` và `payments` với Transaction an toàn (Quy tắc bất biến 1). Cung cấp `MemoryRepository` dùng cho mock và standalone testing. |
| **6** | Hiện thực HTTP Adapter (`internal/adapter/http`) | Triển khai handler `POST /v1/events/{eventID}/payments` (bắt buộc `Idempotency-Key` BR-O5 qua middleware `idem`), `GET /v1/payments/{paymentID}`, `GET /v1/payments/methods`. Viết `handler_test.go`. |
| **7** | Khởi tạo Server & Đi dây tổng thể (`cmd/server/main.go`) | Đọc cấu hình môi trường, khởi tạo gateway registry theo config, gắn router HTTP và graceful shutdown. |
| **8** | Chạy Gate tự động & Thu thập chứng cứ (`evidence.md`, `traceability.md`, `code-review.md`) | Chạy typecheck, lint, test coverage, đối chiếu 4 quy tắc bất biến. |

---

## 2. Danh sách File thay đổi

### Thêm mới
- `services/payment/internal/config/config.go`
- `services/payment/internal/domain/payment.go`
- `services/payment/internal/domain/errors.go`
- `services/payment/internal/domain/payment_test.go`
- `services/payment/internal/port/gateway.go`
- `services/payment/internal/port/repository.go`
- `services/payment/internal/adapter/gateway/sandbox.go`
- `services/payment/internal/adapter/gateway/sandbox_test.go`
- `services/payment/internal/adapter/gateway/vnpay.go`
- `services/payment/internal/adapter/gateway/vnpay_test.go`
- `services/payment/internal/adapter/gateway/momo.go`
- `services/payment/internal/adapter/gateway/momo_test.go`
- `services/payment/internal/adapter/gateway/stripe.go`
- `services/payment/internal/adapter/gateway/stripe_test.go`
- `services/payment/internal/adapter/gateway/factory.go`
- `services/payment/internal/adapter/gateway/factory_test.go`
- `services/payment/internal/app/initiate_payment.go`
- `services/payment/internal/app/initiate_payment_test.go`
- `services/payment/internal/app/get_payment.go`
- `services/payment/internal/app/list_methods.go`
- `services/payment/internal/adapter/postgres/repository.go`
- `services/payment/internal/adapter/postgres/memory_repository.go`
- `services/payment/internal/adapter/http/handler.go`
- `services/payment/internal/adapter/http/dto.go`
- `services/payment/internal/adapter/http/handler_test.go`
- `docs/workflow/EV-150/traceability.md`
- `docs/workflow/EV-150/evidence.md`
- `docs/workflow/EV-150/code-review.md`

### Sửa đổi
- `services/payment/cmd/server/main.go`: Khởi tạo và wire cấu hình, dependencies, HTTP handlers.
- `services/payment/go.mod`: Bổ sung dependency cần thiết (`github.com/jackc/pgx/v5`, `github.com/google/uuid`,...).

### TUYỆT ĐỐI CẤM SỬA
- `migrations/0001_core.sql`: Không sửa schema DB vì các bảng `orders`, `payments`, `payment_inbox` đã có sẵn.
- `libs/go/httpx`, `libs/go/idem`: Giữ nguyên tính toàn vẹn của thư viện dùng chung.
- `services/ticketing/**`: Không can thiệp vào logic giữ vé của ticketing.

---

## 3. Hợp đồng API / Type Signatures

### Domain & Port
```go
package domain

type Provider string
const (
    ProviderSandbox Provider = "sandbox"
    ProviderVNPay   Provider = "vnpay"
    ProviderMoMo    Provider = "momo"
    ProviderStripe  Provider = "stripe"
)

type PaymentStatus string
const (
    StatusPending   PaymentStatus = "PENDING"
    StatusSucceeded PaymentStatus = "SUCCEEDED"
    StatusFailed    PaymentStatus = "FAILED"
    StatusRefunded  PaymentStatus = "REFUNDED"
)

type Payment struct {
    ID            string
    OrderID       string
    Provider      Provider
    ProviderTxnID string
    AmountCents   int64
    Currency      string
    Status        PaymentStatus
    CreatedAt     time.Time
}
```

```go
package port

type PaymentRequest struct {
    EventID     string
    OrderID     string
    HoldID      string
    AmountCents int64
    Currency    string
    BuyerName   string
    BuyerEmail  string
    BuyerPhone  string
    ReturnURL   string
    ClientIP    string
}

type PaymentResult struct {
    PaymentID     string
    OrderID       string
    RedirectURL   string
    Status        string
    ProviderTxnID string
}

type PaymentGateway interface {
    Provider() domain.Provider
    InitiatePayment(ctx context.Context, req PaymentRequest) (PaymentResult, error)
    VerifyCallback(ctx context.Context, params map[string]string) (bool, string, error)
}
```
