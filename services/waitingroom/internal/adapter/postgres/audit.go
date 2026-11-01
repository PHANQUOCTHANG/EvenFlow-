// Package postgresadapter ghi audit_logs cho Ops API cua phong cho (EVF-60).
//
// Phong cho chi dung Postgres cho audit: duong mua ve va readiness KHONG phu thuoc vao no.
package postgresadapter

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/eventflow/eventflow/services/waitingroom/internal/app"
)

// Auditor ghi mot dong audit_logs cho moi lenh ghi de da ap dung. Hien thuc app.Auditor.
type Auditor struct {
	pool *pgxpool.Pool
}

func NewAuditor(pool *pgxpool.Pool) *Auditor { return &Auditor{pool: pool} }

func (a *Auditor) Record(ctx context.Context, e app.AuditEntry) error {
	var meta []byte
	if e.Metadata != nil {
		b, err := json.Marshal(e.Metadata)
		if err != nil {
			return fmt.Errorf("ma hoa metadata audit: %w", err)
		}
		meta = b
	}
	_, err := a.pool.Exec(ctx,
		`INSERT INTO audit_logs (actor_type, actor_id, action, subject, reason, metadata)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		e.ActorType, e.ActorID, e.Action, e.Subject, e.Reason, meta)
	if err != nil {
		return fmt.Errorf("ghi audit_logs: %w", err)
	}
	return nil
}

const (
	auditMaxConns       = 2
	auditConnectTimeout = 2 * time.Second
	auditPingTimeout    = 2 * time.Second
)

// NewAuditorFromEnv dung Auditor tu DATABASE_URL (AC-12, CR-1):
//
//   - DATABASE_URL thieu hoac rong -> auditor nil (WARN): lenh ghi de fail-closed 503 audit-unavailable.
//     Chuoi rong duoc coi nhu thieu vi pgxpool.ParseConfig("") van thanh cong voi mac dinh libpq.
//   - Khong parse duoc -> auditor nil (ERROR): cau hinh sai that.
//   - Parse duoc -> pool ket noi luoi (MinConns 0, khong quay so khi tao). Ping luc khoi dong that bai chi
//     WARN va VAN giu auditor: request sau di nhanh D2 (audit:"failed") va tu hoi phuc khi DB len lai.
//
// Ket qua tra ve la interface nil THUAN (khong phai *Auditor nil boc trong interface), de kiem tra
// `auditor == nil` trong app.OpsOverride dung. close luon goi duoc.
func NewAuditorFromEnv(ctx context.Context, getenv func(string) string, log *slog.Logger) (app.Auditor, func()) {
	noop := func() {}
	dsn := strings.TrimSpace(getenv("DATABASE_URL"))
	if dsn == "" {
		log.Warn("DATABASE_URL chua dat: Ops khong the ghi de admit rate (khong co kho audit)")
		return nil, noop
	}
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		// Khong log dsn: co the chua mat khau.
		log.Error("DATABASE_URL khong hop le: Ops khong the ghi de admit rate", "err", err)
		return nil, noop
	}
	cfg.MaxConns = auditMaxConns
	cfg.MinConns = 0
	cfg.ConnConfig.ConnectTimeout = auditConnectTimeout
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		log.Error("khong tao duoc pool Postgres cho audit", "err", err)
		return nil, noop
	}
	pctx, cancel := context.WithTimeout(ctx, auditPingTimeout)
	defer cancel()
	if err := pool.Ping(pctx); err != nil {
		log.Warn("chua ket noi duoc Postgres cho audit; ghi de van ap dung, audit se bao failed toi khi DB len lai",
			"err", err)
	}
	return NewAuditor(pool), pool.Close
}
