package worker

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/eventflow/eventflow/libs/go/mq"
	"github.com/eventflow/eventflow/libs/go/otelx"
)

// TicketingRoutingKeyPrefix: relay ticketing chi phat dong co routing key thuoc domain cua
// minh (quy uoc <domain>.<entity>.<action>, docs/03 §7).
//
// Bang outbox_events dung chung moi service (mot database). Dong cua service khac khong co
// binding tren ticketing.events, broker tra ve, va vi relay dung o dong loi dau tien de giu
// thu tu, MOI chu ky sau se ket dung cho do -- toan bo outbox ticketing ngung chay. Loc theo
// aggregate_type la KHONG DU: payment (EV-151) ghi 'order.paid' voi cung aggregate_type 'order'.
const TicketingRoutingKeyPrefix = "ticketing."

// OutboxRelayConfig cấu hình cho tiến trình outbox relay.
type OutboxRelayConfig struct {
	Interval time.Duration // Mặc định 1 giây
	Batch    int           // Kích thước lô (mặc định 100)
	// RoutingKeyPrefix: chi relay dong co routing_key bat dau bang tien to nay. Rong ->
	// TicketingRoutingKeyPrefix (khong bao gio la "moi dong").
	RoutingKeyPrefix string
}

func DefaultOutboxRelayConfig() OutboxRelayConfig {
	return OutboxRelayConfig{
		Interval:         time.Second,
		Batch:            100,
		RoutingKeyPrefix: TicketingRoutingKeyPrefix,
	}
}

// OutboxMetrics nhận số liệu của tiến trình outbox relay.
type OutboxMetrics interface {
	EventsRelayed(n int)
}

type noopOutboxMetrics struct{}

func (noopOutboxMetrics) EventsRelayed(int) {}

// EventPublisher là giao diện để phát message vào message broker (RabbitMQ).
// *mq.Publisher thoả mãn giao diện này.
type EventPublisher interface {
	Publish(ctx context.Context, exchange, key string, msg mq.Message) error
}

// OutboxRelay quét bảng outbox_events và phát các sự kiện chưa gửi sang RabbitMQ (EV-131 / EVF-36).
//
// Đảm bảo at-least-once:
//  1. Chỉ cập nhật published_at sau khi RabbitMQ broker confirm ack.
//  2. Sử dụng SELECT ... FOR UPDATE SKIP LOCKED để nhiều instance relay chạy song song
//     an toàn, không tranh chấp và không xung đột khoá dòng.
//  3. Mỗi message mang MessageID = ID của outbox event để phía consumer khử trùng lặp (idempotent).
type OutboxRelay struct {
	pool     *pgxpool.Pool
	pub      EventPublisher
	exchange string
	cfg      OutboxRelayConfig
	log      *slog.Logger
	metrics  OutboxMetrics
}

func NewOutboxRelay(
	pool *pgxpool.Pool,
	pub EventPublisher,
	exchange string,
	cfg OutboxRelayConfig,
	log *slog.Logger,
) *OutboxRelay {
	if cfg.RoutingKeyPrefix == "" {
		cfg.RoutingKeyPrefix = TicketingRoutingKeyPrefix
	}
	return &OutboxRelay{
		pool:     pool,
		pub:      pub,
		exchange: exchange,
		cfg:      cfg,
		log:      log,
		metrics:  noopOutboxMetrics{},
	}
}

func (r *OutboxRelay) WithMetrics(m OutboxMetrics) *OutboxRelay {
	r.metrics = m
	return r
}

func (r *OutboxRelay) Run(ctx context.Context) error {
	t := time.NewTicker(r.cfg.Interval)
	defer t.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.C:
			if err := r.relay(ctx); err != nil {
				r.log.Error("vong relay outbox that bai", "err", err)
			}
		}
	}
}

type outboxItem struct {
	id          int64
	aggregateID string
	routingKey  string
	payload     []byte
	traceID     *string
}

func (r *OutboxRelay) relay(ctx context.Context) error {
	if r.pool == nil {
		return errors.New("outbox relay: pool is nil")
	}

	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	const q = `
		SELECT id, aggregate_id::text, routing_key, payload, trace_id
		  FROM outbox_events
		 WHERE published_at IS NULL
		   AND starts_with(routing_key, $2)
		 ORDER BY id
		 LIMIT $1
		 FOR UPDATE SKIP LOCKED`

	rows, err := tx.Query(ctx, q, r.cfg.Batch, r.cfg.RoutingKeyPrefix)
	if err != nil {
		return err
	}
	defer rows.Close()

	var items []outboxItem
	for rows.Next() {
		var item outboxItem
		if err := rows.Scan(&item.id, &item.aggregateID, &item.routingKey, &item.payload, &item.traceID); err != nil {
			return err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	rows.Close()

	if len(items) == 0 {
		return nil
	}

	var relayed int
	for _, item := range items {
		msg := mq.Message{
			Body:        item.payload,
			ContentType: "application/json",
			MessageID:   fmt.Sprintf("%d", item.id),
		}
		if item.traceID != nil && *item.traceID != "" {
			msg.CorrelationID = *item.traceID
		}

		// Noi tiep trace cua tien trinh da ghi dong outbox nay (EV-103).
		var tp string
		if item.traceID != nil {
			tp = *item.traceID
		}
		pubCtx, cancel := context.WithTimeout(otelx.ContextWithTraceparent(ctx, tp), 5*time.Second)
		err := r.pub.Publish(pubCtx, r.exchange, item.routingKey, msg)
		cancel()

		if err != nil {
			r.log.Warn("phat outbox event sang broker that bai, se thu lai o chu ky sau",
				"id", item.id, "routing_key", item.routingKey, "err", err)
			// Dung batch de retry o chu ky sau, nhung van commit nhung row da publish thanh cong truoc do
			break
		}

		const markQ = `UPDATE outbox_events SET published_at = now() WHERE id = $1`
		if _, err := tx.Exec(ctx, markQ, item.id); err != nil {
			return err
		}
		relayed++
	}

	if relayed > 0 {
		commitCtx, commitCancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer commitCancel()
		if err := tx.Commit(commitCtx); err != nil {
			return err
		}
		r.metrics.EventsRelayed(relayed)
		r.log.Info("da relay outbox events sang broker", "so_luong", relayed)
	}

	return nil
}
