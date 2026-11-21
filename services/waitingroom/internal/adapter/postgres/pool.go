package postgresadapter

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	decisionMaxConns       = 2
	decisionConnectTimeout = 2 * time.Second
)

// NewPoolFromEnv builds the small, lazy pool of the bot_decisions writer from DATABASE_URL (contracts 5, P-14):
// its own 2 connections (the audit pool of NewAuditorFromEnv is untouched) and the simple query protocol, so it
// works behind PgBouncer in transaction mode. DATABASE_URL missing or invalid -> nil pool (the writer then
// drops and counts every row; the join path never depends on Postgres). close is always callable.
func NewPoolFromEnv(ctx context.Context, getenv func(string) string, log *slog.Logger) (*pgxpool.Pool, func()) {
	noop := func() {}
	dsn := strings.TrimSpace(getenv("DATABASE_URL"))
	if dsn == "" {
		log.Warn("DATABASE_URL chua dat: bot_decisions khong duoc ghi (anti-bot van chay)")
		return nil, noop
	}
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		// Khong log dsn: co the chua mat khau.
		log.Error("DATABASE_URL khong hop le: bot_decisions khong duoc ghi", "err", err)
		return nil, noop
	}
	cfg.MaxConns = decisionMaxConns
	cfg.MinConns = 0
	cfg.ConnConfig.ConnectTimeout = decisionConnectTimeout
	cfg.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		log.Error("khong tao duoc pool Postgres cho bot_decisions", "err", err)
		return nil, noop
	}
	return pool, pool.Close
}
