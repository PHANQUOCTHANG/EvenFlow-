package domain_test

import (
	"testing"

	"github.com/eventflow/eventflow/services/ticketing/internal/domain"
)

func TestDistributeQuota_Invariants(t *testing.T) {
	testCases := []struct {
		name        string
		quota       int
		bucketCount int
	}{
		{"Zero quota", 0, 32},
		{"Quota less than buckets", 10, 32},
		{"Quota exact multiple", 32, 32},
		{"Quota 100 tickets into 32 buckets", 100, 32},
		{"Odd quota 33 tickets", 33, 32},
		{"Large quota 50,000 tickets", 50000, 32},
		{"Custom bucket count 16", 50, 16},
		{"Custom bucket count default fallback", 64, 0},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			buckets, err := domain.DistributeQuota(tc.quota, tc.bucketCount)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			expectedBuckets := tc.bucketCount
			if expectedBuckets <= 0 {
				expectedBuckets = domain.DefaultBucketCount
			}
			if len(buckets) != expectedBuckets {
				t.Fatalf("expected %d buckets, got %d", expectedBuckets, len(buckets))
			}

			sum := 0
			minVal := -1
			maxVal := -1

			for _, b := range buckets {
				if b < 0 {
					t.Errorf("bucket value cannot be negative: %d", b)
				}
				sum += b
				if minVal == -1 || b < minVal {
					minVal = b
				}
				if maxVal == -1 || b > maxVal {
					maxVal = b
				}
			}

			// Invariant 1: Tổng luôn đúng bằng quota ban đầu
			if sum != tc.quota {
				t.Errorf("expected total sum %d, got %d", tc.quota, sum)
			}

			// Invariant 2: Chênh lệch giữa max và min tối đa là 1 vé
			if tc.quota > 0 && maxVal-minVal > 1 {
				t.Errorf("bucket dispersion too high: max=%d, min=%d, diff=%d > 1", maxVal, minVal, maxVal-minVal)
			}
		})
	}
}

func TestDistributeQuota_NegativeQuota_ReturnsError(t *testing.T) {
	_, err := domain.DistributeQuota(-5, 32)
	if err != domain.ErrNegativeQuota {
		t.Errorf("expected ErrNegativeQuota, got %v", err)
	}
}

func TestDistributeAdditionalQuota(t *testing.T) {
	// Khởi tạo 32 bucket từ 100 vé
	buckets, _ := domain.DistributeQuota(100, 32)
	current := make([]domain.BucketInventory, len(buckets))
	for i, b := range buckets {
		current[i] = domain.BucketInventory{
			Bucket:    i,
			Capacity:  b,
			Available: b,
		}
	}

	// Tăng thêm 50 vé theo BR-E3
	updated, err := domain.DistributeAdditionalQuota(current, 50)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	newTotalAvailable := 0
	newTotalCapacity := 0
	for _, b := range updated {
		newTotalAvailable += b.Available
		newTotalCapacity += b.Capacity
	}

	if newTotalAvailable != 150 {
		t.Errorf("expected total available 150, got %d", newTotalAvailable)
	}
	if newTotalCapacity != 150 {
		t.Errorf("expected total capacity 150, got %d", newTotalCapacity)
	}

	// Lỗi khi delta <= 0
	_, err = domain.DistributeAdditionalQuota(current, 0)
	if err != domain.ErrQuantityNotAllowed {
		t.Errorf("expected ErrQuantityNotAllowed, got %v", err)
	}

	// Lỗi khi danh sách rỗng
	_, err = domain.DistributeAdditionalQuota([]domain.BucketInventory{}, 10)
	if err != domain.ErrInvalidBucketCount {
		t.Errorf("expected ErrInvalidBucketCount, got %v", err)
	}
}
