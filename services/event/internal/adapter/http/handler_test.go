package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	adapterHTTP "github.com/eventflow/eventflow/services/event/internal/adapter/http"
	"github.com/eventflow/eventflow/services/event/internal/app"
	"github.com/eventflow/eventflow/services/event/internal/domain"
	"github.com/eventflow/eventflow/services/event/internal/port"
	"github.com/google/uuid"
)

type fakeRepo struct {
	failList bool
	events   map[uuid.UUID]*domain.Event
	tiers    map[uuid.UUID][]domain.TicketTier
	outbox   []port.OutboxMessage
}

func newTestRepo() *fakeRepo {
	return &fakeRepo{
		events: make(map[uuid.UUID]*domain.Event),
		tiers:  make(map[uuid.UUID][]domain.TicketTier),
	}
}

func (r *fakeRepo) Create(ctx context.Context, e *domain.Event) error {
	r.events[e.ID] = e
	return nil
}

func (r *fakeRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.Event, error) {
	e, ok := r.events[id]
	if !ok {
		return nil, domain.ErrEventNotFound
	}
	return e, nil
}

func (r *fakeRepo) GetBySlug(ctx context.Context, slug string) (*domain.Event, error) {
	for _, e := range r.events {
		if e.Slug == slug {
			return e, nil
		}
	}
	return nil, domain.ErrEventNotFound
}

func (r *fakeRepo) ListByOrganizer(ctx context.Context, orgID uuid.UUID) ([]*domain.Event, error) {
	if r.failList {
		return nil, errors.New("pq: connection refused to 10.0.0.5")
	}
	var list []*domain.Event
	for _, e := range r.events {
		if e.OrganizerID == orgID {
			list = append(list, e)
		}
	}
	return list, nil
}

func (r *fakeRepo) Update(ctx context.Context, e *domain.Event) error {
	r.events[e.ID] = e
	return nil
}

func (r *fakeRepo) UpdateWithOutbox(ctx context.Context, e *domain.Event, msgs ...port.OutboxMessage) error {
	r.events[e.ID] = e
	r.outbox = append(r.outbox, msgs...)
	return nil
}

func (r *fakeRepo) AddTier(ctx context.Context, t *domain.TicketTier) error {
	r.tiers[t.EventID] = append(r.tiers[t.EventID], *t)
	return nil
}

func (r *fakeRepo) UpdateTier(ctx context.Context, t *domain.TicketTier) error {
	list := r.tiers[t.EventID]
	for i, existing := range list {
		if existing.ID == t.ID {
			list[i] = *t
			r.tiers[t.EventID] = list
			return nil
		}
	}
	return domain.ErrTierNotFound
}

func (r *fakeRepo) GetTiersByEventID(ctx context.Context, eventID uuid.UUID) ([]domain.TicketTier, error) {
	return r.tiers[eventID], nil
}

// as gan danh tinh nhu gateway da xac minh token (EV-114).
func as(req *http.Request, id uuid.UUID, role string) *http.Request {
	req.Header.Set("X-Identity-Id", id.String())
	req.Header.Set("X-Identity-Role", role)
	return req
}

func setupTestServer() (http.Handler, *fakeRepo) {
	repo := newTestRepo()
	createUC := app.NewCreateEventUseCase(repo)
	updateUC := app.NewUpdateEventUseCase(repo)
	tiersUC := app.NewManageTiersUseCase(repo)
	transitionUC := app.NewTransitionStateUseCase(repo)
	getUC := app.NewGetEventUseCase(repo)

	h := adapterHTTP.NewEventHandler(createUC, updateUC, tiersUC, transitionUC, getUC)
	return adapterHTTP.NewRouter(h), repo
}

