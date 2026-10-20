package httpadapter

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/eventflow/eventflow/libs/go/idem"
	"github.com/eventflow/eventflow/services/ticketing/internal/app"
	"github.com/eventflow/eventflow/services/ticketing/internal/domain"
	"github.com/eventflow/eventflow/services/ticketing/internal/port"
)

type fakeOrderGetter struct {
	d     app.OrderDetails
	err   error
	owner string // identity cua don; khac thi ErrOrderNotFound
}

func (f *fakeOrderGetter) Execute(_ context.Context, _, identityID string) (app.OrderDetails, error) {
	if f.err != nil {
		return app.OrderDetails{}, f.err
	}
	if f.owner != "" && f.owner != identityID {
		return app.OrderDetails{}, domain.ErrOrderNotFound
	}
	return f.d, nil
}

type fakeCancel struct {
	err   error
	calls atomic.Int32
	hold  string
	who   string
}

func (f *fakeCancel) Execute(_ context.Context, holdID, identityID string) error {
	f.calls.Add(1)
	f.hold, f.who = holdID, identityID
	return f.err
}

type fakeAvailability struct {
	tiers []app.TierAvailability
	err   error
	calls atomic.Int32
}

func (f *fakeAvailability) Execute(context.Context, string) ([]app.TierAvailability, error) {
	f.calls.Add(1)
	return f.tiers, f.err
}

func orderServer(g OrderGetter, c HoldCancelUC, a AvailabilityReader) http.Handler {
	h := New(&fakeHolds{}, orderLog()).WithOrders(g, c, a)
	mux := http.NewServeMux()
	h.Routes(mux, idem.Middleware(&memIdem{},
		idem.WithIdentity(func(r *http.Request) string { return r.Header.Get("X-Identity-Id") })))
	return mux
}

func call(h http.Handler, method, path string, hdr map[string]string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(""))
	for k, v := range hdr {
		r.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func buyer() map[string]string {
	return map[string]string{"X-Identity-Id": orderTestUser, "X-Identity-Role": "buyer"}
}

func decode(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &m); err != nil {
		t.Fatalf("body khong phai JSON: %v: %s", err, w.Body)
	}
	return m
}

func TestGetOrder_TraDonVaDichTrangThaiChoWeb(t *testing.T) {
	paid := time.Date(2026, 10, 19, 10, 0, 0, 0, time.UTC)
	cases := []struct {
		name        string
		view        port.OrderView
		wantStatus  string
		wantFailure bool
	}{
		{"dang giu", port.OrderView{Status: "HELD"}, "PENDING", false},
		{"cho thanh toan", port.OrderView{Status: "PAYMENT_PENDING"}, "PENDING", false},
		{"thanh toan that bai", port.OrderView{Status: "HELD", PaymentFailed: true, PaymentProvider: "vnpay"}, "FAILED", true},
		{"da thanh toan", port.OrderView{Status: "PAID", PaidAt: &paid, PaymentProvider: "momo"}, "PAID", false},
		{"da phat hanh ve", port.OrderView{Status: "ISSUED"}, "PAID", false},
		{"het han", port.OrderView{Status: "EXPIRED"}, "EXPIRED", false},
		{"khach huy", port.OrderView{Status: "CANCELLED"}, "CANCELLED", false},
		{"dang hoan tien", port.OrderView{Status: "REFUNDING"}, "CANCELLED", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			c.view.OrderID, c.view.EventID, c.view.EventTitle, c.view.TotalCents = "o1", "e1", "Rock Fest", 1_200_000
			c.view.Items = []port.OrderItemView{{TicketTypeID: "tt1", TierName: "VIP", Quantity: 2, UnitCents: 600_000}}
			srv := orderServer(&fakeOrderGetter{d: app.OrderDetails{Order: c.view}}, nil, nil)

			w := call(srv, http.MethodGet, "/v1/orders/o1", buyer())
			if w.Code != http.StatusOK {
				t.Fatalf("status %d: %s", w.Code, w.Body)
			}
			m := decode(t, w)
			if m["status"] != c.wantStatus || m["order_status"] != c.view.Status {
				t.Fatalf("status=%v order_status=%v, muon %s/%s", m["status"], m["order_status"], c.wantStatus, c.view.Status)
			}
			if m["tier_name"] != "VIP" || m["quantity"] != float64(2) || m["total_amount"] != float64(1_200_000) || m["event_title"] != "Rock Fest" {
				t.Fatalf("thieu truong web can: %v", m)
			}
			if _, has := m["failure_reason"]; has != c.wantFailure {
				t.Fatalf("failure_reason co=%v, muon %v", has, c.wantFailure)
			}
			if cc := w.Header().Get("Cache-Control"); !strings.Contains(cc, "no-store") {
				t.Fatalf("don khong duoc cache dung chung: %q", cc)
			}
		})
	}
}

