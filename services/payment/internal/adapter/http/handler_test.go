package httpadapter_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/eventflow/eventflow/libs/go/httpx"
	gatewayadapter "github.com/eventflow/eventflow/services/payment/internal/adapter/gateway"
	httpadapter "github.com/eventflow/eventflow/services/payment/internal/adapter/http"
	"github.com/eventflow/eventflow/services/payment/internal/adapter/postgres"
	"github.com/eventflow/eventflow/services/payment/internal/app"
	"github.com/eventflow/eventflow/services/payment/internal/domain"
	"github.com/eventflow/eventflow/services/payment/internal/port"
)

type mockInitiator struct {
	fn func(ctx context.Context, in app.InitiatePaymentInput) (app.InitiatePaymentOutput, error)
}

func (m *mockInitiator) Execute(ctx context.Context, in app.InitiatePaymentInput) (app.InitiatePaymentOutput, error) {
	return m.fn(ctx, in)
}

type mockGetter struct {
	fn func(ctx context.Context, paymentID string) (domain.Payment, error)
}

func (m *mockGetter) Execute(ctx context.Context, paymentID string) (domain.Payment, error) {
	return m.fn(ctx, paymentID)
}

type mockLister struct {
	fn func(ctx context.Context) []domain.PaymentMethodInfo
}

func (m *mockLister) Execute(ctx context.Context) []domain.PaymentMethodInfo {
	return m.fn(ctx)
}

type mockProcessor struct {
	fn func(ctx context.Context, in app.ProcessWebhookInput) (domain.WebhookProcessResult, error)
}

func (m *mockProcessor) Execute(ctx context.Context, in app.ProcessWebhookInput) (domain.WebhookProcessResult, error) {
	if m.fn != nil {
		return m.fn(ctx, in)
	}
	return domain.WebhookProcessResult{}, nil
}

func setupTestServer(
	initiator *mockInitiator,
	getter *mockGetter,
	lister *mockLister,
	optionalProcessor ...*mockProcessor,
) *http.ServeMux {
	var processor *mockProcessor
	if len(optionalProcessor) > 0 {
		processor = optionalProcessor[0]
	}
	log := slog.New(slog.NewJSONHandler(io.Discard, nil))
	h := httpadapter.NewHandler(initiator, getter, lister, processor, log)

	mux := http.NewServeMux()
	// Middleware pass-through de test logic cua handler
	dummyMiddleware := func(next http.Handler) http.Handler {
		return next
	}
	h.Routes(mux, dummyMiddleware)
	return mux
}

func TestHandler_InitiatePayment_Success(t *testing.T) {
	initiator := &mockInitiator{
		fn: func(_ context.Context, in app.InitiatePaymentInput) (app.InitiatePaymentOutput, error) {
			return app.InitiatePaymentOutput{
				PaymentID:     "pay-123",
				OrderID:       in.OrderID,
				RedirectURL:   "https://eventflow.local/checkout/result?orderId=" + in.OrderID,
				Status:        "PENDING",
				ProviderTxnID: "sbx_txn_999",
			}, nil
		},
	}

	mux := setupTestServer(initiator, nil, nil)

	reqBody := httpadapter.InitiatePaymentRequest{
		OrderID:    "ord-1",
		HoldID:     "hold-1",
		Amount:     100000,
		Method:     "sandbox",
		BuyerEmail: "test@example.com",
		ReturnURL:  "https://eventflow.local/checkout/result",
	}
	raw, _ := json.Marshal(reqBody)

	req := httptest.NewRequest("POST", "/v1/events/ev-1/payments", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", "idem-uuid-1")

	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", w.Code, w.Body.String())
	}

	var resp httpadapter.InitiatePaymentResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid json response: %v", err)
	}

	if resp.PaymentID != "pay-123" {
		t.Errorf("expected payment_id pay-123, got %s", resp.PaymentID)
	}
	if resp.OrderID != "ord-1" {
		t.Errorf("expected order_id ord-1, got %s", resp.OrderID)
	}
}

