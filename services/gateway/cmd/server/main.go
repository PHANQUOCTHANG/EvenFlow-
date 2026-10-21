// Command server la diem vao cua gateway-svc: cua duy nhat tu ngoai vao backend.
//
// Xac minh access token (EV-112), xoa danh tinh client tu khai roi gan lai tu token, gioi han
// toc do, chan /internal/* va chuyen tiep toi service. Khong giu trang thai: chay nhieu replica.
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/eventflow/eventflow/libs/go/httpx"
	"github.com/eventflow/eventflow/libs/go/otelx"
	"github.com/eventflow/eventflow/services/gateway/internal/config"
	"github.com/eventflow/eventflow/services/gateway/internal/proxy"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(log); err != nil {
		log.Error("gateway dung", "err", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if cfg.DevSecret {
		log.Warn("JWT_SECRET chua dat, dang dung secret mac dinh cho moi truong dev")
	}

	// Trace: OTEL_EXPORTER_OTLP_ENDPOINT trong thi khong xuat di dau (xem otelx.Init).
	shutdownTrace, err := otelx.Init(context.Background(), "gateway")
	if err != nil {
		return err
	}
	defer func() {
		sh, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := shutdownTrace(sh); err != nil {
			log.Warn("tat trace loi", "err", err)
		}
	}()

	h, err := proxy.New(cfg, log)
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Khong dat WriteTimeout (xem httpx.NewServer): queue/stream la SSE song lau.
	srv := httpx.NewServer(cfg.HTTPAddr, h)
	upstreams := map[string]string{}
	for name, u := range cfg.Upstreams {
		upstreams[name] = u.String()
	}
	log.Info("gateway dang lang nghe", "addr", cfg.HTTPAddr, "upstreams", upstreams, "cors", cfg.AllowedOrigins)
	return httpx.Run(ctx, srv, 20*time.Second)
}
