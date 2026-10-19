package app

import (
	"context"
	"testing"
	"time"

	"github.com/eventflow/eventflow/services/notification/internal/adapter/sender"
	"github.com/eventflow/eventflow/services/notification/internal/adapter/store"
	"github.com/eventflow/eventflow/services/notification/internal/adapter/template"
	"github.com/eventflow/eventflow/services/notification/internal/domain"
)

func TestProcessRefund_Success_And_Idempotent(t *testing.T) {
	ctx := context.Background()
	emailSender := sender.NewMockEmailSender()
	smsSender := sender.NewMockSMSSender()
	renderer, _ := template.NewGoTemplateRenderer()
	memStore := store.NewMemoryNotificationStore()

	uc := NewProcessRefundUseCase(emailSender, smsSender, renderer, memStore, nil)

	evt := domain.PaymentRefundedEvent{
		OrderID:        "ord-refund-1",
		EventID:        "evt-storm",
		IdentityID:     "usr-1",
		AmountCents:    1000000,
		Reason:         "Bão lũ, hủy sự kiện",
		RecipientEmail: "buyer@example.com",
		RecipientPhone: "0912345678",
		RefundedAt:     time.Now(),
	}

	// Lan 1
	res1, err := uc.Execute(ctx, evt)
	if err != nil {
		t.Fatalf("unexpected error on run 1: %v", err)
	}
	if !res1.EmailSent || !res1.SMSSent {
		t.Errorf("expected both sent on run 1")
	}

	// Lan 2 (retry / replay)
	res2, err := uc.Execute(ctx, evt)
	if err != nil {
		t.Fatalf("unexpected error on run 2: %v", err)
	}
	if !res2.EmailSkipped || !res2.SMSSkipped {
		t.Errorf("expected both skipped on run 2")
	}

	if len(emailSender.GetSentEmails()) != 1 {
		t.Errorf("expected 1 email, got %d", len(emailSender.GetSentEmails()))
	}
	if len(smsSender.GetSentSMS()) != 1 {
		t.Errorf("expected 1 sms, got %d", len(smsSender.GetSentSMS()))
	}
}
