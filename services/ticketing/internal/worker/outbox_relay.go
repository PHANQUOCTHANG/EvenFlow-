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
)

// TicketingAggregateTypes la cac aggregate_type ma ticketing ghi vao outbox_events.
//
// Bang outbox_events dung chung moi service (mot database). Relay chi duoc lay dong cua
// chinh minh: dong cua service khac (vd event -> "moderation") khong co binding tren
// ticketing.events, broker tra ve, va vi relay dung o dong loi dau tien de giu thu tu,
// MOI chu ky sau se ket dung cho do -- toan bo outbox ticketing ngung chay.
var TicketingAggregateTypes = []string{"order"}

// OutboxRelayConfig cấu hình cho tiến trình outbox relay.
type OutboxRelayConfig struct {
	Interval time.Duration // Mặc định 1 giây
	Batch    int           // Kích thước lô (mặc định 100)
	// AggregateTypes: chi relay dong co aggregate_type trong danh sach nay. Rong ->
	// TicketingAggregateTypes (khong bao gio la "moi dong").
	AggregateTypes []string
}

func DefaultOutboxRelayConfig() OutboxRelayConfig {
	return OutboxRelayConfig{
		Interval:       time.Second,
		Batch:          100,
		AggregateTypes: TicketingAggregateTypes,
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
	if len(cfg.AggregateTypes) == 0 {
		cfg.AggregateTypes = TicketingAggregateTypes
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
		   AND aggregate_type = ANY($2)
		 ORDER BY id
		 LIMIT $1
		 FOR UPDATE SKIP LOCKED`

	rows, err := tx.Query(ctx, q, r.cfg.Batch, r.cfg.AggregateTypes)
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

		pubCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
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
