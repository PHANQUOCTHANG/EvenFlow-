package otelx

import (
	"context"
	"log/slog"

	"go.opentelemetry.io/otel/trace"
)

// LogHandler boc handler slog de moi dong log ghi voi ctx co span deu mang
// trace_id va span_id. Nho do tu mot dong log nhay thang sang trace trong Jaeger.
//
// Chi co tac dung voi lenh log truyen ctx (log.InfoContext, log.Log); log.Info
// khong co ctx thi khong biet span nao.
func LogHandler(inner slog.Handler) slog.Handler { return logHandler{inner} }

type logHandler struct{ slog.Handler }

func (h logHandler) Handle(ctx context.Context, r slog.Record) error {
	if sc := trace.SpanContextFromContext(ctx); sc.IsValid() {
		r.AddAttrs(
			slog.String("trace_id", sc.TraceID().String()),
			slog.String("span_id", sc.SpanID().String()))
	}
	return h.Handler.Handle(ctx, r)
}

func (h logHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return logHandler{h.Handler.WithAttrs(attrs)}
}

func (h logHandler) WithGroup(name string) slog.Handler {
	return logHandler{h.Handler.WithGroup(name)}
}
