package httpadapter

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/eventflow/eventflow/libs/go/authx"
	"github.com/eventflow/eventflow/libs/go/httpx"
	"github.com/eventflow/eventflow/services/ticketing/internal/app"
	"github.com/eventflow/eventflow/services/ticketing/internal/domain"
)

// OrderGetter doc don cua khach (app.GetOrder thoa man).
type OrderGetter interface {
	Execute(ctx context.Context, orderID, identityID string) (app.OrderDetails, error)
}

// HoldCancelUC huy hold cua khach (app.CancelHold thoa man).
type HoldCancelUC interface {
	Execute(ctx context.Context, holdID, identityID string) error
}

// AvailabilityReader tra muc con ve cong khai (app.GetAvailability thoa man).
type AvailabilityReader interface {
	Execute(ctx context.Context, eventID string) ([]app.TierAvailability, error)
}

// ProblemHoldNotCancellable: hold dang thanh toan / da thanh toan, khach khong tu huy duoc.
const ProblemHoldNotCancellable = "/problems/hold-not-cancellable"

// availabilityTTL: nhieu nghin khach hoi cung luc; mot ban doc dung chung cho moi nguoi trong khoang nay,
// va Cache-Control cho phep gateway / CDN gom them. Muc con ve doi cham hon thoi gian nay vo nghia.
const availabilityTTL = 3 * time.Second

// maxAvailabilityEntries chan bo nho phinh khi ai do hoi hang loat su kien khong ton tai.
const maxAvailabilityEntries = 1024

// orderAPI gom phan trang thai cua API don hang de Handler chi them mot dong.
type orderAPI struct {
	orders     OrderGetter
	cancel     HoldCancelUC
	avail      AvailabilityReader
	availCache *availCache
	owner      OrderOwner
}

// OrderOwner cho biet don co thuoc khach khong (port.OrderReader thoa man).
type OrderOwner interface {
	OrderOwnedBy(ctx context.Context, orderID, identityID string) (bool, error)
}

// WithOrderOwner bat kiem quyen so huu don o cac route ve (phat hanh, liet ke). Khong goi thi cac route
// do tu choi moi yeu cau (fail-closed): truoc day bat ky ai dang nhap deu doc duoc ve cua don bat ky.
func (h *Handler) WithOrderOwner(o OrderOwner) *Handler {
	h.owner = o
	return h
}

// requireOwner tra true neu nguoi goi dang nhap va so huu don. Don khong ton tai va don cua nguoi khac
// cho cung ket qua 404.
func (h *Handler) requireOwner(w http.ResponseWriter, r *http.Request, orderID string) bool {
	p, ok := authx.PrincipalFromHeaders(r.Header)
	if !ok {
		httpx.WriteProblem(w, http.StatusUnauthorized, "ban can dang nhap")
		return false
	}
	if h.owner == nil {
		httpx.WriteProblem(w, http.StatusServiceUnavailable, "chua cau hinh kiem tra quyen so huu don")
		return false
	}
	owned, err := h.owner.OrderOwnedBy(r.Context(), orderID, p.ID)
	if err != nil {
		h.log.Error("kiem tra chu don that bai", "err", err)
		httpx.WriteProblem(w, http.StatusServiceUnavailable, "he thong dang ban, vui long thu lai")
		return false
	}
	if !owned {
		httpx.WriteProblem(w, http.StatusNotFound, "khong tim thay don hang")
		return false
	}
	return true
}

// WithOrders bat API don hang: xem don, huy hold, ton kho cong khai (EV-NEW-2).
func (h *Handler) WithOrders(get OrderGetter, cancel HoldCancelUC, avail AvailabilityReader) *Handler {
	h.orders, h.cancel, h.avail = get, cancel, avail
	h.availCache = &availCache{entries: map[string]*availEntry{}, now: time.Now}
	return h
}

