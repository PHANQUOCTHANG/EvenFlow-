package domain_test

import (
	"testing"
	"time"

	"github.com/eventflow/eventflow/services/payment/internal/domain"
)

func TestProvider_NormalizeAndValidate(t *testing.T) {
	tests := []struct {
		input    string
		expected domain.Provider
		valid    bool
	}{
		{"vnpay", domain.ProviderVNPay, true},
		{"VNPAY", domain.ProviderVNPay, true},
		{" momo ", domain.ProviderMoMo, true},
		{"stripe", domain.ProviderStripe, true},
		{"sandbox", domain.ProviderSandbox, true},
		{"unknown", domain.Provider("unknown"), false},
		{"", domain.Provider(""), false},
	}

	for _, tc := range tests {
		normalized := domain.NormalizeProvider(tc.input)
		if normalized != tc.expected {
			t.Errorf("NormalizeProvider(%q) = %q; want %q", tc.input, normalized, tc.expected)
		}
		if valid := domain.IsValidProvider(normalized); valid != tc.valid {
			t.Errorf("IsValidProvider(%q) = %v; want %v", normalized, valid, tc.valid)
		}
	}
}

func TestPaymentStatus_IsTerminal(t *testing.T) {
	if domain.StatusPending.IsTerminal() {
		t.Errorf("StatusPending should not be terminal")
	}
	if !domain.StatusSucceeded.IsTerminal() {
		t.Errorf("StatusSucceeded should be terminal")
	}
	if !domain.StatusFailed.IsTerminal() {
		t.Errorf("StatusFailed should be terminal")
	}
	if !domain.StatusRefunded.IsTerminal() {
		t.Errorf("StatusRefunded should be terminal")
	}
}

func TestOrderView_HoldAndExpiry(t *testing.T) {
	now := time.Date(2026, 10, 15, 10, 0, 0, 0, time.UTC)

	heldOrder := domain.OrderView{
		ID:        "order-1",
		Status:    "HELD",
		ExpiresAt: now.Add(5 * time.Minute),
	}

	if !heldOrder.IsHeld() {
		t.Errorf("heldOrder should be held")
	}
	if heldOrder.IsExpired(now) {
		t.Errorf("heldOrder should not be expired before expires_at")
	}

	// Dung thoi diem expires_at -> tinh la expired
	if !heldOrder.IsExpired(heldOrder.ExpiresAt) {
		t.Errorf("order at exact expires_at should be considered expired")
	}

	// Qua thoi diem expires_at -> expired
	if !heldOrder.IsExpired(now.Add(10 * time.Minute)) {
		t.Errorf("order after expires_at should be considered expired")
	}

	pendingOrder := domain.OrderView{
		ID:        "order-2",
		Status:    "PAYMENT_PENDING",
		ExpiresAt: now.Add(5 * time.Minute),
	}
	if pendingOrder.IsHeld() {
		t.Errorf("PAYMENT_PENDING order should not report IsHeld() == true")
	}
}

func TestMaskPII(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"buyer@gmail.com", "b***r@gmail.com"},
		{"a@domain.vn", "a***@domain.vn"},
		{"ab@domain.vn", "a***@domain.vn"},
		{"0912345678", "091****678"},
		{"0987654321", "098****321"},
		{"", ""},
		{"short", "***"},
	}

	for _, tc := range tests {
		masked := domain.MaskPII(tc.input)
		if masked != tc.expected {
			t.Errorf("MaskPII(%q) = %q; want %q", tc.input, masked, tc.expected)
		}
	}
}

func TestValidationError(t *testing.T) {
	err := &domain.ValidationError{Field: "amount", Message: "phải lớn hơn 0"}
	if err.Error() != "trường amount: phải lớn hơn 0" {
		t.Errorf("unexpected error message: %s", err.Error())
	}
}

func TestWebhookDomainErrorsAndStructs(t *testing.T) {
	if domain.ErrInvalidSignature == nil {
		t.Fatal("ErrInvalidSignature should not be nil")
	}
	if domain.ErrReplayDetected == nil {
		t.Fatal("ErrReplayDetected should not be nil")
	}
	if domain.ErrDuplicateWebhook == nil {
		t.Fatal("ErrDuplicateWebhook should not be nil")
	}

	res := domain.WebhookProcessResult{
		Success:       true,
		AlreadyDone:   false,
		OrderID:       "ord-1",
		PaymentID:     "pay-1",
		ProviderTxnID: "txn-1",
		Status:        domain.StatusSucceeded,
		Message:       "ok",
	}
	if !res.Success || res.OrderID != "ord-1" || res.Status != domain.StatusSucceeded {
		t.Errorf("unexpected WebhookProcessResult: %+v", res)
	}

	payload := domain.OutboxOrderPaidPayload{
		OrderID:       "ord-1",
		EventID:       "evt-1",
		IdentityID:    "user-1",
		PaymentID:     "pay-1",
		Provider:      "sandbox",
		ProviderTxnID: "txn-1",
		AmountCents:   100000,
		Currency:      "VND",
		PaidAt:        time.Now().UTC(),
	}
	if payload.AmountCents != 100000 || payload.Provider != "sandbox" {
		t.Errorf("unexpected OutboxOrderPaidPayload: %+v", payload)
	}
}

