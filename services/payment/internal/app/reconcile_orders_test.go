package app_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	gatewayadapter "github.com/eventflow/eventflow/services/payment/internal/adapter/gateway"
	"github.com/eventflow/eventflow/services/payment/internal/adapter/postgres"
	"github.com/eventflow/eventflow/services/payment/internal/app"
	"github.com/eventflow/eventflow/services/payment/internal/domain"
	"github.com/eventflow/eventflow/services/payment/internal/port"
)

type mockMetrics struct {
	mu           sync.Mutex
	lagRecords   []time.Duration
	reconcileInc map[string]int
	durRecords   []time.Duration
}

func newMockMetrics() *mockMetrics {
	return &mockMetrics{
		reconcileInc: make(map[string]int),
	}
}

func (m *mockMetrics) ObserveWebhookLag(d time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lagRecords = append(m.lagRecords, d)
}

func (m *mockMetrics) IncReconciliation(outcome string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.reconcileInc[outcome]++
}

func (m *mockMetrics) ObserveReconciliationDuration(d time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.durRecords = append(m.durRecords, d)
}

func (m *mockMetrics) getOutcomeCount(outcome string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.reconcileInc[outcome]
}

func setupReconcileTestEnv(clock fixedClock) (*postgres.MemoryRepository, *gatewayadapter.Registry, *gatewayadapter.SandboxGateway, *mockMetrics, *slog.Logger) {
	repo := postgres.NewMemoryRepository()
	repo.SetClock(func() time.Time { return clock.currentTime })
	registry := gatewayadapter.NewRegistry(domain.ProviderSandbox, []string{"sandbox", "vnpay", "momo", "stripe"})
	sbxGW := gatewayadapter.NewSandboxGateway(gatewayadapter.SandboxModeSuccess)
	registry.Register(sbxGW)

	metrics := newMockMetrics()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return repo, registry, sbxGW, metrics, logger
}

// AC-1: Quét đơn PAYMENT_PENDING quá 15 phút, bỏ qua đơn mới hoặc đơn khác trạng thái
func TestReconcileOrders_ScanStalePending(t *testing.T) {
	now := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	clock := fixedClock{currentTime: now}
	repo, registry, _, metrics, logger := setupReconcileTestEnv(clock)

	// 1 stale order (> 15m)
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

	// 1 fresh order (< 15m)
	repo.SeedOrder(domain.OrderView{
		ID:         "ord-fresh-2",
		EventID:    "evt-1",
		IdentityID: "user-2",
		Status:     "PAYMENT_PENDING",
		TotalCents: 150000,
		Currency:   "VND",
		CreatedAt:  now.Add(-5 * time.Minute),
		UpdatedAt:  now.Add(-3 * time.Minute),
	})

	uc := app.NewReconcileStaleOrdersUseCase(repo, registry, clock, metrics, logger)
	res, err := uc.Execute(context.Background(), 50)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.TotalScanned != 1 {
		t.Errorf("expected 1 stale order scanned, got %d", res.TotalScanned)
	}
}

