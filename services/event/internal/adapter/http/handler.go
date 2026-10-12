package http

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/eventflow/eventflow/libs/go/httpx"
	"github.com/eventflow/eventflow/services/event/internal/app"
	"github.com/eventflow/eventflow/services/event/internal/domain"
	"github.com/google/uuid"
)

type EventHandler struct {
	createUC     app.CreateEventUseCase
	updateUC     app.UpdateEventUseCase
	tiersUC      app.ManageTiersUseCase
	transitionUC app.TransitionStateUseCase
	getUC        app.GetEventUseCase
}

func NewEventHandler(
	createUC app.CreateEventUseCase,
	updateUC app.UpdateEventUseCase,
	tiersUC app.ManageTiersUseCase,
	transitionUC app.TransitionStateUseCase,
	getUC app.GetEventUseCase,
) *EventHandler {
	return &EventHandler{
		createUC:     createUC,
		updateUC:     updateUC,
		tiersUC:      tiersUC,
		transitionUC: transitionUC,
		getUC:        getUC,
	}
}

// POST /api/v1/events
func (h *EventHandler) CreateEvent(w http.ResponseWriter, r *http.Request) {
	var req CreateEventRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteProblem(w, http.StatusBadRequest, "invalid request body")
		return
	}

	orgID, err := uuid.Parse(req.OrganizerID)
	if err != nil {
		httpx.WriteProblem(w, http.StatusBadRequest, "invalid organizer_id uuid")
		return
	}

	evt, err := h.createUC.Execute(r.Context(), app.CreateEventInput{
		OrganizerID: orgID,
		Slug:        req.Slug,
		Title:       req.Title,
		Description: req.Description,
		Venue:       req.Venue,
		StartsAt:    req.StartsAt,
		SaleStartAt: req.SaleStartAt,
		SaleEndAt:   req.SaleEndAt,
	})
	if err != nil {
		handleDomainError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusCreated, ToEventResponse(evt))
}

// GET /api/v1/events/{id}
func (h *EventHandler) GetEventByID(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	evtID, err := uuid.Parse(idStr)
	if err != nil {
		httpx.WriteProblem(w, http.StatusBadRequest, "invalid event_id uuid")
		return
	}

	evt, err := h.getUC.GetByID(r.Context(), evtID)
	if err != nil {
		handleDomainError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, ToEventResponse(evt))
}

// GET /api/v1/events/slug/{slug}
func (h *EventHandler) GetEventBySlug(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	if slug == "" {
		httpx.WriteProblem(w, http.StatusBadRequest, "slug is required")
		return
	}

	evt, err := h.getUC.GetBySlug(r.Context(), slug)
	if err != nil {
		handleDomainError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, ToEventResponse(evt))
}

// GET /api/v1/events?organizer_id=...
func (h *EventHandler) ListOrganizerEvents(w http.ResponseWriter, r *http.Request) {
	orgIDStr := r.URL.Query().Get("organizer_id")
	orgID, err := uuid.Parse(orgIDStr)
	if err != nil {
		httpx.WriteProblem(w, http.StatusBadRequest, "valid organizer_id query param is required")
		return
	}

	events, err := h.getUC.ListByOrganizer(r.Context(), orgID)
	if err != nil {
		handleDomainError(w, err)
		return
	}

	res := make([]EventResponse, len(events))
	for i, e := range events {
		res[i] = ToEventResponse(e)
	}

	httpx.WriteJSON(w, http.StatusOK, res)
}

// PUT /api/v1/events/{id}
func (h *EventHandler) UpdateEvent(w http.ResponseWriter, r *http.Request) {
	evtID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		httpx.WriteProblem(w, http.StatusBadRequest, "invalid event_id uuid")
		return
	}

	var req UpdateEventRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteProblem(w, http.StatusBadRequest, "invalid request body")
		return
	}

	orgID, err := uuid.Parse(req.OrganizerID)
	if err != nil {
		httpx.WriteProblem(w, http.StatusBadRequest, "invalid organizer_id uuid")
		return
	}

	evt, err := h.updateUC.Execute(r.Context(), app.UpdateEventInput{
		EventID:     evtID,
		OrganizerID: orgID,
		Title:       req.Title,
		Description: req.Description,
		Venue:       req.Venue,
		StartsAt:    req.StartsAt,
		SaleStartAt: req.SaleStartAt,
		SaleEndAt:   req.SaleEndAt,
	})
	if err != nil {
		handleDomainError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, ToEventResponse(evt))
}

// POST /api/v1/events/{id}/tiers
func (h *EventHandler) AddTier(w http.ResponseWriter, r *http.Request) {
	evtID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		httpx.WriteProblem(w, http.StatusBadRequest, "invalid event_id uuid")
		return
	}

	var req AddTierRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteProblem(w, http.StatusBadRequest, "invalid request body")
		return
	}

	orgID, err := uuid.Parse(req.OrganizerID)
	if err != nil {
		httpx.WriteProblem(w, http.StatusBadRequest, "invalid organizer_id uuid")
		return
	}

	tier, err := h.tiersUC.AddTier(r.Context(), app.AddTierInput{
		EventID:     evtID,
		OrganizerID: orgID,
		Name:        req.Name,
		PriceCents:  req.PriceCents,
		Quota:       req.Quota,
	})
	if err != nil {
		handleDomainError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusCreated, tier)
}