func TestPaymentInboxItem_Methods(t *testing.T) {
	now := time.Date(2026, 10, 17, 10, 0, 0, 0, time.UTC)
	item := domain.PaymentInboxItem{
		ID:            1,
		Provider:      domain.ProviderVNPay,
		ProviderTxnID: "vnp-12345",
		Payload:       map[string]any{"vnp_Amount": "10000000"},
		ReceivedAt:    now.Add(-20 * time.Minute),
		ProcessedAt:   nil,
		Attempts:      0,
	}

	if item.IsProcessed() {
		t.Errorf("item with nil ProcessedAt should not be processed")
	}

	if !item.IsStale(now, 15*time.Minute) {
		t.Errorf("item received 20m ago should be stale with 15m threshold")
	}

	if item.IsStale(now, 30*time.Minute) {
		t.Errorf("item received 20m ago should not be stale with 30m threshold")
	}

	processedTime := now.Add(-5 * time.Minute)
	item.ProcessedAt = &processedTime

	if !item.IsProcessed() {
		t.Errorf("item with non-nil ProcessedAt should be processed")
	}

	if item.IsStale(now, 15*time.Minute) {
		t.Errorf("processed item should never be stale")
	}
}

func TestReconciliationDomainStructs(t *testing.T) {
	now := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	stale := domain.StaleOrderInfo{
		OrderID:       "ord-stale-1",
		EventID:       "evt-1",
		IdentityID:    "user-1",
		Status:        "PAYMENT_PENDING",
		TotalCents:    500000,
		Currency:      "VND",
		ExpiresAt:     now.Add(-5 * time.Minute),
		UpdatedAt:     now.Add(-20 * time.Minute),
		CreatedAt:     now.Add(-25 * time.Minute),
		PaymentID:     "pay-1",
		Provider:      domain.ProviderSandbox,
		ProviderTxnID: "sbx-txn-1",
	}

	if stale.OrderID != "ord-stale-1" || stale.Provider != domain.ProviderSandbox {
		t.Errorf("unexpected StaleOrderInfo: %+v", stale)
	}

	qReq := domain.PaymentQueryRequest{
		OrderID:       "ord-stale-1",
		PaymentID:     "pay-1",
		ProviderTxnID: "sbx-txn-1",
	}
	if qReq.OrderID != "ord-stale-1" {
		t.Errorf("unexpected PaymentQueryRequest: %+v", qReq)
	}

	qRes := domain.PaymentQueryResult{
		ProviderTxnID: "sbx-txn-1",
		Status:        domain.StatusSucceeded,
		AmountCents:   500000,
		Message:       "thanh toan thanh cong",
	}
	if qRes.Status != domain.StatusSucceeded || qRes.AmountCents != 500000 {
		t.Errorf("unexpected PaymentQueryResult: %+v", qRes)
	}

	cParams := domain.CancelOrderParams{
		OrderID:   "ord-stale-1",
		TraceID:   "trace-1",
		Reason:    "qua 15 phut khong co webhook",
		PaymentID: "pay-1",
	}
	if cParams.Reason != "qua 15 phut khong co webhook" {
		t.Errorf("unexpected CancelOrderParams: %+v", cParams)
	}

	cRes := domain.CancelOrderResult{
		OrderID:         "ord-stale-1",
		AlreadyDone:     false,
		HoldsReleased:   1,
		TicketsReturned: 2,
		Status:          "CANCELLED",
		Message:         "huy thanh cong",
	}
	if cRes.HoldsReleased != 1 || cRes.TicketsReturned != 2 {
		t.Errorf("unexpected CancelOrderResult: %+v", cRes)
	}

	reconRes := domain.ReconciliationResult{
		TotalScanned:    10,
		HealedPaid:      5,
		HealedCancelled: 3,
		StillPending:    1,
		Skipped:         0,
		Errors:          1,
		Duration:        250 * time.Millisecond,
	}
	if reconRes.TotalScanned != 10 || reconRes.HealedPaid != 5 || reconRes.HealedCancelled != 3 {
		t.Errorf("unexpected ReconciliationResult: %+v", reconRes)
	}
}

func TestOrderView_IsRefundable(t *testing.T) {
	tests := []struct {
		status     string
		refundable bool
	}{
		{"PAID", true},
		{"ISSUED", true},
		{"REFUNDING", true},
		{"HELD", false},
		{"PAYMENT_PENDING", false},
		{"CANCELLED", false},
		{"EXPIRED", false},
		{"REFUNDED", false},
	}

	for _, tc := range tests {
		order := domain.OrderView{
			ID:     "ord-1",
			Status: tc.status,
		}
		if got := order.IsRefundable(); got != tc.refundable {
			t.Errorf("OrderView(status=%s).IsRefundable() = %v; want %v", tc.status, got, tc.refundable)
		}
	}
}