// AC-2 & AC-3: Cổng xác nhận thanh toán thành công -> Tự chữa sang PAID và phát outbox order.paid
func TestReconcileOrders_HealPaid_Success(t *testing.T) {
	now := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	clock := fixedClock{currentTime: now}
	repo, registry, sbxGW, metrics, logger := setupReconcileTestEnv(clock)

	orderID := "ord-heal-paid"
	repo.SeedOrder(domain.OrderView{
		ID:         orderID,
		EventID:    "evt-10",
		IdentityID: "user-10",
		Status:     "PAYMENT_PENDING",
		TotalCents: 350000,
		Currency:   "VND",
		CreatedAt:  now.Add(-20 * time.Minute),
		UpdatedAt:  now.Add(-18 * time.Minute),
	})

	repo.SeedPayment(domain.Payment{
		ID:            "pay-10",
		OrderID:       orderID,
		Provider:      domain.ProviderSandbox,
		ProviderTxnID: "sbx_txn_success_10",
		AmountCents:   350000,
		Currency:      "VND",
		Status:        domain.StatusPending,
		CreatedAt:     now.Add(-18 * time.Minute),
	})

	sbxGW.SetQueryStatus(orderID, domain.StatusSucceeded)

	uc := app.NewReconcileStaleOrdersUseCase(repo, registry, clock, metrics, logger)
	res, err := uc.Execute(context.Background(), 50)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if res.HealedPaid != 1 {
		t.Errorf("expected 1 healed_paid, got %d", res.HealedPaid)
	}

	// Verify order status is PAID
	ord, _ := repo.GetOrder(context.Background(), orderID)
	if ord.Status != "PAID" {
		t.Errorf("expected order status PAID, got %s", ord.Status)
	}

	// Verify outbox event order.paid
	outbox := repo.GetOutboxEvents()
	foundPaid := false
	for _, ev := range outbox {
		if ev["routing_key"] == "order.paid" && ev["aggregate_id"] == orderID {
			foundPaid = true
			break
		}
	}
	if !foundPaid {
		t.Errorf("expected order.paid outbox event")
	}

	// Verify webhook lag metric was recorded
	if len(metrics.lagRecords) != 1 {
		t.Errorf("expected 1 lag record, got %d", len(metrics.lagRecords))
	}
	if metrics.getOutcomeCount("healed_paid") != 1 {
		t.Errorf("expected metric healed_paid = 1, got %d", metrics.getOutcomeCount("healed_paid"))
	}
}

// AC-3: Idempotent - Nếu đơn đã sang PAID (do webhook tới muộn song song), đối soát xử lý an toàn
func TestReconcileOrders_IdempotentAlreadyPaid(t *testing.T) {
	now := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	clock := fixedClock{currentTime: now}
	repo, registry, sbxGW, metrics, logger := setupReconcileTestEnv(clock)

	orderID := "ord-already-paid"
	repo.SeedOrder(domain.OrderView{
		ID:         orderID,
		EventID:    "evt-11",
		IdentityID: "user-11",
		Status:     "PAYMENT_PENDING",
		TotalCents: 500000,
		Currency:   "VND",
		CreatedAt:  now.Add(-22 * time.Minute),
		UpdatedAt:  now.Add(-20 * time.Minute),
	})

	repo.SeedPayment(domain.Payment{
		ID:            "pay-11",
		OrderID:       orderID,
		Provider:      domain.ProviderSandbox,
		ProviderTxnID: "sbx_txn_already_done",
		AmountCents:   500000,
		Currency:      "VND",
		Status:        domain.StatusPending,
		CreatedAt:     now.Add(-20 * time.Minute),
	})

	sbxGW.SetQueryStatus(orderID, domain.StatusSucceeded)

	// Gia lap don da sang PAID boi tien trinh khac truoc khi doi soat goi DB
	_, _ = repo.ProcessPaymentSuccessTx(context.Background(), domain.ProcessWebhookParams{
		Provider:      domain.ProviderSandbox,
		ProviderTxnID: "sbx_txn_already_done",
		OrderID:       orderID,
		PaymentID:     "pay-11",
		AmountCents:   500000,
		Currency:      "VND",
		Status:        domain.StatusSucceeded,
		TraceID:       "trace-ahead",
		Timestamp:     now,
	})

	uc := app.NewReconcileStaleOrdersUseCase(repo, registry, clock, metrics, logger)
	res, err := uc.Execute(context.Background(), 50)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.Skipped != 1 && res.HealedPaid != 0 {
		t.Errorf("expected 1 skipped / 0 healed_paid for already paid order, got %+v", res)
	}
}

