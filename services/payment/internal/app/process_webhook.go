package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/eventflow/eventflow/services/payment/internal/domain"
	"github.com/eventflow/eventflow/services/payment/internal/port"
)

// ProcessWebhookInput chua thong tin dau vao khi tiep nhan webhook tu cong thanh toan.
type ProcessWebhookInput struct {
	Provider domain.Provider
	Params   map[string]string
	TraceID  string
}

// ProcessWebhookUseCase dieu phoi quy trinh: xac thuc chu ky, chong replay attack qua cua so thoi gian,
// chong duplicate exactly-once (BR-O6) va thuc thi atomic DB transaction (Quy tac bat bien 1).
type ProcessWebhookUseCase struct {
	repo         port.PaymentRepository
	registry     port.GatewayRegistry
	clock        port.Clock
	log          *slog.Logger
	replayWindow time.Duration
	clockSkewMax time.Duration
}

// NewProcessWebhookUseCase khoi tao use case xu ly webhook.
func NewProcessWebhookUseCase(
	repo port.PaymentRepository,
	registry port.GatewayRegistry,
	clock port.Clock,
	log *slog.Logger,
) *ProcessWebhookUseCase {
	if clock == nil {
		clock = port.SystemClock{}
	}
	return &ProcessWebhookUseCase{
		repo:         repo,
		registry:     registry,
		clock:        clock,
		log:          log,
		replayWindow: 300 * time.Second, // 5 phut theo AC-3
		clockSkewMax: 60 * time.Second,  // 1 phut clock skew cho phep
	}
}

// WithReplayWindow cho phep tuy chinh cua so thoi gian kiem tra replay (dung cho test).
func (uc *ProcessWebhookUseCase) WithReplayWindow(window time.Duration, maxSkew time.Duration) *ProcessWebhookUseCase {
	uc.replayWindow = window
	uc.clockSkewMax = maxSkew
	return uc
}

