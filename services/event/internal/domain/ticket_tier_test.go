package domain_test

import (
	"testing"

	"github.com/google/uuid"

	"github.com/eventflow/eventflow/services/event/internal/domain"
)

func TestTicketTier_DistributeQuota_Invariants(t *testing.T) {
	eventID := uuid.New()

	testCases := []struct {
		name        string
		quota       int
		bucketCount int
	}{
		{name: "Zero quota", quota: 0, bucketCount: 32},
		{name: "Quota less than bucket count", quota: 10, bucketCount: 32},
		{name: "Quota exact bucket count", quota: 32, bucketCount: 32},
		{name: "Quota with remainder", quota: 100, bucketCount: 32},
		{name: "Large quota", quota: 50000, bucketCount: 32},
		{name: "Odd bucket count", quota: 100, bucketCount: 16},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tier := &domain.TicketTier{
				ID:          uuid.New(),
				EventID:     eventID,
				Name:        "Test Tier",
				PriceCents:  100000,
				Quota:       tc.quota,
				BucketCount: tc.bucketCount,
			}

			buckets := tier.DistributeQuota()

			if len(buckets) != tc.bucketCount {
				t.Fatalf("expected %d buckets, got %d", tc.bucketCount, len(buckets))
			}

			sum := 0
			minVal := int(^uint(0) >> 1)
			maxVal := -1

			for _, qty := range buckets {
				if qty < 0 {
					t.Fatalf("bucket quantity cannot be negative: %d", qty)
				}
				sum += qty
				if qty < minVal {
					minVal = qty
				}
				if qty > maxVal {
					maxVal = qty
				}
			}

			// Invariant 1: Sum of all buckets must match total quota exactly
			if sum != tc.quota {
				t.Fatalf("invariant 1 failed: sum(%d) != quota(%d)", sum, tc.quota)
			}

			// Invariant 2: Difference between any two buckets <= 1
			if tc.quota > 0 && maxVal-minVal > 1 {
				t.Fatalf("invariant 2 failed: max(%d) - min(%d) > 1", maxVal, minVal)
			}
		})
	}
}

func TestTicketTier_DistributeAdditionalQuota(t *testing.T) {
	eventID := uuid.New()
	tier := &domain.TicketTier{
		ID:          uuid.New(),
		EventID:     eventID,
		Name:        "VIP",
		PriceCents:  500000,
		Quota:       100,
		BucketCount: 32,
	}

	delta := 50
	additional := tier.DistributeAdditionalQuota(delta)

	if len(additional) != 32 {
		t.Fatalf("expected 32 buckets, got %d", len(additional))
	}

	sum := 0
	for _, add := range additional {
		sum += add
	}

	if sum != delta {
		t.Fatalf("expected additional sum %d, got %d", delta, sum)
	}

	// Negative or zero delta
	if sumZero := sumSlice(tier.DistributeAdditionalQuota(0)); sumZero != 0 {
		t.Fatalf("expected 0 for delta 0, got %d", sumZero)
	}
	if sumNeg := sumSlice(tier.DistributeAdditionalQuota(-10)); sumNeg != 0 {
		t.Fatalf("expected 0 for negative delta, got %d", sumNeg)
	}
}

func sumSlice(nums []int) int {
	s := 0
	for _, n := range nums {
		s += n
	}
	return s
}
