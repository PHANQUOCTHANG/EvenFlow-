// Command worker chay cac tien trinh nen cua ticketing:
//   - hold sweeper: nha lenh giu ghe het han, tra ve vao kho (BR-O2, BR-O7)
//   - inventory seeder: nap ton kho tu Postgres vao Redis (ban toi thieu cua EVF-37)
//   - TODO(EVF-36): outbox relay
//   - TODO(EVF-37): inventory reconciler
//
// Tach khoi server la co y: mot dot tra kho lon khong duoc phep lam cham
// duong di nong cua khach dang mua ve.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/redis/go-redis/v9"

	"github.com/eventflow/eventflow/libs/go/otelx"
	metricsadapter "github.com/eventflow/eventflow/services/ticketing/internal/adapter/metrics"
	postgresadapter "github.com/eventflow/eventflow/services/ticketing/internal/adapter/postgres"
	redisadapter "github.com/eventflow/eventflow/services/ticketing/internal/adapter/redis"
	"github.com/eventflow/eventflow/services/ticketing/internal/worker"
)

func main() {
	log := slog.New(otelx.LogHandler(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})))
	if err := run(log); err != nil && !errors.Is(err, context.Canceled) {
		log.Error("worker dung bat thuong", "err", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	shutdownTrace, err := otelx.Init(ctx, "ticketing-worker")
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

	pool, err := pgxpool.New(ctx, os.Getenv("DATABASE_URL"))
	if err != nil {
		return err
	}
	defer pool.Close()

	rdb := redis.NewClient(&redis.Options{Addr: env("REDIS_ADDR", "localhost:6379")})
	defer func() {
		if err := rdb.Close(); err != nil {
			log.Warn("dong redis client loi", "err", err)
		}
	}()

	gate, err := redisadapter.NewGate(ctx, rdb)
	if err != nil {
		return err
	}

	eventIDs := splitCSV(os.Getenv("ACTIVE_EVENT_IDS"))
	if len(eventIDs) == 0 {
		log.Warn("khong co su kien nao de quet; dat ACTIVE_EVENT_IDS")
	}

	repo := postgresadapter.NewRepository(pool)

	prom := metricsadapter.New(prometheus.DefaultRegisterer)
	sweeper := worker.NewHoldSweeper(
		gate,
		repo,
		logPublisher{log}, // TODO(EVF-36): thay bang publisher RabbitMQ
		worker.DefaultSweeperConfig(eventIDs),
		log,
	).WithMetrics(prom)
	seeder := worker.NewInventorySeeder(
		repo, gate, worker.DefaultSeederConfig(eventIDs), log,
	)

	log.Info("worker bat dau", "so_su_kien", len(eventIDs),
		"chu_ky_quet", time.Second, "chu_ky_nap_kho", worker.DefaultSeederConfig(nil).Interval)

	// Hai tien trinh nen doc lap: loi o mot ben khong duoc dung ben kia, nhung
	// khi ctx bi huy (SIGTERM) ca hai deu phai dung.
	errs := make(chan error, 3)
	go func() { errs <- sweeper.Run(ctx) }()
	go func() { errs <- seeder.Run(ctx) }()
	go func() { errs <- otelx.ServeMetrics(ctx, env("METRICS_ADDR", ":9092"), log) }()

	err = <-errs
	stop()
	<-errs
	<-errs
	return err
}

// logPublisher la publisher tam thoi cho local dev.
type logPublisher struct{ log *slog.Logger }

func (p logPublisher) Publish(_ context.Context, routingKey string, payload any) error {
	b, _ := json.Marshal(payload)
	p.log.Debug("su kien", "routing_key", routingKey, "payload", string(b))
	return nil
}

func splitCSV(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
