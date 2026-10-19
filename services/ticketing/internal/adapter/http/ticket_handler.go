package httpadapter

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/eventflow/eventflow/libs/go/httpx"
	"github.com/eventflow/eventflow/services/ticketing/internal/domain"
	"github.com/eventflow/eventflow/services/ticketing/internal/port"
)

// TicketIssuer dinh nghia use case phat hanh ve.
type TicketIssuer interface {
	Execute(ctx context.Context, in port.IssueTicketsParams) (port.IssueTicketsResult, error)
}

// TicketVerifier dinh nghia use case xac thuc chu ky va trang thai ve.
type TicketVerifier interface {
	Execute(ctx context.Context, in port.VerifyTicketParams) (port.VerifyTicketResult, error)
}

// TicketReader doc danh sach ve cua mot don hang.
type TicketReader interface {
	GetTicketsByOrderID(ctx context.Context, orderID string) ([]domain.Ticket, error)
}

// Loai loi Ticket cho RFC 7807 problem details.
const (
	ProblemTamperedQR        = "/problems/tampered-qr"
	ProblemTicketAlreadyUsed = "/problems/ticket-already-used"
	ProblemTicketExpired     = "/problems/ticket-expired"
	ProblemOrderNotPaid      = "/problems/order-not-paid"
)

func (h *Handler) issueTickets(w http.ResponseWriter, r *http.Request) {
	orderID := r.PathValue("orderID")
	if !h.requireOwner(w, r, orderID) {
		return
	}
	traceID := r.Header.Get("X-Trace-Id")

	out, err := h.ticketIssuer.Execute(r.Context(), port.IssueTicketsParams{
		OrderID: orderID,
		TraceID: traceID,
	})

	switch {
	case err == nil:
		httpx.WriteJSON(w, http.StatusOK, out)
	case errors.Is(err, domain.ErrOrderNotFound):
		httpx.WriteProblem(w, http.StatusNotFound, "khong tim thay don hang")
	case errors.Is(err, domain.ErrOrderNotPaid):
		httpx.WriteProblemType(w, http.StatusUnprocessableEntity, ProblemOrderNotPaid,
			"don hang chua duoc thanh toan de phat hanh ve")
	default:
		h.log.Error("phat hanh ve that bai", "order_id", orderID, "err", err)
		httpx.WriteProblem(w, http.StatusInternalServerError, "phat hanh ve that bai, vui long thu lai")
	}
}

func (h *Handler) listOrderTickets(w http.ResponseWriter, r *http.Request) {
	orderID := r.PathValue("orderID")
	if !h.requireOwner(w, r, orderID) {
		return
	}
	tickets, err := h.ticketReader.GetTicketsByOrderID(r.Context(), orderID)
	if err != nil {
		h.log.Error("doc danh sach ve that bai", "order_id", orderID, "err", err)
		httpx.WriteProblem(w, http.StatusInternalServerError, "doc danh sach ve that bai")
		return
	}

	w.Header().Set("Cache-Control", "private, no-cache")
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"order_id": orderID,
		"count":    len(tickets),
		"tickets":  tickets,
	})
}

type verifyTicketRequest struct {
	QRPayload string `json:"qr_payload"`
}

func (h *Handler) verifyTicket(w http.ResponseWriter, r *http.Request) {
	var req verifyTicketRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteProblem(w, http.StatusBadRequest, "body khong hop le, can co qr_payload")
		return
	}

	traceID := r.Header.Get("X-Trace-Id")
	res, err := h.ticketVerifier.Execute(r.Context(), port.VerifyTicketParams{
		QRPayload: req.QRPayload,
		TraceID:   traceID,
	})

	switch {
	case err == nil:
		httpx.WriteJSON(w, http.StatusOK, res)
	case errors.Is(err, domain.ErrTamperedQR):
		httpx.WriteProblemType(w, http.StatusBadRequest, ProblemTamperedQR,
			"ma QR gia mao hoac chu ky khong hop le")
	case errors.Is(err, domain.ErrTicketExpired):
		httpx.WriteProblemType(w, http.StatusGone, ProblemTicketExpired,
			"ma QR da het han su dung")
	case errors.Is(err, domain.ErrTicketNotFound):
		httpx.WriteProblem(w, http.StatusNotFound, "khong tim thay ve trong he thong")
	case errors.Is(err, domain.ErrTicketAlreadyUsed):
		httpx.WriteProblemType(w, http.StatusConflict, ProblemTicketAlreadyUsed,
			"ve da duoc su dung de vao cua truoc do (AC-5)")
	case errors.Is(err, domain.ErrTicketVoid):
		httpx.WriteProblem(w, http.StatusForbidden, "ve da bi vo hieu hoa")
	case errors.Is(err, domain.ErrTicketRefunded):
		httpx.WriteProblem(w, http.StatusForbidden, "ve da duoc hoan tien")
	default:
		h.log.Error("xac thuc ve that bai", "err", err)
		httpx.WriteProblem(w, http.StatusInternalServerError, "xac thuc ve that bai")
	}
}
