// Command worker chay cac tien trinh nen cua ticketing:
//   - hold sweeper: nha lenh giu ghe het han, tra ve vao kho (BR-O2, BR-O7)
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
	"github.com/redis/go-redis/v9"

	postgresadapter "github.com/eventflow/eventflow/services/ticketing/internal/adapter/postgres"
	redisadapter "github.com/eventflow/eventflow/services/ticketing/internal/adapter/redis"
	"github.com/eventflow/eventflow/services/ticketing/internal/worker"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	if err := run(log); err != nil && !errors.Is(err, context.Canceled) {
		log.Error("worker dung bat thuong", "err", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	pool, err := pgxpool.New(ctx, os.Getenv("DATABASE_URL"))
	if err != nil {
		return err
	}
	defer pool.Close()

	rdb := redis.NewClient(&redis.Options{Addr: env("REDIS_ADDR", "localhost:6379")})
	defer rdb.Close()

	gate, err := redisadapter.NewGate(ctx, rdb)
	if err != nil {
		return err
	}

	eventIDs := splitCSV(os.Getenv("ACTIVE_EVENT_IDS"))
	if len(eventIDs) == 0 {
		log.Warn("khong co su kien nao de quet; dat ACTIVE_EVENT_IDS")
	}

	sweeper := worker.NewHoldSweeper(
		gate,
		postgresadapter.NewRepository(pool),
		logPublisher{log}, // TODO(EVF-36): thay bang publisher RabbitMQ
		worker.DefaultSweeperConfig(eventIDs),
		log,
	)

	log.Info("hold sweeper bat dau", "so_su_kien", len(eventIDs), "chu_ky", time.Second)
	return sweeper.Run(ctx)
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
