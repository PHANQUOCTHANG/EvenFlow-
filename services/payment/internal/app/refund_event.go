// Package app chua cac use case dieu phoi nghiep vu cua payment service theo kien truc Hexagonal.
package app

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/eventflow/eventflow/services/payment/internal/domain"
	"github.com/eventflow/eventflow/services/payment/internal/port"
)

// RefundCancelledEventInput chua tham so dau vao cho use case hoan tien su kien bi huy (EV-154 / EVF-74).
type RefundCancelledEventInput struct {
	EventID   string `json:"event_id"`
	Reason    string `json:"reason"`
	TraceID   string `json:"trace_id"`
	BatchSize int    `json:"batch_size"`
}

// RefundCancelledEventOutput chua ket qua tong hop sau khi thuc hien hoan tien theo lo.
type RefundCancelledEventOutput struct {
	EventID       string                       `json:"event_id"`
	TotalOrders   int                          `json:"total_orders"`
	RefundedCount int                          `json:"refunded_count"`
	FailedCount   int                          `json:"failed_count"`
	SkippedCount  int                          `json:"skipped_count"`
	TotalAmount   int64                        `json:"total_amount"`
	Duration      time.Duration                `json:"duration"`
	OrderResults  []domain.ProcessRefundResult `json:"order_results,omitempty"`
}

// RefundCancelledEventUseCase dieu phoi toan bo luong hoan tien khi su kien bi huy (EV-154 / EVF-74).
//
// 4 Quy tac bat bien:
//  1. Tuyet doi khong sua DB ngoai transaction: orders -> REFUNDED, payments -> REFUNDED, tickets -> REFUNDED
//     va outbox_events duoc thuc thi trong CUNG MOT TRANSACTION DATABASE an toan.
//  2. Khong goi API ben thu ba chan luong HTTP: RefundPayment toi cong thanh toan duoc goi NGOAI DB transaction,
//     co context timeout chat che.
//  3. Bao toan tinh toan ven: Idempotent hoan tien, moi don chi hoan tien 1 lan, giai phong quota nguoi mua (BR-O4).
//  4. Structured JSON Logging: moi log co day du trace_id va masked PII.
type RefundCancelledEventUseCase struct {
	repo     port.PaymentRepository
	registry port.GatewayRegistry
	clock    port.Clock
	log      *slog.Logger
}

// NewRefundCancelledEventUseCase khoi tao use case hoan tien cho su kien bi huy.
func NewRefundCancelledEventUseCase(
	repo port.PaymentRepository,
	registry port.GatewayRegistry,
	clock port.Clock,
	log *slog.Logger,
) *RefundCancelledEventUseCase {
	if clock == nil {
		clock = port.SystemClock{}
	}
	return &RefundCancelledEventUseCase{
		repo:     repo,
		registry: registry,
		clock:    clock,
		log:      log,
	}
}

