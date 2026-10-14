package domain

import (
	"strings"
	"time"

	"github.com/google/uuid"
)

type EventStatus string

const (
	StatusDraft         EventStatus = "DRAFT"
	StatusPendingReview EventStatus = "PENDING_REVIEW"
	StatusScheduled     EventStatus = "SCHEDULED"
	StatusOnSale        EventStatus = "ON_SALE"
	StatusSoldOut       EventStatus = "SOLD_OUT"
	StatusClosed        EventStatus = "CLOSED"
	StatusCompleted     EventStatus = "COMPLETED"
	StatusRejected      EventStatus = "REJECTED"
	StatusCancelled     EventStatus = "CANCELLED"
)

// Event đại diện cho một sự kiện trong hệ thống EvenFlow.
type Event struct {
	ID              uuid.UUID    `json:"id"`
	OrganizerID     uuid.UUID    `json:"organizer_id"`
	Slug            string       `json:"slug"`
	Title           string       `json:"title"`
	Description     string       `json:"description"`
	Venue           string       `json:"venue"`
	StartsAt        time.Time    `json:"starts_at"`
	SaleStartAt     time.Time    `json:"sale_start_at"`
	SaleEndAt       *time.Time   `json:"sale_end_at,omitempty"`
	Status          EventStatus  `json:"status"`
	MaxPerIdentity  int          `json:"max_per_identity"`
	MaxPerOrder     int          `json:"max_per_order"`
	HoldTTLSeconds  int          `json:"hold_ttl_seconds"`
	CreatedAt       time.Time    `json:"created_at"`
	UpdatedAt       time.Time    `json:"updated_at"`
	Tiers           []TicketTier `json:"tiers,omitempty"`
}

// NewEvent tạo một sự kiện mới ở trạng thái DRAFT.
func NewEvent(
	organizerID uuid.UUID,
	slug, title, description, venue string,
	startsAt, saleStartAt time.Time,
	saleEndAt *time.Time,
) (*Event, error) {
	trimmedTitle := strings.TrimSpace(title)
	if trimmedTitle == "" {
		return nil, ErrInvalidTitle
	}

	trimmedSlug := strings.TrimSpace(slug)
	if trimmedSlug == "" {
		trimmedSlug = strings.ToLower(strings.ReplaceAll(trimmedTitle, " ", "-"))
	}

	if !startsAt.After(saleStartAt) {
		return nil, ErrInvalidEventDates
	}

	now := time.Now().UTC()
	return &Event{
		ID:             uuid.New(),
		OrganizerID:    organizerID,
		Slug:           trimmedSlug,
		Title:          trimmedTitle,
		Description:    description,
		Venue:          venue,
		StartsAt:       startsAt,
		SaleStartAt:    saleStartAt,
		SaleEndAt:      saleEndAt,
		Status:         StatusDraft,
		MaxPerIdentity: 4,
		MaxPerOrder:    4,
		HoldTTLSeconds: 600, // 10 phút theo BR-O2
		CreatedAt:      now,
		UpdatedAt:      now,
		Tiers:          make([]TicketTier, 0),
	}, nil
}

// UpdateDetails cập nhật thông tin sự kiện (chỉ được phép khi DRAFT hoặc SCHEDULED).
func (e *Event) UpdateDetails(
	title, description, venue string,
	startsAt, saleStartAt time.Time,
	saleEndAt *time.Time,
) error {
	if e.Status != StatusDraft && e.Status != StatusScheduled && e.Status != StatusRejected {
		return ErrEventNotEditable
	}

	trimmedTitle := strings.TrimSpace(title)
	if trimmedTitle == "" {
		return ErrInvalidTitle
	}

	if !startsAt.After(saleStartAt) {
		return nil
	}

	e.Title = trimmedTitle
	e.Description = description
	e.Venue = venue
	e.StartsAt = startsAt
	e.SaleStartAt = saleStartAt
	e.SaleEndAt = saleEndAt
	e.UpdatedAt = time.Now().UTC()
	return nil
}

// AddTier thêm một hạng vé mới vào sự kiện.
func (e *Event) AddTier(tier TicketTier) error {
	if e.Status != StatusDraft && e.Status != StatusScheduled && e.Status != StatusRejected {
		return ErrEventNotEditable
	}

	for _, existing := range e.Tiers {
		if strings.EqualFold(existing.Name, tier.Name) {
			return ErrDuplicateTierName
		}
	}

	e.Tiers = append(e.Tiers, tier)
	e.UpdatedAt = time.Now().UTC()
	return nil
}

// UpdateTierQuota cập nhật số lượng vé của một hạng vé.
// BR-E3: Sau khi ON_SALE, CẤM giảm quota của hạng vé; chỉ được tăng.
func (e *Event) UpdateTierQuota(tierID uuid.UUID, newQuota int) error {
	if newQuota <= 0 {
		return ErrInvalidQuota
	}

	tierIndex := -1
	for i, t := range e.Tiers {
		if t.ID == tierID {
			tierIndex = i
			break
		}
	}

	if tierIndex == -1 {
		return ErrTierNotFound
	}

	currentQuota := e.Tiers[tierIndex].Quota

	// BR-E3: Kiểm tra khi ON_SALE
	if e.Status == StatusOnSale {
		if newQuota < currentQuota {
			return ErrCannotReduceQuotaOnSale
		}
	} else if e.Status == StatusSoldOut {
		// Nếu đang SOLD_OUT mà tăng quota -> tự động quay lại ON_SALE
		if newQuota > currentQuota {
			e.Status = StatusOnSale
		}
	} else if e.Status != StatusDraft && e.Status != StatusScheduled && e.Status != StatusRejected {
		return ErrEventNotEditable
	}

	e.Tiers[tierIndex].Quota = newQuota
	e.UpdatedAt = time.Now().UTC()
	return nil
}