func TestHTTP_CreateAndGetEvent(t *testing.T) {
	handler, _ := setupTestServer()
	orgID := uuid.New()
	now := time.Now().UTC()

	reqBody := map[string]any{
		"organizer_id":  orgID.String(),
		"title":         "Rock Fest 2026",
		"starts_at":     now.Add(48 * time.Hour),
		"sale_start_at": now.Add(24 * time.Hour),
	}
	bodyBytes, _ := json.Marshal(reqBody)

	// POST /api/v1/events
	req := as(httptest.NewRequest("POST", "/api/v1/events", bytes.NewReader(bodyBytes)), orgID, "organizer")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created, got %d: %s", rec.Code, rec.Body.String())
	}

	var res adapterHTTP.EventResponse
	if err := json.NewDecoder(rec.Body).Decode(&res); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if res.Title != "Rock Fest 2026" || res.Status != domain.StatusDraft {
		t.Errorf("unexpected event response: %+v", res)
	}

	// GET /api/v1/events/{id}
	getReq := as(httptest.NewRequest("GET", "/api/v1/events/"+res.ID.String(), nil), orgID, "organizer")
	getRec := httptest.NewRecorder()
	handler.ServeHTTP(getRec, getReq)

	if getRec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", getRec.Code)
	}
}

func TestHTTP_AddTier_And_Submit_BR_E1(t *testing.T) {
	handler, repo := setupTestServer()
	orgID := uuid.New()
	now := time.Now().UTC()

	// 1. Tạo sự kiện
	createBody, _ := json.Marshal(map[string]any{
		"organizer_id":  orgID.String(),
		"title":         "Orchestra Night",
		"starts_at":     now.Add(48 * time.Hour),
		"sale_start_at": now.Add(24 * time.Hour),
	})
	rec := httptest.NewRecorder()
	createReq := as(httptest.NewRequest("POST", "/api/v1/events", bytes.NewReader(createBody)), orgID, "organizer")
	handler.ServeHTTP(rec, createReq)

	var evt adapterHTTP.EventResponse
	_ = json.NewDecoder(rec.Body).Decode(&evt)

	// 2. Submit khi chưa có hạng vé -> 422 Unprocessable Entity (BR-E1)
	submitBody, _ := json.Marshal(map[string]any{"organizer_id": orgID.String()})
	submitRec := httptest.NewRecorder()
	submitReq := as(httptest.NewRequest("POST", "/api/v1/events/"+evt.ID.String()+"/submit", bytes.NewReader(submitBody)), orgID, "organizer")
	handler.ServeHTTP(submitRec, submitReq)

	if submitRec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 for BR-E1 violation, got %d", submitRec.Code)
	}

	// 3. Thêm hạng vé
	tierBody, _ := json.Marshal(map[string]any{
		"organizer_id": orgID.String(),
		"name":         "Tier 1",
		"price_cents":  500000,
		"quota":        100,
	})
	tierRec := httptest.NewRecorder()
	tierReq := as(httptest.NewRequest("POST", "/api/v1/events/"+evt.ID.String()+"/tiers", bytes.NewReader(tierBody)), orgID, "organizer")
	handler.ServeHTTP(tierRec, tierReq)

	if tierRec.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created for tier, got %d", tierRec.Code)
	}

	// 4. Submit lại sau khi có vé -> 200 OK
	submitRec2 := httptest.NewRecorder()
	submitReq2 := as(httptest.NewRequest("POST", "/api/v1/events/"+evt.ID.String()+"/submit", bytes.NewReader(submitBody)), orgID, "organizer")
	handler.ServeHTTP(submitRec2, submitReq2)

	if submitRec2.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for valid submit, got %d", submitRec2.Code)
	}

	// 5. EV-120: submit hop le ghi DUNG MOT job kiem duyet; submit bi tu choi (buoc 2) khong ghi.
	if len(repo.outbox) != 1 || repo.outbox[0].RoutingKey != "moderation" {
		t.Fatalf("expected 1 moderation job in outbox, got %+v", repo.outbox)
	}
}