func TestGetOrder_VeDaPhatHanhDungKeyCuaWeb(t *testing.T) {
	d := app.OrderDetails{
		Order:   port.OrderView{Status: "ISSUED", Items: []port.OrderItemView{{TicketTypeID: "tt1", TierName: "VIP", Quantity: 1}}},
		Tickets: []domain.Ticket{{ID: "t1", JTI: "jti-1", TicketTypeID: "tt1", Status: domain.TicketStatusIssued}},
	}
	m := decode(t, call(orderServer(&fakeOrderGetter{d: d}, nil, nil), http.MethodGet, "/v1/orders/o1", buyer()))
	tickets, _ := m["issued_tickets"].([]any)
	if len(tickets) != 1 {
		t.Fatalf("issued_tickets = %v", m["issued_tickets"])
	}
	t0 := tickets[0].(map[string]any)
	if t0["ticketId"] != "t1" || t0["ticketCode"] != "jti-1" || t0["tierName"] != "VIP" {
		t.Fatalf("ve = %v", t0)
	}
}

func TestGetOrder_MaLoi(t *testing.T) {
	cases := []struct {
		name string
		hdr  map[string]string
		g    *fakeOrderGetter
		want int
	}{
		{"chua dang nhap", nil, &fakeOrderGetter{}, http.StatusUnauthorized},
		{"don cua nguoi khac", buyer(), &fakeOrderGetter{owner: "22222222-2222-4222-8222-222222222222"}, http.StatusNotFound},
		{"khong ton tai", buyer(), &fakeOrderGetter{err: domain.ErrOrderNotFound}, http.StatusNotFound},
		{"loi ha tang", buyer(), &fakeOrderGetter{err: errors.New("pq: mat ket noi 10.0.0.5")}, http.StatusServiceUnavailable},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			w := call(orderServer(c.g, nil, nil), http.MethodGet, "/v1/orders/o1", c.hdr)
			if w.Code != c.want {
				t.Fatalf("status %d, muon %d", w.Code, c.want)
			}
			if strings.Contains(w.Body.String(), "10.0.0.5") {
				t.Fatal("loi noi bo bi lo ra client")
			}
		})
	}
}

func TestCancelHold(t *testing.T) {
	withKey := func() map[string]string {
		h := buyer()
		h[idem.Header] = "key-1"
		return h
	}
	cases := []struct {
		name string
		hdr  map[string]string
		err  error
		want int
		typ  string
	}{
		{"thanh cong", withKey(), nil, http.StatusNoContent, ""},
		{"khong ton tai hoac cua nguoi khac", withKey(), domain.ErrOrderNotFound, http.StatusNotFound, ""},
		{"dang thanh toan", withKey(), domain.ErrHoldNotCancellable, http.StatusConflict, ProblemHoldNotCancellable},
		{"loi ha tang", withKey(), errors.New("redis down"), http.StatusServiceUnavailable, ""},
		{"thieu Idempotency-Key (BR-O5)", buyer(), nil, http.StatusBadRequest, ""},
		{"chua dang nhap", map[string]string{idem.Header: "key-1"}, nil, http.StatusUnauthorized, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cancel := &fakeCancel{err: c.err}
			w := call(orderServer(nil, cancel, nil), http.MethodDelete, "/v1/holds/h1", c.hdr)
			if w.Code != c.want {
				t.Fatalf("status %d, muon %d: %s", w.Code, c.want, w.Body)
			}
			if c.typ != "" && decode(t, w)["type"] != c.typ {
				t.Fatalf("type %v, muon %s", decode(t, w)["type"], c.typ)
			}
			ranUseCase := c.want != http.StatusBadRequest && c.want != http.StatusUnauthorized
			if (cancel.calls.Load() == 1) != ranUseCase {
				t.Fatalf("use case chay %d lan", cancel.calls.Load())
			}
			if cancel.calls.Load() == 1 && (cancel.hold != "h1" || cancel.who != orderTestUser) {
				t.Fatalf("use case nhan hold=%q who=%q, muon h1/%s (tu gateway)", cancel.hold, cancel.who, orderTestUser)
			}
		})
	}
}

