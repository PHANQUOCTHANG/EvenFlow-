package http

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/eventflow/eventflow/services/notification/internal/adapter/sender"
	"github.com/eventflow/eventflow/services/notification/internal/adapter/store"
	"github.com/eventflow/eventflow/services/notification/internal/adapter/template"
	"github.com/eventflow/eventflow/services/notification/internal/app"
	"github.com/eventflow/eventflow/services/notification/internal/domain"
)

func setupHTTPTest() (*NotificationHandler, *http.ServeMux, *store.MemoryNotificationStore) {
	emailSender := sender.NewMockEmailSender()
	smsSender := sender.NewMockSMSSender()
	renderer, _ := template.NewGoTemplateRenderer()
	memStore := store.NewMemoryNotificationStore()

	orderPaidUC := app.NewProcessOrderPaidUseCase(emailSender, smsSender, renderer, memStore, nil)
	ticketIssuedUC := app.NewProcessTicketIssuedUseCase(emailSender, smsSender, renderer, memStore, nil)
	refundUC := app.NewProcessRefundUseCase(emailSender, smsSender, renderer, memStore, nil)
	getHistoryUC := app.NewGetNotificationHistoryUseCase(memStore)

	handler := NewNotificationHandler(orderPaidUC, ticketIssuedUC, refundUC, getHistoryUC)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	return handler, mux, memStore
}

func TestHandler_Healthz(t *testing.T) {
	_, mux, _ := setupHTTPTest()

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rr.Code)
	}

	var body map[string]string
	_ = json.NewDecoder(rr.Body).Decode(&body)
	if body["status"] != "ok" {
		t.Errorf("expected status ok, got %v", body)
	}
}

func TestHandler_ManualSend_Success(t *testing.T) {
	_, mux, memStore := setupHTTPTest()

	payload := manualSendRequest{
		EventType:      "order.paid",
		OrderID:        "ord-http-1",
		IdentityID:     "usr-1",
		AmountCents:    500000,
		Provider:       "MOMO",
		RecipientEmail: "buyer@example.com",
		RecipientPhone: "0912345678",
	}
	body, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPost, "/v1/notifications/send", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d (body: %s)", rr.Code, rr.Body.String())
	}

	history, err := memStore.GetHistoryByOrderID(req.Context(), "ord-http-1")
	if err != nil || len(history) != 2 {
		t.Fatalf("expected 2 notifications in store, got %d", len(history))
	}
}

func TestHandler_ManualSend_TicketIssued(t *testing.T) {
	_, mux, _ := setupHTTPTest()

	payload := manualSendRequest{
		EventType:      "ticket.issued",
		OrderID:        "ord-http-ticket",
		IdentityID:     "usr-1",
		RecipientEmail: "buyer@example.com",
		Tickets: []domain.TicketDetail{
			{ID: "t-1", JTI: "jti-1", QRSig: "sig-1"},
		},
	}
	body, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPost, "/v1/notifications/send", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d (body: %s)", rr.Code, rr.Body.String())
	}
}

func TestHandler_ManualSend_Refund(t *testing.T) {
	_, mux, _ := setupHTTPTest()

	payload := manualSendRequest{
		EventType:      "payment.refunded",
		OrderID:        "ord-http-refund",
		IdentityID:     "usr-1",
		AmountCents:    250000,
		Reason:         "Cancelled",
		RecipientEmail: "buyer@example.com",
	}
	body, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPost, "/v1/notifications/send", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rr.Code)
	}
}

func TestHandler_ManualSend_InvalidRequests(t *testing.T) {
	_, mux, _ := setupHTTPTest()

	// 1. Invalid JSON
	req1 := httptest.NewRequest(http.MethodPost, "/v1/notifications/send", bytes.NewReader([]byte("{invalid")))
	rr1 := httptest.NewRecorder()
	mux.ServeHTTP(rr1, req1)
	if rr1.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid JSON, got %d", rr1.Code)
	}

	// 2. Missing event_type or order_id
	req2 := httptest.NewRequest(http.MethodPost, "/v1/notifications/send", bytes.NewReader([]byte(`{"order_id":""}`)))
	rr2 := httptest.NewRecorder()
	mux.ServeHTTP(rr2, req2)
	if rr2.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for missing fields, got %d", rr2.Code)
	}

	// 3. Unsupported event_type
	req3 := httptest.NewRequest(http.MethodPost, "/v1/notifications/send", bytes.NewReader([]byte(`{"event_type":"unknown","order_id":"123"}`)))
	rr3 := httptest.NewRecorder()
	mux.ServeHTTP(rr3, req3)
	if rr3.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for unsupported event type, got %d", rr3.Code)
	}
}

func TestHandler_GetHistory_Found(t *testing.T) {
	_, mux, memStore := setupHTTPTest()

	notif := domain.Notification{
		ID:        "notif-hist-1",
		DedupKey:  "order.paid:ord-find:email",
		EventType: "order.paid",
		OrderID:   "ord-find",
		Recipient: "user@example.com",
		Channel:   domain.ChannelEmail,
		Status:    domain.StatusSent,
		CreatedAt: time.Now(),
	}
	_, _ = memStore.CheckOrSetDedup(t.Context(), notif.DedupKey, notif)

	req := httptest.NewRequest(http.MethodGet, "/v1/notifications/history/ord-find", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rr.Code)
	}

	var resp struct {
		OrderID       string                `json:"order_id"`
		Notifications []domain.Notification `json:"notifications"`
	}
	_ = json.NewDecoder(rr.Body).Decode(&resp)

	if resp.OrderID != "ord-find" || len(resp.Notifications) != 1 {
		t.Errorf("expected 1 notification in response, got %+v", resp)
	}
}
