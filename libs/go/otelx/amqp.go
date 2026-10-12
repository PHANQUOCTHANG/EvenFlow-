package otelx

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel"
)

// AMQPHeaders la bang header cua mot message AMQP (amqp091.Table co cung kieu
// nen chuyen kieu truc tiep duoc). Hien thuc propagation.TextMapCarrier de trace
// di tiep qua RabbitMQ: consumer noi duoc vao trace cua nguoi gui.
type AMQPHeaders map[string]any

// Get tra ve gia tri header dang chuoi; header khong phai chuoi coi nhu vang.
func (h AMQPHeaders) Get(key string) string {
	switch v := h[key].(type) {
	case string:
		return v
	case []byte:
		return string(v)
	case fmt.Stringer:
		return v.String()
	default:
		return ""
	}
}

// Set ghi header.
func (h AMQPHeaders) Set(key, value string) { h[key] = value }

// Keys liet ke ten header.
func (h AMQPHeaders) Keys() []string {
	keys := make([]string, 0, len(h))
	for k := range h {
		keys = append(keys, k)
	}
	return keys
}

// InjectAMQP ghi trace context hien tai vao headers truoc khi publish.
func InjectAMQP(ctx context.Context, headers AMQPHeaders) {
	otel.GetTextMapPropagator().Inject(ctx, headers)
}

// ExtractAMQP doc trace context tu headers cua message nhan duoc.
func ExtractAMQP(ctx context.Context, headers AMQPHeaders) context.Context {
	return otel.GetTextMapPropagator().Extract(ctx, headers)
}