func TestAvailability_CongKhaiVaGomRequest(t *testing.T) {
	src := &fakeAvailability{tiers: []app.TierAvailability{
		{TicketTypeID: "tt1", Name: "VIP", Availability: domain.AvailabilityFewLeft},
		{TicketTypeID: "tt2", Name: "Std", Availability: domain.AvailabilityAvailable},
	}}
	h := New(&fakeHolds{}, orderLog()).WithOrders(nil, nil, src)
	now := time.Now()
	h.availCache.now = func() time.Time { return now }
	mux := http.NewServeMux()
	h.Routes(mux, idem.Middleware(&memIdem{}))

	// Khach vang lai, khong can danh tinh.
	for range 5 {
		w := call(mux, http.MethodGet, "/v1/events/e1/availability", nil)
		if w.Code != http.StatusOK {
			t.Fatalf("status %d: %s", w.Code, w.Body)
		}
		if cc := w.Header().Get("Cache-Control"); !strings.Contains(cc, "max-age=2") || !strings.Contains(cc, "public") {
			t.Fatalf("Cache-Control = %q", cc)
		}
		if strings.Contains(w.Body.String(), "remaining") || strings.Contains(w.Body.String(), "capacity") {
			t.Fatalf("khong duoc lo so ve chinh xac: %s", w.Body)
		}
	}
	if n := src.calls.Load(); n != 1 {
		t.Fatalf("5 request trong TTL chi duoc xuong nguon 1 lan, nhan %d", n)
	}

	now = now.Add(availabilityTTL + time.Second)
	call(mux, http.MethodGet, "/v1/events/e1/availability", nil)
	if n := src.calls.Load(); n != 2 {
		t.Fatalf("het TTL phai doc lai, nhan %d lan", n)
	}
	call(mux, http.MethodGet, "/v1/events/e2/availability", nil)
	if n := src.calls.Load(); n != 3 {
		t.Fatalf("su kien khac khong dung chung cache, nhan %d lan", n)
	}
}

func TestAvailability_LoiKhongDuocCache(t *testing.T) {
	src := &fakeAvailability{err: domain.ErrEventNotFound}
	srv := orderServer(nil, nil, src)
	for range 3 {
		if w := call(srv, http.MethodGet, "/v1/events/nope/availability", nil); w.Code != http.StatusNotFound {
			t.Fatalf("status %d, muon 404", w.Code)
		}
	}
	if src.calls.Load() != 3 {
		t.Fatalf("loi khong duoc cache: %d lan", src.calls.Load())
	}

	src2 := &fakeAvailability{err: errors.New("redis down")}
	if w := call(orderServer(nil, nil, src2), http.MethodGet, "/v1/events/e1/availability", nil); w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status %d, muon 503", w.Code)
	}
}

func TestAvailability_CacheKhongPhinhVoHan(t *testing.T) {
	src := &fakeAvailability{tiers: []app.TierAvailability{{TicketTypeID: "a"}}}
	c := &availCache{entries: map[string]*availEntry{}, now: time.Now}
	for i := range maxAvailabilityEntries + 500 {
		if _, err := c.get(context.Background(), "ev-"+strings.Repeat("x", i%7)+string(rune('a'+i%26))+time.Duration(i).String(),
			func(ctx context.Context) ([]app.TierAvailability, error) { return src.Execute(ctx, "") }); err != nil {
			t.Fatal(err)
		}
	}
	if len(c.entries) > maxAvailabilityEntries {
		t.Fatalf("cache co %d muc, qua gioi han %d", len(c.entries), maxAvailabilityEntries)
	}
}

// Quyen so huu cua route ve: truoc day bat ky ai dang nhap cung doc / phat hanh duoc ve cua don bat ky.
type fakeOwner struct {
	owned bool
	err   error
}

func (f fakeOwner) OrderOwnedBy(context.Context, string, string) (bool, error) { return f.owned, f.err }

func TestTicketRoutes_ChiChuDonDuocDocVaPhatHanh(t *testing.T) {
	cases := []struct {
		name  string
		owner OrderOwner
		hdr   map[string]string
		want  int
	}{
		{"chu don", fakeOwner{owned: true}, buyer(), http.StatusOK},
		{"nguoi khac", fakeOwner{owned: false}, buyer(), http.StatusNotFound},
		{"chua dang nhap", fakeOwner{owned: true}, nil, http.StatusUnauthorized},
		{"chua cau hinh kiem quyen (fail-closed)", nil, buyer(), http.StatusServiceUnavailable},
		{"loi ha tang", fakeOwner{err: errors.New("db down")}, buyer(), http.StatusServiceUnavailable},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := New(&fakeHolds{}, orderLog()).WithTickets(nil, nil, emptyTicketReader{})
			if c.owner != nil {
				h.WithOrderOwner(c.owner)
			}
			mux := http.NewServeMux()
			h.Routes(mux, idem.Middleware(&memIdem{}))
			w := call(mux, http.MethodGet, "/v1/orders/o1/tickets", c.hdr)
			if w.Code != c.want {
				t.Fatalf("status %d, muon %d: %s", w.Code, c.want, w.Body)
			}
		})
	}
}

type emptyTicketReader struct{}

func (emptyTicketReader) GetTicketsByOrderID(context.Context, string) ([]domain.Ticket, error) {
	return nil, nil
}

const orderTestUser = "11111111-1111-4111-8111-111111111111"

func orderLog() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }
