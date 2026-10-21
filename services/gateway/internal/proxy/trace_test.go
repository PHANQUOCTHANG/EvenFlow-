package proxy_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/eventflow/eventflow/libs/go/otelx"
)

// EV-103: gateway nhan traceparent cua client (hoac tao trace moi) roi chuyen tiep traceparent CUNG trace ID
// toi upstream, voi span cha la span cua gateway (khac span cua client). Nho do Jaeger thay mot yeu cau di qua
// nhieu service thanh mot trace.
func TestGateway_NoiTraceSangUpstream(t *testing.T) {
	shutdown, err := otelx.Init(context.Background(), "gateway-test")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = shutdown(context.Background()) })
	f := newFixture(t)

	const traceID = "4bf92f3577b34da6a3ce929d0e0e4736"
	const clientSpan = "00f067aa0ba902b7"
	resp := f.do(t, http.MethodGet, "/api/v1/events/e1", "", map[string]string{
		"traceparent": "00-" + traceID + "-" + clientSpan + "-01",
	}, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d", resp.StatusCode)
	}

	tp := f.up.last(t).Header.Get("traceparent")
	parts := strings.Split(tp, "-")
	if len(parts) != 4 || parts[1] != traceID {
		t.Fatalf("upstream nhan traceparent %q, muon cung trace ID %s", tp, traceID)
	}
	if parts[2] == clientSpan {
		t.Errorf("span cha toi upstream van la span cua client (%s): gateway phai tao span cua minh", clientSpan)
	}
}

// Client khong gui traceparent: gateway tu khoi tao trace va van chuyen tiep cho upstream.
func TestGateway_KhoiTaoTraceKhiClientKhongGui(t *testing.T) {
	shutdown, err := otelx.Init(context.Background(), "gateway-test")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = shutdown(context.Background()) })
	f := newFixture(t)

	f.do(t, http.MethodGet, "/api/v1/events/e1", "", nil, "")
	tp := f.up.last(t).Header.Get("traceparent")
	if parts := strings.Split(tp, "-"); len(parts) != 4 || len(parts[1]) != 32 {
		t.Fatalf("upstream nhan traceparent %q", tp)
	}
}
