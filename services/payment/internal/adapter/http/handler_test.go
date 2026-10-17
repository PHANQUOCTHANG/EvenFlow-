package httpadapter_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/eventflow/eventflow/libs/go/httpx"
	httpadapter "github.com/eventflow/eventflow/services/payment/internal/adapter/http"
	"github.com/eventflow/eventflow/services/payment/internal/app"
	"github.com/eventflow/eventflow/services/payment/internal/domain"
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
