// Command worker chay cac tien trinh nen cua ticketing:
//   - hold sweeper: nha lenh giu ghe het han, tra ve vao kho (BR-O2, BR-O7)
//   - inventory seeder: nap ton kho tu Postgres vao Redis vao bucket con thieu (nhanh, 5 giay)
//   - inventory reconciler: so Redis voi Postgres moi 10 giay va sua lech (EV-132)
//   - outbox relay: relay su kien tu outbox_events sang RabbitMQ (EV-131, EVF-36)
//
// Tach khoi server la co y: mot dot tra kho lon khong duoc phep lam cham
// duong di nong cua khach dang mua ve.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/redis/go-redis/v9"

	"github.com/eventflow/eventflow/libs/go/mq"
	"github.com/eventflow/eventflow/libs/go/otelx"
	metricsadapter "github.com/eventflow/eventflow/services/ticketing/internal/adapter/metrics"
	mqadapter "github.com/eventflow/eventflow/services/ticketing/internal/adapter/mq"
	postgresadapter "github.com/eventflow/eventflow/services/ticketing/internal/adapter/postgres"
	redisadapter "github.com/eventflow/eventflow/services/ticketing/internal/adapter/redis"
	"github.com/eventflow/eventflow/services/ticketing/internal/port"
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

	var pub port.Publisher = logPublisher{log}
	var outboxRelay *worker.OutboxRelay

	if rabbitURL := os.Getenv("RABBITMQ_URL"); rabbitURL != "" {
		amqpConn, err := amqp.Dial(rabbitURL)
		if err != nil {
			return fmt.Errorf("ket noi rabbitmq: %w", err)
		}
		defer func() {
			if err := amqpConn.Close(); err != nil {
				log.Warn("dong rabbitmq connection loi", "err", err)
			}
		}()

		ch, err := amqpConn.Channel()
		if err != nil {
			return fmt.Errorf("mo rabbitmq channel: %w", err)
		}
		defer func() {
			if err := ch.Close(); err != nil {
				log.Warn("dong rabbitmq channel loi", "err", err)
			}
		}()

		if err := mq.Declare(ch, mq.TicketingTopology()); err != nil {
			return fmt.Errorf("declare ticketing topology: %w", err)
		}

		mqPub, err := mq.NewPublisher(ch)
		if err != nil {
			return fmt.Errorf("tao rabbitmq publisher: %w", err)
		}

		pub = mqadapter.NewPublisherAdapter(mqPub, "ticketing.events")
		outboxRelay = worker.NewOutboxRelay(
			pool,
			mqPub,
			"ticketing.events",
			worker.DefaultOutboxRelayConfig(),
			log,
		)
		log.Info("da khoi tao rabbitmq publisher va outbox relay", "exchange", "ticketing.events")
	} else {
		log.Info("khong co RABBITMQ_URL; dung logPublisher cho dev")
	}

	prom := metricsadapter.New(prometheus.DefaultRegisterer)
	sweeper := worker.NewHoldSweeper(
		gate,
		repo,
		pub,
		worker.DefaultSweeperConfig(eventIDs),
		log,
	).WithMetrics(prom)
	seeder := worker.NewInventorySeeder(
		repo, gate, worker.DefaultSeederConfig(eventIDs), log,
	)
	reconciler := worker.NewInventoryReconciler(
		repo, repo, gate, worker.DefaultReconcilerConfig(eventIDs), log,
	).WithMetrics(prom)

	// EV-146: bao het ve cho phong cho (co meta.sold_out / stock_zero).
	soldOut := worker.NewSoldOutWatcher(
		worker.NewTierSource(repo), gate, gate, worker.DefaultSoldOutWatcherConfig(eventIDs), log,
	)

	log.Info("worker bat dau", "so_su_kien", len(eventIDs),
		"chu_ky_quet", time.Second, "chu_ky_nap_kho", worker.DefaultSeederConfig(nil).Interval,
		"chu_ky_doi_soat", worker.DefaultReconcilerConfig(nil).Interval)

	errs := make(chan error, 6)
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		errs <- sweeper.Run(ctx)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		errs <- seeder.Run(ctx)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		errs <- reconciler.Run(ctx)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		errs <- soldOut.Run(ctx)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		errs <- otelx.ServeMetrics(ctx, env("METRICS_ADDR", ":9092"), log)
	}()

	if outboxRelay != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- outboxRelay.Run(ctx)
		}()
	}

	err = <-errs
	stop()
	wg.Wait()
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
