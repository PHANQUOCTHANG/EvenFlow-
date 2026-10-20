package domain_test

import (
	"testing"

	"github.com/eventflow/eventflow/services/ticketing/internal/domain"
)

func TestLevelOf(t *testing.T) {
	cases := []struct {
		name                string
		available, capacity int
		want                domain.AvailabilityLevel
	}{
		{"con nhieu", 500, 1000, domain.AvailabilityAvailable},
		{"vua tren nguong 10%", 101, 1000, domain.AvailabilityAvailable},
		{"dung nguong 10%", 100, 1000, domain.AvailabilityFewLeft},
		{"sap het", 3, 1000, domain.AvailabilityFewLeft},
		{"con 1", 1, 1000, domain.AvailabilityFewLeft},
		{"het", 0, 1000, domain.AvailabilitySoldOut},
		{"am (loi lech) coi la het", -5, 1000, domain.AvailabilitySoldOut},
		{"suc chua 0 coi la het", 5, 0, domain.AvailabilitySoldOut},
		{"hang ve nho: con 1/10 = 10%", 1, 10, domain.AvailabilityFewLeft},
		{"hang ve nho: con 1/5 = 20%", 1, 5, domain.AvailabilityAvailable},
	}
	for _, c := range cases {
		if got := domain.LevelOf(c.available, c.capacity); got != c.want {
			t.Errorf("%s: LevelOf(%d,%d) = %s, muon %s", c.name, c.available, c.capacity, got, c.want)
		}
	}
}
