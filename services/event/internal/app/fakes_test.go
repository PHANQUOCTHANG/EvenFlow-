package app_test

import (
	"context"

	"github.com/eventflow/eventflow/services/event/internal/domain"
	"github.com/google/uuid"
)

type fakeEventRepo struct {
	events    map[uuid.UUID]*domain.Event
	tiers     map[uuid.UUID][]domain.TicketTier
	createErr error
	getErr    error
	updateErr error
}

func newFakeRepo() *fakeEventRepo {
	return &fakeEventRepo{
		events: make(map[uuid.UUID]*domain.Event),
		tiers:  make(map[uuid.UUID][]domain.TicketTier),
	}
}

func (r *fakeEventRepo) Create(ctx context.Context, event *domain.Event) error {
	if r.createErr != nil {
		return r.createErr
	}
	r.events[event.ID] = event
	return nil
}

func (r *fakeEventRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.Event, error) {
	if r.getErr != nil {
		return nil, r.getErr
	}
	evt, ok := r.events[id]
	if !ok {
		return nil, domain.ErrEventNotFound
	}
	return evt, nil
}

func (r *fakeEventRepo) GetBySlug(ctx context.Context, slug string) (*domain.Event, error) {
	if r.getErr != nil {
		return nil, r.getErr
	}
	for _, evt := range r.events {
		if evt.Slug == slug {
			return evt, nil
		}
	}
	return nil, domain.ErrEventNotFound
}

func (r *fakeEventRepo) ListByOrganizer(ctx context.Context, organizerID uuid.UUID) ([]*domain.Event, error) {
	var list []*domain.Event
	for _, evt := range r.events {
		if evt.OrganizerID == organizerID {
			list = append(list, evt)
		}
	}
	return list, nil
}

func (r *fakeEventRepo) Update(ctx context.Context, event *domain.Event) error {
	if r.updateErr != nil {
		return r.updateErr
	}
	r.events[event.ID] = event
	return nil
}

func (r *fakeEventRepo) AddTier(ctx context.Context, tier *domain.TicketTier) error {
	r.tiers[tier.EventID] = append(r.tiers[tier.EventID], *tier)
	return nil
}

func (r *fakeEventRepo) UpdateTier(ctx context.Context, tier *domain.TicketTier) error {
	tierList := r.tiers[tier.EventID]
	for i, t := range tierList {
		if t.ID == tier.ID {
			tierList[i] = *tier
			r.tiers[tier.EventID] = tierList
			return nil
		}
	}
	return domain.ErrTierNotFound
}

func (r *fakeEventRepo) GetTiersByEventID(ctx context.Context, eventID uuid.UUID) ([]domain.TicketTier, error) {
	return r.tiers[eventID], nil
}
