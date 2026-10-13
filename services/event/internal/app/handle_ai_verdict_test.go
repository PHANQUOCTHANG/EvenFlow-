package app_test

import (
	"context"
	"testing"
	"time"

	"github.com/eventflow/eventflow/services/event/internal/app"
	"github.com/eventflow/eventflow/services/event/internal/domain"
	"github.com/google/uuid"
)

func createPendingEvent(t *testing.T, repo *fakeEventRepo) *domain.Event {
	t.Helper()
	now := time.Now().UTC()
	evt, err := domain.NewEvent(
		uuid.New(),
		"pending-concert",
		"Pending Concert",
		"Description",
		"Saigon Center",
		now.Add(24*time.Hour),
		now.Add(2*time.Hour),
		nil,
	)
	if err != nil {
		t.Fatalf("failed to create event: %v", err)
	}
	tier, _ := domain.NewTicketTier(evt.ID, "Standard", 100000, 100)
	_ = evt.AddTier(*tier)
	_ = evt.SubmitForReview()
	if err := repo.Create(context.Background(), evt); err != nil {
		t.Fatalf("failed to save event: %v", err)
	}
	return evt
}

func TestHandleAiVerdict_AutoApproved(t *testing.T) {
	ctx := context.Background()
	eventRepo := newFakeRepo()
	modRepo := newFakeModerationRepo()
	auditRepo := newFakeAuditRepo()

	evt := createPendingEvent(t, eventRepo)
	uc := app.NewHandleAiVerdictUseCase(eventRepo, modRepo, auditRepo)

	updatedEvt, rev, err := uc.Execute(ctx, app.HandleAiVerdictInput{
		EventID: evt.ID,
		Score:   0.15,
		Verdict: domain.VerdictAutoApproved,
		Reasons: []string{"content passes all 4 axes"},
		Axes: map[string]float64{
			"forbidden_content": 0.05,
			"misleading":        0.1,
			"copyright":         0.0,
			"incomplete_info":   0.05,
		},
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if updatedEvt.Status != domain.StatusScheduled {
		t.Errorf("expected SCHEDULED, got %s", updatedEvt.Status)
	}
	if rev.AiVerdict != domain.VerdictAutoApproved {
		t.Errorf("expected AUTO_APPROVED, got %s", rev.AiVerdict)
	}

	logs, _ := auditRepo.ListBySubject(ctx, "event:"+evt.ID.String())
	if len(logs) != 1 {
		t.Fatalf("expected 1 audit log, got %d", len(logs))
	}
	if logs[0].Action != "event.auto_approved" || logs[0].ActorType != "ai" {
		t.Errorf("unexpected audit log: %+v", logs[0])
	}
}

func TestHandleAiVerdict_NeedsHuman(t *testing.T) {
	ctx := context.Background()
	eventRepo := newFakeRepo()
	modRepo := newFakeModerationRepo()
	auditRepo := newFakeAuditRepo()

	evt := createPendingEvent(t, eventRepo)
	uc := app.NewHandleAiVerdictUseCase(eventRepo, modRepo, auditRepo)

	updatedEvt, rev, err := uc.Execute(ctx, app.HandleAiVerdictInput{
		EventID: evt.ID,
		Score:   0.55,
		Verdict: domain.VerdictNeedsHuman,
		Reasons: []string{"unclear venue description"},
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Event remains PENDING_REVIEW for human moderator
	if updatedEvt.Status != domain.StatusPendingReview {
		t.Errorf("expected PENDING_REVIEW, got %s", updatedEvt.Status)
	}
	if rev.AiVerdict != domain.VerdictNeedsHuman {
		t.Errorf("expected NEEDS_HUMAN, got %s", rev.AiVerdict)
	}

	logs, _ := auditRepo.ListBySubject(ctx, "event:"+evt.ID.String())
	if len(logs) != 1 || logs[0].Action != "event.needs_human_review" {
		t.Errorf("unexpected audit log: %+v", logs)
	}
}

func TestHandleAiVerdict_AutoRejected(t *testing.T) {
	ctx := context.Background()
	eventRepo := newFakeRepo()
	modRepo := newFakeModerationRepo()
	auditRepo := newFakeAuditRepo()

	evt := createPendingEvent(t, eventRepo)
	uc := app.NewHandleAiVerdictUseCase(eventRepo, modRepo, auditRepo)

	updatedEvt, rev, err := uc.Execute(ctx, app.HandleAiVerdictInput{
		EventID: evt.ID,
		Score:   0.85,
		Verdict: domain.VerdictAutoRejected,
		Reasons: []string{"forbidden content detected", "misleading promotional claims"},
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if updatedEvt.Status != domain.StatusRejected {
		t.Errorf("expected REJECTED, got %s", updatedEvt.Status)
	}
	if rev.AiVerdict != domain.VerdictAutoRejected {
		t.Errorf("expected AUTO_REJECTED, got %s", rev.AiVerdict)
	}

	logs, _ := auditRepo.ListBySubject(ctx, "event:"+evt.ID.String())
	if len(logs) != 1 || logs[0].Action != "event.auto_rejected" {
		t.Fatalf("unexpected audit log: %+v", logs)
	}
	if logs[0].Reason == nil || *logs[0].Reason == "" {
		t.Errorf("expected reason to be recorded for rejection in audit logs (BR-E5)")
	}
}

func TestHandleAiVerdict_InvalidScore(t *testing.T) {
	ctx := context.Background()
	eventRepo := newFakeRepo()
	uc := app.NewHandleAiVerdictUseCase(eventRepo, nil, nil)

	_, _, err := uc.Execute(ctx, app.HandleAiVerdictInput{
		EventID: uuid.New(),
		Score:   1.5,
	})
	if err != domain.ErrInvalidAiScore {
		t.Errorf("expected ErrInvalidAiScore, got %v", err)
	}
}

func TestHandleAiVerdict_InferredVerdict(t *testing.T) {
	ctx := context.Background()
	eventRepo := newFakeRepo()
	modRepo := newFakeModerationRepo()
	auditRepo := newFakeAuditRepo()

	evt := createPendingEvent(t, eventRepo)
	uc := app.NewHandleAiVerdictUseCase(eventRepo, modRepo, auditRepo)

	// Score 0.2 without explicit verdict -> inferred AUTO_APPROVED
	updatedEvt, rev, err := uc.Execute(ctx, app.HandleAiVerdictInput{
		EventID: evt.ID,
		Score:   0.2,
		Reasons: []string{"passed"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if updatedEvt.Status != domain.StatusScheduled {
		t.Errorf("expected SCHEDULED, got %s", updatedEvt.Status)
	}
	if rev.AiVerdict != domain.VerdictAutoApproved {
		t.Errorf("expected AUTO_APPROVED, got %s", rev.AiVerdict)
	}
}

func TestEnqueueModerationUseCase(t *testing.T) {
	ctx := context.Background()
	outboxRepo := newFakeOutboxRepo()
	uc := app.NewEnqueueModerationUseCase(outboxRepo)

	evt, _ := domain.NewEvent(
		uuid.New(),
		"rock-show",
		"Rock Show",
		"Description",
		"Stadium",
		time.Now().Add(48*time.Hour),
		time.Now().Add(2*time.Hour),
		nil,
	)

	err := uc.Execute(ctx, evt)
	if err != nil {
		t.Fatalf("unexpected error enqueuing: %v", err)
	}
	if len(outboxRepo.messages) != 1 {
		t.Fatalf("expected 1 message in outbox, got %d", len(outboxRepo.messages))
	}
	msg := outboxRepo.messages[0]
	if msg.routingKey != "ai.moderation.q" {
		t.Errorf("expected routing key ai.moderation.q, got %s", msg.routingKey)
	}
}
