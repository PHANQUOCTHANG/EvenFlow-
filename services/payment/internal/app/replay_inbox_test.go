package app_test

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/eventflow/eventflow/services/payment/internal/adapter/postgres"
	"github.com/eventflow/eventflow/services/payment/internal/app"
	"github.com/eventflow/eventflow/services/payment/internal/domain"
)

func setupReplayTestEnv() (*postgres.MemoryRepository, *slog.Logger) {
	repo := postgres.NewMemoryRepository()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return repo, logger
}

// TestReplayPaymentInbox_SuccessWhenOrderAppears kiem chung AC-3 (BR-O6):
// Khi don hang xuat hien, job replay quet inbox va thuc thi thanh cong: don sang PAID, outbox order.paid, inbox processed.
func TestReplayPaymentInbox_SuccessWhenOrderAppears(t *testing.T) {
	repo, logger := setupReplayTestEnv()
	now := time.Now().UTC()
	clock := fixedClock{currentTime: now}
	ctx := context.Background()

	orderID := "ord-replay-success"
	txnID := "txn-replay-vnp-1"

	// 1. Gia lap webhook den som khi don chua ton tai
	inboxID, err := repo.SavePaymentInbox(ctx, domain.PaymentInboxItem{
		Provider:      domain.ProviderVNPay,
		ProviderTxnID: txnID,
		Payload: map[string]any{
			"order_id":        orderID,
			"provider_txn_id": txnID,
			"amount_cents":    int64(200000),
			"status":          "SUCCEEDED",
		},
		ReceivedAt: now.Add(-2 * time.Minute),
		Attempts:   0,
	})
	if err != nil {
		t.Fatalf("SavePaymentInbox failed: %v", err)
	}

	uc := app.NewReplayPaymentInboxUseCase(repo, clock, logger)

	// 2. Chay replay lan 1: Don hang van chua ton tai
	res1, err := uc.Execute(ctx, 10)
	if err != nil {
		t.Fatalf("Replay Execute failed: %v", err)
	}
	if res1.TotalScanned != 1 || res1.StillPending != 1 || res1.Succeeded != 0 {
		t.Errorf("expected StillPending = 1, got %+v", res1)
	}

	// 3. Khach hang / he thong tao don hang thanh cong trong DB
	repo.SeedOrder(domain.OrderView{
		ID:         orderID,
		EventID:    "evt-concert-1",
		IdentityID: "buyer-1",
		Status:     "PAYMENT_PENDING",
		TotalCents: 200000,
		Currency:   "VND",
		ExpiresAt:  now.Add(10 * time.Minute),
		CreatedAt:  now.Add(-1 * time.Minute),
	})

	// 4. Chay replay lan 2: Don hang da xuat hien -> phai thanh cong
	res2, err := uc.Execute(ctx, 10)
	if err != nil {
		t.Fatalf("Replay Execute run 2 failed: %v", err)
	}
	if res2.TotalScanned != 1 || res2.Succeeded != 1 {
		t.Fatalf("expected Succeeded = 1, got %+v", res2)
	}

	// 5. Kiem tra don hang da chuyen sang PAID (Quy tac bat bien 1)
	order, _ := repo.GetOrder(ctx, orderID)
	if order.Status != "PAID" {
		t.Errorf("expected order status PAID, got %s", order.Status)
	}

	// 6. Kiem tra su kien outbox order.paid da duoc phat
	outbox := repo.GetOutboxEvents()
	if len(outbox) != 1 {
		t.Fatalf("expected 1 outbox event, got %d", len(outbox))
	}
	if outbox[0]["routing_key"] != "order.paid" {
		t.Errorf("expected routing_key order.paid, got %v", outbox[0]["routing_key"])
	}

	// 7. Kiem tra ban ghi inbox khong con nam trong danh sach pending
	pendingAfter, _ := repo.GetPendingPaymentInbox(ctx, 10, 5)
	if len(pendingAfter) != 0 {
		t.Errorf("expected 0 pending items after successful replay, got %d", len(pendingAfter))
	}

	_ = inboxID
}

// TestReplayPaymentInbox_AlreadyPaid_Idempotent kiem chung AC-4 (BR-O6):
// Neu don hang da sang PAID tu truoc (vi du do webhook thu 2 den sau da xu ly), replay nhan biet AlreadyDone va khong ghi trung outbox.
func TestReplayPaymentInbox_AlreadyPaid_Idempotent(t *testing.T) {
	repo, logger := setupReplayTestEnv()
	now := time.Now().UTC()
	clock := fixedClock{currentTime: now}
	ctx := context.Background()

	orderID := "ord-already-paid"
	txnID := "txn-already-paid-1"

	// Don hang da PAID
	repo.SeedOrder(domain.OrderView{
		ID:         orderID,
		EventID:    "evt-1",
		IdentityID: "user-1",
		Status:     "PAID",
		TotalCents: 150000,
		Currency:   "VND",
	})

	_, _ = repo.SavePaymentInbox(ctx, domain.PaymentInboxItem{
		Provider:      domain.ProviderMoMo,
		ProviderTxnID: txnID,
		Payload: map[string]any{
			"order_id":     orderID,
			"amount_cents": int64(150000),
			"status":       "SUCCEEDED",
		},
		ReceivedAt: now.Add(-3 * time.Minute),
	})

	uc := app.NewReplayPaymentInboxUseCase(repo, clock, logger)
	res, err := uc.Execute(ctx, 10)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if res.AlreadyDone != 1 {
		t.Errorf("expected AlreadyDone = 1, got %+v", res)
	}

	// Khong sinh outbox event moi
	if len(repo.GetOutboxEvents()) != 0 {
		t.Errorf("expected 0 outbox events on already-paid order, got %d", len(repo.GetOutboxEvents()))
	}
}

