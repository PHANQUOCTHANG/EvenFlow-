package domain

import (
	"strings"
	"time"

	"github.com/google/uuid"
)

// TicketTier đại diện cho một hạng vé thuộc một sự kiện.
type TicketTier struct {
	ID          uuid.UUID `json:"id"`
	EventID     uuid.UUID `json:"event_id"`
	Name        string    `json:"name"`
	PriceCents  int64     `json:"price_cents"`
	Currency    string    `json:"currency"`
	Quota       int       `json:"quota"`
	BucketCount int       `json:"bucket_count"`
	CreatedAt   time.Time `json:"created_at"`
}

// NewTicketTier khởi tạo một TicketTier mới và kiểm tra tính hợp lệ cơ bản.
func NewTicketTier(eventID uuid.UUID, name string, priceCents int64, quota int) (*TicketTier, error) {
	trimmedName := strings.TrimSpace(name)
	if trimmedName == "" {
		return nil, ErrDuplicateTierName // Tên không được rỗng
	}
	if priceCents < 0 {
		return nil, ErrInvalidPrice
	}
	if quota <= 0 {
		return nil, ErrInvalidQuota
	}

	return &TicketTier{
		ID:          uuid.New(),
		EventID:     eventID,
		Name:        trimmedName,
		PriceCents:  priceCents,
		Currency:    "VND",
		Quota:       quota,
		BucketCount: 32, // Phân mảnh 32 bucket theo kiến trúc chống oversell
		CreatedAt:   time.Now().UTC(),
	}, nil
}

// Update cập nhật thông tin hạng vé (tên, giá, quota).
func (t *TicketTier) Update(name string, priceCents int64, newQuota int) error {
	trimmedName := strings.TrimSpace(name)
	if trimmedName == "" {
		return ErrDuplicateTierName
	}
	if priceCents < 0 {
		return ErrInvalidPrice
	}
	if newQuota <= 0 {
		return ErrInvalidQuota
	}

	t.Name = trimmedName
	t.PriceCents = priceCents
	t.Quota = newQuota
	return nil
}