func TestHandler_InitiatePayment_HoldExpired_409(t *testing.T) {
	initiator := &mockInitiator{
		fn: func(_ context.Context, _ app.InitiatePaymentInput) (app.InitiatePaymentOutput, error) {
			return app.InitiatePaymentOutput{}, domain.ErrHoldExpired
		},
	}

	mux := setupTestServer(initiator, nil, nil)

	reqBody := httpadapter.InitiatePaymentRequest{
		OrderID: "ord-expired",
		Amount:  100000,
		Method:  "sandbox",
	}
	raw, _ := json.Marshal(reqBody)

	req := httptest.NewRequest("POST", "/v1/events/ev-1/payments", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	// Phai tra ve 409 Conflict cho loi hold het han (BR-O2)
	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409 Conflict, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandler_InitiatePayment_AmountMismatch_422(t *testing.T) {
	initiator := &mockInitiator{
		fn: func(_ context.Context, _ app.InitiatePaymentInput) (app.InitiatePaymentOutput, error) {
			return app.InitiatePaymentOutput{}, domain.ErrAmountMismatch
		},
	}

	mux := setupTestServer(initiator, nil, nil)

	reqBody := httpadapter.InitiatePaymentRequest{
		OrderID: "ord-1",
		Amount:  50000,
	}
	raw, _ := json.Marshal(reqBody)

	req := httptest.NewRequest("POST", "/v1/events/ev-1/payments", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 Unprocessable Entity, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandler_InitiatePayment_Validation_400(t *testing.T) {
	initiator := &mockInitiator{
		fn: func(_ context.Context, _ app.InitiatePaymentInput) (app.InitiatePaymentOutput, error) {
			return app.InitiatePaymentOutput{}, domain.ErrInvalidInput
		},
	}

	mux := setupTestServer(initiator, nil, nil)

	// Bad JSON
	req := httptest.NewRequest("POST", "/v1/events/ev-1/payments", bytes.NewReader([]byte("{invalid-json")))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request, got %d", w.Code)
	}
}

func TestHandler_GetPayment(t *testing.T) {
	getter := &mockGetter{
		fn: func(_ context.Context, paymentID string) (domain.Payment, error) {
			if paymentID == "pay-found" {
				return domain.Payment{
					ID:            "pay-found",
					OrderID:       "ord-1",
					Provider:      domain.ProviderVNPay,
					ProviderTxnID: "vnp_123",
					AmountCents:   500000,
					Currency:      "VND",
					Status:        domain.StatusPending,
					CreatedAt:     time.Date(2026, 10, 15, 10, 0, 0, 0, time.UTC),
				}, nil
			}
			return domain.Payment{}, domain.ErrOrderNotFound
		},
	}

	mux := setupTestServer(nil, getter, nil)

	// Case 1: Found
	req := httptest.NewRequest("GET", "/v1/payments/pay-found", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", w.Code)
	}

	var resp httpadapter.PaymentDetailsResponse
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.PaymentID != "pay-found" || resp.Provider != "vnpay" {
		t.Errorf("unexpected response: %+v", resp)
	}

	// Case 2: Not found
	req404 := httptest.NewRequest("GET", "/v1/payments/pay-not-found", nil)
	w404 := httptest.NewRecorder()
	mux.ServeHTTP(w404, req404)
	if w404.Code != http.StatusNotFound {
		t.Errorf("expected 404 Not Found, got %d", w404.Code)
	}
}

func TestHandler_ListMethods(t *testing.T) {
	lister := &mockLister{
		fn: func(_ context.Context) []domain.PaymentMethodInfo {
			return []domain.PaymentMethodInfo{
				{Code: domain.ProviderVNPay, Name: "VNPay", Enabled: true},
				{Code: domain.ProviderMoMo, Name: "MoMo", Enabled: true},
			}
		},
	}

	mux := setupTestServer(nil, nil, lister)

	req := httptest.NewRequest("GET", "/v1/payments/methods", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", w.Code)
	}

	var resp httpadapter.PaymentMethodsResponse
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if len(resp.Methods) != 2 {
		t.Errorf("expected 2 methods, got %d", len(resp.Methods))
	}
}

func TestHandler_IdempotencyKey_Integration(t *testing.T) {
	// Kiem tra middleware Idempotency-Key (BR-O5)
	calls := 0
	initiator := &mockInitiator{
		fn: func(_ context.Context, in app.InitiatePaymentInput) (app.InitiatePaymentOutput, error) {
			calls++
			return app.InitiatePaymentOutput{
				PaymentID:     "pay-idem",
				OrderID:       in.OrderID,
				RedirectURL:   "https://eventflow.local/result",
				Status:        "PENDING",
				ProviderTxnID: "txn-idem",
			}, nil
		},
	}

	// Tao middleware idempotency gia lap (kiem tra co chan duplicate request cung key)
	cache := make(map[string][]byte)
	idemMiddleware := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := r.Header.Get("Idempotency-Key")
			if key == "" {
				httpx.WriteProblem(w, http.StatusBadRequest, "bat buoc Idempotency-Key")
				return
			}
			if cached, ok := cache[key]; ok {
				w.Header().Set("Content-Type", "application/json; charset=utf-8")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write(cached)
				return
			}
			rec := httptest.NewRecorder()
			next.ServeHTTP(rec, r)
			if rec.Code == http.StatusOK {
				cache[key] = rec.Body.Bytes()
			}
			for k, v := range rec.Header() {
				w.Header()[k] = v
			}
			w.WriteHeader(rec.Code)
			_, _ = w.Write(rec.Body.Bytes())
		})
	}

	log := slog.New(slog.NewJSONHandler(io.Discard, nil))
	h := httpadapter.NewHandler(initiator, nil, nil, nil, log)
	mux := http.NewServeMux()
	h.Routes(mux, idemMiddleware)

	body := []byte(`{"order_id":"ord-1","amount":100000,"method":"sandbox"}`)

	// Call 1
	req1 := httptest.NewRequest("POST", "/v1/events/ev-1/payments", bytes.NewReader(body))
	req1.Header.Set("Content-Type", "application/json")
	req1.Header.Set("Idempotency-Key", "test-key-100")
	w1 := httptest.NewRecorder()
	mux.ServeHTTP(w1, req1)

	if w1.Code != http.StatusOK {
		t.Fatalf("call 1 failed with status %d", w1.Code)
	}

	// Call 2 with identical key -> returns cached response without incrementing calls
	req2 := httptest.NewRequest("POST", "/v1/events/ev-1/payments", bytes.NewReader(body))
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("Idempotency-Key", "test-key-100")
	w2 := httptest.NewRecorder()
	mux.ServeHTTP(w2, req2)

	if w2.Code != http.StatusOK {
		t.Fatalf("call 2 failed with status %d", w2.Code)
	}
	if calls != 1 {
		t.Errorf("expected underlying handler called exactly 1 time, but called %d times (BR-O5 failed)", calls)
	}
}

