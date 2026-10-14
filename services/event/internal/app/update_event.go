package app

import (
	"context"
	"time"

	"github.com/eventflow/eventflow/services/event/internal/domain"
	"github.com/eventflow/eventflow/services/event/internal/port"
	"github.com/google/uuid"
)

type UpdateEventInput struct {
	EventID     uuid.UUID  `json:"event_id"`
	OrganizerID uuid.UUID  `json:"organizer_id"`
	Title       string     `json:"title"`
	Description string     `json:"description"`
	Venue       string     `json:"venue"`
	StartsAt    time.Time  `json:"starts_at"`
	SaleStartAt time.Time  `json:"sale_start_at"`
	SaleEndAt   *time.Time `json:"sale_end_at,omitempty"`
}

type UpdateEventUseCase interface {
	Execute(ctx context.Context, input UpdateEventInput) (*domain.Event, error)
}

type updateEventUseCase struct {
	repo port.EventRepository
}

func NewUpdateEventUseCase(repo port.EventRepository) UpdateEventUseCase {
	return &updateEventUseCase{repo: repo}
}

func (uc *updateEventUseCase) Execute(ctx context.Context, input UpdateEventInput) (*domain.Event, error) {
	evt, err := uc.repo.GetByID(ctx, input.EventID)
	if err != nil {
		return nil, err
	}

	if evt.OrganizerID != input.OrganizerID {
		return nil, domain.ErrUnauthorizedOrganizer
	}

	if err := evt.UpdateDetails(
		input.Title,
		input.Description,
		input.Venue,
		input.StartsAt,
		input.SaleStartAt,
		input.SaleEndAt,
	); err != nil {
		return nil, err
	}

	if err := uc.repo.Update(ctx, evt); err != nil {
		return nil, err
	}

	return evt, nil
}
