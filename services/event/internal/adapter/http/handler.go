package http

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
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

	aiVerdictUC app.HandleAiVerdictUseCase
	modQueueUC  app.ListModerationQueueUseCase
	modActionUC app.ModeratorActionUseCase
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

// WithModeration gan cac use case lien quan den kiem duyet AI va Moderator (EV-120, EV-121).
func (h *EventHandler) WithModeration(
	aiVerdictUC app.HandleAiVerdictUseCase,
	modQueueUC app.ListModerationQueueUseCase,
	modActionUC app.ModeratorActionUseCase,
) *EventHandler {
	h.aiVerdictUC = aiVerdictUC
	h.modQueueUC = modQueueUC
	h.modActionUC = modActionUC
	return h
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
	id, err := uuid.Parse(idStr)
	if err != nil {
		httpx.WriteProblem(w, http.StatusBadRequest, "invalid event uuid")
		return
	}

	evt, err := h.getUC.GetByID(r.Context(), id)
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
		httpx.WriteProblem(w, http.StatusBadRequest, "slug cannot be empty")
		return
	}

	evt, err := h.getUC.GetBySlug(r.Context(), slug)
	if err != nil {
		handleDomainError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, ToEventResponse(evt))
}

// GET /api/v1/events?organizer_id={id}
func (h *EventHandler) ListOrganizerEvents(w http.ResponseWriter, r *http.Request) {
	orgIDStr := r.URL.Query().Get("organizer_id")
	if orgIDStr == "" {
		httpx.WriteProblem(w, http.StatusBadRequest, "query parameter organizer_id is required")
		return
	}

	orgID, err := uuid.Parse(orgIDStr)
	if err != nil {
		httpx.WriteProblem(w, http.StatusBadRequest, "invalid organizer_id uuid")
		return
	}

	events, err := h.getUC.ListByOrganizer(r.Context(), orgID)
	if err != nil {
		handleDomainError(w, err)
		return
	}

	resp := make([]EventResponse, len(events))
	for i, e := range events {
		resp[i] = ToEventResponse(e)
	}

	httpx.WriteJSON(w, http.StatusOK, resp)
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

// POST /api/v1/events/{id}/submit (EV-117 - BR-E1 & EV-120)
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

// POST /api/v1/events/{id}/approve (EV-118 - BR-E2 & EV-121)
func (h *EventHandler) ApproveEvent(w http.ResponseWriter, r *http.Request) {
	evtID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		httpx.WriteProblem(w, http.StatusBadRequest, "invalid event_id uuid")
		return
	}

	var req ModeratorApproveRequest
	_ = json.NewDecoder(r.Body).Decode(&req)

	var evt *domain.Event
	if req.ModeratorID != "" && h.modActionUC != nil {
		modID, parseErr := uuid.Parse(req.ModeratorID)
		if parseErr != nil {
			httpx.WriteProblem(w, http.StatusBadRequest, "invalid moderator_id uuid")
			return
		}
		evt, err = h.modActionUC.Approve(r.Context(), app.ModeratorApproveInput{
			EventID:     evtID,
			ModeratorID: modID,
			Note:        req.Note,
		})
	} else {
		evt, err = h.transitionUC.Approve(r.Context(), evtID, time.Now().UTC())
	}

	if err != nil {
		handleDomainError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, ToEventResponse(evt))
}

// POST /api/v1/events/{id}/reject (EV-121 - BR-E5)
func (h *EventHandler) RejectEvent(w http.ResponseWriter, r *http.Request) {
	evtID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		httpx.WriteProblem(w, http.StatusBadRequest, "invalid event_id uuid")
		return
	}

	var req ModeratorRejectRequest
	decodeErr := json.NewDecoder(r.Body).Decode(&req)

	var evt *domain.Event
	// Neu co request body voi thong tin moderator hoac reason thi dung ModeratorActionUseCase
	if decodeErr == nil && (req.ModeratorID != "" || req.Reason != "") && h.modActionUC != nil {
		var modID uuid.UUID
		if req.ModeratorID != "" {
			var parseErr error
			modID, parseErr = uuid.Parse(req.ModeratorID)
			if parseErr != nil {
				httpx.WriteProblem(w, http.StatusBadRequest, "invalid moderator_id uuid")
				return
			}
		}
		evt, err = h.modActionUC.Reject(r.Context(), app.ModeratorRejectInput{
			EventID:     evtID,
			ModeratorID: modID,
			Reason:      req.Reason,
		})
	} else {
		evt, err = h.transitionUC.Reject(r.Context(), evtID)
	}

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

// PATCH /internal/v1/events/{id}/ai-verdict (EV-120 callback noi bo tu ai-worker)
func (h *EventHandler) HandleAiVerdict(w http.ResponseWriter, r *http.Request) {
	evtID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		httpx.WriteProblem(w, http.StatusBadRequest, "invalid event_id uuid")
		return
	}

	var req AiVerdictRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteProblem(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if h.aiVerdictUC == nil {
		httpx.WriteProblem(w, http.StatusNotImplemented, "ai verdict use case not configured")
		return
	}

	evt, _, err := h.aiVerdictUC.Execute(r.Context(), app.HandleAiVerdictInput{
		EventID: evtID,
		Score:   req.Score,
		Verdict: req.Verdict,
		Reasons: req.Reasons,
		Axes:    req.Axes,
	})
	if err != nil {
		handleDomainError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"status": evt.Status,
	})
}

// GET /api/v1/moderation/queue (EV-121 hang doi cho Moderator)
func (h *EventHandler) GetModerationQueue(w http.ResponseWriter, r *http.Request) {
	if h.modQueueUC == nil {
		httpx.WriteJSON(w, http.StatusOK, []any{})
		return
	}

	items, err := h.modQueueUC.Execute(r.Context())
	if err != nil {
		handleDomainError(w, err)
		return
	}

	if items == nil {
		items = []*app.ModerationQueueItem{}
	}

	httpx.WriteJSON(w, http.StatusOK, items)
}

func handleDomainError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, domain.ErrEventNotFound) ||
		errors.Is(err, domain.ErrTierNotFound) ||
		errors.Is(err, domain.ErrModReviewNotFound):
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
		errors.Is(err, domain.ErrInvalidPrice) ||
		errors.Is(err, domain.ErrRejectReasonRequired) ||
		errors.Is(err, domain.ErrInvalidAiScore) ||
		strings.Contains(err.Error(), "reason is required"):
		httpx.WriteProblem(w, http.StatusUnprocessableEntity, err.Error())
	default:
		httpx.WriteProblem(w, http.StatusInternalServerError, err.Error())
	}
}