// TestReplayPaymentInbox_ConcurrentReplay_Safe kiem chung AC-4 ve an toan dong thoi:
// 20 workers cung replay dong thoi mot ban ghi inbox -> dung 1 lan chuyen PAID va dung 1 outbox event.
func TestReplayPaymentInbox_ConcurrentReplay_Safe(t *testing.T) {
	repo, logger := setupReplayTestEnv()
	now := time.Now().UTC()
	clock := fixedClock{currentTime: now}
	ctx := context.Background()

	orderID := "ord-concurrent-replay"
	txnID := "txn-concurrent-replay-1"

	repo.SeedOrder(domain.OrderView{
		ID:         orderID,
		EventID:    "evt-concurrent",
		IdentityID: "user-concurrent",
		Status:     "PAYMENT_PENDING",
		TotalCents: 100000,
		Currency:   "VND",
		ExpiresAt:  now.Add(10 * time.Minute),
	})

	_, _ = repo.SavePaymentInbox(ctx, domain.PaymentInboxItem{
		Provider:      domain.ProviderSandbox,
		ProviderTxnID: txnID,
		Payload: map[string]any{
			"order_id":     orderID,
			"amount_cents": int64(100000),
			"status":       "SUCCEEDED",
		},
		ReceivedAt: now.Add(-1 * time.Minute),
	})

	concurrency := 20
	var wg sync.WaitGroup
	wg.Add(concurrency)

	for i := 0; i < concurrency; i++ {
		go func() {
			defer wg.Done()
			uc := app.NewReplayPaymentInboxUseCase(repo, clock, logger)
			_, _ = uc.Execute(ctx, 10)
		}()
	}

	wg.Wait()

	// Kiem tra don hang o trang thai PAID
	order, _ := repo.GetOrder(ctx, orderID)
	if order.Status != "PAID" {
		t.Errorf("expected order status PAID, got %s", order.Status)
	}

	// Kiem tra outbox chi dung 1 su kien duy nhat (BR-O6 exactly-once)
	events := repo.GetOutboxEvents()
	if len(events) != 1 {
		t.Errorf("expected exactly 1 outbox event across concurrent replays, got %d", len(events))
	}
}

// TestReplayPaymentInbox_MaxAttempts_StopsRetrying kiem chung AC-5:
// Khi so lan thu vuot qua maxAttempts (vi du don hang khong bao gio ton tai), dung retry de tranh hao phi tai nguyen.
func TestReplayPaymentInbox_MaxAttempts_StopsRetrying(t *testing.T) {
	repo, logger := setupReplayTestEnv()
	now := time.Now().UTC()
	clock := fixedClock{currentTime: now}
	ctx := context.Background()

	uc := app.NewReplayPaymentInboxUseCase(repo, clock, logger).WithMaxAttempts(3)

	_, _ = repo.SavePaymentInbox(ctx, domain.PaymentInboxItem{
		Provider:      domain.ProviderStripe,
		ProviderTxnID: "txn-never-order",
		Payload: map[string]any{
			"order_id":     "ord-phantom",
			"amount_cents": int64(50000),
			"status":       "SUCCEEDED",
		},
		ReceivedAt: now.Add(-10 * time.Minute),
		Attempts:   0,
	})

	// Lan 1: attempts -> 1
	res1, _ := uc.Execute(ctx, 10)
	if res1.TotalScanned != 1 || res1.StillPending != 1 {
		t.Errorf("run 1: expected scanned 1, got %+v", res1)
	}

	// Lan 2: attempts -> 2
	res2, _ := uc.Execute(ctx, 10)
	if res2.TotalScanned != 1 || res2.StillPending != 1 {
		t.Errorf("run 2: expected scanned 1, got %+v", res2)
	}

	// Lan 3: attempts -> 3 (dat maxAttempts)
	res3, _ := uc.Execute(ctx, 10)
	if res3.TotalScanned != 1 || res3.StillPending != 1 {
		t.Errorf("run 3: expected scanned 1, got %+v", res3)
	}

	// Lan 4: Vuot nguong maxAttempts=3 -> khong con duoc quet de thu lai
	res4, _ := uc.Execute(ctx, 10)
	if res4.TotalScanned != 0 {
		t.Errorf("run 4: expected 0 scanned after maxAttempts reached, got %d", res4.TotalScanned)
	}
}

