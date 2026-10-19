package rabbitmq

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/eventflow/eventflow/libs/go/mq"
	"github.com/eventflow/eventflow/services/notification/internal/app"
	"github.com/eventflow/eventflow/services/notification/internal/domain"
)

// NotificationEventConsumer dieu phoi tiep nhan va xu ly cac message tu RabbitMQ.
type NotificationEventConsumer struct {
	orderPaidUC    *app.ProcessOrderPaidUseCase
	ticketIssuedUC *app.ProcessTicketIssuedUseCase
	refundUC       *app.ProcessRefundUseCase
	logger         *slog.Logger
}

func NewNotificationEventConsumer(
	orderPaidUC *app.ProcessOrderPaidUseCase,
	ticketIssuedUC *app.ProcessTicketIssuedUseCase,
	refundUC *app.ProcessRefundUseCase,
	logger *slog.Logger,
) *NotificationEventConsumer {
	if logger == nil {
		logger = slog.Default()
	}
	return &NotificationEventConsumer{
		orderPaidUC:    orderPaidUC,
		ticketIssuedUC: ticketIssuedUC,
		refundUC:       refundUC,
		logger:         logger,
	}
}

// HandleDelivery xu ly mot message nhan duoc tu RabbitMQ theo chuan mq.Handler.
func (c *NotificationEventConsumer) HandleDelivery(ctx context.Context, d mq.Delivery) error {
	traceID, _ := d.Headers["x-trace-id"].(string)
	c.logger.Info("received message from mq",
		"routing_key", d.RoutingKey,
		"message_id", d.MessageID,
		"attempt", d.Attempt,
		"trace_id", traceID,
	)

	switch d.RoutingKey {
	case "order.paid":
		var evt domain.OrderPaidEvent
		if err := json.Unmarshal(d.Body, &evt); err != nil {
			c.logger.Error("failed to unmarshal order.paid payload (poison pill, route to DLQ)", "err", err)
			return fmt.Errorf("%w: unmarshal order.paid: %w", mq.ErrPermanent, err)
		}

		_, err := c.orderPaidUC.Execute(ctx, evt)
		if err != nil {
			if errors.Is(err, domain.ErrInvalidEventPayload) || errors.Is(err, domain.ErrInvalidRecipient) {
				return fmt.Errorf("%w: %w", mq.ErrPermanent, err)
			}
			return err
		}
		return nil

	case "ticket.issued", "ticketing.ticket.issued":
		var evt domain.TicketIssuedEvent
		if err := json.Unmarshal(d.Body, &evt); err != nil {
			c.logger.Error("failed to unmarshal ticket.issued payload (poison pill, route to DLQ)", "err", err)
			return fmt.Errorf("%w: unmarshal ticket.issued: %w", mq.ErrPermanent, err)
		}

		_, err := c.ticketIssuedUC.Execute(ctx, evt)
		if err != nil {
			if errors.Is(err, domain.ErrInvalidEventPayload) || errors.Is(err, domain.ErrInvalidRecipient) {
				return fmt.Errorf("%w: %w", mq.ErrPermanent, err)
			}
			return err
		}
		return nil

	case "payment.refunded":
		var evt domain.PaymentRefundedEvent
		if err := json.Unmarshal(d.Body, &evt); err != nil {
			c.logger.Error("failed to unmarshal payment.refunded payload (poison pill, route to DLQ)", "err", err)
			return fmt.Errorf("%w: unmarshal payment.refunded: %w", mq.ErrPermanent, err)
		}

		_, err := c.refundUC.Execute(ctx, evt)
		if err != nil {
			if errors.Is(err, domain.ErrInvalidEventPayload) || errors.Is(err, domain.ErrInvalidRecipient) {
				return fmt.Errorf("%w: %w", mq.ErrPermanent, err)
			}
			return err
		}
		return nil

	default:
		c.logger.Warn("unsupported routing key, routing to DLQ", "routing_key", d.RoutingKey)
		return fmt.Errorf("%w: unsupported routing key: %s", mq.ErrPermanent, d.RoutingKey)
	}
}
