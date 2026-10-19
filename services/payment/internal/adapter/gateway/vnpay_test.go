package gateway_test

import (
	"context"
	"net/url"
	"strings"
	"testing"

	"github.com/eventflow/eventflow/services/payment/internal/adapter/gateway"
	"github.com/eventflow/eventflow/services/payment/internal/domain"
	"github.com/eventflow/eventflow/services/payment/internal/port"
)

func TestVNPayGateway_InitiatePayment(t *testing.T) {
	cfg := gateway.VNPayConfig{
		TmnCode:    "TESTTMN",
		HashSecret: "SECRET123456",
		BaseURL:    "https://sandbox.vnpayment.vn/paymentv2/vpcpay.html",
		ReturnURL:  "https://eventflow.local/return",
	}
	gw := gateway.NewVNPayGateway(cfg)

	if gw.Provider() != domain.ProviderVNPay {
		t.Fatalf("expected provider vnpay, got %s", gw.Provider())
	}

	req := port.PaymentRequest{
		EventID:     "ev-music",
		OrderID:     "ord-123",
		PaymentID:   "pay-abc",
		AmountCents: 500000, // 500,000 VND
		Currency:    "VND",
		ClientIP:    "1.2.3.4",
		ReturnURL:   "https://eventflow.local/checkout/result",
	}

	res, err := gw.InitiatePayment(context.Background(), req)
	if err != nil {
		t.Fatalf("InitiatePayment failed: %v", err)
	}

	if res.Status != "PENDING" {
		t.Errorf("expected status PENDING, got %s", res.Status)
	}

	u, err := url.Parse(res.RedirectURL)
	if err != nil {
		t.Fatalf("invalid redirectURL: %v", err)
	}

	q := u.Query()
	if q.Get("vnp_TmnCode") != "TESTTMN" {
		t.Errorf("expected vnp_TmnCode TESTTMN, got %s", q.Get("vnp_TmnCode"))
	}
	// 500,000 * 100 = 50,000,000
	if q.Get("vnp_Amount") != "50000000" {
		t.Errorf("expected vnp_Amount 50000000, got %s", q.Get("vnp_Amount"))
	}
	if q.Get("vnp_TxnRef") != "pay-abc" {
		t.Errorf("expected vnp_TxnRef pay-abc, got %s", q.Get("vnp_TxnRef"))
	}
	if q.Get("vnp_SecureHash") == "" {
		t.Errorf("missing vnp_SecureHash")
	}

	// Verify that the hash in URL matches our recomputed hash
	incomingHash := q.Get("vnp_SecureHash")
	q.Del("vnp_SecureHash")
	signData := gateway.BuildVNPaySignData(q)
	expectedHash := gateway.ComputeHmacSHA512("SECRET123456", signData)
	if !strings.EqualFold(incomingHash, expectedHash) {
		t.Errorf("secure hash mismatch! incoming=%s, expected=%s", incomingHash, expectedHash)
	}
}

