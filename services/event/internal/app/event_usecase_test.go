package app_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/eventflow/eventflow/services/event/internal/app"
	"github.com/eventflow/eventflow/services/event/internal/domain"
	"github.com/google/uuid"
)

func TestCreateEventUseCase(t *testing.T) {
	ctx := context.Background()
	repo := newFakeRepo()
	uc := app.NewCreateEventUseCase(repo)

	organizerID := uuid.New()
	now := time.Now().UTC()

	t.Run("successful creation", func(t *testing.T) {
		input := app.CreateEventInput{
			OrganizerID: organizerID,
			Title:       "Tech Conference 2026",
			Description: "Annual dev conference",
			Venue:       "Convention Center",
			StartsAt:    now.Add(72 * time.Hour),
			SaleStartAt: now.Add(24 * time.Hour),
		}

		evt, err := uc.Execute(ctx, input)
		if err != nil {
			t.Fatalf("unexpected error creating event: %v", err)
		}
		if evt.Status != domain.StatusDraft {
			t.Errorf("expected DRAFT status, got %s", evt.Status)
		}
		if repo.events[evt.ID] == nil {
			t.Errorf("expected event to be persisted in repo")
		}
	})

	t.Run("invalid title", func(t *testing.T) {
		input := app.CreateEventInput{
			OrganizerID: organizerID,
			Title:       "",
			StartsAt:    now.Add(72 * time.Hour),
			SaleStartAt: now.Add(24 * time.Hour),
		}

		_, err := uc.Execute(ctx, input)
		if !errors.Is(err, domain.ErrInvalidTitle) {
			t.Errorf("expected ErrInvalidTitle, got %v", err)
		}
	})

	t.Run("repo error", func(t *testing.T) {
		failingRepo := newFakeRepo()
		failingRepo.createErr = errors.New("db error")
		failingUC := app.NewCreateEventUseCase(failingRepo)

		input := app.CreateEventInput{
			OrganizerID: organizerID,
			Title:       "Valid Event",
			StartsAt:    now.Add(72 * time.Hour),
			SaleStartAt: now.Add(24 * time.Hour),
		}

		_, err := failingUC.Execute(ctx, input)
		if err == nil || err.Error() != "db error" {
			t.Errorf("expected db error, got %v", err)
		}
	})
}

