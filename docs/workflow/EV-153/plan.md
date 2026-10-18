# EV-153 (EVF-73) — Kế hoạch thực thi (Plan)

| Thuộc tính | Chi tiết |
|---|---|
| Task | **EV-153** (Mã backlog: **EVF-73**) |
| Tiêu đề | Job đối soát: đơn quá 15 phút không có webhook → hỏi cổng thanh toán, tự chữa |
| Phương pháp | **Disciplined Vibe Coding (6-Phase Vibe Loop)** |
| Kiến trúc | **Hexagonal Architecture (Domain -> Port -> App -> Adapter)** |

---

## 1. Thứ tự thực hiện

| Bước | Tầng | Nội dung thực hiện | Lý do & Ràng buộc |
|---|---|---|---|
| **1** | **Domain** (`internal/domain`) | Bổ sung các entity & value object: `StaleOrderInfo`, `ReconciliationResult`, `ReconciliationOutcome`, `PaymentQueryRequest`, `PaymentQueryResult`. Viết unit test domain thuần túy. | Domain không phụ thuộc vào bất kỳ thư viện ngoài hay DB/HTTP nào. |
| **2** | **Port** (`internal/port`) | Cập nhật `PaymentGateway` thêm hàm `QueryPayment`. Cập nhật `PaymentRepository` thêm hàm `GetStalePendingOrders`, `ProcessOrderCancellationTx`. Định nghĩa `ReconciliationMetrics` port. | Khung giao tiếp trừu tượng cho use case và adapter. |
| **3** | **Gateway Adapters** (`internal/adapter/gateway`) | Hiện thực `QueryPayment` cho cả 4 cổng: `SandboxGateway` (hỗ trợ các mode Success, Fail, Delay), `VNPayGateway` (QueryDR), `MoMoGateway` (Query Status), `StripeGateway` (PaymentIntent Retrieve). Viết unit tests cho từng adapter. | Cho phép hệ thống chủ động hỏi trạng thái tiền thực tế từ nhà cung cấp. |
| **4** | **Metrics Adapter** (`internal/adapter/metrics`) | Xây dựng Prometheus metrics adapter cho payment service: `payment_webhook_lag_seconds`, `payment_reconciliation_total`, `payment_reconciliation_duration_seconds`. Viết unit test metrics. | Đáp ứng AC-2 của Jira ticket và kiến trúc quan sát (Observability). |
| **5** | **Postgres & Memory Repo** (`internal/adapter/postgres`) | Hiện thực `GetStalePendingOrders` và `ProcessOrderCancellationTx` trên cả `PostgresRepository` và `MemoryRepository`. Tuân thủ tuyệt đối Quy tắc 1 (Atomic DB Transaction) và BR-O7 (Idempotent hold release). Viết unit & integration tests. | Nền tảng dữ liệu phục vụ đối soát và tự chữa. |
| **6** | **App Use Case** (`internal/app`) | Hiện thực `ReconcileStaleOrdersUseCase`: Quét đơn treo quá 15 phút -> Gọi Gateway Query ngoài transaction (Quy tắc 2) -> Tự chữa atomic sang `PAID` hoặc `CANCELLED` (Quy tắc 1) -> Export metric lag -> Trả về báo cáo tổng hợp. Viết unit test toàn diện (TDD). | Bộ não điều phối toàn bộ nghiệp vụ đối soát. |
| **7** | **HTTP & Server Wiring** (`internal/adapter/http`, `cmd/server`) | Bổ sung route `POST /internal/reconcile`, endpoint `/metrics`. Tích hợp background worker ticker vào `cmd/server/main.go` với graceful shutdown context. | Kích hoạt worker định kỳ và phục vụ API vận hành. |
| **8** | **Chạy Gate & Review** | Chạy toàn bộ test suites (`go test -v -race ./...`), kiểm tra zero error/warning, thu thập log vào `evidence.md`, thực hiện review đối kháng vào `code-review.md`. | Bảo đảm chất lượng theo quy trình Vibe Coding chuẩn hóa. |

---

## 2. Danh sách File thay đổi

