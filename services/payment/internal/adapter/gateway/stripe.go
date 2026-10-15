package gateway

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/eventflow/eventflow/services/payment/internal/domain"
	"github.com/eventflow/eventflow/services/payment/internal/port"
)

type StripeConfig struct {
	PublishableKey string
	SecretKey      string
	WebhookSecret  string
	SuccessURL     string
	CancelURL      string
}

// StripeGateway hien thuc cong thanh toan the quoc te qua Stripe Checkout.
type StripeGateway struct {
	cfg StripeConfig
}

func NewStripeGateway(cfg StripeConfig) *StripeGateway {
	return &StripeGateway{cfg: cfg}
}

func (s *StripeGateway) Provider() domain.Provider {
	return domain.ProviderStripe
}

func (s *StripeGateway) InitiatePayment(ctx context.Context, req port.PaymentRequest) (port.PaymentResult, error) {
	select {
	case <-ctx.Done():
		return port.PaymentResult{}, ctx.Err()
	default:
	}

	sessionID := fmt.Sprintf("cs_test_%s", req.PaymentID)

	successURL := req.ReturnURL
	if successURL == "" {
		successURL = s.cfg.SuccessURL
	}
	if successURL == "" {
		successURL = fmt.Sprintf("/checkout/%s/result?orderId=%s", url.QueryEscape(req.EventID), url.QueryEscape(req.OrderID))
	}

	u, err := url.Parse(successURL)
	if err == nil {
		q := u.Query()
		q.Set("session_id", sessionID)
		q.Set("payment_id", req.PaymentID)
		q.Set("order_id", req.OrderID)
		q.Set("provider", string(domain.ProviderStripe))
		u.RawQuery = q.Encode()
		successURL = u.String()
	}

	checkoutURL := fmt.Sprintf("https://checkout.stripe.com/pay/%s#%s", sessionID, url.QueryEscape(req.OrderID))

	currency := strings.ToLower(req.Currency)
	if currency == "" {
		currency = "vnd"
	}

	return port.PaymentResult{
		PaymentID:     req.PaymentID,
		OrderID:       req.OrderID,
		RedirectURL:   checkoutURL,
		Status:        "PENDING",
		ProviderTxnID: sessionID,
		RawResponse: map[string]any{
			"session_id": sessionID,
			"provider":   string(domain.ProviderStripe),
			"currency":   currency,
			"amount":     req.AmountCents,
		},
	}, nil
}

func (s *StripeGateway) VerifyCallback(ctx context.Context, params map[string]string) (port.CallbackResult, error) {
	select {
	case <-ctx.Done():
		return port.CallbackResult{}, ctx.Err()
	default:
	}

	sessionID := params["session_id"]
	if sessionID == "" {
		sessionID = params["id"]
	}
	if sessionID == "" {
		return port.CallbackResult{
			Valid:   false,
			Message: "thieu session_id hoac payment_intent_id cua Stripe",
		}, nil
	}

	statusParam := strings.ToLower(params["status"])
	paymentStatus := domain.StatusSucceeded
	if statusParam == "failed" || statusParam == "unpaid" {
		paymentStatus = domain.StatusFailed
	}

	return port.CallbackResult{
		Valid:         true,
		ProviderTxnID: sessionID,
		Status:        paymentStatus,
		Message:       "Stripe session xac thuc thanh cong",
	}, nil
}