func TestHandler_Webhook_Sandbox_Success(t *testing.T) {
	proc := &mockProcessor{
		fn: func(_ context.Context, in app.ProcessWebhookInput) (domain.WebhookProcessResult, error) {
			if in.Provider != domain.ProviderSandbox {
				t.Errorf("expected provider sandbox, got %s", in.Provider)
			}
			return domain.WebhookProcessResult{
				Success:       true,
				AlreadyDone:   false,
				OrderID:       "ord-1",
				PaymentID:     "pay-1",
				ProviderTxnID: "sbx_txn_1",
				Status:        domain.StatusSucceeded,
				Message:       "ok",
			}, nil
		},
	}

	mux := setupTestServer(nil, nil, nil, proc)

	body := []byte(`{"provider_txn_id":"sbx_txn_1","order_id":"ord-1","status":"SUCCESS"}`)
	req := httptest.NewRequest("POST", "/v1/payments/webhook/sandbox", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", w.Code, w.Body.String())
	}

	var resp httpadapter.WebhookResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid json response: %v", err)
	}
	if resp.OrderID != "ord-1" || resp.AlreadyProcessed {
		t.Errorf("unexpected response: %+v", resp)
	}
}

func TestHandler_Webhook_VNPay_GET_Success(t *testing.T) {
	proc := &mockProcessor{
		fn: func(_ context.Context, in app.ProcessWebhookInput) (domain.WebhookProcessResult, error) {
			if in.Provider != domain.ProviderVNPay {
				t.Errorf("expected provider vnpay, got %s", in.Provider)
			}
			return domain.WebhookProcessResult{
				Success:       true,
				AlreadyDone:   false,
				OrderID:       "ord-vnp",
				PaymentID:     "pay-vnp",
				ProviderTxnID: "vnp_123",
				Status:        domain.StatusSucceeded,
				Message:       "ok",
			}, nil
		},
	}

	mux := setupTestServer(nil, nil, nil, proc)

	req := httptest.NewRequest("GET", "/v1/payments/webhook/vnpay?vnp_TransactionNo=14000&vnp_OrderInfo=ord-vnp&vnp_ResponseCode=00", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for GET webhook, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandler_Webhook_Duplicate_200OK(t *testing.T) {
	proc := &mockProcessor{
		fn: func(_ context.Context, _ app.ProcessWebhookInput) (domain.WebhookProcessResult, error) {
			return domain.WebhookProcessResult{
				Success:       true,
				AlreadyDone:   true,
				OrderID:       "ord-dup",
				ProviderTxnID: "txn-dup",
				Status:        domain.StatusSucceeded,
				Message:       "already processed",
			}, nil
		},
	}

	mux := setupTestServer(nil, nil, nil, proc)

	req := httptest.NewRequest("POST", "/v1/payments/webhook/momo", bytes.NewReader([]byte(`{"orderId":"ord-dup"}`)))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for duplicate webhook, got %d", w.Code)
	}

	var resp httpadapter.WebhookResponse
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if !resp.AlreadyProcessed {
		t.Errorf("expected already_processed=true for duplicate webhook")
	}
}

func TestHandler_Webhook_InvalidSignature_400(t *testing.T) {
	proc := &mockProcessor{
		fn: func(_ context.Context, _ app.ProcessWebhookInput) (domain.WebhookProcessResult, error) {
			return domain.WebhookProcessResult{}, domain.ErrInvalidSignature
		},
	}

	mux := setupTestServer(nil, nil, nil, proc)

	req := httptest.NewRequest("POST", "/v1/payments/webhook/momo", bytes.NewReader([]byte(`{"signature":"invalid"}`)))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request on invalid signature, got %d", w.Code)
	}
}

