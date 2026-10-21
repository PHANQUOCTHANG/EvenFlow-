//go:build integration

package worker_test

import (
	"context"
	"io"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/eventflow/eventflow/libs/go/mq"
	"github.com/eventflow/eventflow/libs/go/otelx"
	"github.com/eventflow/eventflow/libs/go/testx"
	"github.com/eventflow/eventflow/services/payment/internal/worker"
)

type spyPublisher struct {
	mu       sync.Mutex
	keys     []string
	ids      []string
	exchange string
	failKey  string // publish key nay thi loi (gia lap broker nack)
}

func (p *spyPublisher) Publish(_ context.Context, exchange, key string, msg mq.Message) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if key == p.failKey {
		return mq.ErrNacked
	}
	p.exchange = exchange
	p.keys = append(p.keys, key)
	p.ids = append(p.ids, msg.MessageID)
	return nil
}

func setup(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool, err := pgxpool.New(context.Background(), testx.Postgres(t))
	if err != nil {
		t.Fatalf("connect postgres: %v", err)
	}
	t.Cleanup(pool.Close)
	if _, err := pool.Exec(context.Background(), "DELETE FROM outbox_events"); err != nil {
		t.Fatalf("don outbox: %v", err)
	}
	return pool
}

func insert(t *testing.T, pool *pgxpool.Pool, key string) int64 {
	t.Helper()
	var id int64
	err := pool.QueryRow(context.Background(), `
		INSERT INTO outbox_events (aggregate_type, aggregate_id, routing_key, payload)
		VALUES ('order', $1::uuid, $2, '{}') RETURNING id`, uuid.New().String(), key).Scan(&id)
	if err != nil {
		t.Fatalf("chen outbox %s: %v", key, err)
	}
	return id
}

func published(t *testing.T, pool *pgxpool.Pool, id int64) bool {
	t.Helper()
	var at *time.Time
	if err := pool.QueryRow(context.Background(), "SELECT published_at FROM outbox_events WHERE id = $1", id).Scan(&at); err != nil {
		t.Fatalf("doc outbox %d: %v", id, err)
	}
	return at != nil
}

func newRelay(pool *pgxpool.Pool, pub worker.EventPublisher) *worker.OutboxRelay {
	return worker.NewOutboxRelay(pool, pub, worker.OutboxRelayConfig{Batch: 10}, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

// Relay payment chi phat dong cua payment, sang payment.events, va KHONG dung vao dong cua
// ticketing (cung aggregate_type 'order') hay event service.
func TestRelay_ChiPhatDongCuaPayment(t *testing.T) {
	pool := setup(t)
	idTicketing := insert(t, pool, "ticketing.hold.created")
	idEvent := insert(t, pool, "moderation")
	idPaid := insert(t, pool, "order.paid")
	idRefund := insert(t, pool, "payment.refunded")

	pub := &spyPublisher{}
	n, err := newRelay(pool, pub).RelayOnce(context.Background())
	if err != nil || n != 2 {
		t.Fatalf("RelayOnce = %d, %v; muon 2, nil", n, err)
	}
	if pub.exchange != worker.Exchange {
		t.Errorf("exchange = %q, muon %q", pub.exchange, worker.Exchange)
	}
	if len(pub.keys) != 2 || pub.keys[0] != "order.paid" || pub.keys[1] != "payment.refunded" {
		t.Errorf("keys = %v, muon [order.paid payment.refunded] dung thu tu id", pub.keys)
	}
	if !published(t, pool, idPaid) || !published(t, pool, idRefund) {
		t.Error("dong cua payment chua duoc danh dau published")
	}
	if published(t, pool, idTicketing) || published(t, pool, idEvent) {
		t.Error("relay payment danh dau dong cua service khac")
	}
}

// Broker tu choi mot dong: dong do va cac dong sau giu nguyen de thu lai (thu tu), dong truoc
// da phat van duoc commit; lan sau broker on thi phat not.
func TestRelay_BrokerLoiGiuThuTuVaThuLai(t *testing.T) {
	pool := setup(t)
	id1 := insert(t, pool, "order.paid")
	id2 := insert(t, pool, "payment.refunded")
	id3 := insert(t, pool, "order.paid")

	pub := &spyPublisher{failKey: "payment.refunded"}
	relay := newRelay(pool, pub)
	if n, err := relay.RelayOnce(context.Background()); err != nil || n != 1 {
		t.Fatalf("lan 1 = %d, %v; muon 1, nil", n, err)
	}
	if !published(t, pool, id1) || published(t, pool, id2) || published(t, pool, id3) {
		t.Fatal("lan 1: chi dong dau duoc danh dau")
	}

	pub.failKey = ""
	if n, err := relay.RelayOnce(context.Background()); err != nil || n != 2 {
		t.Fatalf("lan 2 = %d, %v; muon 2, nil", n, err)
	}
	if !published(t, pool, id2) || !published(t, pool, id3) {
		t.Fatal("lan 2: hai dong con lai phai duoc phat")
	}
	if got := pub.ids[1:]; got[0] != strconv.FormatInt(id2, 10) || got[1] != strconv.FormatInt(id3, 10) {
		t.Errorf("MessageID = %v, muon id outbox theo thu tu", got)
	}
}

// EV-103: dong outbox mang traceparent -> relay publish duoi CUNG trace (consumer ben kia noi tiep trace
// cua request webhook); ma request cu (khong phai traceparent) van phat binh thuong, khong trace.
func TestRelay_NoiTiepTraceTuOutbox(t *testing.T) {
	shutdown, err := otelx.Init(context.Background(), "relay-test")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = shutdown(context.Background()) })
	pool := setup(t)

	const traceID = "4bf92f3577b34da6a3ce929d0e0e4736"
	withTrace := insert(t, pool, "order.paid")
	legacy := insert(t, pool, "order.paid")
	if _, err := pool.Exec(context.Background(), `UPDATE outbox_events SET trace_id = $2 WHERE id = $1`,
		withTrace, "00-"+traceID+"-00f067aa0ba902b7-01"); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(context.Background(), `UPDATE outbox_events SET trace_id = '8027108cd73783fd' WHERE id = $1`, legacy); err != nil {
		t.Fatal(err)
	}

	pub := &traceSpy{}
	if n, err := newRelay(pool, pub).RelayOnce(context.Background()); err != nil || n != 2 {
		t.Fatalf("RelayOnce = %d, %v", n, err)
	}
	if len(pub.traces) != 2 || pub.traces[0] != traceID {
		t.Fatalf("trace khi publish = %v, muon [%s \"\"]", pub.traces, traceID)
	}
	if pub.traces[1] != "" {
		t.Errorf("dong cu (ma request) khong duoc tao trace, nhan %q", pub.traces[1])
	}
}

type traceSpy struct{ traces []string }

func (s *traceSpy) Publish(ctx context.Context, _, _ string, _ mq.Message) error {
	id := ""
	if parts := strings.Split(otelx.Traceparent(ctx), "-"); len(parts) == 4 {
		id = parts[1]
	}
	s.traces = append(s.traces, id)
	return nil
}
