package httpadapter

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"time"

	"github.com/eventflow/eventflow/libs/go/httpx"
	"github.com/eventflow/eventflow/services/payment/internal/app"
	"github.com/eventflow/eventflow/services/payment/internal/domain"
)

type PaymentInitiator interface {
	Execute(ctx context.Context, in app.InitiatePaymentInput) (app.InitiatePaymentOutput, error)
}

type PaymentGetter interface {
	Execute(ctx context.Context, paymentID string) (domain.Payment, error)
}

type MethodsLister interface {
	Execute(ctx context.Context) []domain.PaymentMethodInfo
}

type WebhookProcessor interface {
	Execute(ctx context.Context, in app.ProcessWebhookInput) (domain.WebhookProcessResult, error)
}

type Handler struct {
	initiator PaymentInitiator
	getter    PaymentGetter
	lister    MethodsLister
	processor WebhookProcessor
	log       *slog.Logger
}

func NewHandler(
	initiator PaymentInitiator,
	getter PaymentGetter,
	lister MethodsLister,
	processor WebhookProcessor,
	log *slog.Logger,
) *Handler {
	return &Handler{
		initiator: initiator,
		getter:    getter,
		lister:    lister,
		processor: processor,
		log:       log,
	}
}

// Routes dang ky cac endpoints REST cho payment service.
// idempotency BAT BUOC cho endpoint ghi thanh toan theo BR-O5.
func (h *Handler) Routes(mux *http.ServeMux, idempotency httpx.Middleware) {
	if idempotency != nil {
		mux.Handle("POST /v1/events/{eventID}/payments", idempotency(http.HandlerFunc(h.initiatePayment)))
	} else {
		mux.HandleFunc("POST /v1/events/{eventID}/payments", h.initiatePayment)
	}

	mux.HandleFunc("GET /v1/payments/{paymentID}", h.getPayment)
	mux.HandleFunc("GET /v1/payments/methods", h.listMethods)

	// Endpoints Webhook tiep nhan tu cac cong thanh toan (BR-O6 exactly-once)
	mux.HandleFunc("POST /v1/payments/webhook/{provider}", h.handleWebhook)
	mux.HandleFunc("GET /v1/payments/webhook/{provider}", h.handleWebhook)
}

func (h *Handler) initiatePayment(w http.ResponseWriter, r *http.Request) {
	eventID := r.PathValue("eventID")
	if eventID == "" {
		httpx.WriteProblem(w, http.StatusBadRequest, "thiếu eventID trong đường dẫn")
		return
	}

	var req InitiatePaymentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteProblem(w, http.StatusBadRequest, "dữ liệu JSON không hợp lệ")
		return
	}

	clientIP := r.Header.Get("X-Forwarded-For")
	if clientIP == "" {
		clientIP = r.RemoteAddr
	}

	out, err := h.initiator.Execute(r.Context(), app.InitiatePaymentInput{
		EventID:        eventID,
		OrderID:        req.OrderID,
		HoldID:         req.HoldID,
		AmountCents:    req.Amount,
		Currency:       "VND",
		Method:         domain.NormalizeProvider(req.Method),
		BuyerName:      req.BuyerName,
		BuyerEmail:     req.BuyerEmail,
		BuyerPhone:     req.BuyerPhone,
		ReturnURL:      req.ReturnURL,
		ClientIP:       clientIP,
		IdempotencyKey: r.Header.Get("Idempotency-Key"),
	})

	if err != nil {
		switch {
		case errors.Is(err, domain.ErrHoldExpired):
			// 409 Conflict: Hold da het han truoc khi thanh toan (BR-O2)
			httpx.WriteProblem(w, http.StatusConflict, "Phiên giữ vé đã hết hạn trước khi thanh toán (BR-O2)")
		case errors.Is(err, domain.ErrOrderNotFound):
			httpx.WriteProblem(w, http.StatusNotFound, "Không tìm thấy đơn hàng")
		case errors.Is(err, domain.ErrOrderNotHeld):
			httpx.WriteProblem(w, http.StatusConflict, err.Error())
		case errors.Is(err, domain.ErrAmountMismatch), errors.Is(err, domain.ErrCurrencyMismatch):
			httpx.WriteProblem(w, http.StatusUnprocessableEntity, err.Error())
		case errors.Is(err, domain.ErrUnsupportedProvider), errors.Is(err, domain.ErrProviderDisabled), errors.Is(err, domain.ErrInvalidInput):
			httpx.WriteProblem(w, http.StatusBadRequest, err.Error())
		default:
			h.log.Error("khoi tao thanh toan loi he thong",
				"event_id", eventID,
				"order_id", req.OrderID,
				"err", err,
			)
			httpx.WriteProblem(w, http.StatusServiceUnavailable, "Hệ thống thanh toán đang bận, vui lòng thử lại")
		}
		return
	}

	resp := InitiatePaymentResponse{
		PaymentID:     out.PaymentID,
		OrderID:       out.OrderID,
		RedirectURL:   out.RedirectURL,
		Status:        out.Status,
		ProviderTxnID: out.ProviderTxnID,
	}

	httpx.WriteJSON(w, http.StatusOK, resp)
}