// SubmitForReview nộp sự kiện để duyệt (DRAFT -> PENDING_REVIEW).
// BR-E1: Sự kiện chỉ chuyển PENDING_REVIEW khi đã có >= 1 hạng vé với quota > 0 và price >= 0.
func (e *Event) SubmitForReview() error {
	if e.Status != StatusDraft && e.Status != StatusRejected {
		return ErrInvalidStateTransition
	}

	hasValidTier := false
	for _, t := range e.Tiers {
		if t.Quota > 0 && t.PriceCents >= 0 {
			hasValidTier = true
			break
		}
	}

	if !hasValidTier {
		return ErrNoValidTicketTiers
	}

	e.Status = StatusPendingReview
	e.UpdatedAt = time.Now().UTC()
	return nil
}

// Approve duyệt sự kiện và chuyển sang SCHEDULED (EV-118).
// BR-E2: sale_start_at phải cách thời điểm duyệt >= 30 phút để kịp pre-warm hạ tầng.
func (e *Event) Approve(now time.Time) error {
	if e.Status != StatusPendingReview {
		return ErrInvalidStateTransition
	}

	// Kiểm tra BR-E2: T0 >= approval_time + 30 phút
	earliestAllowedSaleStart := now.Add(30 * time.Minute)
	if e.SaleStartAt.Before(earliestAllowedSaleStart) {
		return ErrSaleStartTimeTooEarly
	}

	e.Status = StatusScheduled
	e.UpdatedAt = time.Now().UTC()
	return nil
}

// ApproveOverride cho phép Moderator lật ngược quyết định REJECTED hoặc duyệt từ PENDING_REVIEW sang SCHEDULED (EV-121).
// BR-E2: sale_start_at phải cách thời điểm duyệt >= 30 phút để kịp pre-warm hạ tầng.
func (e *Event) ApproveOverride(now time.Time) error {
	if e.Status != StatusPendingReview && e.Status != StatusRejected {
		return ErrInvalidStateTransition
	}

	earliestAllowedSaleStart := now.Add(30 * time.Minute)
	if e.SaleStartAt.Before(earliestAllowedSaleStart) {
		return ErrSaleStartTimeTooEarly
	}

	e.Status = StatusScheduled
	e.UpdatedAt = time.Now().UTC()
	return nil
}

// Reject từ chối sự kiện (PENDING_REVIEW -> REJECTED).
func (e *Event) Reject() error {
	if e.Status != StatusPendingReview {
		return ErrInvalidStateTransition
	}

	e.Status = StatusRejected
	e.UpdatedAt = time.Now().UTC()
	return nil
}

// OpenSale kích hoạt mở bán sự kiện khi đến T0 (SCHEDULED -> ON_SALE).
func (e *Event) OpenSale(now time.Time) error {
	if e.Status != StatusScheduled {
		return ErrInvalidStateTransition
	}

	if now.Before(e.SaleStartAt) {
		return ErrInvalidStateTransition // Chưa đến giờ mở bán
	}

	e.Status = StatusOnSale
	e.UpdatedAt = time.Now().UTC()
	return nil
}

// MarkSoldOut đánh dấu sự kiện đã hết vé (ON_SALE -> SOLD_OUT).
func (e *Event) MarkSoldOut() error {
	if e.Status != StatusOnSale {
		return ErrInvalidStateTransition
	}

	e.Status = StatusSoldOut
	e.UpdatedAt = time.Now().UTC()
	return nil
}

// CloseSale đóng bán vé khi hết hạn bán (ON_SALE/SOLD_OUT -> CLOSED).
func (e *Event) CloseSale() error {
	if e.Status != StatusOnSale && e.Status != StatusSoldOut {
		return ErrInvalidStateTransition
	}

	e.Status = StatusClosed
	e.UpdatedAt = time.Now().UTC()
	return nil
}

// Complete kết thúc sự kiện sau khi diễn ra thành công (CLOSED/SOLD_OUT -> COMPLETED).
func (e *Event) Complete() error {
	if e.Status != StatusClosed && e.Status != StatusSoldOut {
		return ErrInvalidStateTransition
	}

	e.Status = StatusCompleted
	e.UpdatedAt = time.Now().UTC()
	return nil
}

// Cancel hủy sự kiện (hoàn tiền) từ bất kỳ trạng thái nào trước COMPLETED.
func (e *Event) Cancel() error {
	if e.Status == StatusCompleted || e.Status == StatusCancelled {
		return ErrInvalidStateTransition
	}

	e.Status = StatusCancelled
	e.UpdatedAt = time.Now().UTC()
	return nil
}
