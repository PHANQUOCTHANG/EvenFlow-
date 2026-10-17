package gateway_test

import (
	"context"
	"strings"
	"testing"

	"github.com/eventflow/eventflow/services/payment/internal/adapter/gateway"
	"github.com/eventflow/eventflow/services/payment/internal/domain"
	"github.com/eventflow/eventflow/services/payment/internal/port"
)

func TestStripeGateway_InitiatePayment(t *testing.T) {
	cfg := gateway.StripeConfig{
		PublishableKey: "pk_test_123",
		SecretKey:      "sk_test_123",
		SuccessURL:     "https://eventflow.local/checkout/result",
	}
	gw := gateway.NewStripeGateway(cfg)

	if gw.Provider() != domain.ProviderStripe {
		t.Fatalf("expected provider stripe, got %s", gw.Provider())
	}

	req := port.PaymentRequest{
		EventID:     "ev-intl",
		OrderID:     "ord-stripe",
		PaymentID:   "pay-stripe",
		AmountCents: 1000000,
		Currency:    "VND",
	}

	res, err := gw.InitiatePayment(context.Background(), req)
	if err != nil {
		t.Fatalf("InitiatePayment failed: %v", err)
	}

	if res.Status != "PENDING" {
		t.Errorf("expected status PENDING, got %s", res.Status)
	}
	if !strings.Contains(res.RedirectURL, "checkout.stripe.com/pay/cs_test_pay-stripe") {
		t.Errorf("unexpected redirectURL: %s", res.RedirectURL)
	}
	if res.ProviderTxnID != "cs_test_pay-stripe" {
		t.Errorf("expected providerTxnID cs_test_pay-stripe, got %s", res.ProviderTxnID)
	}
}

func TestStripeGateway_VerifyCallback(t *testing.T) {
	gw := gateway.NewStripeGateway(gateway.StripeConfig{})

	// Case 1: Paid session
	cb, err := gw.VerifyCallback(context.Background(), map[string]string{
		"session_id": "cs_test_123",
		"status":     "paid",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cb.Valid || cb.Status != domain.StatusSucceeded {
		t.Errorf("expected valid succeeded callback, got valid=%v status=%s", cb.Valid, cb.Status)
	}

	// Case 2: Missing session_id
	cbMissing, err := gw.VerifyCallback(context.Background(), map[string]string{
		"status": "paid",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cbMissing.Valid {
		t.Errorf("expected invalid result when session_id is missing")
	}

	// Case 3: Invalid signature
	cbInvalidSig, err := gw.VerifyCallback(context.Background(), map[string]string{
		"session_id": "cs_test_123",
		"status":     "paid",
		"signature":  "invalid",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cbInvalidSig.Valid {
		t.Errorf("expected invalid result when signature is invalid")
	}
}
