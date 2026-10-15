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
