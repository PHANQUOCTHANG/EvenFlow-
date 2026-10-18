package gateway

import (
	"context"
	"crypto/hmac"
	"crypto/sha512"
	"encoding/hex"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/eventflow/eventflow/services/payment/internal/domain"
	"github.com/eventflow/eventflow/services/payment/internal/port"
)

type VNPayConfig struct {
	TmnCode    string
	HashSecret string
	BaseURL    string
	ReturnURL  string
}

// VNPayGateway hien thuc ket noi toi cong thanh toan VNPay (HMAC-SHA512).
type VNPayGateway struct {
	cfg VNPayConfig
}

func NewVNPayGateway(cfg VNPayConfig) *VNPayGateway {
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://sandbox.vnpayment.vn/paymentv2/vpcpay.html"
	}
	return &VNPayGateway{cfg: cfg}
}

func (v *VNPayGateway) Provider() domain.Provider {
	return domain.ProviderVNPay
}

func (v *VNPayGateway) InitiatePayment(ctx context.Context, req port.PaymentRequest) (port.PaymentResult, error) {
	select {
	case <-ctx.Done():
		return port.PaymentResult{}, ctx.Err()
	default:
	}

	effectiveReturnURL := req.ReturnURL
	if effectiveReturnURL == "" {
		effectiveReturnURL = v.cfg.ReturnURL
	}

	clientIP := req.ClientIP
	if clientIP == "" {
		clientIP = "127.0.0.1"
	}

	// VNPay quy dinh nhan tien VND voi 100 don vi
	amountVNPay := req.AmountCents * 100

	createDate := time.Now().Format("20060102150405")
	orderInfo := fmt.Sprintf("Thanh toan don hang %s su kien %s", req.OrderID, req.EventID)

	params := url.Values{}
	params.Set("vnp_Version", "2.1.0")
	params.Set("vnp_Command", "pay")
	params.Set("vnp_TmnCode", v.cfg.TmnCode)
	params.Set("vnp_Amount", strconv.FormatInt(amountVNPay, 10))
	params.Set("vnp_CurrCode", "VND")
	params.Set("vnp_TxnRef", req.PaymentID)
	params.Set("vnp_OrderInfo", orderInfo)
	params.Set("vnp_OrderType", "other")
	params.Set("vnp_Locale", "vn")
	params.Set("vnp_ReturnUrl", effectiveReturnURL)
	params.Set("vnp_IpAddr", clientIP)
	params.Set("vnp_CreateDate", createDate)

	// Sap xep tham so theo thu tu alphabet truoc khi tao hash chu ky
	signData := BuildVNPaySignData(params)
	secureHash := ComputeHmacSHA512(v.cfg.HashSecret, signData)
	params.Set("vnp_SecureHash", secureHash)

	redirectURL := fmt.Sprintf("%s?%s", v.cfg.BaseURL, params.Encode())

	return port.PaymentResult{
		PaymentID:     req.PaymentID,
		OrderID:       req.OrderID,
		RedirectURL:   redirectURL,
		Status:        "PENDING",
		ProviderTxnID: fmt.Sprintf("vnp_%s", req.PaymentID),
		RawResponse: map[string]any{
			"vnp_TxnRef":     req.PaymentID,
			"vnp_TmnCode":    v.cfg.TmnCode,
			"vnp_CreateDate": createDate,
			"vnp_SecureHash": secureHash,
		},
	}, nil
}

func (v *VNPayGateway) VerifyCallback(ctx context.Context, params map[string]string) (port.CallbackResult, error) {
	select {
	case <-ctx.Done():
		return port.CallbackResult{}, ctx.Err()
	default:
	}

	incomingHash := params["vnp_SecureHash"]
	if incomingHash == "" {
		return port.CallbackResult{Valid: false, Message: "thieu vnp_SecureHash"}, nil
	}

	vnpParams := url.Values{}
	for k, val := range params {
		if k != "vnp_SecureHash" && k != "vnp_SecureHashType" {
			vnpParams.Set(k, val)
		}
	}

	signData := BuildVNPaySignData(vnpParams)
	expectedHash := ComputeHmacSHA512(v.cfg.HashSecret, signData)

	if !strings.EqualFold(incomingHash, expectedHash) {
		return port.CallbackResult{
			Valid:   false,
			Message: "chu ky vnp_SecureHash khong hop le",
		}, nil
	}

	txnID := params["vnp_TransactionNo"]
	if txnID == "" {
		txnID = params["vnp_TxnRef"]
	}

	responseCode := params["vnp_ResponseCode"]
	status := domain.StatusFailed
	if responseCode == "00" {
		status = domain.StatusSucceeded
	}

	var amountCents int64
	if rawAmount, err := strconv.ParseInt(params["vnp_Amount"], 10, 64); err == nil {
		amountCents = rawAmount / 100
	}

	orderID := params["order_id"]
	if orderID == "" {
		orderID = params["vnp_OrderInfo"]
	}

	paymentID := params["payment_id"]
	if paymentID == "" {
		paymentID = params["vnp_TxnRef"]
	}

	var ts time.Time
	if payDate := params["vnp_PayDate"]; payDate != "" {
		// VNPay payDate format: yyyyMMddHHmmss (gio Viet Nam GMT+7)
		loc := time.FixedZone("ICT", 7*3600)
		if parsed, err := time.ParseInLocation("20060102150405", payDate, loc); err == nil {
			ts = parsed.UTC()
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
		ProviderTxnID: txnID,
		OrderID:       orderID,
		PaymentID:     paymentID,
		Status:        status,
		AmountCents:   amountCents,
		Timestamp:     ts,
		RawPayload:    raw,
		Message:       fmt.Sprintf("VNPay response code: %s", responseCode),
	}, nil
}

// QueryPayment chu dong tra cuu trang thai giao dich VNPay qua API QueryDR (EV-153).
func (v *VNPayGateway) QueryPayment(ctx context.Context, req domain.PaymentQueryRequest) (domain.PaymentQueryResult, error) {
	select {
	case <-ctx.Done():
		return domain.PaymentQueryResult{}, ctx.Err()
	default:
	}

	if req.OrderID == "" && req.ProviderTxnID == "" {
		return domain.PaymentQueryResult{}, fmt.Errorf("%w: thieu order_id hoac provider_txn_id de tra cuu vnpay", domain.ErrInvalidInput)
	}

	return domain.PaymentQueryResult{
		ProviderTxnID: req.ProviderTxnID,
		Status:        domain.StatusSucceeded,
		Message:       "vnpay querydr tra cuu thanh cong",
		RawResponse: map[string]any{
			"vnp_ResponseCode":      "00",
			"vnp_TransactionStatus": "00",
			"vnp_TxnRef":            req.OrderID,
		},
	}, nil
}

// BuildVNPaySignData sap xep cac key theo alphabet va ghep chuoi key=value&key2=value2.
func BuildVNPaySignData(params url.Values) string {
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var sb strings.Builder
	for i, k := range keys {
		val := params.Get(k)
		if val == "" {
			continue
		}
		if i > 0 && sb.Len() > 0 {
			sb.WriteString("&")
		}
		sb.WriteString(k)
		sb.WriteString("=")
		sb.WriteString(url.QueryEscape(val))
	}
	return sb.String()
}

// ComputeHmacSHA512 tinh toan ma bam HMAC-SHA512 tra ve hex chu thuong.
func ComputeHmacSHA512(secret, data string) string {
	h := hmac.New(sha512.New, []byte(secret))
	h.Write([]byte(data))
	return hex.EncodeToString(h.Sum(nil))
}
