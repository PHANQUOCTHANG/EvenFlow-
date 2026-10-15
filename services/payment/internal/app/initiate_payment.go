package app

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/eventflow/eventflow/services/payment/internal/domain"
	"github.com/eventflow/eventflow/services/payment/internal/port"
)

type InitiatePaymentInput struct {
	EventID        string
	OrderID        string
	HoldID         string
	AmountCents    int64
	Currency       string
	Method         domain.Provider
	BuyerName      string
	BuyerEmail     string
	BuyerPhone     string
	ReturnURL      string
	ClientIP       string
	IdempotencyKey string
}

type InitiatePaymentOutput struct {
	PaymentID     string `json:"payment_id"`
	OrderID       string `json:"order_id"`
	RedirectURL   string `json:"redirect_url"`
	Status        string `json:"status"`
	ProviderTxnID string `json:"provider_txn_id"`
}

type InitiatePaymentUseCase struct {
	repo     port.PaymentRepository
	registry port.GatewayRegistry
	clock    port.Clock
	idGen    port.IDGenerator
	log      *slog.Logger
}

func NewInitiatePaymentUseCase(
	repo port.PaymentRepository,
	registry port.GatewayRegistry,
	clock port.Clock,
	idGen port.IDGenerator,
	log *slog.Logger,
) *InitiatePaymentUseCase {
	if clock == nil {
		clock = port.SystemClock{}
	}
	return &InitiatePaymentUseCase{
		repo:     repo,
		registry: registry,
		clock:    clock,
		idGen:    idGen,
		log:      log,
	}
}

func (uc *InitiatePaymentUseCase) Execute(ctx context.Context, in InitiatePaymentInput) (InitiatePaymentOutput, error) {
	// 1. Kiem tra tham so co ban
	if in.OrderID == "" {
		return InitiatePaymentOutput{}, fmt.Errorf("%w: thieu order_id", domain.ErrInvalidInput)
	}
	if in.EventID == "" {
		return InitiatePaymentOutput{}, fmt.Errorf("%w: thieu event_id", domain.ErrInvalidInput)
	}
	if in.AmountCents <= 0 {
		return InitiatePaymentOutput{}, fmt.Errorf("%w: so tien phai lon hon 0", domain.ErrInvalidInput)
	}

	// 2. Doc thong tin don hang tu Repository
	order, err := uc.repo.GetOrder(ctx, in.OrderID)
	if err != nil {
		return InitiatePaymentOutput{}, err
	}

	if order.EventID != in.EventID {
		return InitiatePaymentOutput{}, fmt.Errorf("%w: don hang khong thuoc su kien nay", domain.ErrInvalidInput)
	}

	// 3. Kiem tra trang thai don hang
	if !order.IsHeld() {
		// Neu don da o trang thai PAYMENT_PENDING va da co payment ton tai, tra ve payment do
		if order.Status == "PAYMENT_PENDING" {
			existing, pErr := uc.repo.GetPaymentByOrder(ctx, in.OrderID)
			if pErr == nil && existing.ID != "" {
				gw, gErr := uc.registry.Get(existing.Provider)
				if gErr == nil {
					pRes, initErr := gw.InitiatePayment(ctx, port.PaymentRequest{
						EventID:     in.EventID,
						OrderID:     in.OrderID,
						HoldID:      in.HoldID,
						PaymentID:   existing.ID,
						AmountCents: existing.AmountCents,
						Currency:    existing.Currency,
						BuyerName:   in.BuyerName,
						BuyerEmail:  in.BuyerEmail,
						BuyerPhone:  in.BuyerPhone,
						ReturnURL:   in.ReturnURL,
						ClientIP:    in.ClientIP,
					})
					if initErr == nil {
						return InitiatePaymentOutput{
							PaymentID:     existing.ID,
							OrderID:       in.OrderID,
							RedirectURL:   pRes.RedirectURL,
							Status:        string(existing.Status),
							ProviderTxnID: existing.ProviderTxnID,
						}, nil
					}
				}
			}
		}
		return InitiatePaymentOutput{}, fmt.Errorf("%w: don hang o trang thai %s", domain.ErrOrderNotHeld, order.Status)
	}

	// 4. Chot chan giu ghe BR-O2: Kiem tra han giu ghe expires_at
	now := uc.clock.Now()
	if order.IsExpired(now) {
		uc.log.Warn("tu choi thanh toan vi hold da het han (BR-O2)",
			"order_id", in.OrderID,
			"expires_at", order.ExpiresAt,
			"now", now,
		)
		return InitiatePaymentOutput{}, domain.ErrHoldExpired
	}

	// 5. Kiem tra tinh toan ven cua so tien va loai tien te
	if in.AmountCents != order.TotalCents {
		return InitiatePaymentOutput{}, fmt.Errorf("%w: don yeu cau %d nhung nhan %d", domain.ErrAmountMismatch, order.TotalCents, in.AmountCents)
	}
	currency := in.Currency
	if currency == "" {
		currency = order.Currency
	}
	if currency == "" {
		currency = "VND"
	}

	// 6. Tim cong thanh toan tu Gateway Registry
	gw, err := uc.registry.Get(in.Method)
	if err != nil {
		return InitiatePaymentOutput{}, err
	}

	// 7. Sinh Payment ID va khoi tao qua Gateway Adapter
	paymentID := uc.idGen.NewID()
	req := port.PaymentRequest{
		EventID:     in.EventID,
		OrderID:     in.OrderID,
		HoldID:      in.HoldID,
		PaymentID:   paymentID,
		AmountCents: in.AmountCents,
		Currency:    currency,
		BuyerName:   in.BuyerName,
		BuyerEmail:  in.BuyerEmail,
		BuyerPhone:  in.BuyerPhone,
		ReturnURL:   in.ReturnURL,
		ClientIP:    in.ClientIP,
	}

	gwResult, err := gw.InitiatePayment(ctx, req)
	if err != nil {
		uc.log.Error("khoi tao qua cong thanh toan that bai",
			"provider", gw.Provider(),
			"order_id", in.OrderID,
			"err", err,
		)
		return InitiatePaymentOutput{}, fmt.Errorf("%w: %v", domain.ErrGatewayCommunication, err)
	}

	// 8. Luu Payment va chuyen trang thai don sang PAYMENT_PENDING trong DB Transaction (Quy tac 1)
	paymentRecord := domain.Payment{
		ID:            paymentID,
		OrderID:       in.OrderID,
		Provider:      gw.Provider(),
		ProviderTxnID: gwResult.ProviderTxnID,
		AmountCents:   in.AmountCents,
		Currency:      currency,
		Status:        domain.StatusPending,
		RawPayload:    gwResult.RawResponse,
		CreatedAt:     now,
	}

	if err := uc.repo.CreatePaymentAndMarkPending(ctx, paymentRecord); err != nil {
		uc.log.Error("luu payment va cap nhat don hang that bai",
			"order_id", in.OrderID,
			"payment_id", paymentID,
			"err", err,
		)
		return InitiatePaymentOutput{}, err
	}

	uc.log.Info("khoi tao thanh toan thanh cong",
		"payment_id", paymentID,
		"order_id", in.OrderID,
		"provider", gw.Provider(),
		"amount_cents", in.AmountCents,
		"masked_buyer", domain.MaskPII(in.BuyerEmail),
	)

	return InitiatePaymentOutput{
		PaymentID:     paymentID,
		OrderID:       in.OrderID,
		RedirectURL:   gwResult.RedirectURL,
		Status:        gwResult.Status,
		ProviderTxnID: gwResult.ProviderTxnID,
	}, nil
}
