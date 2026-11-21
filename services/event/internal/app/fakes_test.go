package app_test

import (
	"context"
	"time"

	"github.com/eventflow/eventflow/services/event/internal/domain"
	"github.com/eventflow/eventflow/services/event/internal/port"
	"github.com/google/uuid"
)

type fakeEventRepo struct {
	events    map[uuid.UUID]*domain.Event
	tiers     map[uuid.UUID][]domain.TicketTier
	createErr error
	getErr    error
	updateErr error
	outbox    []port.OutboxMessage
	statuses  map[uuid.UUID]domain.EventStatus
}

func newFakeRepo() *fakeEventRepo {
	return &fakeEventRepo{
		events:   make(map[uuid.UUID]*domain.Event),
		tiers:    make(map[uuid.UUID][]domain.TicketTier),
		statuses: make(map[uuid.UUID]domain.EventStatus),
	}
}

func (r *fakeEventRepo) Create(ctx context.Context, event *domain.Event) error {
	if r.createErr != nil {
		return r.createErr
	}
	r.events[event.ID] = event
	r.statuses[event.ID] = event.Status
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

// committed tra trang thai da "commit" qua Create/Update/ApplyDecision. Repo gia luu con tro nen
// use case sua su kien trong bo nho cung lam doi `events`; trang thai that nam o day.
func (r *fakeEventRepo) committed(id uuid.UUID) domain.EventStatus {
	return r.statuses[id]
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
	r.statuses[event.ID] = event.Status
	return nil
}

// UpdateWithOutbox mo phong mot transaction: loi thi KHONG ghi gi ca (ca su kien lan outbox).
func (r *fakeEventRepo) UpdateWithOutbox(ctx context.Context, event *domain.Event, msgs ...port.OutboxMessage) error {
	if r.updateErr != nil {
		return r.updateErr
	}
	r.events[event.ID] = event
	r.statuses[event.ID] = event.Status
	r.outbox = append(r.outbox, msgs...)
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

// ListPendingHuman: review MOI NHAT cua moi su kien, AI NEEDS_HUMAN hoac AUTO_REJECTED, chua co quyet dinh nguoi.
func (r *fakeModerationRepo) ListPendingHuman(ctx context.Context) ([]*domain.ModerationReview, error) {
	var list []*domain.ModerationReview
	for _, rev := range r.byEvent {
		pending := rev.AiVerdict == domain.VerdictNeedsHuman || rev.AiVerdict == domain.VerdictAutoRejected
		if pending && rev.HumanVerdict == nil {
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

// fakeDecisionStore mo phong MOT transaction: kiem dieu kien truoc, loi thi khong ghi gi.
type fakeDecisionStore struct {
	events *fakeEventRepo
	mod    *fakeModerationRepo
	audit  *fakeAuditRepo
	err    error
	calls  []port.ModerationDecision
}

func newFakeDecisionStore(e *fakeEventRepo, m *fakeModerationRepo, a *fakeAuditRepo) *fakeDecisionStore {
	return &fakeDecisionStore{events: e, mod: m, audit: a}
}

func (s *fakeDecisionStore) ApplyDecision(ctx context.Context, d port.ModerationDecision) error {
	s.calls = append(s.calls, d)
	if s.err != nil {
		return s.err
	}
	if stored, ok := s.events.statuses[d.Event.ID]; ok && stored != d.PrevStatus {
		return domain.ErrAlreadyDecided
	}
	if d.ReviewID != nil {
		rev, ok := s.mod.reviews[*d.ReviewID]
		if !ok {
			return domain.ErrModReviewNotFound
		}
		verdict, note, at, human := d.Verdict, d.Note, d.DecidedAt, d.HumanID
		rev.HumanVerdict, rev.HumanNote, rev.DecidedAt, rev.HumanID = &verdict, &note, &at, &human
	}
	s.events.events[d.Event.ID] = d.Event
	s.events.statuses[d.Event.ID] = d.Event.Status
	s.audit.logs = append(s.audit.logs, d.Audit)
	return nil
}
