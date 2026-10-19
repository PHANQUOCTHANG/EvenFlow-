package domain

import (
	"strings"
	"time"
)

// Provider dai dien cho nha cung cap cong thanh toan.
type Provider string

const (
	ProviderSandbox Provider = "sandbox"
	ProviderVNPay   Provider = "vnpay"
	ProviderMoMo    Provider = "momo"
	ProviderStripe  Provider = "stripe"
)

// NormalizeProvider chuan hoa chuoi ten nha cung cap.
func NormalizeProvider(s string) Provider {
	return Provider(strings.ToLower(strings.TrimSpace(s)))
}

// IsValidProvider kiem tra xem provider co hop le khong.
func IsValidProvider(p Provider) bool {
	switch p {
	case ProviderSandbox, ProviderVNPay, ProviderMoMo, ProviderStripe:
		return true
	default:
		return false
	}
}

// PaymentStatus dai dien cho trang thai ban ghi thanh toan trong payments table.
type PaymentStatus string

const (
	StatusPending   PaymentStatus = "PENDING"
	StatusSucceeded PaymentStatus = "SUCCEEDED"
	StatusFailed    PaymentStatus = "FAILED"
	StatusRefunded  PaymentStatus = "REFUNDED"
)

// IsTerminal kiem tra xem trang thai thanh toan da dung o ket cuc cuoi cung chua.
func (s PaymentStatus) IsTerminal() bool {
	return s == StatusSucceeded || s == StatusFailed || s == StatusRefunded
}

// Payment dai dien cho thuc the giao dich thanh toan (anh xa vao bang payments).
type Payment struct {
	ID            string         `json:"id"`
	OrderID       string         `json:"order_id"`
	Provider      Provider       `json:"provider"`
	ProviderTxnID string         `json:"provider_txn_id"`
	AmountCents   int64          `json:"amount_cents"`
	Currency      string         `json:"currency"`
	Status        PaymentStatus  `json:"status"`
	RawPayload    map[string]any `json:"raw_payload,omitempty"`
	CreatedAt     time.Time      `json:"created_at"`
}

// CanBeProcessed kiem tra xem payment co the tiep tuc xu ly duoc khong.
func (p *Payment) CanBeProcessed() bool {
	return p.Status == StatusPending
}

// OrderView bieu dien thong tin don hang ma use case thanh toan can doc.
type OrderView struct {
	ID         string    `json:"id"`
	EventID    string    `json:"event_id"`
	IdentityID string    `json:"identity_id"`
	Status     string    `json:"status"`
	TotalCents int64     `json:"total_cents"`
	Currency   string    `json:"currency"`
	ExpiresAt  time.Time `json:"expires_at"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at,omitempty"`
}

// IsHeld kiem tra don hang co dang o trang thai giu ve khong.
func (o *OrderView) IsHeld() bool {
	return o.Status == "HELD"
}

// IsExpired kiem tra xem thoi diem giu ghe da troi qua chua (BR-O2).
func (o *OrderView) IsExpired(now time.Time) bool {
	return !now.Before(o.ExpiresAt)
}

// IsRefundable kiem tra don hang co o trang thai co the hoan tien khong (PAID, ISSUED, REFUNDING).
func (o *OrderView) IsRefundable() bool {
	return o.Status == "PAID" || o.Status == "ISSUED" || o.Status == "REFUNDING"
}

