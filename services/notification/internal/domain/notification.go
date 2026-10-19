package domain

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

var (
	emailRegex = regexp.MustCompile(`^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$`)
	phoneRegex = regexp.MustCompile(`^(\+84|0)[3|5|7|8|9][0-9]{8}$`)
)

// Channel la kenh truyen thong thong bao (Email hoac SMS).
type Channel string

const (
	ChannelEmail Channel = "email"
	ChannelSMS   Channel = "sms"
)

// IsValid kiem tra kenh truyen thong co hop le khong.
func (c Channel) IsValid() bool {
	return c == ChannelEmail || c == ChannelSMS
}

// NotificationStatus la trang thai cua mot ban ghi thong bao.
type NotificationStatus string

const (
	StatusPending NotificationStatus = "PENDING"
	StatusSent    NotificationStatus = "SENT"
	StatusFailed  NotificationStatus = "FAILED"
	StatusSkipped NotificationStatus = "SKIPPED"
)

// Notification dai dien cho mot ban ghi thong bao da duoc xu ly hoac luu tru.
type Notification struct {
	ID          string             `json:"id"`
	DedupKey    string             `json:"dedup_key"`
	EventType   string             `json:"event_type"`
	OrderID     string             `json:"order_id"`
	Recipient   string             `json:"recipient"`
	Channel     Channel            `json:"channel"`
	Subject     string             `json:"subject,omitempty"`
	Body        string             `json:"body,omitempty"`
	Status      NotificationStatus `json:"status"`
	ErrorReason string             `json:"error_reason,omitempty"`
	SentAt      *time.Time         `json:"sent_at,omitempty"`
	CreatedAt   time.Time          `json:"created_at"`
}

// EmailMessage chua noi dung email can gui.
type EmailMessage struct {
	To       string `json:"to"`
	Subject  string `json:"subject"`
	HTMLBody string `json:"html_body"`
	TextBody string `json:"text_body"`
}

// Validate kiem tra hop le cua email message.
func (m EmailMessage) Validate() error {
	m.To = strings.TrimSpace(m.To)
	if m.To == "" || !emailRegex.MatchString(m.To) {
		return ErrInvalidRecipient
	}
	if strings.TrimSpace(m.Subject) == "" || (strings.TrimSpace(m.HTMLBody) == "" && strings.TrimSpace(m.TextBody) == "") {
		return ErrEmptyContent
	}
	return nil
}

// SMSMessage chua noi dung tin nhan SMS can gui.
type SMSMessage struct {
	To          string `json:"to"`
	MessageBody string `json:"message_body"`
}

// Validate kiem tra hop le cua SMS message.
func (m SMSMessage) Validate() error {
	m.To = strings.TrimSpace(m.To)
	if m.To == "" || !phoneRegex.MatchString(m.To) {
		return ErrInvalidRecipient
	}
	if strings.TrimSpace(m.MessageBody) == "" {
		return ErrEmptyContent
	}
	return nil
}

// TicketDetail chua thong tin ve dien tu phat hanh cho khach.
type TicketDetail struct {
	ID             string `json:"id"`
	TicketTypeID   string `json:"ticket_type_id"`
	TicketTypeName string `json:"ticket_type_name,omitempty"`
	JTI            string `json:"jti"`
	QRSig          string `json:"qr_sig"`
	Status         string `json:"status"`
}

// OrderPaidEvent dai dien cho su kien thanh toan don hang thanh cong (order.paid).
type OrderPaidEvent struct {
	OrderID        string    `json:"order_id"`
	EventID        string    `json:"event_id"`
	EventTitle     string    `json:"event_title,omitempty"`
	IdentityID     string    `json:"identity_id"`
	AmountCents    int64     `json:"amount_cents"`
	Currency       string    `json:"currency"`
	Provider       string    `json:"provider"`
	ProviderTxnID  string    `json:"provider_txn_id"`
	RecipientEmail string    `json:"recipient_email"`
	RecipientPhone string    `json:"recipient_phone,omitempty"`
	PaidAt         time.Time `json:"paid_at"`
}

