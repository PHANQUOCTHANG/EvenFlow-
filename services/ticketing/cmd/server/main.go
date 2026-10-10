// Command server phuc vu API ticketing.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/eventflow/eventflow/libs/go/httpx"
	httpadapter "github.com/eventflow/eventflow/services/ticketing/internal/adapter/http"
	postgresadapter "github.com/eventflow/eventflow/services/ticketing/internal/adapter/postgres"
	redisadapter "github.com/eventflow/eventflow/services/ticketing/internal/adapter/redis"
	"github.com/eventflow/eventflow/services/ticketing/internal/app"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	if err := run(log); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Error("server dung bat thuong", "err", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	cfg, err := pgxpool.ParseConfig(env("DATABASE_URL", ""))
	if err != nil {
		return err
	}
	// Pool nho la CO Y. Duong di toi day da bi phong cho ghim luu luong, nen
	// nut co chai that su la Postgres. Mo qua nhieu ket noi chi lam tang
	// contention va lam p99 xau di, khong lam tang thong luong.
	cfg.MaxConns = 25
	cfg.MinConns = 5
	cfg.MaxConnLifetime = 30 * time.Minute

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return err
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		return err
	}

	rdb := redis.NewClient(&redis.Options{
		Addr:         env("REDIS_ADDR", "localhost:6379"),
		PoolSize:     256,
		MinIdleConns: 32,
		ReadTimeout:  200 * time.Millisecond,
		WriteTimeout: 200 * time.Millisecond,
	})
	defer func() {
		if err := rdb.Close(); err != nil {
			log.Warn("dong redis client loi", "err", err)
		}
	}()

	gate, err := redisadapter.NewGate(ctx, rdb)
	if err != nil {
		return err
	}
	repo := postgresadapter.NewRepository(pool)

	h := httpadapter.New(app.NewCreateHold(gate, repo, log), log)

	mux := http.NewServeMux()
	h.Routes(mux)
	httpx.Health(mux,
		pool.Ping,
		func(ctx context.Context) error { return rdb.Ping(ctx).Err() },
	)

	srv := httpx.NewServer(env("HTTP_ADDR", ":8082"),
		httpx.Chain(mux, httpx.RequestID(), httpx.AccessLog(log), httpx.Recover(log)))

	log.Info("ticketing dang lang nghe", "addr", srv.Addr)
	// 20 giay de cac transaction dang duoc commit xong. Cat ngang giua mot
	// transaction tao hold la cach nhanh nhat de tao ra ve bi khoa.
	return httpx.Run(ctx, srv, 20*time.Second)
}

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
