// Command server phuc vu API phong cho.
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

	"github.com/redis/go-redis/v9"

	"github.com/eventflow/eventflow/libs/go/httpx"
	httpadapter "github.com/eventflow/eventflow/services/waitingroom/internal/adapter/http"
	redisadapter "github.com/eventflow/eventflow/services/waitingroom/internal/adapter/redis"
	"github.com/eventflow/eventflow/services/waitingroom/internal/domain"
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

	rdb := redis.NewClient(&redis.Options{
		Addr: env("REDIS_ADDR", "localhost:6379"),
		// Pool phai du rong cho dinh tai: moi request la mot lenh Lua ngan, nen
		// nut co chai thuong la so ket noi chu khong phai bang thong.
		PoolSize:     512,
		MinIdleConns: 64,
		ReadTimeout:  200 * time.Millisecond,
		WriteTimeout: 200 * time.Millisecond,
	})
	defer func() {
		if err := rdb.Close(); err != nil {
			log.Warn("dong redis client loi", "err", err)
		}
	}()

	if err := rdb.Ping(ctx).Err(); err != nil {
		return err
	}

	queue, err := redisadapter.NewQueue(ctx, rdb, 24*time.Hour)
	if err != nil {
		return err
	}

	h := httpadapter.New(store{queue}, nil, log)

	mux := http.NewServeMux()
	h.Routes(mux)
	// Readiness tach rieng khoi liveness: pod mat Redis thi phai bi rut khoi load
	// balancer, nhung KHONG nen bi giet va khoi dong lai giua dot mo ban.
	httpx.Health(mux, func(ctx context.Context) error { return rdb.Ping(ctx).Err() })

	// Khong dat WriteTimeout: /queue/stream la SSE, ket noi song rat lau.
	srv := httpx.NewServer(env("HTTP_ADDR", ":8081"),
		httpx.Chain(mux, httpx.RequestID(), httpx.AccessLog(log), httpx.Recover(log)))

	log.Info("waitingroom dang lang nghe", "addr", srv.Addr)
	return httpx.Run(ctx, srv, 20*time.Second)
}

// store noi kieu tra ve cua adapter Redis voi interface ma handler mong doi.
type store struct{ q *redisadapter.Queue }

func (s store) Join(ctx context.Context, eventID, identityID, token string) (httpadapter.JoinResult, error) {
	r, err := s.q.Join(ctx, eventID, identityID, token)
	if err != nil {
		return httpadapter.JoinResult{}, err
	}
	return httpadapter.JoinResult{State: r.State, Token: r.Token, Rank: r.Rank, IsNew: r.IsNew}, nil
}

func (s store) Status(ctx context.Context, eventID, token string) (domain.Position, error) {
	return s.q.Status(ctx, eventID, token)
}

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
