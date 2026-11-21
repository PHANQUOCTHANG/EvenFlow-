package httpadapter_test

// ops-warroom AC-14..AC-17 (ticketing ops inventory + organizer sales), black box over HTTP.
//
// ASSUMED INTERFACES (names from plan.md WP2; shapes chosen by the test designer):
//   package port:
//     type OpsReader interface {
//       EventExists(ctx, eventID string) (bool, error)
//       EventOwner(ctx, eventID string) (organizerID string, err error)   // domain.ErrEventNotFound when absent
//       TierInventory(ctx, eventID string) ([]port.TierInventoryRow, error)
//       EventSales(ctx, eventID string) (port.SalesRows, error)
//     }
//     type TierInventoryRow struct { TicketTypeID, Name string; Quota, Available, Held, Paid, Issued int }
//     type SalesRows struct { Tiers []TierSalesRow; Payments []PaymentRow; PaidOrders int }
//     type TierSalesRow struct { TicketTypeID, Name string; PriceCents int64; Quota, Sold, ActiveHolds int; RevenueCents int64 }
//     type PaymentRow struct { Provider string; Orders int; AmountCents int64 }
//   package app:  func NewGetOpsInventory(port.OpsReader) *GetOpsInventory ; func NewGetOrganizerSales(port.OpsReader) *GetOrganizerSales
//   package httpadapter: (*Handler).WithOps(inv, sales) registering the two routes (plan WP2); httpadapter.New(nil, log) is valid.
// The SQL-level definitions of held/paid/issued/sold/revenue are covered by ops_repository_integration_test.go (docker).
// The 2 s statement timeout lives in the Postgres adapter and is not asserted here.

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	httpadapter "github.com/eventflow/eventflow/services/ticketing/internal/adapter/http"
	"github.com/eventflow/eventflow/services/ticketing/internal/app"
	"github.com/eventflow/eventflow/services/ticketing/internal/domain"
	"github.com/eventflow/eventflow/services/ticketing/internal/port"
)

const (
	opsEv      = "11111111-1111-4111-8111-111111111111"
	opsEv2     = "55555555-5555-4555-8555-555555555555"
	organizerA = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	organizerB = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
	opsUser    = "99999999-9999-4999-8999-999999999999"
)

type fakeOpsReader struct {
	owners map[string]string // eventID -> organizer
	err    error
	delay  time.Duration

	invCalls   atomic.Int64
	salesCalls atomic.Int64
	tiers      []port.TierInventoryRow
	sales      port.SalesRows
}

func (f *fakeOpsReader) EventExists(_ context.Context, id string) (bool, error) {
	_, ok := f.owners[id]
	return ok, f.err
}

func (f *fakeOpsReader) EventOwner(_ context.Context, id string) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	o, ok := f.owners[id]
	if !ok {
		return "", domain.ErrEventNotFound
	}
	return o, nil
}

func (f *fakeOpsReader) TierInventory(_ context.Context, id string) ([]port.TierInventoryRow, error) {
	f.invCalls.Add(1)
	time.Sleep(f.delay)
	if f.err != nil {
		return nil, f.err
	}
	if _, ok := f.owners[id]; !ok {
		return nil, domain.ErrEventNotFound
	}
	return f.tiers, nil
}

func (f *fakeOpsReader) EventSales(_ context.Context, id string) (port.SalesRows, error) {
	f.salesCalls.Add(1)
	time.Sleep(f.delay)
	if f.err != nil {
		return port.SalesRows{}, f.err
	}
	return f.sales, nil
}

func newOpsMux(r *fakeOpsReader) *http.ServeMux {
	mux := http.NewServeMux()
	h := httpadapter.New(nil, slog.New(slog.NewTextHandler(io.Discard, nil))).
		WithOps(app.NewGetOpsInventory(r), app.NewGetOrganizerSales(r))
	h.Routes(mux)
	return mux
}

