package main

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/eventflow/eventflow/libs/go/antibot"
	"github.com/eventflow/eventflow/libs/go/antibot/redisstate"
	httpadapter "github.com/eventflow/eventflow/services/waitingroom/internal/adapter/http"
	postgresadapter "github.com/eventflow/eventflow/services/waitingroom/internal/adapter/postgres"
)

// buildGuard wires the anti-bot guard (antibot-phase1). For ANTIBOT_MODE=off it returns an UNTYPED nil
// BotGuard (P-15), so the handler's `bots == nil` check holds and the join is exactly develop's. The returned
// close flushes the bot_decisions writer and closes the dedicated Redis client; it is always callable.
//
// A Redis failure at startup (observe.lua not loadable) does not stop the server: decisions are DEGRADED until
// Redis answers again (D7, the join never depends on antibot).
func buildGuard(ctx context.Context, cfg antibot.Config, redisAddr string, pool *pgxpool.Pool,
	reg prometheus.Registerer, log *slog.Logger) (httpadapter.BotGuard, *antibot.Metrics, func()) {
	noop := func() {}
	if cfg.Mode == antibot.ModeOff {
		log.Info("antibot tat (ANTIBOT_MODE=off): join giu nguyen hanh vi develop")
		return nil, nil, noop
	}
	if cfg.DevKey {
		log.Warn("ANTIBOT_HASH_KEY chua dat, dang dung key mac dinh cho dev")
	}
	m := antibot.NewMetrics(reg)
	rdb := redisstate.NewClient(redisAddr, cfg)
	loadCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	store, err := redisstate.New(loadCtx, rdb, cfg)
	cancel()
	if err != nil {
		// Khong chan khoi dong: Observe tu nap lai script (NOSCRIPT -> EVAL) khi Redis len lai; truoc do moi
		// quyet dinh la DEGRADED (D7).
		log.Error("antibot: chua nap duoc observe.lua", "err", err)
		store = redisstate.NewLazy(rdb, cfg)
	}
	writer := postgresadapter.NewDecisionWriter(pool, m)
	g := antibot.NewGuard(cfg, store, writer, m)
	log.Info("antibot bat", "mode", cfg.Mode)
	return g, m, func() {
		sh, cancel := context.WithTimeout(context.Background(), 2500*time.Millisecond)
		defer cancel()
		if err := writer.Close(sh); err != nil {
			log.Warn("flush bot_decisions chua xong", "err", err)
		}
		if err := rdb.Close(); err != nil {
			log.Warn("dong redis client antibot loi", "err", err)
		}
	}
}
