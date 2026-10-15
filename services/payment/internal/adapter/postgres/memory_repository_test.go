package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/eventflow/eventflow/services/payment/internal/adapter/postgres"
	"github.com/eventflow/eventflow/services/payment/internal/domain"
)

func TestMemoryRepository(t *testing.T) {
	repo := postgres.NewMemoryRepository()
	ctx := context.Background()

	// 1. Seed order
	order := domain.OrderView{
		ID:         "ord-1",
		EventID:    "ev-1",
		IdentityID: "user-1",
		Status:     "HELD",
		TotalCents: 100000,
		Currency:   "VND",
		ExpiresAt:  time.Now().Add(10 * time.Minute),
	}
	repo.SeedOrder(order)

	// 2. Get order
	gotOrder, err := repo.GetOrder(ctx, "ord-1")
	if err != nil {
		t.Fatalf("failed to get order: %v", err)
	}
	if gotOrder.ID != "ord-1" || gotOrder.Status != "HELD" {
		t.Errorf("unexpected order: %+v", gotOrder)
	}

	// 3. Get non-existent order
	_, err = repo.GetOrder(ctx, "ord-nonexistent")
	if !errors.Is(err, domain.ErrOrderNotFound) {
		t.Errorf("expected ErrOrderNotFound, got %v", err)
	}

	// 4. Create payment and mark pending
	payment := domain.Payment{
		ID:            "pay-1",
		OrderID:       "ord-1",
		Provider:      domain.ProviderSandbox,
		ProviderTxnID: "sbx_123",
		AmountCents:   100000,
		Currency:      "VND",
		Status:        domain.StatusPending,
		CreatedAt:     time.Now(),
	}
	err = repo.CreatePaymentAndMarkPending(ctx, payment)
	if err != nil {
		t.Fatalf("failed to create payment and mark pending: %v", err)
	}

	// Verify order status updated to PAYMENT_PENDING
	updatedOrder, _ := repo.GetOrder(ctx, "ord-1")
	if updatedOrder.Status != "PAYMENT_PENDING" {
		t.Errorf("expected status PAYMENT_PENDING, got %s", updatedOrder.Status)
	}

	// Verify payment can be queried by payment_id
	gotPayment, err := repo.GetPayment(ctx, "pay-1")
	if err != nil {
		t.Fatalf("failed to get payment: %v", err)
	}
	if gotPayment.ID != "pay-1" || gotPayment.Provider != domain.ProviderSandbox {
		t.Errorf("unexpected payment: %+v", gotPayment)
	}

	// Verify payment can be queried by order_id
	gotPaymentByOrder, err := repo.GetPaymentByOrder(ctx, "ord-1")
	if err != nil {
		t.Fatalf("failed to get payment by order: %v", err)
	}
	if gotPaymentByOrder.ID != "pay-1" {
		t.Errorf("expected payment pay-1, got %s", gotPaymentByOrder.ID)
	}

	// 5. Duplicate create on non-held order should fail with ErrOrderNotHeld
	err = repo.CreatePaymentAndMarkPending(ctx, payment)
	if !errors.Is(err, domain.ErrOrderNotHeld) {
		t.Errorf("expected ErrOrderNotHeld when order is not HELD, got %v", err)
	}
}
