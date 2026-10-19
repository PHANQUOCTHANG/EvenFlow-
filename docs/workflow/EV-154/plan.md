# EV-154 (EVF-74) — Kế hoạch thực thi (Plan)

| Thuộc tính | Chi tiết |
|---|---|
| Task | **EV-154** (Mã backlog: **EVF-74**) |
| Tiêu đề | Luồng hoàn tiền khi sự kiện bị hủy (Refund flow when event is cancelled) |
| Phương pháp | **Disciplined Vibe Coding (6-Phase Vibe Loop)** |
| Kiến trúc | **Hexagonal Architecture (Domain -> Port -> App -> Adapter)** |

---

## 1. Thứ tự thực hiện

| Bước | Tầng | Nội dung thực hiện | Lý do & Ràng buộc |
|---|---|---|---|
| **1** | **Domain** (`internal/domain`) | Bổ sung các entity & value object: `PaymentRefundRequest`, `PaymentRefundResult`, `RefundOrderInfo`, `ProcessRefundParams`, `ProcessRefundResult`, `RefundBatchSummary`, `OutboxPaymentRefundedPayload`. Viết unit test domain thuần túy. | Domain độc lập 100%, 0 external dependencies (không DB, không HTTP). |
| **2** | **Port** (`internal/port`) | Cập nhật `PaymentGateway` thêm hàm `RefundPayment`. Cập nhật `PaymentRepository` thêm các hàm: `GetPaidOrdersByEvent`, `MarkOrderRefundingTx`, `ProcessRefundSuccessTx`. | Hợp đồng giao tiếp trừu tượng phục vụ usecase và adapters. |
| **3** | **Gateway Adapters** (`internal/adapter/gateway`) | Hiện thực `RefundPayment` cho cả 4 cổng: `SandboxGateway`, `VNPayGateway`, `MoMoGateway`, `StripeGateway`. Viết unit tests cho từng adapter. | Khả năng tương tác với cổng thanh toán để hoàn tiền thực tế. |
| **4** | **Postgres & Memory Repo** (`internal/adapter/postgres`) | Hiện thực các hàm truy vấn đơn cần hoàn tiền, đánh dấu `REFUNDING`, và cập nhật atomic `orders`, `payments`, `tickets`, `outbox_events` trong cùng một transaction (Quy tắc 1). Viết unit tests. | Bảo đảm tính nhất quán dữ liệu và chống hoàn trùng (idempotency). |
| **5** | **App Use Case** (`internal/app`) | Hiện thực `RefundCancelledEventUseCase`: Quét đơn theo lô -> Đánh dấu `REFUNDING` -> Gọi `RefundPayment` ngoài transaction (Quy tắc 2) -> Cập nhật DB atomic sang `REFUNDED` (Quy tắc 1) -> Trả về báo cáo tổng hợp. Viết unit tests toàn diện (TDD). | Bộ não nghiệp vụ điều phối luồng hoàn tiền sự kiện bị hủy. |
| **6** | **HTTP & Server Wiring** (`internal/adapter/http`, `cmd/server`) | Bổ sung endpoint `POST /v1/events/{eventID}/refund` (áp dụng middleware `Idempotency-Key` theo BR-O5). Đăng ký vào HTTP handler và `main.go`. | Cung cấp giao diện kích hoạt hoàn tiền cho Ops và hệ thống. |
| **7** | **Event Integration** (`services/event`) | Đảm bảo use case `Cancel` trong `services/event/internal/app/transition_state.go` phát sự kiện outbox `event.cancelled` qua `UpdateWithOutbox`. | Đồng bộ vòng đời sự kiện với luồng thanh toán. |
| **8** | **Chạy Gates & Review** | Chạy toàn bộ test suites (`go test -v -race ./...`), kiểm tra 100% pass, đo coverage, thu thập terminal output vào `evidence.md`, thực hiện review đối kháng vào `code-review.md`. | Bảo đảm chất lượng theo quy trình Vibe Coding chuẩn hóa. |