func TestHandler_Webhook_ReplayExpired_400(t *testing.T) {
	proc := &mockProcessor{
		fn: func(_ context.Context, _ app.ProcessWebhookInput) (domain.WebhookProcessResult, error) {
			return domain.WebhookProcessResult{}, domain.ErrReplayDetected
		},
	}

	mux := setupTestServer(nil, nil, nil, proc)

	req := httptest.NewRequest("POST", "/v1/payments/webhook/stripe", bytes.NewReader([]byte(`{"id":"cs_old"}`)))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request on replay detected, got %d", w.Code)
	}
}

func TestHandler_Webhook_OrderNotFound_404(t *testing.T) {
	proc := &mockProcessor{
		fn: func(_ context.Context, _ app.ProcessWebhookInput) (domain.WebhookProcessResult, error) {
			return domain.WebhookProcessResult{}, domain.ErrOrderNotFound
		},
	}

	mux := setupTestServer(nil, nil, nil, proc)

	req := httptest.NewRequest("POST", "/v1/payments/webhook/sandbox", bytes.NewReader([]byte(`{"order_id":"ord-not-exist"}`)))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 Not Found, got %d", w.Code)
	}
}

func TestHandler_Webhook_ExactlyOnce_HTTP100Concurrent(t *testing.T) {
	repo := postgres.NewMemoryRepository()
	registry := gatewayadapter.NewRegistry(domain.ProviderSandbox, []string{"sandbox"})
	sbxGW := gatewayadapter.NewSandboxGateway(gatewayadapter.SandboxModeSuccess)
	registry.Register(sbxGW)

	now := time.Now().UTC()
	orderID := "ord-http-concurrent-100"
	repo.SeedOrder(domain.OrderView{
		ID:         orderID,
		EventID:    "evt-http-100",
		IdentityID: "user-100",
		Status:     "PAYMENT_PENDING",
		TotalCents: 200000,
		Currency:   "VND",
		ExpiresAt:  now.Add(10 * time.Minute),
	})

	log := slog.New(slog.NewJSONHandler(io.Discard, nil))
	uc := app.NewProcessWebhookUseCase(repo, registry, port.SystemClock{}, log)
	h := httpadapter.NewHandler(nil, nil, nil, uc, log)
	mux := http.NewServeMux()
	h.Routes(mux, nil)

	concurrency := 100
	var wg sync.WaitGroup
	wg.Add(concurrency)
	codes := make([]int, concurrency)

	body := []byte(fmt.Sprintf(`{"provider_txn_id":"txn_http_100","order_id":"%s","status":"SUCCESS","timestamp":"%s"}`, orderID, now.Format(time.RFC3339)))

	for i := 0; i < concurrency; i++ {
		idx := i
		go func() {
			defer wg.Done()
			req := httptest.NewRequest("POST", "/v1/payments/webhook/sandbox", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, req)
			codes[idx] = w.Code
		}()
	}

	wg.Wait()

	for _, code := range codes {
		if code != http.StatusOK {
			t.Fatalf("expected all HTTP 200, got %d", code)
		}
	}

	order, err := repo.GetOrder(context.Background(), orderID)
	if err != nil {
		t.Fatalf("failed to get order: %v", err)
	}
	if order.Status != "PAID" {
		t.Errorf("expected order status PAID, got %s", order.Status)
	}

	events := repo.GetOutboxEvents()
	if len(events) != 1 {
		t.Errorf("expected exactly 1 outbox event, got %d", len(events))
	}
}

