package otelx

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// StartSpan mo mot span con cho mot buoc ben trong (goi Redis, giao dich Postgres).
//
// Dung o tang adapter, nhung cho ma tung buoc cham ha tang: span HTTP chi cho
// biet request cham, con span nay cho biet cham o dau.
//
//	ctx, end := otelx.StartSpan(ctx, "redis.hold")
//	defer func() { end(err) }()
func StartSpan(ctx context.Context, name string, attrs ...attribute.KeyValue) (context.Context, func(err error)) {
	ctx, span := otel.Tracer("eventflow/otelx").Start(ctx, name,
		trace.WithSpanKind(trace.SpanKindInternal), trace.WithAttributes(attrs...))
	return ctx, func(err error) {
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		}
		span.End()
	}
}
