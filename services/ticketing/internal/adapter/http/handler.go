// Package httpadapter phoi bay ticketing qua REST.
package httpadapter

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/eventflow/eventflow/libs/go/httpx"
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

// Routes dang ky route. idempotency BAT BUOC: BR-O5 yeu cau moi API ghi co Idempotency-Key, nen
// khong cho phep dang ky route ghi ma khong truyen middleware nay (truyen idem.Middleware(...)).
func (h *Handler) Routes(mux *http.ServeMux, idempotency httpx.Middleware) {
	mux.Handle("POST /v1/events/{eventID}/holds", idempotency(http.HandlerFunc(h.createHold)))
}

type createHoldRequest struct {
	TicketTypeID string `json:"ticket_type_id"`
	Quantity     int    `json:"quantity"`
}

func (h *Handler) createHold(w http.ResponseWriter, r *http.Request) {
	// BR-O5 (bat buoc Idempotency-Key, phat lai response cu) do middleware idem lo, xem Routes.
	var req createHoldRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteProblem(w, http.StatusBadRequest, "body khong hop le")
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
		httpx.WriteJSON(w, http.StatusOK, out)
	case errors.Is(err, domain.ErrNotAdmitted):
		httpx.WriteProblem(w, http.StatusForbidden, "ban chua duoc vao mua ve, vui long quay lai phong cho")
	case errors.Is(err, domain.ErrSoldOut):
		// 409 chu khong phai 500: het ve la ket qua nghiep vu binh thuong, va
		// day la ma trang thai ma client dua vao de hien dung thong bao.
		httpx.WriteProblem(w, http.StatusConflict, "rat tiec, hang ve nay da het")
	case errors.Is(err, domain.ErrPerIdentityLimit), errors.Is(err, domain.ErrQuantityNotAllowed):
		httpx.WriteProblem(w, http.StatusUnprocessableEntity, "so luong vuot gioi han cho phep")
	default:
		h.log.Error("tao hold that bai", "err", err)
		httpx.WriteProblem(w, http.StatusServiceUnavailable, "he thong dang ban, vui long thu lai")
	}
}
