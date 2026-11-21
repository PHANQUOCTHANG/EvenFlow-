package proxy_test

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/eventflow/eventflow/services/gateway/internal/config"
	"github.com/eventflow/eventflow/services/gateway/internal/proxy"
)

// newPeerFixture dung gateway rieng (khong dung newFixture) phia sau mot wrapper dat
// r.RemoteAddr = peer, de gia lap ket noi tu ingress. Lop auth: 1 rps / burst 10. Dong ho
// co dinh nen burst khong hoi lai giua cac request.
func newPeerFixture(t *testing.T, peer string, trusted []netip.Prefix, mutate ...func(*config.Config)) *fixture {
	t.Helper()
	f := &fixture{up: &upstreams{srvs: map[string]*httptest.Server{}}, now: time.Now()}
	cfg := &config.Config{
		JWTSecret:      secret,
		Upstreams:      map[string]*url.URL{},
		AllowedOrigins: []string{"http://localhost:3000"},
		Rates: map[string]config.Rate{
			config.RateAuth:      {PerSecond: 1, Burst: 10},
			config.RateQueuePoll: {PerSecond: 100, Burst: 100},
			config.RateDefault:   {PerSecond: 100, Burst: 100},
		},
		MaxBodyBytes:   1 << 10,
		TrustedProxies: trusted,
	}
	for _, name := range []string{config.Identity, config.Event, config.Waitingroom, config.Ticketing, config.Payment, config.Assistant} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, _ := io.ReadAll(r.Body)
			f.up.mu.Lock()
			f.up.hits = append(f.up.hits, seen{Upstream: name, Method: r.Method, Path: r.URL.Path, Header: r.Header.Clone(), Body: string(body)})
			f.up.mu.Unlock()
			w.WriteHeader(http.StatusOK)
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
	f.gw = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.RemoteAddr = peer
		h.ServeHTTP(w, r)
	}))
	t.Cleanup(f.gw.Close)
	return f
}

func trust(ss ...string) []netip.Prefix {
	out := make([]netip.Prefix, 0, len(ss))
	for _, s := range ss {
		out = append(out, netip.MustParsePrefix(s))
	}
	return out
}

// send gui request voi header nhieu dong (vd hai dong X-Forwarded-For).
func send(t *testing.T, f *fixture, method, path, bearer string, h http.Header) *http.Response {
	t.Helper()
	var body io.Reader
	if method == http.MethodPost {
		body = strings.NewReader("{}")
	}
	req, _ := http.NewRequest(method, f.gw.URL+path, body)
	for k, vs := range h {
		for _, v := range vs {
			req.Header.Add(k, v)
		}
	}
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
	return resp
}

func xff(lines ...string) http.Header {
	h := http.Header{}
	for _, l := range lines {
		h.Add("X-Forwarded-For", l)
	}
	return h
}

func login(t *testing.T, f *fixture, bearer string, h http.Header) *http.Response {
	t.Helper()
	return send(t, f, http.MethodPost, "/api/v1/auth/login", bearer, h)
}

func wantUpstreamXFF(t *testing.T, f *fixture, want string) {
	t.Helper()
	got := f.up.last(t).Header.Values("X-Forwarded-For")
	if len(got) != 1 || got[0] != want {
		t.Fatalf("upstream X-Forwarded-For = %q, want exactly [%q]", got, want)
	}
}

// AC-1: trust rong = develop. XFF, X-Real-IP, Forwarded gia mao deu bi bo qua: khoa la peer,
// upstream XFF la host cua peer, voi moi RemoteAddr.
func TestTrustedProxy_AC1_TrustOffIgnoresForwardingHeaders(t *testing.T) {
	for _, tc := range []struct {
		peer    string
		wantXFF string // "" = upstream khong co XFF (SetXForwarded xoa, nhu develop)
	}{
		{"203.0.113.9:4000", "203.0.113.9"},
		{"10.1.2.3:4000", "10.1.2.3"},
		{"[::ffff:10.1.2.3]:4000", "::ffff:10.1.2.3"},
		{"[fe80::1%eth0]:51234", "fe80::1%eth0"},
		{"garbage", ""},
	} {
		t.Run(tc.peer, func(t *testing.T) {
			for _, trusted := range [][]netip.Prefix{nil, {}} {
				f := newPeerFixture(t, tc.peer, trusted)
				for i := 1; i <= 11; i++ {
					h := xff(fmt.Sprintf("198.51.100.%d", i))
					h.Set("X-Real-IP", fmt.Sprintf("198.51.101.%d", i))
					h.Set("Forwarded", fmt.Sprintf("for=198.51.102.%d", i))
					resp := login(t, f, "", h)
					want := http.StatusOK
					if i == 11 {
						want = http.StatusTooManyRequests
					}
					if resp.StatusCode != want {
						t.Fatalf("login %d: %d, want %d (forged headers must not change the bucket)", i, resp.StatusCode, want)
					}
					if i == 1 {
						got := f.up.last(t).Header
						if tc.wantXFF == "" {
							if v := got.Values("X-Forwarded-For"); len(v) != 0 {
								t.Fatalf("upstream XFF = %q, want none (as develop)", v)
							}
						} else {
							wantUpstreamXFF(t, f, tc.wantXFF)
						}
						// Rewrite xoa Forwarded nhu develop (X-Real-IP khong bi xoa, chi khong duoc doc).
						if got.Get("Forwarded") != "" {
							t.Fatalf("Forwarded reached upstream: %q", got.Get("Forwarded"))
						}
					}
				}
			}
		})
	}
}

