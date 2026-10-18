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

// EV-120: nop duyet phai ghi job kiem duyet vao outbox TRONG CUNG transaction voi viec
// chuyen PENDING_REVIEW. Truoc day handler goi rieng va nuot loi: ghi outbox hong thi su
// kien van PENDING_REVIEW ma khong co job nao -> ket mai. Routing key cu "ai.moderation.q"
// (test cu khang dinh dung key sai nay) khong khop binding "moderation" cua ai.direct.
func newSubmittableEvent(t *testing.T, repo *fakeEventRepo) (*domain.Event, uuid.UUID) {
	t.Helper()
	organizerID := uuid.New()
	evt, err := domain.NewEvent(organizerID, "rock-show", "Rock Show", "Mo ta", "Stadium",
		time.Now().Add(48*time.Hour), time.Now().Add(2*time.Hour), nil)
	if err != nil {
		t.Fatalf("NewEvent: %v", err)
	}
	repo.events[evt.ID] = evt
	repo.tiers[evt.ID] = []domain.TicketTier{{ID: uuid.New(), EventID: evt.ID, Name: "GA", PriceCents: 1000, Quota: 10}}
	return evt, organizerID
}

func TestSubmitForReview_GhiJobKiemDuyetCungTransaction(t *testing.T) {
	repo := newFakeRepo()
	evt, organizerID := newSubmittableEvent(t, repo)

	submitted, err := app.NewTransitionStateUseCase(repo).SubmitForReview(context.Background(), evt.ID, organizerID)
	if err != nil {
		t.Fatalf("SubmitForReview: %v", err)
	}
	if submitted.Status != domain.StatusPendingReview {
		t.Fatalf("expected PENDING_REVIEW, got %s", submitted.Status)
	}
	if len(repo.outbox) != 1 {
		t.Fatalf("expected 1 outbox message, got %d", len(repo.outbox))
	}
	msg := repo.outbox[0]
	if msg.RoutingKey != "moderation" {
		t.Errorf("routing key phai khop binding ai.direct 'moderation', got %q", msg.RoutingKey)
	}
	if msg.AggregateType != "event" || msg.AggregateID != evt.ID {
		t.Errorf("aggregate sai: %s/%s", msg.AggregateType, msg.AggregateID)
	}
	payload, ok := msg.Payload.(map[string]any)
	if !ok {
		t.Fatalf("payload kieu %T", msg.Payload)
	}
	for _, k := range []string{"job_id", "event_id", "event_title", "event_description", "enqueued_at"} {
		if _, ok := payload[k]; !ok {
			t.Errorf("payload thieu %q (hop dong voi ai-worker ModerationJob)", k)
		}
	}
	if payload["event_id"] != evt.ID.String() {
		t.Errorf("event_id sai: %v", payload["event_id"])
	}
}

func TestSubmitForReview_GhiLoiThiTraLoiVaKhongCoJob(t *testing.T) {
	repo := newFakeRepo()
	evt, organizerID := newSubmittableEvent(t, repo)
	repo.updateErr = errors.New("db sap")

	_, err := app.NewTransitionStateUseCase(repo).SubmitForReview(context.Background(), evt.ID, organizerID)
	if err == nil {
		t.Fatal("ghi that bai phai tra loi cho nguoi goi, khong duoc nuot")
	}
	if len(repo.outbox) != 0 {
		t.Fatalf("khong duoc co job khi transaction hong, got %d", len(repo.outbox))
	}
}
