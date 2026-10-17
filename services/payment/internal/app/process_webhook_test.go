package app_test

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	gatewayadapter "github.com/eventflow/eventflow/services/payment/internal/adapter/gateway"
	"github.com/eventflow/eventflow/services/payment/internal/adapter/postgres"
	"github.com/eventflow/eventflow/services/payment/internal/app"
	"github.com/eventflow/eventflow/services/payment/internal/domain"
)

type fixedClock struct {
	currentTime time.Time
}

func (c fixedClock) Now() time.Time {
	return c.currentTime
}

func setupWebhookTestEnv() (*postgres.MemoryRepository, *gatewayadapter.Registry, *slog.Logger) {
	repo := postgres.NewMemoryRepository()
	registry := gatewayadapter.NewRegistry(domain.ProviderSandbox, []string{"sandbox", "vnpay", "momo", "stripe"})
	sbxGW := gatewayadapter.NewSandboxGateway(gatewayadapter.SandboxModeSuccess)
	registry.Register(sbxGW)

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return repo, registry, logger
}

// TestProcessWebhook_ExactlyOnce_Concurrent100 kiem chung tieu chi cot loi AC-1:
// Ban 100 request dong thoi cho cung 1 webhook -> dung 1 don sang PAID, dung 1 outbox event.
func TestProcessWebhook_ExactlyOnce_Concurrent100(t *testing.T) {
	repo, registry, logger := setupWebhookTestEnv()
	now := time.Date(2026, 10, 17, 10, 0, 0, 0, time.UTC)
	clock := fixedClock{currentTime: now}

	// Seed don hang o trang thai PAYMENT_PENDING
	orderID := "ord-concurrent-100"
	repo.SeedOrder(domain.OrderView{
		ID:         orderID,
		EventID:    "evt-100",
		IdentityID: "user-100",
		Status:     "PAYMENT_PENDING",
		TotalCents: 250000,
		Currency:   "VND",
		ExpiresAt:  now.Add(10 * time.Minute),
		CreatedAt:  now.Add(-2 * time.Minute),
	})

	uc := app.NewProcessWebhookUseCase(repo, registry, clock, logger)

	concurrency := 100
	var wg sync.WaitGroup
	wg.Add(concurrency)

	var successCount int64
	var alreadyDoneCount int64
	errChan := make(chan error, concurrency)

	txnID := "txn_concurrent_test_999"
	timestampStr := now.Format(time.RFC3339)

	for i := 0; i < concurrency; i++ {
		go func() {
			defer wg.Done()
			res, err := uc.Execute(context.Background(), app.ProcessWebhookInput{
				Provider: domain.ProviderSandbox,
				Params: map[string]string{
					"provider_txn_id": txnID,
					"order_id":        orderID,
					"status":          "SUCCESS",
					"timestamp":       timestampStr,
				},
				TraceID: "trace-concurrent",
			})
			if err != nil {
				errChan <- err
				return
			}
			if res.Success {
				atomic.AddInt64(&successCount, 1)
			}
			if res.AlreadyDone {
				atomic.AddInt64(&alreadyDoneCount, 1)
			}
		}()
	}

	wg.Wait()
	close(errChan)

	for err := range errChan {
		t.Fatalf("unexpected error during concurrent webhook processing: %v", err)
	}

	// Toan bo 100 request phai thanh cong (tra ve 200)
	if successCount != int64(concurrency) {
		t.Errorf("expected %d successes, got %d", concurrency, successCount)
	}

	// Dung 1 request la xu ly dau tien, 99 request con lai la AlreadyDone (idempotent no-op)
	if alreadyDoneCount != int64(concurrency-1) {
		t.Errorf("expected %d alreadyDone calls, got %d", concurrency-1, alreadyDoneCount)
	}

	// Kiem tra trang thai don hang sau 100 request
	order, err := repo.GetOrder(context.Background(), orderID)
	if err != nil {
		t.Fatalf("failed to get order: %v", err)
	}
	if order.Status != "PAID" {
		t.Errorf("expected order status PAID, got %s", order.Status)
	}

	// Kiem tra dung 1 outbox event duoc phat (order.paid)
	events := repo.GetOutboxEvents()
	if len(events) != 1 {
		t.Fatalf("expected exactly 1 outbox event, got %d", len(events))
	}
	if events[0]["routing_key"] != "order.paid" {
		t.Errorf("expected routing_key order.paid, got %v", events[0]["routing_key"])
	}
}

// TestProcessWebhook_InvalidSignature kiem chung AC-2: chu ky khong hop le bi tu choi.
func TestProcessWebhook_InvalidSignature(t *testing.T) {
	repo, registry, logger := setupWebhookTestEnv()
	now := time.Now().UTC()
	clock := fixedClock{currentTime: now}

	uc := app.NewProcessWebhookUseCase(repo, registry, clock, logger)

	res, err := uc.Execute(context.Background(), app.ProcessWebhookInput{
		Provider: domain.ProviderSandbox,
		Params: map[string]string{
			"provider_txn_id": "sbx_txn_invalid",
			"order_id":        "ord-1",
			"signature":       "invalid",
		},
		TraceID: "trace-invalid-sig",
	})

	if err != domain.ErrInvalidSignature {
		t.Fatalf("expected ErrInvalidSignature, got %v", err)
	}
	if res.Success {
		t.Errorf("expected res.Success == false")
	}
}

