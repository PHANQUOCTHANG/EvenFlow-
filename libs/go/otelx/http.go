package otelx

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"

	"github.com/eventflow/eventflow/libs/go/httpx"
)

// Bucket min hon mac dinh: duong nong ky vong tra loi trong vai mili giay, va
// p99 la con so HealthProbe cua admit controller can do duoc.
var httpDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
	Name:    "http_server_request_duration_seconds",
	Help:    "Thoi gian xu ly request HTTP, theo route (khong theo URL that).",
	Buckets: []float64{.001, .0025, .005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5},
}, []string{"service", "method", "route", "status"})

// HTTP tra ve middleware tao span va do thoi gian cho moi request.
//
// THU TU QUAN TRONG: dat ngay SAU RequestID va TRUOC AccessLog/Recover:
//
//	httpx.Chain(mux, httpx.RequestID(), otelx.HTTP(svc), httpx.AccessLog(l), httpx.Recover(l))
//
// ServeMux ghi route da khop (r.Pattern) len request ma NO nhan duoc. Moi middleware
// tao ban sao request (WithContext) dat TRUOC HTTP se khien Pattern nam o ban sao
// khac va moi route bi gan nhan "unmatched". Cac middleware phia trong HTTP phai
// chuyen nguyen con tro request xuong mux. AccessLog va Recover da lam dung.
//
// Span van bao AccessLog/Recover, nen log cua chung mang duoc trace_id.
//
// Nhan "route" la MAU route (vd "/v1/events/{eventID}/holds"), khong phai URL
// that: URL that chua id va se lam so luong chuoi metric no tung.
func HTTP(service string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := otel.GetTextMapPropagator().Extract(r.Context(), propagation.HeaderCarrier(r.Header))
			ctx, span := otel.Tracer("eventflow/otelx").Start(ctx, r.Method,
				trace.WithSpanKind(trace.SpanKindServer),
				trace.WithAttributes(attribute.String("http.request.method", r.Method)))
			defer span.End()

			sw := httpx.Track(w)
			start := time.Now()
			// Giu con tro cua BAN SAO: WithContext tao request moi, ServeMux ghi
			// Pattern len chinh request no nhan duoc chu khong len `r` goc.
			req := r.WithContext(ctx)
			next.ServeHTTP(sw, req)

			// Pattern chi doc duoc SAU khi handler chay xong.
			route := routeLabel(req.Pattern)
			status := sw.Status()

			span.SetName(r.Method + " " + route)
			span.SetAttributes(
				attribute.String("http.route", route),
				attribute.Int("http.response.status_code", status))
			if status >= 500 {
				span.SetStatus(codes.Error, http.StatusText(status))
			}

			httpDuration.WithLabelValues(service, r.Method, route, strconv.Itoa(status)).
				Observe(time.Since(start).Seconds())
		})
	}
}

// routeLabel bo tien to method ("POST /v1/x" -> "/v1/x") va dat nhan cho request
// khong khop route nao.
func routeLabel(pattern string) string {
	if pattern == "" {
		return "unmatched"
	}
	if i := strings.IndexByte(pattern, ' '); i > 0 {
		return pattern[i+1:]
	}
	return pattern
}