type mockReplayer struct {
	executeFn func(ctx context.Context, limit int) (domain.ReplayInboxResult, error)
	staleFn   func(ctx context.Context, limit int) ([]domain.StaleInboxRecord, error)
}

func (m *mockReplayer) Execute(ctx context.Context, limit int) (domain.ReplayInboxResult, error) {
	if m.executeFn != nil {
		return m.executeFn(ctx, limit)
	}
	return domain.ReplayInboxResult{}, nil
}

func (m *mockReplayer) CheckStaleRecords(ctx context.Context, limit int) ([]domain.StaleInboxRecord, error) {
	if m.staleFn != nil {
		return m.staleFn(ctx, limit)
	}
	return nil, nil
}

// TestHandler_Webhook_OrderNotFound_StoresInInbox_200OK kiem chung AC-1 (BR-O6):
// Khi don chua ton tai, webhook tra ve HTTP 200 kem stored_in_inbox=true.
func TestHandler_Webhook_OrderNotFound_StoresInInbox_200OK(t *testing.T) {
	proc := &mockProcessor{
		fn: func(_ context.Context, in app.ProcessWebhookInput) (domain.WebhookProcessResult, error) {
			return domain.WebhookProcessResult{
				Success:       true,
				AlreadyDone:   false,
				StoredInInbox: true,
				OrderID:       in.Params["order_id"],
				ProviderTxnID: in.Params["provider_txn_id"],
				Status:        domain.StatusPending,
				Message:       "stored in inbox for delayed replay",
			}, nil
		},
	}

	mux := setupTestServer(nil, nil, nil, proc)

	body := []byte(`{"order_id":"ord-not-exist-yet","provider_txn_id":"txn-early-http"}`)
	req := httptest.NewRequest("POST", "/v1/payments/webhook/sandbox", bytes.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK when stored in inbox, got %d", w.Code)
	}

	var resp httpadapter.WebhookResponse
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if !resp.StoredInInbox {
		t.Errorf("expected stored_in_inbox=true in response")
	}
	if resp.Status != "ok" {
		t.Errorf("expected status 'ok', got %s", resp.Status)
	}
}

func TestHandler_ReplayInbox_Endpoint(t *testing.T) {
	log := slog.New(slog.NewJSONHandler(io.Discard, nil))
	replayer := &mockReplayer{
		executeFn: func(_ context.Context, limit int) (domain.ReplayInboxResult, error) {
			return domain.ReplayInboxResult{
				TotalScanned: 3,
				Succeeded:    2,
				StillPending: 1,
			}, nil
		},
	}

	h := httpadapter.NewHandler(nil, nil, nil, nil, log).WithInboxReplayer(replayer)
	mux := http.NewServeMux()
	h.Routes(mux, nil)

	req := httptest.NewRequest("POST", "/v1/payments/inbox/replay?limit=10", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for replay endpoint, got %d", w.Code)
	}

	var result domain.ReplayInboxResult
	_ = json.Unmarshal(w.Body.Bytes(), &result)
	if result.TotalScanned != 3 || result.Succeeded != 2 {
		t.Errorf("unexpected replay result: %+v", result)
	}
}

func TestHandler_StaleInbox_Endpoint(t *testing.T) {
	log := slog.New(slog.NewJSONHandler(io.Discard, nil))
	now := time.Now().UTC()
	replayer := &mockReplayer{
		staleFn: func(_ context.Context, limit int) ([]domain.StaleInboxRecord, error) {
			return []domain.StaleInboxRecord{
				{
					ID:            1,
					Provider:      domain.ProviderVNPay,
					ProviderTxnID: "vnp-stale-1",
					ReceivedAt:    now.Add(-20 * time.Minute),
					Age:           20 * time.Minute,
					Attempts:      2,
				},
			}, nil
		},
	}

	h := httpadapter.NewHandler(nil, nil, nil, nil, log).WithInboxReplayer(replayer)
	mux := http.NewServeMux()
	h.Routes(mux, nil)

	req := httptest.NewRequest("GET", "/v1/payments/inbox/stale", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for stale endpoint, got %d", w.Code)
	}

	var data map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &data)
	if count, ok := data["count"].(float64); !ok || count != 1 {
		t.Errorf("expected count=1 in stale response, got %v", data["count"])
	}
}

