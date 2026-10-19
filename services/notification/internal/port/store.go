package port

import (
	"context"
	"time"

	"github.com/eventflow/eventflow/services/notification/internal/domain"
)

// NotificationStore la kho luu tru phuc vu Idempotency/Deduplication va tra cuu lich su.
type NotificationStore interface {
	// CheckOrSetDedup kiem tra xem thong bao voi dedupKey da duoc xu ly hoac dang xu ly chua.
	// Neu da thanh cong (SENT) hoac dang PENDING, tra ve alreadyProcessed = true.
	// Neu chua, ghi nhan thong bao vao kho voi trang thai PENDING va tra ve alreadyProcessed = false.
	CheckOrSetDedup(ctx context.Context, dedupKey string, notif domain.Notification) (alreadyProcessed bool, err error)

	// MarkSent cap nhat trang thai thong bao thanh SENT kem thoi diem gui.
	MarkSent(ctx context.Context, dedupKey string, notifID string, sentAt time.Time) error

	// MarkFailed cap nhat trang thai thong bao thanh FAILED kem ly do loi.
	MarkFailed(ctx context.Context, dedupKey string, notifID string, reason string) error

	// GetHistoryByOrderID tra cuu toan bo lich su thong bao lien quan toi mot don hang.
	GetHistoryByOrderID(ctx context.Context, orderID string) ([]domain.Notification, error)
}
