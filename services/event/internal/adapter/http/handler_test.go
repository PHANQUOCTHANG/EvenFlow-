package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	adapterHTTP "github.com/eventflow/eventflow/services/event/internal/adapter/http"
	"github.com/eventflow/eventflow/services/event/internal/app"
	"github.com/eventflow/eventflow/services/event/internal/domain"
	"github.com/google/uuid"
)

type fakeRepo struct {
	events map[uuid.UUID]*domain.Event
	tiers  map[uuid.UUID][]domain.TicketTier
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
	req := httptest.NewRequest("POST", "/api/v1/events", bytes.NewReader(bodyBytes))
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
	getReq := httptest.NewRequest("GET", "/api/v1/events/"+res.ID.String(), nil)
	getRec := httptest.NewRecorder()
	handler.ServeHTTP(getRec, getReq)

	if getRec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", getRec.Code)
	}
}

func TestHTTP_AddTier_And_Submit_BR_E1(t *testing.T) {
	handler, _ := setupTestServer()
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
	createReq := httptest.NewRequest("POST", "/api/v1/events", bytes.NewReader(createBody))
	handler.ServeHTTP(rec, createReq)

	var evt adapterHTTP.EventResponse
	_ = json.NewDecoder(rec.Body).Decode(&evt)

	// 2. Submit khi chưa có hạng vé -> 422 Unprocessable Entity (BR-E1)
	submitBody, _ := json.Marshal(map[string]any{"organizer_id": orgID.String()})
	submitRec := httptest.NewRecorder()
	submitReq := httptest.NewRequest("POST", "/api/v1/events/"+evt.ID.String()+"/submit", bytes.NewReader(submitBody))
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
	tierReq := httptest.NewRequest("POST", "/api/v1/events/"+evt.ID.String()+"/tiers", bytes.NewReader(tierBody))
	handler.ServeHTTP(tierRec, tierReq)

	if tierRec.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created for tier, got %d", tierRec.Code)
	}

	// 4. Submit lại sau khi có vé -> 200 OK
	submitRec2 := httptest.NewRecorder()
	submitReq2 := httptest.NewRequest("POST", "/api/v1/events/"+evt.ID.String()+"/submit", bytes.NewReader(submitBody))
	handler.ServeHTTP(submitRec2, submitReq2)

	if submitRec2.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for valid submit, got %d", submitRec2.Code)
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
