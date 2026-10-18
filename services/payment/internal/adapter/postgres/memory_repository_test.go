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

func TestMemoryRepository_PaymentInbox(t *testing.T) {
	repo := postgres.NewMemoryRepository()
	ctx := context.Background()
	now := time.Now().UTC()

	// 1. Save payment inbox
	item1 := domain.PaymentInboxItem{
		Provider:      domain.ProviderVNPay,
		ProviderTxnID: "vnp_txn_001",
		Payload:       map[string]any{"vnp_Amount": "5000000", "vnp_OrderInfo": "ord-early-1"},
		ReceivedAt:    now.Add(-20 * time.Minute),
		Attempts:      0,
	}

	id1, err := repo.SavePaymentInbox(ctx, item1)
	if err != nil {
		t.Fatalf("SavePaymentInbox failed: %v", err)
	}
	if id1 <= 0 {
		t.Fatalf("expected positive inbox ID, got %d", id1)
	}

	// 2. Save duplicate should be idempotent and return same ID
	dupID, err := repo.SavePaymentInbox(ctx, item1)
	if err != nil {
		t.Fatalf("duplicate SavePaymentInbox failed: %v", err)
	}
	if dupID != id1 {
		t.Errorf("expected dupID %d to equal id1 %d", dupID, id1)
	}

	// 3. Save second item
	item2 := domain.PaymentInboxItem{
		Provider:      domain.ProviderMoMo,
		ProviderTxnID: "momo_txn_002",
		Payload:       map[string]any{"amount": "300000", "orderId": "ord-early-2"},
		ReceivedAt:    now.Add(-5 * time.Minute),
		Attempts:      0,
	}
	id2, err := repo.SavePaymentInbox(ctx, item2)
	if err != nil {
		t.Fatalf("SavePaymentInbox item2 failed: %v", err)
	}

	// 4. GetPendingPaymentInbox
	pending, err := repo.GetPendingPaymentInbox(ctx, 10, 5)
	if err != nil {
		t.Fatalf("GetPendingPaymentInbox failed: %v", err)
	}
	if len(pending) != 2 {
		t.Fatalf("expected 2 pending items, got %d", len(pending))
	}
	// Verify ordered by ReceivedAt ascending
	if pending[0].ID != id1 {
		t.Errorf("expected first pending item to be id1, got %d", pending[0].ID)
	}

	// 5. IncrementPaymentInboxAttempts
	err = repo.IncrementPaymentInboxAttempts(ctx, id1)
	if err != nil {
		t.Fatalf("IncrementPaymentInboxAttempts failed: %v", err)
	}
	pendingAfterInc, _ := repo.GetPendingPaymentInbox(ctx, 10, 5)
	if pendingAfterInc[0].Attempts != 1 {
		t.Errorf("expected attempts = 1, got %d", pendingAfterInc[0].Attempts)
	}

	// 6. GetStalePaymentInbox (> 15 minutes)
	stale, err := repo.GetStalePaymentInbox(ctx, 15*time.Minute, 10)
	if err != nil {
		t.Fatalf("GetStalePaymentInbox failed: %v", err)
	}
	// item1 was received 20m ago, item2 5m ago
	if len(stale) != 1 {
		t.Fatalf("expected 1 stale record, got %d", len(stale))
	}
	if stale[0].ID != id1 {
		t.Errorf("expected stale item id1, got %d", stale[0].ID)
	}

	// 7. MarkPaymentInboxProcessed
	err = repo.MarkPaymentInboxProcessed(ctx, id1, now)
	if err != nil {
		t.Fatalf("MarkPaymentInboxProcessed failed: %v", err)
	}

	// Verify id1 no longer in pending
	pendingAfterMark, _ := repo.GetPendingPaymentInbox(ctx, 10, 5)
	if len(pendingAfterMark) != 1 || pendingAfterMark[0].ID != id2 {
		t.Errorf("expected only id2 in pending, got %+v", pendingAfterMark)
	}

	// Verify id1 no longer in stale
	staleAfterMark, _ := repo.GetStalePaymentInbox(ctx, 15*time.Minute, 10)
	if len(staleAfterMark) != 0 {
		t.Errorf("expected 0 stale items after marking processed, got %d", len(staleAfterMark))
	}
}