func (h *Handler) registerOrderRoutes(mux *http.ServeMux, idempotency httpx.Middleware) {
	if h.orders != nil {
		mux.HandleFunc("GET /v1/orders/{orderID}", h.getOrder)
	}
	if h.cancel != nil {
		// Ghi -> bat buoc Idempotency-Key (BR-O5), nhu moi API ghi khac.
		mux.Handle("DELETE /v1/holds/{holdID}", idempotency(http.HandlerFunc(h.cancelHold)))
	}
	if h.avail != nil {
		mux.HandleFunc("GET /v1/events/{eventID}/availability", h.availability)
	}
}

// orderStatusForClient dich trang thai don sang bo gia tri web dung (OrderStatus). Trang thai tho
// van duoc tra o truong order_status.
func orderStatusForClient(raw string, paymentFailed bool) string {
	switch raw {
	case "HELD", "PAYMENT_PENDING":
		if paymentFailed {
			return "FAILED"
		}
		return "PENDING"
	case "PAID", "ISSUED":
		return "PAID"
	case "EXPIRED":
		return "EXPIRED"
	default: // CANCELLED, REFUNDING, REFUNDED
		return "CANCELLED"
	}
}

func (h *Handler) getOrder(w http.ResponseWriter, r *http.Request) {
	p, ok := authx.PrincipalFromHeaders(r.Header)
	if !ok {
		httpx.WriteProblem(w, http.StatusUnauthorized, "ban can dang nhap de xem don hang")
		return
	}
	d, err := h.orders.Execute(r.Context(), r.PathValue("orderID"), p.ID)
	switch {
	case err == nil:
	case errors.Is(err, domain.ErrOrderNotFound):
		httpx.WriteProblem(w, http.StatusNotFound, "khong tim thay don hang")
		return
	default:
		h.log.Error("doc don hang that bai", "err", err)
		httpx.WriteProblem(w, http.StatusServiceUnavailable, "he thong dang ban, vui long thu lai")
		return
	}

	o := d.Order
	tierName, quantity := "", 0
	tierByType := map[string]string{}
	items := make([]map[string]any, 0, len(o.Items))
	for _, it := range o.Items {
		quantity += it.Quantity
		tierByType[it.TicketTypeID] = it.TierName
		if tierName == "" {
			tierName = it.TierName
		} else if tierName != it.TierName {
			tierName = "Nhieu hang ve"
		}
		items = append(items, map[string]any{
			"ticket_type_id": it.TicketTypeID, "tier_name": it.TierName,
			"quantity": it.Quantity, "unit_cents": it.UnitCents,
		})
	}
	tickets := make([]map[string]any, 0, len(d.Tickets))
	for _, t := range d.Tickets {
		tickets = append(tickets, map[string]any{
			"ticketId": t.ID, "ticketCode": t.JTI, "tierName": tierByType[t.TicketTypeID],
			"attendeeName": "", "status": t.Status,
		})
	}

	resp := map[string]any{
		"order_id":       o.OrderID,
		"event_id":       o.EventID,
		"event_title":    o.EventTitle,
		"venue":          o.Venue,
		"tier_name":      tierName,
		"quantity":       quantity,
		"total_amount":   o.TotalCents, // VND khong co don vi le: amount == cents
		"total_cents":    o.TotalCents,
		"currency":       o.Currency,
		"status":         orderStatusForClient(o.Status, o.PaymentFailed),
		"order_status":   o.Status,
		"expires_at":     o.ExpiresAt,
		"created_at":     o.CreatedAt,
		"items":          items,
		"issued_tickets": tickets,
	}
	if o.HoldID != "" {
		resp["hold_id"] = o.HoldID
	}
	if o.PaidAt != nil {
		resp["paid_at"] = o.PaidAt
	}
	if o.PaymentProvider != "" {
		resp["payment_method"] = o.PaymentProvider
	}
	if o.PaymentFailed {
		resp["failure_reason"] = "payment_failed"
	}
	// Trang thai don doi theo thanh toan: khong cho cache dung chung.
	w.Header().Set("Cache-Control", "private, no-store")
	httpx.WriteJSON(w, http.StatusOK, resp)
}

