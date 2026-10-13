// Command worker chay cac tien trinh nen cua ticketing:
//   - hold sweeper: nha lenh giu ghe het han, tra ve vao kho (BR-O2, BR-O7)
//   - inventory seeder: nap ton kho tu Postgres vao Redis (ban toi thieu cua EVF-37)
//   - outbox relay: doc outbox_events va publish ra RabbitMQ (libs/go/outbox, EVF-36)
//   - TODO(EVF-37): inventory reconciler
//
// Tach khoi server la co y: mot dot tra kho lon khong duoc phep lam cham
// duong di nong cua khach dang mua ve.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/redis/go-redis/v9"

	"github.com/eventflow/eventflow/libs/go/mq"
	"github.com/eventflow/eventflow/libs/go/otelx"
	"github.com/eventflow/eventflow/libs/go/outbox"
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
	cfg, err := loadConfig(os.Getenv)
	if err != nil {
		return err
	}

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

	poolCfg, err := newPoolConfig(cfg.DatabaseURL)
	if err != nil {
		return err
	}
	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return fmt.Errorf("mo pool postgres: %w", err)
	}
	defer pool.Close()

	rdb := redis.NewClient(&redis.Options{Addr: cfg.RedisAddr})
	defer func() {
		if err := rdb.Close(); err != nil {
			log.Warn("dong redis client loi", "err", err)
		}
	}()

	gate, err := redisadapter.NewGate(ctx, rdb)
	if err != nil {
		return err
	}

	eventIDs := cfg.EventIDs
	if len(eventIDs) == 0 {
		log.Warn("khong co su kien nao de quet; dat ACTIVE_EVENT_IDS")
	}

	repo := postgresadapter.NewRepository(pool)

	prom := metricsadapter.New(prometheus.DefaultRegisterer)
	sweeper := worker.NewHoldSweeper(
		gate,
		repo,
		worker.DefaultSweeperConfig(eventIDs),
		log,
	).WithMetrics(prom)
	seeder := worker.NewInventorySeeder(
		repo, gate, worker.DefaultSeederConfig(eventIDs), log,
	)

	log.Info("worker bat dau", "so_su_kien", len(eventIDs),
		"chu_ky_quet", time.Second, "chu_ky_nap_kho", worker.DefaultSeederConfig(nil).Interval)

	relay, err := outbox.NewRelay(pool, amqpDialer(cfg.AMQPURL), relayConfig(prometheus.DefaultRegisterer))
	if err != nil {
		return fmt.Errorf("tao outbox relay: %w", err)
	}

	// Cac tien trinh nen doc lap: relay tu dial lai khi RabbitMQ sap va khong bao gio
	// tra ve vi broker, nen khong keo sweeper dung theo (event tich lai trong outbox).
	// Khi ctx bi huy (SIGTERM) tat ca deu phai dung.
	return runLoops(ctx,
		sweeper.Run,
		seeder.Run,
		relay.Run,
		func(ctx context.Context) error {
			return otelx.ServeMetrics(ctx, cfg.MetricsAddr, log)
		},
	)
}

// config la cau hinh doc tu bien moi truong.
type config struct {
	DatabaseURL string
	RedisAddr   string
	MetricsAddr string
	EventIDs    []string
	AMQPURL     string
}

// loadConfig doc cau hinh va fail fast khi AMQP_URL thieu hoac sai cu phap.
// Loi KHONG chua gia tri URL (co the mang mat khau).
func loadConfig(getenv func(string) string) (config, error) {
	def := func(k, d string) string {
		if v := getenv(k); v != "" {
			return v
		}
		return d
	}
	cfg := config{
		DatabaseURL: getenv("DATABASE_URL"),
		RedisAddr:   def("REDIS_ADDR", "localhost:6379"),
		MetricsAddr: def("METRICS_ADDR", ":9092"),
		EventIDs:    splitCSV(getenv("ACTIVE_EVENT_IDS")),
		AMQPURL:     getenv("AMQP_URL"),
	}
	if cfg.AMQPURL == "" {
		return config{}, errors.New("thieu bien moi truong AMQP_URL")
	}
	if _, err := amqp.ParseURI(cfg.AMQPURL); err != nil {
		return config{}, errors.New("AMQP_URL khong hop le (can dang amqp://... hoac amqps://...)")
	}
	return cfg, nil
}

// newPoolConfig parse DATABASE_URL. QueryExecModeCacheDescribe: pgx cache ket qua describe thay vi tao
// prepared statement ten, nen an toan voi PgBouncer o che do transaction bat ke max_prepared_statements.
func newPoolConfig(dsn string) (*pgxpool.Config, error) {
	pc, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, errors.New("DATABASE_URL khong hop le")
	}
	pc.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeCacheDescribe
	return pc, nil
}

// relayConfig la cau hinh relay cua ticketing. ticketing.hold.created chua co consumer (khong co binding)
// nen unroutable duoc bo; ticketing.hold.released thi KHONG, vi bo se mat event.
func relayConfig(reg prometheus.Registerer) outbox.RelayConfig {
	return outbox.RelayConfig{
		Exchange:       "ticketing.events",
		DropUnroutable: []string{"ticketing.hold.created"},
		Registerer:     reg,
	}
}

// amqpDialer tra outbox.Dialer that: dial, khai bao topology tren mot kenh, roi mo kenh rieng cho publish
// (doc.go cua mq). Khong dial luc tao. Loi tra ve khong chua URL.
func amqpDialer(url string) outbox.Dialer {
	return func(ctx context.Context) (outbox.Publisher, io.Closer, error) {
		if err := ctx.Err(); err != nil {
			return nil, nil, err
		}
		conn, err := amqp.DialConfig(url, amqp.Config{
			Dial:      amqp.DefaultDial(10 * time.Second),
			Heartbeat: 10 * time.Second,
		})
		if err != nil {
			return nil, nil, redactErr("dial rabbitmq", err, url)
		}
		topo, err := conn.Channel()
		if err != nil {
			_ = conn.Close()
			return nil, nil, redactErr("mo kenh topology", err, url)
		}
		if err := mq.Declare(topo, mq.TicketingTopology()); err != nil {
			_ = conn.Close()
			return nil, nil, redactErr("khai bao topology", err, url)
		}
		_ = topo.Close()
		pch, err := conn.Channel()
		if err != nil {
			_ = conn.Close()
			return nil, nil, redactErr("mo kenh publish", err, url)
		}
		pub, err := mq.NewPublisher(pch)
		if err != nil {
			_ = conn.Close()
			return nil, nil, redactErr("tao publisher", err, url)
		}
		return pub, conn, nil
	}
}

// redactErr dung loi moi khong chua URL (mat khau) cua broker.
func redactErr(what string, err error, url string) error {
	msg := strings.ReplaceAll(err.Error(), url, "[amqp-url]")
	if u, perr := amqp.ParseURI(url); perr == nil && u.Password != "" {
		msg = strings.ReplaceAll(msg, u.Password, "[redacted]")
	}
	return fmt.Errorf("%s: %s", what, msg)
}

// runLoops chay cac vong lap song song. Vong lap dau tien tra ve (loi hoac nil) lam huy ctx chung;
// ham cho tat ca dung roi tra ket qua cua vong lap do.
func runLoops(ctx context.Context, loops ...func(context.Context) error) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	errs := make(chan error, len(loops))
	for _, l := range loops {
		go func() { errs <- l(ctx) }()
	}
	first := <-errs
	cancel()
	for i := 1; i < len(loops); i++ {
		<-errs
	}
	return first
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
