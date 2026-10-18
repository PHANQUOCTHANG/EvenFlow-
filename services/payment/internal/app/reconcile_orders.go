package app

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/eventflow/eventflow/services/payment/internal/domain"
	"github.com/eventflow/eventflow/services/payment/internal/port"
)

// ReconcileStaleOrdersUseCase dieu phoi tien trinh quet va tu chua cac don hang treo qua 15 phut (EV-153 / EVF-73).
//
// Quy tac bat bien:
// 1. Khong sua DB ngoai transaction: Cap nhat orders, seat_holds, ticket_inventory, outbox_events luon nam trong DB Transaction an toan.
// 2. Khong goi API ben thu ba (cong thanh toan) ben trong DB transaction: QueryPayment duoc goi truoc/ngoai transaction.
// 3. Tra kho idempotent: seat_holds duoc giai phong qua chot chan WHERE released_at IS NULL AND consumed_at IS NULL.
// 4. Structured JSON Logging day du trace_id va masked PII.
type ReconcileStaleOrdersUseCase struct {
	repo           port.PaymentRepository
	registry       port.GatewayRegistry
	clock          port.Clock
	metrics        port.ReconciliationMetrics
	log            *slog.Logger
	staleThreshold time.Duration
}

// NewReconcileStaleOrdersUseCase khoi tao use case doi soat don treo.
func NewReconcileStaleOrdersUseCase(
	repo port.PaymentRepository,
	registry port.GatewayRegistry,
	clock port.Clock,
	metrics port.ReconciliationMetrics,
	log *slog.Logger,
) *ReconcileStaleOrdersUseCase {
	if clock == nil {
		clock = port.SystemClock{}
	}
	return &ReconcileStaleOrdersUseCase{
		repo:           repo,
		registry:       registry,
		clock:          clock,
		metrics:        metrics,
		log:            log,
		staleThreshold: 15 * time.Minute, // Nguong mac dinh 15 phut theo BR-O6 va Jira ticket EV-153
	}
}

// WithStaleThreshold cho phep tuy chinh nguong thoi gian ton dong de phat hien don treo (ho tro test).
func (uc *ReconcileStaleOrdersUseCase) WithStaleThreshold(threshold time.Duration) *ReconcileStaleOrdersUseCase {
	if threshold > 0 {
		uc.staleThreshold = threshold
	}
	return uc
}

