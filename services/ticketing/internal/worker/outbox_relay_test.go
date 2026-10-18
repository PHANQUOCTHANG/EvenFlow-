package worker_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/eventflow/eventflow/libs/go/mq"
	"github.com/eventflow/eventflow/services/ticketing/internal/worker"
)

type fakeEventPublisher struct {
	mu       sync.Mutex
	messages []mq.Message
	failNext bool
	err      error
}

func (p *fakeEventPublisher) Publish(_ context.Context, _, _ string, msg mq.Message) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.failNext {
		p.failNext = false
		if p.err != nil {
			return p.err
		}
		return errors.New("simulated broker error")
	}
	p.messages = append(p.messages, msg)
	return nil
}

func TestDefaultOutboxRelayConfig(t *testing.T) {
	cfg := worker.DefaultOutboxRelayConfig()
	if cfg.Interval != time.Second {
		t.Errorf("expected default interval 1s, got %v", cfg.Interval)
	}
	if cfg.Batch <= 0 {
		t.Errorf("expected positive batch size, got %d", cfg.Batch)
	}
}

func TestOutboxRelay_GracefulShutdown(t *testing.T) {
	pub := &fakeEventPublisher{}
	cfg := worker.OutboxRelayConfig{
		Interval: 10 * time.Millisecond,
		Batch:    10,
	}
	// Pool nil ở đây vì test chỉ kiểm tra vòng lặp select ctx.Done()
	relay := worker.NewOutboxRelay(nil, pub, "ticketing.events", cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- relay.Run(ctx)
	}()

	time.Sleep(10 * time.Millisecond)
	cancel()

	select {
	case err := <-errCh:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected context.Canceled, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("outbox relay did not stop on context cancellation")
	}
}

func TestDefaultOutboxRelayConfig_ChiLayDongCuaTicketing(t *testing.T) {
	cfg := worker.DefaultOutboxRelayConfig()
	if len(cfg.AggregateTypes) != 1 || cfg.AggregateTypes[0] != "order" {
		t.Fatalf("relay ticketing chi duoc lay aggregate_type 'order', got %v", cfg.AggregateTypes)
	}
}
