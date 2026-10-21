package proxy_test

import (
	"bufio"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/eventflow/eventflow/libs/go/authx"
	"github.com/eventflow/eventflow/services/gateway/internal/config"
	"github.com/eventflow/eventflow/services/gateway/internal/proxy"
)

const (
	secret  = "gateway-test-secret-at-least-16"
	buyerID = "11111111-1111-4111-8111-111111111111"
	opsID   = "22222222-2222-4222-8222-222222222222"
)

// seen la nhung gi upstream gia nhan duoc.
type seen struct {
	Upstream string
	Method   string
	Path     string
	Header   http.Header
	Body     string
}

type upstreams struct {
	mu   sync.Mutex
	hits []seen
	srvs map[string]*httptest.Server
}

func (u *upstreams) last(t *testing.T) seen {
	t.Helper()
	u.mu.Lock()
	defer u.mu.Unlock()
	if len(u.hits) == 0 {
		t.Fatal("upstream khong nhan request nao")
	}
	return u.hits[len(u.hits)-1]
}

func (u *upstreams) count() int {
	u.mu.Lock()
	defer u.mu.Unlock()
	return len(u.hits)
}

type fixture struct {
	gw  *httptest.Server
	up  *upstreams
	now time.Time
}

func newFixture(t *testing.T, mutate ...func(*config.Config)) *fixture {
	t.Helper()
	f := &fixture{up: &upstreams{srvs: map[string]*httptest.Server{}}, now: time.Now()}
	cfg := &config.Config{
		JWTSecret:      secret,
		Upstreams:      map[string]*url.URL{},
		AllowedOrigins: []string{"http://localhost:3000"},
		Rates: map[string]config.Rate{
			config.RateAuth:      {PerSecond: 100, Burst: 100},
			config.RateQueuePoll: {PerSecond: 100, Burst: 100},
			config.RateDefault:   {PerSecond: 100, Burst: 100},
		},
		MaxBodyBytes: 1 << 10,
	}
	for _, name := range []string{config.Identity, config.Event, config.Waitingroom, config.Ticketing, config.Payment, config.Assistant} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, _ := io.ReadAll(r.Body)
			f.up.mu.Lock()
			f.up.hits = append(f.up.hits, seen{Upstream: name, Method: r.Method, Path: r.URL.Path, Header: r.Header.Clone(), Body: string(body)})
			f.up.mu.Unlock()
			// Identity cu tu dat CORS "*": gateway phai xoa.
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, name)
		}))
		t.Cleanup(srv.Close)
		f.up.srvs[name] = srv
		cfg.Upstreams[name], _ = url.Parse(srv.URL)
	}
	for _, m := range mutate {
		m(cfg)
	}
	h, err := proxy.New(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)), proxy.WithClock(func() time.Time { return f.now }))
	if err != nil {
		t.Fatal(err)
	}
	f.gw = httptest.NewServer(h)
	t.Cleanup(f.gw.Close)
	return f
}

func token(t *testing.T, id, role string, iat time.Time) string {
	t.Helper()
	tok, err := authx.SignAccessToken([]byte(secret), authx.AccessClaims{Subject: id, Role: role, ID: "jti", IssuedAt: iat})
	if err != nil {
		t.Fatal(err)
	}
	return tok
}

func (f *fixture) do(t *testing.T, method, path, bearer string, hdr map[string]string, body string) *http.Response {
	t.Helper()
	var rd io.Reader
	if body != "" {
		rd = strings.NewReader(body)
	}
	req, _ := http.NewRequest(method, f.gw.URL+path, rd)
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	return resp
}

