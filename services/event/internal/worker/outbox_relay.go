// Package worker chua tien trinh nen cua event service.
package worker

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"strconv"
	"time"

	"github.com/eventflow/eventflow/libs/go/mq"
	"github.com/eventflow/eventflow/services/event/internal/app"
)

// Publisher phat message va chi tra nil khi broker xac nhan. *mq.Publisher thoa man.
type Publisher interface {
	Publish(ctx context.Context, exchange, key string, msg mq.Message) error
}

// DefaultRoutes: routing key -> exchange cho moi message event service ghi vao outbox.
// Routing key khong co trong bang la loi lap trinh: relay bo qua dong do (log loi) chu
// khong de no chan cac dong phia sau.
func DefaultRoutes() map[string]string {
	return map[string]string{
		app.ModerationRoutingKey: "ai.direct", // EV-120 -> ai.moderation.q (libs/go/mq.AITopology)
	}
}

// OutboxRelayConfig cau hinh relay.
type OutboxRelayConfig struct {
	Interval time.Duration
	Batch    int
	// AggregateType: bang outbox_events dung chung moi service; relay chi lay dong cua
	// event service. Lay dong cua ticketing la day message sai exchange.
	AggregateType string
}

// DefaultOutboxRelayConfig: quet moi giay, lo 100 dong, chi dong aggregate_type "event".
func DefaultOutboxRelayConfig() OutboxRelayConfig {
	return OutboxRelayConfig{Interval: time.Second, Batch: 100, AggregateType: app.OutboxAggregateEvent}
}

// OutboxRelay phat cac dong outbox_events cua event service ra RabbitMQ (EV-120).
//
// At-least-once, giong relay cua ticketing (EV-131):
//  1. Chi danh dau published_at sau khi broker xac nhan.
//  2. SELECT ... FOR UPDATE SKIP LOCKED: nhieu instance chay song song khong phat trung.
//  3. MessageID = id cua dong outbox de consumer khu trung lap.
type OutboxRelay struct {
	db     *sql.DB
	pub    Publisher
	routes map[string]string
	cfg    OutboxRelayConfig
	log    *slog.Logger
}

// NewOutboxRelay kiem cau hinh luc khoi dong: sai thi loi ngay, khong doi toi vong dau.
func NewOutboxRelay(db *sql.DB, pub Publisher, routes map[string]string, cfg OutboxRelayConfig, log *slog.Logger) (*OutboxRelay, error) {
	if cfg.Interval <= 0 {
		cfg.Interval = time.Second
	}
	if cfg.AggregateType == "" {
		cfg.AggregateType = app.OutboxAggregateEvent
	}
	switch {
	case cfg.Batch <= 0:
		return nil, errors.New("outbox relay: Batch phai > 0")
	case db == nil || pub == nil:
		return nil, errors.New("outbox relay: thieu db hoac publisher")
	case len(routes) == 0:
		return nil, errors.New("outbox relay: bang dinh tuyen rong")
	}
	return &OutboxRelay{db: db, pub: pub, routes: routes, cfg: cfg, log: log}, nil
}

// Run quet theo nhip cho toi khi ctx bi huy.
func (r *OutboxRelay) Run(ctx context.Context) error {
	t := time.NewTicker(r.cfg.Interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.C:
			if err := r.relay(ctx); err != nil && ctx.Err() == nil {
				r.log.Error("vong relay outbox that bai", "err", err)
			}
		}
	}
}

type outboxRow struct {
	id         int64
	routingKey string
	payload    []byte
	traceID    sql.NullString
	createdAt  time.Time
}

func (r *OutboxRelay) relay(ctx context.Context) error {
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	const q = `
		SELECT id, routing_key, payload, trace_id, created_at
		  FROM outbox_events
		 WHERE published_at IS NULL
		   AND aggregate_type = $2
		 ORDER BY id
		 LIMIT $1
		 FOR UPDATE SKIP LOCKED`
	rows, err := tx.QueryContext(ctx, q, r.cfg.Batch, r.cfg.AggregateType)
	if err != nil {
		return err
	}
	var items []outboxRow
	for rows.Next() {
		var it outboxRow
		if err := rows.Scan(&it.id, &it.routingKey, &it.payload, &it.traceID, &it.createdAt); err != nil {
			_ = rows.Close()
			return err
		}
		items = append(items, it)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if err := rows.Err(); err != nil {
		return err
	}

	relayed := 0
	for _, it := range items {
		exchange, ok := r.routes[it.routingKey]
		if !ok {
			r.log.Error("routing key khong co tuyen, bo qua dong outbox", "id", it.id, "routing_key", it.routingKey)
			continue
		}
		msg := mq.Message{
			Body:        it.payload,
			ContentType: "application/json",
			MessageID:   strconv.FormatInt(it.id, 10),
			// ai-worker do ai_queue_lag_seconds tu timestamp nay.
			Timestamp: it.createdAt,
		}
		if it.traceID.Valid && it.traceID.String != "" {
			msg.CorrelationID = it.traceID.String
		}

		pubCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		err := r.pub.Publish(pubCtx, exchange, it.routingKey, msg)
		cancel()
		if err != nil {
			// Dung lo de giu thu tu; dong nay thu lai o chu ky sau. Van commit nhung dong da phat.
			r.log.Warn("phat outbox that bai, thu lai o chu ky sau", "id", it.id, "routing_key", it.routingKey, "err", err)
			break
		}
		if _, err := tx.ExecContext(ctx, `UPDATE outbox_events SET published_at = now() WHERE id = $1`, it.id); err != nil {
			return err
		}
		relayed++
	}

	if relayed == 0 {
		return nil
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	r.log.Info("da relay outbox", "so_luong", relayed)
	return nil
}