func TestHTTP_Healthz(t *testing.T) {
	handler, _ := setupTestServer()
	req := httptest.NewRequest("GET", "/healthz", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

type fakeModRepo struct {
	reviews map[uuid.UUID]*domain.ModerationReview
	byEvent map[uuid.UUID]*domain.ModerationReview
}

func (r *fakeModRepo) Create(ctx context.Context, review *domain.ModerationReview) error {
	r.reviews[review.ID] = review
	r.byEvent[review.EventID] = review
	return nil
}

func (r *fakeModRepo) GetLatestByEventID(ctx context.Context, eventID uuid.UUID) (*domain.ModerationReview, error) {
	rev, ok := r.byEvent[eventID]
	if !ok {
		return nil, domain.ErrModReviewNotFound
	}
	return rev, nil
}

func (r *fakeModRepo) ListPendingHuman(ctx context.Context) ([]*domain.ModerationReview, error) {
	var list []*domain.ModerationReview
	for _, rev := range r.byEvent {
		pending := rev.AiVerdict == domain.VerdictNeedsHuman || rev.AiVerdict == domain.VerdictAutoRejected
		if pending && rev.HumanVerdict == nil {
			list = append(list, rev)
		}
	}
	return list, nil
}

func (r *fakeModRepo) SetHumanDecision(ctx context.Context, reviewID uuid.UUID, humanID uuid.UUID, verdict, note string, decidedAt time.Time) error {
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

func setupModerationTestServer() (http.Handler, *fakeRepo, *fakeModRepo, *fakeAuditRepo) {
	repo := newTestRepo()
	modRepo := &fakeModRepo{
		reviews: make(map[uuid.UUID]*domain.ModerationReview),
		byEvent: make(map[uuid.UUID]*domain.ModerationReview),
	}
	auditRepo := &fakeAuditRepo{logs: make([]*domain.AuditLog, 0)}

	createUC := app.NewCreateEventUseCase(repo)
	updateUC := app.NewUpdateEventUseCase(repo)
	tiersUC := app.NewManageTiersUseCase(repo)
	transitionUC := app.NewTransitionStateUseCase(repo)
	getUC := app.NewGetEventUseCase(repo)

	aiVerdictUC := app.NewHandleAiVerdictUseCase(repo, modRepo, auditRepo)
	modQueueUC := app.NewListModerationQueueUseCase(repo, modRepo)
	lastDecisionStore = &fakeDecisionStore{repo: repo, mod: modRepo, audit: auditRepo}
	modActionUC := app.NewModeratorActionUseCase(repo, modRepo, lastDecisionStore)

	h := adapterHTTP.NewEventHandler(createUC, updateUC, tiersUC, transitionUC, getUC).
		WithModeration(aiVerdictUC, modQueueUC, modActionUC).
		WithModerationDetail(app.NewGetModerationDetailUseCase(repo, modRepo, auditRepo)).
		WithAudit(auditRepo)

	return adapterHTTP.NewRouter(h), repo, modRepo, auditRepo
}

func TestHTTP_AiVerdict_Callback(t *testing.T) {
	router, repo, _, _ := setupModerationTestServer()
	now := time.Now().UTC()

	evt, _ := domain.NewEvent(
		uuid.New(), "rock-concert", "Rock Concert", "desc", "hall",
		now.Add(24*time.Hour), now.Add(2*time.Hour), nil,
	)
	tier, _ := domain.NewTicketTier(evt.ID, "VIP", 200000, 50)
	_ = evt.AddTier(*tier)
	_ = evt.SubmitForReview()
	_ = repo.Create(context.Background(), evt)

	body, _ := json.Marshal(adapterHTTP.AiVerdictRequest{
		Score:   0.1,
		Verdict: domain.VerdictAutoApproved,
		Reasons: []string{"all clean"},
	})
	req := httptest.NewRequest("PATCH", "/internal/v1/events/"+evt.ID.String()+"/ai-verdict", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]string
	_ = json.NewDecoder(rec.Body).Decode(&resp)
	if resp["status"] != "SCHEDULED" {
		t.Errorf("expected SCHEDULED, got %s", resp["status"])
	}
}

func TestHTTP_ModerationQueue(t *testing.T) {
	router, repo, modRepo, _ := setupModerationTestServer()
	now := time.Now().UTC()

	evt, _ := domain.NewEvent(
		uuid.New(), "indie-gig", "Indie Gig", "desc", "bar",
		now.Add(24*time.Hour), now.Add(2*time.Hour), nil,
	)
	// Can hang ve de nop duyet hop le (BR-E1); thieu thi su kien o DRAFT va review khong con hieu luc.
	tier, _ := domain.NewTicketTier(evt.ID, "GA", 100000, 10)
	_ = evt.AddTier(*tier)
	if err := evt.SubmitForReview(); err != nil {
		t.Fatalf("submit: %v", err)
	}
	_ = repo.Create(context.Background(), evt)

	_ = modRepo.Create(context.Background(), &domain.ModerationReview{
		ID:        uuid.New(),
		EventID:   evt.ID,
		AiScore:   0.5,
		AiVerdict: domain.VerdictNeedsHuman,
		CreatedAt: now,
	})

	req := as(httptest.NewRequest("GET", "/api/v1/moderation/queue", nil), uuid.New(), "moderator")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var items []adapterHTTP.ModerationQueueItemResponse
	_ = json.NewDecoder(rec.Body).Decode(&items)
	if len(items) != 1 {
		t.Fatalf("expected 1 item in queue, got %d", len(items))
	}
	if items[0].EventID != evt.ID {
		t.Errorf("expected event id %s, got %s", evt.ID, items[0].EventID)
	}
}

func TestHTTP_ModeratorApprove_And_Reject(t *testing.T) {
	router, repo, modRepo, auditRepo := setupModerationTestServer()
	now := time.Now().UTC()

	evt, _ := domain.NewEvent(
		uuid.New(), "festival", "Festival", "desc", "park",
		now.Add(24*time.Hour), now.Add(2*time.Hour), nil,
	)
	tier, _ := domain.NewTicketTier(evt.ID, "General", 50000, 100)
	_ = evt.AddTier(*tier)
	_ = evt.SubmitForReview()
	_ = repo.Create(context.Background(), evt)

	_ = modRepo.Create(context.Background(), &domain.ModerationReview{
		ID:        uuid.New(),
		EventID:   evt.ID,
		AiScore:   0.6,
		AiVerdict: domain.VerdictNeedsHuman,
		CreatedAt: now,
	})

	// 1. Moderator Reject without reason -> 422 (BR-E5)
	modUUID := uuid.New()
	modID := modUUID.String()
	rejectNoReasonBody, _ := json.Marshal(adapterHTTP.ModeratorRejectRequest{
		ModeratorID: modID,
		Reason:      "   ",
	})
	rejReq1 := as(httptest.NewRequest("POST", "/api/v1/events/"+evt.ID.String()+"/reject", bytes.NewReader(rejectNoReasonBody)), modUUID, "moderator")
	rejRec1 := httptest.NewRecorder()
	router.ServeHTTP(rejRec1, rejReq1)
	if rejRec1.Code != http.StatusUnprocessableEntity {
		t.Errorf("expected 422 when rejecting without reason, got %d", rejRec1.Code)
	}

	// 2. Moderator Reject with valid reason -> 200 OK
	rejectValidBody, _ := json.Marshal(adapterHTTP.ModeratorRejectRequest{
		ModeratorID: modID,
		Reason:      "Permit not submitted",
	})
	rejReq2 := as(httptest.NewRequest("POST", "/api/v1/events/"+evt.ID.String()+"/reject", bytes.NewReader(rejectValidBody)), modUUID, "moderator")
	rejRec2 := httptest.NewRecorder()
	router.ServeHTTP(rejRec2, rejReq2)
	if rejRec2.Code != http.StatusOK {
		t.Errorf("expected 200 on valid reject, got %d", rejRec2.Code)
	}

	// 3. Moderator Override: Approve previously REJECTED event -> 200 OK
	approveBody, _ := json.Marshal(adapterHTTP.ModeratorApproveRequest{
		ModeratorID: modID,
		Note:        "Organizer submitted permit later, approved override",
	})
	appReq := as(httptest.NewRequest("POST", "/api/v1/events/"+evt.ID.String()+"/approve", bytes.NewReader(approveBody)), modUUID, "moderator")
	appRec := httptest.NewRecorder()
	router.ServeHTTP(appRec, appReq)
	if appRec.Code != http.StatusOK {
		t.Errorf("expected 200 on moderator approve override, got %d", appRec.Code)
	}

	// Verify audit logs were written for both actions
	logs, _ := auditRepo.ListBySubject(context.Background(), "event:"+evt.ID.String())
	if len(logs) < 2 {
		t.Errorf("expected at least 2 audit logs recorded, got %d", len(logs))
	}
}
