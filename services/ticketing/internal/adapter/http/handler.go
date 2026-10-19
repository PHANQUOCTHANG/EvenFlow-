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
	"github.com/eventflow/eventflow/services/ticketing/internal/port"
)

type HoldCreator interface {
	Execute(ctx context.Context, in app.CreateHoldInput) (app.CreateHoldOutput, error)
}

// PurchaseLimitReader tra han muc mua cua mot khach (BR-O4).
type PurchaseLimitReader interface {
	Execute(ctx context.Context, eventID, identityID string) (port.PurchaseLimit, error)
}

type Handler struct {
	holds          HoldCreator
	limits         PurchaseLimitReader
	ticketIssuer   TicketIssuer
	ticketVerifier TicketVerifier
	ticketReader   TicketReader
	log            *slog.Logger
}

func New(holds HoldCreator, log *slog.Logger) *Handler {
	return &Handler{holds: holds, log: log}
}

// WithPurchaseLimits bat route GET han muc mua. Khong goi thi route khong duoc dang ky.
func (h *Handler) WithPurchaseLimits(l PurchaseLimitReader) *Handler {
	h.limits = l
	return h
}

// WithTickets dang ky use case phat hanh, xac thuc va xem ve (EV-133).
func (h *Handler) WithTickets(issuer TicketIssuer, verifier TicketVerifier, reader TicketReader) *Handler {
	h.ticketIssuer = issuer
	h.ticketVerifier = verifier
	h.ticketReader = reader
	return h
}

// Loai loi BR-O4. Cung 422 nhung client xu ly khac nhau: so luong sai thi chon lai, vuot han muc
// ca nhan thi khong mua them duoc nua.
const (
	ProblemQuantityNotAllowed = "/problems/quantity-not-allowed"
	ProblemPerIdentityLimit   = "/problems/per-identity-limit"
)

// Routes dang ky route. idempotency BAT BUOC: BR-O5 yeu cau moi API ghi co Idempotency-Key, nen
// khong cho phep dang ky route ghi ma khong truyen middleware nay (truyen idem.Middleware(...)).
func (h *Handler) Routes(mux *http.ServeMux, idempotency httpx.Middleware) {
	mux.Handle("POST /v1/events/{eventID}/holds", idempotency(http.HandlerFunc(h.createHold)))
	if h.limits != nil {
		// Doc, khong ghi: khong can Idempotency-Key.
		mux.HandleFunc("GET /v1/events/{eventID}/purchase-limit", h.purchaseLimit)
	}
	if h.ticketIssuer != nil {
		// Phat hanh ve can Idempotency-Key chong duplicate retry (BR-O5)
		mux.Handle("POST /v1/orders/{orderID}/issue-tickets", idempotency(http.HandlerFunc(h.issueTickets)))
	}
	if h.ticketReader != nil {
		mux.HandleFunc("GET /v1/orders/{orderID}/tickets", h.listOrderTickets)
	}
	if h.ticketVerifier != nil {
		mux.HandleFunc("POST /v1/tickets/verify", h.verifyTicket)
	}
}

func (h *Handler) purchaseLimit(w http.ResponseWriter, r *http.Request) {
	// Dinh danh lay tu X-Identity-Id cho toi khi gateway cap JWT (giai doan 3), giong createHold.
	identityID := r.Header.Get("X-Identity-Id")
	if identityID == "" {
		httpx.WriteProblem(w, http.StatusUnauthorized, "ban can dang nhap de xem han muc mua")
		return
	}
	out, err := h.limits.Execute(r.Context(), r.PathValue("eventID"), identityID)
	switch {
	case err == nil:
		// Han muc doi ngay khi khach giu ghe hay thanh toan: khong cho cache dung chung.
		w.Header().Set("Cache-Control", "private, no-store")
		httpx.WriteJSON(w, http.StatusOK, out)
	case errors.Is(err, domain.ErrEventNotFound):
		httpx.WriteProblem(w, http.StatusNotFound, "su kien khong ton tai hoac chua mo ban")
	default:
		h.log.Error("doc han muc mua that bai", "err", err)
		httpx.WriteProblem(w, http.StatusServiceUnavailable, "he thong dang ban, vui long thu lai")
	}
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
	case errors.Is(err, domain.ErrQuantityNotAllowed):
		httpx.WriteProblemType(w, http.StatusUnprocessableEntity, ProblemQuantityNotAllowed,
			"so luong ve moi don khong hop le")
	case errors.Is(err, domain.ErrPerIdentityLimit):
		httpx.WriteProblemType(w, http.StatusUnprocessableEntity, ProblemPerIdentityLimit,
			"ban da dat gioi han so ve moi nguoi cho su kien nay")
	default:
		h.log.Error("tao hold that bai", "err", err)
		httpx.WriteProblem(w, http.StatusServiceUnavailable, "he thong dang ban, vui long thu lai")
	}
}
