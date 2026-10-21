package proxy_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/eventflow/eventflow/services/gateway/internal/config"
)

// Contract test cho frontend-real-api: AC-25 (CORS) va AC-26 (route /api/v1/events/public cong khai).
// Dung lai fixture cua gateway_test.go (cung package proxy_test).

func headerHas(h http.Header, name, want string) bool {
	for _, v := range h.Values(name) {
		for _, part := range strings.Split(v, ",") {
			if strings.EqualFold(strings.TrimSpace(part), want) {
				return true
			}
		}
	}
	return false
}

func preflight(t *testing.T, f *fixture, origin, path string) *http.Response {
	t.Helper()
	return f.do(t, http.MethodOptions, path, "", map[string]string{
		"Origin":                         origin,
		"Access-Control-Request-Method":  "GET",
		"Access-Control-Request-Headers": "authorization, x-queue-token, if-none-match",
	}, "")
}

// AC-25: preflight cua origin duoc phep cho X-Queue-Token va If-None-Match.
func TestGateway_CORS_PreflightChoPhepHeaderCuaPhongCho(t *testing.T) {
	f := newFixture(t)
	resp := preflight(t, f, "http://localhost:3000", "/v1/events/e1/queue/status")

	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("preflight: status %d", resp.StatusCode)
	}
	for _, want := range []string{"X-Queue-Token", "If-None-Match", "Authorization", "Content-Type", "Idempotency-Key", "Last-Event-ID"} {
		if !headerHas(resp.Header, "Access-Control-Allow-Headers", want) {
			t.Errorf("Allow-Headers thieu %q: %q", want, resp.Header.Get("Access-Control-Allow-Headers"))
		}
	}
	if f.up.count() != 0 {
		t.Fatal("preflight khong duoc toi upstream")
	}
}

// AC-25: Expose-Headers co ETag va X-Poll-After-Ms (de JS doc duoc), giu X-Request-Id, Retry-After.
func TestGateway_CORS_ExposeHeaders(t *testing.T) {
	f := newFixture(t)
	origin := "http://localhost:3000"

	pre := preflight(t, f, origin, "/v1/events/e1/queue/status")
	real := f.do(t, http.MethodGet, "/api/v1/events/slug/live-show", "", map[string]string{"Origin": origin}, "")

	for name, resp := range map[string]*http.Response{"preflight": pre, "request that": real} {
		for _, want := range []string{"ETag", "X-Poll-After-Ms", "X-Request-Id", "Retry-After"} {
			if !headerHas(resp.Header, "Access-Control-Expose-Headers", want) {
				t.Errorf("%s: Expose-Headers thieu %q: %q", name, want, resp.Header.Get("Access-Control-Expose-Headers"))
			}
		}
	}
}

// AC-25: van KHONG co Allow-Credentials (web ngung gui credentials:"include").
func TestGateway_CORS_KhongCoAllowCredentials(t *testing.T) {
	f := newFixture(t)
	origin := "http://localhost:3000"
	pre := preflight(t, f, origin, "/v1/events/e1/queue/status")
	real := f.do(t, http.MethodPost, "/api/v1/auth/login", "", map[string]string{"Origin": origin}, "{}")
	for name, resp := range map[string]*http.Response{"preflight": pre, "request that": real} {
		if v := resp.Header.Values("Access-Control-Allow-Credentials"); len(v) != 0 {
			t.Errorf("%s: khong duoc co Allow-Credentials, got %v", name, v)
		}
	}
}

// AC-25: origin la khong doi: 204 khong header cho phep, khong lo Expose/Allow-Headers.
func TestGateway_CORS_OriginLaKhongDoi(t *testing.T) {
	f := newFixture(t)
	resp := preflight(t, f, "https://evil.example", "/v1/events/e1/queue/status")
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("status %d", resp.StatusCode)
	}
	for _, h := range []string{"Access-Control-Allow-Origin", "Access-Control-Allow-Headers", "Access-Control-Expose-Headers", "Access-Control-Allow-Credentials"} {
		if v := resp.Header.Get(h); v != "" {
			t.Errorf("origin la nhan %s=%q", h, v)
		}
	}
}

// AC-26: GET /api/v1/events/public la route cong khai toi event-svc, giu nguyen duong dan, khong can token.
func TestGateway_EventsPublic_CongKhaiToiEventService(t *testing.T) {
	f := newFixture(t)

	resp := f.do(t, http.MethodGet, "/api/v1/events/public", "", nil, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("khong token: status %d (route phai Public)", resp.StatusCode)
	}
	got := f.up.last(t)
	if got.Upstream != config.Event || got.Path != "/api/v1/events/public" || got.Method != http.MethodGet {
		t.Fatalf("-> %s %s %s, muon event GET /api/v1/events/public", got.Upstream, got.Method, got.Path)
	}
}

// AC-26: token hong tren route cong khai bi bo qua nhu khach (khong 401).
func TestGateway_EventsPublic_TokenHongBiBoQua(t *testing.T) {
	f := newFixture(t)
	resp := f.do(t, http.MethodGet, "/api/v1/events/public", "khong.phai.jwt", nil, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("token hong: status %d", resp.StatusCode)
	}
	if got := f.up.last(t); got.Header.Get("X-Identity-Id") != "" {
		t.Fatalf("token hong khong duoc gan danh tinh, got %q", got.Header.Get("X-Identity-Id"))
	}
}

// AC-26: khong lam hong route cu: /{id} va /slug/{slug} van cong khai; danh sach organizer van can token;
// chi GET moi cong khai tren /public.
func TestGateway_EventsPublic_KhongLamHongRouteCu(t *testing.T) {
	f := newFixture(t)

	for _, p := range []string{"/api/v1/events/0b6d3b52-6f6e-4f0e-9d36-2a2f1c0a9a11", "/api/v1/events/slug/live-show"} {
		if resp := f.do(t, http.MethodGet, p, "", nil, ""); resp.StatusCode != http.StatusOK {
			t.Errorf("GET %s khong token: %d", p, resp.StatusCode)
		}
	}
	if resp := f.do(t, http.MethodGet, "/api/v1/events", "", nil, ""); resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("GET /api/v1/events khong token: %d, muon 401", resp.StatusCode)
	}
	before := f.up.count()
	if resp := f.do(t, http.MethodPost, "/api/v1/events/public", "", nil, "{}"); resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("POST /api/v1/events/public khong token: %d, muon 401 (chi GET cong khai)", resp.StatusCode)
	}
	if f.up.count() != before {
		t.Error("POST /api/v1/events/public khong token khong duoc toi upstream")
	}
}
