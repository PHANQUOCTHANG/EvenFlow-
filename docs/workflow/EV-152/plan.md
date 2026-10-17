# EV-152 — Kế hoạch thực thi (Plan)

## 1. Thứ tự thực hiện

| Bước | Nội dung | Lý do / Ràng buộc |
|---|---|---|
| **1** | Mở rộng Tầng Domain (`internal/domain`) | Thêm struct `PaymentInboxItem`, `ReplayInboxResult`, `StaleInboxRecord`, cập nhật `WebhookProcessResult`. Viết unit test domain. Tuyệt đối không import thư viện ngoài. |
| **2** | Mở rộng Tầng Port (`internal/port/repository.go`) | Khai báo các interface thao tác với `payment_inbox`: `SavePaymentInbox`, `GetPendingPaymentInbox`, `MarkPaymentInboxProcessed`, `IncrementPaymentInboxAttempts`, `GetStalePaymentInbox`. |
| **3** | Hiện thực Tầng Adapter Persistence (`internal/adapter/postgres`) | Hiện thực các hàm SQL chuẩn trong `Repository` (dùng `pgxpool`, `ON CONFLICT DO NOTHING`) và trong `MemoryRepository` cho local mock/test. Viết unit test kiểm thử persistence. |
| **4** | Tích hợp xử lý đơn chưa tồn tại vào `ProcessWebhookUseCase` | Khi gặp `ErrOrderNotFound`, use case tự động gọi `SavePaymentInbox` và trả về kết quả thành công với cờ `StoredInInbox: true`. Viết unit test cho kịch bản AC-1 & AC-2. |
| **5** | Hiện thực Use Case `ReplayPaymentInboxUseCase` (`internal/app/replay_inbox.go`) | Quét bản ghi pending, tăng attempts, thử chuyển đơn sang `PAID` (atomic tx), mark processed khi thành công, phát hiện & cảnh báo bản ghi tồn > 15 phút. Viết unit test chi tiết cho AC-3, AC-4, AC-5, AC-6. |
| **6** | Cập nhật HTTP Handler & Server Entrypoint | Cập nhật `handleWebhook` trả về JSON `stored_in_inbox: true`, đăng ký route và khởi động background replay ticker trong `cmd/server/main.go`. |
| **7** | Chạy Verification Gate & Thu thập bằng chứng (`evidence.md`) | Chạy toàn bộ test suites (`go test -v -cover ./services/payment/...`), linter, typecheck, thu thập terminal logs. |
| **8** | AI Review Đối kháng (`code-review.md`) & Hoàn tất Commit | Review đối chiếu 4 quy tắc bất biến và hoàn thiện PR / Conventional Commits. |

---

## 2. Danh sách File thay đổi

### Thêm mới
- `services/payment/internal/app/replay_inbox.go`: Use case quét và replay các bản ghi inbox định kỳ + cảnh báo stale inbox.
- `services/payment/internal/app/replay_inbox_test.go`: Unit test và concurrency test cho nghiệp vụ replay.
- `docs/workflow/EV-152/baseline.md`
- `docs/workflow/EV-152/spec.md`
- `docs/workflow/EV-152/plan.md`
- `docs/workflow/EV-152/traceability.md`
- `docs/workflow/EV-152/evidence.md`
- `docs/workflow/EV-152/code-review.md`

### Sửa đổi
- `services/payment/internal/domain/payment.go`: Thêm struct `PaymentInboxItem`, `ReplayInboxResult`, `StaleInboxRecord`, trường `StoredInInbox` trong `WebhookProcessResult`.
- `services/payment/internal/domain/payment_test.go`: Test struct và hàm domain.
- `services/payment/internal/port/repository.go`: Bổ sung methods inbox vào `PaymentRepository`.
- `services/payment/internal/adapter/postgres/repository.go`: Truy vấn SQL với bảng `payment_inbox`.
- `services/payment/internal/adapter/postgres/memory_repository.go`: Lưu trữ in-memory `payment_inbox` cho test/local.
- `services/payment/internal/adapter/postgres/memory_repository_test.go`: Test các thao tác inbox in-memory.
- `services/payment/internal/app/process_webhook.go`: Xử lý lưu inbox khi đơn chưa tồn tại.
- `services/payment/internal/app/process_webhook_test.go`: Test lưu inbox khi `ErrOrderNotFound`.
- `services/payment/internal/adapter/http/handler.go`: Trả về thông điệp `stored_in_inbox` cho client.
- `services/payment/internal/adapter/http/handler_test.go`: Test route HTTP khi đơn chưa tồn tại.
- `services/payment/cmd/server/main.go`: Khởi tạo `ReplayPaymentInboxUseCase` và background runner.

