// Package httpadapter phoi bay ticketing qua REST.
package httpadapter

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/eventflow/eventflow/libs/go/authx"
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
	orderAPI       // API don hang (EV-NEW-2), xem order_handler.go
	log            *slog.Logger

	queueSecret []byte // xac minh queue token (EV-137); rong -> tu choi tao hold (fail-closed)
	now         func() time.Time
}

// WithQueueTokens bat xac minh queue token ky HMAC o createHold (EV-137).
func (h *Handler) WithQueueTokens(secret []byte) *Handler {
	h.queueSecret = secret
	return h
}

// Loai loi queue token. Tat ca 403: khach chua duoc vao mua ve theo nghia nay.
const (
	ProblemQueueTokenInvalid  = "/problems/queue-token-invalid"
	ProblemQueueTokenExpired  = "/problems/queue-token-expired"
	ProblemQueueTokenMismatch = "/problems/queue-token-mismatch"
)

func New(holds HoldCreator, log *slog.Logger) *Handler {
	return &Handler{holds: holds, log: log, now: time.Now}
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
	h.registerOrderRoutes(mux, idempotency)
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
	// Danh tinh do gateway xac minh va gan (authx.PrincipalFromHeaders).
	p, ok := authx.PrincipalFromHeaders(r.Header)
	if !ok {
		httpx.WriteProblem(w, http.StatusUnauthorized, "ban can dang nhap de xem han muc mua")
		return
	}
	out, err := h.limits.Execute(r.Context(), r.PathValue("eventID"), p.ID)
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

// admittedJTI xac minh X-Queue-Token va tra jti de doi chieu voi ZSET admitted. Token phai ky dung,
// con han, va thuoc dung su kien + dung nguoi + dung thiet bi dang goi: chia token cho tai khoan khac
// hay mang sang su kien khac deu bi chan o day (EV-137).
func (h *Handler) admittedJTI(w http.ResponseWriter, r *http.Request, eventID string, p authx.Principal) (string, bool) {
	if h.queueSecret == nil {
		httpx.WriteProblem(w, http.StatusServiceUnavailable, "chua cau hinh queue token")
		return "", false
	}
	claims, err := authx.VerifyQueueToken(h.queueSecret, r.Header.Get("X-Queue-Token"), h.now())
	switch {
	case errors.Is(err, authx.ErrExpiredQueueToken):
		httpx.WriteProblemType(w, http.StatusForbidden, ProblemQueueTokenExpired, "queue token da het han, hay vao hang cho lai")
	case err != nil:
		httpx.WriteProblemType(w, http.StatusForbidden, ProblemQueueTokenInvalid, "ban chua duoc vao mua ve, vui long quay lai phong cho")
	case !claims.Matches(eventID, p.ID, p.DeviceID):
		h.log.Warn("queue token khong khop nguoi goi khi giu ve", "event", eventID)
		httpx.WriteProblemType(w, http.StatusForbidden, ProblemQueueTokenMismatch, "queue token khong thuoc ve ban hoac thiet bi nay")
	default:
		return claims.JTI, true
	}
	return "", false
}

type createHoldRequest struct {
	TicketTypeID string `json:"ticket_type_id"`
	Quantity     int    `json:"quantity"`
}

func (h *Handler) createHold(w http.ResponseWriter, r *http.Request) {
	// BR-O5 (bat buoc Idempotency-Key, phat lai response cu) do middleware idem lo, xem Routes.
	p, ok := authx.PrincipalFromHeaders(r.Header)
	if !ok {
		httpx.WriteProblem(w, http.StatusUnauthorized, "ban can dang nhap de giu ve")
		return
	}
	eventID := r.PathValue("eventID")
	jti, ok := h.admittedJTI(w, r, eventID, p)
	if !ok {
		return
	}

	var req createHoldRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteProblem(w, http.StatusBadRequest, "body khong hop le")
		return
	}

	out, err := h.holds.Execute(r.Context(), app.CreateHoldInput{
		EventID:      eventID,
		TicketTypeID: req.TicketTypeID,
		IdentityID:   p.ID,
		QueueToken:   jti,
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