func opsGet(mux http.Handler, path, id, role string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if role != "" {
		req.Header.Set("X-Identity-Id", id)
		req.Header.Set("X-Identity-Role", role)
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func invPath(ev string) string   { return "/v1/ops/events/" + ev + "/inventory" }
func salesPath(ev string) string { return "/v1/organizer/events/" + ev + "/sales" }

func baseReader() *fakeOpsReader {
	return &fakeOpsReader{
		owners: map[string]string{opsEv: organizerA, opsEv2: organizerA},
		tiers: []port.TierInventoryRow{
			{TicketTypeID: "t1", Name: "Standard", Quota: 1000, Available: 410, Held: 40, Paid: 300, Issued: 250},
			{TicketTypeID: "t2", Name: "VIP", Quota: 200, Available: 0, Held: 0, Paid: 0, Issued: 200},
		},
		sales: port.SalesRows{
			PaidOrders: 455,
			Tiers: []port.TierSalesRow{
				{TicketTypeID: "t1", Name: "Standard", PriceCents: 150000, Quota: 1000, Sold: 700, ActiveHolds: 40, RevenueCents: 105000000},
				{TicketTypeID: "t2", Name: "VIP", PriceCents: 500000, Quota: 200, Sold: 120, ActiveHolds: 8, RevenueCents: 18000000},
			},
			Payments: []port.PaymentRow{
				{Provider: "vnpay", Orders: 300, AmountCents: 80000000},
				{Provider: "momo", Orders: 155, AmountCents: 43000000},
			},
		},
	}
}

func jsonOf(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
		t.Fatalf("not JSON: %v %s", err, rec.Body)
	}
	return m
}

// ---------- AC-14 inventory ----------

func TestOpsInventory_AC14_ShapeAndNoStore(t *testing.T) {
	mux := newOpsMux(baseReader())
	rec := opsGet(mux, invPath(opsEv), opsUser, "ops")
	if rec.Code != 200 {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("Cache-Control no-store missing")
	}
	m := jsonOf(t, rec)
	if m["event_id"] != opsEv {
		t.Fatalf("event_id %v", m["event_id"])
	}
	if _, err := time.Parse(time.RFC3339, m["generated_at"].(string)); err != nil {
		t.Fatalf("generated_at: %v", err)
	}
	tiers := m["tiers"].([]any)
	if len(tiers) != 2 {
		t.Fatalf("tiers %d", len(tiers))
	}
	t0 := tiers[0].(map[string]any)
	want := map[string]any{"ticket_type_id": "t1", "name": "Standard", "quota": 1000.0, "available": 410.0,
		"held": 40.0, "paid": 300.0, "issued": 250.0}
	for k, v := range want {
		if t0[k] != v {
			t.Errorf("tier[0].%s = %v, want %v", k, t0[k], v)
		}
	}
}

func TestOpsInventory_AC14_EmptyTiersIsArrayNotNull(t *testing.T) {
	r := baseReader()
	r.tiers = nil
	rec := opsGet(newOpsMux(r), invPath(opsEv), opsUser, "ops")
	if rec.Code != 200 {
		t.Fatalf("%d", rec.Code)
	}
	if arr, ok := jsonOf(t, rec)["tiers"].([]any); !ok || len(arr) != 0 {
		t.Fatalf("tiers must be [], got %v", jsonOf(t, rec)["tiers"])
	}
}

func TestOpsInventory_AC16_ErrorsAndRoles(t *testing.T) {
	mux := newOpsMux(baseReader())
	for _, role := range []string{"buyer", "organizer", "moderator"} {
		if rec := opsGet(mux, invPath(opsEv), organizerA, role); rec.Code != 403 {
			t.Errorf("role %s: %d, want 403", role, rec.Code)
		}
	}
	if rec := opsGet(mux, invPath(opsEv), "", ""); rec.Code != 401 {
		t.Errorf("no principal: %d", rec.Code)
	}
	if rec := opsGet(mux, invPath("nope"), opsUser, "ops"); rec.Code != 400 {
		t.Errorf("non-uuid: %d", rec.Code)
	}
	if rec := opsGet(mux, invPath("77777777-7777-4777-8777-777777777777"), opsUser, "ops"); rec.Code != 404 {
		t.Errorf("unknown event: %d", rec.Code)
	}
	r := baseReader()
	r.err = errors.New("db down")
	if rec := opsGet(newOpsMux(r), invPath(opsEv), opsUser, "ops"); rec.Code != 503 {
		t.Errorf("db error: %d", rec.Code)
	}
}

// ---------- AC-15 sales ----------

func TestOpsSales_AC15_ShapeTotalsAndBreakdown(t *testing.T) {
	rec := opsGet(newOpsMux(baseReader()), salesPath(opsEv), organizerA, "organizer")
	if rec.Code != 200 {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("Cache-Control no-store missing")
	}
	m := jsonOf(t, rec)
	if m["currency"] != "VND" || m["event_id"] != opsEv {
		t.Fatalf("header fields: %v", m)
	}
	tot := m["totals"].(map[string]any)
	want := map[string]any{
		"gross_revenue_cents": 123000000.0, // 105,000,000 + 18,000,000
		"tickets_sold":        820.0,       // 700 + 120
		"quota":               1200.0,
		"active_holds":        48.0,
	}
	for k, v := range want {
		if tot[k] != v {
			t.Errorf("totals.%s = %v, want %v", k, tot[k], v)
		}
	}
	if _, ok := tot["paid_orders"]; !ok {
		t.Error("totals.paid_orders missing")
	}
	tiers := m["tiers"].([]any)
	t0 := tiers[0].(map[string]any)
	for k, v := range map[string]any{"ticket_type_id": "t1", "name": "Standard", "price_cents": 150000.0,
		"quota": 1000.0, "sold": 700.0, "active_holds": 40.0, "revenue_cents": 105000000.0} {
		if t0[k] != v {
			t.Errorf("tier[0].%s = %v, want %v", k, t0[k], v)
		}
	}
	pb := m["payment_breakdown"].([]any)
	if len(pb) != 2 {
		t.Fatalf("payment_breakdown %v", pb)
	}
	p0 := pb[0].(map[string]any)
	if p0["provider"] != "vnpay" || p0["orders"] != 300.0 || p0["amount_cents"] != 80000000.0 {
		t.Fatalf("payment[0] %v", p0)
	}
	if tot["paid_orders"] != 455.0 {
		t.Errorf("paid_orders = %v, want 455", tot["paid_orders"])
	}
}

func TestOpsSales_AC15_EmptyEventIsZeroNot404(t *testing.T) {
	r := baseReader()
	r.sales = port.SalesRows{}
	rec := opsGet(newOpsMux(r), salesPath(opsEv), organizerA, "organizer")
	if rec.Code != 200 {
		t.Fatalf("%d", rec.Code)
	}
	m := jsonOf(t, rec)
	tot := m["totals"].(map[string]any)
	if tot["gross_revenue_cents"] != 0.0 || tot["tickets_sold"] != 0.0 || tot["paid_orders"] != 0.0 {
		t.Fatalf("totals %v", tot)
	}
	if a, ok := m["payment_breakdown"].([]any); !ok || len(a) != 0 {
		t.Fatalf("payment_breakdown must be []: %v", m["payment_breakdown"])
	}
	if a, ok := m["tiers"].([]any); !ok || len(a) != 0 {
		t.Fatalf("tiers must be []: %v", m["tiers"])
	}
}

// ---------- AC-16 ownership / roles ----------

func TestOpsSales_AC16_OwnershipIs404Indistinguishable(t *testing.T) {
	mux := newOpsMux(baseReader())
	other := opsGet(mux, salesPath(opsEv), organizerB, "organizer")
	unknown := opsGet(mux, salesPath("77777777-7777-4777-8777-777777777777"), organizerB, "organizer")
	if other.Code != 404 || unknown.Code != 404 {
		t.Fatalf("non-owner %d unknown %d, want 404/404", other.Code, unknown.Code)
	}
	if other.Body.String() != unknown.Body.String() {
		t.Errorf("responses differ, leaking existence:\n%s\n%s", other.Body, unknown.Body)
	}
}

func TestOpsSales_AC16_RolesAndInput(t *testing.T) {
	mux := newOpsMux(baseReader())
	for _, role := range []string{"buyer", "ops", "moderator"} {
		if rec := opsGet(mux, salesPath(opsEv), organizerA, role); rec.Code != 403 {
			t.Errorf("role %s: %d, want 403 (D3 owner only)", role, rec.Code)
		}
	}
	if rec := opsGet(mux, salesPath(opsEv), "", ""); rec.Code != 401 {
		t.Errorf("no principal: %d", rec.Code)
	}
	if rec := opsGet(mux, salesPath("nope"), organizerA, "organizer"); rec.Code != 400 {
		t.Errorf("non-uuid: %d", rec.Code)
	}
	r := baseReader()
	r.err = errors.New("db down")
	if rec := opsGet(newOpsMux(r), salesPath(opsEv), organizerA, "organizer"); rec.Code != 503 {
		t.Errorf("db error: %d", rec.Code)
	}
}

// ---------- AC-17 cache + single-flight ----------

func TestOpsCache_AC17_ConcurrentRequestsShareOneQueryRound(t *testing.T) {
	r := baseReader()
	r.delay = 150 * time.Millisecond
	mux := newOpsMux(r)
	var wg sync.WaitGroup
	for i := 0; i < 25; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			if rec := opsGet(mux, invPath(opsEv), opsUser, "ops"); rec.Code != 200 {
				t.Errorf("inventory %d", rec.Code)
			}
		}()
		go func() {
			defer wg.Done()
			if rec := opsGet(mux, salesPath(opsEv), organizerA, "organizer"); rec.Code != 200 {
				t.Errorf("sales %d", rec.Code)
			}
		}()
	}
	wg.Wait()
	if n := r.invCalls.Load(); n != 1 {
		t.Errorf("inventory DB rounds = %d, want 1", n)
	}
	if n := r.salesCalls.Load(); n != 1 {
		t.Errorf("sales DB rounds = %d, want 1", n)
	}
	// still inside the 2 s TTL: served from cache
	opsGet(mux, invPath(opsEv), opsUser, "ops")
	opsGet(mux, salesPath(opsEv), organizerA, "organizer")
	if r.invCalls.Load() != 1 || r.salesCalls.Load() != 1 {
		t.Errorf("cache miss inside TTL: inv=%d sales=%d", r.invCalls.Load(), r.salesCalls.Load())
	}
}

