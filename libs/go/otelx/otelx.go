// Package otelx dung chung phan quan trac cho moi service: trace (OpenTelemetry),
// metric (Prometheus) va log gan trace_id.
//
// Y tuong chinh: bat quan trac khong duoc la dieu kien de service chay. Khong co
// OTEL_EXPORTER_OTLP_ENDPOINT thi trace van duoc tao (de log co trace_id) nhung
// khong gui di dau; Jaeger chet khong duoc lam cham hay lam hong request.
package otelx

import (
	"context"
	"log/slog"
	"os"
	"sync/atomic"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// Init cau hinh trace cho process va tra ve ham shutdown (xa not span con dem).
//
// Cau hinh qua bien moi truong chuan cua OpenTelemetry:
//   - OTEL_EXPORTER_OTLP_ENDPOINT: noi nhan trace (vd http://jaeger:4318). Trong
//     thi khong xuat di dau.
//   - OTEL_TRACES_SAMPLER / OTEL_TRACES_SAMPLER_ARG: lay mau. Tren duong nong luc
//     mo ban hay dat parentbased_traceidratio voi ti le thap.
func Init(ctx context.Context, service string) (shutdown func(context.Context) error, err error) {
	res, err := resource.Merge(resource.Default(),
		resource.NewSchemaless(attribute.String("service.name", service)))
	if err != nil {
		return nil, err
	}

	opts := []sdktrace.TracerProviderOption{sdktrace.WithResource(res)}
	if os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT") != "" || os.Getenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT") != "" {
		exp, err := otlptracehttp.New(ctx)
		if err != nil {
			return nil, err
		}
		// Batcher gui bat dong bo: request khong bao gio cho Jaeger.
		opts = append(opts, sdktrace.WithBatcher(exp))
	}

	tp := sdktrace.NewTracerProvider(opts...)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{}, propagation.Baggage{}))
	otel.SetErrorHandler(newErrorHandler(time.Minute))

	return tp.Shutdown, nil
}

// errorHandler gioi han tan suat log loi cua OpenTelemetry. Khi Jaeger chet, moi
// lan xuat that bai deu sinh mot loi; khong chan lai thi log bi ngap dung luc
// can doc log nhat.
type errorHandler struct {
	every time.Duration
	last  atomic.Int64
}

func newErrorHandler(every time.Duration) otel.ErrorHandler {
	return &errorHandler{every: every}
}

func (h *errorHandler) Handle(err error) {
	now := time.Now().UnixNano()
	last := h.last.Load()
	if now-last < int64(h.every) || !h.last.CompareAndSwap(last, now) {
		return
	}
	slog.Warn("opentelemetry bao loi (chi log moi phut mot lan)", "err", err)
}
