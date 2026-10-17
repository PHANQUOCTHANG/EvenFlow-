package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/eventflow/eventflow/services/payment/internal/domain"
	"github.com/eventflow/eventflow/services/payment/internal/port"
)

// ReplayPaymentInboxUseCase dieu phoi viec quet va xu ly lai cac webhook den som duoc luu trong payment_inbox (BR-O6).
// Use case bao gom: quet ban ghi pending, kiem tra don hang, cap nhat atomic sang PAID,
// ngan chan retry vo han qua maxAttempts va phat hien/canh bao ban ghi stale (> 15 phut).
type ReplayPaymentInboxUseCase struct {
	repo           port.PaymentRepository
	clock          port.Clock
	log            *slog.Logger
	maxAttempts    int
	staleThreshold time.Duration
}

// NewReplayPaymentInboxUseCase khoi tao use case replay inbox.
func NewReplayPaymentInboxUseCase(
	repo port.PaymentRepository,
	clock port.Clock,
	log *slog.Logger,
) *ReplayPaymentInboxUseCase {
	if clock == nil {
		clock = port.SystemClock{}
	}
	return &ReplayPaymentInboxUseCase{
		repo:           repo,
		clock:          clock,
		log:            log,
		maxAttempts:    5,                // Mac dinh toi da 5 lan thu truoc khi tam dung
		staleThreshold: 15 * time.Minute, // Nguong 15 phut theo BR-O6 va doi soat
	}
}

// WithMaxAttempts tuy chinh so lan thu toi da cho moi ban ghi.
func (uc *ReplayPaymentInboxUseCase) WithMaxAttempts(max int) *ReplayPaymentInboxUseCase {
	if max > 0 {
		uc.maxAttempts = max
	}
	return uc
}

// WithStaleThreshold tuy chinh nguong thoi gian ton dong de phat canh bao.
func (uc *ReplayPaymentInboxUseCase) WithStaleThreshold(threshold time.Duration) *ReplayPaymentInboxUseCase {
	if threshold > 0 {
		uc.staleThreshold = threshold
	}
	return uc
}

