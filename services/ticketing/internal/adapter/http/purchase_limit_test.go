package httpadapter

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/eventflow/eventflow/libs/go/idem"
	"github.com/eventflow/eventflow/services/ticketing/internal/domain"
	"github.com/eventflow/eventflow/services/ticketing/internal/port"
)

// BR-O4: hai loi 422 phai phan biet duoc bang "type" -- client hien thong bao khac nhau
// (chon lai so luong / khong mua them duoc) va khong duoc doan tu cau detail.
func TestCreateHold_BRO4_Hai422CoTypeRieng(t *testing.T) {
	cases := []struct {
		err      error
		wantType string
	}{
		{domain.ErrQuantityNotAllowed, ProblemQuantityNotAllowed},
		{domain.ErrPerIdentityLimit, ProblemPerIdentityLimit},
		{fmt.Errorf("persist hold: %w", domain.ErrPerIdentityLimit), ProblemPerIdentityLimit},
	}
	for _, tc := range cases {
		t.Run(tc.wantType, func(t *testing.T) {
			w := post(newServer(&fakeHolds{err: tc.err}), "key-1", holdBody)
			if w.Code != http.StatusUnprocessableEntity {
				t.Fatalf("status = %d, muon 422", w.Code)
			}
			var body map[string]any
			if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
				t.Fatalf("body khong phai JSON: %v", err)
			}
			if body["type"] != tc.wantType {
				t.Fatalf("type = %v, muon %s", body["type"], tc.wantType)
			}
		})
	}
}

type fakeLimits struct {
	out      port.PurchaseLimit
	err      error
	calls    int
	identity string
	event    string
}

func (f *fakeLimits) Execute(_ context.Context, eventID, identityID string) (port.PurchaseLimit, error) {
	f.calls++
	f.event, f.identity = eventID, identityID
	return f.out, f.err
}

func newLimitServer(l PurchaseLimitReader) http.Handler {
	h := New(&fakeHolds{}, slog.New(slog.NewTextHandler(io.Discard, nil))).WithPurchaseLimits(l)
	mux := http.NewServeMux()
	h.Routes(mux, idem.Middleware(&memIdem{}))
	return mux
}

func getLimit(h http.Handler, identity string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(http.MethodGet, "/v1/events/e1/purchase-limit", nil)
	if identity != "" {
		r.Header.Set("X-Identity-Id", identity)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestPurchaseLimit_TraHanMucChoDungKhach(t *testing.T) {
	l := &fakeLimits{out: port.PurchaseLimit{MaxPerOrder: 4, MaxPerIdentity: 4, Used: 1, MaxSelectable: 3}}
	w := getLimit(newLimitServer(l), "u1")

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, muon 200", w.Code)
	}
	if l.event != "e1" || l.identity != "u1" {
		t.Errorf("phai doc han muc cua dung su kien/khach, nhan %s/%s", l.event, l.identity)
	}
	var got map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("body khong phai JSON: %v", err)
	}
	want := map[string]any{
		"max_per_order": float64(4), "max_per_identity": float64(4), "used": float64(1),
		"max_selectable": float64(3), "has_active_hold": false,
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %v, muon %v", k, got[k], v)
		}
	}
	// Han muc doi theo tung lan giu ghe/thanh toan: CDN hay proxy khong duoc cache dung chung.
	if cc := w.Header().Get("Cache-Control"); cc != "private, no-store" {
		t.Errorf("Cache-Control = %q, muon private, no-store", cc)
	}
}

// Doc khong phai ghi: khong doi Idempotency-Key (BR-O5 chi ap cho API ghi).
func TestPurchaseLimit_KhongCanIdempotencyKey(t *testing.T) {
	if w := getLimit(newLimitServer(&fakeLimits{}), "u1"); w.Code != http.StatusOK {
		t.Fatalf("status = %d, muon 200", w.Code)
	}
}

func TestPurchaseLimit_LoiDuocDichDungMa(t *testing.T) {
	cases := []struct {
		name     string
		identity string
		err      error
		want     int
		wantCall bool
	}{
		{"thieu dinh danh", "", nil, http.StatusUnauthorized, false},
		{"su kien khong dang ban", "u1", domain.ErrEventNotFound, http.StatusNotFound, true},
		{"loi ha tang", "u1", errors.New("mat ket noi"), http.StatusServiceUnavailable, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			l := &fakeLimits{err: tc.err}
			w := getLimit(newLimitServer(l), tc.identity)
			if w.Code != tc.want {
				t.Fatalf("status = %d, muon %d", w.Code, tc.want)
			}
			if (l.calls > 0) != tc.wantCall {
				t.Errorf("goi use case %d lan, mong doi goi=%v", l.calls, tc.wantCall)
			}
		})
	}
}

// Khong cau hinh nguon han muc thi route khong ton tai (khong 500 vi nil).
func TestPurchaseLimit_KhongCauHinhThiKhongCoRoute(t *testing.T) {
	h := New(&fakeHolds{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	mux := http.NewServeMux()
	h.Routes(mux, idem.Middleware(&memIdem{}))
	if w := getLimit(mux, "u1"); w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, muon 404", w.Code)
	}
}
