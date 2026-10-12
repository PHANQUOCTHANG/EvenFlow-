package otelx

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"

	"github.com/eventflow/eventflow/libs/go/httpx"
)

// Test viet tu hanh vi can bao dam: span mang MAU route (khong phai URL that),
// trace noi duoc qua HTTP header va qua AMQP header, log co trace_id, va khong
// bat quan trac thi service van chay.

// newRecorder dat mot TracerProvider ghi span vao bo nho cho toi het test.
func newRecorder(t *testing.T) *tracetest.SpanRecorder {
	t.Helper()
	rec := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(rec))
	prevTP, prevProp := otel.GetTracerProvider(), otel.GetTextMapPropagator()
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{}, propagation.Baggage{}))
	t.Cleanup(func() {
		otel.SetTracerProvider(prevTP)
		otel.SetTextMapPropagator(prevProp)
		_ = tp.Shutdown(context.Background())
	})
	return rec
}

func histogramCount(t *testing.T, service, route, status string) uint64 {
	t.Helper()
	families, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range families {
		if f.GetName() != "http_server_request_duration_seconds" {
			continue
		}
		for _, m := range f.GetMetric() {
			labels := map[string]string{}
			for _, l := range m.GetLabel() {
				labels[l.GetName()] = l.GetValue()
			}
			if labels["service"] == service && labels["route"] == route && labels["status"] == status {
				return m.GetHistogram().GetSampleCount()
			}
		}
	}
	return 0
}

func serve(t *testing.T, service string, h http.HandlerFunc, req *http.Request) *httptest.ResponseRecorder {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/events/{eventID}/holds", h)
	rec := httptest.NewRecorder()
	HTTP(service)(mux).ServeHTTP(rec, req)
	return rec
}

func TestHTTP_SpanVaMetricTheoMauRoute(t *testing.T) {
	spans := newRecorder(t)

	req := httptest.NewRequest("POST", "/v1/events/ev-123/holds", nil)
	serve(t, "svc-a", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusConflict)
	}, req)

	ended := spans.Ended()
	if len(ended) != 1 {
		t.Fatalf("muon 1 span, nhan %d", len(ended))
	}
	if got, want := ended[0].Name(), "POST /v1/events/{eventID}/holds"; got != want {
		t.Errorf("ten span = %q, muon %q", got, want)
	}
	for _, kv := range ended[0].Attributes() {
		if strings.Contains(kv.Value.String(), "ev-123") {
			t.Errorf("span khong duoc chua URL that: %s=%s", kv.Key, kv.Value.String())
		}
	}
	if n := histogramCount(t, "svc-a", "/v1/events/{eventID}/holds", "409"); n != 1 {
		t.Errorf("histogram theo route/status = %d mau, muon 1", n)
	}
}

// Chuoi middleware THAT cua service: RequestID tao ban sao request (WithContext),
// nen neu otelx.HTTP dat sai cho thi moi route thanh "unmatched". Unit test goi HTTP
// truc tiep len mux khong bat duoc loi nay.
func TestHTTP_TrongChuoiMiddlewareThat_VanThayMauRoute(t *testing.T) {
	spans := newRecorder(t)
	log := slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil))
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/events/{eventID}/holds", func(http.ResponseWriter, *http.Request) {})

	h := httpx.Chain(mux, httpx.RequestID(), HTTP("svc-chain"), httpx.AccessLog(log), httpx.Recover(log))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("POST", "/v1/events/ev-9/holds", nil))

	if n := histogramCount(t, "svc-chain", "/v1/events/{eventID}/holds", "200"); n != 1 {
		t.Errorf("route phai la mau, khong phai unmatched; so mau = %d", n)
	}
	ended := spans.Ended()
	if len(ended) != 1 || ended[0].Name() != "POST /v1/events/{eventID}/holds" {
		t.Errorf("ten span sai: %+v", ended)
	}
}

func TestHTTP_5xxDanhDauSpanLoi(t *testing.T) {
	spans := newRecorder(t)

	serve(t, "svc-b", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}, httptest.NewRequest("POST", "/v1/events/x/holds", nil))

	ended := spans.Ended()
	if len(ended) != 1 || ended[0].Status().Code.String() != "Error" {
		t.Fatalf("span 5xx phai o trang thai Error: %+v", ended)
	}
}

func TestHTTP_NoiTraceTuHeaderTraceparent(t *testing.T) {
	spans := newRecorder(t)

	const traceID = "4bf92f3577b34da6a3ce929d0e0e4736"
	req := httptest.NewRequest("POST", "/v1/events/x/holds", nil)
	req.Header.Set("traceparent", "00-"+traceID+"-00f067aa0ba902b7-01")
	serve(t, "svc-c", func(http.ResponseWriter, *http.Request) {}, req)

	ended := spans.Ended()
	if len(ended) != 1 {
		t.Fatalf("muon 1 span, nhan %d", len(ended))
	}
	if got := ended[0].SpanContext().TraceID().String(); got != traceID {
		t.Errorf("trace id = %s, muon %s (phai noi tiep trace cua nguoi goi)", got, traceID)
	}
	if got := ended[0].Parent().SpanID().String(); got != "00f067aa0ba902b7" {
		t.Errorf("parent span = %s", got)
	}
}