// Execute thuc hien mot luot doi soat chu dong va tu chua cac don PAYMENT_PENDING bi treo.
func (uc *ReconcileStaleOrdersUseCase) Execute(ctx context.Context, limit int) (domain.ReconciliationResult, error) {
	startTime := uc.clock.Now()
	if limit <= 0 {
		limit = 50
	}

	staleOrders, err := uc.repo.GetStalePendingOrders(ctx, uc.staleThreshold, limit)
	if err != nil {
		uc.log.Error("truy van danh sach don treo that bai", "err", err)
		if uc.metrics != nil {
			uc.metrics.IncReconciliation("error")
		}
		return domain.ReconciliationResult{}, fmt.Errorf("truy van don treo that bai: %w", err)
	}

	result := domain.ReconciliationResult{
		TotalScanned: len(staleOrders),
	}

	for _, order := range staleOrders {
		select {
		case <-ctx.Done():
			result.Duration = uc.clock.Now().Sub(startTime)
			return result, ctx.Err()
		default:
		}

		traceID := fmt.Sprintf("recon-%s-%d", order.OrderID, uc.clock.Now().Unix())

		// Truong hop 1: Don khong co provider (khach chua tung chon cong thanh toan) -> Tu chua CANCELLED
		if order.Provider == "" {
			uc.log.Info("don treo khong co thong tin cong thanh toan, tu dong huy va tra kho",
				"order_id", order.OrderID,
				"trace_id", traceID,
			)

			cancelParams := domain.CancelOrderParams{
				OrderID: order.OrderID,
				TraceID: traceID,
				Reason:  "don hang treo qua 15 phut khong co giao dich thanh toan",
			}
			cancelRes, cErr := uc.repo.ProcessOrderCancellationTx(ctx, cancelParams)
			if cErr != nil {
				uc.log.Error("tu dong huy don treo khong provider that bai", "order_id", order.OrderID, "err", cErr)
				result.Errors++
				if uc.metrics != nil {
					uc.metrics.IncReconciliation("error")
				}
				continue
			}

			if cancelRes.AlreadyDone {
				result.Skipped++
				if uc.metrics != nil {
					uc.metrics.IncReconciliation("skipped")
				}
			} else {
				result.HealedCancelled++
				if uc.metrics != nil {
					uc.metrics.IncReconciliation("healed_cancelled")
				}
			}
			continue
		}

		// Truong hop 2: Lay gateway tu registry
		gw, gwErr := uc.registry.Get(order.Provider)
		if gwErr != nil {
			uc.log.Error("khong tim thay cong thanh toan tuong ung de tra cuu",
				"order_id", order.OrderID,
				"provider", order.Provider,
				"err", gwErr,
			)
			result.Errors++
			if uc.metrics != nil {
				uc.metrics.IncReconciliation("error")
			}
			continue
		}

		// Goi API tra cuu cong thanh toan NGOAI DB TRANSACTION (Quy tac bat bien 2)
		queryCtx, queryCancel := context.WithTimeout(ctx, 5*time.Second)
		queryReq := domain.PaymentQueryRequest{
			OrderID:       order.OrderID,
			PaymentID:     order.PaymentID,
			ProviderTxnID: order.ProviderTxnID,
		}
		qRes, qErr := gw.QueryPayment(queryCtx, queryReq)
		queryCancel()

		if qErr != nil {
			uc.log.Warn("loi ket noi toi cong thanh toan khi tra cuu, tam bo qua don nay",
				"order_id", order.OrderID,
				"provider", order.Provider,
				"err", qErr,
			)
			result.Skipped++
			if uc.metrics != nil {
				uc.metrics.IncReconciliation("skipped")
			}
			continue
		}

		// Xu ly ket qua tra cuu tu cong thanh toan
		switch qRes.Status {
		case domain.StatusSucceeded:
			// Cổng xác nhận khách đã trả tiền thành công -> Tự chữa sang PAID
			uc.log.Info("cong thanh toan xac nhan da thu tien, tu chua don sang PAID",
				"order_id", order.OrderID,
				"provider", order.Provider,
				"provider_txn_id", qRes.ProviderTxnID,
				"trace_id", traceID,
			)

			txnID := qRes.ProviderTxnID
			if txnID == "" {
				txnID = order.ProviderTxnID
			}

			successParams := domain.ProcessWebhookParams{
				Provider:      order.Provider,
				ProviderTxnID: txnID,
				OrderID:       order.OrderID,
				PaymentID:     order.PaymentID,
				AmountCents:   order.TotalCents,
				Currency:      order.Currency,
				Status:        domain.StatusSucceeded,
				RawPayload:    qRes.RawResponse,
				TraceID:       traceID,
				Timestamp:     uc.clock.Now(),
			}

			txRes, txErr := uc.repo.ProcessPaymentSuccessTx(ctx, successParams)
			if txErr != nil {
				uc.log.Error("tu chua don sang PAID that bai trong DB transaction",
					"order_id", order.OrderID,
					"err", txErr,
				)
				result.Errors++
				if uc.metrics != nil {
					uc.metrics.IncReconciliation("error")
				}
				continue
			}

			if txRes.AlreadyDone {
				result.Skipped++
				if uc.metrics != nil {
					uc.metrics.IncReconciliation("skipped")
				}
			} else {
				result.HealedPaid++
				if uc.metrics != nil {
					uc.metrics.IncReconciliation("healed_paid")
					// Tinh do tre cua webhook (thoi gian tu luc tao don den khi doi soat chua thanh cong)
					refTime := order.CreatedAt
					if !order.UpdatedAt.IsZero() {
						refTime = order.UpdatedAt
					}
					lag := uc.clock.Now().Sub(refTime)
					if lag > 0 {
						uc.metrics.ObserveWebhookLag(lag)
					}
				}
			}

		case domain.StatusFailed:
			// Cổng xác nhận giao dịch thất bại / khách hủy / hết hạn -> Tự chữa CANCELLED & Trả kho vé
			uc.log.Info("cong thanh toan xac nhan giao dich that bai, tu chua sang CANCELLED va tra kho",
				"order_id", order.OrderID,
				"provider", order.Provider,
				"trace_id", traceID,
			)

			cancelParams := domain.CancelOrderParams{
				OrderID:   order.OrderID,
				TraceID:   traceID,
				Reason:    fmt.Sprintf("doi soat: cong %s bao giao dich that bai hoac khong ton tai (%s)", order.Provider, qRes.Message),
				PaymentID: order.PaymentID,
			}

			cancelRes, cErr := uc.repo.ProcessOrderCancellationTx(ctx, cancelParams)
			if cErr != nil {
				uc.log.Error("tu chua don sang CANCELLED that bai trong DB transaction",
					"order_id", order.OrderID,
					"err", cErr,
				)
				result.Errors++
				if uc.metrics != nil {
					uc.metrics.IncReconciliation("error")
				}
				continue
			}

			if cancelRes.AlreadyDone {
				result.Skipped++
				if uc.metrics != nil {
					uc.metrics.IncReconciliation("skipped")
				}
			} else {
				result.HealedCancelled++
				if uc.metrics != nil {
					uc.metrics.IncReconciliation("healed_cancelled")
				}
			}

		case domain.StatusPending:
			// Giao dịch vẫn đang pending tại cổng -> Không vội hủy, giữ nguyên cho chu kỳ sau
			uc.log.Info("giao dich van dang xu ly tai cong, giu nguyen trang thai",
				"order_id", order.OrderID,
				"provider", order.Provider,
			)
			result.StillPending++
			if uc.metrics != nil {
				uc.metrics.IncReconciliation("still_pending")
			}

		default:
			uc.log.Warn("trang thai khong xac dinh tu cong thanh toan",
				"order_id", order.OrderID,
				"status", qRes.Status,
			)
			result.Skipped++
			if uc.metrics != nil {
				uc.metrics.IncReconciliation("skipped")
			}
		}
	}

	result.Duration = uc.clock.Now().Sub(startTime)
	if uc.metrics != nil {
		uc.metrics.ObserveReconciliationDuration(result.Duration)
	}

	return result, nil
}
