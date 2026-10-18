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
	mode          SandboxMode
	queryStatuses map[string]domain.PaymentStatus
}

// NewSandboxGateway khoi tao sandbox gateway voi che do gia lap chi dinh.
func NewSandboxGateway(mode SandboxMode) *SandboxGateway {
	if mode == "" {
		mode = SandboxModeSuccess
	}
	return &SandboxGateway{
		mode:          mode,
		queryStatuses: make(map[string]domain.PaymentStatus),
	}
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

// SetQueryStatus ghi de trang thai tra ve khi QueryPayment cho mot order_id cu the (ho tro unit test).
func (s *SandboxGateway) SetQueryStatus(orderID string, status domain.PaymentStatus) {
	if s.queryStatuses == nil {
		s.queryStatuses = make(map[string]domain.PaymentStatus)
	}
	s.queryStatuses[orderID] = status
}

// QueryPayment chu dong tra cuu trang thai giao dich tu sandbox gateway (EV-153).
func (s *SandboxGateway) QueryPayment(ctx context.Context, req domain.PaymentQueryRequest) (domain.PaymentQueryResult, error) {
	select {
	case <-ctx.Done():
		return domain.PaymentQueryResult{}, ctx.Err()
	default:
	}

	// 1. Kiem tra ghi de rieng theo orderID
	if s.queryStatuses != nil {
		if st, ok := s.queryStatuses[req.OrderID]; ok {
			return domain.PaymentQueryResult{
				ProviderTxnID: req.ProviderTxnID,
				Status:        st,
				Message:       fmt.Sprintf("sandbox tra cuu thanh cong voi trang thai %s", st),
			}, nil
		}
	}

	// 2. Tra ve theo che do sandbox hien tai
	switch s.mode {
	case SandboxModeFail:
		return domain.PaymentQueryResult{
			ProviderTxnID: req.ProviderTxnID,
			Status:        domain.StatusFailed,
			Message:       "sandbox mo phong giao dich that bai hoac khong ton tai",
		}, nil
	case SandboxModeDelay:
		// Neu dat simulate delay: mac dinh co the la PENDING (dang cho) hoac SUCCEEDED neu don da xong o cong
		status := domain.StatusSucceeded
		if req.OrderID == "ord-still-pending" {
			status = domain.StatusPending
		}
		return domain.PaymentQueryResult{
			ProviderTxnID: req.ProviderTxnID,
			Status:        status,
			Message:       "sandbox delay query result",
		}, nil
	default:
		return domain.PaymentQueryResult{
			ProviderTxnID: req.ProviderTxnID,
			Status:        domain.StatusSucceeded,
			Message:       "sandbox giao dich thanh cong",
		}, nil
	}
}

