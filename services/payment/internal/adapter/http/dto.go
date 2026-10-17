package httpadapter

import "github.com/eventflow/eventflow/services/payment/internal/domain"

// InitiatePaymentRequest khop 100% voi payload tu apps/web/src/lib/payment-client.ts.
type InitiatePaymentRequest struct {
	OrderID    string `json:"order_id"`
	HoldID     string `json:"hold_id"`
	Amount     int64  `json:"amount"`
	Method     string `json:"method"`
	BuyerName  string `json:"buyer_name"`
	BuyerEmail string `json:"buyer_email"`
	BuyerPhone string `json:"buyer_phone"`
	ReturnURL  string `json:"return_url"`
}

// InitiatePaymentResponse la ket qua phan hoi cho client.
type InitiatePaymentResponse struct {
	PaymentID     string `json:"payment_id"`
	OrderID       string `json:"order_id"`
	RedirectURL   string `json:"redirect_url"`
	Status        string `json:"status"`
	ProviderTxnID string `json:"provider_txn_id,omitempty"`
}

// PaymentDetailsResponse tra ve chi tiet thong tin thanh toan khi truy van GET.
type PaymentDetailsResponse struct {
	PaymentID     string `json:"payment_id"`
	OrderID       string `json:"order_id"`
	Provider      string `json:"provider"`
	ProviderTxnID string `json:"provider_txn_id"`
	AmountCents   int64  `json:"amount_cents"`
	Currency      string `json:"currency"`
	Status        string `json:"status"`
	CreatedAt     string `json:"created_at"`
}

// PaymentMethodsResponse tra ve danh sach cac phuong thuc thanh toan kha dung.
type PaymentMethodsResponse struct {
	Methods []domain.PaymentMethodInfo `json:"methods"`
}

// WebhookResponse la ket qua tra ve khi xu ly webhook thanh cong.
type WebhookResponse struct {
	Status           string `json:"status"`
	OrderID          string `json:"order_id,omitempty"`
	PaymentID        string `json:"payment_id,omitempty"`
	AlreadyProcessed bool   `json:"already_processed"`
	StoredInInbox    bool   `json:"stored_in_inbox,omitempty"`
	Message          string `json:"message"`
}
