package app

import (
	"context"
	"testing"
	"time"

	"github.com/eventflow/eventflow/services/notification/internal/adapter/store"
	"github.com/eventflow/eventflow/services/notification/internal/domain"
)

func TestGetNotificationHistory(t *testing.T) {
	ctx := context.Background()
	memStore := store.NewMemoryNotificationStore()
	uc := NewGetNotificationHistoryUseCase(memStore)

	// Chua co ban ghi
	h1, err := uc.Execute(ctx, "ord-none")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(h1) != 0 {
		t.Errorf("expected empty history")
	}

	// Them ban ghi
	notif := domain.Notification{
		ID:        "notif-h1",
		DedupKey:  "order.paid:ord-exists:email",
		EventType: "order.paid",
		OrderID:   "ord-exists",
		Recipient: "user@example.com",
		Channel:   domain.ChannelEmail,
		CreatedAt: time.Now(),
	}
	_, _ = memStore.CheckOrSetDedup(ctx, notif.DedupKey, notif)

	h2, err := uc.Execute(ctx, "ord-exists")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(h2) != 1 || h2[0].ID != "notif-h1" {
		t.Errorf("expected 1 record with id notif-h1, got %+v", h2)
	}

	// Empty orderID
	_, err = uc.Execute(ctx, "   ")
	if err == nil {
		t.Errorf("expected error for empty orderID")
	}
}
