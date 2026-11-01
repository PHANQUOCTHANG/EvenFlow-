package httpadapter

import (
	"context"
	"errors"
	"net/http"
	"regexp"
	"sync"
	"time"

	"github.com/eventflow/eventflow/libs/go/authx"
	"github.com/eventflow/eventflow/libs/go/httpx"
	"github.com/eventflow/eventflow/services/ticketing/internal/app"
	"github.com/eventflow/eventflow/services/ticketing/internal/domain"
	"github.com/eventflow/eventflow/services/ticketing/internal/port"
)

// OpsInventoryReader / OrganizerSalesReader la use case cua war room Ops va doanh so organizer.
type OpsInventoryReader interface {
	Execute(ctx context.Context, eventID string) ([]port.TierInventoryRow, error)
}

type OrganizerSalesReader interface {
	Execute(ctx context.Context, eventID string) (app.OrganizerSales, error)
}

const (
	// opsCacheTTL: moi su kien toi da mot truy van snapshot moi 2 giay (AC-17).
	opsCacheTTL = 2 * time.Second
	// opsLoadTimeout: truy van chay tren context tach khoi request, gioi han rieng.
	opsLoadTimeout     = 2 * time.Second
	maxOpsCacheEntries = 1024

	problemForbidden = "/problems/forbidden"
	problemNotFound  = "/problems/not-found"
)

var opsEventUUID = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// opsAPI duoc nhung vao Handler; route chi duoc dang ky khi WithOps da goi.
type opsAPI struct {
	opsInv     OpsInventoryReader
	opsSales   OrganizerSalesReader
	invCache   *ttlCache[[]port.TierInventoryRow]
	salesCache *ttlCache[app.OrganizerSales]
}

// WithOps bat GET /v1/ops/events/{id}/inventory (role ops) va GET /v1/organizer/events/{id}/sales
// (organizer so huu su kien).
func (h *Handler) WithOps(inv OpsInventoryReader, sales OrganizerSalesReader) *Handler {
	h.opsInv, h.opsSales = inv, sales
	h.invCache = newTTLCache[[]port.TierInventoryRow](opsCacheTTL)
	h.salesCache = newTTLCache[app.OrganizerSales](opsCacheTTL)
	return h
}

func (h *Handler) registerOpsRoutes(mux *http.ServeMux) {
	if h.opsInv != nil {
		mux.HandleFunc("GET /v1/ops/events/{eventID}/inventory", h.opsInventory)
	}
	if h.opsSales != nil {
		mux.HandleFunc("GET /v1/organizer/events/{eventID}/sales", h.organizerSales)
	}
}

// opsPrincipal: 401 thieu danh tinh, 403 sai role, 400 eventID khong phai UUID.
func opsPrincipal(w http.ResponseWriter, r *http.Request, role string) (authx.Principal, string, bool) {
	p, ok := authx.PrincipalFromHeaders(r.Header)
	if !ok {
		httpx.WriteProblem(w, http.StatusUnauthorized, "ban can dang nhap")
		return authx.Principal{}, "", false
	}
	if p.Role != role {
		httpx.WriteProblemType(w, http.StatusForbidden, problemForbidden, "ban khong co quyen xem du lieu nay")
		return authx.Principal{}, "", false
	}
	eventID := r.PathValue("eventID")
	if !opsEventUUID.MatchString(eventID) {
		httpx.WriteProblem(w, http.StatusBadRequest, "ma su kien khong hop le")
		return authx.Principal{}, "", false
	}
	return p, eventID, true
}

type opsTierJSON struct {
	TicketTypeID string `json:"ticket_type_id"`
	Name         string `json:"name"`
	Quota        int    `json:"quota"`
	Available    int    `json:"available"`
	Held         int    `json:"held"`
	Paid         int    `json:"paid"`
	Issued       int    `json:"issued"`
}

func (h *Handler) opsInventory(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	_, eventID, ok := opsPrincipal(w, r, "ops")
	if !ok {
		return
	}
	rows, filledAt, err := h.invCache.get(r.Context(), eventID, h.opsInv.Execute)
	switch {
	case errors.Is(err, domain.ErrEventNotFound):
		httpx.WriteProblemType(w, http.StatusNotFound, problemNotFound, "khong tim thay su kien")
		return
	case err != nil:
		h.log.Error("doc ton kho cho Ops that bai", "event", eventID, "err", err)
		httpx.WriteProblem(w, http.StatusServiceUnavailable, "he thong dang ban, vui long thu lai")
		return
	}
	tiers := make([]opsTierJSON, 0, len(rows))
	for _, t := range rows {
		tiers = append(tiers, opsTierJSON(t))
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"event_id":     eventID,
		"generated_at": filledAt.UTC().Format(time.RFC3339),
		"tiers":        tiers,
	})
}

type salesTierJSON struct {
	TicketTypeID string `json:"ticket_type_id"`
	Name         string `json:"name"`
	PriceCents   int64  `json:"price_cents"`
	Quota        int    `json:"quota"`
	Sold         int    `json:"sold"`
	ActiveHolds  int    `json:"active_holds"`
	RevenueCents int64  `json:"revenue_cents"`
}

