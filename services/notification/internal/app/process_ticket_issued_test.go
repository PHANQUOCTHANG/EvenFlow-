package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/eventflow/eventflow/services/notification/internal/adapter/sender"
	"github.com/eventflow/eventflow/services/notification/internal/adapter/store"
	"github.com/eventflow/eventflow/services/notification/internal/adapter/template"
	"github.com/eventflow/eventflow/services/notification/internal/domain"
)

func setupTicketIssuedTest() (*ProcessTicketIssuedUseCase, *sender.MockEmailSender, *sender.MockSMSSender, *store.MemoryNotificationStore) {
	emailSender := sender.NewMockEmailSender()
	smsSender := sender.NewMockSMSSender()
	renderer, _ := template.NewGoTemplateRenderer()
	memStore := store.NewMemoryNotificationStore()

	uc := NewProcessTicketIssuedUseCase(emailSender, smsSender, renderer, memStore, nil)
	return uc, emailSender, smsSender, memStore
}

func TestProcessTicketIssued_Success(t *testing.T) {
	ctx := context.Background()
	uc, emailSender, smsSender, memStore := setupTicketIssuedTest()

	evt := domain.TicketIssuedEvent{
		OrderID:     "ord-tickets-1",
		EventID:     "evt-gala",
		EventTitle:  "Gala Chào Xuân 2026",
		IdentityID:  "usr-99",
		TicketCount: 2,
		Tickets: []domain.TicketDetail{
			{ID: "t-1", JTI: "jti-1", QRSig: "sig-1"},
			{ID: "t-2", JTI: "jti-2", QRSig: "sig-2"},
		},
		RecipientEmail: "buyer@example.com",
		RecipientPhone: "0901234567",
		IssuedAt:       time.Now(),
	}

	res, err := uc.Execute(ctx, evt)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !res.EmailSent || !res.SMSSent {
		t.Errorf("expected both email and sms sent, got %+v", res)
	}

	// Kiem tra noi dung email co thong tin QR
	sentEmails := emailSender.GetSentEmails()
	if len(sentEmails) != 1 {
		t.Fatalf("expected 1 sent email, got %d", len(sentEmails))
	}
	if sentEmails[0].To != "buyer@example.com" {
		t.Errorf("expected recipient buyer@example.com")
	}

	sentSMS := smsSender.GetSentSMS()
	if len(sentSMS) != 1 {
		t.Fatalf("expected 1 sent sms, got %d", len(sentSMS))
	}
	if sentSMS[0].To != "0901234567" {
		t.Errorf("expected recipient 0901234567")
	}

	history, err := memStore.GetHistoryByOrderID(ctx, "ord-tickets-1")
	if err != nil || len(history) != 2 {
		t.Fatalf("expected 2 history records")
	}
}

func TestProcessTicketIssued_Idempotent(t *testing.T) {
	ctx := context.Background()
	uc, emailSender, smsSender, _ := setupTicketIssuedTest()

	evt := domain.TicketIssuedEvent{
		OrderID:        "ord-tickets-dup",
		IdentityID:     "usr-dup",
		RecipientEmail: "buyer@example.com",
		IssuedAt:       time.Now(),
	}

	_, _ = uc.Execute(ctx, evt)
	res2, err := uc.Execute(ctx, evt)
	if err != nil {
		t.Fatalf("run 2 unexpected error: %v", err)
	}

	if !res2.EmailSkipped {
		t.Errorf("expected email to be skipped on duplicate event")
	}
	if len(emailSender.GetSentEmails()) != 1 {
		t.Errorf("expected only 1 email sent, got %d", len(emailSender.GetSentEmails()))
	}
	if len(smsSender.GetSentSMS()) != 0 {
		t.Errorf("no phone was provided, should be 0 sms")
	}
}

func TestProcessTicketIssued_SenderFailure(t *testing.T) {
	ctx := context.Background()
	uc, emailSender, _, memStore := setupTicketIssuedTest()

	emailSender.SetSimulateError(errors.New("mail quota exceeded"))

	evt := domain.TicketIssuedEvent{
		OrderID:        "ord-fail-t",
		IdentityID:     "usr-fail",
		RecipientEmail: "fail@example.com",
		IssuedAt:       time.Now(),
	}

	_, err := uc.Execute(ctx, evt)
	if err == nil {
		t.Fatalf("expected error when email sender fails")
	}

	history, err := memStore.GetHistoryByOrderID(ctx, "ord-fail-t")
	if err != nil || len(history) != 1 || history[0].Status != domain.StatusFailed {
		t.Errorf("expected FAILED status in history")
	}
}
