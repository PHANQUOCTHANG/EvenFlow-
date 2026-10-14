package app_test

import (
	"context"
	"testing"
	"time"

	"github.com/eventflow/eventflow/services/event/internal/app"
	"github.com/eventflow/eventflow/services/event/internal/domain"
	"github.com/google/uuid"
)

func TestModeratorApprove_FromPending(t *testing.T) {
	ctx := context.Background()
	eventRepo := newFakeRepo()
	modRepo := newFakeModerationRepo()
	auditRepo := newFakeAuditRepo()

	evt := createPendingEvent(t, eventRepo)
	rev := &domain.ModerationReview{
		ID:        uuid.New(),
		EventID:   evt.ID,
		AiScore:   0.6,
		AiVerdict: domain.VerdictNeedsHuman,
		CreatedAt: time.Now().UTC(),
	}
	_ = modRepo.Create(ctx, rev)

	uc := app.NewModeratorActionUseCase(eventRepo, modRepo, auditRepo)
	modID := uuid.New()

	approvedEvt, err := uc.Approve(ctx, app.ModeratorApproveInput{
		EventID:     evt.ID,
		ModeratorID: modID,
		Note:        "Manually verified organizer credentials and license",
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if approvedEvt.Status != domain.StatusScheduled {
		t.Errorf("expected SCHEDULED, got %s", approvedEvt.Status)
	}

	updatedRev, _ := modRepo.GetLatestByEventID(ctx, evt.ID)
	if updatedRev.HumanVerdict == nil || *updatedRev.HumanVerdict != "APPROVED" {
		t.Errorf("expected HumanVerdict APPROVED, got %v", updatedRev.HumanVerdict)
	}
	if updatedRev.HumanID == nil || *updatedRev.HumanID != modID {
		t.Errorf("expected HumanID %s, got %v", modID, updatedRev.HumanID)
	}

	logs, _ := auditRepo.ListBySubject(ctx, "event:"+evt.ID.String())
	if len(logs) != 1 {
		t.Fatalf("expected 1 audit log, got %d", len(logs))
	}
	if logs[0].ActorType != "human" || logs[0].Action != "event.moderator_approved" {
		t.Errorf("unexpected audit log: %+v", logs[0])
	}
}

func TestModeratorApprove_OverrideAiRejection(t *testing.T) {
	ctx := context.Background()
	eventRepo := newFakeRepo()
	modRepo := newFakeModerationRepo()
	auditRepo := newFakeAuditRepo()

	evt := createPendingEvent(t, eventRepo)
	_ = evt.Reject() // Event was rejected by AI
	_ = eventRepo.Update(ctx, evt)

	rev := &domain.ModerationReview{
		ID:        uuid.New(),
		EventID:   evt.ID,
		AiScore:   0.9,
		AiVerdict: domain.VerdictAutoRejected,
		CreatedAt: time.Now().UTC(),
	}
	_ = modRepo.Create(ctx, rev)

	uc := app.NewModeratorActionUseCase(eventRepo, modRepo, auditRepo)
	modID := uuid.New()

	// Moderator overrides AI rejection (EV-121)
	approvedEvt, err := uc.Approve(ctx, app.ModeratorApproveInput{
		EventID:     evt.ID,
		ModeratorID: modID,
		Note:        "False positive: verified permission documents",
	})

	if err != nil {
		t.Fatalf("unexpected error overriding rejection: %v", err)
	}
	if approvedEvt.Status != domain.StatusScheduled {
		t.Errorf("expected SCHEDULED, got %s", approvedEvt.Status)
	}
}

func TestModeratorReject_Success(t *testing.T) {
	ctx := context.Background()
	eventRepo := newFakeRepo()
	modRepo := newFakeModerationRepo()
	auditRepo := newFakeAuditRepo()

	evt := createPendingEvent(t, eventRepo)
	rev := &domain.ModerationReview{
		ID:        uuid.New(),
		EventID:   evt.ID,
		AiScore:   0.5,
		AiVerdict: domain.VerdictNeedsHuman,
		CreatedAt: time.Now().UTC(),
	}
	_ = modRepo.Create(ctx, rev)

	uc := app.NewModeratorActionUseCase(eventRepo, modRepo, auditRepo)
	modID := uuid.New()

	rejectedEvt, err := uc.Reject(ctx, app.ModeratorRejectInput{
		EventID:     evt.ID,
		ModeratorID: modID,
		Reason:      "Organizer failed to provide business permit",
	})

	if err != nil {
		t.Fatalf("unexpected error rejecting: %v", err)
	}
	if rejectedEvt.Status != domain.StatusRejected {
		t.Errorf("expected REJECTED, got %s", rejectedEvt.Status)
	}

	updatedRev, _ := modRepo.GetLatestByEventID(ctx, evt.ID)
	if updatedRev.HumanVerdict == nil || *updatedRev.HumanVerdict != "REJECTED" {
		t.Errorf("expected HumanVerdict REJECTED, got %v", updatedRev.HumanVerdict)
	}

	logs, _ := auditRepo.ListBySubject(ctx, "event:"+evt.ID.String())
	if len(logs) != 1 {
		t.Fatalf("expected 1 audit log, got %d", len(logs))
	}
	if logs[0].ActorType != "human" || logs[0].Action != "event.moderator_rejected" {
		t.Errorf("unexpected audit log: %+v", logs[0])
	}
	if logs[0].Reason == nil || *logs[0].Reason != "Organizer failed to provide business permit" {
		t.Errorf("expected rejection reason in audit log (BR-E5), got %v", logs[0].Reason)
	}
}

func TestModeratorReject_MissingReason_Fails(t *testing.T) {
	ctx := context.Background()
	eventRepo := newFakeRepo()
	modRepo := newFakeModerationRepo()
	auditRepo := newFakeAuditRepo()

	evt := createPendingEvent(t, eventRepo)
	uc := app.NewModeratorActionUseCase(eventRepo, modRepo, auditRepo)

	// Thiếu reason -> lỗi domain.ErrRejectReasonRequired theo BR-E5
	_, err := uc.Reject(ctx, app.ModeratorRejectInput{
		EventID:     evt.ID,
		ModeratorID: uuid.New(),
		Reason:      "   ",
	})

	if err != domain.ErrRejectReasonRequired {
		t.Errorf("expected ErrRejectReasonRequired, got %v", err)
	}
}

func TestListModerationQueue(t *testing.T) {
	ctx := context.Background()
	eventRepo := newFakeRepo()
	modRepo := newFakeModerationRepo()

	evt := createPendingEvent(t, eventRepo)
	rev := &domain.ModerationReview{
		ID:        uuid.New(),
		EventID:   evt.ID,
		AiScore:   0.65,
		AiVerdict: domain.VerdictNeedsHuman,
		AiReasons: []string{"possible duplicate venue booking"},
		CreatedAt: time.Now().UTC(),
	}
	_ = modRepo.Create(ctx, rev)

	uc := app.NewListModerationQueueUseCase(eventRepo, modRepo)
	items, err := uc.Execute(ctx)
	if err != nil {
		t.Fatalf("unexpected error listing queue: %v", err)
	}

	if len(items) != 1 {
		t.Fatalf("expected 1 item in moderation queue, got %d", len(items))
	}
	if items[0].EventID != evt.ID {
		t.Errorf("expected event id %s, got %s", evt.ID, items[0].EventID)
	}
	if items[0].Review.AiScore != 0.65 {
		t.Errorf("expected AI score 0.65, got %f", items[0].Review.AiScore)
	}
}
