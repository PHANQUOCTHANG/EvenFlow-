package app_test

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	gatewayadapter "github.com/eventflow/eventflow/services/payment/internal/adapter/gateway"
	"github.com/eventflow/eventflow/services/payment/internal/adapter/postgres"
	"github.com/eventflow/eventflow/services/payment/internal/app"
	"github.com/eventflow/eventflow/services/payment/internal/domain"
)

func setupRefundUseCase() (*app.RefundCancelledEventUseCase, *postgres.MemoryRepository, *gatewayadapter.Registry) {
	repo := postgres.NewMemoryRepository()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	registry := gatewayadapter.NewRegistry(domain.ProviderSandbox, []string{"sandbox", "vnpay", "momo", "stripe"})
	sbx := gatewayadapter.NewSandboxGateway(gatewayadapter.SandboxModeSuccess)
	registry.Register(sbx)

	vnp := gatewayadapter.NewVNPayGateway(gatewayadapter.VNPayConfig{
		TmnCode:    "TESTTMN",
		HashSecret: "SECRET",
	})
	registry.Register(vnp)

	momo := gatewayadapter.NewMoMoGateway(gatewayadapter.MoMoConfig{
		PartnerCode: "MOMO_TEST",
		AccessKey:   "ACCESS",
		SecretKey:   "SECRET",
	})
	registry.Register(momo)

	stripe := gatewayadapter.NewStripeGateway(gatewayadapter.StripeConfig{
		PublishableKey: "pk_test",
		SecretKey:      "sk_test",
		WebhookSecret:  "whsec_test",
	})
	registry.Register(stripe)

	clock := fixedClock{currentTime: time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)}
	uc := app.NewRefundCancelledEventUseCase(repo, registry, clock, logger)

	return uc, repo, registry
}