func problemType(t *testing.T, resp *http.Response) string {
	t.Helper()
	var p struct {
		Type string `json:"type"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&p)
	return p.Type
}

// AC-1: header danh tinh do client gui khong bao gio toi duoc service.
func TestGateway_XoaDanhTinhGiaMaoTrenMoiRoute(t *testing.T) {
	f := newFixture(t)
	spoof := map[string]string{
		"X-Identity-Id": opsID, "X-Identity-Role": "ops", "X-Role": "ops",
		"X-Forwarded-For": "1.2.3.4",
	}

	// Route cong khai, khong token: khong con danh tinh nao.
	f.do(t, http.MethodGet, "/api/v1/events/abc", "", spoof, "")
	got := f.up.last(t)
	for _, k := range []string{"X-Identity-Id", "X-Identity-Role", "X-Role"} {
		if v := got.Header.Get(k); v != "" {
			t.Fatalf("route cong khai: %s = %q lot xuong upstream", k, v)
		}
	}
	if xff := got.Header.Get("X-Forwarded-For"); strings.Contains(xff, "1.2.3.4") {
		t.Fatalf("X-Forwarded-For gia mao lot xuong: %q", xff)
	}

	// Route can dang nhap: danh tinh lay tu token, khong phai tu header.
	resp := f.do(t, http.MethodPost, "/v1/events/e1/queue/join", token(t, buyerID, "buyer", f.now), spoof, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d", resp.StatusCode)
	}
	got = f.up.last(t)
	if got.Header.Get("X-Identity-Id") != buyerID || got.Header.Get("X-Identity-Role") != "buyer" {
		t.Fatalf("upstream nhan id=%q role=%q, muon tu token", got.Header.Get("X-Identity-Id"), got.Header.Get("X-Identity-Role"))
	}
	if vals := got.Header.Values("X-Identity-Id"); len(vals) != 1 {
		t.Fatalf("X-Identity-Id co %d gia tri", len(vals))
	}
	if got.Header.Get("X-Role") != "" {
		t.Fatal("X-Role gia mao lot xuong")
	}
	if got.Header.Get("X-Request-Id") == "" {
		t.Fatal("thieu X-Request-Id cho upstream")
	}
}

// BR-Q3: co "da xac thuc OTP" chi den tu claim cua token, khong tu header client.
func TestGateway_ChuyenCoXacThucOTPTuToken(t *testing.T) {
	f := newFixture(t)
	sign := func(verified bool) string {
		tok, _ := authx.SignAccessToken([]byte(secret), authx.AccessClaims{
			Subject: buyerID, Role: "buyer", ID: "j", Verified: verified, IssuedAt: f.now})
		return tok
	}
	spoof := map[string]string{"X-Identity-Verified": "true"}

	f.do(t, http.MethodPost, "/v1/events/e1/queue/join", sign(false), spoof, "")
	if got := f.up.last(t).Header.Get("X-Identity-Verified"); got != "false" {
		t.Fatalf("token chua verify + header gia: upstream nhan %q, muon false", got)
	}
	f.do(t, http.MethodPost, "/v1/events/e1/queue/join", sign(true), nil, "")
	if got := f.up.last(t).Header.Get("X-Identity-Verified"); got != "true" {
		t.Fatalf("token da verify: upstream nhan %q, muon true", got)
	}
	f.do(t, http.MethodGet, "/api/v1/events/e1", "", spoof, "")
	if got := f.up.last(t).Header.Get("X-Identity-Verified"); got != "" {
		t.Fatalf("khach vang lai: header verified lot xuong upstream (%q)", got)
	}
}

// EV-113: thiet bi chi den tu claim did cua token.
func TestGateway_ChuyenThietBiTuToken(t *testing.T) {
	f := newFixture(t)
	const did = "33333333-3333-4333-8333-333333333333"
	sign := func(d string) string {
		tok, _ := authx.SignAccessToken([]byte(secret), authx.AccessClaims{
			Subject: buyerID, Role: "buyer", ID: "j", Verified: true, DeviceID: d, IssuedAt: f.now})
		return tok
	}
	spoof := map[string]string{"X-Device-Id": "44444444-4444-4444-8444-444444444444"}

	f.do(t, http.MethodPost, "/v1/events/e1/queue/join", sign(did), spoof, "")
	if got := f.up.last(t).Header.Get("X-Device-Id"); got != did {
		t.Fatalf("X-Device-Id = %q, muon %s (tu token)", got, did)
	}
	f.do(t, http.MethodPost, "/v1/events/e1/queue/join", sign(""), spoof, "")
	if got := f.up.last(t).Header.Get("X-Device-Id"); got != "" {
		t.Fatalf("token khong gan thiet bi nhung upstream nhan %q", got)
	}
}

// AC-2: route can dang nhap tu choi khi thieu/sai/het han token, upstream khong bi cham.
func TestGateway_RouteCanDangNhapTuChoiTokenKhongHopLe(t *testing.T) {
	f := newFixture(t)
	cases := []struct {
		name, bearer, wantType string
	}{
		{"thieu token", "", proxy.ProblemUnauthorized},
		{"token rac", "abc.def.ghi", proxy.ProblemUnauthorized},
		{"sai secret", func() string {
			tok, _ := authx.SignAccessToken([]byte("other-secret-at-least-16-bytes"), authx.AccessClaims{Subject: buyerID, Role: "buyer", ID: "j", IssuedAt: time.Now()})
			return tok
		}(), proxy.ProblemUnauthorized},
		{"het han", token(t, buyerID, "buyer", time.Now().Add(-time.Hour)), proxy.ProblemTokenExpired},
		{"role khong ton tai", token(t, buyerID, "admin", time.Now()), proxy.ProblemUnauthorized},
		{"sub khong phai uuid", token(t, "user-1", "buyer", time.Now()), proxy.ProblemUnauthorized},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			before := f.up.count()
			resp := f.do(t, http.MethodPost, "/v1/events/e1/holds", c.bearer, nil, `{}`)
			if resp.StatusCode != http.StatusUnauthorized {
				t.Fatalf("status %d, muon 401", resp.StatusCode)
			}
			if got := problemType(t, resp); got != c.wantType {
				t.Fatalf("type %q, muon %q", got, c.wantType)
			}
			if resp.Header.Get("WWW-Authenticate") == "" {
				t.Fatal("thieu WWW-Authenticate")
			}
			if f.up.count() != before {
				t.Fatal("upstream khong duoc nhan request bi tu choi")
			}
		})
	}
}

// AC-3: /internal/*, /metrics, /admin/* khong bao gio ra ngoai, ke ca voi token ops.
func TestGateway_ChanDuongDanNoiBo(t *testing.T) {
	f := newFixture(t)
	ops := token(t, opsID, "ops", f.now)
	for _, p := range []struct{ method, path string }{
		{http.MethodPatch, "/internal/v1/events/e1/ai-verdict"},
		{http.MethodPost, "/internal/reconcile"},
		{http.MethodGet, "/metrics"},
		{http.MethodPost, "/admin/mode/degraded"},
		{http.MethodGet, "/khong-ton-tai"},
		{http.MethodGet, "/api/v1/events/../../internal/v1/x"},
	} {
		resp := f.do(t, p.method, p.path, ops, nil, "")
		if resp.StatusCode != http.StatusNotFound {
			t.Fatalf("%s %s: status %d, muon 404", p.method, p.path, resp.StatusCode)
		}
	}
	if n := f.up.count(); n != 0 {
		t.Fatalf("upstream nhan %d request noi bo", n)
	}
}

// AC-4: route chi danh cho mot role.
func TestGateway_RouteTheoRole(t *testing.T) {
	f := newFixture(t)
	buyer := token(t, buyerID, "buyer", f.now)

	if resp := f.do(t, http.MethodPost, "/v1/payments/inbox/replay", buyer, nil, ""); resp.StatusCode != http.StatusForbidden {
		t.Fatalf("buyer goi inbox replay: %d, muon 403", resp.StatusCode)
	}
	if resp := f.do(t, http.MethodGet, "/api/v1/moderation/queue", buyer, nil, ""); resp.StatusCode != http.StatusForbidden {
		t.Fatalf("buyer xem hang doi kiem duyet: %d, muon 403", resp.StatusCode)
	}
	if resp := f.do(t, http.MethodPost, "/v1/payments/inbox/replay", token(t, opsID, "ops", f.now), nil, ""); resp.StatusCode != http.StatusOK {
		t.Fatalf("ops goi inbox replay: %d, muon 200", resp.StatusCode)
	}

	// EV-135: check-in doi trang thai ve nen chi ops; khach van kiem (verify) duoc.
	if resp := f.do(t, http.MethodPost, "/v1/tickets/check-in", buyer, nil, ""); resp.StatusCode != http.StatusForbidden {
		t.Fatalf("buyer check-in ve: %d, muon 403", resp.StatusCode)
	}
	if resp := f.do(t, http.MethodPost, "/v1/tickets/check-in", token(t, opsID, "ops", f.now), nil, ""); resp.StatusCode != http.StatusOK {
		t.Fatalf("ops check-in ve: %d, muon 200", resp.StatusCode)
	}
	if got := f.up.last(t); got.Upstream != config.Ticketing || got.Path != "/v1/tickets/check-in" {
		t.Fatalf("check-in -> %s %s", got.Upstream, got.Path)
	}
	if resp := f.do(t, http.MethodPost, "/v1/tickets/verify", buyer, nil, ""); resp.StatusCode != http.StatusOK {
		t.Fatalf("buyer verify ve: %d, muon 200", resp.StatusCode)
	}
}

// AC-5: moi duong dan web dang goi tro dung service.
func TestGateway_DinhTuyenDungService(t *testing.T) {
	f := newFixture(t)
	buyer := token(t, buyerID, "buyer", f.now)
	cases := []struct {
		method, path, bearer, want string
	}{
		{http.MethodPost, "/api/v1/auth/login", "", config.Identity},
		{http.MethodPost, "/api/v1/auth/refresh", "", config.Identity},
		{http.MethodGet, "/api/v1/auth/me", buyer, config.Identity},
		{http.MethodGet, "/api/v1/events/public", "", config.Event},
		{http.MethodGet, "/api/v1/events/e1", "", config.Event},
		{http.MethodGet, "/api/v1/events/slug/live-show", "", config.Event},
		{http.MethodPost, "/api/v1/events", buyer, config.Event},
		{http.MethodPost, "/api/v1/events/e1/approve", buyer, config.Event},
		{http.MethodPost, "/v1/events/e1/queue/join", buyer, config.Waitingroom},
		{http.MethodGet, "/v1/events/e1/queue/status", buyer, config.Waitingroom},
		{http.MethodGet, "/v1/events/e1/queue/stream", buyer, config.Waitingroom},
		{http.MethodPost, "/v1/events/e1/holds", buyer, config.Ticketing},
		{http.MethodGet, "/v1/events/e1/purchase-limit", buyer, config.Ticketing},
		{http.MethodGet, "/v1/orders/o1/tickets", buyer, config.Ticketing},
		{http.MethodGet, "/v1/orders/o1", buyer, config.Ticketing},
		{http.MethodDelete, "/v1/holds/h1", buyer, config.Ticketing},
		{http.MethodGet, "/v1/events/e1/availability", "", config.Ticketing},
		{http.MethodPost, "/v1/events/e1/payments", buyer, config.Payment},
		{http.MethodGet, "/v1/payments/p1", buyer, config.Payment},
		{http.MethodGet, "/v1/payments/methods", "", config.Payment},
		{http.MethodPost, "/v1/payments/webhook/vnpay", "", config.Payment},
		{http.MethodPost, "/v1/events/e1/assistant/messages", "", config.Assistant},
	}
	for _, c := range cases {
		resp := f.do(t, c.method, c.path, c.bearer, nil, "")
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("%s %s: status %d", c.method, c.path, resp.StatusCode)
		}
		got := f.up.last(t)
		if got.Upstream != c.want || got.Path != c.path {
			t.Fatalf("%s %s -> %s %s, muon %s", c.method, c.path, got.Upstream, got.Path, c.want)
		}
	}
}

// Route cong khai: token hop le van duoc gan danh tinh; token hong bi bo qua (khach).
func TestGateway_RouteCongKhaiGanDanhTinhNeuTokenHopLe(t *testing.T) {
	f := newFixture(t)
	f.do(t, http.MethodGet, "/api/v1/events/e1", token(t, buyerID, "buyer", f.now), nil, "")
	if got := f.up.last(t).Header.Get("X-Identity-Id"); got != buyerID {
		t.Fatalf("X-Identity-Id = %q, muon %s", got, buyerID)
	}
	resp := f.do(t, http.MethodGet, "/api/v1/events/e1", "token-hong", nil, "")
	if resp.StatusCode != http.StatusOK || f.up.last(t).Header.Get("X-Identity-Id") != "" {
		t.Fatal("token hong tren route cong khai phai thanh khach, khong 401")
	}
}

// AC-6: gioi han toc do tra 429 + Retry-After; poll phong cho dem theo nguoi va su kien.
func TestGateway_GioiHanTocDo(t *testing.T) {
	f := newFixture(t, func(c *config.Config) {
		c.Rates[config.RateQueuePoll] = config.Rate{PerSecond: 0.5, Burst: 2}
		c.Rates[config.RateAuth] = config.Rate{PerSecond: 1, Burst: 2}
	})
	buyer := token(t, buyerID, "buyer", f.now)
	other := token(t, opsID, "buyer", f.now)

	for i := range 2 {
		if resp := f.do(t, http.MethodGet, "/v1/events/e1/queue/status", buyer, nil, ""); resp.StatusCode != http.StatusOK {
			t.Fatalf("poll %d: %d", i+1, resp.StatusCode)
		}
	}
	resp := f.do(t, http.MethodGet, "/v1/events/e1/queue/status", buyer, nil, "")
	if resp.StatusCode != http.StatusTooManyRequests || resp.Header.Get("Retry-After") != "2" {
		t.Fatalf("poll thu 3: status %d Retry-After %q, muon 429 va 2", resp.StatusCode, resp.Header.Get("Retry-After"))
	}
	if problemType(t, resp) != proxy.ProblemRateLimited {
		t.Fatal("sai type 429")
	}
	// Nguoi khac va su kien khac khong bi anh huong.
	if resp := f.do(t, http.MethodGet, "/v1/events/e1/queue/status", other, nil, ""); resp.StatusCode != http.StatusOK {
		t.Fatalf("nguoi khac bi chan oan: %d", resp.StatusCode)
	}
	if resp := f.do(t, http.MethodGet, "/v1/events/e2/queue/status", buyer, nil, ""); resp.StatusCode != http.StatusOK {
		t.Fatalf("su kien khac bi chan oan: %d", resp.StatusCode)
	}
	// Het 2 giay thi poll lai duoc.
	f.now = f.now.Add(2 * time.Second)
	if resp := f.do(t, http.MethodGet, "/v1/events/e1/queue/status", buyer, nil, ""); resp.StatusCode != http.StatusOK {
		t.Fatalf("sau Retry-After van bi chan: %d", resp.StatusCode)
	}

	// Lop auth theo IP: token khac nhau khong lach duoc.
	for i, tok := range []string{"", buyer, other} {
		resp := f.do(t, http.MethodPost, "/api/v1/auth/login", tok, nil, "{}")
		want := http.StatusOK
		if i == 2 {
			want = http.StatusTooManyRequests
		}
		if resp.StatusCode != want {
			t.Fatalf("login %d: %d, muon %d", i+1, resp.StatusCode, want)
		}
	}
}

func TestGateway_CORS(t *testing.T) {
	f := newFixture(t)

	resp := f.do(t, http.MethodOptions, "/v1/events/e1/holds", "", map[string]string{
		"Origin": "http://localhost:3000", "Access-Control-Request-Method": "POST",
		"Access-Control-Request-Headers": "authorization, idempotency-key",
	}, "")
	if resp.StatusCode != http.StatusNoContent || resp.Header.Get("Access-Control-Allow-Origin") != "http://localhost:3000" {
		t.Fatalf("preflight: %d, origin %q", resp.StatusCode, resp.Header.Get("Access-Control-Allow-Origin"))
	}
	if !strings.Contains(resp.Header.Get("Access-Control-Allow-Headers"), "Idempotency-Key") {
		t.Fatal("preflight thieu Idempotency-Key")
	}
	if f.up.count() != 0 {
		t.Fatal("preflight khong duoc toi upstream")
	}

	// Request that: chi mot gia tri Allow-Origin (cua gateway), "*" cua upstream bi xoa.
	resp = f.do(t, http.MethodPost, "/api/v1/auth/login", "", map[string]string{"Origin": "http://localhost:3000"}, "{}")
	if v := resp.Header.Values("Access-Control-Allow-Origin"); len(v) != 1 || v[0] != "http://localhost:3000" {
		t.Fatalf("Allow-Origin = %v", v)
	}

	// Origin la: khong co header cho phep.
	resp = f.do(t, http.MethodPost, "/api/v1/auth/login", "", map[string]string{"Origin": "https://evil.example"}, "{}")
	if v := resp.Header.Get("Access-Control-Allow-Origin"); v != "" {
		t.Fatalf("origin la nhan Allow-Origin %q", v)
	}
}

func TestGateway_UpstreamSapTra502(t *testing.T) {
	f := newFixture(t)
	f.up.srvs[config.Ticketing].Close()
	resp := f.do(t, http.MethodPost, "/v1/events/e1/holds", token(t, buyerID, "buyer", f.now), nil, "{}")
	if resp.StatusCode != http.StatusBadGateway || problemType(t, resp) != proxy.ProblemUpstream {
		t.Fatalf("status %d, muon 502 upstream-unavailable", resp.StatusCode)
	}
}

func TestGateway_GioiHanKichThuocBody(t *testing.T) {
	f := newFixture(t) // MaxBodyBytes = 1 KiB
	resp := f.do(t, http.MethodPost, "/v1/events/e1/holds", token(t, buyerID, "buyer", f.now), nil, strings.Repeat("x", 4<<10))
	if resp.StatusCode != http.StatusRequestEntityTooLarge || problemType(t, resp) != proxy.ProblemTooLarge {
		t.Fatalf("body 4 KiB qua gioi han 1 KiB: status %d, muon 413", resp.StatusCode)
	}
	if f.up.count() != 0 {
		t.Fatal("request qua lon khong duoc toi upstream")
	}
	if resp := f.do(t, http.MethodPost, "/v1/events/e1/holds", token(t, buyerID, "buyer", f.now), nil, "{}"); resp.StatusCode != http.StatusOK {
		t.Fatalf("body nho phai qua: %d", resp.StatusCode)
	}
}

// SSE: gateway phai day tung su kien ngay, khong gom buffer toi khi stream dong.
func TestGateway_SSEDuocDayNgay(t *testing.T) {
	release := make(chan struct{})
	sse := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: rank=1\n\n")
		w.(http.Flusher).Flush()
		<-release
	}))
	defer sse.Close()
	defer close(release)

	f := newFixture(t, func(c *config.Config) { c.Upstreams[config.Waitingroom], _ = url.Parse(sse.URL) })
	req, _ := http.NewRequest(http.MethodGet, f.gw.URL+"/v1/events/e1/queue/stream", nil)
	req.Header.Set("Authorization", "Bearer "+token(t, buyerID, "buyer", f.now))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()

	line := make(chan string, 1)
	go func() {
		s, _ := bufio.NewReader(resp.Body).ReadString('\n')
		line <- s
	}()
	select {
	case got := <-line:
		if got != "data: rank=1\n" {
			t.Fatalf("doc duoc %q", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("su kien SSE bi gateway giu lai")
	}
}

func TestGateway_SecurityHeaders(t *testing.T) {
	f := newFixture(t)
	resp := f.do(t, http.MethodGet, "/healthz", "", nil, "")
	if resp.StatusCode != http.StatusOK || resp.Header.Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("healthz %d, nosniff %q", resp.StatusCode, resp.Header.Get("X-Content-Type-Options"))
	}
}
