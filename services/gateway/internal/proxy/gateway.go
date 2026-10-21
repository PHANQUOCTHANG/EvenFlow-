package proxy

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/eventflow/eventflow/libs/go/authx"
	"github.com/eventflow/eventflow/libs/go/httpx"
	"github.com/eventflow/eventflow/libs/go/otelx"
	"github.com/eventflow/eventflow/services/gateway/internal/config"
	"github.com/eventflow/eventflow/services/gateway/internal/ratelimit"
)

// Loai loi RFC 7807 cua gateway: client dua vao "type", khong dua vao "detail".
const (
	ProblemUnauthorized = "/problems/unauthorized"
	ProblemTokenExpired = "/problems/token-expired"
	ProblemForbidden    = "/problems/forbidden"
	ProblemRateLimited  = "/problems/rate-limited"
	ProblemNotFound     = "/problems/not-found"
	ProblemTooLarge     = "/problems/payload-too-large"
	ProblemUpstream     = "/problems/upstream-unavailable"
)

// Gateway ghep bang route voi xac thuc, gioi han toc do va reverse proxy.
type Gateway struct {
	secret   []byte
	proxies  map[string]http.Handler
	limiters map[string]*ratelimit.Limiter
	maxBody  int64
	now      func() time.Time
	log      *slog.Logger
}

// Option tuy chinh Gateway (cho test).
type Option func(*Gateway)

// WithClock thay dong ho dung de xac minh token va dem gioi han toc do.
func WithClock(now func() time.Time) Option {
	return func(g *Gateway) { g.now = now }
}

// New dung handler hoan chinh cua gateway tu cau hinh.
func New(cfg *config.Config, log *slog.Logger, opts ...Option) (http.Handler, error) {
	g := &Gateway{
		secret:   []byte(cfg.JWTSecret),
		proxies:  map[string]http.Handler{},
		limiters: map[string]*ratelimit.Limiter{},
		maxBody:  cfg.MaxBodyBytes,
		now:      time.Now,
		log:      log,
	}
	for _, o := range opts {
		o(g)
	}
	for name, u := range cfg.Upstreams {
		g.proxies[name] = newReverseProxy(u, log)
	}
	for class, r := range cfg.Rates {
		g.limiters[class] = ratelimit.New(r.PerSecond, r.Burst, g.now)
	}

	mux := http.NewServeMux()
	httpx.Health(mux)
	for _, rt := range Routes() {
		h, err := g.routeHandler(rt)
		if err != nil {
			return nil, err
		}
		mux.Handle(rt.Pattern, h)
	}
	mux.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		httpx.WriteProblemType(w, http.StatusNotFound, ProblemNotFound, "khong tim thay duong dan")
	}))

	return httpx.Chain(mux,
		httpx.RequestID(),
		securityHeaders,
		CORS(cfg.AllowedOrigins),
		// Sau moi middleware tao ban sao request (de route khop trong span), truoc AccessLog (log mang trace_id).
		otelx.HTTP("gateway"),
		httpx.AccessLog(log),
		httpx.Recover(log),
	), nil
}

func (g *Gateway) routeHandler(rt Route) (http.Handler, error) {
	if rt.Access == Blocked {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			httpx.WriteProblemType(w, http.StatusNotFound, ProblemNotFound, "khong tim thay duong dan")
		}), nil
	}
	upstream, ok := g.proxies[rt.Upstream]
	if !ok {
		return nil, fmt.Errorf("route %q tro toi upstream %q chua cau hinh", rt.Pattern, rt.Upstream)
	}
	limiter, ok := g.limiters[rt.Rate]
	if !ok {
		return nil, fmt.Errorf("route %q dung lop gioi han %q chua cau hinh", rt.Pattern, rt.Rate)
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 1. Khong tin bat ky danh tinh nao client tu khai, ke ca tren route cong khai.
		authx.StripIdentityHeaders(r.Header)

		// 2. Xac thuc.
		principal, authErr := g.authenticate(r)
		if rt.Access == Authenticated {
			if principal == nil {
				writeUnauthorized(w, authErr)
				return
			}
			if len(rt.Roles) > 0 && !slices.Contains(rt.Roles, principal.Role) {
				httpx.WriteProblemType(w, http.StatusForbidden, ProblemForbidden, "ban khong co quyen goi duong dan nay")
				return
			}
		}

		// 3. Gioi han toc do: theo nguoi neu da dang nhap, nguoc lai theo IP. Lop auth luon theo
		// IP: muc dich la chan do mat khau tu mot may.
		if ok, wait := limiter.Allow(rateKey(rt, r, principal)); !ok {
			w.Header().Set("Retry-After", strconv.Itoa(int(math.Ceil(wait.Seconds()))))
			httpx.WriteProblemType(w, http.StatusTooManyRequests, ProblemRateLimited, "qua nhieu request, vui long thu lai sau")
			return
		}

		// 4. Gan danh tinh da xac minh va chuyen tiep.
		if principal != nil {
			authx.SetIdentityHeaders(r.Header, *principal)
		}
		if id := httpx.RequestIDFrom(r.Context()); id != "" {
			r.Header.Set(httpx.RequestIDHeader, id)
		}
		if r.ContentLength > g.maxBody {
			httpx.WriteProblemType(w, http.StatusRequestEntityTooLarge, ProblemTooLarge, "noi dung request qua lon")
			return
		}
		if r.Body != nil && r.Body != http.NoBody {
			// Body chunked khong khai Content-Length: cat o day, upstream nhan loi doc body.
			r.Body = http.MaxBytesReader(w, r.Body, g.maxBody)
		}
		upstream.ServeHTTP(w, r)
	}), nil
}

