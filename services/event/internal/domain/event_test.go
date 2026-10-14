package domain_test

import (
	"testing"
	"time"

	"github.com/eventflow/eventflow/services/event/internal/domain"
	"github.com/google/uuid"
)

func createValidBaseEvent(t *testing.T) *domain.Event {
	t.Helper()
	organizerID := uuid.New()
	now := time.Now().UTC()
	saleStartAt := now.Add(2 * time.Hour) // 2 giờ sau
	startsAt := now.Add(48 * time.Hour)   // 2 ngày sau

	evt, err := domain.NewEvent(
		organizerID,
		"music-festival-2026",
		"Music Festival 2026",
		"Annual live music fest",
		"National Stadium",
		startsAt,
		saleStartAt,
		nil,
	)
	if err != nil {
		t.Fatalf("unexpected error creating event: %v", err)
	}
	return evt
}

func TestNewEvent_Validation(t *testing.T) {
	organizerID := uuid.New()
	now := time.Now().UTC()

	t.Run("empty title", func(t *testing.T) {
		_, err := domain.NewEvent(
			organizerID, "slug", "", "desc", "venue",
			now.Add(2*time.Hour), now.Add(1*time.Hour), nil,
		)
		if err != domain.ErrInvalidTitle {
			t.Errorf("expected ErrInvalidTitle, got %v", err)
		}
	})

	t.Run("starts_at before sale_start_at", func(t *testing.T) {
		_, err := domain.NewEvent(
			organizerID, "slug", "Title", "desc", "venue",
			now.Add(1*time.Hour), now.Add(2*time.Hour), nil,
		)
		if err != domain.ErrInvalidEventDates {
			t.Errorf("expected ErrInvalidEventDates, got %v", err)
		}
	})

	t.Run("valid event defaults", func(t *testing.T) {
		evt, err := domain.NewEvent(
			organizerID, "", "My Awesome Concert", "desc", "venue",
			now.Add(24*time.Hour), now.Add(2*time.Hour), nil,
		)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if evt.Status != domain.StatusDraft {
			t.Errorf("expected status DRAFT, got %s", evt.Status)
		}
		if evt.Slug != "my-awesome-concert" {
			t.Errorf("expected auto slug my-awesome-concert, got %s", evt.Slug)
		}
		if evt.MaxPerOrder != 4 || evt.MaxPerIdentity != 4 || evt.HoldTTLSeconds != 600 {
			t.Errorf("unexpected default parameters: %+v", evt)
		}
	})
}

func TestAddTier_Validation(t *testing.T) {
	evt := createValidBaseEvent(t)

	tier1, err := domain.NewTicketTier(evt.ID, "VIP", 1000000, 100)
	if err != nil {
		t.Fatalf("unexpected error creating tier: %v", err)
	}

	err = evt.AddTier(*tier1)
	if err != nil {
		t.Fatalf("unexpected error adding tier: %v", err)
	}

	// Trùng tên hạng vé
	tierDuplicate, _ := domain.NewTicketTier(evt.ID, "vip", 1500000, 50)
	err = evt.AddTier(*tierDuplicate)
	if err != domain.ErrDuplicateTierName {
		t.Errorf("expected ErrDuplicateTierName, got %v", err)
	}
}

func TestSubmitForReview_BR_E1(t *testing.T) {
	t.Run("cannot submit without ticket tiers", func(t *testing.T) {
		evt := createValidBaseEvent(t)
		err := evt.SubmitForReview()
		if err != domain.ErrNoValidTicketTiers {
			t.Errorf("expected ErrNoValidTicketTiers, got %v", err)
		}
	})

	t.Run("can submit when >= 1 valid tier exists", func(t *testing.T) {
		evt := createValidBaseEvent(t)
		tier, _ := domain.NewTicketTier(evt.ID, "General Admission", 200000, 500)
		_ = evt.AddTier(*tier)

		err := evt.SubmitForReview()
		if err != nil {
			t.Fatalf("unexpected error submitting event: %v", err)
		}
		if evt.Status != domain.StatusPendingReview {
			t.Errorf("expected status PENDING_REVIEW, got %s", evt.Status)
		}
	})
}

