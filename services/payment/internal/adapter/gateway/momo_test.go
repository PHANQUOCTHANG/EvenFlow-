package gateway_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/eventflow/eventflow/services/payment/internal/adapter/gateway"
	"github.com/eventflow/eventflow/services/payment/internal/domain"
	"github.com/eventflow/eventflow/services/payment/internal/port"
)

func TestMoMoGateway_InitiatePayment(t *testing.T) {
	cfg := gateway.MoMoConfig{
		PartnerCode: "MOMO_TEST",
		AccessKey:   "KEY123",
		SecretKey:   "SECRET123",
		Endpoint:    "https://test-payment.momo.vn/v2/gateway/api/create",
		ReturnURL:   "https://eventflow.local/return",
	}
	gw := gateway.NewMoMoGateway(cfg)

	if gw.Provider() != domain.ProviderMoMo {
		t.Fatalf("expected provider momo, got %s", gw.Provider())
	}

	req := port.PaymentRequest{
		EventID:     "ev-1",
		OrderID:     "ord-1",
		PaymentID:   "pay-1",
		AmountCents: 150000,
		Currency:    "VND",
	}

	res, err := gw.InitiatePayment(context.Background(), req)
	if err != nil {
		t.Fatalf("InitiatePayment failed: %v", err)
	}

	if res.Status != "PENDING" {
		t.Errorf("expected status PENDING, got %s", res.Status)
	}
	if !strings.Contains(res.RedirectURL, "signature=") {
		t.Errorf("expected redirectURL to contain signature, got %s", res.RedirectURL)
	}
	if res.ProviderTxnID != "momo_pay-1" {
		t.Errorf("expected providerTxnID momo_pay-1, got %s", res.ProviderTxnID)
	}
}

func TestMoMoGateway_VerifyCallback(t *testing.T) {
	cfg := gateway.MoMoConfig{
		PartnerCode: "MOMO_TEST",
		AccessKey:   "KEY123",
		SecretKey:   "SECRET123",
	}
	gw := gateway.NewMoMoGateway(cfg)

	raw := fmt.Sprintf(
		"accessKey=%s&amount=%s&extraData=%s&message=%s&orderId=%s&orderInfo=%s&orderType=%s&partnerCode=%s&payType=%s&requestId=%s&responseTime=%s&resultCode=%s&transId=%s",
		"KEY123", "150000", "", "Success", "ord-1", "Ve xem nhac", "momo_wallet", "MOMO_TEST", "qr", "req-1", "1700000000", "0", "trans_999",
	)
	sig := gateway.ComputeHmacSHA256("SECRET123", raw)

	params := map[string]string{
		"accessKey":    "KEY123",
		"amount":       "150000",
		"extraData":    "",
		"message":      "Success",
		"orderId":      "ord-1",
		"orderInfo":    "Ve xem nhac",
		"orderType":    "momo_wallet",
		"partnerCode":  "MOMO_TEST",
		"payType":      "qr",
		"requestId":    "req-1",
		"responseTime": "1700000000",
		"resultCode":   "0",
		"transId":      "trans_999",
		"signature":    sig,
	}

	// Case 1: Valid callback
	cb, err := gw.VerifyCallback(context.Background(), params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cb.Valid || cb.Status != domain.StatusSucceeded {
		t.Errorf("expected valid succeeded callback, got valid=%v status=%s", cb.Valid, cb.Status)
	}
	if cb.ProviderTxnID != "trans_999" {
		t.Errorf("expected transId trans_999, got %s", cb.ProviderTxnID)
	}

	// Case 2: Tampered signature
	params["signature"] = "tampered_sig"
	cbInvalid, err := gw.VerifyCallback(context.Background(), params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cbInvalid.Valid {
		t.Errorf("expected invalid result for tampered signature")
	}
}
