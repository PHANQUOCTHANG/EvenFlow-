package app

import (
	"context"
	"errors"
	"time"

	"github.com/eventflow/eventflow/services/event/internal/domain"
	"github.com/eventflow/eventflow/services/event/internal/port"
	"github.com/google/uuid"
)

type ModerationQueueItem struct {
	EventID     uuid.UUID                `json:"event_id"`
	EventTitle  string                   `json:"event_title"`
	EventStatus domain.EventStatus       `json:"event_status"`
	Venue       string                   `json:"venue"`
	StartsAt    time.Time                `json:"starts_at"`
	SaleStartAt time.Time                `json:"sale_start_at"`
	Review      *domain.ModerationReview `json:"review,omitempty"`
}

type ListModerationQueueUseCase interface {
	Execute(ctx context.Context) ([]*ModerationQueueItem, error)
}

type listModerationQueueUseCase struct {
	eventRepo port.EventRepository
	modRepo   port.ModerationRepository
}

func NewListModerationQueueUseCase(
	eventRepo port.EventRepository,
	modRepo port.ModerationRepository,
) ListModerationQueueUseCase {
	return &listModerationQueueUseCase{
		eventRepo: eventRepo,
		modRepo:   modRepo,
	}
}

func (uc *listModerationQueueUseCase) Execute(ctx context.Context) ([]*ModerationQueueItem, error) {
	if uc.modRepo == nil {
		return []*ModerationQueueItem{}, nil
	}

	reviews, err := uc.modRepo.ListPendingHuman(ctx)
	if err != nil {
		return nil, err
	}

	items := make([]*ModerationQueueItem, 0, len(reviews))
	for _, rev := range reviews {
		evt, err := uc.eventRepo.GetByID(ctx, rev.EventID)
		if errors.Is(err, domain.ErrEventNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		// Review chi con hieu luc khi su kien van o trang thai ma AI de lai: BTC sua va nop lai
		// (ve DRAFT/PENDING moi) thi review cu khong con la viec cua Moderator.
		if !reviewStillActionable(rev.AiVerdict, evt.Status) {
			continue
		}
		items = append(items, &ModerationQueueItem{
			EventID:     evt.ID,
			EventTitle:  evt.Title,
			EventStatus: evt.Status,
			Venue:       evt.Venue,
			StartsAt:    evt.StartsAt,
			SaleStartAt: evt.SaleStartAt,
			Review:      rev,
		})
	}

	return items, nil
}

func reviewStillActionable(v domain.AiVerdict, status domain.EventStatus) bool {
	switch v {
	case domain.VerdictNeedsHuman:
		return status == domain.StatusPendingReview
	case domain.VerdictAutoRejected:
		return status == domain.StatusRejected
	default:
		return false
	}
}