// AC-2: peer khong tin cay -> XFF bi bo qua; 11 XFF khac nhau van chung mot bucket.
func TestTrustedProxy_AC2_UntrustedPeerCannotSpoof(t *testing.T) {
	f := newPeerFixture(t, "203.0.113.9:4000", trust("10.0.0.0/8"))
	for i := 1; i <= 11; i++ {
		resp := login(t, f, "", xff(fmt.Sprintf("1.2.3.%d", i)))
		want := http.StatusOK
		if i == 11 {
			want = http.StatusTooManyRequests
		}
		if resp.StatusCode != want {
			t.Fatalf("login %d: %d, want %d", i, resp.StatusCode, want)
		}
		if i == 1 {
			wantUpstreamXFF(t, f, "203.0.113.9")
		}
	}
}

// AC-3 (R-1): hai client sau cung mot proxy tin cay co bucket rieng.
func TestTrustedProxy_AC3_ClientsBehindTrustedProxyHaveOwnBucket(t *testing.T) {
	f := newPeerFixture(t, "10.1.2.3:4000", trust("10.0.0.0/8"))
	a, b := xff("198.51.100.1"), xff("198.51.100.2")
	for i := 1; i <= 10; i++ {
		if resp := login(t, f, "", a); resp.StatusCode != http.StatusOK {
			t.Fatalf("client A login %d: %d", i, resp.StatusCode)
		}
	}
	wantUpstreamXFF(t, f, "198.51.100.1")
	if resp := login(t, f, "", b); resp.StatusCode == http.StatusTooManyRequests {
		t.Fatal("client B blocked by client A's burst (shared ingress bucket)")
	}
	wantUpstreamXFF(t, f, "198.51.100.2")
	resp := login(t, f, "", a)
	if resp.StatusCode != http.StatusTooManyRequests || resp.Header.Get("Retry-After") == "" {
		t.Fatalf("client A 11th: %d Retry-After %q, want 429 with Retry-After", resp.StatusCode, resp.Header.Get("Retry-After"))
	}
}

// AC-4: doi phan gia mao ben trai khong doi bucket.
func TestTrustedProxy_AC4_ForgedLeftmostSharesBucket(t *testing.T) {
	f := newPeerFixture(t, "10.1.2.3:4000", trust("10.0.0.0/8"))
	for i := 1; i <= 11; i++ {
		resp := login(t, f, "", xff(fmt.Sprintf("6.6.6.%d, 198.51.100.1", i)))
		want := http.StatusOK
		if i == 11 {
			want = http.StatusTooManyRequests
		}
		if resp.StatusCode != want {
			t.Fatalf("login %d: %d, want %d", i, resp.StatusCode, want)
		}
		if i == 1 {
			wantUpstreamXFF(t, f, "198.51.100.1")
		}
	}
	if resp := login(t, f, "", xff("6.6.6.6, 198.51.100.2")); resp.StatusCode != http.StatusOK {
		t.Fatalf("other real client blocked: %d", resp.StatusCode)
	}
}

