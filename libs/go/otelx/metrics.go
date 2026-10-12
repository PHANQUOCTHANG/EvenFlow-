package otelx

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/eventflow/eventflow/libs/go/httpx"
)

// MetricsHandler phuc vu metric Prometheus cua process (Go runtime, process va
// cac metric da dang ky vao registry mac dinh).
//
// LUU Y: dang ky o cong cong thi bat ky ai toi duoc cong do deu doc duoc. Gateway
// khong duoc route /metrics ra ngoai.
func MetricsHandler() http.Handler { return promhttp.Handler() }

// ServeMetrics chay mot server chi phuc vu /metrics, cho process khong co HTTP
// API (admit controller, worker). Chay cho toi khi ctx bi huy.
func ServeMetrics(ctx context.Context, addr string, log *slog.Logger) error {
	mux := http.NewServeMux()
	mux.Handle("GET /metrics", MetricsHandler())
	srv := httpx.NewServer(addr, mux)
	log.Info("metrics dang lang nghe", "addr", addr)
	return httpx.Run(ctx, srv, 5*time.Second)
}
