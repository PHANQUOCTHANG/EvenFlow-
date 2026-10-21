package domain_test

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/eventflow/eventflow/services/event/internal/domain"
)

// BR-E2 mac dinh 30 phut; moi truong demo ha xuong qua SetMinSaleLead.
func TestApprove_KhoangBRE2CauHinhDuoc(t *testing.T) {
	t.Cleanup(func() { domain.SetMinSaleLead(domain.DefaultMinSaleLead) })
	now := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	newEvt := func() *domain.Event {
		return &domain.Event{ID: uuid.New(), Status: domain.StatusPendingReview, SaleStartAt: now.Add(2 * time.Minute)}
	}

	if domain.MinSaleLead() != 30*time.Minute {
		t.Fatalf("mac dinh = %v, muon 30m", domain.MinSaleLead())
	}
	if err := newEvt().Approve(now); !errors.Is(err, domain.ErrSaleStartTimeTooEarly) {
		t.Fatalf("T0 sau 2 phut voi BR-E2 30 phut: err = %v", err)
	}

	domain.SetMinSaleLead(time.Minute)
	e := newEvt()
	if err := e.Approve(now); err != nil || e.Status != domain.StatusScheduled {
		t.Fatalf("lead 1 phut: err = %v, status %s", err, e.Status)
	}
	if err := newEvt().ApproveOverride(now); err != nil {
		t.Fatalf("ApproveOverride lead 1 phut: %v", err)
	}
	domain.SetMinSaleLead(-time.Hour)
	if domain.MinSaleLead() != 0 {
		t.Errorf("lead am phai ve 0, nhan %v", domain.MinSaleLead())
	}
}
