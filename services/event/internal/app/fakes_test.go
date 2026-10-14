package app_test

import (
	"context"
	"time"

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

type fakeModerationRepo struct {
	reviews   map[uuid.UUID]*domain.ModerationReview
	byEvent   map[uuid.UUID]*domain.ModerationReview
	createErr error
}

func newFakeModerationRepo() *fakeModerationRepo {
	return &fakeModerationRepo{
		reviews: make(map[uuid.UUID]*domain.ModerationReview),
		byEvent: make(map[uuid.UUID]*domain.ModerationReview),
	}
}

func (r *fakeModerationRepo) Create(ctx context.Context, review *domain.ModerationReview) error {
	if r.createErr != nil {
		return r.createErr
	}
	r.reviews[review.ID] = review
	r.byEvent[review.EventID] = review
	return nil
}

func (r *fakeModerationRepo) GetLatestByEventID(ctx context.Context, eventID uuid.UUID) (*domain.ModerationReview, error) {
	rev, ok := r.byEvent[eventID]
	if !ok {
		return nil, domain.ErrModReviewNotFound
	}
	return rev, nil
}

func (r *fakeModerationRepo) ListNeedsHuman(ctx context.Context) ([]*domain.ModerationReview, error) {
	var list []*domain.ModerationReview
	for _, rev := range r.reviews {
		if rev.AiVerdict == domain.VerdictNeedsHuman && rev.HumanVerdict == nil {
			list = append(list, rev)
		}
	}
	return list, nil
}

func (r *fakeModerationRepo) SetHumanDecision(ctx context.Context, reviewID uuid.UUID, humanID uuid.UUID, verdict, note string, decidedAt time.Time) error {
	rev, ok := r.reviews[reviewID]
	if !ok {
		return domain.ErrModReviewNotFound
	}
	rev.HumanID = &humanID
	rev.HumanVerdict = &verdict
	rev.HumanNote = &note
	rev.DecidedAt = &decidedAt
	return nil
}

type fakeAuditRepo struct {
	logs []*domain.AuditLog
}

func newFakeAuditRepo() *fakeAuditRepo {
	return &fakeAuditRepo{logs: make([]*domain.AuditLog, 0)}
}

func (r *fakeAuditRepo) Create(ctx context.Context, log *domain.AuditLog) error {
	r.logs = append(r.logs, log)
	return nil
}

func (r *fakeAuditRepo) ListBySubject(ctx context.Context, subject string) ([]*domain.AuditLog, error) {
	var list []*domain.AuditLog
	for _, l := range r.logs {
		if l.Subject == subject {
			list = append(list, l)
		}
	}
	return list, nil
}

type fakeOutboxRepo struct {
	messages []struct {
		aggregateType string
		aggregateID   uuid.UUID
		routingKey    string
		payload       any
	}
}

func newFakeOutboxRepo() *fakeOutboxRepo {
	return &fakeOutboxRepo{}
}

func (r *fakeOutboxRepo) Publish(ctx context.Context, aggregateType string, aggregateID uuid.UUID, routingKey string, payload any) error {
	r.messages = append(r.messages, struct {
		aggregateType string
		aggregateID   uuid.UUID
		routingKey    string
		payload       any
	}{
		aggregateType: aggregateType,
		aggregateID:   aggregateID,
		routingKey:    routingKey,
		payload:       payload,
	})
	return nil
}
