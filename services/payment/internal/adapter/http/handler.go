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
	"strconv"
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

// InboxReplayer dinh nghia giao dien replay va kiem tra payment_inbox.
type InboxReplayer interface {
	Execute(ctx context.Context, limit int) (domain.ReplayInboxResult, error)
	CheckStaleRecords(ctx context.Context, limit int) ([]domain.StaleInboxRecord, error)
}

// StaleOrderReconciler dinh nghia giao dien doi soat va tu chua don treo (EV-153).
type StaleOrderReconciler interface {
	Execute(ctx context.Context, limit int) (domain.ReconciliationResult, error)
}

// EventRefunder dinh nghia giao dien hoan tien cho su kien bi huy (EV-154 / EVF-74).
type EventRefunder interface {
	Execute(ctx context.Context, in app.RefundCancelledEventInput) (app.RefundCancelledEventOutput, error)
}

type Handler struct {
	initiator  PaymentInitiator
	getter     PaymentGetter
	lister     MethodsLister
	processor  WebhookProcessor
	replayer   InboxReplayer
	reconciler StaleOrderReconciler
	refunder   EventRefunder
	log        *slog.Logger
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

// WithInboxReplayer gan InboxReplayer cho Handler de kich hoat cac endpoint quan tri inbox.
func (h *Handler) WithInboxReplayer(replayer InboxReplayer) *Handler {
	h.replayer = replayer
	return h
}

// WithOrderReconciler gan StaleOrderReconciler cho Handler de kich hoat cac endpoint doi soat don treo.
func (h *Handler) WithOrderReconciler(reconciler StaleOrderReconciler) *Handler {
	h.reconciler = reconciler
	return h
}

// WithEventRefunder gan EventRefunder cho Handler de kich hoat endpoint hoan tien su kien bi huy (EV-154).
func (h *Handler) WithEventRefunder(refunder EventRefunder) *Handler {
	h.refunder = refunder
	return h
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

	// Endpoints Replay va giam sat Stale Inbox (EV-152 / BR-O6)
	if h.replayer != nil {
		mux.HandleFunc("POST /v1/payments/inbox/replay", h.handleReplayInbox)
		mux.HandleFunc("GET /v1/payments/inbox/stale", h.handleStaleInbox)
	}

	// Endpoints Doi soat don treo (EV-153 / EVF-73)
	if h.reconciler != nil {
		mux.HandleFunc("POST /v1/payments/reconcile", h.handleReconcileOrders)
		mux.HandleFunc("POST /internal/reconcile", h.handleReconcileOrders)
	}

	// Endpoints Hoan tien su kien bi huy (EV-154 / EVF-74)
	if h.refunder != nil {
		if idempotency != nil {
			mux.Handle("POST /v1/events/{eventID}/refund", idempotency(http.HandlerFunc(h.handleRefundEvent)))
		} else {
			mux.HandleFunc("POST /v1/events/{eventID}/refund", h.handleRefundEvent)
		}
		mux.HandleFunc("POST /internal/events/{eventID}/refund", h.handleRefundEvent)
	}
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
		StoredInInbox:    res.StoredInInbox,
		Message:          res.Message,
	}

	httpx.WriteJSON(w, http.StatusOK, resp)
}

func (h *Handler) handleReplayInbox(w http.ResponseWriter, r *http.Request) {
	if h.replayer == nil {
		httpx.WriteProblem(w, http.StatusNotImplemented, "chưa cấu hình inbox replayer")
		return
	}
	limit := 50
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}
	res, err := h.replayer.Execute(r.Context(), limit)
	if err != nil {
		h.log.Error("loi khi thuc thi replay inbox", "err", err)
		httpx.WriteProblem(w, http.StatusInternalServerError, "lỗi khi thực thi replay inbox")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, res)
}

func (h *Handler) handleStaleInbox(w http.ResponseWriter, r *http.Request) {
	if h.replayer == nil {
		httpx.WriteProblem(w, http.StatusNotImplemented, "chưa cấu hình inbox replayer")
		return
	}
	limit := 50
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}
	records, err := h.replayer.CheckStaleRecords(r.Context(), limit)
	if err != nil {
		h.log.Error("loi khi kiem tra stale inbox", "err", err)
		httpx.WriteProblem(w, http.StatusInternalServerError, "lỗi khi kiểm tra stale inbox")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"stale_records": records,
		"count":         len(records),
	})
}

func (h *Handler) handleReconcileOrders(w http.ResponseWriter, r *http.Request) {
	if h.reconciler == nil {
		httpx.WriteProblem(w, http.StatusNotImplemented, "chưa cấu hình order reconciler")
		return
	}

	limit := 50
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}

	result, err := h.reconciler.Execute(r.Context(), limit)
	if err != nil {
		h.log.Error("loi khi thuc thi doi soat don treo", "err", err)
		httpx.WriteProblem(w, http.StatusInternalServerError, "lỗi khi thực thi đối soát đơn treo")
		return
	}

	httpx.WriteJSON(w, http.StatusOK, result)
}

// RefundEventRequest chua tham so tuy chon khi kich hoat hoan tien su kien.
type RefundEventRequest struct {
	Reason    string `json:"reason"`
	BatchSize int    `json:"batch_size"`
}

func (h *Handler) handleRefundEvent(w http.ResponseWriter, r *http.Request) {
	if h.refunder == nil {
		httpx.WriteProblem(w, http.StatusNotImplemented, "chưa cấu hình event refunder")
		return
	}

	eventID := r.PathValue("eventID")
	if eventID == "" {
		httpx.WriteProblem(w, http.StatusBadRequest, "thiếu eventID trong đường dẫn")
		return
	}

	var req RefundEventRequest
	if r.Body != nil && r.ContentLength > 0 {
		_ = json.NewDecoder(r.Body).Decode(&req)
	}

	traceID := r.Header.Get("X-Trace-ID")
	if traceID == "" {
		traceID = fmt.Sprintf("refund-http-%s-%d", eventID, time.Now().UnixNano())
	}

	result, err := h.refunder.Execute(r.Context(), app.RefundCancelledEventInput{
		EventID:   eventID,
		Reason:    req.Reason,
		TraceID:   traceID,
		BatchSize: req.BatchSize,
	})
	if err != nil {
		if errors.Is(err, domain.ErrInvalidInput) {
			httpx.WriteProblem(w, http.StatusBadRequest, err.Error())
			return
		}
		h.log.Error("loi khi thuc thi hoan tien su kien bi huy", "event_id", eventID, "err", err)
		httpx.WriteProblem(w, http.StatusInternalServerError, "lỗi khi thực thi hoàn tiền sự kiện")
		return
	}

	httpx.WriteJSON(w, http.StatusOK, result)
}