func TestHandler_ReplayInbox_Errors(t *testing.T) {
	log := slog.New(slog.NewJSONHandler(io.Discard, nil))
	replayer := &mockReplayer{
		executeFn: func(_ context.Context, _ int) (domain.ReplayInboxResult, error) {
			return domain.ReplayInboxResult{}, errors.New("db error")
		},
	}

	h := httpadapter.NewHandler(nil, nil, nil, nil, log).WithInboxReplayer(replayer)
	mux := http.NewServeMux()
	h.Routes(mux, nil)

	req := httptest.NewRequest("POST", "/v1/payments/inbox/replay?limit=invalid", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 on replay error, got %d", w.Code)
	}
}

func TestHandler_StaleInbox_Errors(t *testing.T) {
	log := slog.New(slog.NewJSONHandler(io.Discard, nil))
	replayer := &mockReplayer{
		staleFn: func(_ context.Context, _ int) ([]domain.StaleInboxRecord, error) {
			return nil, errors.New("db error")
		},
	}

	h := httpadapter.NewHandler(nil, nil, nil, nil, log).WithInboxReplayer(replayer)
	mux := http.NewServeMux()
	h.Routes(mux, nil)

	req := httptest.NewRequest("GET", "/v1/payments/inbox/stale?limit=invalid", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 on stale error, got %d", w.Code)
	}
}

func TestHandler_Webhook_ProcessorNil_501(t *testing.T) {
	log := slog.New(slog.NewJSONHandler(io.Discard, nil))
	h := httpadapter.NewHandler(nil, nil, nil, nil, log)
	mux := http.NewServeMux()
	h.Routes(mux, nil)

	req := httptest.NewRequest("POST", "/v1/payments/webhook/sandbox", bytes.NewReader([]byte(`{}`)))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotImplemented {
		t.Fatalf("expected 501 when processor is nil, got %d", w.Code)
	}
}

type mockReconciler struct {
	executeFn func(ctx context.Context, limit int) (domain.ReconciliationResult, error)
}

func (m *mockReconciler) Execute(ctx context.Context, limit int) (domain.ReconciliationResult, error) {
	if m.executeFn != nil {
		return m.executeFn(ctx, limit)
	}
	return domain.ReconciliationResult{}, nil
}

func TestHandler_ReconcileOrders_Success(t *testing.T) {
	log := slog.New(slog.NewJSONHandler(io.Discard, nil))
	reconciler := &mockReconciler{
		executeFn: func(_ context.Context, limit int) (domain.ReconciliationResult, error) {
			if limit != 25 {
				t.Errorf("expected limit 25, got %d", limit)
			}
			return domain.ReconciliationResult{
				TotalScanned:    5,
				HealedPaid:      3,
				HealedCancelled: 1,
				StillPending:    1,
				Duration:        50 * time.Millisecond,
			}, nil
		},
	}

	h := httpadapter.NewHandler(nil, nil, nil, nil, log).WithOrderReconciler(reconciler)
	mux := http.NewServeMux()
	h.Routes(mux, nil)

	// Test route /internal/reconcile
	req := httptest.NewRequest("POST", "/internal/reconcile?limit=25", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", w.Code, w.Body.String())
	}

	var res domain.ReconciliationResult
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if res.TotalScanned != 5 || res.HealedPaid != 3 || res.HealedCancelled != 1 {
		t.Errorf("unexpected reconcile result: %+v", res)
	}

	// Test route /v1/payments/reconcile
	req2 := httptest.NewRequest("POST", "/v1/payments/reconcile?limit=25", nil)
	w2 := httptest.NewRecorder()
	mux.ServeHTTP(w2, req2)

	if w2.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for /v1/payments/reconcile, got %d", w2.Code)
	}
}

