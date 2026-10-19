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

func setupOrderPaidTest() (*ProcessOrderPaidUseCase, *sender.MockEmailSender, *sender.MockSMSSender, *store.MemoryNotificationStore) {
	emailSender := sender.NewMockEmailSender()
	smsSender := sender.NewMockSMSSender()
	renderer, _ := template.NewGoTemplateRenderer()
	memStore := store.NewMemoryNotificationStore()

	uc := NewProcessOrderPaidUseCase(emailSender, smsSender, renderer, memStore, nil)
	return uc, emailSender, smsSender, memStore
}

func TestProcessOrderPaid_Success_BothChannels(t *testing.T) {
	ctx := context.Background()
	uc, emailSender, smsSender, memStore := setupOrderPaidTest()

	evt := domain.OrderPaidEvent{
		OrderID:        "ord-1001",
		EventID:        "evt-music",
		EventTitle:     "Concert 2026",
		IdentityID:     "usr-1",
		AmountCents:    500000,
		Provider:       "VNPAY",
		ProviderTxnID:  "TXN-888",
		RecipientEmail: "customer@example.com",
		RecipientPhone: "0912345678",
		PaidAt:         time.Now(),
	}

	res, err := uc.Execute(ctx, evt)
	if err != nil {
		t.Fatalf("unexpected error executing usecase: %v", err)
	}

	if !res.EmailSent || !res.SMSSent {
		t.Errorf("expected both email and sms sent, got: %+v", res)
	}
	if len(emailSender.GetSentEmails()) != 1 {
		t.Errorf("expected 1 sent email, got %d", len(emailSender.GetSentEmails()))
	}
	if len(smsSender.GetSentSMS()) != 1 {
		t.Errorf("expected 1 sent sms, got %d", len(smsSender.GetSentSMS()))
	}

	history, err := memStore.GetHistoryByOrderID(ctx, "ord-1001")
	if err != nil || len(history) != 2 {
		t.Fatalf("expected 2 history records, got %d (err: %v)", len(history), err)
	}
}

func TestProcessOrderPaid_Idempotent_SkipDuplicate(t *testing.T) {
	ctx := context.Background()
	uc, emailSender, smsSender, _ := setupOrderPaidTest()

	evt := domain.OrderPaidEvent{
		OrderID:        "ord-dup",
		EventID:        "evt-dup",
		IdentityID:     "usr-dup",
		AmountCents:    200000,
		RecipientEmail: "dup@example.com",
		RecipientPhone: "0988776655",
		PaidAt:         time.Now(),
	}

	// Lan 1: Gui thanh cong
	res1, err := uc.Execute(ctx, evt)
	if err != nil {
		t.Fatalf("run 1 failed: %v", err)
	}
	if !res1.EmailSent || !res1.SMSSent {
		t.Errorf("run 1 should send both")
	}

	// Lan 2: Goi lai cung su kien (at-least-once message retry)
	res2, err := uc.Execute(ctx, evt)
	if err != nil {
		t.Fatalf("run 2 failed: %v", err)
	}
	if !res2.EmailSkipped || !res2.SMSSkipped {
		t.Errorf("run 2 should skip both due to deduplication, got %+v", res2)
	}

	// So luong email va sms gui di khong duoc tang them
	if len(emailSender.GetSentEmails()) != 1 {
		t.Errorf("expected still 1 email sent, got %d", len(emailSender.GetSentEmails()))
	}
	if len(smsSender.GetSentSMS()) != 1 {
		t.Errorf("expected still 1 sms sent, got %d", len(smsSender.GetSentSMS()))
	}
}

func TestProcessOrderPaid_SenderFailure_ReturnsError(t *testing.T) {
	ctx := context.Background()
	uc, emailSender, _, memStore := setupOrderPaidTest()

	simErr := errors.New("smtp connection refused")
	emailSender.SetSimulateError(simErr)

	evt := domain.OrderPaidEvent{
		OrderID:        "ord-fail",
		IdentityID:     "usr-fail",
		RecipientEmail: "fail@example.com",
		PaidAt:         time.Now(),
	}

	_, err := uc.Execute(ctx, evt)
	if err == nil {
		t.Fatalf("expected error when email sender fails")
	}

	history, err := memStore.GetHistoryByOrderID(ctx, "ord-fail")
	if err != nil || len(history) != 1 {
		t.Fatalf("expected 1 history record, got %d", len(history))
	}
	if history[0].Status != domain.StatusFailed {
		t.Errorf("expected status FAILED, got %s", history[0].Status)
	}
}

func TestProcessOrderPaid_InvalidPayload(t *testing.T) {
	ctx := context.Background()
	uc, _, _, _ := setupOrderPaidTest()

	evt := domain.OrderPaidEvent{
		OrderID: "", // Invalid empty
	}

	_, err := uc.Execute(ctx, evt)
	if !errors.Is(err, domain.ErrInvalidEventPayload) {
		t.Errorf("expected ErrInvalidEventPayload, got %v", err)
	}
}
