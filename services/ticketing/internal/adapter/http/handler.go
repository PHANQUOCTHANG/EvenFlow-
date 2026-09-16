// Package httpadapter phoi bay ticketing qua REST.
package httpadapter

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/eventflow/eventflow/services/ticketing/internal/app"
	"github.com/eventflow/eventflow/services/ticketing/internal/domain"
)

type HoldCreator interface {
	Execute(ctx context.Context, in app.CreateHoldInput) (app.CreateHoldOutput, error)
}

type Handler struct {
	holds HoldCreator
	log   *slog.Logger
}

func New(holds HoldCreator, log *slog.Logger) *Handler {
	return &Handler{holds: holds, log: log}
}

func (h *Handler) Routes(mux *http.ServeMux) {
	mux.HandleFunc("POST /v1/events/{eventID}/holds", h.createHold)
}

type createHoldRequest struct {
	TicketTypeID string `json:"ticket_type_id"`
	Quantity     int    `json:"quantity"`
}

func (h *Handler) createHold(w http.ResponseWriter, r *http.Request) {
	// BR-O5: khong co Idempotency-Key thi khong duoc ghi. Mang di dong chap chon
	// khien client retry rat thuong xuyen; thieu khoa nay la tao don trung.
	if r.Header.Get("Idempotency-Key") == "" {
		problem(w, http.StatusBadRequest, "thieu header Idempotency-Key")
		return
	}

	var req createHoldRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		problem(w, http.StatusBadRequest, "body khong hop le")
		return
	}

	out, err := h.holds.Execute(r.Context(), app.CreateHoldInput{
		EventID:      r.PathValue("eventID"),
		TicketTypeID: req.TicketTypeID,
		IdentityID:   r.Header.Get("X-Identity-Id"),
		QueueToken:   r.Header.Get("X-Queue-Token"),
		Quantity:     req.Quantity,
	})

	switch {
	case err == nil:
		writeJSON(w, http.StatusOK, out)
	case errors.Is(err, domain.ErrNotAdmitted):
		problem(w, http.StatusForbidden, "ban chua duoc vao mua ve, vui long quay lai phong cho")
	case errors.Is(err, domain.ErrSoldOut):
		// 409 chu khong phai 500: het ve la ket qua nghiep vu binh thuong, va
		// day la ma trang thai ma client dua vao de hien dung thong bao.
		problem(w, http.StatusConflict, "rat tiec, hang ve nay da het")
	case errors.Is(err, domain.ErrPerIdentityLimit), errors.Is(err, domain.ErrQuantityNotAllowed):
		problem(w, http.StatusUnprocessableEntity, "so luong vuot gioi han cho phep")
	default:
		h.log.Error("tao hold that bai", "err", err)
		problem(w, http.StatusServiceUnavailable, "he thong dang ban, vui long thu lai")
	}
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func problem(w http.ResponseWriter, code int, detail string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"type":   "about:blank",
		"title":  http.StatusText(code),
		"status": code,
		"detail": detail,
	})
}
