package gateway_test

import (
	"context"
	"strings"
	"testing"

	"github.com/eventflow/eventflow/services/payment/internal/adapter/gateway"
	"github.com/eventflow/eventflow/services/payment/internal/domain"
	"github.com/eventflow/eventflow/services/payment/internal/port"
)

func TestSandboxGateway_AutoSuccess(t *testing.T) {
	gw := gateway.NewSandboxGateway(gateway.SandboxModeSuccess)
	if gw.Provider() != domain.ProviderSandbox {
		t.Fatalf("expected provider sandbox, got %s", gw.Provider())
	}

	req := port.PaymentRequest{
		EventID:     "ev-1",
		OrderID:     "ord-1",
		HoldID:      "hold-1",
		PaymentID:   "pay-1",
		AmountCents: 200000,
		Currency:    "VND",
		ReturnURL:   "/checkout/ev-1/result?orderId=ord-1",
	}

	res, err := gw.InitiatePayment(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.Status != "PENDING" {
		t.Errorf("expected status PENDING, got %s", res.Status)
	}
	if !strings.Contains(res.RedirectURL, "status=SUCCESS") {
		t.Errorf("expected redirectURL to contain status=SUCCESS, got %s", res.RedirectURL)
	}
	if !strings.HasPrefix(res.ProviderTxnID, "sbx_txn_") {
		t.Errorf("expected providerTxnID prefix sbx_txn_, got %s", res.ProviderTxnID)
	}

	// Verify callback
	cb, err := gw.VerifyCallback(context.Background(), map[string]string{
		"provider_txn_id": res.ProviderTxnID,
		"status":          "SUCCESS",
	})
	if err != nil {
		t.Fatalf("verify callback failed: %v", err)
	}
	if !cb.Valid || cb.Status != domain.StatusSucceeded {
		t.Errorf("expected valid succeeded callback, got valid=%v status=%s", cb.Valid, cb.Status)
	}
}

func TestSandboxGateway_AutoFail(t *testing.T) {
	gw := gateway.NewSandboxGateway(gateway.SandboxModeFail)

	req := port.PaymentRequest{
		EventID:   "ev-1",
		OrderID:   "ord-1",
		PaymentID: "pay-fail",
	}

	_, err := gw.InitiatePayment(context.Background(), req)
	if err == nil {
		t.Fatalf("expected error on mode fail, got nil")
	}

	// Verify callback fail
	cb, err := gw.VerifyCallback(context.Background(), map[string]string{
		"provider_txn_id": "sbx_fail_123",
		"status":          "FAILED",
	})
	if err != nil {
		t.Fatalf("verify callback failed: %v", err)
	}
	if !cb.Valid || cb.Status != domain.StatusFailed {
		t.Errorf("expected valid failed callback, got valid=%v status=%s", cb.Valid, cb.Status)
	}
}

func TestSandboxGateway_SimulateDelay(t *testing.T) {
	gw := gateway.NewSandboxGateway(gateway.SandboxModeDelay)

	req := port.PaymentRequest{
		EventID:   "ev-1",
		OrderID:   "ord-1",
		PaymentID: "pay-delay",
	}

	res, err := gw.InitiatePayment(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.Status != "PROCESSING" {
		t.Errorf("expected status PROCESSING for delay mode, got %s", res.Status)
	}
	if !strings.Contains(res.RedirectURL, "simulated_delay=true") {
		t.Errorf("expected redirectURL to contain simulated_delay=true, got %s", res.RedirectURL)
	}
}