func TestApprove_BR_E2(t *testing.T) {
	evt := createValidBaseEvent(t)
	tier, _ := domain.NewTicketTier(evt.ID, "Standard", 300000, 200)
	_ = evt.AddTier(*tier)
	_ = evt.SubmitForReview()

	t.Run("reject approval if T0 < approval_time + 30m", func(t *testing.T) {
		// Giả lập duyệt lúc chỉ còn 15 phút trước giờ mở bán
		simulatedApprovalTime := evt.SaleStartAt.Add(-15 * time.Minute)
		err := evt.Approve(simulatedApprovalTime)
		if err != domain.ErrSaleStartTimeTooEarly {
			t.Errorf("expected ErrSaleStartTimeTooEarly, got %v", err)
		}
	})

	t.Run("approve successfully if T0 >= approval_time + 30m", func(t *testing.T) {
		// Giả lập duyệt lúc còn 40 phút trước giờ mở bán
		simulatedApprovalTime := evt.SaleStartAt.Add(-40 * time.Minute)
		err := evt.Approve(simulatedApprovalTime)
		if err != nil {
			t.Fatalf("unexpected error approving event: %v", err)
		}
		if evt.Status != domain.StatusScheduled {
			t.Errorf("expected status SCHEDULED, got %s", evt.Status)
		}
	})
}

func TestReject_And_Resubmit(t *testing.T) {
	evt := createValidBaseEvent(t)
	tier, _ := domain.NewTicketTier(evt.ID, "Standard", 300000, 200)
	_ = evt.AddTier(*tier)
	_ = evt.SubmitForReview()

	err := evt.Reject()
	if err != nil {
		t.Fatalf("unexpected error rejecting: %v", err)
	}
	if evt.Status != domain.StatusRejected {
		t.Errorf("expected status REJECTED, got %s", evt.Status)
	}

	// Organizer sửa lại và submit lại
	err = evt.SubmitForReview()
	if err != nil {
		t.Fatalf("expected to be able to resubmit from REJECTED: %v", err)
	}
	if evt.Status != domain.StatusPendingReview {
		t.Errorf("expected status PENDING_REVIEW, got %s", evt.Status)
	}
}

func TestOpenSale_StateTransition(t *testing.T) {
	evt := createValidBaseEvent(t)
	tier, _ := domain.NewTicketTier(evt.ID, "Standard", 300000, 200)
	_ = evt.AddTier(*tier)
	_ = evt.SubmitForReview()
	_ = evt.Approve(evt.SaleStartAt.Add(-1 * time.Hour))

	t.Run("cannot open sale before T0", func(t *testing.T) {
		err := evt.OpenSale(evt.SaleStartAt.Add(-5 * time.Minute))
		if err != domain.ErrInvalidStateTransition {
			t.Errorf("expected ErrInvalidStateTransition, got %v", err)
		}
	})

	t.Run("open sale when T0 arrives", func(t *testing.T) {
		err := evt.OpenSale(evt.SaleStartAt.Add(1 * time.Second))
		if err != nil {
			t.Fatalf("unexpected error opening sale: %v", err)
		}
		if evt.Status != domain.StatusOnSale {
			t.Errorf("expected status ON_SALE, got %s", evt.Status)
		}
	})
}

func TestUpdateTierQuota_BR_E3(t *testing.T) {
	evt := createValidBaseEvent(t)
	tier, _ := domain.NewTicketTier(evt.ID, "Standard", 300000, 200)
	_ = evt.AddTier(*tier)
	_ = evt.SubmitForReview()
	_ = evt.Approve(evt.SaleStartAt.Add(-1 * time.Hour))
	_ = evt.OpenSale(evt.SaleStartAt)

	// Đang ở trạng thái ON_SALE:
	t.Run("cấm giảm quota sau khi ON_SALE", func(t *testing.T) {
		err := evt.UpdateTierQuota(tier.ID, 150) // 150 < 200
		if err != domain.ErrCannotReduceQuotaOnSale {
			t.Errorf("expected ErrCannotReduceQuotaOnSale, got %v", err)
		}
	})

	t.Run("cho phép tăng quota khi ON_SALE", func(t *testing.T) {
		err := evt.UpdateTierQuota(tier.ID, 300) // 300 > 200
		if err != nil {
			t.Fatalf("unexpected error increasing quota: %v", err)
		}
		if evt.Tiers[0].Quota != 300 {
			t.Errorf("expected quota 300, got %d", evt.Tiers[0].Quota)
		}
	})
}

