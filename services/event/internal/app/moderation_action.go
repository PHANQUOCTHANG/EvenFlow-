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

type ModeratorApproveInput struct {
	EventID     uuid.UUID
	ModeratorID uuid.UUID
	Note        string
}

type ModeratorRejectInput struct {
	EventID     uuid.UUID
	ModeratorID uuid.UUID
	Reason      string // Bat buoc theo BR-E5
}

type ModeratorActionUseCase interface {
	Approve(ctx context.Context, input ModeratorApproveInput) (*domain.Event, error)
	Reject(ctx context.Context, input ModeratorRejectInput) (*domain.Event, error)
}

type moderatorActionUseCase struct {
	eventRepo port.EventRepository
	modRepo   port.ModerationRepository
	auditRepo port.AuditLogRepository
}

func NewModeratorActionUseCase(
	eventRepo port.EventRepository,
	modRepo port.ModerationRepository,
	auditRepo port.AuditLogRepository,
) ModeratorActionUseCase {
	return &moderatorActionUseCase{
		eventRepo: eventRepo,
		modRepo:   modRepo,
		auditRepo: auditRepo,
	}
}

func (uc *moderatorActionUseCase) Approve(ctx context.Context, input ModeratorApproveInput) (*domain.Event, error) {
	evt, err := uc.eventRepo.GetByID(ctx, input.EventID)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	// ApproveOverride cho phep Moderator lat nguoc ca REJECTED va PENDING_REVIEW -> SCHEDULED (EV-121)
	if err := evt.ApproveOverride(now); err != nil {
		return nil, err
	}

	if err := uc.eventRepo.Update(ctx, evt); err != nil {
		return nil, err
	}

	if uc.modRepo != nil {
		rev, _ := uc.modRepo.GetLatestByEventID(ctx, evt.ID)
		if rev != nil {
			_ = uc.modRepo.SetHumanDecision(ctx, rev.ID, input.ModeratorID, "APPROVED", input.Note, now)
		}
	}

	if uc.auditRepo != nil {
		actorID := input.ModeratorID.String()
		reason := input.Note
		_ = uc.auditRepo.Create(ctx, &domain.AuditLog{
			ActorType: "human",
			ActorID:   &actorID,
			Action:    "event.moderator_approved",
			Subject:   fmt.Sprintf("event:%s", evt.ID),
			Reason:    &reason,
			CreatedAt: now,
		})
	}

	return evt, nil
}

func (uc *moderatorActionUseCase) Reject(ctx context.Context, input ModeratorRejectInput) (*domain.Event, error) {
	trimmedReason := strings.TrimSpace(input.Reason)
	if trimmedReason == "" {
		return nil, domain.ErrRejectReasonRequired
	}

	evt, err := uc.eventRepo.GetByID(ctx, input.EventID)
	if err != nil {
		return nil, err
	}

	if err := evt.Reject(); err != nil {
		return nil, err
	}

	if err := uc.eventRepo.Update(ctx, evt); err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	if uc.modRepo != nil {
		rev, _ := uc.modRepo.GetLatestByEventID(ctx, evt.ID)
		if rev != nil {
			_ = uc.modRepo.SetHumanDecision(ctx, rev.ID, input.ModeratorID, "REJECTED", trimmedReason, now)
		}
	}

	if uc.auditRepo != nil {
		actorID := input.ModeratorID.String()
		_ = uc.auditRepo.Create(ctx, &domain.AuditLog{
			ActorType: "human",
			ActorID:   &actorID,
			Action:    "event.moderator_rejected",
			Subject:   fmt.Sprintf("event:%s", evt.ID),
			Reason:    &trimmedReason,
			CreatedAt: now,
		})
	}

	return evt, nil
}
