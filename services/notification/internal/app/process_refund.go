package app

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/eventflow/eventflow/services/notification/internal/domain"
	"github.com/eventflow/eventflow/services/notification/internal/port"
)

type ProcessRefundUseCase struct {
	emailSender port.EmailSender
	smsSender   port.SMSSender
	renderer    port.TemplateRenderer
	store       port.NotificationStore
	logger      *slog.Logger
}

func NewProcessRefundUseCase(
	emailSender port.EmailSender,
	smsSender port.SMSSender,
	renderer port.TemplateRenderer,
	store port.NotificationStore,
	logger *slog.Logger,
) *ProcessRefundUseCase {
	if logger == nil {
		logger = slog.Default()
	}
	return &ProcessRefundUseCase{
		emailSender: emailSender,
		smsSender:   smsSender,
		renderer:    renderer,
		store:       store,
		logger:      logger,
	}
}

func (uc *ProcessRefundUseCase) Execute(ctx context.Context, evt domain.PaymentRefundedEvent) (ProcessResult, error) {
	if err := evt.Validate(); err != nil {
		uc.logger.Error("invalid payment.refunded event payload", "err", err, "order_id", evt.OrderID)
		return ProcessResult{}, fmt.Errorf("%w: %w", domain.ErrInvalidEventPayload, err)
	}

	var res ProcessResult

	// 1. Email thong bao hoan tien
	if evt.RecipientEmail != "" {
		dedupKey := domain.MakeDedupKey("payment.refunded", evt.OrderID, domain.ChannelEmail)
		notif := domain.Notification{
			ID:        uuid.New().String(),
			DedupKey:  dedupKey,
			EventType: "payment.refunded",
			OrderID:   evt.OrderID,
			Recipient: evt.RecipientEmail,
			Channel:   domain.ChannelEmail,
			CreatedAt: time.Now(),
		}

		alreadyProcessed, err := uc.store.CheckOrSetDedup(ctx, dedupKey, notif)
		if err != nil {
			return res, fmt.Errorf("check dedup email: %w", err)
		}

		if alreadyProcessed {
			uc.logger.Info("payment.refunded email already processed, skipping duplicate", "order_id", evt.OrderID, "dedup_key", dedupKey)
			res.EmailSkipped = true
		} else {
			subject, htmlBody, textBody, err := uc.renderer.RenderRefundEmail(evt)
			if err != nil {
				_ = uc.store.MarkFailed(ctx, dedupKey, notif.ID, err.Error())
				return res, fmt.Errorf("render refund email: %w", err)
			}

			emailMsg := domain.EmailMessage{
				To:       evt.RecipientEmail,
				Subject:  subject,
				HTMLBody: htmlBody,
				TextBody: textBody,
			}

			if err := uc.emailSender.SendEmail(ctx, emailMsg); err != nil {
				_ = uc.store.MarkFailed(ctx, dedupKey, notif.ID, err.Error())
				uc.logger.Error("failed to send refund email", "err", err, "order_id", evt.OrderID)
				return res, fmt.Errorf("send email: %w", err)
			}

			_ = uc.store.MarkSent(ctx, dedupKey, notif.ID, time.Now())
			uc.logger.Info("payment.refunded email sent successfully", "order_id", evt.OrderID, "to", evt.RecipientEmail)
			res.EmailSent = true
		}
	}

	// 2. SMS thong bao hoan tien
	if evt.RecipientPhone != "" {
		dedupKey := domain.MakeDedupKey("payment.refunded", evt.OrderID, domain.ChannelSMS)
		notif := domain.Notification{
			ID:        uuid.New().String(),
			DedupKey:  dedupKey,
			EventType: "payment.refunded",
			OrderID:   evt.OrderID,
			Recipient: evt.RecipientPhone,
			Channel:   domain.ChannelSMS,
			CreatedAt: time.Now(),
		}

		alreadyProcessed, err := uc.store.CheckOrSetDedup(ctx, dedupKey, notif)
		if err != nil {
			return res, fmt.Errorf("check dedup sms: %w", err)
		}

		if alreadyProcessed {
			uc.logger.Info("payment.refunded sms already processed, skipping duplicate", "order_id", evt.OrderID, "dedup_key", dedupKey)
			res.SMSSkipped = true
		} else {
			smsBody, err := uc.renderer.RenderSMS("payment.refunded", evt)
			if err != nil {
				_ = uc.store.MarkFailed(ctx, dedupKey, notif.ID, err.Error())
				return res, fmt.Errorf("render refund sms: %w", err)
			}

			smsMsg := domain.SMSMessage{
				To:          evt.RecipientPhone,
				MessageBody: smsBody,
			}

			if err := uc.smsSender.SendSMS(ctx, smsMsg); err != nil {
				_ = uc.store.MarkFailed(ctx, dedupKey, notif.ID, err.Error())
				uc.logger.Error("failed to send refund sms", "err", err, "order_id", evt.OrderID)
				return res, fmt.Errorf("send sms: %w", err)
			}

			_ = uc.store.MarkSent(ctx, dedupKey, notif.ID, time.Now())
			uc.logger.Info("payment.refunded sms sent successfully", "order_id", evt.OrderID, "to", evt.RecipientPhone)
			res.SMSSent = true
		}
	}

	return res, nil
}