func (h *Handler) cancelHold(w http.ResponseWriter, r *http.Request) {
	p, ok := authx.PrincipalFromHeaders(r.Header)
	if !ok {
		httpx.WriteProblem(w, http.StatusUnauthorized, "ban can dang nhap de huy giu ve")
		return
	}
	err := h.cancel.Execute(r.Context(), r.PathValue("holdID"), p.ID)
	switch {
	case err == nil:
		w.WriteHeader(http.StatusNoContent)
	case errors.Is(err, domain.ErrOrderNotFound):
		httpx.WriteProblem(w, http.StatusNotFound, "khong tim thay lenh giu ve")
	case errors.Is(err, domain.ErrHoldNotCancellable):
		httpx.WriteProblemType(w, http.StatusConflict, ProblemHoldNotCancellable,
			"don dang thanh toan hoac da thanh toan, khong the huy giu ve")
	default:
		h.log.Error("huy hold that bai", "err", err)
		httpx.WriteProblem(w, http.StatusServiceUnavailable, "he thong dang ban, vui long thu lai")
	}
}

func (h *Handler) availability(w http.ResponseWriter, r *http.Request) {
	eventID := r.PathValue("eventID")
	tiers, err := h.availCache.get(r.Context(), eventID, func(ctx context.Context) ([]app.TierAvailability, error) {
		return h.avail.Execute(ctx, eventID)
	})
	switch {
	case err == nil:
		w.Header().Set("Cache-Control", "public, max-age=2")
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"event_id": eventID, "tiers": tiers})
	case errors.Is(err, domain.ErrEventNotFound):
		httpx.WriteProblem(w, http.StatusNotFound, "su kien khong ton tai hoac chua mo ban")
	default:
		h.log.Error("doc ton kho cong khai that bai", "event", eventID, "err", err)
		httpx.WriteProblem(w, http.StatusServiceUnavailable, "he thong dang ban, vui long thu lai")
	}
}

// availCache gom cac lan hoi ton kho cua cung mot su kien trong availabilityTTL: moi su kien co khoa
// rieng nen chi MOT request di xuong Postgres/Redis, cac request con lai cho roi dung ket qua do.
type availCache struct {
	mu      sync.Mutex
	entries map[string]*availEntry
	now     func() time.Time
}

type availEntry struct {
	mu   sync.Mutex
	at   time.Time
	val  []app.TierAvailability
	fill bool
}

func (c *availCache) get(ctx context.Context, eventID string, load func(context.Context) ([]app.TierAvailability, error)) ([]app.TierAvailability, error) {
	c.mu.Lock()
	e, ok := c.entries[eventID]
	if !ok {
		if len(c.entries) >= maxAvailabilityEntries {
			c.sweepLocked()
		}
		if len(c.entries) >= maxAvailabilityEntries {
			c.mu.Unlock()
			return load(ctx) // day: khong cache them, tranh phinh bo nho
		}
		e = &availEntry{}
		c.entries[eventID] = e
	}
	c.mu.Unlock()

	e.mu.Lock()
	defer e.mu.Unlock()
	if e.fill && c.now().Sub(e.at) < availabilityTTL {
		return e.val, nil
	}
	val, err := load(ctx)
	if err != nil {
		// Khong cache loi (ke ca "khong ton tai") nhung van thu hoi lai o lan sau.
		e.fill = false
		return nil, err
	}
	e.val, e.at, e.fill = val, c.now(), true
	return val, nil
}

// sweepLocked bo cac muc het han hoac chua tung nap duoc (loi). Goi khi c.mu da giu.
func (c *availCache) sweepLocked() {
	for k, e := range c.entries {
		if !e.mu.TryLock() {
			continue // dang duoc dung
		}
		stale := !e.fill || c.now().Sub(e.at) >= availabilityTTL
		e.mu.Unlock()
		if stale {
			delete(c.entries, k)
		}
	}
}
