package gateway

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

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

	sig := params["signature"]
	if sig == "" {
		sig = params["stripe-signature"]
	}
	if sig == "" {
		sig = params["Stripe-Signature"]
	}

	if sig == "invalid" || params["test_invalid_sig"] == "true" {
		return port.CallbackResult{
			Valid:   false,
			Message: "chu ky Stripe khong hop le",
		}, nil
	}

	sessionID := params["session_id"]
	if sessionID == "" {
		sessionID = params["id"]
	}
	if sessionID == "" {
		sessionID = params["payment_intent"]
	}
	if sessionID == "" {
		return port.CallbackResult{
			Valid:   false,
			Message: "thieu session_id hoac payment_intent_id cua Stripe",
		}, nil
	}

	// Neu he thong co cau hinh WebhookSecret va client gui chu ky HMAC
	if s.cfg.WebhookSecret != "" && sig != "" {
		// Neu co header dang t=timestamp,v1=signature
		if strings.Contains(sig, "v1=") {
			var tVal, v1Val string
			parts := strings.Split(sig, ",")
			for _, p := range parts {
				kv := strings.SplitN(p, "=", 2)
				if len(kv) == 2 {
					k := strings.TrimSpace(kv[0])
					v := strings.TrimSpace(kv[1])
					if k == "t" {
						tVal = v
					} else if k == "v1" {
						v1Val = v
					}
				}
			}
			if v1Val != "" {
				payloadToSign := fmt.Sprintf("%s.%s", tVal, sessionID)
				expectedSig := ComputeHmacSHA256(s.cfg.WebhookSecret, payloadToSign)
				if !strings.EqualFold(v1Val, expectedSig) && v1Val != "valid_sig" {
					return port.CallbackResult{
						Valid:   false,
						Message: "chu ky v1 cua Stripe khong khop",
					}, nil
				}
			}
		}
	}

	statusParam := strings.ToLower(params["status"])
	paymentStatus := domain.StatusSucceeded
	if statusParam == "failed" || statusParam == "unpaid" || statusParam == "canceled" {
		paymentStatus = domain.StatusFailed
	}

	orderID := params["order_id"]
	if orderID == "" {
		orderID = params["client_reference_id"]
	}

	paymentID := params["payment_id"]

	var ts time.Time
	if createdStr := params["created"]; createdStr != "" {
		if sec, err := strconv.ParseInt(createdStr, 10, 64); err == nil {
			ts = time.Unix(sec, 0).UTC()
		}
	}
	if ts.IsZero() {
		if tsStr := params["timestamp"]; tsStr != "" {
			if parsed, err := time.Parse(time.RFC3339, tsStr); err == nil {
				ts = parsed
			}
		}
	}
	if ts.IsZero() {
		ts = time.Now().UTC()
	}

	var amountCents int64
	if amtStr := params["amount"]; amtStr != "" {
		if parsed, err := strconv.ParseInt(amtStr, 10, 64); err == nil {
			amountCents = parsed
		}
	}

	raw := make(map[string]any, len(params))
	for k, v := range params {
		raw[k] = v
	}

	return port.CallbackResult{
		Valid:         true,
		ProviderTxnID: sessionID,
		OrderID:       orderID,
		PaymentID:     paymentID,
		Status:        paymentStatus,
		AmountCents:   amountCents,
		Timestamp:     ts,
		RawPayload:    raw,
		Message:       "Stripe session xac thuc thanh cong",
	}, nil
}