func TestSoldOut_Closed_Completed_Transitions(t *testing.T) {
	evt := createValidBaseEvent(t)
	tier, _ := domain.NewTicketTier(evt.ID, "Standard", 300000, 200)
	_ = evt.AddTier(*tier)
	_ = evt.SubmitForReview()
	_ = evt.Approve(evt.SaleStartAt.Add(-1 * time.Hour))
	_ = evt.OpenSale(evt.SaleStartAt)

	// ON_SALE -> SOLD_OUT
	err := evt.MarkSoldOut()
	if err != nil {
		t.Fatalf("unexpected error marking sold out: %v", err)
	}
	if evt.Status != domain.StatusSoldOut {
		t.Errorf("expected status SOLD_OUT, got %s", evt.Status)
	}

	// SOLD_OUT mà tăng quota -> tự quay lại ON_SALE
	err = evt.UpdateTierQuota(tier.ID, 500)
	if err != nil {
		t.Fatalf("unexpected error updating quota in SOLD_OUT: %v", err)
	}
	if evt.Status != domain.StatusOnSale {
		t.Errorf("expected auto transition back to ON_SALE, got %s", evt.Status)
	}

	// ON_SALE -> CLOSED
	err = evt.CloseSale()
	if err != nil {
		t.Fatalf("unexpected error closing sale: %v", err)
	}
	if evt.Status != domain.StatusClosed {
		t.Errorf("expected status CLOSED, got %s", evt.Status)
	}

	// CLOSED -> COMPLETED
	err = evt.Complete()
	if err != nil {
		t.Fatalf("unexpected error completing event: %v", err)
	}
	if evt.Status != domain.StatusCompleted {
		t.Errorf("expected status COMPLETED, got %s", evt.Status)
	}
}

func TestCancel_Event(t *testing.T) {
	evt := createValidBaseEvent(t)
	err := evt.Cancel()
	if err != nil {
		t.Fatalf("unexpected error cancelling event: %v", err)
	}
	if evt.Status != domain.StatusCancelled {
		t.Errorf("expected status CANCELLED, got %s", evt.Status)
	}

	// Không thể cancel lần 2
	err = evt.Cancel()
	if err != domain.ErrInvalidStateTransition {
		t.Errorf("expected ErrInvalidStateTransition, got %v", err)
	}
}

func TestTicketTier_UpdateValidation(t *testing.T) {
	tier, err := domain.NewTicketTier(uuid.New(), "VIP", 500000, 50)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	err = tier.Update("VIP Gold", 600000, 100)
	if err != nil {
		t.Fatalf("unexpected update error: %v", err)
	}
	if tier.Name != "VIP Gold" || tier.PriceCents != 600000 || tier.Quota != 100 {
		t.Errorf("unexpected tier fields after update: %+v", tier)
	}

	// Tên rỗng
	err = tier.Update("", 600000, 100)
	if err == nil {
		t.Error("expected error on empty name")
	}

	// Giá âm
	err = tier.Update("VIP", -10, 100)
	if err != domain.ErrInvalidPrice {
		t.Errorf("expected ErrInvalidPrice, got %v", err)
	}

	// Quota âm
	err = tier.Update("VIP", 500000, 0)
	if err != domain.ErrInvalidQuota {
		t.Errorf("expected ErrInvalidQuota, got %v", err)
	}
}

func TestApproveOverride(t *testing.T) {
	evt := createValidBaseEvent(t)
	tier, _ := domain.NewTicketTier(evt.ID, "Standard", 300000, 200)
	_ = evt.AddTier(*tier)
	_ = evt.SubmitForReview()

	// 1. ApproveOverride from PENDING_REVIEW -> SCHEDULED
	now := evt.SaleStartAt.Add(-1 * time.Hour)
	err := evt.ApproveOverride(now)
	if err != nil {
		t.Fatalf("unexpected error on ApproveOverride from PENDING_REVIEW: %v", err)
	}
	if evt.Status != domain.StatusScheduled {
		t.Errorf("expected status SCHEDULED, got %s", evt.Status)
	}

	// 2. Reject then ApproveOverride from REJECTED -> SCHEDULED
	evt2 := createValidBaseEvent(t)
	_ = evt2.AddTier(*tier)
	_ = evt2.SubmitForReview()
	_ = evt2.Reject()
	if evt2.Status != domain.StatusRejected {
		t.Fatalf("expected REJECTED, got %s", evt2.Status)
	}

	err = evt2.ApproveOverride(now)
	if err != nil {
		t.Fatalf("unexpected error on ApproveOverride from REJECTED: %v", err)
	}
	if evt2.Status != domain.StatusScheduled {
		t.Errorf("expected status SCHEDULED, got %s", evt2.Status)
	}

	// 3. Fails if sale_start_at < now + 30 min (BR-E2)
	evt3 := createValidBaseEvent(t)
	_ = evt3.AddTier(*tier)
	_ = evt3.SubmitForReview()
	lateApproval := evt3.SaleStartAt.Add(-10 * time.Minute)
	err = evt3.ApproveOverride(lateApproval)
	if err != domain.ErrSaleStartTimeTooEarly {
		t.Errorf("expected ErrSaleStartTimeTooEarly, got %v", err)
	}

	// 4. Fails from invalid status (e.g. DRAFT)
	draftEvt := createValidBaseEvent(t)
	err = draftEvt.ApproveOverride(now)
	if err != domain.ErrInvalidStateTransition {
		t.Errorf("expected ErrInvalidStateTransition, got %v", err)
	}
}
