package idem

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/eventflow/eventflow/libs/go/httpx"
)

// ErrLeaseLost: khoa da bi request khac tiep quan nen khong ghi de duoc.
var ErrLeaseLost = errors.New("idem: khoa da bi tiep quan")

// Loai loi (RFC 7807 `type`) de client phan biet 409 "dang xu ly" voi 409 "het ve" cua nghiep vu.
const (
	ProblemInProgress = "urn:eventflow:problem:idempotency-in-progress"
	ProblemKeyReused  = "urn:eventflow:problem:idempotency-key-reused"
)

// finishTimeout la thoi gian toi da de ghi ket qua, doc lap voi request da bi client huy.
const finishTimeout = 5 * time.Second

type config struct {
	ttl      time.Duration
	lease    time.Duration
	maxBody  int64
	identity func(*http.Request) string
	log      *slog.Logger
}

// Option tuy chinh Middleware.
type Option func(*config)

// WithTTL doi thoi gian giu response (mac dinh 24 gio).
func WithTTL(d time.Duration) Option { return func(c *config) { c.ttl = d } }

// WithLease doi thoi gian toi da mot request duoc coi la dang xu ly (mac dinh 30 giay).
func WithLease(d time.Duration) Option { return func(c *config) { c.lease = d } }

// WithMaxBody doi gioi han body (mac dinh 1 MiB). Vuot gioi han: 413.
func WithMaxBody(n int64) Option { return func(c *config) { c.maxBody = n } }

// WithIdentity cho biet ai dang goi. Gia tri nay nam trong hash cua request, nen cung mot key
// do hai nguoi khac nhau dung se nhan 422 chu khong phai response cua nhau.
func WithIdentity(f func(*http.Request) string) Option { return func(c *config) { c.identity = f } }

// WithLogger dat logger (mac dinh slog.Default).
func WithLogger(l *slog.Logger) Option { return func(c *config) { c.log = l } }

// Middleware bao handler ghi bang co che Idempotency-Key. Xem hop dong o doc goi.
func Middleware(store Store, opts ...Option) httpx.Middleware {
	cfg := config{ttl: DefaultTTL, lease: DefaultLease, maxBody: DefaultMaxBody, log: slog.Default()}
	for _, o := range opts {
		o(&cfg)
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			serve(w, r, next, store, &cfg)
		})
	}
}

func serve(w http.ResponseWriter, r *http.Request, next http.Handler, store Store, cfg *config) {
	vals := r.Header.Values(Header)
	if len(vals) != 1 || !validKey(vals[0]) {
		httpx.WriteProblem(w, http.StatusBadRequest,
			"thieu hoac sai dinh dang header Idempotency-Key (1-255 ky tu ASCII in duoc, khong khoang trang)")
		return
	}
	key := vals[0]

	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, cfg.maxBody))
	if err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			httpx.WriteProblem(w, http.StatusRequestEntityTooLarge, "body qua lon")
			return
		}
		httpx.WriteProblem(w, http.StatusBadRequest, "khong doc duoc body")
		return
	}

	identity := ""
	if cfg.identity != nil {
		identity = cfg.identity(r)
	}
	req := Request{
		Key:      key,
		Endpoint: r.Method + " " + r.URL.Path,
		Identity: identity,
		Hash:     hashRequest(r, identity, body),
	}

	out, err := store.Begin(r.Context(), req, cfg.ttl, cfg.lease)
	if err != nil {
		// Khong chay handler khi khong ghi duoc khoa: chay tiep la ghi ma khong co bao ve, va
		// handler cung can chinh Postgres nay.
		cfg.log.Error("idempotency: khong giu duoc khoa", "err", err, "endpoint", req.Endpoint)
		httpx.WriteProblem(w, http.StatusServiceUnavailable, "he thong dang ban, vui long thu lai")
		return
	}

	switch out.State {
	case Replay:
		replay(w, out.Response)
		return
	case InProgress:
		w.Header().Set("Retry-After", "1")
		writeProblem(w, http.StatusConflict, ProblemInProgress,
			"yeu cau cung Idempotency-Key dang duoc xu ly, vui long thu lai sau giay lat")
		return
	case Mismatch:
		writeProblem(w, http.StatusUnprocessableEntity, ProblemKeyReused,
			"Idempotency-Key da duoc dung cho mot yeu cau khac")
		return
	case Acquired:
	default:
		cfg.log.Error("idempotency: trang thai la", "state", out.State)
		httpx.WriteProblem(w, http.StatusServiceUnavailable, "he thong dang ban, vui long thu lai")
		return
	}

	run(w, r, next, store, cfg, req, out.Token, body)
}

