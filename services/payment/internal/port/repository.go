package port

import (
	"context"
	"time"

	"github.com/eventflow/eventflow/services/payment/internal/domain"
)

// PaymentRepository dinh nghia cac thao tac luu tru du lieu don hang va thanh toan.
type PaymentRepository interface {
	// GetOrder doc thong tin don hang de kiem tra tinh hop le truoc khi thanh toan.
	GetOrder(ctx context.Context, orderID string) (domain.OrderView, error)

	// CreatePaymentAndMarkPending cap nhat trang thai don hang sang PAYMENT_PENDING
	// va ghi ban ghi payments voi trang thai PENDING trong CUNG MOT TRANSACTION (Quy tac bat bien 1).
	CreatePaymentAndMarkPending(ctx context.Context, payment domain.Payment) error

	// GetPayment doc thong tin thanh toan theo payment_id.
	GetPayment(ctx context.Context, paymentID string) (domain.Payment, error)

	// GetPaymentByOrder doc thong tin thanh toan gan nhat cua mot order_id.
	GetPaymentByOrder(ctx context.Context, orderID string) (domain.Payment, error)

	// ProcessPaymentSuccessTx cap nhat trang thai payments -> SUCCEEDED, orders -> PAID
	// va ghi nhan su kien vao outbox_events trong CUNG MOT TRANSACTION (Quy tac bat bien 1).
	// Neu giao dich da duoc xu ly truoc do (trung provider_txn_id hoac don da PAID),
	// ham tra ve AlreadyDone = true ma khong gay loi (Exactly-once / BR-O6).
	ProcessPaymentSuccessTx(ctx context.Context, params domain.ProcessWebhookParams) (domain.WebhookProcessResult, error)

	// ProcessPaymentFailedTx ghi nhan trang thai thanh toan that bai vao bang payments
	// trong CUNG MOT TRANSACTION, khong chuyen don sang PAID.
	ProcessPaymentFailedTx(ctx context.Context, params domain.ProcessWebhookParams) (domain.WebhookProcessResult, error)

	// SavePaymentInbox luu webhook chua khop don vao payment_inbox (BR-O6).
	// Neu ban ghi (provider, provider_txn_id) da ton tai, khong tao loi duplicate key (idempotent).
	SavePaymentInbox(ctx context.Context, item domain.PaymentInboxItem) (int64, error)

	// GetPendingPaymentInbox lay danh sach ban ghi chua xu ly (processed_at IS NULL) voi so lan thu < maxAttempts.
	GetPendingPaymentInbox(ctx context.Context, limit int, maxAttempts int) ([]domain.PaymentInboxItem, error)

	// MarkPaymentInboxProcessed danh dau ban ghi inbox da duoc xu ly thanh cong (processed_at = now()).
	MarkPaymentInboxProcessed(ctx context.Context, id int64, processedAt time.Time) error

	// IncrementPaymentInboxAttempts tang so lan thu attempts sau khi replay chua thanh cong.
	IncrementPaymentInboxAttempts(ctx context.Context, id int64) error

	// GetStalePaymentInbox tim cac ban ghi chua xu ly co thoi gian ton vuot qua nguong cho phep de canh bao.
	GetStalePaymentInbox(ctx context.Context, olderThan time.Duration, limit int) ([]domain.StaleInboxRecord, error)
}

// Clock ho tro inject thoi gian hien tai giup kiem thu chinh xac cac moc expires_at (BR-O2).
type Clock interface {
	Now() time.Time
}

// SystemClock la hien thuc Clock su dung gio he thong thuc te.
type SystemClock struct{}

func (SystemClock) Now() time.Time {
	return time.Now().UTC()
}

// IDGenerator sinh chuoi ID (vi du UUID v4) cho ban ghi Payment.
type IDGenerator interface {
	NewID() string
}
