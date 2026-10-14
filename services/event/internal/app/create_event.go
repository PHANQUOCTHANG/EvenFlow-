package app

import (
	"context"
	"time"

	"github.com/eventflow/eventflow/services/event/internal/domain"
	"github.com/eventflow/eventflow/services/event/internal/port"
	"github.com/google/uuid"
)

type CreateEventInput struct {
	OrganizerID uuid.UUID  `json:"organizer_id"`
	Slug        string     `json:"slug"`
	Title       string     `json:"title"`
	Description string     `json:"description"`
	Venue       string     `json:"venue"`
	StartsAt    time.Time  `json:"starts_at"`
	SaleStartAt time.Time  `json:"sale_start_at"`
	SaleEndAt   *time.Time `json:"sale_end_at,omitempty"`
}

type CreateEventUseCase interface {
	Execute(ctx context.Context, input CreateEventInput) (*domain.Event, error)
}

type createEventUseCase struct {
	repo port.EventRepository
}

func NewCreateEventUseCase(repo port.EventRepository) CreateEventUseCase {
	return &createEventUseCase{repo: repo}
}

func (uc *createEventUseCase) Execute(ctx context.Context, input CreateEventInput) (*domain.Event, error) {
	evt, err := domain.NewEvent(
		input.OrganizerID,
		input.Slug,
		input.Title,
		input.Description,
		input.Venue,
		input.StartsAt,
		input.SaleStartAt,
		input.SaleEndAt,
	)
	if err != nil {
		return nil, err
	}

	if err := uc.repo.Create(ctx, evt); err != nil {
		return nil, err
	}

	return evt, nil
}