### Thêm mới
- `services/payment/internal/app/reconcile_orders.go`
- `services/payment/internal/app/reconcile_orders_test.go`
- `services/payment/internal/adapter/metrics/prom.go`
- `services/payment/internal/adapter/metrics/prom_test.go`
- `docs/workflow/EV-153/baseline.md`
- `docs/workflow/EV-153/spec.md`
- `docs/workflow/EV-153/plan.md`
- `docs/workflow/EV-153/traceability.md`
- `docs/workflow/EV-153/evidence.md`
- `docs/workflow/EV-153/code-review.md`

### Sửa đổi
- `services/payment/internal/domain/payment.go`
- `services/payment/internal/domain/payment_test.go`
- `services/payment/internal/port/gateway.go`
- `services/payment/internal/port/repository.go`
- `services/payment/internal/port/metrics.go`
- `services/payment/internal/adapter/gateway/sandbox.go`
- `services/payment/internal/adapter/gateway/sandbox_test.go`
- `services/payment/internal/adapter/gateway/vnpay.go`
- `services/payment/internal/adapter/gateway/vnpay_test.go`
- `services/payment/internal/adapter/gateway/momo.go`
- `services/payment/internal/adapter/gateway/momo_test.go`
- `services/payment/internal/adapter/gateway/stripe.go`
- `services/payment/internal/adapter/gateway/stripe_test.go`
- `services/payment/internal/adapter/postgres/repository.go`
- `services/payment/internal/adapter/postgres/memory_repository.go`
- `services/payment/internal/adapter/postgres/repository_test.go`
- `services/payment/internal/adapter/http/handler.go`
- `services/payment/internal/adapter/http/handler_test.go`
- `services/payment/cmd/server/main.go`

### TUYỆT ĐỐI CẤM SỬA
- `migrations/0001_core.sql` (Cấu trúc DB đã sẵn sàng, không thay đổi schema làm gãy baseline)
- `services/ticketing/...` (Dịch vụ vé độc lập, không sửa mã)
- `services/waitingroom/...` (Dịch vụ phòng chờ độc lập, không sửa mã)
- `services/payment/internal/domain/errors.go` (Bảo toàn các lỗi domain gốc)

---

## 3. Hợp đồng API & Type Signatures

### Domain & Port Signatures
```go
// StaleOrderInfo dai dien cho mot don hang treo qua 15 phut can doi soat
type StaleOrderInfo struct {
    OrderID       string
    EventID       string
    IdentityID    string
    Status        string
    TotalCents    int64
    Currency      string
    UpdatedAt     time.Time
    CreatedAt     time.Time
    PaymentID     string
    Provider      Provider
    ProviderTxnID string
}

// PaymentQueryRequest tham so gui toi cong thanh toan de tra cuu
type PaymentQueryRequest struct {
    OrderID       string
    PaymentID     string
    ProviderTxnID string
}

// PaymentQueryResult ket qua tra ve tu cong thanh toan
type PaymentQueryResult struct {
    ProviderTxnID string
    Status        domain.PaymentStatus // SUCCEEDED, FAILED, PENDING
    AmountCents   int64
    RawResponse   map[string]any
    Message       string
}

// PaymentGateway interface mo rong
type PaymentGateway interface {
    Provider() domain.Provider
    InitiatePayment(ctx context.Context, req PaymentRequest) (PaymentResult, error)
    VerifyCallback(ctx context.Context, params map[string]string) (CallbackResult, error)
    QueryPayment(ctx context.Context, req PaymentQueryRequest) (PaymentQueryResult, error)
}

// PaymentRepository interface mo rong
type PaymentRepository interface {
    // ... cac ham cu ...
    GetStalePendingOrders(ctx context.Context, olderThan time.Duration, limit int) ([]domain.StaleOrderInfo, error)
    ProcessOrderCancellationTx(ctx context.Context, params domain.CancelOrderParams) (domain.CancelOrderResult, error)
}

// ReconciliationResult tong hop ket qua sau moi dot doi soat
type ReconciliationResult struct {
    TotalScanned    int           `json:"total_scanned"`
    HealedPaid      int           `json:"healed_paid"`
    HealedCancelled int           `json:"healed_cancelled"`
    StillPending    int           `json:"still_pending"`
    Skipped         int           `json:"skipped"`
    Errors          int           `json:"errors"`
    Duration        time.Duration `json:"duration"`
}
```
