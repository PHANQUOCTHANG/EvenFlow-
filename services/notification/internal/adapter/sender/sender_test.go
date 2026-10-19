package sender

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/smtp"
	"testing"

	"github.com/eventflow/eventflow/services/notification/internal/domain"
)

func TestMockSender_SendEmail_RecordMessages(t *testing.T) {
	ctx := context.Background()
	s := NewMockEmailSender()

	msg := domain.EmailMessage{
		To:       "user@example.com",
		Subject:  "Test Subject",
		HTMLBody: "<p>Hello</p>",
	}

	if err := s.SendEmail(ctx, msg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	sent := s.GetSentEmails()
	if len(sent) != 1 {
		t.Fatalf("expected 1 sent email, got %d", len(sent))
	}
	if sent[0].To != "user@example.com" {
		t.Errorf("expected to user@example.com, got %s", sent[0].To)
	}

	// Test error simulation
	simErr := errors.New("simulated network failure")
	s.SetSimulateError(simErr)
	if err := s.SendEmail(ctx, msg); !errors.Is(err, simErr) {
		t.Errorf("expected simulated error %v, got %v", simErr, err)
	}

	s.Reset()
	if len(s.GetSentEmails()) != 0 {
		t.Errorf("expected 0 emails after reset")
	}
}

func TestMockSender_SendSMS_RecordMessages(t *testing.T) {
	ctx := context.Background()
	s := NewMockSMSSender()

	msg := domain.SMSMessage{
		To:          "0912345678",
		MessageBody: "Test SMS Content",
	}

	if err := s.SendSMS(ctx, msg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	sent := s.GetSentSMS()
	if len(sent) != 1 {
		t.Fatalf("expected 1 sent sms, got %d", len(sent))
	}
	if sent[0].To != "0912345678" {
		t.Errorf("expected to 0912345678, got %s", sent[0].To)
	}

	// Error simulation
	simErr := errors.New("sms gateway down")
	s.SetSimulateError(simErr)
	if err := s.SendSMS(ctx, msg); !errors.Is(err, simErr) {
		t.Errorf("expected simulated error, got %v", err)
	}

	s.Reset()
	if len(s.GetSentSMS()) != 0 {
		t.Errorf("expected 0 sms after reset")
	}
}

func TestSMTPSender_ConnectionValidation(t *testing.T) {
	ctx := context.Background()
	var calledAddr string
	var calledTo []string

	s := NewSMTPEmailSender("smtp.test.com", 587, "user", "pass", "no-reply@evenflow.vn")
	s.SendMailFn = func(addr string, a smtp.Auth, from string, to []string, msg []byte) error {
		calledAddr = addr
		calledTo = to
		return nil
	}

	msg := domain.EmailMessage{
		To:       "buyer@example.com",
		Subject:  "Test",
		HTMLBody: "<b>Content</b>",
	}

	err := s.SendEmail(ctx, msg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calledAddr != "smtp.test.com:587" {
		t.Errorf("expected addr smtp.test.com:587, got %s", calledAddr)
	}
	if len(calledTo) != 1 || calledTo[0] != "buyer@example.com" {
		t.Errorf("expected recipient buyer@example.com, got %v", calledTo)
	}

	// Validation error
	invalidMsg := domain.EmailMessage{To: "invalid-email", Subject: "Hi", HTMLBody: "text"}
	if err := s.SendEmail(ctx, invalidMsg); err == nil {
		t.Errorf("expected validation error for invalid recipient")
	}
}

func TestWebhookSMSSender_Dispatch(t *testing.T) {
	ctx := context.Background()
	var receivedBody string
	var authHeader string

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader = r.Header.Get("Authorization")
		buf := make([]byte, 1024)
		n, _ := r.Body.Read(buf)
		receivedBody = string(buf[:n])
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	s := NewWebhookSMSSender(ts.URL, "secret-token")
	s.Client = ts.Client()

	msg := domain.SMSMessage{
		To:          "0987654321",
		MessageBody: "Xin chao EvenFlow",
	}

	if err := s.SendSMS(ctx, msg); err != nil {
		t.Fatalf("unexpected error sending sms via webhook: %v", err)
	}

	if authHeader != "Bearer secret-token" {
		t.Errorf("expected Authorization header Bearer secret-token, got %s", authHeader)
	}
	if receivedBody == "" {
		t.Errorf("expected non-empty received body")
	}
}