func TestHTTP_KhongKhopRoute_NhanUnmatched(t *testing.T) {
	newRecorder(t)

	rec := httptest.NewRecorder()
	HTTP("svc-d")(http.NewServeMux()).ServeHTTP(rec, httptest.NewRequest("GET", "/khong/co/that/123", nil))

	if n := histogramCount(t, "svc-d", "unmatched", "404"); n != 1 {
		t.Errorf("route khong khop phai gop vao nhan 'unmatched' (tranh no cardinality), nhan %d", n)
	}
}

func TestLogHandler_GanTraceID(t *testing.T) {
	newRecorder(t)
	var buf bytes.Buffer
	log := slog.New(LogHandler(slog.NewJSONHandler(&buf, nil)))

	ctx, span := otel.Tracer("test").Start(context.Background(), "op")
	defer span.End()
	log.InfoContext(ctx, "co span")

	var line map[string]any
	if err := json.Unmarshal(buf.Bytes(), &line); err != nil {
		t.Fatal(err)
	}
	want := span.SpanContext().TraceID().String()
	if line["trace_id"] != want {
		t.Errorf("trace_id = %v, muon %s", line["trace_id"], want)
	}
	if line["span_id"] != span.SpanContext().SpanID().String() {
		t.Errorf("span_id = %v", line["span_id"])
	}
}

func TestLogHandler_KhongCoSpan_KhongThemTruong(t *testing.T) {
	var buf bytes.Buffer
	log := slog.New(LogHandler(slog.NewJSONHandler(&buf, nil))).With("k", "v")

	log.InfoContext(context.Background(), "khong span")

	if s := buf.String(); strings.Contains(s, "trace_id") {
		t.Errorf("khong co span thi khong duoc co trace_id: %s", s)
	}
	if !strings.Contains(buf.String(), `"k":"v"`) {
		t.Errorf("WithAttrs bi mat: %s", buf.String())
	}
}

func TestAMQP_TraceDiQuaHeader(t *testing.T) {
	newRecorder(t)

	ctx, span := otel.Tracer("producer").Start(context.Background(), "publish")
	defer span.End()

	headers := AMQPHeaders{"x-khac": 42}
	InjectAMQP(ctx, headers)
	if headers.Get("traceparent") == "" {
		t.Fatal("Inject phai ghi header traceparent")
	}

	got := trace.SpanContextFromContext(ExtractAMQP(context.Background(), headers))
	if got.TraceID() != span.SpanContext().TraceID() {
		t.Errorf("trace id sau Extract = %s, muon %s", got.TraceID(), span.SpanContext().TraceID())
	}
	if !got.IsRemote() {
		t.Error("span context lay tu header phai la remote")
	}
}

func TestAMQPHeaders_GetChapNhanByteVaBoQuaKieuLa(t *testing.T) {
	h := AMQPHeaders{"a": "chuoi", "b": []byte("byte"), "c": 7}
	if h.Get("a") != "chuoi" || h.Get("b") != "byte" {
		t.Errorf("Get sai: a=%q b=%q", h.Get("a"), h.Get("b"))
	}
	if h.Get("c") != "" || h.Get("khong-co") != "" {
		t.Error("kieu la / header vang phai tra chuoi rong")
	}
	if len(h.Keys()) != 3 {
		t.Errorf("Keys = %v", h.Keys())
	}
}

func TestInit_KhongCoEndpoint_VanChay(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")
	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", "")
	prevTP, prevProp := otel.GetTracerProvider(), otel.GetTextMapPropagator()
	t.Cleanup(func() {
		otel.SetTracerProvider(prevTP)
		otel.SetTextMapPropagator(prevProp)
	})

	shutdown, err := Init(context.Background(), "svc-test")
	if err != nil {
		t.Fatalf("Init khong co endpoint phai thanh cong: %v", err)
	}
	// Van tao span hop le de log co trace_id du khong xuat di dau.
	_, span := otel.Tracer("t").Start(context.Background(), "x")
	if !span.SpanContext().IsValid() {
		t.Error("span phai hop le du khong co exporter")
	}
	span.End()

	if err := shutdown(context.Background()); err != nil {
		t.Errorf("shutdown: %v", err)
	}
}

func TestErrorHandler_ChiLogMotLanMoiKhoang(t *testing.T) {
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })

	h := newErrorHandler(1 << 40) // khoang rat dai
	for range 5 {
		h.Handle(context.DeadlineExceeded)
	}
	if n := strings.Count(buf.String(), "opentelemetry"); n != 1 {
		t.Errorf("loi lap lai phai chi log 1 lan, log %d lan", n)
	}
}

func TestMetricsHandler_PhucVuMetric(t *testing.T) {
	newRecorder(t)
	serve(t, "svc-e", func(http.ResponseWriter, *http.Request) {},
		httptest.NewRequest("POST", "/v1/events/x/holds", nil))

	rec := httptest.NewRecorder()
	MetricsHandler().ServeHTTP(rec, httptest.NewRequest("GET", "/metrics", nil))

	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `http_server_request_duration_seconds_bucket{method="POST",route="/v1/events/{eventID}/holds",service="svc-e"`) {
		t.Errorf("thieu metric trong /metrics (status %d)", rec.Code)
	}
}