func run(w http.ResponseWriter, r *http.Request, next http.Handler, store Store, cfg *config,
	req Request, token string, body []byte) {
	// Ket qua phai duoc ghi ke ca khi client da ngat ket noi: nghiep vu da chay xong, mat ket
	// qua o day nghia la lan thu lai se chay nghiep vu lan nua.
	finishCtx := func() (context.Context, context.CancelFunc) {
		return context.WithTimeout(context.WithoutCancel(r.Context()), finishTimeout)
	}

	finished := false
	defer func() {
		if finished {
			return
		}
		// Handler panic: xoa khoa roi de panic di tiep cho httpx.Recover tra 500.
		ctx, cancel := finishCtx()
		defer cancel()
		if err := store.Abandon(ctx, req, token); err != nil {
			cfg.log.Error("idempotency: khong xoa duoc khoa sau panic", "err", err)
		}
	}()

	r.Body = io.NopCloser(bytes.NewReader(body))
	rec := &recorder{header: http.Header{}}
	next.ServeHTTP(rec, r)
	finished = true

	status := rec.statusCode()
	ctx, cancel := finishCtx()
	defer cancel()
	if status >= http.StatusInternalServerError {
		// Loi he thong khong phai la "ket qua" cua yeu cau: nho no se khoa client vao 503 24 gio.
		if err := store.Abandon(ctx, req, token); err != nil {
			cfg.log.Error("idempotency: khong xoa duoc khoa", "err", err)
		}
	} else {
		res := Response{Status: status, ContentType: rec.header.Get("Content-Type"), Body: rec.body.Bytes()}
		if err := store.Complete(ctx, req, token, res); err != nil {
			// Khoa se tu het lease. Van tra response that cho client.
			cfg.log.Error("idempotency: khong luu duoc response", "err", err, "endpoint", req.Endpoint)
		}
	}

	dst := w.Header()
	for k, v := range rec.header {
		dst[k] = v
	}
	w.WriteHeader(status)
	if _, err := w.Write(rec.body.Bytes()); err != nil {
		cfg.log.Debug("idempotency: khong ghi duoc response", "err", err)
	}
}

func replay(w http.ResponseWriter, res Response) {
	h := w.Header()
	if res.ContentType != "" {
		h.Set("Content-Type", res.ContentType)
	}
	h.Set(ReplayedHeader, "true")
	w.WriteHeader(res.Status)
	_, _ = w.Write(res.Body)
}

// writeProblem la httpx.WriteProblem co `type` rieng de client phan biet loi.
func writeProblem(w http.ResponseWriter, code int, typ, detail string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"type":   typ,
		"title":  http.StatusText(code),
		"status": code,
		"detail": detail,
	})
}

// validKey chap nhan 1-255 ky tu ASCII in duoc, khong khoang trang. Gia tri nay di vao log va
// khoa chinh, nen khong nhan ky tu dieu khien hay chuoi dai tuy y.
func validKey(k string) bool {
	if len(k) == 0 || len(k) > 255 {
		return false
	}
	for i := 0; i < len(k); i++ {
		if k[i] < 0x21 || k[i] > 0x7e {
			return false
		}
	}
	return true
}

// hashRequest rang buoc khoa voi dung noi dung va dung nguoi goi. Moi truong ngan cach bang
// do dai de ("ab","c") khong trung ("a","bc").
func hashRequest(r *http.Request, identity string, body []byte) string {
	h := sha256.New()
	for _, part := range []string{r.Method, r.URL.Path, r.URL.RawQuery, identity} {
		h.Write([]byte(strconv.Itoa(len(part))))
		h.Write([]byte{':'})
		h.Write([]byte(part))
	}
	h.Write(body)
	return hex.EncodeToString(h.Sum(nil))
}

// recorder dem response de chi tra ve sau khi da luu. Co y KHONG ho tro http.Flusher.
type recorder struct {
	header http.Header
	status int
	body   bytes.Buffer
}

func (r *recorder) Header() http.Header { return r.header }

func (r *recorder) WriteHeader(code int) {
	if r.status == 0 {
		r.status = code
	}
}

func (r *recorder) Write(p []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	return r.body.Write(p)
}

func (r *recorder) statusCode() int {
	if r.status == 0 {
		return http.StatusOK
	}
	return r.status
}
