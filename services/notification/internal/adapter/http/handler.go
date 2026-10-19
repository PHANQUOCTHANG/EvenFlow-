package http

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/eventflow/eventflow/services/notification/internal/app"
	"github.com/eventflow/eventflow/services/notification/internal/domain"
)

type NotificationHandler struct {
	orderPaidUC    *app.ProcessOrderPaidUseCase
	ticketIssuedUC *app.ProcessTicketIssuedUseCase
	refundUC       *app.ProcessRefundUseCase
	getHistoryUC   *app.GetNotificationHistoryUseCase
}

func NewNotificationHandler(
	orderPaidUC *app.ProcessOrderPaidUseCase,
	ticketIssuedUC *app.ProcessTicketIssuedUseCase,
	refundUC *app.ProcessRefundUseCase,
	getHistoryUC *app.GetNotificationHistoryUseCase,
) *NotificationHandler {
	return &NotificationHandler{
		orderPaidUC:    orderPaidUC,
		ticketIssuedUC: ticketIssuedUC,
		refundUC:       refundUC,
		getHistoryUC:   getHistoryUC,
	}
}

func (h *NotificationHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /healthz", h.HandleHealthz)
	mux.HandleFunc("POST /v1/notifications/send", h.HandleManualSend)
	mux.HandleFunc("GET /v1/notifications/history/{order_id}", h.HandleGetHistory)
}

func (h *NotificationHandler) HandleHealthz(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

type manualSendRequest struct {
	EventType      string                `json:"event_type"`
	OrderID        string                `json:"order_id"`
	EventID        string                `json:"event_id"`
	EventTitle     string                `json:"event_title,omitempty"`
	IdentityID     string                `json:"identity_id"`
	AmountCents    int64                 `json:"amount_cents,omitempty"`
	Currency       string                `json:"currency,omitempty"`
	Provider       string                `json:"provider,omitempty"`
	ProviderTxnID  string                `json:"provider_txn_id,omitempty"`
	RecipientEmail string                `json:"recipient_email,omitempty"`
	RecipientPhone string                `json:"recipient_phone,omitempty"`
	Tickets        []domain.TicketDetail `json:"tickets,omitempty"`
	Reason         string                `json:"reason,omitempty"`
}

func (h *NotificationHandler) HandleManualSend(w http.ResponseWriter, r *http.Request) {
	var req manualSendRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid json body"}`, http.StatusBadRequest)
		return
	}

	req.EventType = strings.TrimSpace(req.EventType)
	req.OrderID = strings.TrimSpace(req.OrderID)

	if req.EventType == "" || req.OrderID == "" {
		http.Error(w, `{"error":"event_type and order_id are required"}`, http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	var res app.ProcessResult
	var err error

	switch req.EventType {
	case "order.paid":
		evt := domain.OrderPaidEvent{
			OrderID:        req.OrderID,
			EventID:        req.EventID,
			EventTitle:     req.EventTitle,
			IdentityID:     req.IdentityID,
			AmountCents:    req.AmountCents,
			Currency:       req.Currency,
			Provider:       req.Provider,
			ProviderTxnID:  req.ProviderTxnID,
			RecipientEmail: req.RecipientEmail,
			RecipientPhone: req.RecipientPhone,
		}
		res, err = h.orderPaidUC.Execute(ctx, evt)

	case "ticket.issued", "ticketing.ticket.issued":
		evt := domain.TicketIssuedEvent{
			OrderID:        req.OrderID,
			EventID:        req.EventID,
			EventTitle:     req.EventTitle,
			IdentityID:     req.IdentityID,
			TicketCount:    len(req.Tickets),
			Tickets:        req.Tickets,
			RecipientEmail: req.RecipientEmail,
			RecipientPhone: req.RecipientPhone,
		}
		res, err = h.ticketIssuedUC.Execute(ctx, evt)

	case "payment.refunded":
		evt := domain.PaymentRefundedEvent{
			OrderID:        req.OrderID,
			EventID:        req.EventID,
			EventTitle:     req.EventTitle,
			IdentityID:     req.IdentityID,
			AmountCents:    req.AmountCents,
			Currency:       req.Currency,
			Reason:         req.Reason,
			RecipientEmail: req.RecipientEmail,
			RecipientPhone: req.RecipientPhone,
		}
		res, err = h.refundUC.Execute(ctx, evt)

	default:
		http.Error(w, `{"error":"unsupported event_type"}`, http.StatusBadRequest)
		return
	}

	if err != nil {
		if errors.Is(err, domain.ErrInvalidEventPayload) || errors.Is(err, domain.ErrInvalidRecipient) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status": "success",
		"result": res,
	})
}

func (h *NotificationHandler) HandleGetHistory(w http.ResponseWriter, r *http.Request) {
	orderID := r.PathValue("order_id")
	if strings.TrimSpace(orderID) == "" {
		http.Error(w, `{"error":"order_id is required"}`, http.StatusBadRequest)
		return
	}

	history, err := h.getHistoryUC.Execute(r.Context(), orderID)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"order_id":      orderID,
		"notifications": history,
	})
}
