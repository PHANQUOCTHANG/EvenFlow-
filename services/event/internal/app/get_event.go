package app

import (
	"context"

	"github.com/eventflow/eventflow/services/event/internal/domain"
	"github.com/eventflow/eventflow/services/event/internal/port"
	"github.com/google/uuid"
)

type GetEventUseCase interface {
	GetByID(ctx context.Context, id uuid.UUID) (*domain.Event, error)
	GetBySlug(ctx context.Context, slug string) (*domain.Event, error)
	ListByOrganizer(ctx context.Context, organizerID uuid.UUID) ([]*domain.Event, error)
}

type getEventUseCase struct {
	repo port.EventRepository
}

func NewGetEventUseCase(repo port.EventRepository) GetEventUseCase {
	return &getEventUseCase{repo: repo}
}

func (uc *getEventUseCase) GetByID(ctx context.Context, id uuid.UUID) (*domain.Event, error) {
	evt, err := uc.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	tiers, _ := uc.repo.GetTiersByEventID(ctx, id)
	evt.Tiers = tiers
	return evt, nil
}

func (uc *getEventUseCase) GetBySlug(ctx context.Context, slug string) (*domain.Event, error) {
	evt, err := uc.repo.GetBySlug(ctx, slug)
	if err != nil {
		return nil, err
	}

	tiers, _ := uc.repo.GetTiersByEventID(ctx, evt.ID)
	evt.Tiers = tiers
	return evt, nil
}

func (uc *getEventUseCase) ListByOrganizer(ctx context.Context, organizerID uuid.UUID) ([]*domain.Event, error) {
	return uc.repo.ListByOrganizer(ctx, organizerID)
}