func TestUpdateEventUseCase(t *testing.T) {
	ctx := context.Background()
	repo := newFakeRepo()
	createUC := app.NewCreateEventUseCase(repo)
	updateUC := app.NewUpdateEventUseCase(repo)

	organizerID := uuid.New()
	now := time.Now().UTC()

	evt, _ := createUC.Execute(ctx, app.CreateEventInput{
		OrganizerID: organizerID,
		Title:       "Initial Title",
		StartsAt:    now.Add(72 * time.Hour),
		SaleStartAt: now.Add(24 * time.Hour),
	})

	t.Run("successful update by organizer", func(t *testing.T) {
		updated, err := updateUC.Execute(ctx, app.UpdateEventInput{
			EventID:     evt.ID,
			OrganizerID: organizerID,
			Title:       "Updated Title",
			Description: "Updated desc",
			Venue:       "Hall B",
			StartsAt:    now.Add(80 * time.Hour),
			SaleStartAt: now.Add(25 * time.Hour),
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if updated.Title != "Updated Title" || updated.Venue != "Hall B" {
			t.Errorf("unexpected updated event fields: %+v", updated)
		}
	})

	t.Run("unauthorized organizer", func(t *testing.T) {
		otherOrganizer := uuid.New()
		_, err := updateUC.Execute(ctx, app.UpdateEventInput{
			EventID:     evt.ID,
			OrganizerID: otherOrganizer,
			Title:       "Hacked Title",
			StartsAt:    now.Add(80 * time.Hour),
			SaleStartAt: now.Add(25 * time.Hour),
		})
		if !errors.Is(err, domain.ErrUnauthorizedOrganizer) {
			t.Errorf("expected ErrUnauthorizedOrganizer, got %v", err)
		}
	})

	t.Run("event not found", func(t *testing.T) {
		_, err := updateUC.Execute(ctx, app.UpdateEventInput{
			EventID:     uuid.New(),
			OrganizerID: organizerID,
			Title:       "Nonexistent",
			StartsAt:    now.Add(80 * time.Hour),
			SaleStartAt: now.Add(25 * time.Hour),
		})
		if !errors.Is(err, domain.ErrEventNotFound) {
			t.Errorf("expected ErrEventNotFound, got %v", err)
		}
	})
}

func TestManageTiersUseCase(t *testing.T) {
	ctx := context.Background()
	repo := newFakeRepo()
	createUC := app.NewCreateEventUseCase(repo)
	tiersUC := app.NewManageTiersUseCase(repo)

	organizerID := uuid.New()
	now := time.Now().UTC()

	evt, _ := createUC.Execute(ctx, app.CreateEventInput{
		OrganizerID: organizerID,
		Title:       "Tier Test Event",
		StartsAt:    now.Add(72 * time.Hour),
		SaleStartAt: now.Add(24 * time.Hour),
	})

	var createdTier *domain.TicketTier
	t.Run("add tier successfully", func(t *testing.T) {
		var err error
		createdTier, err = tiersUC.AddTier(ctx, app.AddTierInput{
			EventID:     evt.ID,
			OrganizerID: organizerID,
			Name:        "VIP",
			PriceCents:  1500000,
			Quota:       100,
		})
		if err != nil {
			t.Fatalf("unexpected error adding tier: %v", err)
		}
		if createdTier.Name != "VIP" || createdTier.Quota != 100 {
			t.Errorf("unexpected tier details: %+v", createdTier)
		}
	})

	t.Run("update quota in DRAFT", func(t *testing.T) {
		updated, err := tiersUC.UpdateTierQuota(ctx, app.UpdateTierQuotaInput{
			EventID:     evt.ID,
			OrganizerID: organizerID,
			TierID:      createdTier.ID,
			NewQuota:    80, // Giảm được khi DRAFT
		})
		if err != nil {
			t.Fatalf("unexpected error updating quota in DRAFT: %v", err)
		}
		if updated.Quota != 80 {
			t.Errorf("expected quota 80, got %d", updated.Quota)
		}
	})

	t.Run("BR-E3: cấm giảm quota khi ON_SALE", func(t *testing.T) {
		evt.Status = domain.StatusOnSale
		_, err := tiersUC.UpdateTierQuota(ctx, app.UpdateTierQuotaInput{
			EventID:     evt.ID,
			OrganizerID: organizerID,
			TierID:      createdTier.ID,
			NewQuota:    50, // 50 < 80
		})
		if !errors.Is(err, domain.ErrCannotReduceQuotaOnSale) {
			t.Errorf("expected ErrCannotReduceQuotaOnSale, got %v", err)
		}
	})

	t.Run("BR-E3: cho phép tăng quota khi ON_SALE", func(t *testing.T) {
		evt.Status = domain.StatusOnSale
		updated, err := tiersUC.UpdateTierQuota(ctx, app.UpdateTierQuotaInput{
			EventID:     evt.ID,
			OrganizerID: organizerID,
			TierID:      createdTier.ID,
			NewQuota:    120, // 120 > 80
		})
		if err != nil {
			t.Fatalf("unexpected error increasing quota: %v", err)
		}
		if updated.Quota != 120 {
			t.Errorf("expected quota 120, got %d", updated.Quota)
		}
	})
}

func TestTransitionStateUseCase(t *testing.T) {
	ctx := context.Background()
	repo := newFakeRepo()
	createUC := app.NewCreateEventUseCase(repo)
	tiersUC := app.NewManageTiersUseCase(repo)
	transUC := app.NewTransitionStateUseCase(repo)

	organizerID := uuid.New()
	now := time.Now().UTC()
	saleStartAt := now.Add(2 * time.Hour)

	evt, _ := createUC.Execute(ctx, app.CreateEventInput{
		OrganizerID: organizerID,
		Title:       "State Transition Event",
		StartsAt:    now.Add(48 * time.Hour),
		SaleStartAt: saleStartAt,
	})

	t.Run("BR-E1: Submit without tiers fails", func(t *testing.T) {
		_, err := transUC.SubmitForReview(ctx, evt.ID, organizerID)
		if !errors.Is(err, domain.ErrNoValidTicketTiers) {
			t.Errorf("expected ErrNoValidTicketTiers, got %v", err)
		}
	})

	t.Run("BR-E1: Submit with tier succeeds", func(t *testing.T) {
		_, _ = tiersUC.AddTier(ctx, app.AddTierInput{
			EventID:     evt.ID,
			OrganizerID: organizerID,
			Name:        "General",
			PriceCents:  250000,
			Quota:       300,
		})

		submitted, err := transUC.SubmitForReview(ctx, evt.ID, organizerID)
		if err != nil {
			t.Fatalf("unexpected error submitting: %v", err)
		}
		if submitted.Status != domain.StatusPendingReview {
			t.Errorf("expected PENDING_REVIEW, got %s", submitted.Status)
		}
	})

	t.Run("BR-E2: Approve rejected when T0 < 30m", func(t *testing.T) {
		// Lúc duyệt chỉ còn 10 phút trước T0
		tooLateApproval := saleStartAt.Add(-10 * time.Minute)
		_, err := transUC.Approve(ctx, evt.ID, tooLateApproval)
		if !errors.Is(err, domain.ErrSaleStartTimeTooEarly) {
			t.Errorf("expected ErrSaleStartTimeTooEarly, got %v", err)
		}
	})

	t.Run("BR-E2: Approve succeeds when T0 >= 30m", func(t *testing.T) {
		validApproval := saleStartAt.Add(-45 * time.Minute)
		approved, err := transUC.Approve(ctx, evt.ID, validApproval)
		if err != nil {
			t.Fatalf("unexpected error approving: %v", err)
		}
		if approved.Status != domain.StatusScheduled {
			t.Errorf("expected SCHEDULED, got %s", approved.Status)
		}
	})

	t.Run("Publish when T0 arrives", func(t *testing.T) {
		published, err := transUC.Publish(ctx, evt.ID, saleStartAt.Add(1*time.Minute))
		if err != nil {
			t.Fatalf("unexpected error publishing: %v", err)
		}
		if published.Status != domain.StatusOnSale {
			t.Errorf("expected ON_SALE, got %s", published.Status)
		}
	})

	t.Run("Cancel event", func(t *testing.T) {
		cancelled, err := transUC.Cancel(ctx, evt.ID, organizerID)
		if err != nil {
			t.Fatalf("unexpected error cancelling: %v", err)
		}
		if cancelled.Status != domain.StatusCancelled {
			t.Errorf("expected CANCELLED, got %s", cancelled.Status)
		}

		foundOutbox := false
		for _, msg := range repo.outbox {
			if msg.RoutingKey == app.EventCancelledRoutingKey && msg.AggregateID == evt.ID {
				foundOutbox = true
				break
			}
		}
		if !foundOutbox {
			t.Errorf("expected event.cancelled outbox message")
		}
	})

	t.Run("Reject transition", func(t *testing.T) {
		pendingEvt, _ := createUC.Execute(ctx, app.CreateEventInput{
			OrganizerID: organizerID,
			Title:       "To Reject",
			StartsAt:    now.Add(48 * time.Hour),
			SaleStartAt: now.Add(2 * time.Hour),
		})
		_, _ = tiersUC.AddTier(ctx, app.AddTierInput{
			EventID:     pendingEvt.ID,
			OrganizerID: organizerID,
			Name:        "Tier",
			PriceCents:  1000,
			Quota:       10,
		})
		_, _ = transUC.SubmitForReview(ctx, pendingEvt.ID, organizerID)

		rejected, err := transUC.Reject(ctx, pendingEvt.ID)
		if err != nil {
			t.Fatalf("unexpected error rejecting: %v", err)
		}
		if rejected.Status != domain.StatusRejected {
			t.Errorf("expected REJECTED, got %s", rejected.Status)
		}
	})
}

func TestGetEventUseCase(t *testing.T) {
	ctx := context.Background()
	repo := newFakeRepo()
	createUC := app.NewCreateEventUseCase(repo)
	tiersUC := app.NewManageTiersUseCase(repo)
	getUC := app.NewGetEventUseCase(repo)

	organizerID := uuid.New()
	now := time.Now().UTC()

	evt, _ := createUC.Execute(ctx, app.CreateEventInput{
		OrganizerID: organizerID,
		Slug:        "festival-slug",
		Title:       "Festival Title",
		StartsAt:    now.Add(72 * time.Hour),
		SaleStartAt: now.Add(24 * time.Hour),
	})
	_, _ = tiersUC.AddTier(ctx, app.AddTierInput{
		EventID:     evt.ID,
		OrganizerID: organizerID,
		Name:        "GA",
		PriceCents:  100000,
		Quota:       50,
	})

	t.Run("GetByID", func(t *testing.T) {
		found, err := getUC.GetByID(ctx, evt.ID)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if found.Title != "Festival Title" || len(found.Tiers) != 1 {
			t.Errorf("unexpected event: %+v", found)
		}
	})

	t.Run("GetBySlug", func(t *testing.T) {
		found, err := getUC.GetBySlug(ctx, "festival-slug")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if found.ID != evt.ID {
			t.Errorf("expected ID %v, got %v", evt.ID, found.ID)
		}
	})

	t.Run("ListByOrganizer", func(t *testing.T) {
		list, err := getUC.ListByOrganizer(ctx, organizerID)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(list) != 1 {
			t.Errorf("expected 1 event, got %d", len(list))
		}
	})
}
