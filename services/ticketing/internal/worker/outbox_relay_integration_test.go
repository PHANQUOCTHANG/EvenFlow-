//go:build integration

package worker_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/eventflow/eventflow/libs/go/mq"
	"github.com/eventflow/eventflow/libs/go/testx"
	"github.com/eventflow/eventflow/services/ticketing/internal/worker"
)

type spyOutboxPublisher struct {
	mu       sync.Mutex
	messages []mq.Message
	failNext bool
}

func (p *spyOutboxPublisher) Publish(_ context.Context, _, _ string, msg mq.Message) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.failNext {
		p.failNext = false
		return mq.ErrNacked
	}
	p.messages = append(p.messages, msg)
	return nil
}

func (p *spyOutboxPublisher) count() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.messages)
}

func setupOutboxIntegrationTest(t *testing.T) *pgxpool.Pool {
	t.Helper()
	ctx := context.Background()
	dsn := testx.Postgres(t)

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect postgres: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func TestOutboxRelay_Integration_RelayAndMarkPublished(t *testing.T) {
	pool := setupOutboxIntegrationTest(t)
	ctx := context.Background()

	// Seed 3 outbox events unpublished, 1 đã published
	const insQ = `
		INSERT INTO outbox_events (aggregate_type, aggregate_id, routing_key, payload, published_at)
		VALUES ($1, $2::uuid, $3, $4, $5)
		RETURNING id`

	payload1, _ := json.Marshal(map[string]any{"order_id": "ord-1", "action": "create"})
	payload2, _ := json.Marshal(map[string]any{"order_id": "ord-2", "action": "create"})
	payload3, _ := json.Marshal(map[string]any{"order_id": "ord-3", "action": "create"})
	payloadOld, _ := json.Marshal(map[string]any{"order_id": "ord-old", "action": "create"})

	var id1, id2, id3, idOld int64
	_ = pool.QueryRow(ctx, insQ, "order", uuid.New().String(), "ticketing.hold.created", payload1, nil).Scan(&id1)
	_ = pool.QueryRow(ctx, insQ, "order", uuid.New().String(), "ticketing.hold.created", payload2, nil).Scan(&id2)
	_ = pool.QueryRow(ctx, insQ, "order", uuid.New().String(), "ticketing.hold.created", payload3, nil).Scan(&id3)
	_ = pool.QueryRow(ctx, insQ, "order", uuid.New().String(), "ticketing.hold.created", payloadOld, time.Now()).Scan(&idOld)

	pub := &spyOutboxPublisher{}
	cfg := worker.OutboxRelayConfig{
		Interval: 20 * time.Millisecond,
		Batch:    100,
	}
	relay := worker.NewOutboxRelay(pool, pub, "ticketing.events", cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))

	ctxRelay, cancelRelay := context.WithCancel(context.Background())
	defer cancelRelay()
	go func() { _ = relay.Run(ctxRelay) }()

	deadline := time.Now().Add(2 * time.Second)
	for pub.count() < 3 {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for 3 events to be relayed, got %d", pub.count())
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancelRelay()

	// Kiểm tra publisher đã nhận đúng 3 messages mới (không nhận event đã published từ trước)
	pub.mu.Lock()
	if len(pub.messages) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(pub.messages))
	}
	if pub.messages[0].MessageID != "" && pub.messages[0].ContentType != "application/json" {
		t.Errorf("invalid message metadata: %+v", pub.messages[0])
	}
	pub.mu.Unlock()

	// Kiểm tra trong DB: 3 events này đã có published_at IS NOT NULL
	var pubCount int
	_ = pool.QueryRow(ctx,
		"SELECT count(*) FROM outbox_events WHERE id IN ($1, $2, $3) AND published_at IS NOT NULL",
		id1, id2, id3).Scan(&pubCount)
	if pubCount != 3 {
		t.Fatalf("expected 3 events marked published, got %d", pubCount)
	}
}

func TestOutboxRelay_Integration_BrokerFailure_AtLeastOnce(t *testing.T) {
	pool := setupOutboxIntegrationTest(t)
	ctx := context.Background()

	const insQ = `
		INSERT INTO outbox_events (aggregate_type, aggregate_id, routing_key, payload)
		VALUES ('order', $1::uuid, 'ticketing.hold.created', '{"test":"retry"}')
		RETURNING id`

	var id int64
	_ = pool.QueryRow(ctx, insQ, uuid.New().String()).Scan(&id)

	pub := &spyOutboxPublisher{failNext: true} // Lần 1 broker fail
	cfg := worker.OutboxRelayConfig{
		Interval: 20 * time.Millisecond,
		Batch:    10,
	}
	relay := worker.NewOutboxRelay(pool, pub, "ticketing.events", cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))

	ctxRelay, cancelRelay := context.WithCancel(context.Background())
	defer cancelRelay()
	go func() { _ = relay.Run(ctxRelay) }()

	// Cho chạy qua vòng 1 (bị fail) và vòng 2 (thành công)
	deadline := time.Now().Add(2 * time.Second)
	for pub.count() < 1 {
		if time.Now().After(deadline) {
			t.Fatalf("event was not retried and published successfully within 2s")
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancelRelay()

	// Event cuối cùng phải được đánh dấu published_at
	var publishedAt *time.Time
	_ = pool.QueryRow(ctx, "SELECT published_at FROM outbox_events WHERE id = $1", id).Scan(&publishedAt)
	if publishedAt == nil {
		t.Fatal("event should be marked published after retry")
	}
}