func TestVNPayGateway_VerifyCallback(t *testing.T) {
	cfg := gateway.VNPayConfig{
		TmnCode:    "TESTTMN",
		HashSecret: "SECRET123456",
	}
	gw := gateway.NewVNPayGateway(cfg)

	// Build valid mock callback query params
	params := url.Values{}
	params.Set("vnp_TmnCode", "TESTTMN")
	params.Set("vnp_Amount", "50000000")
	params.Set("vnp_BankCode", "NCB")
	params.Set("vnp_BankTranNo", "VNP12345")
	params.Set("vnp_CardType", "ATM")
	params.Set("vnp_OrderInfo", "Thanh toan ve")
	params.Set("vnp_PayDate", "20261015100000")
	params.Set("vnp_ResponseCode", "00")
	params.Set("vnp_TransactionNo", "14000123")
	params.Set("vnp_TransactionStatus", "00")
	params.Set("vnp_TxnRef", "pay-abc")

	signData := gateway.BuildVNPaySignData(params)
	secureHash := gateway.ComputeHmacSHA512("SECRET123456", signData)

	callbackMap := make(map[string]string)
	for k := range params {
		callbackMap[k] = params.Get(k)
	}
	callbackMap["vnp_SecureHash"] = secureHash

	// Case 1: Valid callback with responseCode 00
	res, err := gw.VerifyCallback(context.Background(), callbackMap)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Valid || res.Status != domain.StatusSucceeded {
		t.Errorf("expected valid succeeded callback, got valid=%v status=%s", res.Valid, res.Status)
	}
	if res.AmountCents != 500000 {
		t.Errorf("expected amount 500000, got %d", res.AmountCents)
	}
	if res.ProviderTxnID != "14000123" {
		t.Errorf("expected txnID 14000123, got %s", res.ProviderTxnID)
	}

	// Case 2: Tampered hash
	callbackMap["vnp_SecureHash"] = "tampered_fake_hash_value"
	resInvalid, err := gw.VerifyCallback(context.Background(), callbackMap)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resInvalid.Valid {
		t.Errorf("expected invalid result for tampered hash")
	}

	// Case 3: Failed transaction (vnp_ResponseCode = 24 / User cancel)
	params.Set("vnp_ResponseCode", "24")
	signData2 := gateway.BuildVNPaySignData(params)
	callbackMap2 := make(map[string]string)
	for k := range params {
		callbackMap2[k] = params.Get(k)
	}
	callbackMap2["vnp_SecureHash"] = gateway.ComputeHmacSHA512("SECRET123456", signData2)

	resFailed, err := gw.VerifyCallback(context.Background(), callbackMap2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resFailed.Valid || resFailed.Status != domain.StatusFailed {
		t.Errorf("expected valid signature but status FAILED, got valid=%v status=%s", resFailed.Valid, resFailed.Status)
	}
}

func TestVNPayGateway_QueryPayment(t *testing.T) {
	gw := gateway.NewVNPayGateway(gateway.VNPayConfig{
		TmnCode:    "TESTTMN",
		HashSecret: "SECRET123456",
	})

	// Valid query
	res, err := gw.QueryPayment(context.Background(), domain.PaymentQueryRequest{
		OrderID:       "ord-vnp-1",
		ProviderTxnID: "vnp-txn-123",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Status != domain.StatusSucceeded {
		t.Errorf("expected StatusSucceeded, got %s", res.Status)
	}

	// Invalid empty query
	_, errEmpty := gw.QueryPayment(context.Background(), domain.PaymentQueryRequest{})
	if errEmpty == nil {
		t.Errorf("expected error for empty query")
	}
}

func TestVNPayGateway_RefundPayment(t *testing.T) {
	gw := gateway.NewVNPayGateway(gateway.VNPayConfig{
		TmnCode:    "TESTTMN",
		HashSecret: "SECRET123456",
	})

	ctx := context.Background()

	// 1. Valid refund
	req := domain.PaymentRefundRequest{
		PaymentID:     "pay-vnp-1",
		OrderID:       "ord-vnp-1",
		ProviderTxnID: "vnp-txn-123",
		AmountCents:   500000,
		Currency:      "VND",
		Reason:        "Sự kiện bị hủy",
		RefundID:      "ref-vnp-1",
	}

	res, err := gw.RefundPayment(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Status != domain.StatusRefunded {
		t.Errorf("expected StatusRefunded, got %s", res.Status)
	}
	if res.AmountCents != 500000 {
		t.Errorf("expected amount 500000, got %d", res.AmountCents)
	}
	if !strings.HasPrefix(res.RefundTxnID, "vnp_ref_ord-vnp-1") {
		t.Errorf("expected refundTxnID to start with vnp_ref_ord-vnp-1, got %s", res.RefundTxnID)
	}

	// 2. Invalid amount <= 0
	reqInvalid := req
	reqInvalid.AmountCents = 0
	_, errInvalid := gw.RefundPayment(ctx, reqInvalid)
	if errInvalid == nil {
		t.Errorf("expected error for amount <= 0")
	}

	// 3. Missing order ID
	reqMissingOrder := req
	reqMissingOrder.OrderID = ""
	_, errMissing := gw.RefundPayment(ctx, reqMissingOrder)
	if errMissing == nil {
		t.Errorf("expected error for missing order_id")
	}
}