var errNoToken = errors.New("thieu access token")

// authenticate tra Principal neu request mang access token hop le. Khong co header thi
// (nil, errNoToken); token hong thi (nil, loi xac minh).
func (g *Gateway) authenticate(r *http.Request) (*authx.Principal, error) {
	h := r.Header.Get("Authorization")
	if h == "" {
		return nil, errNoToken
	}
	scheme, token, ok := strings.Cut(h, " ")
	if !ok || !strings.EqualFold(scheme, "Bearer") {
		return nil, authx.ErrInvalidAccessToken
	}
	claims, err := authx.VerifyAccessToken(g.secret, strings.TrimSpace(token), g.now())
	if err != nil {
		return nil, err
	}
	p := authx.Principal{ID: claims.Subject, Role: claims.Role, Verified: claims.Verified, DeviceID: claims.DeviceID}
	if !p.Valid() {
		return nil, authx.ErrInvalidAccessToken
	}
	return &p, nil
}

func writeUnauthorized(w http.ResponseWriter, err error) {
	if errors.Is(err, authx.ErrExpiredAccessToken) {
		w.Header().Set("WWW-Authenticate", `Bearer error="invalid_token", error_description="token expired"`)
		httpx.WriteProblemType(w, http.StatusUnauthorized, ProblemTokenExpired, "phien dang nhap da het han, hay lam moi token")
		return
	}
	w.Header().Set("WWW-Authenticate", `Bearer`)
	httpx.WriteProblemType(w, http.StatusUnauthorized, ProblemUnauthorized, "can dang nhap")
}

func rateKey(rt Route, r *http.Request, p *authx.Principal) string {
	key := "ip:" + clientIP(r)
	if p != nil && rt.Rate != config.RateAuth {
		key = "id:" + p.ID
	}
	// Poll phong cho dem rieng tung su kien: mot nguoi xep hang hai su kien khong bi chan oan.
	if ev := r.PathValue("eventID"); ev != "" && rt.Rate == config.RateQueuePoll {
		key += "|" + ev
	}
	return key
}

// clientIP lay IP cua ket noi. Gateway la lop ngoai cung nen KHONG tin X-Forwarded-For do
// client gui; dat sau ingress/LB thi phai cau hinh proxy tin cay truoc khi doi o day.
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// newReverseProxy chuyen tiep toi target. Rewrite (khac Director) tu xoa X-Forwarded-* do
// client gui truoc khi gan lai, nen client khong gia duoc IP qua header.
func newReverseProxy(target *url.URL, log *slog.Logger) *httputil.ReverseProxy {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.MaxIdleConnsPerHost = 256
	transport.ResponseHeaderTimeout = 30 * time.Second

	return &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(target)
			pr.SetXForwarded()
			// Noi trace sang service phia sau (traceparent) de mot yeu cau la mot trace.
			otelx.InjectHTTP(pr.In.Context(), pr.Out.Header)
		},
		Transport: transport,
		// SSE (queue/stream, vneid/sse): day tung su kien ngay, khong gom buffer.
		FlushInterval: -1,
		// CORS chi do gateway quyet. Service cu (identity) tu dat "*", de lai se thanh hai gia
		// tri va trinh duyet tu choi.
		ModifyResponse: func(resp *http.Response) error {
			for k := range resp.Header {
				if strings.HasPrefix(k, "Access-Control-") {
					resp.Header.Del(k)
				}
			}
			return nil
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			if errors.Is(err, context.Canceled) {
				return // client da ngat
			}
			log.Error("upstream loi", "upstream", target.Host, "route", r.Pattern, "err", err,
				"request_id", httpx.RequestIDFrom(r.Context()))
			httpx.WriteProblemType(w, http.StatusBadGateway, ProblemUpstream, "dich vu tam thoi khong san sang")
		},
	}
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		next.ServeHTTP(w, r)
	})
}

// CORS cho phep trinh duyet tu cac origin da khai bao goi gateway. Preflight tra 204 ngay tai
// gateway, khong cham service phia sau va khong can token.
func CORS(origins []string) httpx.Middleware {
	allowAll := slices.Contains(origins, "*")
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin != "" && (allowAll || slices.Contains(origins, origin)) {
				h := w.Header()
				h.Set("Access-Control-Allow-Origin", origin)
				h.Add("Vary", "Origin")
				h.Set("Access-Control-Expose-Headers", "X-Request-Id, Retry-After")
				if r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != "" {
					h.Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
					h.Set("Access-Control-Allow-Headers", "Authorization, Content-Type, Idempotency-Key, X-Request-Id, Last-Event-ID")
					h.Set("Access-Control-Max-Age", "600")
					w.WriteHeader(http.StatusNoContent)
					return
				}
			} else if r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != "" {
				// Origin la: tra 204 khong kem header cho phep, trinh duyet se chan.
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
