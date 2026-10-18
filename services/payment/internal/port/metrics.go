package port

import "time"

// ReconciliationMetrics dinh nghia cac phuong thuc ghi nhan metric doi soat va do tre webhook (AC-6).
type ReconciliationMetrics interface {
	// ObserveWebhookLag ghi nhan do tre tinh bang giay tu luc don duoc tao/pending toi khi doi soat chua thanh cong.
	ObserveWebhookLag(d time.Duration)

	// IncReconciliation tang so dem don hang da doi soat theo ket qua nghiep vu (healed_paid, healed_cancelled, still_pending, skipped, error).
	IncReconciliation(outcome string)

	// ObserveReconciliationDuration ghi nhan thoi gian thuc thi mot dot doi soat.
	ObserveReconciliationDuration(d time.Duration)
}
