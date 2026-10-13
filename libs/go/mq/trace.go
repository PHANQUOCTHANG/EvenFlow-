package mq

import (
	"context"

	amqp "github.com/rabbitmq/amqp091-go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"

	"github.com/eventflow/eventflow/libs/go/otelx"
)

const tracerName = "libs/go/mq"

// startPublishSpan mo span Producer roi inject trace vao BAN SAO cua headers
// (nil an toan, khong sua map cua nguoi goi).
func startPublishSpan(ctx context.Context, exchange, routingKey string, headers amqp.Table) (context.Context, trace.Span, amqp.Table) {
	ctx, span := otel.Tracer(tracerName).Start(ctx, "mq.publish "+exchange+"/"+routingKey,
		trace.WithSpanKind(trace.SpanKindProducer))
	out := make(amqp.Table, len(headers)+2)
	for k, v := range headers {
		out[k] = v
	}
	otelx.InjectAMQP(ctx, otelx.AMQPHeaders(out))
	return ctx, span, out
}

// startConsumeSpan lay trace context tu headers roi mo span Consumer lam con.
func startConsumeSpan(ctx context.Context, queue string, headers amqp.Table) (context.Context, trace.Span) {
	ctx = otelx.ExtractAMQP(ctx, otelx.AMQPHeaders(headers))
	return otel.Tracer(tracerName).Start(ctx, "mq.consume "+queue, trace.WithSpanKind(trace.SpanKindConsumer))
}
