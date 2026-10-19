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

func TestStripeGateway_QueryPayment(t *testing.T) {
	gw := gateway.NewStripeGateway(gateway.StripeConfig{
		PublishableKey: "pk_test_123",
		SecretKey:      "sk_test_456",
		WebhookSecret:  "whsec_789",
	})

	res, err := gw.QueryPayment(context.Background(), domain.PaymentQueryRequest{
		OrderID:       "ord-stripe-1",
		ProviderTxnID: "cs_test_123",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Status != domain.StatusSucceeded {
		t.Errorf("expected StatusSucceeded, got %s", res.Status)
	}

	_, errEmpty := gw.QueryPayment(context.Background(), domain.PaymentQueryRequest{})
	if errEmpty == nil {
		t.Errorf("expected error for empty query")
	}
}

func TestStripeGateway_RefundPayment(t *testing.T) {
	gw := gateway.NewStripeGateway(gateway.StripeConfig{
		PublishableKey: "pk_test_123",
		SecretKey:      "sk_test_456",
		WebhookSecret:  "whsec_789",
	})

	ctx := context.Background()

	// 1. Valid refund
	req := domain.PaymentRefundRequest{
		PaymentID:     "pay-stripe-1",
		OrderID:       "ord-stripe-1",
		ProviderTxnID: "pi_123456",
		AmountCents:   450000,
		Currency:      "VND",
		Reason:        "Sự kiện bị hủy",
		RefundID:      "ref-stripe-1",
	}

	res, err := gw.RefundPayment(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Status != domain.StatusRefunded {
		t.Errorf("expected StatusRefunded, got %s", res.Status)
	}
	if res.AmountCents != 450000 {
		t.Errorf("expected amount 450000, got %d", res.AmountCents)
	}
	if res.RefundTxnID != "re_ref-stripe-1" {
		t.Errorf("expected refundTxnID re_ref-stripe-1, got %s", res.RefundTxnID)
	}

	// 2. Invalid amount <= 0
	reqInvalid := req
	reqInvalid.AmountCents = 0
	_, errInvalid := gw.RefundPayment(ctx, reqInvalid)
	if errInvalid == nil {
		t.Errorf("expected error for amount <= 0")
	}

	// 3. Missing order ID and provider txn ID
	reqMissing := req
	reqMissing.OrderID = ""
	reqMissing.ProviderTxnID = ""
	_, errMissing := gw.RefundPayment(ctx, reqMissing)
	if errMissing == nil {
		t.Errorf("expected error for missing identifiers")
	}
}
