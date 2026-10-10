package httpx

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net/http"
	"regexp"
	"runtime/debug"
	"strings"
	"time"
)

// Middleware boc mot http.Handler.
type Middleware func(http.Handler) http.Handler

// Chain ap cac middleware len h. Phan tu DAU TIEN la lop ngoai cung, nen
// Chain(h, RequestID(), AccessLog(l), Recover(l)) chay theo thu tu do va
// AccessLog nhin thay ca ma 500 do Recover tra ve.
func Chain(h http.Handler, mws ...Middleware) http.Handler {
	for i := len(mws) - 1; i >= 0; i-- {
		h = mws[i](h)
	}
	return h
}

// RequestIDHeader la header mang ma truy vet request.
const RequestIDHeader = "X-Request-Id"

type ctxKey struct{}

// RequestIDFrom lay request id da duoc RequestID middleware gan vao ctx.
func RequestIDFrom(ctx context.Context) string {
	id, _ := ctx.Value(ctxKey{}).(string)
	return id
}

// Chi chap nhan id ngan, ky tu an toan: gia tri nay di thang vao log va header
// phan hoi, nen khong de client nhet xuong dong hay chuoi dai tuy y.
var validRequestID = regexp.MustCompile(`^[A-Za-z0-9._-]{1,64}$`)

// RequestID giu X-Request-Id hop le tu ben ngoai (gateway), nguoc lai tu sinh.
func RequestID() Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id := r.Header.Get(RequestIDHeader)
			if !validRequestID.MatchString(id) {
				id = newRequestID()
			}
			w.Header().Set(RequestIDHeader, id)
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxKey{}, id)))
		})
	}
}

func newRequestID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		panic("khong doc duoc nguon ngau nhien: " + err.Error())
	}
	return hex.EncodeToString(b)
}

// statusWriter ghi nho ma trang thai va so byte da gui.
//
// PHAI giu http.Flusher: handler SSE cua phong cho kiem tra w.(http.Flusher), neu
// wrapper che mat no thi stream tra 500 "streaming khong duoc ho tro".
type statusWriter struct {
	http.ResponseWriter
	status int
	bytes  int64
	wrote  bool
}

func (s *statusWriter) WriteHeader(code int) {
	if !s.wrote {
		s.status, s.wrote = code, true
	}
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusWriter) Write(p []byte) (int, error) {
	if !s.wrote {
		s.status, s.wrote = http.StatusOK, true
	}
	n, err := s.ResponseWriter.Write(p)
	s.bytes += int64(n)
	return n, err
}

// Flush chuyen tiep cho writer goc neu no ho tro.
func (s *statusWriter) Flush() {
	if f, ok := s.ResponseWriter.(http.Flusher); ok {
		if !s.wrote {
			s.status, s.wrote = http.StatusOK, true
		}
		f.Flush()
	}
}

// Unwrap cho http.ResponseController tim duoc writer goc.
func (s *statusWriter) Unwrap() http.ResponseWriter { return s.ResponseWriter }

// track tra ve statusWriter hien co, hoac boc moi neu chua co.
func track(w http.ResponseWriter) *statusWriter {
	if sw, ok := w.(*statusWriter); ok {
		return sw
	}
	return &statusWriter{ResponseWriter: w, status: http.StatusOK}
}

// Recover bien panic trong handler thanh 500 dang problem va ghi stack vao log.
//
// Khong tra chi tiet panic cho client. http.ErrAbortHandler duoc nem lai vi do
// la tin hieu co y cua net/http, khong phai loi.
func Recover(log *slog.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			sw := track(w)
			defer func() {
				rec := recover()
				if rec == nil {
					return
				}
				if rec == http.ErrAbortHandler {
					panic(rec)
				}
				log.Error("panic trong handler",
					"panic", rec,
					"method", r.Method,
					"route", routeOf(r),
					"request_id", RequestIDFrom(r.Context()),
					"stack", string(debug.Stack()))
				// Neu da gui header thi khong the doi ma trang thai nua.
				if !sw.wrote {
					WriteProblem(sw, http.StatusInternalServerError, "loi he thong, vui long thu lai")
				}
			}()
			next.ServeHTTP(sw, r)
		})
	}
}

// AccessLog ghi mot dong log cho moi request.
//
// Chi ghi mau route (vd "POST /v1/events/{eventID}/holds"), KHONG ghi URL that,
// body hay header dinh danh: URL co the chua id, va log khong duoc chua PII
// dang thuong (EVF-15). Probe suc khoe ghi o muc Debug de khong ngap log.
func AccessLog(log *slog.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			sw := track(w)
			next.ServeHTTP(sw, r)

			route := routeOf(r)
			level := slog.LevelInfo
			if strings.HasSuffix(route, "/healthz") || strings.HasSuffix(route, "/readyz") {
				level = slog.LevelDebug
			}
			log.Log(r.Context(), level, "http",
				"method", r.Method,
				"route", route,
				"status", sw.status,
				"bytes", sw.bytes,
				"duration_ms", time.Since(start).Milliseconds(),
				"request_id", RequestIDFrom(r.Context()))
		})
	}
}

// routeOf tra ve mau route da khop. ServeMux ghi r.Pattern TAI CHO tren cung mot
// *http.Request, nen doc duoc sau khi next.ServeHTTP tra ve.
func routeOf(r *http.Request) string {
	if r.Pattern != "" {
		return r.Pattern
	}
	return "unmatched"
}
