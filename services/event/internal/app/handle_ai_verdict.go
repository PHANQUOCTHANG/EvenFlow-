package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/eventflow/eventflow/services/event/internal/domain"
	"github.com/eventflow/eventflow/services/event/internal/port"
	"github.com/google/uuid"
)

type HandleAiVerdictInput struct {
	EventID uuid.UUID
	Score   float64
	Verdict domain.AiVerdict
	Reasons []string
	Axes    map[string]float64
}

type HandleAiVerdictUseCase interface {
	Execute(ctx context.Context, input HandleAiVerdictInput) (*domain.Event, *domain.ModerationReview, error)
}

type handleAiVerdictUseCase struct {
	eventRepo port.EventRepository
	modRepo   port.ModerationRepository
	auditRepo port.AuditLogRepository
}

func NewHandleAiVerdictUseCase(
	eventRepo port.EventRepository,
	modRepo port.ModerationRepository,
	auditRepo port.AuditLogRepository,
) HandleAiVerdictUseCase {
	return &handleAiVerdictUseCase{
		eventRepo: eventRepo,
		modRepo:   modRepo,
		auditRepo: auditRepo,
	}
}

func (uc *handleAiVerdictUseCase) Execute(ctx context.Context, input HandleAiVerdictInput) (*domain.Event, *domain.ModerationReview, error) {
	if input.Score < 0.0 || input.Score > 1.0 {
		return nil, nil, domain.ErrInvalidAiScore
	}

	evt, err := uc.eventRepo.GetByID(ctx, input.EventID)
	if err != nil {
		return nil, nil, err
	}

	// Suy dien verdict dua tren BR-E4 neu chua chi dinh ro
	verdict := input.Verdict
	if verdict == "" {
		switch {
		case input.Score < 0.3:
			verdict = domain.VerdictAutoApproved
		case input.Score <= 0.8:
			verdict = domain.VerdictNeedsHuman
		default:
			verdict = domain.VerdictAutoRejected
		}
	}

	now := time.Now().UTC()
	review := &domain.ModerationReview{
		ID:        uuid.New(),
		EventID:   input.EventID,
		AiScore:   input.Score,
		AiVerdict: verdict,
		AiReasons: input.Reasons,
		CreatedAt: now,
	}

	if uc.modRepo != nil {
		if err := uc.modRepo.Create(ctx, review); err != nil {
			return nil, nil, err
		}
	}

	metadata := map[string]any{
		"score":   input.Score,
		"reasons": input.Reasons,
		"axes":    input.Axes,
	}

	switch verdict {
	case domain.VerdictAutoApproved:
		if err := evt.Approve(now); err != nil {
			return nil, nil, err
		}
		if err := uc.eventRepo.Update(ctx, evt); err != nil {
			return nil, nil, err
		}
		if uc.auditRepo != nil {
			_ = uc.auditRepo.Create(ctx, &domain.AuditLog{
				ActorType: "ai",
				Action:    "event.auto_approved",
				Subject:   fmt.Sprintf("event:%s", evt.ID),
				Metadata:  metadata,
				CreatedAt: now,
			})
		}

	case domain.VerdictAutoRejected:
		if err := evt.Reject(); err != nil {
			return nil, nil, err
		}
		if err := uc.eventRepo.Update(ctx, evt); err != nil {
			return nil, nil, err
		}
		reasonStr := strings.Join(input.Reasons, "; ")
		if uc.auditRepo != nil {
			_ = uc.auditRepo.Create(ctx, &domain.AuditLog{
				ActorType: "ai",
				Action:    "event.auto_rejected",
				Subject:   fmt.Sprintf("event:%s", evt.ID),
				Reason:    &reasonStr,
				Metadata:  metadata,
				CreatedAt: now,
			})
		}

	case domain.VerdictNeedsHuman:
		// Su kien giu nguyen PENDING_REVIEW cho Moderator xu ly (EV-121)
		if uc.auditRepo != nil {
			_ = uc.auditRepo.Create(ctx, &domain.AuditLog{
				ActorType: "ai",
				Action:    "event.needs_human_review",
				Subject:   fmt.Sprintf("event:%s", evt.ID),
				Metadata:  metadata,
				CreatedAt: now,
			})
		}
	}

	return evt, review, nil
}