// PaymentMethodInfo mo ta thong tin hien thi cua mot phuong thuc thanh toan.
type PaymentMethodInfo struct {
	Code        Provider `json:"code"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Enabled     bool     `json:"enabled"`
	Icon        string   `json:"icon"`
}

// MaskPII che giau thong tin nhay cam (email, phone) de bao dam an toan khi log (Quy tac 4).
func MaskPII(val string) string {
	val = strings.TrimSpace(val)
	if val == "" {
		return ""
	}

	// Email masking: user@domain.com -> u***r@domain.com
	if atIdx := strings.Index(val, "@"); atIdx > 0 {
		name := val[:atIdx]
		domainPart := val[atIdx:]
		if len(name) <= 2 {
			return name[:1] + "***" + domainPart
		}
		return name[:1] + strings.Repeat("*", len(name)-2) + name[len(name)-1:] + domainPart
	}

	// Phone masking: 0912345678 -> 091****678
	if len(val) >= 7 {
		prefix := val[:3]
		suffix := val[len(val)-3:]
		return prefix + strings.Repeat("*", len(val)-6) + suffix
	}

	return "***"
}

// WebhookProcessResult dai dien cho ket qua sau khi xu ly webhook.
type WebhookProcessResult struct {
	Success       bool          `json:"success"`
	AlreadyDone   bool          `json:"already_done"`
	StoredInInbox bool          `json:"stored_in_inbox"`
	OrderID       string        `json:"order_id"`
	PaymentID     string        `json:"payment_id"`
	ProviderTxnID string        `json:"provider_txn_id"`
	Status        PaymentStatus `json:"status"`
	Message       string        `json:"message"`
}

// PaymentInboxItem dai dien cho ban ghi trong bang payment_inbox (BR-O6).
type PaymentInboxItem struct {
	ID            int64          `json:"id"`
	Provider      Provider       `json:"provider"`
	ProviderTxnID string         `json:"provider_txn_id"`
	Payload       map[string]any `json:"payload"`
	ReceivedAt    time.Time      `json:"received_at"`
	ProcessedAt   *time.Time     `json:"processed_at,omitempty"`
	Attempts      int            `json:"attempts"`
}

// IsProcessed kiem tra xem ban ghi inbox da duoc xu ly chua.
func (item *PaymentInboxItem) IsProcessed() bool {
	return item.ProcessedAt != nil
}

// IsStale kiem tra xem ban ghi inbox chua xu ly co vuot qua nguong thoi gian cho phep khong.
func (item *PaymentInboxItem) IsStale(now time.Time, threshold time.Duration) bool {
	if item.IsProcessed() {
		return false
	}
	return now.Sub(item.ReceivedAt) > threshold
}

// ReplayInboxResult tong hop ket qua sau moi lan chay job replay payment_inbox.
type ReplayInboxResult struct {
	TotalScanned  int `json:"total_scanned"`
	Succeeded     int `json:"succeeded"`
	AlreadyDone   int `json:"already_done"`
	StillPending  int `json:"still_pending"`
	Failed        int `json:"failed"`
	StaleDetected int `json:"stale_detected"`
}

// StaleInboxRecord chua thong tin ban ghi inbox bi ton dong qua lau can canh bao.
type StaleInboxRecord struct {
	ID            int64         `json:"id"`
	Provider      Provider      `json:"provider"`
	ProviderTxnID string        `json:"provider_txn_id"`
	ReceivedAt    time.Time     `json:"received_at"`
	Age           time.Duration `json:"age"`
	Attempts      int           `json:"attempts"`
}

// ProcessWebhookParams chua toan bo thong tin can thiet de cap nhat giao dich atomic trong DB.
type ProcessWebhookParams struct {
	Provider      Provider       `json:"provider"`
	ProviderTxnID string         `json:"provider_txn_id"`
	OrderID       string         `json:"order_id"`
	PaymentID     string         `json:"payment_id"`
	AmountCents   int64          `json:"amount_cents"`
	Currency      string         `json:"currency"`
	Status        PaymentStatus  `json:"status"`
	RawPayload    map[string]any `json:"raw_payload,omitempty"`
	TraceID       string         `json:"trace_id"`
	Timestamp     time.Time      `json:"timestamp"`
}

// OutboxOrderPaidPayload la du lieu su kien phat vao bang outbox_events cho routing_key order.paid.
type OutboxOrderPaidPayload struct {
	OrderID       string    `json:"order_id"`
	EventID       string    `json:"event_id"`
	IdentityID    string    `json:"identity_id"`
	PaymentID     string    `json:"payment_id"`
	Provider      string    `json:"provider"`
	ProviderTxnID string    `json:"provider_txn_id"`
	AmountCents   int64     `json:"amount_cents"`
	Currency      string    `json:"currency"`
	PaidAt        time.Time `json:"paid_at"`
}

// StaleOrderInfo dai dien cho mot don hang dang o trang thai PAYMENT_PENDING can duoc doi soat.
type StaleOrderInfo struct {
	OrderID       string    `json:"order_id"`
	EventID       string    `json:"event_id"`
	IdentityID    string    `json:"identity_id"`
	Status        string    `json:"status"`
	TotalCents    int64     `json:"total_cents"`
	Currency      string    `json:"currency"`
	ExpiresAt     time.Time `json:"expires_at"`
	UpdatedAt     time.Time `json:"updated_at"`
	CreatedAt     time.Time `json:"created_at"`
	PaymentID     string    `json:"payment_id,omitempty"`
	Provider      Provider  `json:"provider,omitempty"`
	ProviderTxnID string    `json:"provider_txn_id,omitempty"`
}

// PaymentQueryRequest la tham so truyen toi cong thanh toan de tra cuu chu dong trang thai giao dich.
type PaymentQueryRequest struct {
	OrderID       string `json:"order_id"`
	PaymentID     string `json:"payment_id,omitempty"`
	ProviderTxnID string `json:"provider_txn_id,omitempty"`
}

// PaymentQueryResult dai dien cho ket qua tra ve tu cong thanh toan khi tra cuu chu dong.
type PaymentQueryResult struct {
	ProviderTxnID string         `json:"provider_txn_id"`
	Status        PaymentStatus  `json:"status"` // SUCCEEDED, FAILED, PENDING
	AmountCents   int64          `json:"amount_cents"`
	RawResponse   map[string]any `json:"raw_response,omitempty"`
	Message       string         `json:"message"`
}

// CancelOrderParams chua tham so de huy don va giai phong kho ve idempotent (BR-O7).
type CancelOrderParams struct {
	OrderID   string `json:"order_id"`
	TraceID   string `json:"trace_id"`
	Reason    string `json:"reason"`
	PaymentID string `json:"payment_id,omitempty"`
}

// CancelOrderResult chua ket qua sau khi huy don va giai phong ghe.
type CancelOrderResult struct {
	OrderID         string `json:"order_id"`
	AlreadyDone     bool   `json:"already_done"`
	HoldsReleased   int    `json:"holds_released"`
	TicketsReturned int    `json:"tickets_returned"`
	Status          string `json:"status"`
	Message         string `json:"message"`
}

// OutboxHoldReleasedPayload la du lieu su kien phat vao bang outbox_events cho routing_key hold.released.
type OutboxHoldReleasedPayload struct {
	OrderID    string    `json:"order_id"`
	EventID    string    `json:"event_id"`
	Reason     string    `json:"reason"`
	ReleasedAt time.Time `json:"released_at"`
}

// ReconciliationResult tong hop ket qua sau moi dot chay job doi soat don treo.
type ReconciliationResult struct {
	TotalScanned    int           `json:"total_scanned"`
	HealedPaid      int           `json:"healed_paid"`
	HealedCancelled int           `json:"healed_cancelled"`
	StillPending    int           `json:"still_pending"`
	Skipped         int           `json:"skipped"`
	Errors          int           `json:"errors"`
	Duration        time.Duration `json:"duration"`
}

// PaymentRefundRequest chua toan bo tham so de yeu cau hoan tien toi cong thanh toan (EV-154 / EVF-74).
type PaymentRefundRequest struct {
	PaymentID     string `json:"payment_id"`
	OrderID       string `json:"order_id"`
	ProviderTxnID string `json:"provider_txn_id"`
	AmountCents   int64  `json:"amount_cents"`
	Currency      string `json:"currency"`
	Reason        string `json:"reason"`
	RefundID      string `json:"refund_id"`
}

// PaymentRefundResult la ket qua tra ve tu cong thanh toan sau khi thuc hien hoan tien.
type PaymentRefundResult struct {
	RefundTxnID string         `json:"refund_txn_id"`
	Status      PaymentStatus  `json:"status"` // REFUNDED, FAILED, PENDING
	AmountCents int64          `json:"amount_cents"`
	RawResponse map[string]any `json:"raw_response,omitempty"`
	Message     string         `json:"message"`
}

// RefundOrderInfo bieu dien don hang can duoc hoan tien khi su kien bi huy.
type RefundOrderInfo struct {
	OrderID       string    `json:"order_id"`
	EventID       string    `json:"event_id"`
	IdentityID    string    `json:"identity_id"`
	Status        string    `json:"status"`
	TotalCents    int64     `json:"total_cents"`
	Currency      string    `json:"currency"`
	PaymentID     string    `json:"payment_id,omitempty"`
	Provider      Provider  `json:"provider,omitempty"`
	ProviderTxnID string    `json:"provider_txn_id,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at,omitempty"`
}

// ProcessRefundParams chua thong tin can thiet de cap nhat hoan tien atomic trong DB (Quy tac 1).
type ProcessRefundParams struct {
	OrderID     string    `json:"order_id"`
	PaymentID   string    `json:"payment_id"`
	Provider    Provider  `json:"provider"`
	RefundTxnID string    `json:"refund_txn_id"`
	AmountCents int64     `json:"amount_cents"`
	Reason      string    `json:"reason"`
	TraceID     string    `json:"trace_id"`
	Timestamp   time.Time `json:"timestamp"`
}

// ProcessRefundResult chua ket qua cap nhat sau khi hoan tien trong DB.
type ProcessRefundResult struct {
	OrderID         string `json:"order_id"`
	PaymentID       string `json:"payment_id"`
	AlreadyDone     bool   `json:"already_done"`
	TicketsRefunded int    `json:"tickets_refunded"`
	Status          string `json:"status"`
	Message         string `json:"message"`
}

// OutboxPaymentRefundedPayload la du lieu su kien phat vao bang outbox_events cho routing_key payment.refunded.
type OutboxPaymentRefundedPayload struct {
	OrderID     string    `json:"order_id"`
	EventID     string    `json:"event_id"`
	IdentityID  string    `json:"identity_id"`
	PaymentID   string    `json:"payment_id"`
	Provider    string    `json:"provider"`
	RefundTxnID string    `json:"refund_txn_id"`
	AmountCents int64     `json:"amount_cents"`
	Currency    string    `json:"currency"`
	Reason      string    `json:"reason"`
	RefundedAt  time.Time `json:"refunded_at"`
}

// RefundBatchSummary tong hop ket qua sau khi hoan tien theo su kien.
type RefundBatchSummary struct {
	EventID       string        `json:"event_id"`
	TotalOrders   int           `json:"total_orders"`
	RefundedCount int           `json:"refunded_count"`
	FailedCount   int           `json:"failed_count"`
	SkippedCount  int           `json:"skipped_count"`
	TotalAmount   int64         `json:"total_amount"`
	Duration      time.Duration `json:"duration"`
}

// EventCancelledPayload la du lieu nhan duoc khi su kien bi huy.
type EventCancelledPayload struct {
	EventID     string    `json:"event_id"`
	CancelledAt time.Time `json:"cancelled_at"`
	Reason      string    `json:"reason,omitempty"`
}