---

## 2. Danh sách File thay đổi

### Thêm mới
- `services/payment/internal/app/refund_event.go`
- `services/payment/internal/app/refund_event_test.go`
- `docs/workflow/EV-154/baseline.md`
- `docs/workflow/EV-154/spec.md`
- `docs/workflow/EV-154/plan.md`
- `docs/workflow/EV-154/traceability.md`
- `docs/workflow/EV-154/evidence.md`
- `docs/workflow/EV-154/code-review.md`

### Sửa đổi
- `services/payment/internal/domain/payment.go`
- `services/payment/internal/domain/payment_test.go`
- `services/payment/internal/domain/errors.go`
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
- `services/payment/internal/adapter/postgres/repository.go`
- `services/payment/internal/adapter/postgres/memory_repository.go`
- `services/payment/internal/adapter/postgres/repository_test.go`
- `services/payment/internal/adapter/http/handler.go`
- `services/payment/internal/adapter/http/handler_test.go`
- `services/payment/cmd/server/main.go`
- `services/event/internal/app/transition_state.go`
- `services/event/internal/app/transition_state_test.go`

### TUYỆT ĐỐI CẤM SỬA
- `services/ticketing/internal/domain/purchase_limit.go` (Quy tắc hạn mức mua BR-O4)
- `migrations/0001_core.sql` (Schema gốc đã có đầy đủ enum `REFUNDING`, `REFUNDED` cho cả `orders`, `payments`, `tickets`)
- Các test case baseline hiện có (không được sửa để che giấu lỗi hồi quy)

---

## 3. Hợp đồng API / Type Signatures

### Domain Types
```go
type PaymentRefundRequest struct {
    PaymentID     string `json:"payment_id"`
    OrderID       string `json:"order_id"`
    ProviderTxnID string `json:"provider_txn_id"`
    AmountCents   int64  `json:"amount_cents"`
    Currency      string `json:"currency"`
    Reason        string `json:"reason"`
    RefundID      string `json:"refund_id"`
}

type PaymentRefundResult struct {
    RefundTxnID string         `json:"refund_txn_id"`
    Status      PaymentStatus  `json:"status"` // REFUNDED, FAILED, PENDING
    RawResponse map[string]any `json:"raw_response,omitempty"`
    Message     string         `json:"message"`
}

type RefundOrderInfo struct {
    OrderID       string   `json:"order_id"`
    EventID       string   `json:"event_id"`
    IdentityID    string   `json:"identity_id"`
    Status        string   `json:"status"`
    TotalCents    int64    `json:"total_cents"`
    Currency      string   `json:"currency"`
    PaymentID     string   `json:"payment_id"`
    Provider      Provider `json:"provider"`
    ProviderTxnID string   `json:"provider_txn_id"`
}

type ProcessRefundParams struct {
    OrderID     string `json:"order_id"`
    PaymentID   string `json:"payment_id"`
    Provider    string `json:"provider"`
    RefundTxnID string `json:"refund_txn_id"`
    AmountCents int64  `json:"amount_cents"`
    Reason      string `json:"reason"`
    TraceID     string `json:"trace_id"`
}

type ProcessRefundResult struct {
    OrderID         string `json:"order_id"`
    PaymentID       string `json:"payment_id"`
    AlreadyDone     bool   `json:"already_done"`
    TicketsRefunded int    `json:"tickets_refunded"`
    Status          string `json:"status"`
    Message         string `json:"message"`
}
```

### Port Signatures
```go
// PaymentGateway
RefundPayment(ctx context.Context, req domain.PaymentRefundRequest) (domain.PaymentRefundResult, error)

// PaymentRepository
GetPaidOrdersByEvent(ctx context.Context, eventID string, limit int) ([]domain.RefundOrderInfo, error)
MarkOrderRefundingTx(ctx context.Context, orderID string) error
ProcessRefundSuccessTx(ctx context.Context, params domain.ProcessRefundParams) (domain.ProcessRefundResult, error)
```
