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
}

// IsHeld kiem tra don hang co dang o trang thai giu ve khong.
func (o *OrderView) IsHeld() bool {
	return o.Status == "HELD"
}

// IsExpired kiem tra xem thoi diem giu ghe da troi qua chua (BR-O2).
func (o *OrderView) IsExpired(now time.Time) bool {
	return !now.Before(o.ExpiresAt)
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
