package gateway

import (
	"context"
	"fmt"
	"net/url"
	"time"

	"github.com/eventflow/eventflow/services/payment/internal/domain"
	"github.com/eventflow/eventflow/services/payment/internal/port"
)

type SandboxMode string

const (
	SandboxModeSuccess SandboxMode = "success"
	SandboxModeFail    SandboxMode = "fail"
	SandboxModeDelay   SandboxMode = "delay"
)

// SandboxGateway la hien thuc mock chay local 100%, 0 dependency ngoai.
type SandboxGateway struct {
	mode SandboxMode
}

// NewSandboxGateway khoi tao sandbox gateway voi che do gia lap chi dinh.
func NewSandboxGateway(mode SandboxMode) *SandboxGateway {
	if mode == "" {
		mode = SandboxModeSuccess
	}
	return &SandboxGateway{mode: mode}
}

func (s *SandboxGateway) Provider() domain.Provider {
	return domain.ProviderSandbox
}

func (s *SandboxGateway) SetMode(mode SandboxMode) {
	s.mode = mode
}

func (s *SandboxGateway) InitiatePayment(ctx context.Context, req port.PaymentRequest) (port.PaymentResult, error) {
	select {
	case <-ctx.Done():
		return port.PaymentResult{}, ctx.Err()
	default:
	}

	if s.mode == SandboxModeFail {
		return port.PaymentResult{}, fmt.Errorf("%w: sandbox mo phong giao dich bi tu choi boi ngan hang", domain.ErrGatewayCommunication)
	}

	providerTxnID := fmt.Sprintf("sbx_txn_%s_%d", req.PaymentID, time.Now().UnixNano()%1000000)

	// Xay dung redirect URL dua nguoi dung quay ve trang ket qua don hang (EVF-114, EV-184)
	returnURL := req.ReturnURL
	if returnURL == "" {
		returnURL = fmt.Sprintf("/checkout/%s/result?orderId=%s", url.QueryEscape(req.EventID), url.QueryEscape(req.OrderID))
	}

	u, err := url.Parse(returnURL)
	if err == nil {
		q := u.Query()
		q.Set("payment_id", req.PaymentID)
		q.Set("order_id", req.OrderID)
		q.Set("provider", string(domain.ProviderSandbox))
		q.Set("provider_txn_id", providerTxnID)
		if s.mode == SandboxModeDelay {
			q.Set("status", "PENDING")
			q.Set("simulated_delay", "true")
		} else {
			q.Set("status", "SUCCESS")
		}
		u.RawQuery = q.Encode()
		returnURL = u.String()
	}

	status := "PENDING"
	if s.mode == SandboxModeDelay {
		status = "PROCESSING"
	}

	return port.PaymentResult{
		PaymentID:     req.PaymentID,
		OrderID:       req.OrderID,
		RedirectURL:   returnURL,
		Status:        status,
		ProviderTxnID: providerTxnID,
		RawResponse: map[string]any{
			"mode":            string(s.mode),
			"provider":        string(domain.ProviderSandbox),
			"provider_txn_id": providerTxnID,
			"simulated_at":    time.Now().UTC().Format(time.RFC3339),
		},
	}, nil
}

func (s *SandboxGateway) VerifyCallback(ctx context.Context, params map[string]string) (port.CallbackResult, error) {
	select {
	case <-ctx.Done():
		return port.CallbackResult{}, ctx.Err()
	default:
	}

	if params["signature"] == "invalid" || params["test_invalid_sig"] == "true" {
		return port.CallbackResult{
			Valid:   false,
			Message: "sandbox chu ky khong hop le",
		}, nil
	}

	txnID := params["provider_txn_id"]
	if txnID == "" {
		txnID = params["vnp_TransactionNo"]
	}
	if txnID == "" {
		txnID = params["transId"]
	}
	if txnID == "" {
		return port.CallbackResult{
			Valid:   false,
			Message: "thieu provider_txn_id trong callback",
		}, nil
	}

	statusParam := params["status"]
	paymentStatus := domain.StatusSucceeded
	if statusParam == "FAILED" || statusParam == "fail" {
		paymentStatus = domain.StatusFailed
	}

	orderID := params["order_id"]
	if orderID == "" {
		orderID = params["orderId"]
	}

	paymentID := params["payment_id"]
	if paymentID == "" {
		paymentID = params["paymentId"]
	}

	var ts time.Time
	if tsStr := params["timestamp"]; tsStr != "" {
		if parsed, err := time.Parse(time.RFC3339, tsStr); err == nil {
			ts = parsed
		} else if parsedUnix, err := time.Parse(time.RFC3339Nano, tsStr); err == nil {
			ts = parsedUnix
		}
	}
	if ts.IsZero() {
		ts = time.Now().UTC()
	}

	raw := make(map[string]any, len(params))
	for k, v := range params {
		raw[k] = v
	}

	return port.CallbackResult{
		Valid:         true,
		ProviderTxnID: txnID,
		OrderID:       orderID,
		PaymentID:     paymentID,
		Status:        paymentStatus,
		Timestamp:     ts,
		RawPayload:    raw,
		Message:       "sandbox callback hop le",
	}, nil
}