func (uc *ProcessWebhookUseCase) Execute(ctx context.Context, in ProcessWebhookInput) (domain.WebhookProcessResult, error) {
	// 1. Chuan hoa va xac thuc Provider
	provider := domain.NormalizeProvider(string(in.Provider))
	if !domain.IsValidProvider(provider) {
		return domain.WebhookProcessResult{}, domain.ErrUnsupportedProvider
	}

	// 2. Tra cuu Gateway Adapter tu Registry
	gw, err := uc.registry.Get(provider)
	if err != nil {
		return domain.WebhookProcessResult{}, err
	}

	// 3. Xac thuc chu ky so tu cong thanh toan (AC-2)
	cb, err := gw.VerifyCallback(ctx, in.Params)
	if err != nil {
		return domain.WebhookProcessResult{}, fmt.Errorf("%w: %v", domain.ErrGatewayCommunication, err)
	}
	if !cb.Valid {
		uc.log.Warn("tu choi webhook: chu ky khong hop le",
			"provider", provider,
			"message", cb.Message,
			"trace_id", in.TraceID,
		)
		return domain.WebhookProcessResult{
			Success: false,
			Status:  domain.StatusFailed,
			Message: cb.Message,
		}, domain.ErrInvalidSignature
	}

	// 4. Kiem tra cua so thoi gian chong Replay Attack (AC-3)
	if !cb.Timestamp.IsZero() {
		now := uc.clock.Now()
		if now.Sub(cb.Timestamp) > uc.replayWindow {
			uc.log.Warn("tu choi webhook: qua cua so thoi gian cho phep (phat hien replay attack)",
				"provider", provider,
				"timestamp", cb.Timestamp,
				"now", now,
				"age_seconds", now.Sub(cb.Timestamp).Seconds(),
				"trace_id", in.TraceID,
			)
			return domain.WebhookProcessResult{
				Success: false,
				Status:  domain.StatusFailed,
				Message: "webhook qua cua so thoi gian cho phep (phat hien replay attack)",
			}, domain.ErrReplayDetected
		}

		if cb.Timestamp.Sub(now) > uc.clockSkewMax {
			uc.log.Warn("tu choi webhook: timestamp trong tuong lai vuot qua dung sai clock skew",
				"provider", provider,
				"timestamp", cb.Timestamp,
				"now", now,
				"future_seconds", cb.Timestamp.Sub(now).Seconds(),
				"trace_id", in.TraceID,
			)
			return domain.WebhookProcessResult{
				Success: false,
				Status:  domain.StatusFailed,
				Message: "timestamp webhook vuot qua dung sai clock skew tuong lai",
			}, domain.ErrReplayDetected
		}
	}

	// 5. Xac dinh OrderID va PaymentID
	orderID := cb.OrderID
	paymentID := cb.PaymentID
	if orderID == "" && paymentID != "" {
		if p, pErr := uc.repo.GetPayment(ctx, paymentID); pErr == nil {
			orderID = p.OrderID
		}
	}
	if orderID == "" && in.Params["order_id"] != "" {
		orderID = in.Params["order_id"]
	}
	if orderID == "" && in.Params["orderId"] != "" {
		orderID = in.Params["orderId"]
	}
	if orderID == "" && in.Params["vnp_OrderInfo"] != "" {
		orderID = in.Params["vnp_OrderInfo"]
	}
	if orderID == "" {
		return domain.WebhookProcessResult{}, fmt.Errorf("%w: thieu order_id trong du lieu webhook", domain.ErrInvalidInput)
	}

	params := domain.ProcessWebhookParams{
		Provider:      provider,
		ProviderTxnID: cb.ProviderTxnID,
		OrderID:       orderID,
		PaymentID:     paymentID,
		AmountCents:   cb.AmountCents,
		Currency:      "VND",
		Status:        cb.Status,
		RawPayload:    cb.RawPayload,
		TraceID:       in.TraceID,
		Timestamp:     cb.Timestamp,
	}

	// 6. Thuc thi DB Transaction an toan (Quy tac bat bien 1)
	if cb.Status == domain.StatusSucceeded {
		res, err := uc.repo.ProcessPaymentSuccessTx(ctx, params)
		if err != nil {
			if errors.Is(err, domain.ErrOrderNotFound) {
				// BR-O6: Webhook den truoc khi don ton tai -> luu payment_inbox va replay sau
				inboxPayload := make(map[string]any)
				for k, v := range cb.RawPayload {
					inboxPayload[k] = v
				}
				for k, v := range in.Params {
					if _, exists := inboxPayload[k]; !exists {
						inboxPayload[k] = v
					}
				}
				inboxPayload["order_id"] = orderID
				inboxPayload["provider_txn_id"] = cb.ProviderTxnID
				inboxPayload["amount_cents"] = cb.AmountCents
				inboxPayload["status"] = string(cb.Status)

				inboxItem := domain.PaymentInboxItem{
					Provider:      provider,
					ProviderTxnID: cb.ProviderTxnID,
					Payload:       inboxPayload,
					ReceivedAt:    uc.clock.Now(),
					Attempts:      0,
				}

				inboxID, saveErr := uc.repo.SavePaymentInbox(ctx, inboxItem)
				if saveErr != nil {
					uc.log.Error("luu payment_inbox that bai",
						"provider", provider,
						"order_id", orderID,
						"provider_txn_id", cb.ProviderTxnID,
						"err", saveErr,
						"trace_id", in.TraceID,
					)
					return domain.WebhookProcessResult{}, fmt.Errorf("luu payment_inbox that bai: %w", saveErr)
				}

				uc.log.Info("webhook den truoc khi don ton tai, da luu payment_inbox thanh cong de replay (BR-O6)",
					"inbox_id", inboxID,
					"provider", provider,
					"order_id", orderID,
					"provider_txn_id", cb.ProviderTxnID,
					"trace_id", in.TraceID,
				)

				return domain.WebhookProcessResult{
					Success:       true,
					AlreadyDone:   false,
					StoredInInbox: true,
					OrderID:       orderID,
					ProviderTxnID: cb.ProviderTxnID,
					Status:        domain.StatusPending,
					Message:       "webhook lưu vào payment_inbox thành công, chờ đơn xuất hiện để replay (BR-O6)",
				}, nil
			}

			uc.log.Error("xu ly transaction webhook thanh toan that bai",
				"provider", provider,
				"order_id", orderID,
				"provider_txn_id", cb.ProviderTxnID,
				"err", err,
				"trace_id", in.TraceID,
			)
			return res, err
		}

		if res.AlreadyDone {
			uc.log.Info("webhook trung lap da xu ly truoc do, bo qua no-op tra 200 (BR-O6 exactly-once)",
				"provider", provider,
				"order_id", orderID,
				"provider_txn_id", cb.ProviderTxnID,
				"trace_id", in.TraceID,
			)
		} else {
			uc.log.Info("xu ly webhook thanh toan thanh cong, don da sang PAID va phat outbox",
				"provider", provider,
				"order_id", orderID,
				"payment_id", res.PaymentID,
				"provider_txn_id", cb.ProviderTxnID,
				"trace_id", in.TraceID,
			)
		}
		return res, nil
	}

	// Truong hop giao dich that bai (AC-5)
	res, err := uc.repo.ProcessPaymentFailedTx(ctx, params)
	if err != nil {
		if errors.Is(err, domain.ErrOrderNotFound) {
			inboxPayload := make(map[string]any)
			for k, v := range cb.RawPayload {
				inboxPayload[k] = v
			}
			inboxPayload["order_id"] = orderID
			inboxPayload["provider_txn_id"] = cb.ProviderTxnID
			inboxPayload["amount_cents"] = cb.AmountCents
			inboxPayload["status"] = string(cb.Status)

			_, saveErr := uc.repo.SavePaymentInbox(ctx, domain.PaymentInboxItem{
				Provider:      provider,
				ProviderTxnID: cb.ProviderTxnID,
				Payload:       inboxPayload,
				ReceivedAt:    uc.clock.Now(),
				Attempts:      0,
			})
			if saveErr != nil {
				return domain.WebhookProcessResult{}, fmt.Errorf("luu payment_inbox failed that bai: %w", saveErr)
			}
			return domain.WebhookProcessResult{
				Success:       true,
				AlreadyDone:   false,
				StoredInInbox: true,
				OrderID:       orderID,
				ProviderTxnID: cb.ProviderTxnID,
				Status:        domain.StatusPending,
				Message:       "webhook failed lưu vào payment_inbox thành công để chờ đơn (BR-O6)",
			}, nil
		}

		uc.log.Error("ghi nhan payment failed that bai",
			"provider", provider,
			"order_id", orderID,
			"provider_txn_id", cb.ProviderTxnID,
			"err", err,
			"trace_id", in.TraceID,
		)
		return res, err
	}

	uc.log.Info("ghi nhan webhook bao giao dich that bai tu cong thanh toan",
		"provider", provider,
		"order_id", orderID,
		"provider_txn_id", cb.ProviderTxnID,
		"trace_id", in.TraceID,
	)
	return res, nil
}
