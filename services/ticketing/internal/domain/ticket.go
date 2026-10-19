package domain

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"
)

// TicketStatus la trang thai cua mot ve trong he thong.
type TicketStatus string

const (
	TicketStatusIssued    TicketStatus = "ISSUED"
	TicketStatusCheckedIn TicketStatus = "CHECKED_IN"
	TicketStatusVoid      TicketStatus = "VOID"
	TicketStatusRefunded  TicketStatus = "REFUNDED"
)

// IsValidTicketStatus kiem tra xem chuoi trang thai co hop le khong.
func IsValidTicketStatus(s TicketStatus) bool {
	switch s {
	case TicketStatusIssued, TicketStatusCheckedIn, TicketStatusVoid, TicketStatusRefunded:
		return true
	default:
		return false
	}
}

// Ticket dai dien cho mot ban ghi ve dien tu trong co so du lieu (bang tickets).
type Ticket struct {
	ID           string       `json:"id"`
	OrderID      string       `json:"order_id"`
	TicketTypeID string       `json:"ticket_type_id"`
	JTI          string       `json:"jti"`
	QRSig        string       `json:"qr_sig"`
	Status       TicketStatus `json:"status"`
	CheckedInAt  *time.Time   `json:"checked_in_at,omitempty"`
	CreatedAt    time.Time    `json:"created_at"`
}

// CanAdmit kiem tra xem ve co du dieu kien de duoc chap nhan vao cua khong (AC-4, AC-5).
func (t *Ticket) CanAdmit() error {
	switch t.Status {
	case TicketStatusIssued:
		return nil
	case TicketStatusCheckedIn:
		return ErrTicketAlreadyUsed
	case TicketStatusVoid:
		return ErrTicketVoid
	case TicketStatusRefunded:
		return ErrTicketRefunded
	default:
		return ErrInvalidTicketStatus
	}
}

// TicketWithQR la ve kem chuoi QR payload day du de hien thi / in an.
type TicketWithQR struct {
	Ticket
	QRPayload string `json:"qr_payload"`
}

// NewJTI tao mot dinh danh token duy nhat (JTI) cho ve chong sao chep va replay (BR-O8).
func NewJTI() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(fmt.Sprintf("domain: tao jti that bai: %v", err))
	}
	return hex.EncodeToString(b)
}