func (h *Handler) getPayment(w http.ResponseWriter, r *http.Request) {
	paymentID := r.PathValue("paymentID")
	if paymentID == "" {
		httpx.WriteProblem(w, http.StatusBadRequest, "thiếu paymentID")
		return
	}

	p, err := h.getter.Execute(r.Context(), paymentID)
	if err != nil {
		httpx.WriteProblem(w, http.StatusNotFound, "không tìm thấy giao dịch thanh toán")
		return
	}

	resp := PaymentDetailsResponse{
		PaymentID:     p.ID,
		OrderID:       p.OrderID,
		Provider:      string(p.Provider),
		ProviderTxnID: p.ProviderTxnID,
		AmountCents:   p.AmountCents,
		Currency:      p.Currency,
		Status:        string(p.Status),
		CreatedAt:     p.CreatedAt.Format(time.RFC3339),
	}

	httpx.WriteJSON(w, http.StatusOK, resp)
}

func (h *Handler) listMethods(w http.ResponseWriter, r *http.Request) {
	methods := h.lister.Execute(r.Context())
	httpx.WriteJSON(w, http.StatusOK, PaymentMethodsResponse{Methods: methods})
}

func (h *Handler) handleWebhook(w http.ResponseWriter, r *http.Request) {
	providerStr := r.PathValue("provider")
	if providerStr == "" {
		httpx.WriteProblem(w, http.StatusBadRequest, "thiếu provider trong đường dẫn webhook")
		return
	}

	params := make(map[string]string)

	// 1. Doc query parameters (VD: VNPay IPN/redirect callback)
	for k, v := range r.URL.Query() {
		if len(v) > 0 {
			params[k] = v[0]
		}
	}

	// 2. Doc body neu la POST
	if r.Method == http.MethodPost && r.Body != nil {
		bodyBytes, err := io.ReadAll(io.LimitReader(r.Body, 1024*1024))
		if err == nil && len(bodyBytes) > 0 {
			// Thu parse JSON
			var jsonMap map[string]any
			if jsonErr := json.Unmarshal(bodyBytes, &jsonMap); jsonErr == nil {
				for k, v := range jsonMap {
					if _, exists := params[k]; !exists {
						params[k] = fmt.Sprintf("%v", v)
					}
				}
			} else {
				// Thu parse form data
				if formVals, parseErr := url.ParseQuery(string(bodyBytes)); parseErr == nil {
					for k, v := range formVals {
						if len(v) > 0 {
							if _, exists := params[k]; !exists {
								params[k] = v[0]
							}
						}
					}
				}
			}
		}
	}

	// 3. Trich xuat cac header chu ky so (vi du Stripe-Signature)
	if stripeSig := r.Header.Get("Stripe-Signature"); stripeSig != "" {
		params["signature"] = stripeSig
		params["stripe-signature"] = stripeSig
	}
	if headerSig := r.Header.Get("X-Signature"); headerSig != "" {
		if _, exists := params["signature"]; !exists {
			params["signature"] = headerSig
		}
	}

	traceID := r.Header.Get("X-Trace-ID")
	if traceID == "" {
		traceID = r.Header.Get("X-Request-ID")
	}

	if h.processor == nil {
		httpx.WriteProblem(w, http.StatusNotImplemented, "chưa cấu hình webhook processor")
		return
	}

	res, err := h.processor.Execute(r.Context(), app.ProcessWebhookInput{
		Provider: domain.NormalizeProvider(providerStr),
		Params:   params,
		TraceID:  traceID,
	})

	if err != nil {
		switch {
		case errors.Is(err, domain.ErrInvalidSignature):
			httpx.WriteProblem(w, http.StatusBadRequest, "chữ ký số của cổng thanh toán không hợp lệ")
		case errors.Is(err, domain.ErrReplayDetected):
			httpx.WriteProblem(w, http.StatusBadRequest, "webhook quá cửa sổ thời gian cho phép (phát hiện replay attack)")
		case errors.Is(err, domain.ErrUnsupportedProvider):
			httpx.WriteProblem(w, http.StatusBadRequest, "cổng thanh toán không được hỗ trợ")
		case errors.Is(err, domain.ErrOrderNotFound):
			httpx.WriteProblem(w, http.StatusNotFound, "không tìm thấy đơn hàng")
		default:
			h.log.Error("xu ly webhook loi he thong",
				"provider", providerStr,
				"err", err,
				"trace_id", traceID,
			)
			httpx.WriteProblem(w, http.StatusInternalServerError, "lỗi xử lý webhook")
		}
		return
	}

	resp := WebhookResponse{
		Status:           "ok",
		OrderID:          res.OrderID,
		PaymentID:        res.PaymentID,
		AlreadyProcessed: res.AlreadyDone,
		Message:          res.Message,
	}

	httpx.WriteJSON(w, http.StatusOK, resp)
}