// Execute thuc hien quy trinh hoan tien tu dong cho tat ca cac don hang da thanh toan thuoc su kien bi huy.
func (uc *RefundCancelledEventUseCase) Execute(ctx context.Context, in RefundCancelledEventInput) (RefundCancelledEventOutput, error) {
	startTime := uc.clock.Now()

	if in.EventID == "" {
		return RefundCancelledEventOutput{}, fmt.Errorf("%w: event_id khong duoc de trong", domain.ErrInvalidInput)
	}
	if in.Reason == "" {
		in.Reason = "Sự kiện bị hủy bởi ban tổ chức"
	}
	if in.TraceID == "" {
		in.TraceID = fmt.Sprintf("refund-%s-%d", in.EventID, uc.clock.Now().UnixNano())
	}
	if in.BatchSize <= 0 {
		in.BatchSize = 50
	}

	uc.log.Info("bat dau tien trinh hoan tien cho su kien bi huy",
		"event_id", in.EventID,
		"reason", in.Reason,
		"trace_id", in.TraceID,
		"batch_size", in.BatchSize,
	)

	// 1. Quet danh sach cac don hang can hoan tien (PAID, ISSUED, REFUNDING)
	orders, err := uc.repo.GetPaidOrdersByEvent(ctx, in.EventID, in.BatchSize)
	if err != nil {
		uc.log.Error("truy van danh sach don hang can hoan tien that bai",
			"event_id", in.EventID,
			"trace_id", in.TraceID,
			"err", err,
		)
		return RefundCancelledEventOutput{}, fmt.Errorf("truy van don hang can hoan tien that bai: %w", err)
	}

	out := RefundCancelledEventOutput{
		EventID:      in.EventID,
		TotalOrders:  len(orders),
		OrderResults: make([]domain.ProcessRefundResult, 0, len(orders)),
	}

	// 2. Xu ly hoan tien cho tung don hang
	for _, order := range orders {
		select {
		case <-ctx.Done():
			out.Duration = uc.clock.Now().Sub(startTime)
			return out, ctx.Err()
		default:
		}

		orderTraceID := fmt.Sprintf("%s-%s", in.TraceID, order.OrderID)

		// Bo qua neu don da REFUNDED
		if order.Status == "REFUNDED" {
			out.SkippedCount++
			out.OrderResults = append(out.OrderResults, domain.ProcessRefundResult{
				OrderID:     order.OrderID,
				PaymentID:   order.PaymentID,
				AlreadyDone: true,
				Status:      "REFUNDED",
				Message:     "don hang da o trang thai REFUNDED truoc do",
			})
			continue
		}

		// Danh dau trang thai REFUNDING
		if order.Status != "REFUNDING" {
			if mErr := uc.repo.MarkOrderRefundingTx(ctx, order.OrderID); mErr != nil {
				uc.log.Warn("danh dau don hang REFUNDING that bai, bo qua de giu an toan",
					"order_id", order.OrderID,
					"err", mErr,
				)
				out.FailedCount++
				continue
			}
		}

		// Xac dinh cong thanh toan
		provider := order.Provider
		if provider == "" {
			provider = uc.registry.DefaultProvider()
		}

		gw, gwErr := uc.registry.Get(provider)
		if gwErr != nil {
			uc.log.Error("khong the phan giai cong thanh toan de hoan tien",
				"order_id", order.OrderID,
				"provider", provider,
				"err", gwErr,
			)
			_ = uc.repo.ProcessRefundFailedTx(ctx, order.OrderID, order.PaymentID, gwErr.Error())
			out.FailedCount++
			continue
		}

		// Goi API cong thanh toan NGOAI DB transaction (Quy tac bat bien 2)
		refundReq := domain.PaymentRefundRequest{
			PaymentID:     order.PaymentID,
			OrderID:       order.OrderID,
			ProviderTxnID: order.ProviderTxnID,
			AmountCents:   order.TotalCents,
			Currency:      order.Currency,
			Reason:        in.Reason,
			RefundID:      fmt.Sprintf("ref-%s", order.OrderID),
		}

		refundRes, rErr := gw.RefundPayment(ctx, refundReq)
		if rErr != nil || refundRes.Status == domain.StatusFailed {
			errMsg := "cong thanh toan tu choi hoan tien"
			if rErr != nil {
				errMsg = rErr.Error()
			} else if refundRes.Message != "" {
				errMsg = refundRes.Message
			}

			uc.log.Error("hoan tien qua cong thanh toan that bai",
				"order_id", order.OrderID,
				"provider", provider,
				"err", errMsg,
				"trace_id", orderTraceID,
			)

			_ = uc.repo.ProcessRefundFailedTx(ctx, order.OrderID, order.PaymentID, errMsg)
			out.FailedCount++
			continue
		}

		// Cap nhat DB atomic trong transaction: orders -> REFUNDED, payments -> REFUNDED, outbox -> payment.refunded (Quy tac bat bien 1)
		processParams := domain.ProcessRefundParams{
			OrderID:     order.OrderID,
			PaymentID:   order.PaymentID,
			Provider:    provider,
			RefundTxnID: refundRes.RefundTxnID,
			AmountCents: order.TotalCents,
			Reason:      in.Reason,
			TraceID:     orderTraceID,
			Timestamp:   uc.clock.Now(),
		}

		dbRes, dbErr := uc.repo.ProcessRefundSuccessTx(ctx, processParams)
		if dbErr != nil {
			uc.log.Error("cap nhat trang thai hoan tien DB that bai",
				"order_id", order.OrderID,
				"err", dbErr,
				"trace_id", orderTraceID,
			)
			out.FailedCount++
			continue
		}

		if dbRes.AlreadyDone {
			out.SkippedCount++
		} else {
			out.RefundedCount++
			out.TotalAmount += order.TotalCents
		}
		out.OrderResults = append(out.OrderResults, dbRes)

		uc.log.Info("hoan tien thanh cong cho don hang",
			"order_id", order.OrderID,
			"amount_cents", order.TotalCents,
			"refund_txn_id", refundRes.RefundTxnID,
			"already_done", dbRes.AlreadyDone,
			"trace_id", orderTraceID,
		)
	}

	out.Duration = uc.clock.Now().Sub(startTime)

	uc.log.Info("hoan tat tien trinh hoan tien su kien bi huy",
		"event_id", in.EventID,
		"total_orders", out.TotalOrders,
		"refunded", out.RefundedCount,
		"failed", out.FailedCount,
		"skipped", out.SkippedCount,
		"total_amount", out.TotalAmount,
		"duration", out.Duration,
	)

	return out, nil
}