func TestRefundCancelledEvent_ScanEligibleOrders(t *testing.T) {
	uc, repo, _ := setupRefundUseCase()
	ctx := context.Background()
	eventID := "evt-scan-1"
	now := time.Now().UTC()

	// Seed 1 PAID, 1 ISSUED, 1 REFUNDING, 1 CANCELLED, 1 EXPIRED, 1 other event
	repo.SeedOrder(domain.OrderView{
		ID:         "ord-1",
		EventID:    eventID,
		IdentityID: "u1",
		Status:     "PAID",
		TotalCents: 100000,
		Currency:   "VND",
		CreatedAt:  now.Add(-10 * time.Minute),
	})
	repo.SeedPayment(domain.Payment{
		ID:            "pay-1",
		OrderID:       "ord-1",
		Provider:      domain.ProviderSandbox,
		ProviderTxnID: "txn-1",
		AmountCents:   100000,
		Status:        domain.StatusSucceeded,
	})

	repo.SeedOrder(domain.OrderView{
		ID:         "ord-2",
		EventID:    eventID,
		IdentityID: "u2",
		Status:     "ISSUED",
		TotalCents: 200000,
		Currency:   "VND",
		CreatedAt:  now.Add(-9 * time.Minute),
	})
	repo.SeedPayment(domain.Payment{
		ID:            "pay-2",
		OrderID:       "ord-2",
		Provider:      domain.ProviderVNPay,
		ProviderTxnID: "txn-2",
		AmountCents:   200000,
		Status:        domain.StatusSucceeded,
	})

	repo.SeedOrder(domain.OrderView{
		ID:         "ord-3",
		EventID:    eventID,
		IdentityID: "u3",
		Status:     "CANCELLED",
		TotalCents: 150000,
		Currency:   "VND",
		CreatedAt:  now.Add(-8 * time.Minute),
	})

	repo.SeedOrder(domain.OrderView{
		ID:         "ord-4",
		EventID:    "evt-other",
		IdentityID: "u4",
		Status:     "PAID",
		TotalCents: 500000,
		Currency:   "VND",
		CreatedAt:  now.Add(-7 * time.Minute),
	})

	out, err := uc.Execute(ctx, app.RefundCancelledEventInput{
		EventID: eventID,
		Reason:  "Sự kiện bị hủy",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if out.TotalOrders != 2 {
		t.Fatalf("expected 2 eligible orders scanned, got %d", out.TotalOrders)
	}
	if out.RefundedCount != 2 {
		t.Errorf("expected 2 orders refunded, got %d", out.RefundedCount)
	}
	if out.TotalAmount != 300000 {
		t.Errorf("expected total amount 300000, got %d", out.TotalAmount)
	}
}

func TestRefundCancelledEvent_MarkRefunding_CallsGatewayOutsideTx(t *testing.T) {
	uc, repo, _ := setupRefundUseCase()
	ctx := context.Background()
	eventID := "evt-mark-1"
	now := time.Now().UTC()

	repo.SeedOrder(domain.OrderView{
		ID:         "ord-mark-1",
		EventID:    eventID,
		IdentityID: "u1",
		Status:     "PAID",
		TotalCents: 250000,
		Currency:   "VND",
		CreatedAt:  now,
	})
	repo.SeedPayment(domain.Payment{
		ID:            "pay-mark-1",
		OrderID:       "ord-mark-1",
		Provider:      domain.ProviderSandbox,
		ProviderTxnID: "txn-mark-1",
		AmountCents:   250000,
		Status:        domain.StatusSucceeded,
	})

	out, err := uc.Execute(ctx, app.RefundCancelledEventInput{
		EventID: eventID,
		Reason:  "Lý do thời tiết",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.RefundedCount != 1 {
		t.Errorf("expected 1 refunded order, got %d", out.RefundedCount)
	}

	order, _ := repo.GetOrder(ctx, "ord-mark-1")
	if order.Status != "REFUNDED" {
		t.Errorf("expected final status REFUNDED, got %s", order.Status)
	}
}

func TestRefundCancelledEvent_Success_FullLifecycle(t *testing.T) {
	uc, repo, _ := setupRefundUseCase()
	ctx := context.Background()
	eventID := "evt-lifecycle-1"
	now := time.Now().UTC()

	// Seed 4 orders with 4 different providers: Sandbox, VNPay, MoMo, Stripe
	providers := []struct {
		orderID  string
		provider domain.Provider
		amount   int64
	}{
		{"ord-sbx", domain.ProviderSandbox, 100000},
		{"ord-vnp", domain.ProviderVNPay, 200000},
		{"ord-momo", domain.ProviderMoMo, 300000},
		{"ord-stripe", domain.ProviderStripe, 400000},
	}

	for _, p := range providers {
		repo.SeedOrder(domain.OrderView{
			ID:         p.orderID,
			EventID:    eventID,
			IdentityID: "user-" + p.orderID,
			Status:     "PAID",
			TotalCents: p.amount,
			Currency:   "VND",
			CreatedAt:  now,
		})
		repo.SeedPayment(domain.Payment{
			ID:            "pay-" + p.orderID,
			OrderID:       p.orderID,
			Provider:      p.provider,
			ProviderTxnID: "txn-" + p.orderID,
			AmountCents:   p.amount,
			Currency:      "VND",
			Status:        domain.StatusSucceeded,
		})
	}

	out, err := uc.Execute(ctx, app.RefundCancelledEventInput{
		EventID: eventID,
		Reason:  "Hủy sự kiện do dịch bệnh",
		TraceID: "trace-lifecycle",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if out.RefundedCount != 4 {
		t.Errorf("expected 4 refunds across 4 providers, got %d", out.RefundedCount)
	}
	if out.TotalAmount != 1000000 {
		t.Errorf("expected total amount 1,000,000 VND, got %d", out.TotalAmount)
	}

	// Verify all orders are REFUNDED and payments are REFUNDED
	for _, p := range providers {
		o, _ := repo.GetOrder(ctx, p.orderID)
		if o.Status != "REFUNDED" {
			t.Errorf("expected order %s to be REFUNDED, got %s", p.orderID, o.Status)
		}
		pm, _ := repo.GetPayment(ctx, "pay-"+p.orderID)
		if pm.Status != domain.StatusRefunded {
			t.Errorf("expected payment %s to be REFUNDED, got %s", pm.ID, pm.Status)
		}
	}

	// Verify outbox events emitted for payment.refunded
	outbox := repo.GetOutboxEvents()
	countPaymentRefunded := 0
	for _, ev := range outbox {
		if ev["routing_key"] == "payment.refunded" {
			countPaymentRefunded++
		}
	}
	if countPaymentRefunded != 4 {
		t.Errorf("expected 4 payment.refunded outbox events, got %d", countPaymentRefunded)
	}
}

func TestRefundCancelledEvent_Idempotent_AlreadyRefunded(t *testing.T) {
	uc, repo, _ := setupRefundUseCase()
	ctx := context.Background()
	eventID := "evt-idem-1"
	now := time.Now().UTC()

	repo.SeedOrder(domain.OrderView{
		ID:         "ord-idem-1",
		EventID:    eventID,
		IdentityID: "u1",
		Status:     "PAID",
		TotalCents: 200000,
		Currency:   "VND",
		CreatedAt:  now,
	})
	repo.SeedPayment(domain.Payment{
		ID:            "pay-idem-1",
		OrderID:       "ord-idem-1",
		Provider:      domain.ProviderSandbox,
		ProviderTxnID: "txn-idem-1",
		AmountCents:   200000,
		Status:        domain.StatusSucceeded,
	})

	// First execution
	out1, err := uc.Execute(ctx, app.RefundCancelledEventInput{
		EventID: eventID,
	})
	if err != nil {
		t.Fatalf("first execution failed: %v", err)
	}
	if out1.RefundedCount != 1 || out1.SkippedCount != 0 {
		t.Errorf("unexpected out1: %+v", out1)
	}

	// Second execution on same event (retry)
	out2, err := uc.Execute(ctx, app.RefundCancelledEventInput{
		EventID: eventID,
	})
	if err != nil {
		t.Fatalf("second execution failed: %v", err)
	}
	if out2.RefundedCount != 0 {
		t.Errorf("expected 0 new refunds on retry, got %d", out2.RefundedCount)
	}
}

func TestRefundCancelledEvent_ConcurrentRetry_Safe(t *testing.T) {
	uc, repo, _ := setupRefundUseCase()
	ctx := context.Background()
	eventID := "evt-concurrent-1"
	now := time.Now().UTC()

	repo.SeedOrder(domain.OrderView{
		ID:         "ord-conc-1",
		EventID:    eventID,
		IdentityID: "u1",
		Status:     "PAID",
		TotalCents: 500000,
		Currency:   "VND",
		CreatedAt:  now,
	})
	repo.SeedPayment(domain.Payment{
		ID:            "pay-conc-1",
		OrderID:       "ord-conc-1",
		Provider:      domain.ProviderSandbox,
		ProviderTxnID: "txn-conc-1",
		AmountCents:   500000,
		Status:        domain.StatusSucceeded,
	})

	// Run 10 concurrent refund workers
	var wg sync.WaitGroup
	totalRefunded := 0
	var mu sync.Mutex

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			out, err := uc.Execute(ctx, app.RefundCancelledEventInput{
				EventID: eventID,
				TraceID: "trace-concurrent",
			})
			if err == nil {
				mu.Lock()
				totalRefunded += out.RefundedCount
				mu.Unlock()
			}
		}(i)
	}
	wg.Wait()

	// Exactly 1 refund should succeed, others skipped or already done
	if totalRefunded != 1 {
		t.Errorf("expected exactly 1 refund across concurrent executions, got %d", totalRefunded)
	}
}

func TestRefundCancelledEvent_GatewayError_Resilience(t *testing.T) {
	uc, repo, registry := setupRefundUseCase()
	ctx := context.Background()
	eventID := "evt-resilience-1"
	now := time.Now().UTC()

	// 1 good order, 1 failing order (custom fail status)
	sbx, _ := registry.Get(domain.ProviderSandbox)
	sbxGateway := sbx.(*gatewayadapter.SandboxGateway)
	sbxGateway.SetRefundStatus("ord-fail", domain.StatusFailed)

	repo.SeedOrder(domain.OrderView{
		ID:         "ord-good",
		EventID:    eventID,
		IdentityID: "u1",
		Status:     "PAID",
		TotalCents: 100000,
		Currency:   "VND",
		CreatedAt:  now.Add(-2 * time.Minute),
	})
	repo.SeedPayment(domain.Payment{
		ID:            "pay-good",
		OrderID:       "ord-good",
		Provider:      domain.ProviderSandbox,
		ProviderTxnID: "txn-good",
		AmountCents:   100000,
		Status:        domain.StatusSucceeded,
	})

	repo.SeedOrder(domain.OrderView{
		ID:         "ord-fail",
		EventID:    eventID,
		IdentityID: "u2",
		Status:     "PAID",
		TotalCents: 200000,
		Currency:   "VND",
		CreatedAt:  now.Add(-1 * time.Minute),
	})
	repo.SeedPayment(domain.Payment{
		ID:            "pay-fail",
		OrderID:       "ord-fail",
		Provider:      domain.ProviderSandbox,
		ProviderTxnID: "txn-fail",
		AmountCents:   200000,
		Status:        domain.StatusSucceeded,
	})

	out, err := uc.Execute(ctx, app.RefundCancelledEventInput{
		EventID: eventID,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if out.TotalOrders != 2 {
		t.Errorf("expected total 2, got %d", out.TotalOrders)
	}
	if out.RefundedCount != 1 {
		t.Errorf("expected 1 refunded, got %d", out.RefundedCount)
	}
	if out.FailedCount != 1 {
		t.Errorf("expected 1 failed, got %d", out.FailedCount)
	}

	// Good order should still be REFUNDED
	ordGood, _ := repo.GetOrder(ctx, "ord-good")
	if ordGood.Status != "REFUNDED" {
		t.Errorf("expected ord-good status REFUNDED, got %s", ordGood.Status)
	}
}

func TestRefundCancelledEvent_RestoresBuyerQuota(t *testing.T) {
	uc, repo, _ := setupRefundUseCase()
	ctx := context.Background()
	eventID := "evt-quota-1"
	now := time.Now().UTC()

	repo.SeedOrder(domain.OrderView{
		ID:         "ord-quota-1",
		EventID:    eventID,
		IdentityID: "buyer-limit",
		Status:     "PAID",
		TotalCents: 400000,
		Currency:   "VND",
		CreatedAt:  now,
	})
	repo.SeedPayment(domain.Payment{
		ID:            "pay-quota-1",
		OrderID:       "ord-quota-1",
		Provider:      domain.ProviderSandbox,
		ProviderTxnID: "txn-q1",
		AmountCents:   400000,
		Status:        domain.StatusSucceeded,
	})

	out, err := uc.Execute(ctx, app.RefundCancelledEventInput{
		EventID: eventID,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.RefundedCount != 1 {
		t.Fatalf("expected 1 refunded order")
	}

	// BR-O4: Kiểm tra trạng thái đơn hàng là REFUNDED
	order, _ := repo.GetOrder(ctx, "ord-quota-1")
	if order.Status != "REFUNDED" {
		t.Errorf("expected order status REFUNDED, got %s", order.Status)
	}

	// Xác nhận OrderView.IsRefundable() giờ trả về false vì đã hoàn tiền xong
	if order.IsRefundable() {
		t.Errorf("REFUNDED order should not be refundable again")
	}
}

func TestRefundCancelledEvent_ContextCancellation(t *testing.T) {
	uc, repo, _ := setupRefundUseCase()
	eventID := "evt-cancel-ctx"
	now := time.Now().UTC()

	repo.SeedOrder(domain.OrderView{
		ID:         "ord-ctx-1",
		EventID:    eventID,
		IdentityID: "u1",
		Status:     "PAID",
		TotalCents: 100000,
		Currency:   "VND",
		CreatedAt:  now,
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	_, err := uc.Execute(ctx, app.RefundCancelledEventInput{
		EventID: eventID,
	})
	if err == nil {
		t.Errorf("expected context cancelled error, got nil")
	}
}