func TestHandler_ReconcileOrders_Errors(t *testing.T) {
	log := slog.New(slog.NewJSONHandler(io.Discard, nil))
	reconciler := &mockReconciler{
		executeFn: func(_ context.Context, _ int) (domain.ReconciliationResult, error) {
			return domain.ReconciliationResult{}, errors.New("database connection lost")
		},
	}

	h := httpadapter.NewHandler(nil, nil, nil, nil, log).WithOrderReconciler(reconciler)
	mux := http.NewServeMux()
	h.Routes(mux, nil)

	req := httptest.NewRequest("POST", "/internal/reconcile?limit=invalid", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 on reconcile error, got %d", w.Code)
	}
}

type mockRefunder struct {
	executeFn func(ctx context.Context, in app.RefundCancelledEventInput) (app.RefundCancelledEventOutput, error)
}

func (m *mockRefunder) Execute(ctx context.Context, in app.RefundCancelledEventInput) (app.RefundCancelledEventOutput, error) {
	if m.executeFn != nil {
		return m.executeFn(ctx, in)
	}
	return app.RefundCancelledEventOutput{}, nil
}

func TestHandler_RefundEvent_Success(t *testing.T) {
	log := slog.New(slog.NewJSONHandler(io.Discard, nil))
	refunder := &mockRefunder{
		executeFn: func(_ context.Context, in app.RefundCancelledEventInput) (app.RefundCancelledEventOutput, error) {
			if in.EventID != "evt-test-123" {
				t.Errorf("expected EventID evt-test-123, got %s", in.EventID)
			}
			return app.RefundCancelledEventOutput{
				EventID:       in.EventID,
				TotalOrders:   3,
				RefundedCount: 3,
				TotalAmount:   750000,
			}, nil
		},
	}

	h := httpadapter.NewHandler(nil, nil, nil, nil, log).WithEventRefunder(refunder)
	mux := http.NewServeMux()
	h.Routes(mux, nil)

	// 1. POST /v1/events/{eventID}/refund
	body := bytes.NewReader([]byte(`{"reason":"Sự kiện bị hủy do bão","batch_size":20}`))
	req := httptest.NewRequest("POST", "/v1/events/evt-test-123/refund", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for refund, got %d: %s", w.Code, w.Body.String())
	}

	var res app.RefundCancelledEventOutput
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if res.RefundedCount != 3 || res.TotalAmount != 750000 {
		t.Errorf("unexpected refund output: %+v", res)
	}

	// 2. POST /internal/events/{eventID}/refund
	reqInternal := httptest.NewRequest("POST", "/internal/events/evt-test-123/refund", nil)
	wInternal := httptest.NewRecorder()
	mux.ServeHTTP(wInternal, reqInternal)

	if wInternal.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for internal refund endpoint, got %d", wInternal.Code)
	}
}

func TestHandler_RefundEvent_Errors(t *testing.T) {
	log := slog.New(slog.NewJSONHandler(io.Discard, nil))

	// Refunder not configured
	hNil := httpadapter.NewHandler(nil, nil, nil, nil, log)
	muxNil := http.NewServeMux()
	hNil.Routes(muxNil, nil)

	// Calling on non-registered returns 404 or not implemented if handler called directly
	// When refunder is configured but returns errors:
	refunder := &mockRefunder{
		executeFn: func(_ context.Context, in app.RefundCancelledEventInput) (app.RefundCancelledEventOutput, error) {
			if in.EventID == "evt-invalid" {
				return app.RefundCancelledEventOutput{}, domain.ErrInvalidInput
			}
			return app.RefundCancelledEventOutput{}, errors.New("database failure")
		},
	}

	h := httpadapter.NewHandler(nil, nil, nil, nil, log).WithEventRefunder(refunder)
	mux := http.NewServeMux()
	h.Routes(mux, nil)

	// 400 Bad Request
	reqInvalid := httptest.NewRequest("POST", "/v1/events/evt-invalid/refund", nil)
	wInvalid := httptest.NewRecorder()
	mux.ServeHTTP(wInvalid, reqInvalid)
	if wInvalid.Code != http.StatusBadRequest {
		t.Errorf("expected 400 on invalid input, got %d", wInvalid.Code)
	}

	// 500 Internal Server Error
	reqErr := httptest.NewRequest("POST", "/v1/events/evt-err/refund", nil)
	wErr := httptest.NewRecorder()
	mux.ServeHTTP(wErr, reqErr)
	if wErr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 on execution error, got %d", wErr.Code)
	}
}
