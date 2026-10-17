package gateway

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/eventflow/eventflow/services/payment/internal/domain"
	"github.com/eventflow/eventflow/services/payment/internal/port"
)

type MoMoConfig struct {
	PartnerCode string
	AccessKey   string
	SecretKey   string
	Endpoint    string
	ReturnURL   string
	NotifyURL   string
}

// MoMoGateway hien thuc ket noi toi cong thanh toan MoMo (HMAC-SHA256).
type MoMoGateway struct {
	cfg MoMoConfig
}

func NewMoMoGateway(cfg MoMoConfig) *MoMoGateway {
	if cfg.Endpoint == "" {
		cfg.Endpoint = "https://test-payment.momo.vn/v2/gateway/api/create"
	}
	return &MoMoGateway{cfg: cfg}
}

func (m *MoMoGateway) Provider() domain.Provider {
	return domain.ProviderMoMo
}

func (m *MoMoGateway) InitiatePayment(ctx context.Context, req port.PaymentRequest) (port.PaymentResult, error) {
	select {
	case <-ctx.Done():
		return port.PaymentResult{}, ctx.Err()
	default:
	}

	redirectURL := req.ReturnURL
	if redirectURL == "" {
		redirectURL = m.cfg.ReturnURL
	}
	ipnURL := m.cfg.NotifyURL
	if ipnURL == "" {
		ipnURL = "https://api.eventflow.local/v1/payments/webhook/momo"
	}

	requestID := req.PaymentID
	orderInfo := fmt.Sprintf("Thanh toan ve EventFlow don hang %s", req.OrderID)
	requestType := "captureWallet"
	extraData := ""

	// Chuoi ky chuan MoMo captureWallet
	rawSignature := fmt.Sprintf(
		"accessKey=%s&amount=%d&extraData=%s&ipnUrl=%s&orderId=%s&orderInfo=%s&partnerCode=%s&redirectUrl=%s&requestId=%s&requestType=%s",
		m.cfg.AccessKey, req.AmountCents, extraData, ipnURL, req.OrderID, orderInfo, m.cfg.PartnerCode, redirectURL, requestID, requestType,
	)

	signature := ComputeHmacSHA256(m.cfg.SecretKey, rawSignature)

	// Sinh payUrl cho client chuyen huong hoac mo app MoMo
	payURL := fmt.Sprintf(
		"%s?partnerCode=%s&orderId=%s&requestId=%s&amount=%d&signature=%s",
		m.cfg.Endpoint, m.cfg.PartnerCode, url.QueryEscape(req.OrderID), url.QueryEscape(requestID), req.AmountCents, signature,
	)

	providerTxnID := fmt.Sprintf("momo_%s", req.PaymentID)

	return port.PaymentResult{
		PaymentID:     req.PaymentID,
		OrderID:       req.OrderID,
		RedirectURL:   payURL,
		Status:        "PENDING",
		ProviderTxnID: providerTxnID,
		RawResponse: map[string]any{
			"partnerCode": m.cfg.PartnerCode,
			"orderId":     req.OrderID,
			"requestId":   requestID,
			"amount":      req.AmountCents,
			"signature":   signature,
		},
	}, nil
}

func (m *MoMoGateway) VerifyCallback(ctx context.Context, params map[string]string) (port.CallbackResult, error) {
	select {
	case <-ctx.Done():
		return port.CallbackResult{}, ctx.Err()
	default:
	}

	incomingSignature := params["signature"]
	if incomingSignature == "" {
		return port.CallbackResult{Valid: false, Message: "thieu chu ky signature cua MoMo"}, nil
	}

	accessKey := params["accessKey"]
	if accessKey == "" {
		accessKey = m.cfg.AccessKey
	}
	amount := params["amount"]
	extraData := params["extraData"]
	message := params["message"]
	orderID := params["orderId"]
	orderInfo := params["orderInfo"]
	orderType := params["orderType"]
	partnerCode := params["partnerCode"]
	payType := params["payType"]
	requestID := params["requestId"]
	responseTime := params["responseTime"]
	resultCode := params["resultCode"]
	transID := params["transId"]

	// Dinh dang chuoi ky tra ve cua MoMo
	rawSignature := fmt.Sprintf(
		"accessKey=%s&amount=%s&extraData=%s&message=%s&orderId=%s&orderInfo=%s&orderType=%s&partnerCode=%s&payType=%s&requestId=%s&responseTime=%s&resultCode=%s&transId=%s",
		accessKey, amount, extraData, message, orderID, orderInfo, orderType, partnerCode, payType, requestID, responseTime, resultCode, transID,
	)

	expectedSignature := ComputeHmacSHA256(m.cfg.SecretKey, rawSignature)
	if !strings.EqualFold(incomingSignature, expectedSignature) {
		return port.CallbackResult{
			Valid:   false,
			Message: "chu ky MoMo khong khop",
		}, nil
	}

	status := domain.StatusFailed
	if resultCode == "0" {
		status = domain.StatusSucceeded
	}

	var amountCents int64
	if parsed, err := strconv.ParseInt(amount, 10, 64); err == nil {
		amountCents = parsed
	}

	var ts time.Time
	if responseTime != "" {
		if ms, err := strconv.ParseInt(responseTime, 10, 64); err == nil {
			ts = time.UnixMilli(ms).UTC()
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

	raw := make(map[string]any, len(params))
	for k, v := range params {
		raw[k] = v
	}

	return port.CallbackResult{
		Valid:         true,
		ProviderTxnID: transID,
		OrderID:       orderID,
		PaymentID:     requestID,
		Status:        status,
		AmountCents:   amountCents,
		Timestamp:     ts,
		RawPayload:    raw,
		Message:       message,
	}, nil
}

// ComputeHmacSHA256 tinh toan ma bam HMAC-SHA256 tra ve hex chu thuong.
func ComputeHmacSHA256(secret, data string) string {
	h := hmac.New(sha256.New, []byte(secret))
	h.Write([]byte(data))
	return hex.EncodeToString(h.Sum(nil))
}
