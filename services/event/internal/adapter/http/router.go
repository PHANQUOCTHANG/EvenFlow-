package http

import "net/http"

// NewRouter dang ky route cua event-svc kem quyen theo ma tran docs/01 muc 3 (EV-114).
//
// Danh tinh do gateway gan (X-Identity-Id / X-Identity-Role). Ownership (organizer chi sua su
// kien cua minh) kiem trong use case bang organizer_id lay tu token.
func NewRouter(h *EventHandler) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// Cong khai: su kien chua duyet chi chu su kien / moderator / ops thay (canSeeUnpublished).
	mux.HandleFunc("GET /api/v1/events/public", h.ListPublicEvents)
	mux.HandleFunc("GET /api/v1/events/{id}", h.GetEventByID)
	mux.HandleFunc("GET /api/v1/events/slug/{slug}", h.GetEventBySlug)

	// Organizer: tao/sua su kien cua minh.
	mux.Handle("POST /api/v1/events", h.require(h.CreateEvent, roleOrganizer))
	mux.Handle("PUT /api/v1/events/{id}", h.require(h.UpdateEvent, roleOrganizer))
	mux.Handle("POST /api/v1/events/{id}/tiers", h.require(h.AddTier, roleOrganizer))
	mux.Handle("PUT /api/v1/events/{id}/tiers/{tierId}", h.require(h.UpdateTierQuota, roleOrganizer))
	mux.Handle("POST /api/v1/events/{id}/submit", h.require(h.SubmitEvent, roleOrganizer))
	mux.Handle("POST /api/v1/events/{id}/cancel", h.require(h.CancelEvent, roleOrganizer))
	mux.Handle("GET /api/v1/events", h.require(h.ListOrganizerEvents, roleOrganizer, roleModerator, roleOps))

	// Moderator: duyet / tu choi / hang doi kiem duyet.
	mux.Handle("POST /api/v1/events/{id}/approve", h.require(h.ApproveEvent, roleModerator))
	mux.Handle("POST /api/v1/events/{id}/reject", h.require(h.RejectEvent, roleModerator))
	mux.Handle("GET /api/v1/moderation/queue", h.require(h.GetModerationQueue, roleModerator))

	// Mo ban (SCHEDULED -> ON_SALE) la thao tac van hanh, khong phai cua organizer: chi Ops,
	// cho toi khi co scheduler tu mo luc sale_start_at.
	mux.Handle("POST /api/v1/events/{id}/publish", h.require(h.PublishEvent, roleOps))

	// EV-120: callback noi bo tu ai-worker. Gateway chan /internal/* tu ben ngoai.
	mux.HandleFunc("PATCH /internal/v1/events/{id}/ai-verdict", h.HandleAiVerdict)

	return mux
}