// Execute thuc hien mot luot quet va replay cac ban ghi payment_inbox dang cho xu ly.
func (uc *ReplayPaymentInboxUseCase) Execute(ctx context.Context, limit int) (domain.ReplayInboxResult, error) {
	if limit <= 0 {
		limit = 50
	}

	items, err := uc.repo.GetPendingPaymentInbox(ctx, limit, uc.maxAttempts)
	if err != nil {
		uc.log.Error("truy van danh sach pending payment_inbox that bai", "err", err)
		return domain.ReplayInboxResult{}, fmt.Errorf("truy van pending payment_inbox that bai: %w", err)
	}

	result := domain.ReplayInboxResult{
		TotalScanned: len(items),
	}

	for _, item := range items {
		// Trich xuat order_id tu payload
		orderID := ""
		if v, ok := item.Payload["order_id"].(string); ok {
			orderID = v
		} else if v, ok := item.Payload["orderId"].(string); ok {
			orderID = v
		} else if v, ok := item.Payload["vnp_OrderInfo"].(string); ok {
			orderID = v
		}

		if orderID == "" {
			uc.log.Warn("ban ghi payment_inbox thieu order_id trong payload",
				"inbox_id", item.ID,
				"provider", item.Provider,
				"provider_txn_id", item.ProviderTxnID,
			)
			_ = uc.repo.IncrementPaymentInboxAttempts(ctx, item.ID)
			result.Failed++
			continue
		}

		// Trich xuat amount_cents tu payload
		var amountCents int64
		switch v := item.Payload["amount_cents"].(type) {
		case int64:
			amountCents = v
		case float64:
			amountCents = int64(v)
		case int:
			amountCents = int64(v)
		case string:
			amountCents, _ = strconv.ParseInt(v, 10, 64)
		}

		// Xac dinh trang thai thanh toan
		statusStr := domain.StatusSucceeded
		if v, ok := item.Payload["status"].(string); ok && v != "" {
			statusStr = domain.PaymentStatus(v)
		}

		params := domain.ProcessWebhookParams{
			Provider:      item.Provider,
			ProviderTxnID: item.ProviderTxnID,
			OrderID:       orderID,
			AmountCents:   amountCents,
			Currency:      "VND",
			Status:        statusStr,
			RawPayload:    item.Payload,
			TraceID:       fmt.Sprintf("replay-inbox-%d", item.ID),
			Timestamp:     item.ReceivedAt,
		}

		if statusStr == domain.StatusSucceeded {
			res, txErr := uc.repo.ProcessPaymentSuccessTx(ctx, params)
			if txErr != nil {
				if errors.Is(txErr, domain.ErrOrderNotFound) {
					// Don hang van chua duoc tao, tang attempts va cho lan quet sau
					_ = uc.repo.IncrementPaymentInboxAttempts(ctx, item.ID)
					result.StillPending++
					uc.log.Debug("don hang van chua xuat hien khi replay inbox",
						"inbox_id", item.ID,
						"order_id", orderID,
						"provider_txn_id", item.ProviderTxnID,
						"current_attempts", item.Attempts+1,
					)
					continue
				}

				// Cac loi khac (vi du don khong o trang thai HELD)
				_ = uc.repo.IncrementPaymentInboxAttempts(ctx, item.ID)
				result.Failed++
				uc.log.Warn("replay payment_inbox gap loi nghiep vu hoac ha tang",
					"inbox_id", item.ID,
					"order_id", orderID,
					"provider_txn_id", item.ProviderTxnID,
					"err", txErr,
				)
				continue
			}

			// Thanh cong hoac da xu ly truoc do (idempotent AC-4)
			now := uc.clock.Now()
			_ = uc.repo.MarkPaymentInboxProcessed(ctx, item.ID, now)

			if res.AlreadyDone {
				result.AlreadyDone++
				uc.log.Info("replay payment_inbox: don hang da duoc thanh toan truoc do (idempotent)",
					"inbox_id", item.ID,
					"order_id", orderID,
					"provider_txn_id", item.ProviderTxnID,
				)
			} else {
				result.Succeeded++
				uc.log.Info("replay payment_inbox thanh cong: don hang da sang PAID va phat outbox",
					"inbox_id", item.ID,
					"order_id", orderID,
					"payment_id", res.PaymentID,
					"provider_txn_id", item.ProviderTxnID,
				)
			}
		} else {
			// Truong hop payment failed
			_, failErr := uc.repo.ProcessPaymentFailedTx(ctx, params)
			if failErr != nil {
				if errors.Is(failErr, domain.ErrOrderNotFound) {
					_ = uc.repo.IncrementPaymentInboxAttempts(ctx, item.ID)
					result.StillPending++
					continue
				}
				_ = uc.repo.IncrementPaymentInboxAttempts(ctx, item.ID)
				result.Failed++
				continue
			}
			now := uc.clock.Now()
			_ = uc.repo.MarkPaymentInboxProcessed(ctx, item.ID, now)
			result.Succeeded++
		}
	}

	// Kiem tra va canh bao cac ban ghi inbox ton dong qua lau (AC-6)
	staleRecords, _ := uc.CheckStaleRecords(ctx, limit)
	result.StaleDetected = len(staleRecords)

	return result, nil
}

// CheckStaleRecords tim kiem va ghi log canh bao co cau truc doi voi cac ban ghi payment_inbox chua xu ly qua 15 phut (AC-6).
func (uc *ReplayPaymentInboxUseCase) CheckStaleRecords(ctx context.Context, limit int) ([]domain.StaleInboxRecord, error) {
	if limit <= 0 {
		limit = 50
	}

	stale, err := uc.repo.GetStalePaymentInbox(ctx, uc.staleThreshold, limit)
	if err != nil {
		uc.log.Error("truy van stale payment_inbox that bai", "err", err)
		return nil, err
	}

	for _, rec := range stale {
		uc.log.Warn("phat hien ban ghi payment_inbox ton dong qua lau (BR-O6 / SLA doi soat)",
			"inbox_id", rec.ID,
			"provider", rec.Provider,
			"provider_txn_id", rec.ProviderTxnID,
			"received_at", rec.ReceivedAt,
			"age_seconds", rec.Age.Seconds(),
			"attempts", rec.Attempts,
		)
	}

	return stale, nil
}
