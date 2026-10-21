// Package worker chua tien trinh nen cua payment-svc.
package worker

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/eventflow/eventflow/libs/go/mq"
	"github.com/eventflow/eventflow/libs/go/otelx"
)

// Exchange nhan su kien cua payment (libs/go/mq.PaymentTopology).
const Exchange = "payment.events"

// PaymentRoutingKeys la cac routing key payment ghi vao outbox_events. Relay CHI lay cac dong nay:
// bang outbox_events dung chung moi service, va payment dung chung aggregate_type 'order' voi
// ticketing, nen loc theo aggregate_type la khong du. Them routing key moi o repository thi
// them vao day, neu khong dong do nam mai trong outbox.
var PaymentRoutingKeys = []string{"order.paid", "hold.released", "payment.refunded"}

// OutboxRelayConfig cau hinh relay.
type OutboxRelayConfig struct {
	Interval    time.Duration
	Batch       int
	RoutingKeys []string
}

// DefaultOutboxRelayConfig: quet moi giay, lo 100 dong, chi cac key cua payment.
func DefaultOutboxRelayConfig() OutboxRelayConfig {
	return OutboxRelayConfig{Interval: time.Second, Batch: 100, RoutingKeys: PaymentRoutingKeys}
}

// EventPublisher la phan cua *mq.Publisher ma relay can.
type EventPublisher interface {
	Publish(ctx context.Context, exchange, key string, msg mq.Message) error
}

// OutboxRelay phat cac dong outbox_events cua payment sang RabbitMQ, at-least-once:
//   - published_at chi duoc dat sau khi broker ack (Publisher cho confirm);
//   - FOR UPDATE SKIP LOCKED cho phep nhieu instance chay song song;
//   - dung o dong loi dau tien trong lo de giu thu tu theo id;
//   - MessageID = id dong outbox de consumer khu trung lap.
type OutboxRelay struct {
	pool *pgxpool.Pool
	pub  EventPublisher
	cfg  OutboxRelayConfig
	log  *slog.Logger
}

// NewOutboxRelay tao relay. cfg.RoutingKeys rong thi dung PaymentRoutingKeys (khong bao gio la "moi dong").
func NewOutboxRelay(pool *pgxpool.Pool, pub EventPublisher, cfg OutboxRelayConfig, log *slog.Logger) *OutboxRelay {
	d := DefaultOutboxRelayConfig()
	if cfg.Interval <= 0 {
		cfg.Interval = d.Interval
	}
	if cfg.Batch <= 0 {
		cfg.Batch = d.Batch
	}
	if len(cfg.RoutingKeys) == 0 {
		cfg.RoutingKeys = d.RoutingKeys
	}
	return &OutboxRelay{pool: pool, pub: pub, cfg: cfg, log: log}
}

// Run quet theo chu ky toi khi ctx bi huy.
func (r *OutboxRelay) Run(ctx context.Context) error {
	t := time.NewTicker(r.cfg.Interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.C:
			if _, err := r.RelayOnce(ctx); err != nil && ctx.Err() == nil {
				r.log.Error("vong relay outbox payment that bai", "err", err)
			}
		}
	}
}

type outboxItem struct {
	id         int64
	routingKey string
	payload    []byte
	traceID    *string
}

// RelayOnce chay mot lo va tra so dong da phat.
func (r *OutboxRelay) RelayOnce(ctx context.Context) (int, error) {
	if r.pool == nil {
		return 0, errors.New("outbox relay payment: pool nil")
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()

	const q = `
		SELECT id, routing_key, payload, trace_id
		  FROM outbox_events
		 WHERE published_at IS NULL
		   AND routing_key = ANY($2)
		 ORDER BY id
		 LIMIT $1
		 FOR UPDATE SKIP LOCKED`
	rows, err := tx.Query(ctx, q, r.cfg.Batch, r.cfg.RoutingKeys)
	if err != nil {
		return 0, err
	}
	var items []outboxItem
	for rows.Next() {
		var it outboxItem
		if err := rows.Scan(&it.id, &it.routingKey, &it.payload, &it.traceID); err != nil {
			rows.Close()
			return 0, err
		}
		items = append(items, it)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}

	relayed := 0
	for _, it := range items {
		msg := mq.Message{
			Body:        it.payload,
			ContentType: "application/json",
			MessageID:   strconv.FormatInt(it.id, 10),
		}
		if it.traceID != nil && *it.traceID != "" {
			msg.CorrelationID = *it.traceID
		}
		// Noi tiep trace cua request da ghi dong outbox nay (EV-103).
		var tp string
		if it.traceID != nil {
			tp = *it.traceID
		}
		pubCtx, cancel := context.WithTimeout(otelx.ContextWithTraceparent(ctx, tp), 5*time.Second)
		err := r.pub.Publish(pubCtx, Exchange, it.routingKey, msg)
		cancel()
		if err != nil {
			// Giu thu tu: dong nay va cac dong sau thu lai o chu ky sau; nhung dong da phat van commit.
			r.log.Warn("phat outbox payment that bai, thu lai o chu ky sau",
				"id", it.id, "routing_key", it.routingKey, "err", err)
			break
		}
		if _, err := tx.Exec(ctx, `UPDATE outbox_events SET published_at = now() WHERE id = $1`, it.id); err != nil {
			return 0, fmt.Errorf("danh dau outbox %d: %w", it.id, err)
		}
		relayed++
	}
	if relayed == 0 {
		return 0, nil
	}
	commitCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	if err := tx.Commit(commitCtx); err != nil {
		return 0, err
	}
	r.log.Info("da relay outbox payment", "so_luong", relayed)
	return relayed, nil
}
