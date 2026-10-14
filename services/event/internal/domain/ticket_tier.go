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

// DistributeQuota chia đều quota của một hạng vé ra số lượng bucket (mặc định 32).
// Phần dư (quota % bucketCount) được dồn lần lượt vào các bucket đầu tiên.
// Bất biến: Tổng kết quả luôn bằng đúng quota ban đầu (BR-O1).
func (t *TicketTier) DistributeQuota() []int {
	bucketCount := t.BucketCount
	if bucketCount <= 0 {
		bucketCount = 32
	}
	buckets := make([]int, bucketCount)
	if t.Quota <= 0 {
		return buckets
	}

	base := t.Quota / bucketCount
	rem := t.Quota % bucketCount

	for i := 0; i < bucketCount; i++ {
		buckets[i] = base
		if i < rem {
			buckets[i]++
		}
	}
	return buckets
}

// DistributeAdditionalQuota phân bổ thêm quota khi ban tổ chức tăng số lượng vé (BR-E3).
func (t *TicketTier) DistributeAdditionalQuota(deltaQuota int) []int {
	bucketCount := t.BucketCount
	if bucketCount <= 0 {
		bucketCount = 32
	}
	buckets := make([]int, bucketCount)
	if deltaQuota <= 0 {
		return buckets
	}

	base := deltaQuota / bucketCount
	rem := deltaQuota % bucketCount

	for i := 0; i < bucketCount; i++ {
		buckets[i] = base
		if i < rem {
			buckets[i]++
		}
	}
	return buckets
}