func TestRefundDomainStructs(t *testing.T) {
	now := time.Date(2026, 10, 19, 10, 0, 0, 0, time.UTC)

	req := domain.PaymentRefundRequest{
		PaymentID:     "pay-1",
		OrderID:       "ord-1",
		ProviderTxnID: "txn-1",
		AmountCents:   250000,
		Currency:      "VND",
		Reason:        "Sự kiện bị hủy",
		RefundID:      "ref-1",
	}
	if req.PaymentID != "pay-1" || req.AmountCents != 250000 {
		t.Errorf("unexpected PaymentRefundRequest: %+v", req)
	}

	res := domain.PaymentRefundResult{
		RefundTxnID: "ref-txn-1",
		Status:      domain.StatusRefunded,
		AmountCents: 250000,
		Message:     "hoàn tiền thành công",
	}
	if res.Status != domain.StatusRefunded || res.RefundTxnID != "ref-txn-1" {
		t.Errorf("unexpected PaymentRefundResult: %+v", res)
	}

	orderInfo := domain.RefundOrderInfo{
		OrderID:       "ord-1",
		EventID:       "evt-1",
		IdentityID:    "user-1",
		Status:        "PAID",
		TotalCents:    250000,
		Currency:      "VND",
		PaymentID:     "pay-1",
		Provider:      domain.ProviderSandbox,
		ProviderTxnID: "txn-1",
		CreatedAt:     now,
	}
	if orderInfo.OrderID != "ord-1" || orderInfo.Provider != domain.ProviderSandbox {
		t.Errorf("unexpected RefundOrderInfo: %+v", orderInfo)
	}

	pParams := domain.ProcessRefundParams{
		OrderID:     "ord-1",
		PaymentID:   "pay-1",
		Provider:    domain.ProviderSandbox,
		RefundTxnID: "ref-txn-1",
		AmountCents: 250000,
		Reason:      "Sự kiện bị hủy",
		TraceID:     "trace-ref-1",
		Timestamp:   now,
	}
	if pParams.RefundTxnID != "ref-txn-1" {
		t.Errorf("unexpected ProcessRefundParams: %+v", pParams)
	}

	pResult := domain.ProcessRefundResult{
		OrderID:         "ord-1",
		PaymentID:       "pay-1",
		AlreadyDone:     false,
		TicketsRefunded: 2,
		Status:          "REFUNDED",
		Message:         "hoàn tiền thành công",
	}
	if pResult.TicketsRefunded != 2 || pResult.Status != "REFUNDED" {
		t.Errorf("unexpected ProcessRefundResult: %+v", pResult)
	}

	outbox := domain.OutboxPaymentRefundedPayload{
		OrderID:     "ord-1",
		EventID:     "evt-1",
		IdentityID:  "user-1",
		PaymentID:   "pay-1",
		Provider:    "sandbox",
		RefundTxnID: "ref-txn-1",
		AmountCents: 250000,
		Currency:    "VND",
		Reason:      "Sự kiện bị hủy",
		RefundedAt:  now,
	}
	if outbox.OrderID != "ord-1" || outbox.RefundTxnID != "ref-txn-1" {
		t.Errorf("unexpected OutboxPaymentRefundedPayload: %+v", outbox)
	}

	batch := domain.RefundBatchSummary{
		EventID:       "evt-1",
		TotalOrders:   5,
		RefundedCount: 4,
		FailedCount:   0,
		SkippedCount:  1,
		TotalAmount:   1000000,
		Duration:      150 * time.Millisecond,
	}
	if batch.RefundedCount != 4 || batch.TotalAmount != 1000000 {
		t.Errorf("unexpected RefundBatchSummary: %+v", batch)
	}

	evtCancelled := domain.EventCancelledPayload{
		EventID:     "evt-1",
		CancelledAt: now,
		Reason:      "Lý do thời tiết",
	}
	if evtCancelled.EventID != "evt-1" || evtCancelled.Reason != "Lý do thời tiết" {
		t.Errorf("unexpected EventCancelledPayload: %+v", evtCancelled)
	}
}

func TestDomain_RefundStatus_RestoresQuota(t *testing.T) {
	// BR-O4: Đơn EXPIRED, CANCELLED, REFUNDED trả lại hạn mức cho khách
	// Kiểm tra tính nhất quán của StatusRefunded
	if !domain.StatusRefunded.IsTerminal() {
		t.Errorf("StatusRefunded must be a terminal status")
	}

	// Đảm bảo các lỗi domain mới được định nghĩa rõ ràng
	if domain.ErrOrderNotRefundable == nil {
		t.Errorf("ErrOrderNotRefundable should not be nil")
	}
	if domain.ErrRefundFailed == nil {
		t.Errorf("ErrRefundFailed should not be nil")
	}
	if domain.ErrAlreadyRefunded == nil {
		t.Errorf("ErrAlreadyRefunded should not be nil")
	}
}