// TestProcessWebhook_ReplayExpired kiem chung AC-3: timestamp qua 5 phut bi tu choi.
func TestProcessWebhook_ReplayExpired(t *testing.T) {
	repo, registry, logger := setupWebhookTestEnv()
	now := time.Date(2026, 10, 17, 10, 10, 0, 0, time.UTC)
	clock := fixedClock{currentTime: now}

	uc := app.NewProcessWebhookUseCase(repo, registry, clock, logger)

	// Timestamp cach day 6 phut (360s > 300s window)
	expiredTime := now.Add(-6 * time.Minute).Format(time.RFC3339)

	res, err := uc.Execute(context.Background(), app.ProcessWebhookInput{
		Provider: domain.ProviderSandbox,
		Params: map[string]string{
			"provider_txn_id": "sbx_replay_txn",
			"order_id":        "ord-1",
			"timestamp":       expiredTime,
		},
		TraceID: "trace-replay",
	})

	if err != domain.ErrReplayDetected {
		t.Fatalf("expected ErrReplayDetected, got %v", err)
	}
	if res.Success {
		t.Errorf("expected res.Success == false")
	}
}

// TestProcessWebhook_ReplayFutureClockSkew kiem chung AC-3: timestamp lech qua 1 phut trong tuong lai bi tu choi.
func TestProcessWebhook_ReplayFutureClockSkew(t *testing.T) {
	repo, registry, logger := setupWebhookTestEnv()
	now := time.Date(2026, 10, 17, 10, 10, 0, 0, time.UTC)
	clock := fixedClock{currentTime: now}

	uc := app.NewProcessWebhookUseCase(repo, registry, clock, logger)

	// Timestamp o tuong lai 2 phut (120s > 60s max skew)
	futureTime := now.Add(2 * time.Minute).Format(time.RFC3339)

	_, err := uc.Execute(context.Background(), app.ProcessWebhookInput{
		Provider: domain.ProviderSandbox,
		Params: map[string]string{
			"provider_txn_id": "sbx_future_txn",
			"order_id":        "ord-1",
			"timestamp":       futureTime,
		},
		TraceID: "trace-future",
	})

	if err != domain.ErrReplayDetected {
		t.Fatalf("expected ErrReplayDetected, got %v", err)
	}
}

// TestProcessWebhook_PaymentFailed kiem chung AC-5: giao dich that bai tu cong thanh toan.
func TestProcessWebhook_PaymentFailed(t *testing.T) {
	repo, registry, logger := setupWebhookTestEnv()
	now := time.Now().UTC()
	clock := fixedClock{currentTime: now}

	orderID := "ord-failed-test"
	repo.SeedOrder(domain.OrderView{
		ID:         orderID,
		EventID:    "evt-1",
		IdentityID: "user-1",
		Status:     "PAYMENT_PENDING",
		TotalCents: 150000,
		Currency:   "VND",
		ExpiresAt:  now.Add(10 * time.Minute),
	})

	uc := app.NewProcessWebhookUseCase(repo, registry, clock, logger)

	res, err := uc.Execute(context.Background(), app.ProcessWebhookInput{
		Provider: domain.ProviderSandbox,
		Params: map[string]string{
			"provider_txn_id": "sbx_fail_txn",
			"order_id":        orderID,
			"status":          "FAILED",
			"timestamp":       now.Format(time.RFC3339),
		},
		TraceID: "trace-failed",
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Status != domain.StatusFailed {
		t.Errorf("expected status FAILED, got %s", res.Status)
	}

	// Don hang khong duoc sang PAID
	order, _ := repo.GetOrder(context.Background(), orderID)
	if order.Status == "PAID" {
		t.Errorf("failed payment should NOT mark order as PAID")
	}

	// Khong co su kien order.paid trong outbox
	events := repo.GetOutboxEvents()
	if len(events) != 0 {
		t.Errorf("expected 0 outbox events on failure, got %d", len(events))
	}
}

// TestProcessWebhook_InputValidation kiem tra validation dau vao.
func TestProcessWebhook_InputValidation(t *testing.T) {
	repo, registry, logger := setupWebhookTestEnv()
	uc := app.NewProcessWebhookUseCase(repo, registry, fixedClock{currentTime: time.Now().UTC()}, logger)

	// Unsupported provider
	_, err := uc.Execute(context.Background(), app.ProcessWebhookInput{
		Provider: domain.Provider("unknown_gateway"),
	})
	if err != domain.ErrUnsupportedProvider {
		t.Errorf("expected ErrUnsupportedProvider, got %v", err)
	}

	// Missing order_id
	_, err = uc.Execute(context.Background(), app.ProcessWebhookInput{
		Provider: domain.ProviderSandbox,
		Params: map[string]string{
			"provider_txn_id": "txn_no_order",
		},
	})
	if err == nil {
		t.Errorf("expected error when order_id is missing")
	}
}