// PUT /api/v1/events/{id}/tiers/{tierId} (EV-119 - BR-E3)
func (h *EventHandler) UpdateTierQuota(w http.ResponseWriter, r *http.Request) {
	evtID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		httpx.WriteProblem(w, http.StatusBadRequest, "invalid event_id uuid")
		return
	}

	tierID, err := uuid.Parse(r.PathValue("tierId"))
	if err != nil {
		httpx.WriteProblem(w, http.StatusBadRequest, "invalid tier_id uuid")
		return
	}

	var req UpdateTierQuotaRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteProblem(w, http.StatusBadRequest, "invalid request body")
		return
	}

	orgID, err := uuid.Parse(req.OrganizerID)
	if err != nil {
		httpx.WriteProblem(w, http.StatusBadRequest, "invalid organizer_id uuid")
		return
	}

	tier, err := h.tiersUC.UpdateTierQuota(r.Context(), app.UpdateTierQuotaInput{
		EventID:     evtID,
		OrganizerID: orgID,
		TierID:      tierID,
		NewQuota:    req.NewQuota,
	})
	if err != nil {
		handleDomainError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, tier)
}

// POST /api/v1/events/{id}/submit (EV-117 - BR-E1)
func (h *EventHandler) SubmitEvent(w http.ResponseWriter, r *http.Request) {
	evtID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		httpx.WriteProblem(w, http.StatusBadRequest, "invalid event_id uuid")
		return
	}

	var req struct {
		OrganizerID string `json:"organizer_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteProblem(w, http.StatusBadRequest, "invalid request body")
		return
	}

	orgID, err := uuid.Parse(req.OrganizerID)
	if err != nil {
		httpx.WriteProblem(w, http.StatusBadRequest, "invalid organizer_id uuid")
		return
	}

	evt, err := h.transitionUC.SubmitForReview(r.Context(), evtID, orgID)
	if err != nil {
		handleDomainError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, ToEventResponse(evt))
}

// POST /api/v1/events/{id}/approve (EV-118 - BR-E2)
func (h *EventHandler) ApproveEvent(w http.ResponseWriter, r *http.Request) {
	evtID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		httpx.WriteProblem(w, http.StatusBadRequest, "invalid event_id uuid")
		return
	}

	evt, err := h.transitionUC.Approve(r.Context(), evtID, time.Now().UTC())
	if err != nil {
		handleDomainError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, ToEventResponse(evt))
}

// POST /api/v1/events/{id}/reject
func (h *EventHandler) RejectEvent(w http.ResponseWriter, r *http.Request) {
	evtID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		httpx.WriteProblem(w, http.StatusBadRequest, "invalid event_id uuid")
		return
	}

	evt, err := h.transitionUC.Reject(r.Context(), evtID)
	if err != nil {
		handleDomainError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, ToEventResponse(evt))
}

// POST /api/v1/events/{id}/publish
func (h *EventHandler) PublishEvent(w http.ResponseWriter, r *http.Request) {
	evtID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		httpx.WriteProblem(w, http.StatusBadRequest, "invalid event_id uuid")
		return
	}

	evt, err := h.transitionUC.Publish(r.Context(), evtID, time.Now().UTC())
	if err != nil {
		handleDomainError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, ToEventResponse(evt))
}

// POST /api/v1/events/{id}/cancel
func (h *EventHandler) CancelEvent(w http.ResponseWriter, r *http.Request) {
	evtID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		httpx.WriteProblem(w, http.StatusBadRequest, "invalid event_id uuid")
		return
	}

	var req struct {
		OrganizerID string `json:"organizer_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteProblem(w, http.StatusBadRequest, "invalid request body")
		return
	}

	orgID, err := uuid.Parse(req.OrganizerID)
	if err != nil {
		httpx.WriteProblem(w, http.StatusBadRequest, "invalid organizer_id uuid")
		return
	}

	evt, err := h.transitionUC.Cancel(r.Context(), evtID, orgID)
	if err != nil {
		handleDomainError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, ToEventResponse(evt))
}

func handleDomainError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, domain.ErrEventNotFound) || errors.Is(err, domain.ErrTierNotFound):
		httpx.WriteProblem(w, http.StatusNotFound, err.Error())
	case errors.Is(err, domain.ErrUnauthorizedOrganizer):
		httpx.WriteProblem(w, http.StatusForbidden, err.Error())
	case errors.Is(err, domain.ErrNoValidTicketTiers) ||
		errors.Is(err, domain.ErrSaleStartTimeTooEarly) ||
		errors.Is(err, domain.ErrCannotReduceQuotaOnSale) ||
		errors.Is(err, domain.ErrInvalidStateTransition) ||
		errors.Is(err, domain.ErrDuplicateTierName) ||
		errors.Is(err, domain.ErrEventNotEditable) ||
		errors.Is(err, domain.ErrInvalidTitle) ||
		errors.Is(err, domain.ErrInvalidEventDates) ||
		errors.Is(err, domain.ErrInvalidQuota) ||
		errors.Is(err, domain.ErrInvalidPrice):
		httpx.WriteProblem(w, http.StatusUnprocessableEntity, err.Error())
	default:
		httpx.WriteProblem(w, http.StatusInternalServerError, err.Error())
	}
}