### TUYỆT ĐỐI CẤM SỬA
- `migrations/0001_core.sql`: Bảng `payment_inbox` đã có sẵn đúng chuẩn (`id, provider, provider_txn_id, payload, received_at, processed_at, attempts, UNIQUE(provider, provider_txn_id)`).
- `go.mod`: Không bổ sung thư viện thứ ba không cần thiết.
- Các test baseline cũ: Không được hạ assertion hay làm gãy các test case đã pass.

---

## 3. Hợp đồng API / Type Signatures

### Domain Signatures
```go
// PaymentInboxItem dai dien cho ban ghi trong bang payment_inbox.
type PaymentInboxItem struct {
	ID            int64          `json:"id"`
	Provider      Provider       `json:"provider"`
	ProviderTxnID string         `json:"provider_txn_id"`
	Payload       map[string]any `json:"payload"`
	ReceivedAt    time.Time      `json:"received_at"`
	ProcessedAt   *time.Time     `json:"processed_at,omitempty"`
	Attempts      int            `json:"attempts"`
}

// ReplayInboxResult tong hop ket qua sau moi lan chay job replay.
type ReplayInboxResult struct {
	TotalScanned  int `json:"total_scanned"`
	Succeeded     int `json:"succeeded"`
	AlreadyDone   int `json:"already_done"`
	StillPending  int `json:"still_pending"`
	Failed        int `json:"failed"`
	StaleDetected int `json:"stale_detected"`
}

// StaleInboxRecord chua thong tin ban ghi inbox bi ton dong qua lau.
type StaleInboxRecord struct {
	ID            int64         `json:"id"`
	Provider      Provider      `json:"provider"`
	ProviderTxnID string        `json:"provider_txn_id"`
	ReceivedAt    time.Time     `json:"received_at"`
	Age           time.Duration `json:"age"`
	Attempts      int           `json:"attempts"`
}
```

### Port Signatures
```go
type PaymentRepository interface {
	// ... (cac methods hien co giu nguyen) ...

	// SavePaymentInbox luu webhook vao payment_inbox khi don chua ton tai (BR-O6).
	// Ap dung ON CONFLICT (provider, provider_txn_id) DO NOTHING de idempotent.
	SavePaymentInbox(ctx context.Context, item domain.PaymentInboxItem) (int64, error)

	// GetPendingPaymentInbox lay cac ban ghi chua processed (processed_at IS NULL) co attempts < maxAttempts.
	GetPendingPaymentInbox(ctx context.Context, limit int, maxAttempts int) ([]domain.PaymentInboxItem, error)

	// MarkPaymentInboxProcessed cap nhat processed_at = now() khi don da thanh toan hoac da xu ly xong.
	MarkPaymentInboxProcessed(ctx context.Context, id int64, processedAt time.Time) error

	// IncrementPaymentInboxAttempts tang so lan thu attempts sau moi luot replay chua thanh cong.
	IncrementPaymentInboxAttempts(ctx context.Context, id int64) error

	// GetStalePaymentInbox tim cac ban ghi chua xu ly co thoi gian ton vuot qua nguong cho phep.
	GetStalePaymentInbox(ctx context.Context, olderThan time.Duration, limit int) ([]domain.StaleInboxRecord, error)
}
```

### Replay Use Case Signature
```go
type ReplayPaymentInboxUseCase struct {
	repo           port.PaymentRepository
	clock          port.Clock
	log            *slog.Logger
	maxAttempts    int
	staleThreshold time.Duration
}

func (uc *ReplayPaymentInboxUseCase) Execute(ctx context.Context, limit int) (domain.ReplayInboxResult, error)
func (uc *ReplayPaymentInboxUseCase) CheckStaleRecords(ctx context.Context, limit int) ([]domain.StaleInboxRecord, error)
```
