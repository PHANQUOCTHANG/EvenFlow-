package store

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/eventflow/eventflow/services/notification/internal/domain"
)

func TestMemoryStore_BasicFlow(t *testing.T) {
	ctx := context.Background()
	s := NewMemoryNotificationStore()

	notif := domain.Notification{
		ID:        "notif-1",
		DedupKey:  "order.paid:ord-1:email",
		EventType: "order.paid",
		OrderID:   "ord-1",
		Recipient: "user@example.com",
		Channel:   domain.ChannelEmail,
	}

	// 1. First check -> should be new
	already, err := s.CheckOrSetDedup(ctx, notif.DedupKey, notif)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if already {
		t.Errorf("expected already = false for first insertion")
	}

	// 2. Immediate second check -> should be detected as pending/already processing
	already2, err := s.CheckOrSetDedup(ctx, notif.DedupKey, notif)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !already2 {
		t.Errorf("expected already = true for pending notification")
	}

	// 3. Mark sent
	sentAt := time.Now()
	if err := s.MarkSent(ctx, notif.DedupKey, notif.ID, sentAt); err != nil {
		t.Fatalf("unexpected error marking sent: %v", err)
	}

	// 4. Check again -> should be detected as already sent
	already3, err := s.CheckOrSetDedup(ctx, notif.DedupKey, notif)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !already3 {
		t.Errorf("expected already = true for sent notification")
	}

	// 5. Get history by order ID
	history, err := s.GetHistoryByOrderID(ctx, "ord-1")
	if err != nil {
		t.Fatalf("unexpected error getting history: %v", err)
	}
	if len(history) != 1 {
		t.Fatalf("expected 1 record in history, got %d", len(history))
	}
	if history[0].Status != domain.StatusSent {
		t.Errorf("expected status SENT, got %s", history[0].Status)
	}
	if history[0].SentAt == nil {
		t.Errorf("expected non-nil SentAt")
	}
}

func TestMemoryStore_MarkFailed(t *testing.T) {
	ctx := context.Background()
	s := NewMemoryNotificationStore()

	notif := domain.Notification{
		ID:        "notif-fail",
		DedupKey:  "order.paid:ord-fail:email",
		EventType: "order.paid",
		OrderID:   "ord-fail",
		Recipient: "user@example.com",
		Channel:   domain.ChannelEmail,
	}

	_, _ = s.CheckOrSetDedup(ctx, notif.DedupKey, notif)

	if err := s.MarkFailed(ctx, notif.DedupKey, notif.ID, "smtp timeout"); err != nil {
		t.Fatalf("unexpected error marking failed: %v", err)
	}

	history, err := s.GetHistoryByOrderID(ctx, "ord-fail")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(history) != 1 || history[0].Status != domain.StatusFailed || history[0].ErrorReason != "smtp timeout" {
		t.Errorf("expected failed status with reason, got %+v", history)
	}
}

func TestMemoryStore_ConcurrentDedup(t *testing.T) {
	ctx := context.Background()
	s := NewMemoryNotificationStore()

	dedupKey := "order.paid:ord-concurrent:email"
	const concurrency = 20

	var wg sync.WaitGroup
	var acceptedCount int
	var mu sync.Mutex

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			notif := domain.Notification{
				ID:        "notif-concurrent",
				DedupKey:  dedupKey,
				EventType: "order.paid",
				OrderID:   "ord-concurrent",
				Recipient: "user@example.com",
				Channel:   domain.ChannelEmail,
			}
			already, err := s.CheckOrSetDedup(ctx, dedupKey, notif)
			if err == nil && !already {
				mu.Lock()
				acceptedCount++
				mu.Unlock()
			}
		}(i)
	}
	wg.Wait()

	if acceptedCount != 1 {
		t.Fatalf("expected exactly 1 accepted insertion under high concurrency, got %d", acceptedCount)
	}
}
