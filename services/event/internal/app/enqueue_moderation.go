package app

import (
	"context"
	"time"

	"github.com/eventflow/eventflow/services/event/internal/domain"
	"github.com/eventflow/eventflow/services/event/internal/port"
	"github.com/google/uuid"
)

// EnqueueModerationUseCase day job kiem duyet AI vao transactional outbox (EV-120).
type EnqueueModerationUseCase interface {
	Execute(ctx context.Context, evt *domain.Event) error
}

type enqueueModerationUseCase struct {
	outboxRepo port.OutboxRepository
}

func NewEnqueueModerationUseCase(outboxRepo port.OutboxRepository) EnqueueModerationUseCase {
	return &enqueueModerationUseCase{outboxRepo: outboxRepo}
}

func (uc *enqueueModerationUseCase) Execute(ctx context.Context, evt *domain.Event) error {
	if uc.outboxRepo == nil || evt == nil {
		return nil
	}

	payload := map[string]any{
		"job_id":            uuid.New().String(),
		"event_id":          evt.ID.String(),
		"event_title":       evt.Title,
		"event_description": evt.Description,
		"enqueued_at":       float64(time.Now().UnixNano()) / 1e9,
	}

	return uc.outboxRepo.Publish(ctx, "event", evt.ID, "ai.moderation.q", payload)
}