// TestReplayPaymentInbox_StaleRecords_Alert kiem chung AC-6:
// Phat hien va ghi log canh bao cac ban ghi inbox chua xu ly co tuoi tho > 15 phut.
func TestReplayPaymentInbox_StaleRecords_Alert(t *testing.T) {
	repo, logger := setupReplayTestEnv()
	now := time.Now().UTC()
	clock := fixedClock{currentTime: now}
	ctx := context.Background()

	uc := app.NewReplayPaymentInboxUseCase(repo, clock, logger).WithStaleThreshold(15 * time.Minute)

	// Ban ghi 1: nhan 25 phut truoc (> 15 phut -> Stale)
	_, _ = repo.SavePaymentInbox(ctx, domain.PaymentInboxItem{
		Provider:      domain.ProviderVNPay,
		ProviderTxnID: "txn-stale-25m",
		Payload: map[string]any{
			"order_id":     "ord-stale-1",
			"amount_cents": int64(100000),
		},
		ReceivedAt: now.Add(-25 * time.Minute),
		Attempts:   2,
	})

	// Ban ghi 2: nhan 5 phut truoc (<= 15 phut -> Chua stale)
	_, _ = repo.SavePaymentInbox(ctx, domain.PaymentInboxItem{
		Provider:      domain.ProviderMoMo,
		ProviderTxnID: "txn-fresh-5m",
		Payload: map[string]any{
			"order_id":     "ord-fresh-1",
			"amount_cents": int64(200000),
		},
		ReceivedAt: now.Add(-5 * time.Minute),
		Attempts:   0,
	})

	stale, err := uc.CheckStaleRecords(ctx, 10)
	if err != nil {
		t.Fatalf("CheckStaleRecords failed: %v", err)
	}

	if len(stale) != 1 {
		t.Fatalf("expected exactly 1 stale record, got %d", len(stale))
	}
	if stale[0].ProviderTxnID != "txn-stale-25m" {
		t.Errorf("expected stale record txn-stale-25m, got %s", stale[0].ProviderTxnID)
	}
	if stale[0].Age < 20*time.Minute {
		t.Errorf("expected age >= 20m, got %v", stale[0].Age)
	}
}

func TestReplayPaymentInbox_EdgeCases(t *testing.T) {
	repo, logger := setupReplayTestEnv()
	now := time.Now().UTC()
	clock := fixedClock{currentTime: now}
	ctx := context.Background()

	uc := app.NewReplayPaymentInboxUseCase(repo, clock, logger).
		WithMaxAttempts(0).   // should keep default
		WithStaleThreshold(0) // should keep default

	// 1. Missing order_id -> should mark Failed
	_, _ = repo.SavePaymentInbox(ctx, domain.PaymentInboxItem{
		Provider:      domain.ProviderSandbox,
		ProviderTxnID: "txn-no-order-id",
		Payload:       map[string]any{"amount_cents": 1000},
		ReceivedAt:    now,
	})

	// 2. Various amount_cents types (string, float64, int) & alternative order_id keys
	_, _ = repo.SavePaymentInbox(ctx, domain.PaymentInboxItem{
		Provider:      domain.ProviderMoMo,
		ProviderTxnID: "txn-alt-order-id",
		Payload: map[string]any{
			"orderId":      "ord-alt",
			"amount_cents": "50000",
		},
		ReceivedAt: now,
	})

	_, _ = repo.SavePaymentInbox(ctx, domain.PaymentInboxItem{
		Provider:      domain.ProviderVNPay,
		ProviderTxnID: "txn-vnp-order-info",
		Payload: map[string]any{
			"vnp_OrderInfo": "ord-vnp",
			"amount_cents":  float64(75000),
		},
		ReceivedAt: now,
	})

	// 3. FAILED payment in inbox
	_, _ = repo.SavePaymentInbox(ctx, domain.PaymentInboxItem{
		Provider:      domain.ProviderSandbox,
		ProviderTxnID: "txn-failed-inbox",
		Payload: map[string]any{
			"order_id":     "ord-failed",
			"amount_cents": int(90000),
			"status":       "FAILED",
		},
		ReceivedAt: now,
	})

	// Run with limit <= 0 to test default limit
	res, err := uc.Execute(ctx, -1)
	if err != nil {
		t.Fatalf("Execute with negative limit failed: %v", err)
	}
	if res.TotalScanned != 4 {
		t.Errorf("expected 4 scanned, got %d", res.TotalScanned)
	}
	if res.Failed < 1 {
		t.Errorf("expected at least 1 failed for missing order_id, got %d", res.Failed)
	}

	// Seed ord-alt and rerun replay to test success path
	repo.SeedOrder(domain.OrderView{
		ID:         "ord-alt",
		Status:     "PAYMENT_PENDING",
		TotalCents: 50000,
	})
	resRun2, _ := uc.Execute(ctx, 10)
	if resRun2.Succeeded < 1 {
		t.Errorf("expected succeeded for ord-alt replay, got %d", resRun2.Succeeded)
	}
}