// AC-4: Cổng xác nhận thất bại -> Tự chữa CANCELLED, trả kho ghế idempotent và phát outbox hold.released
func TestReconcileOrders_HealCancelled_ReleaseHold(t *testing.T) {
	now := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	clock := fixedClock{currentTime: now}
	repo, registry, sbxGW, metrics, logger := setupReconcileTestEnv(clock)

	orderID := "ord-heal-cancelled"
	repo.SeedOrder(domain.OrderView{
		ID:         orderID,
		EventID:    "evt-12",
		IdentityID: "user-12",
		Status:     "PAYMENT_PENDING",
		TotalCents: 400000,
		Currency:   "VND",
		CreatedAt:  now.Add(-30 * time.Minute),
		UpdatedAt:  now.Add(-25 * time.Minute),
	})

	repo.SeedPayment(domain.Payment{
		ID:            "pay-12",
		OrderID:       orderID,
		Provider:      domain.ProviderSandbox,
		ProviderTxnID: "sbx_txn_fail_12",
		AmountCents:   400000,
		Currency:      "VND",
		Status:        domain.StatusPending,
		CreatedAt:     now.Add(-25 * time.Minute),
	})

	sbxGW.SetQueryStatus(orderID, domain.StatusFailed)

	uc := app.NewReconcileStaleOrdersUseCase(repo, registry, clock, metrics, logger)
	res, err := uc.Execute(context.Background(), 50)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.HealedCancelled != 1 {
		t.Errorf("expected 1 healed_cancelled, got %d", res.HealedCancelled)
	}

	// Verify order status CANCELLED
	ord, _ := repo.GetOrder(context.Background(), orderID)
	if ord.Status != "CANCELLED" {
		t.Errorf("expected CANCELLED, got %s", ord.Status)
	}

	// Verify payment status FAILED
	pay, _ := repo.GetPayment(context.Background(), "pay-12")
	if pay.Status != domain.StatusFailed {
		t.Errorf("expected payment FAILED, got %s", pay.Status)
	}

	// Verify outbox hold.released emitted
	outbox := repo.GetOutboxEvents()
	foundHoldReleased := false
	for _, ev := range outbox {
		if ev["routing_key"] == "hold.released" && ev["aggregate_id"] == orderID {
			foundHoldReleased = true
			break
		}
	}
	if !foundHoldReleased {
		t.Errorf("expected hold.released outbox event")
	}

	if metrics.getOutcomeCount("healed_cancelled") != 1 {
		t.Errorf("expected metric healed_cancelled = 1, got %d", metrics.getOutcomeCount("healed_cancelled"))
	}
}

// Đơn treo quá 15 phút không có thông tin payment -> Tự chữa sang CANCELLED
func TestReconcileOrders_HealCancelled_NoPayment(t *testing.T) {
	now := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	clock := fixedClock{currentTime: now}
	repo, registry, _, metrics, logger := setupReconcileTestEnv(clock)

	orderID := "ord-no-payment"
	repo.SeedOrder(domain.OrderView{
		ID:         orderID,
		EventID:    "evt-13",
		IdentityID: "user-13",
		Status:     "PAYMENT_PENDING",
		TotalCents: 100000,
		Currency:   "VND",
		CreatedAt:  now.Add(-20 * time.Minute),
		UpdatedAt:  now.Add(-18 * time.Minute),
	})

	uc := app.NewReconcileStaleOrdersUseCase(repo, registry, clock, metrics, logger)
	res, err := uc.Execute(context.Background(), 50)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.HealedCancelled != 1 {
		t.Errorf("expected 1 healed_cancelled, got %d", res.HealedCancelled)
	}

	ord, _ := repo.GetOrder(context.Background(), orderID)
	if ord.Status != "CANCELLED" {
		t.Errorf("expected CANCELLED, got %s", ord.Status)
	}
}