func TestOpsCache_AC17_PerEventKeyAndExpiry(t *testing.T) {
	r := baseReader()
	mux := newOpsMux(r)
	opsGet(mux, invPath(opsEv), opsUser, "ops")
	opsGet(mux, invPath(opsEv2), opsUser, "ops")
	if n := r.invCalls.Load(); n != 2 {
		t.Fatalf("two events must not share a cache entry: calls=%d", n)
	}
	time.Sleep(2200 * time.Millisecond)
	opsGet(mux, invPath(opsEv), opsUser, "ops")
	if n := r.invCalls.Load(); n != 3 {
		t.Fatalf("entry must expire after ~2 s: calls=%d", n)
	}
}

func TestOpsCache_AC17_ErrorsAreNotCached(t *testing.T) {
	r := baseReader()
	r.err = errors.New("db down")
	mux := newOpsMux(r)
	if rec := opsGet(mux, invPath(opsEv), opsUser, "ops"); rec.Code != 503 {
		t.Fatalf("%d", rec.Code)
	}
	r.err = nil
	if rec := opsGet(mux, invPath(opsEv), opsUser, "ops"); rec.Code != 200 {
		t.Fatalf("after recovery: %d (503 was cached)", rec.Code)
	}
}

// Cross-tenant leak guard: the ownership check runs before the cache, so a warm cache never serves a non-owner.
func TestOpsCache_AC16_OwnershipCheckedBeforeCache(t *testing.T) {
	r := baseReader()
	mux := newOpsMux(r)
	if rec := opsGet(mux, salesPath(opsEv), organizerA, "organizer"); rec.Code != 200 {
		t.Fatalf("owner: %d", rec.Code)
	}
	if rec := opsGet(mux, salesPath(opsEv), organizerB, "organizer"); rec.Code != 404 {
		t.Fatalf("non-owner with warm cache: %d, want 404", rec.Code)
	}
}
