package app

import (
	"context"
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

	reviews, err := uc.modRepo.ListNeedsHuman(ctx)
	if err != nil {
		return nil, err
	}

	items := make([]*ModerationQueueItem, 0, len(reviews))
	for _, rev := range reviews {
		evt, err := uc.eventRepo.GetByID(ctx, rev.EventID)
		if err != nil {
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