// Validate kiem tra su kien order.paid.
func (e OrderPaidEvent) Validate() error {
	if strings.TrimSpace(e.OrderID) == "" || strings.TrimSpace(e.IdentityID) == "" {
		return ErrInvalidEventPayload
	}
	if e.RecipientEmail == "" && e.RecipientPhone == "" {
		return ErrInvalidRecipient
	}
	return nil
}

// TicketIssuedEvent dai dien cho su kien phat hanh ve (ticketing.ticket.issued).
type TicketIssuedEvent struct {
	OrderID        string         `json:"order_id"`
	EventID        string         `json:"event_id"`
	EventTitle     string         `json:"event_title,omitempty"`
	IdentityID     string         `json:"identity_id"`
	TicketCount    int            `json:"ticket_count"`
	Tickets        []TicketDetail `json:"tickets"`
	RecipientEmail string         `json:"recipient_email"`
	RecipientPhone string         `json:"recipient_phone,omitempty"`
	IssuedAt       time.Time      `json:"issued_at"`
}

// Validate kiem tra su kien ticket.issued.
func (e TicketIssuedEvent) Validate() error {
	if strings.TrimSpace(e.OrderID) == "" || strings.TrimSpace(e.IdentityID) == "" {
		return ErrInvalidEventPayload
	}
	if e.RecipientEmail == "" && e.RecipientPhone == "" {
		return ErrInvalidRecipient
	}
	return nil
}

// PaymentRefundedEvent dai dien cho su kien hoan tien don hang (payment.refunded).
type PaymentRefundedEvent struct {
	OrderID        string    `json:"order_id"`
	EventID        string    `json:"event_id"`
	EventTitle     string    `json:"event_title,omitempty"`
	IdentityID     string    `json:"identity_id"`
	AmountCents    int64     `json:"amount_cents"`
	Currency       string    `json:"currency"`
	Reason         string    `json:"reason"`
	RecipientEmail string    `json:"recipient_email"`
	RecipientPhone string    `json:"recipient_phone,omitempty"`
	RefundedAt     time.Time `json:"refunded_at"`
}

// Validate kiem tra su kien payment.refunded.
func (e PaymentRefundedEvent) Validate() error {
	if strings.TrimSpace(e.OrderID) == "" || strings.TrimSpace(e.IdentityID) == "" {
		return ErrInvalidEventPayload
	}
	if e.RecipientEmail == "" && e.RecipientPhone == "" {
		return ErrInvalidRecipient
	}
	return nil
}

// MakeDedupKey tao khoa duy nhat de chong gui trung lap.
// Mau: <eventType>:<orderID>:<channel>
func MakeDedupKey(eventType, orderID string, channel Channel) string {
	return fmt.Sprintf("%s:%s:%s", strings.TrimSpace(eventType), strings.TrimSpace(orderID), channel)
}

// FormatCurrencyVND dinh dang so tien cents sang chuoi hien thi tieng Viet (VND).
// Trong he thong EvenFlow, amount_cents luu dong xu/cent.
// Neu Currency la VND, so tien tuong ung la amount_cents (hoac chia 1 neu la don vi dong).
func FormatCurrencyVND(amount int64) string {
	// Dinh dang co dau cham phan cach hang nghin: 500000 -> "500.000 VND"
	sign := ""
	if amount < 0 {
		sign = "-"
		amount = -amount
	}
	s := fmt.Sprintf("%d", amount)
	n := len(s)
	if n <= 3 {
		return fmt.Sprintf("%s%s VND", sign, s)
	}

	var parts []string
	remainder := n % 3
	if remainder > 0 {
		parts = append(parts, s[:remainder])
	}
	for i := remainder; i < n; i += 3 {
		parts = append(parts, s[i:i+3])
	}
	return fmt.Sprintf("%s%s VND", sign, strings.Join(parts, "."))
}
