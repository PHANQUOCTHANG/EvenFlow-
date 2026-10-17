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

func TestProcessPaymentSuccessTx_Atomic(t *testing.T) {
	repo := postgres.NewMemoryRepository()
	ctx := context.Background()
	now := time.Now().UTC()

	repo.SeedOrder(domain.OrderView{
		ID:         "ord-tx-1",
		EventID:    "evt-tx-1",
		IdentityID: "user-tx-1",
		Status:     "PAYMENT_PENDING",
		TotalCents: 500000,
		Currency:   "VND",
		ExpiresAt:  now.Add(10 * time.Minute),
	})

	params := domain.ProcessWebhookParams{
		Provider:      domain.ProviderSandbox,
		ProviderTxnID: "sbx_txn_atomic_1",
		OrderID:       "ord-tx-1",
		PaymentID:     "pay-atomic-1",
		AmountCents:   500000,
		Currency:      "VND",
		Status:        domain.StatusSucceeded,
		TraceID:       "trace-tx-1",
		Timestamp:     now,
	}

	// Lan 1: Xu ly thanh cong, don chuyen sang PAID va phat outbox
	res, err := repo.ProcessPaymentSuccessTx(ctx, params)
	if err != nil {
		t.Fatalf("ProcessPaymentSuccessTx failed: %v", err)
	}
	if !res.Success || res.AlreadyDone {
		t.Errorf("expected Success=true, AlreadyDone=false, got %+v", res)
	}

	order, err := repo.GetOrder(ctx, "ord-tx-1")
	if err != nil || order.Status != "PAID" {
		t.Errorf("expected order status PAID, got %s", order.Status)
	}

	events := repo.GetOutboxEvents()
	if len(events) != 1 {
		t.Fatalf("expected 1 outbox event, got %d", len(events))
	}
	if events[0]["routing_key"] != "order.paid" {
		t.Errorf("expected routing_key order.paid, got %v", events[0]["routing_key"])
	}

	// Lan 2: Duplicate webhook voi cung (provider, provider_txn_id) -> Exactly-once BR-O6
	resDup, err := repo.ProcessPaymentSuccessTx(ctx, params)
	if err != nil {
		t.Fatalf("ProcessPaymentSuccessTx duplicate failed: %v", err)
	}
	if !resDup.Success || !resDup.AlreadyDone {
		t.Errorf("expected Success=true, AlreadyDone=true for duplicate, got %+v", resDup)
	}

	// Outbox van chi co 1 su kien duy nhat, khong bi duplicate
	eventsAfter := repo.GetOutboxEvents()
	if len(eventsAfter) != 1 {
		t.Errorf("expected exactly 1 outbox event after duplicate, got %d", len(eventsAfter))
	}
}

func TestProcessPaymentFailedTx(t *testing.T) {
	repo := postgres.NewMemoryRepository()
	ctx := context.Background()

	repo.SeedOrder(domain.OrderView{
		ID:         "ord-fail-1",
		EventID:    "evt-1",
		IdentityID: "user-1",
		Status:     "PAYMENT_PENDING",
		TotalCents: 100000,
		Currency:   "VND",
	})

	params := domain.ProcessWebhookParams{
		Provider:      domain.ProviderSandbox,
		ProviderTxnID: "sbx_txn_fail_1",
		OrderID:       "ord-fail-1",
		Status:        domain.StatusFailed,
	}

	res, err := repo.ProcessPaymentFailedTx(ctx, params)
	if err != nil {
		t.Fatalf("ProcessPaymentFailedTx failed: %v", err)
	}
	if res.Status != domain.StatusFailed {
		t.Errorf("expected StatusFailed, got %s", res.Status)
	}

	order, _ := repo.GetOrder(ctx, "ord-fail-1")
	if order.Status == "PAID" {
		t.Errorf("order should not be marked PAID on failed payment")
	}

	events := repo.GetOutboxEvents()
	if len(events) != 0 {
		t.Errorf("expected 0 outbox events on failed payment, got %d", len(events))
	}
}
