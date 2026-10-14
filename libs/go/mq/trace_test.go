package mq

import (
	"context"
	"testing"

	amqp "github.com/rabbitmq/amqp091-go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

// Test trace xuyen publish/consume (spec AC-4; plan §7 m3, m6), khong can broker.
//
// Ten KHONG export ma test nay gia dinh (Publish va Consume phai dung chung hai ham nay):
//
//	// startPublishSpan mo span SpanKindProducer ten "mq.publish <exchange>/<routingKey>" (tracer "libs/go/mq"),
//	// tra ve ctx con, span, va BAN SAO cua headers (nil an toan) da duoc otelx.InjectAMQP SAU khi mo span.
//	func startPublishSpan(ctx context.Context, exchange, routingKey string, headers amqp091.Table) (context.Context, trace.Span, amqp091.Table)
//	// startConsumeSpan otelx.ExtractAMQP tu headers roi mo span SpanKindConsumer ten "mq.consume <queue>".
//	func startConsumeSpan(ctx context.Context, queue string, headers amqp091.Table) (context.Context, trace.Span)

// newRecorder dat mot TracerProvider ghi span vao bo nho cho toi het test
// (chep tu otelx/otelx_test.go vi helper do khong export). Khong dung t.Parallel.
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

func spanByName(t *testing.T, rec *tracetest.SpanRecorder, name string) sdktrace.ReadOnlySpan {
	t.Helper()
	for _, s := range rec.Ended() {
		if s.Name() == name {
			return s
		}
	}
	var names []string
	for _, s := range rec.Ended() {
		names = append(names, s.Name())
	}
	t.Fatalf("khong thay span %q; co %v", name, names)
	return nil
}

func TestTrace_PublishRoiConsumeCungTraceID(t *testing.T) {
	rec := newRecorder(t)

	root, rootSpan := otel.Tracer("test").Start(context.Background(), "nguoi-goi")
	callerHeaders := amqp.Table{"tenant": "evt-1"}
	_, pubSpan, sent := startPublishSpan(root, "ai.direct", "chat", callerHeaders)
	pubSpan.End()
	rootSpan.End()

	if _, ok := callerHeaders["traceparent"]; ok {
		t.Error("startPublishSpan khong duoc ghi vao map headers cua nguoi goi")
	}
	if sent["tenant"] != "evt-1" {
		t.Errorf("ban sao mat header cua nguoi goi: %v", sent)
	}
	if _, ok := sent["traceparent"].(string); !ok {
		t.Fatalf("ban sao khong co traceparent: %v", sent)
	}

	// Phia consume chi thay headers (nhu sau khi qua broker).
	ctx, consSpan := startConsumeSpan(context.Background(), "ai.chat.q", sent)
	handlerTrace := trace.SpanContextFromContext(ctx).TraceID()
	consSpan.End()

	pub := spanByName(t, rec, "mq.publish ai.direct/chat")
	cons := spanByName(t, rec, "mq.consume ai.chat.q")

	if pub.SpanKind() != trace.SpanKindProducer {
		t.Errorf("span publish kind = %v, muon Producer", pub.SpanKind())
	}
	if cons.SpanKind() != trace.SpanKindConsumer {
		t.Errorf("span consume kind = %v, muon Consumer", cons.SpanKind())
	}
	want := rootSpan.SpanContext().TraceID()
	if pub.SpanContext().TraceID() != want || cons.SpanContext().TraceID() != want || handlerTrace != want {
		t.Fatalf("trace ID khac nhau: goc %s publish %s consume %s handler %s",
			want, pub.SpanContext().TraceID(), cons.SpanContext().TraceID(), handlerTrace)
	}
	// Span consume la con cua span publish (inject sau khi mo span publish).
	if cons.Parent().SpanID() != pub.SpanContext().SpanID() {
		t.Errorf("cha cua span consume = %s, muon span publish %s", cons.Parent().SpanID(), pub.SpanContext().SpanID())
	}
}

func TestTrace_HeadersNilAnToan(t *testing.T) {
	newRecorder(t)
	_, span, sent := startPublishSpan(context.Background(), "ex", "key", nil)
	defer span.End()
	if sent == nil {
		t.Fatal("headers nil -> ban sao phai la map (da inject trace)")
	}
	if _, ok := sent["traceparent"]; !ok {
		t.Errorf("thieu traceparent: %v", sent)
	}
	// Consume headers nil: khong panic, van mo span moi.
	_, cs := startConsumeSpan(context.Background(), "q", nil)
	cs.End()
}

// Ba lan thu qua hang tre: moi lan publish retry dung ctx cua span consume truoc,
// nen trace ID giu nguyen o ca 3 lan (AC-4).
func TestTrace_GiuTraceQuaBaLanRetry(t *testing.T) {
	rec := newRecorder(t)

	root, rootSpan := otel.Tracer("test").Start(context.Background(), "nguoi-goi")
	want := rootSpan.SpanContext().TraceID()
	_, ps, headers := startPublishSpan(root, "ai.direct", "chat", nil)
	ps.End()
	rootSpan.End()

	for attempt := 1; attempt <= 3; attempt++ {
		ctx, cs := startConsumeSpan(context.Background(), "ai.chat.q", copyHeaders(headers))
		if got := trace.SpanContextFromContext(ctx).TraceID(); got != want {
			t.Fatalf("lan %d: trace ID %s, muon %s", attempt, got, want)
		}
		// Ban sao retry: chep header (bo header broker) roi publish trong ctx cua span consume.
		_, rs, next := startPublishSpan(ctx, "ai.retry", "chat", copyHeaders(headers))
		rs.End()
		cs.End()
		headers = next
	}
	n := 0
	for _, s := range rec.Ended() {
		if s.SpanContext().TraceID() != want {
			t.Errorf("span %q trace ID %s, muon %s", s.Name(), s.SpanContext().TraceID(), want)
		}
		if s.Name() == "mq.consume ai.chat.q" {
			n++
		}
	}
	if n != 3 {
		t.Errorf("so span consume = %d, muon 3", n)
	}
}