type salesPaymentJSON struct {
	Provider    string `json:"provider"`
	Orders      int    `json:"orders"`
	AmountCents int64  `json:"amount_cents"`
}

type salesTotalsJSON struct {
	GrossRevenueCents int64 `json:"gross_revenue_cents"`
	TicketsSold       int   `json:"tickets_sold"`
	Quota             int   `json:"quota"`
	PaidOrders        int   `json:"paid_orders"`
	ActiveHolds       int   `json:"active_holds"`
}

func (h *Handler) organizerSales(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	p, eventID, ok := opsPrincipal(w, r, "organizer")
	if !ok {
		return
	}
	s, filledAt, err := h.salesCache.get(r.Context(), eventID, h.opsSales.Execute)
	// Khong ton tai va khong so huu cho CUNG mot phan hoi: khong lo su kien co ton tai (nhu requireOwner).
	// Quyen so huu duoc kiem tren MOI request, ke ca khi ban ghi lay tu cache.
	if errors.Is(err, domain.ErrEventNotFound) || (err == nil && s.OrganizerID != p.ID) {
		httpx.WriteProblemType(w, http.StatusNotFound, problemNotFound, "khong tim thay su kien")
		return
	}
	if err != nil {
		h.log.Error("doc doanh so organizer that bai", "event", eventID, "err", err)
		httpx.WriteProblem(w, http.StatusServiceUnavailable, "he thong dang ban, vui long thu lai")
		return
	}
	tiers := make([]salesTierJSON, 0, len(s.Tiers))
	for _, t := range s.Tiers {
		tiers = append(tiers, salesTierJSON(t))
	}
	payments := make([]salesPaymentJSON, 0, len(s.Payments))
	for _, pm := range s.Payments {
		payments = append(payments, salesPaymentJSON(pm))
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"event_id":          eventID,
		"generated_at":      filledAt.UTC().Format(time.RFC3339),
		"currency":          "VND",
		"totals":            salesTotalsJSON(s.Totals),
		"tiers":             tiers,
		"payment_breakdown": payments,
	})
}

// ttlCache gom cac request cung su kien trong ttl: moi su kien co khoa rieng nen chi MOT truy van chay,
// cac request khac cho roi dung ket qua do (single-flight). Khac availCache:
//   - loader chay tren context.WithoutCancel + timeout rieng: caller dau huy khong lam hong nguoi cho;
//   - luu thoi diem nap (generated_at trung thuc);
//   - "khong ton tai" cung duoc cache trong ttl (chan vong lap UUID ngau nhien), loi khac thi khong.
type ttlCache[T any] struct {
	mu      sync.Mutex
	entries map[string]*ttlEntry[T]
	ttl     time.Duration
	now     func() time.Time
}

type ttlEntry[T any] struct {
	mu       sync.Mutex
	fill     bool
	at       time.Time
	val      T
	notFound bool
}

func newTTLCache[T any](ttl time.Duration) *ttlCache[T] {
	return &ttlCache[T]{entries: map[string]*ttlEntry[T]{}, ttl: ttl, now: time.Now}
}

func (c *ttlCache[T]) load(ctx context.Context, key string, f func(context.Context, string) (T, error)) (T, error) {
	lctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), opsLoadTimeout)
	defer cancel()
	return f(lctx, key)
}

func (c *ttlCache[T]) get(ctx context.Context, key string, f func(context.Context, string) (T, error)) (T, time.Time, error) {
	c.mu.Lock()
	e, ok := c.entries[key]
	if !ok {
		if len(c.entries) >= maxOpsCacheEntries {
			c.sweepLocked()
		}
		if len(c.entries) >= maxOpsCacheEntries {
			c.mu.Unlock()
			v, err := c.load(ctx, key, f) // day: khong cache them, tranh phinh bo nho
			return v, c.now(), err
		}
		e = &ttlEntry[T]{}
		c.entries[key] = e
	}
	c.mu.Unlock()

	e.mu.Lock()
	defer e.mu.Unlock()
	if e.fill && c.now().Sub(e.at) < c.ttl {
		if e.notFound {
			var zero T
			return zero, e.at, domain.ErrEventNotFound
		}
		return e.val, e.at, nil
	}
	v, err := c.load(ctx, key, f)
	switch {
	case errors.Is(err, domain.ErrEventNotFound):
		var zero T
		e.val, e.notFound, e.at, e.fill = zero, true, c.now(), true
		return zero, e.at, err
	case err != nil:
		e.fill = false
		var zero T
		return zero, time.Time{}, err
	}
	e.val, e.notFound, e.at, e.fill = v, false, c.now(), true
	return v, e.at, nil
}

// sweepLocked bo cac muc het han hoac chua nap duoc. Goi khi c.mu da giu.
func (c *ttlCache[T]) sweepLocked() {
	for k, e := range c.entries {
		if !e.mu.TryLock() {
			continue
		}
		stale := !e.fill || c.now().Sub(e.at) >= c.ttl
		e.mu.Unlock()
		if stale {
			delete(c.entries, k)
		}
	}
}