func TestMemoryRepository_StaleOrdersAndCancellation(t *testing.T) {
	repo := postgres.NewMemoryRepository()
	ctx := context.Background()
	now := time.Now().UTC()

	// 1. Seed orders: 1 stale pending (> 15m), 1 fresh pending (< 5m), 1 held, 1 paid
	repo.SeedOrder(domain.OrderView{
		ID:         "ord-stale-1",
		EventID:    "evt-1",
		IdentityID: "user-1",
		Status:     "PAYMENT_PENDING",
		TotalCents: 200000,
		Currency:   "VND",
		CreatedAt:  now.Add(-25 * time.Minute),
		UpdatedAt:  now.Add(-20 * time.Minute),
	})

	repo.SeedOrder(domain.OrderView{
		ID:         "ord-fresh-2",
		EventID:    "evt-1",
		IdentityID: "user-2",
		Status:     "PAYMENT_PENDING",
		TotalCents: 150000,
		Currency:   "VND",
		CreatedAt:  now.Add(-3 * time.Minute),
		UpdatedAt:  now.Add(-2 * time.Minute),
	})

	repo.SeedOrder(domain.OrderView{
		ID:         "ord-paid-3",
		EventID:    "evt-1",
		IdentityID: "user-3",
		Status:     "PAID",
		TotalCents: 300000,
		Currency:   "VND",
		CreatedAt:  now.Add(-30 * time.Minute),
		UpdatedAt:  now.Add(-28 * time.Minute),
	})

	// Add payment for ord-stale-1
	repo.SeedPayment(domain.Payment{
		ID:            "pay-stale-1",
		OrderID:       "ord-stale-1",
		Provider:      domain.ProviderSandbox,
		ProviderTxnID: "sbx_txn_99",
		AmountCents:   200000,
		Currency:      "VND",
		Status:        domain.StatusPending,
		CreatedAt:     now.Add(-20 * time.Minute),
	})

	// 2. Query stale orders (> 15m)
	staleOrders, err := repo.GetStalePendingOrders(ctx, 15*time.Minute, 10)
	if err != nil {
		t.Fatalf("GetStalePendingOrders failed: %v", err)
	}
	if len(staleOrders) != 1 {
		t.Fatalf("expected 1 stale order, got %d", len(staleOrders))
	}
	if staleOrders[0].OrderID != "ord-stale-1" {
		t.Errorf("expected ord-stale-1, got %s", staleOrders[0].OrderID)
	}
	if staleOrders[0].Provider != domain.ProviderSandbox || staleOrders[0].ProviderTxnID != "sbx_txn_99" {
		t.Errorf("expected provider sandbox & txn sbx_txn_99, got %s / %s", staleOrders[0].Provider, staleOrders[0].ProviderTxnID)
	}

	// 3. ProcessOrderCancellationTx on stale order
	cancelRes, err := repo.ProcessOrderCancellationTx(ctx, domain.CancelOrderParams{
		OrderID: "ord-stale-1",
		TraceID: "trace-cancel-1",
		Reason:  "qua 15 phut khong nhan duoc webhook",
	})
	if err != nil {
		t.Fatalf("ProcessOrderCancellationTx failed: %v", err)
	}
	if cancelRes.AlreadyDone || cancelRes.Status != "CANCELLED" {
		t.Errorf("unexpected cancel result: %+v", cancelRes)
	}

	// Verify order status is CANCELLED
	cancelledOrder, _ := repo.GetOrder(ctx, "ord-stale-1")
	if cancelledOrder.Status != "CANCELLED" {
		t.Errorf("expected order status CANCELLED, got %s", cancelledOrder.Status)
	}

	// Verify outbox event hold.released emitted
	outbox := repo.GetOutboxEvents()
	foundOutbox := false
	for _, ev := range outbox {
		if ev["routing_key"] == "hold.released" && ev["aggregate_id"] == "ord-stale-1" {
			foundOutbox = true
			break
		}
	}
	if !foundOutbox {
		t.Errorf("expected hold.released outbox event for ord-stale-1")
	}

	// 4. Idempotency: Cancel again returns AlreadyDone = true
	cancelAgain, err := repo.ProcessOrderCancellationTx(ctx, domain.CancelOrderParams{
		OrderID: "ord-stale-1",
		TraceID: "trace-cancel-2",
		Reason:  "retry cancel",
	})
	if err != nil {
		t.Fatalf("second cancel failed: %v", err)
	}
	if !cancelAgain.AlreadyDone {
		t.Errorf("expected AlreadyDone = true on second cancellation")
	}

	// 5. Cancel on PAID order should return AlreadyDone = true, status PAID, not cancelling
	cancelPaid, err := repo.ProcessOrderCancellationTx(ctx, domain.CancelOrderParams{
		OrderID: "ord-paid-3",
		TraceID: "trace-cancel-3",
		Reason:  "cancel paid order attempt",
	})
	if err != nil {
		t.Fatalf("cancel on paid order failed: %v", err)
	}
	if cancelPaid.Status != "PAID" {
		t.Errorf("expected status PAID, got %s", cancelPaid.Status)
	}
}
