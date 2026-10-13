package http

import "net/http"

func NewRouter(h *EventHandler) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	mux.HandleFunc("POST /api/v1/events", h.CreateEvent)
	mux.HandleFunc("GET /api/v1/events/{id}", h.GetEventByID)
	mux.HandleFunc("GET /api/v1/events/slug/{slug}", h.GetEventBySlug)
	mux.HandleFunc("GET /api/v1/events", h.ListOrganizerEvents)
	mux.HandleFunc("PUT /api/v1/events/{id}", h.UpdateEvent)

	mux.HandleFunc("POST /api/v1/events/{id}/tiers", h.AddTier)
	mux.HandleFunc("PUT /api/v1/events/{id}/tiers/{tierId}", h.UpdateTierQuota)

	mux.HandleFunc("POST /api/v1/events/{id}/submit", h.SubmitEvent)
	mux.HandleFunc("POST /api/v1/events/{id}/approve", h.ApproveEvent)
	mux.HandleFunc("POST /api/v1/events/{id}/reject", h.RejectEvent)
	mux.HandleFunc("POST /api/v1/events/{id}/publish", h.PublishEvent)
	mux.HandleFunc("POST /api/v1/events/{id}/cancel", h.CancelEvent)

	// EV-120: AI moderation callback noi bo
	mux.HandleFunc("PATCH /internal/v1/events/{id}/ai-verdict", h.HandleAiVerdict)

	// EV-121: Hang doi moderation cho Moderator
	mux.HandleFunc("GET /api/v1/moderation/queue", h.GetModerationQueue)

	return mux
}