// AC-9: upstream nhan dung mot phan tu XFF = IP da phan giai; X-Identity-* gia mao bi xoa.
func TestTrustedProxy_AC9_UpstreamGetsResolvedIPOnly(t *testing.T) {
	t10 := trust("10.0.0.0/8")
	hops17 := make([]string, 17)
	for i := range hops17 {
		hops17[i] = fmt.Sprintf("10.0.0.%d", i+1)
	}
	for _, tc := range []struct {
		name    string
		peer    string
		trusted []netip.Prefix
		lines   []string
		want    string
	}{
		{"AC2 untrusted peer", "203.0.113.9:4000", t10, []string{"1.2.3.4"}, "203.0.113.9"},
		{"AC3 trusted peer", "10.1.2.3:4000", t10, []string{"198.51.100.1"}, "198.51.100.1"},
		{"AC4 forged leftmost", "10.1.2.3:4000", t10, []string{"6.6.6.6, 198.51.100.1"}, "198.51.100.1"},
		{"AC5 chain", "10.1.2.3:4000", trust("10.0.0.0/8", "192.0.2.0/24"), []string{"198.51.100.1, 192.0.2.10"}, "198.51.100.1"},
		{"AC5 untrusted middle hop", "10.1.2.3:4000", trust("10.0.0.0/8", "192.0.2.0/24"), []string{"198.51.100.1, 203.0.113.50, 192.0.2.10"}, "203.0.113.50"},
		{"AC6 two header lines", "10.1.2.3:4000", t10, []string{"198.51.100.1", "10.0.0.5"}, "198.51.100.1"},
		{"AC6 ipv4 port", "10.1.2.3:4000", t10, []string{"198.51.100.1:5555"}, "198.51.100.1"},
		{"AC6 ipv6 bracket port", "10.1.2.3:4000", t10, []string{"[2001:db8::1]:443"}, "2001:db8::1"},
		{"AC6 ipv6 upper case", "10.1.2.3:4000", t10, []string{"2001:DB8::1"}, "2001:db8::1"},
		{"AC6 mapped element", "10.1.2.3:4000", t10, []string{"::ffff:198.51.100.1"}, "198.51.100.1"},
		{"AC6 mapped peer", "[::ffff:10.1.2.3]:4000", t10, []string{"198.51.100.1"}, "198.51.100.1"},
		{"AC6 mapped peer no xff", "[::ffff:10.1.2.3]:4000", t10, nil, "::ffff:10.1.2.3"},
		{"AC7 malformed rightmost", "10.1.2.3:4000", t10, []string{"198.51.100.1, unknown"}, "10.1.2.3"},
		{"AC7 malformed left of client", "10.1.2.3:4000", t10, []string{"unknown, 198.51.100.1"}, "198.51.100.1"},
		{"AC7 no xff", "10.1.2.3:4000", t10, nil, "10.1.2.3"},
		{"AC7 only empty elements", "10.1.2.3:4000", t10, []string{" , ,"}, "10.1.2.3"},
		{"AC7 all trusted leftmost", "10.1.2.3:4000", t10, []string{"10.0.0.7, 10.0.0.8"}, "10.0.0.7"},
		{"AC7 17 trusted hops", "10.1.2.3:4000", t10, []string{strings.Join(hops17, ", ")}, "10.1.2.3"},
		{"AC7 zoned peer", "[fe80::1%eth0]:51234", t10, []string{"198.51.100.1"}, "fe80::1%eth0"},
		// R2-1: RemoteAddr khong tach duoc -> SetXForwarded xoa XFF nhu develop; kiem rieng ben duoi.
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newPeerFixture(t, tc.peer, tc.trusted)
			h := xff(tc.lines...)
			h.Set("X-Identity-Id", opsID)
			h.Set("X-Identity-Role", "ops")
			if resp := send(t, f, http.MethodGet, "/api/v1/events/abc", "", h); resp.StatusCode != http.StatusOK {
				t.Fatalf("status %d", resp.StatusCode)
			}
			wantUpstreamXFF(t, f, tc.want)
			got := f.up.last(t).Header
			if got.Get("X-Identity-Id") != "" || got.Get("X-Identity-Role") != "" {
				t.Fatalf("forged identity reached upstream: id=%q role=%q", got.Get("X-Identity-Id"), got.Get("X-Identity-Role"))
			}
		})
	}

	// R2-1: RemoteAddr khong tach duoc, trust bat: XFF bi bo qua va upstream khong co XFF (nhu develop).
	t.Run("AC7 unparsable RemoteAddr", func(t *testing.T) {
		f := newPeerFixture(t, "garbage", t10)
		if resp := send(t, f, http.MethodGet, "/api/v1/events/abc", "", xff("198.51.100.1")); resp.StatusCode != http.StatusOK {
			t.Fatalf("status %d", resp.StatusCode)
		}
		if v := f.up.last(t).Header.Values("X-Forwarded-For"); len(v) != 0 {
			t.Fatalf("upstream XFF = %q, want none (as develop)", v)
		}
	})
}

