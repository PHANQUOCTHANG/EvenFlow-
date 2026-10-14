package domain

import (
	"errors"
)

const (
	// DefaultBucketCount là số lượng bucket mặc định để phân mảnh tồn kho (BR-O1).
	DefaultBucketCount = 32
)

var (
	ErrInvalidBucketCount = errors.New("bucket count must be greater than 0")
	ErrNegativeQuota      = errors.New("quota cannot be negative")
)

// BucketInventory đại diện cho tồn kho của một bucket.
type BucketInventory struct {
	Bucket    int `json:"bucket"`
	Capacity  int `json:"capacity"`
	Available int `json:"available"`
}

// DistributeQuota chia đều quota của một hạng vé ra số lượng bucket (mặc định 32).
// Phần dư (quota % bucketCount) được dồn lần lượt vào các bucket đầu tiên.
// Bất biến (Invariants):
// 1. Tổng các bucket luôn bằng đúng quota ban đầu: sum(buckets) == quota.
// 2. Chênh lệch giữa bất kỳ hai bucket nào tối đa là 1 vé: max(buckets) - min(buckets) <= 1.
// 3. Không có bucket nào âm: bucket[i] >= 0 với mọi i.
func DistributeQuota(quota int, bucketCount int) ([]int, error) {
	if quota < 0 {
		return nil, ErrNegativeQuota
	}
	if bucketCount <= 0 {
		bucketCount = DefaultBucketCount
	}

	buckets := make([]int, bucketCount)
	if quota == 0 {
		return buckets, nil
	}

	base := quota / bucketCount
	rem := quota % bucketCount

	for i := 0; i < bucketCount; i++ {
		buckets[i] = base
		if i < rem {
			buckets[i]++
		}
	}

	return buckets, nil
}

// DistributeAdditionalQuota phân bổ thêm quota khi ban tổ chức tăng số lượng vé (BR-E3).
// Nhận vào danh sách các bucket hiện tại và số lượng vé muốn tăng thêm (deltaQuota).
// Bất biến: Tổng available mới = tổng available cũ + deltaQuota.
func DistributeAdditionalQuota(current []BucketInventory, deltaQuota int) ([]BucketInventory, error) {
	if deltaQuota <= 0 {
		return current, ErrQuantityNotAllowed
	}
	n := len(current)
	if n == 0 {
		return nil, ErrInvalidBucketCount
	}

	result := make([]BucketInventory, n)
	copy(result, current)

	base := deltaQuota / n
	rem := deltaQuota % n

	for i := 0; i < n; i++ {
		add := base
		if i < rem {
			add++
		}
		result[i].Capacity += add
		result[i].Available += add
	}

	return result, nil
}
