package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/eventflow/eventflow/libs/go/mq"
	"github.com/eventflow/eventflow/libs/go/otelx"
	"github.com/eventflow/eventflow/services/ticketing/internal/domain"
	"github.com/eventflow/eventflow/services/ticketing/internal/port"
)

// TicketIssuer la use case phat ve (app.IssueTicketsUseCase). Use case idempotent theo order_id:
// don da ISSUED thi tra lai ve cu, nen message order.paid giao lap khong tao ve thu hai.
type TicketIssuer interface {
	Execute(ctx context.Context, in port.IssueTicketsParams) (port.IssueTicketsResult, error)
}

// orderPaidPayload: phan cua domain.OutboxOrderPaidPayload (payment-svc) ma ticketing can.
type orderPaidPayload struct {
	OrderID string `json:"order_id"`
}

// OrderPaidHandler phat ve khi nhan order.paid tu payment.events (EV-133). Dung lam mq.Handler.
type OrderPaidHandler struct {
	issuer TicketIssuer
	log    *slog.Logger
}

// NewOrderPaidHandler tao handler.
func NewOrderPaidHandler(issuer TicketIssuer, log *slog.Logger) *OrderPaidHandler {
	return &OrderPaidHandler{issuer: issuer, log: log}
}

// Handle phat ve cho don trong message.
//
// Loi vinh vien (vao DLQ, khong retry): payload hong, don khong ton tai, don khong con PAID
// (vd da hoan tien truoc khi message toi -- khong duoc phat ve). Payment ghi outbox trong
// CUNG transaction dat don PAID, nen luc message toi don da commit: "khong tim thay" la loi
// du lieu, khong phai do den som. Loi khac (DB tam mat) thi retry.
func (h *OrderPaidHandler) Handle(ctx context.Context, d mq.Delivery) error {
	var p orderPaidPayload
	if err := json.Unmarshal(d.Body, &p); err != nil {
		return fmt.Errorf("%w: payload order.paid hong: %w", mq.ErrPermanent, err)
	}
	if strings.TrimSpace(p.OrderID) == "" {
		return fmt.Errorf("%w: order.paid thieu order_id", mq.ErrPermanent)
	}

	traceID, _ := d.Headers["x-trace-id"].(string)
	if tp := otelx.Traceparent(ctx); tp != "" {
		// Luu vao outbox cua ticketing.ticket.issued de relay noi tiep cung trace toi notification (EV-103).
		traceID = tp
	}
	res, err := h.issuer.Execute(ctx, port.IssueTicketsParams{OrderID: p.OrderID, TraceID: traceID})
	switch {
	case errors.Is(err, domain.ErrOrderNotFound), errors.Is(err, domain.ErrOrderNotPaid):
		h.log.Error("khong phat ve cho order.paid", "order_id", p.OrderID, "message_id", d.MessageID, "err", err)
		return fmt.Errorf("%w: %w", mq.ErrPermanent, err)
	case err != nil:
		return fmt.Errorf("phat ve cho don %s: %w", p.OrderID, err)
	}

	h.log.Info("da phat ve tu order.paid",
		"order_id", p.OrderID, "so_ve", len(res.Tickets), "da_phat_truoc", res.AlreadyIssued,
		"message_id", d.MessageID, "lan_thu", d.Attempt)
	return nil
}

// OrderPaidConsumerConfig la cau hinh mq.Consume cho hang ticketing.order-paid.q.
func OrderPaidConsumerConfig() mq.ConsumerConfig {
	return mq.ConsumerConfig{
		Queue:              mq.TicketingOrderPaidQueue,
		MaxAttempts:        5,
		RetryExchange:      mq.TicketingRetryExchange,
		RetryKey:           mq.TicketingOrderPaidKey,
		DeadLetterExchange: mq.TicketingDeadLetterEx,
		DeadLetterKey:      mq.TicketingOrderPaidKey,
		Prefetch:           10,
	}
}