// AC-10: lop khoa giu nguyen, chi doi nguon IP.
func TestTrustedProxy_AC10_KeyClasses(t *testing.T) {
	t10 := trust("10.0.0.0/8")
	a, b := xff("198.51.100.1"), xff("198.51.100.2")

	t.Run("auth route keyed by resolved IP even when logged in", func(t *testing.T) {
		f := newPeerFixture(t, "10.1.2.3:4000", t10)
		buyer, other := token(t, buyerID, "buyer", f.now), token(t, opsID, "buyer", f.now)
		for i := 1; i <= 10; i++ {
			if resp := login(t, f, buyer, a); resp.StatusCode != http.StatusOK {
				t.Fatalf("login %d: %d", i, resp.StatusCode)
			}
		}
		if resp := login(t, f, other, a); resp.StatusCode != http.StatusTooManyRequests {
			t.Fatalf("other user from same client IP: %d, want 429 (auth keyed by IP)", resp.StatusCode)
		}
		if resp := login(t, f, buyer, b); resp.StatusCode != http.StatusOK {
			t.Fatalf("same user from other client IP: %d, want 200 (auth keyed by resolved IP)", resp.StatusCode)
		}
	})

	t.Run("default route anonymous keyed by resolved IP", func(t *testing.T) {
		f := newPeerFixture(t, "10.1.2.3:4000", t10, func(c *config.Config) {
			c.Rates[config.RateDefault] = config.Rate{PerSecond: 1, Burst: 2}
		})
		for i := 1; i <= 2; i++ {
			if resp := send(t, f, http.MethodGet, "/api/v1/events/abc", "", a); resp.StatusCode != http.StatusOK {
				t.Fatalf("anon A %d: %d", i, resp.StatusCode)
			}
		}
		if resp := send(t, f, http.MethodGet, "/api/v1/events/abc", "", a); resp.StatusCode != http.StatusTooManyRequests {
			t.Fatalf("anon A 3rd: %d, want 429", resp.StatusCode)
		}
		if resp := send(t, f, http.MethodGet, "/api/v1/events/abc", "", b); resp.StatusCode != http.StatusOK {
			t.Fatalf("anon B: %d, want 200", resp.StatusCode)
		}
	})

	t.Run("default route logged in keyed by id", func(t *testing.T) {
		f := newPeerFixture(t, "10.1.2.3:4000", t10, func(c *config.Config) {
			c.Rates[config.RateDefault] = config.Rate{PerSecond: 1, Burst: 2}
		})
		buyer, other := token(t, buyerID, "buyer", f.now), token(t, opsID, "buyer", f.now)
		for i := 1; i <= 2; i++ {
			if resp := send(t, f, http.MethodGet, "/v1/orders/o1", buyer, a); resp.StatusCode != http.StatusOK {
				t.Fatalf("buyer %d: %d", i, resp.StatusCode)
			}
		}
		if resp := send(t, f, http.MethodGet, "/v1/orders/o1", buyer, b); resp.StatusCode != http.StatusTooManyRequests {
			t.Fatalf("buyer from other IP: %d, want 429 (keyed by id)", resp.StatusCode)
		}
		if resp := send(t, f, http.MethodGet, "/v1/orders/o1", other, a); resp.StatusCode != http.StatusOK {
			t.Fatalf("other user from same IP: %d, want 200", resp.StatusCode)
		}
	})

	t.Run("queue poll keeps event suffix", func(t *testing.T) {
		f := newPeerFixture(t, "10.1.2.3:4000", t10, func(c *config.Config) {
			c.Rates[config.RateQueuePoll] = config.Rate{PerSecond: 0.5, Burst: 2}
		})
		buyer := token(t, buyerID, "buyer", f.now)
		for i := 1; i <= 2; i++ {
			if resp := send(t, f, http.MethodGet, "/v1/events/e1/queue/status", buyer, a); resp.StatusCode != http.StatusOK {
				t.Fatalf("poll %d: %d", i, resp.StatusCode)
			}
		}
		if resp := send(t, f, http.MethodGet, "/v1/events/e1/queue/status", buyer, b); resp.StatusCode != http.StatusTooManyRequests {
			t.Fatalf("poll e1 from other IP: %d, want 429 (keyed by id|event)", resp.StatusCode)
		}
		if resp := send(t, f, http.MethodGet, "/v1/events/e2/queue/status", buyer, a); resp.StatusCode != http.StatusOK {
			t.Fatalf("poll e2: %d, want 200 (per-event suffix)", resp.StatusCode)
		}
	})
}
