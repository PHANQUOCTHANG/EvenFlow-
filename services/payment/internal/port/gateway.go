package port

import (
	"context"
	"time"

	"github.com/eventflow/eventflow/services/payment/internal/domain"
)

// PaymentRequest chua toan bo tham so can thiet de khoi tao phien thanh toan.
type PaymentRequest struct {
	EventID     string
	OrderID     string
	HoldID      string
	PaymentID   string
	AmountCents int64
	Currency    string
	BuyerName   string
	BuyerEmail  string
	BuyerPhone  string
	ReturnURL   string
	ClientIP    string
}

// PaymentResult la ket qua tra ve sau khi khoi tao qua cong thanh toan.
type PaymentResult struct {
	PaymentID     string         `json:"payment_id"`
	OrderID       string         `json:"order_id"`
	RedirectURL   string         `json:"redirect_url"`
	Status        string         `json:"status"`
	ProviderTxnID string         `json:"provider_txn_id"`
	RawResponse   map[string]any `json:"raw_response,omitempty"`
}

// CallbackResult dai dien cho ket qua xac minh callback/webhook tu cong thanh toan.
type CallbackResult struct {
	Valid         bool
	ProviderTxnID string
	OrderID       string
	PaymentID     string
	Status        domain.PaymentStatus
	AmountCents   int64
	Timestamp     time.Time
	RawPayload    map[string]any
	Message       string
}

// PaymentGateway la cong giao tiep truu tuong toi mot cong thanh toan cu the.
type PaymentGateway interface {
	// Provider tra ve dinh danh cua cong (vnpay, momo, stripe, sandbox).
	Provider() domain.Provider

	// InitiatePayment khoi tao giao dich va tra ve redirect URL hoac payload thanh toan.
	InitiatePayment(ctx context.Context, req PaymentRequest) (PaymentResult, error)

	// VerifyCallback xac thuc chu ky va tinh toan ven cua du lieu tra ve tu cong.
	VerifyCallback(ctx context.Context, params map[string]string) (CallbackResult, error)

	// QueryPayment chu dong tra cuu trang thai giao dich tu cong thanh toan (EV-153 / BR-O6).
	QueryPayment(ctx context.Context, req domain.PaymentQueryRequest) (domain.PaymentQueryResult, error)

	// RefundPayment chu dong thuc hien giao dich hoan tien toi cong thanh toan (EV-154 / EVF-74).
	RefundPayment(ctx context.Context, req domain.PaymentRefundRequest) (domain.PaymentRefundResult, error)
}

// GatewayRegistry quan ly danh sach cac cong thanh toan duoc ho tro va kich hoat.
type GatewayRegistry interface {
	// Get tra ve gateway tuong ung voi provider, bao loi neu khong ton tai hoac bi disabled.
	Get(provider domain.Provider) (PaymentGateway, error)

	// ListEnabled tra ve danh sach thong tin cac cong thanh toan dang duoc phep hoat dong.
	ListEnabled() []domain.PaymentMethodInfo

	// DefaultProvider tra ve cong mac dinh theo cau hinh he thong.
	DefaultProvider() domain.Provider
}
