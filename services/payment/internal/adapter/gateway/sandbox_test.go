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

func TestSandboxGateway_VerifyCallback_SignatureAndTimestamp(t *testing.T) {
	gw := gateway.NewSandboxGateway(gateway.SandboxModeSuccess)

	// Test invalid signature
	res, err := gw.VerifyCallback(context.Background(), map[string]string{
		"provider_txn_id": "sbx_123",
		"signature":       "invalid",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Valid {
		t.Errorf("expected invalid signature callback to return Valid=false")
	}

	// Test valid with timestamp and order_id
	ts := "2026-10-17T10:00:00Z"
	resOk, err := gw.VerifyCallback(context.Background(), map[string]string{
		"provider_txn_id": "sbx_123",
		"order_id":        "ord-abc",
		"payment_id":      "pay-xyz",
		"timestamp":       ts,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resOk.Valid || resOk.OrderID != "ord-abc" || resOk.PaymentID != "pay-xyz" {
		t.Errorf("unexpected callback result: %+v", resOk)
	}
	if resOk.Timestamp.IsZero() {
		t.Errorf("expected non-zero parsed timestamp")
	}
}

func TestSandboxGateway_QueryPayment(t *testing.T) {
	ctx := context.Background()

	// 1. Success mode
	gwSuccess := gateway.NewSandboxGateway(gateway.SandboxModeSuccess)
	res, err := gwSuccess.QueryPayment(ctx, domain.PaymentQueryRequest{
		OrderID:       "ord-1",
		ProviderTxnID: "sbx-txn-1",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Status != domain.StatusSucceeded {
		t.Errorf("expected StatusSucceeded, got %s", res.Status)
	}

	// 2. Fail mode
	gwFail := gateway.NewSandboxGateway(gateway.SandboxModeFail)
	resFail, err := gwFail.QueryPayment(ctx, domain.PaymentQueryRequest{
		OrderID:       "ord-2",
		ProviderTxnID: "sbx-txn-2",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resFail.Status != domain.StatusFailed {
		t.Errorf("expected StatusFailed, got %s", resFail.Status)
	}

	// 3. Delay mode
	gwDelay := gateway.NewSandboxGateway(gateway.SandboxModeDelay)
	resDelayPending, err := gwDelay.QueryPayment(ctx, domain.PaymentQueryRequest{
		OrderID:       "ord-still-pending",
		ProviderTxnID: "sbx-txn-3",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resDelayPending.Status != domain.StatusPending {
		t.Errorf("expected StatusPending for ord-still-pending, got %s", resDelayPending.Status)
	}

	// 4. Custom status override
	gwDelay.SetQueryStatus("ord-custom-fail", domain.StatusFailed)
	resCustom, err := gwDelay.QueryPayment(ctx, domain.PaymentQueryRequest{
		OrderID:       "ord-custom-fail",
		ProviderTxnID: "sbx-txn-4",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resCustom.Status != domain.StatusFailed {
		t.Errorf("expected StatusFailed for custom override, got %s", resCustom.Status)
	}
}

func TestSandboxGateway_RefundPayment(t *testing.T) {
	ctx := context.Background()

	// 1. Success mode
	gw := gateway.NewSandboxGateway(gateway.SandboxModeSuccess)
	req := domain.PaymentRefundRequest{
		PaymentID:     "pay-1",
		OrderID:       "ord-1",
		ProviderTxnID: "sbx-txn-1",
		AmountCents:   250000,
		Currency:      "VND",
		Reason:        "Sự kiện bị hủy",
		RefundID:      "ref-123",
	}

	res, err := gw.RefundPayment(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Status != domain.StatusRefunded {
		t.Errorf("expected StatusRefunded, got %s", res.Status)
	}
	if res.RefundTxnID != "sbx_ref_ref-123" {
		t.Errorf("expected refundTxnID sbx_ref_ref-123, got %s", res.RefundTxnID)
	}
	if res.AmountCents != 250000 {
		t.Errorf("expected amount 250000, got %d", res.AmountCents)
	}

	// 2. Fail mode
	gwFail := gateway.NewSandboxGateway(gateway.SandboxModeFail)
	_, errFail := gwFail.RefundPayment(ctx, req)
	if errFail == nil {
		t.Fatalf("expected error in fail mode, got nil")
	}

	// 3. Invalid input (amount <= 0)
	reqInvalid := req
	reqInvalid.AmountCents = 0
	_, errInvalid := gw.RefundPayment(ctx, reqInvalid)
	if errInvalid == nil {
		t.Fatalf("expected error for amount <= 0, got nil")
	}

	// 4. Custom status override
	gw.SetRefundStatus("ord-custom-fail", domain.StatusFailed)
	reqCustom := req
	reqCustom.OrderID = "ord-custom-fail"
	resCustom, err := gw.RefundPayment(ctx, reqCustom)
	if err != nil {
		t.Fatalf("unexpected error with custom status: %v", err)
	}
	if resCustom.Status != domain.StatusFailed {
		t.Errorf("expected StatusFailed for custom override, got %s", resCustom.Status)
	}
}
