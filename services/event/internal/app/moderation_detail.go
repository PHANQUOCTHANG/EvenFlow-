package app

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/eventflow/eventflow/services/event/internal/domain"
	"github.com/eventflow/eventflow/services/event/internal/port"
	"github.com/google/uuid"
)

// ModerationDetail la ho so tham dinh cua Moderator (EV-121): noi dung goc cua BTC, ket qua AI va
// lich su audit. AiAxes doc tu metadata cua audit AI moi nhat -- moderation_reviews chua co cot
// truc (them cot can migration, ghi no o spec).
type ModerationDetail struct {
	Event  *domain.Event
	Review *domain.ModerationReview
	AiAxes map[string]float64
	Audit  []*domain.AuditLog
}

type GetModerationDetailUseCase interface {
	Execute(ctx context.Context, eventID uuid.UUID) (*ModerationDetail, error)
}

type getModerationDetailUseCase struct {
	eventRepo port.EventRepository
	modRepo   port.ModerationRepository
	auditRepo port.AuditLogRepository
}

func NewGetModerationDetailUseCase(
	eventRepo port.EventRepository,
	modRepo port.ModerationRepository,
	auditRepo port.AuditLogRepository,
) GetModerationDetailUseCase {
	return &getModerationDetailUseCase{eventRepo: eventRepo, modRepo: modRepo, auditRepo: auditRepo}
}

func (uc *getModerationDetailUseCase) Execute(ctx context.Context, eventID uuid.UUID) (*ModerationDetail, error) {
	evt, err := uc.eventRepo.GetByID(ctx, eventID)
	if err != nil {
		return nil, err
	}
	if len(evt.Tiers) == 0 {
		tiers, err := uc.eventRepo.GetTiersByEventID(ctx, eventID)
		if err != nil {
			return nil, err
		}
		evt.Tiers = tiers
	}

	d := &ModerationDetail{Event: evt, AiAxes: map[string]float64{}, Audit: []*domain.AuditLog{}}

	if uc.modRepo != nil {
		rev, err := uc.modRepo.GetLatestByEventID(ctx, eventID)
		switch {
		case errors.Is(err, domain.ErrModReviewNotFound):
		case err != nil:
			return nil, err
		default:
			d.Review = rev
		}
	}

	if uc.auditRepo != nil {
		logs, err := uc.auditRepo.ListBySubject(ctx, fmt.Sprintf("event:%s", eventID))
		if err != nil {
			return nil, err
		}
		// Sap tang dan theo thoi gian de UI ve dong thoi gian; khong phu thuoc thu tu cua repo.
		sort.SliceStable(logs, func(i, j int) bool { return logs[i].CreatedAt.Before(logs[j].CreatedAt) })
		d.Audit = logs
		d.AiAxes = latestAIAxes(logs)
	}
	return d, nil
}

// latestAIAxes lay truc cua ban ghi AI MOI NHAT co metadata.axes (logs da sap tang dan).
func latestAIAxes(logs []*domain.AuditLog) map[string]float64 {
	for i := len(logs) - 1; i >= 0; i-- {
		l := logs[i]
		if l.ActorType != "ai" {
			continue
		}
		meta, ok := l.Metadata.(map[string]any)
		if !ok {
			continue
		}
		raw, ok := meta["axes"].(map[string]any)
		if !ok || len(raw) == 0 {
			continue
		}
		out := make(map[string]float64, len(raw))
		for k, v := range raw {
			if f, ok := v.(float64); ok {
				out[k] = f
			}
		}
		return out
	}
	return map[string]float64{}
}