// AC-5: Cổng báo giao dịch vẫn PENDING -> Không hủy, giữ nguyên cho chu kỳ sau
func TestReconcileOrders_StillPending_NoAction(t *testing.T) {
	now := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	clock := fixedClock{currentTime: now}
	repo, registry, sbxGW, metrics, logger := setupReconcileTestEnv(clock)

	orderID := "ord-still-pending"
	repo.SeedOrder(domain.OrderView{
		ID:         orderID,
		EventID:    "evt-14",
		IdentityID: "user-14",
		Status:     "PAYMENT_PENDING",
		TotalCents: 200000,
		Currency:   "VND",
		CreatedAt:  now.Add(-17 * time.Minute),
		UpdatedAt:  now.Add(-16 * time.Minute),
	})

	repo.SeedPayment(domain.Payment{
		ID:            "pay-14",
		OrderID:       orderID,
		Provider:      domain.ProviderSandbox,
		ProviderTxnID: "sbx_txn_pending_14",
		AmountCents:   200000,
		Currency:      "VND",
		Status:        domain.StatusPending,
		CreatedAt:     now.Add(-16 * time.Minute),
	})

	sbxGW.SetQueryStatus(orderID, domain.StatusPending)

	uc := app.NewReconcileStaleOrdersUseCase(repo, registry, clock, metrics, logger)
	res, err := uc.Execute(context.Background(), 50)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.StillPending != 1 {
		t.Errorf("expected 1 still_pending, got %d", res.StillPending)
	}
	if res.HealedCancelled != 0 || res.HealedPaid != 0 {
		t.Errorf("expected no changes to order state, got %+v", res)
	}

	// Order status remains PAYMENT_PENDING
	ord, _ := repo.GetOrder(context.Background(), orderID)
	if ord.Status != "PAYMENT_PENDING" {
		t.Errorf("expected PAYMENT_PENDING, got %s", ord.Status)
	}

	if metrics.getOutcomeCount("still_pending") != 1 {
		t.Errorf("expected metric still_pending = 1, got %d", metrics.getOutcomeCount("still_pending"))
	}
}

// AC-5: Lỗi kết nối cổng tạm thời -> Skip an toàn, không báo lỗi làm gián đoạn
type failingGateway struct {
	port.PaymentGateway
}

func (f *failingGateway) QueryPayment(_ context.Context, _ domain.PaymentQueryRequest) (domain.PaymentQueryResult, error) {
	return domain.PaymentQueryResult{}, errors.New("timeout network to gateway")
}

func TestReconcileOrders_NetworkError_RetryLater(t *testing.T) {
	repo := postgres.NewMemoryRepository()
	registry := gatewayadapter.NewRegistry(domain.ProviderSandbox, []string{"sandbox"})

	sbxGW := gatewayadapter.NewSandboxGateway(gatewayadapter.SandboxModeSuccess)
	failingGW := &failingGateway{PaymentGateway: sbxGW}
	registry.Register(failingGW)

	metrics := newMockMetrics()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	now := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	clock := fixedClock{currentTime: now}
	repo.SetClock(func() time.Time { return clock.currentTime })

	orderID := "ord-net-err"
	repo.SeedOrder(domain.OrderView{
		ID:         orderID,
		EventID:    "evt-15",
		IdentityID: "user-15",
		Status:     "PAYMENT_PENDING",
		TotalCents: 200000,
		Currency:   "VND",
		CreatedAt:  now.Add(-20 * time.Minute),
		UpdatedAt:  now.Add(-18 * time.Minute),
	})

	repo.SeedPayment(domain.Payment{
		ID:            "pay-15",
		OrderID:       orderID,
		Provider:      domain.ProviderSandbox,
		ProviderTxnID: "sbx_txn_net_15",
		AmountCents:   200000,
		Currency:      "VND",
		Status:        domain.StatusPending,
		CreatedAt:     now.Add(-18 * time.Minute),
	})

	uc := app.NewReconcileStaleOrdersUseCase(repo, registry, clock, metrics, logger)
	res, err := uc.Execute(context.Background(), 50)
	if err != nil {
		t.Fatalf("unexpected error on network timeout: %v", err)
	}

	if res.Skipped != 1 {
		t.Errorf("expected 1 skipped on network error, got %d", res.Skipped)
	}

	// Order is kept in PAYMENT_PENDING
	ord, _ := repo.GetOrder(context.Background(), orderID)
	if ord.Status != "PAYMENT_PENDING" {
		t.Errorf("expected PAYMENT_PENDING, got %s", ord.Status)
	}
}

// AC-6: Export metrics đầy đủ
func TestReconcileOrders_ExportMetrics(t *testing.T) {
	now := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	clock := fixedClock{currentTime: now}
	repo, registry, _, metrics, logger := setupReconcileTestEnv(clock)

	uc := app.NewReconcileStaleOrdersUseCase(repo, registry, clock, metrics, logger)
	_, err := uc.Execute(context.Background(), 50)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if len(metrics.durRecords) != 1 {
		t.Errorf("expected duration metric to be recorded, got %d", len(metrics.durRecords))
	}
}
