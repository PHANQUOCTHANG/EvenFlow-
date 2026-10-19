package postgres

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/eventflow/eventflow/services/payment/internal/domain"
)

// MemoryRepository cung cap kho luu tru in-memory phuc vu chay local va unit test.
type MemoryRepository struct {
	mu          sync.RWMutex
	orders      map[string]domain.OrderView
	payments    map[string]domain.Payment
	outbox      []map[string]any
	inbox       map[int64]domain.PaymentInboxItem
	nextInboxID int64
	clock       func() time.Time
}

func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{
		orders:   make(map[string]domain.OrderView),
		payments: make(map[string]domain.Payment),
		outbox:   make([]map[string]any, 0),
		inbox:    make(map[int64]domain.PaymentInboxItem),
		clock:    func() time.Time { return time.Now().UTC() },
	}
}

// SetClock cho phep inject thoi gian hien tai phuc vu unit test.
func (m *MemoryRepository) SetClock(nowFunc func() time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.clock = nowFunc
}

func (m *MemoryRepository) now() time.Time {
	if m.clock != nil {
		return m.clock()
	}
	return time.Now().UTC()
}

func (m *MemoryRepository) SeedOrder(order domain.OrderView) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.orders[order.ID] = order
}

func (m *MemoryRepository) SeedPayment(payment domain.Payment) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.payments[payment.ID] = payment
}

func (m *MemoryRepository) GetOrder(_ context.Context, orderID string) (domain.OrderView, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	order, exists := m.orders[orderID]
	if !exists {
		return domain.OrderView{}, domain.ErrOrderNotFound
	}
	return order, nil
}

func (m *MemoryRepository) CreatePaymentAndMarkPending(_ context.Context, p domain.Payment) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	order, exists := m.orders[p.OrderID]
	if !exists {
		return domain.ErrOrderNotFound
	}

	if order.Status != "HELD" {
		return fmt.Errorf("%w: don khong con o trang thai HELD", domain.ErrOrderNotHeld)
	}

	order.Status = "PAYMENT_PENDING"
	m.orders[p.OrderID] = order
	m.payments[p.ID] = p
	return nil
}

func (m *MemoryRepository) GetPayment(_ context.Context, paymentID string) (domain.Payment, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	p, exists := m.payments[paymentID]
	if !exists {
		return domain.Payment{}, errors.New("khong tim thay ban ghi payment")
	}
	return p, nil
}

func (m *MemoryRepository) GetPaymentByOrder(_ context.Context, orderID string) (domain.Payment, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, p := range m.payments {
		if p.OrderID == orderID {
			return p, nil
		}
	}
	return domain.Payment{}, errors.New("khong tim thay payment cho order")
}

// GetOutboxEvents tra ve danh sach su kien outbox da duoc ghi.
func (m *MemoryRepository) GetOutboxEvents() []map[string]any {
	m.mu.RLock()
	defer m.mu.RUnlock()
	events := make([]map[string]any, len(m.outbox))
	copy(events, m.outbox)
	return events
}

// ProcessPaymentSuccessTx xu ly giao dich thanh toan thanh cong mot cach atomic (BR-O6 exactly-once).
func (m *MemoryRepository) ProcessPaymentSuccessTx(_ context.Context, params domain.ProcessWebhookParams) (domain.WebhookProcessResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 1. Kiem tra xem (provider, provider_txn_id) da tung duoc ghi nhan SUCCEEDED chua (BR-O6)
	for _, p := range m.payments {
		if p.Provider == params.Provider && p.ProviderTxnID == params.ProviderTxnID && p.Status == domain.StatusSucceeded {
			return domain.WebhookProcessResult{
				Success:       true,
				AlreadyDone:   true,
				OrderID:       p.OrderID,
				PaymentID:     p.ID,
				ProviderTxnID: p.ProviderTxnID,
				Status:        domain.StatusSucceeded,
				Message:       "giao dich da duoc ghi nhan thanh cong truoc do (idempotent)",
			}, nil
		}
	}

	// 2. Kiem tra don hang
	order, exists := m.orders[params.OrderID]
	if !exists {
		return domain.WebhookProcessResult{}, domain.ErrOrderNotFound
	}

	// Neu don hang da o trang thai PAID roi
	if order.Status == "PAID" {
		return domain.WebhookProcessResult{
			Success:       true,
			AlreadyDone:   true,
			OrderID:       order.ID,
			PaymentID:     params.PaymentID,
			ProviderTxnID: params.ProviderTxnID,
			Status:        domain.StatusSucceeded,
			Message:       "don hang da o trang thai PAID truoc do",
		}, nil
	}

	if order.Status != "HELD" && order.Status != "PAYMENT_PENDING" {
		return domain.WebhookProcessResult{}, fmt.Errorf("%w: don hang o trang thai %s", domain.ErrOrderNotHeld, order.Status)
	}

	// 3. Cap nhat trang thai don hang -> PAID
	order.Status = "PAID"
	m.orders[order.ID] = order

	// 4. Cap nhat hoac them ban ghi thanh toan
	paymentID := params.PaymentID
	var updatedPayment domain.Payment
	found := false
	for id, p := range m.payments {
		if p.OrderID == order.ID {
			p.Status = domain.StatusSucceeded
			p.ProviderTxnID = params.ProviderTxnID
			p.RawPayload = params.RawPayload
			m.payments[id] = p
			updatedPayment = p
			paymentID = id
			found = true
			break
		}
	}

	if !found {
		if paymentID == "" {
			paymentID = fmt.Sprintf("pay-%s", params.ProviderTxnID)
		}
		updatedPayment = domain.Payment{
			ID:            paymentID,
			OrderID:       order.ID,
			Provider:      params.Provider,
			ProviderTxnID: params.ProviderTxnID,
			AmountCents:   order.TotalCents,
			Currency:      order.Currency,
			Status:        domain.StatusSucceeded,
			RawPayload:    params.RawPayload,
			CreatedAt:     time.Now().UTC(),
		}
		m.payments[paymentID] = updatedPayment
	}

	// 5. Ghi su kien outbox order.paid
	now := time.Now().UTC()
	outboxEvent := map[string]any{
		"aggregate_type": "order",
		"aggregate_id":   order.ID,
		"routing_key":    "order.paid",
		"payload": domain.OutboxOrderPaidPayload{
			OrderID:       order.ID,
			EventID:       order.EventID,
			IdentityID:    order.IdentityID,
			PaymentID:     paymentID,
			Provider:      string(params.Provider),
			ProviderTxnID: params.ProviderTxnID,
			AmountCents:   order.TotalCents,
			Currency:      order.Currency,
			PaidAt:        now,
		},
		"trace_id":   params.TraceID,
		"created_at": now,
	}
	m.outbox = append(m.outbox, outboxEvent)

	return domain.WebhookProcessResult{
		Success:       true,
		AlreadyDone:   false,
		OrderID:       order.ID,
		PaymentID:     paymentID,
		ProviderTxnID: params.ProviderTxnID,
		Status:        domain.StatusSucceeded,
		Message:       "thanh toan thanh cong, don da sang PAID va ghi outbox",
	}, nil
}

// ProcessPaymentFailedTx xu ly ghi nhan thanh toan that bai.
func (m *MemoryRepository) ProcessPaymentFailedTx(_ context.Context, params domain.ProcessWebhookParams) (domain.WebhookProcessResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for id, p := range m.payments {
		if p.OrderID == params.OrderID || (p.Provider == params.Provider && p.ProviderTxnID == params.ProviderTxnID) {
			p.Status = domain.StatusFailed
			p.RawPayload = params.RawPayload
			m.payments[id] = p
			return domain.WebhookProcessResult{
				Success:       false,
				AlreadyDone:   false,
				OrderID:       p.OrderID,
				PaymentID:     p.ID,
				ProviderTxnID: params.ProviderTxnID,
				Status:        domain.StatusFailed,
				Message:       "da ghi nhan thanh toan that bai",
			}, nil
		}
	}

	paymentID := params.PaymentID
	if paymentID == "" {
		paymentID = fmt.Sprintf("pay-fail-%s", params.ProviderTxnID)
	}
	m.payments[paymentID] = domain.Payment{
		ID:            paymentID,
		OrderID:       params.OrderID,
		Provider:      params.Provider,
		ProviderTxnID: params.ProviderTxnID,
		AmountCents:   params.AmountCents,
		Currency:      params.Currency,
		Status:        domain.StatusFailed,
		RawPayload:    params.RawPayload,
		CreatedAt:     time.Now().UTC(),
	}

	return domain.WebhookProcessResult{
		Success:       false,
		AlreadyDone:   false,
		OrderID:       params.OrderID,
		PaymentID:     paymentID,
		ProviderTxnID: params.ProviderTxnID,
		Status:        domain.StatusFailed,
		Message:       "da ghi nhan thanh toan that bai",
	}, nil
}

// SavePaymentInbox luu webhook vao payment_inbox khi don chua ton tai (BR-O6).
// Neu ban ghi (provider, provider_txn_id) da ton tai, tra ve ID cu ma khong gay loi (idempotent).
func (m *MemoryRepository) SavePaymentInbox(_ context.Context, item domain.PaymentInboxItem) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for id, it := range m.inbox {
		if it.Provider == item.Provider && it.ProviderTxnID == item.ProviderTxnID {
			return id, nil
		}
	}

	m.nextInboxID++
	item.ID = m.nextInboxID
	if item.ReceivedAt.IsZero() {
		item.ReceivedAt = time.Now().UTC()
	}
	m.inbox[item.ID] = item
	return item.ID, nil
}

// GetPendingPaymentInbox lay danh sach ban ghi chua xu ly (processed_at IS NULL) voi so lan thu < maxAttempts.
func (m *MemoryRepository) GetPendingPaymentInbox(_ context.Context, limit int, maxAttempts int) ([]domain.PaymentInboxItem, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []domain.PaymentInboxItem
	for _, it := range m.inbox {
		if it.ProcessedAt == nil && (maxAttempts <= 0 || it.Attempts < maxAttempts) {
			result = append(result, it)
		}
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].ReceivedAt.Before(result[j].ReceivedAt)
	})

	if limit > 0 && len(result) > limit {
		result = result[:limit]
	}

	return result, nil
}

// MarkPaymentInboxProcessed danh dau ban ghi inbox da duoc xu ly thanh cong (processed_at = now()).
func (m *MemoryRepository) MarkPaymentInboxProcessed(_ context.Context, id int64, processedAt time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	it, exists := m.inbox[id]
	if !exists {
		return errors.New("khong tim thay ban ghi payment_inbox")
	}

	if processedAt.IsZero() {
		processedAt = time.Now().UTC()
	}
	it.ProcessedAt = &processedAt
	m.inbox[id] = it
	return nil
}

// IncrementPaymentInboxAttempts tang so lan thu attempts sau khi replay chua thanh cong.
func (m *MemoryRepository) IncrementPaymentInboxAttempts(_ context.Context, id int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	it, exists := m.inbox[id]
	if !exists {
		return errors.New("khong tim thay ban ghi payment_inbox")
	}

	it.Attempts++
	m.inbox[id] = it
	return nil
}

// GetStalePaymentInbox tim cac ban ghi chua xu ly co thoi gian ton vuot qua nguong cho phep de canh bao.
func (m *MemoryRepository) GetStalePaymentInbox(_ context.Context, olderThan time.Duration, limit int) ([]domain.StaleInboxRecord, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	now := m.now()
	var records []domain.StaleInboxRecord

	for _, it := range m.inbox {
		if it.ProcessedAt == nil && it.IsStale(now, olderThan) {
			records = append(records, domain.StaleInboxRecord{
				ID:            it.ID,
				Provider:      it.Provider,
				ProviderTxnID: it.ProviderTxnID,
				ReceivedAt:    it.ReceivedAt,
				Age:           now.Sub(it.ReceivedAt),
				Attempts:      it.Attempts,
			})
		}
	}

	sort.Slice(records, func(i, j int) bool {
		return records[i].ReceivedAt.Before(records[j].ReceivedAt)
	})

	if limit > 0 && len(records) > limit {
		records = records[:limit]
	}

	return records, nil
}

// GetStalePendingOrders quet cac don hang o trang thai PAYMENT_PENDING ton tai qua nguong olderThan.
func (m *MemoryRepository) GetStalePendingOrders(_ context.Context, olderThan time.Duration, limit int) ([]domain.StaleOrderInfo, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	now := m.now()
	threshold := now.Add(-olderThan)
	var results []domain.StaleOrderInfo

	for _, order := range m.orders {
		if order.Status == "PAYMENT_PENDING" {
			orderTime := order.CreatedAt
			if !order.UpdatedAt.IsZero() {
				orderTime = order.UpdatedAt
			}
			if !orderTime.After(threshold) {
				info := domain.StaleOrderInfo{
					OrderID:    order.ID,
					EventID:    order.EventID,
					IdentityID: order.IdentityID,
					Status:     order.Status,
					TotalCents: order.TotalCents,
					Currency:   order.Currency,
					ExpiresAt:  order.ExpiresAt,
					UpdatedAt:  order.UpdatedAt,
					CreatedAt:  order.CreatedAt,
				}

				// Tim payment gan nhat cua order nay neu co
				for _, p := range m.payments {
					if p.OrderID == order.ID {
						info.PaymentID = p.ID
						info.Provider = p.Provider
						info.ProviderTxnID = p.ProviderTxnID
						break
					}
				}

				results = append(results, info)
			}
		}
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].CreatedAt.Before(results[j].CreatedAt)
	})

	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}

	return results, nil
}

// ProcessOrderCancellationTx huy don hang treo va ghi su kien hold.released.
func (m *MemoryRepository) ProcessOrderCancellationTx(_ context.Context, params domain.CancelOrderParams) (domain.CancelOrderResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	order, exists := m.orders[params.OrderID]
	if !exists {
		return domain.CancelOrderResult{}, domain.ErrOrderNotFound
	}

	if order.Status == "CANCELLED" || order.Status == "EXPIRED" {
		return domain.CancelOrderResult{
			OrderID:     order.ID,
			AlreadyDone: true,
			Status:      order.Status,
			Message:     fmt.Sprintf("don hang da o trang thai %s truoc do", order.Status),
		}, nil
	}

	if order.Status == "PAID" || order.Status == "ISSUED" {
		return domain.CancelOrderResult{
			OrderID:     order.ID,
			AlreadyDone: true,
			Status:      order.Status,
			Message:     "don hang da thanh toan thanh cong (PAID), khong the huy",
		}, nil
	}

	order.Status = "CANCELLED"
	m.orders[order.ID] = order

	for id, p := range m.payments {
		if p.OrderID == order.ID {
			p.Status = domain.StatusFailed
			m.payments[id] = p
		}
	}

	now := time.Now().UTC()
	outboxEvent := map[string]any{
		"aggregate_type": "order",
		"aggregate_id":   order.ID,
		"routing_key":    "hold.released",
		"payload": domain.OutboxHoldReleasedPayload{
			OrderID:    order.ID,
			EventID:    order.EventID,
			Reason:     params.Reason,
			ReleasedAt: now,
		},
		"trace_id":   params.TraceID,
		"created_at": now,
	}
	m.outbox = append(m.outbox, outboxEvent)

	return domain.CancelOrderResult{
		OrderID:         order.ID,
		AlreadyDone:     false,
		HoldsReleased:   1,
		TicketsReturned: 1,
		Status:          "CANCELLED",
		Message:         "huy don thanh cong, da tra kho ve va phat outbox",
	}, nil
}

// GetOutbox tra ve ban sao danh sach su kien outbox da duoc ghi nhan (ho tro kiem thu).
func (m *MemoryRepository) GetOutbox() []map[string]any {
	m.mu.RLock()
	defer m.mu.RUnlock()
	res := make([]map[string]any, len(m.outbox))
	copy(res, m.outbox)
	return res
}

// GetPaidOrdersByEvent lay danh sach cac don hang o trang thai PAID, ISSUED hoac REFUNDING cua mot su kien de hoan tien (EV-154 / EVF-74).
func (m *MemoryRepository) GetPaidOrdersByEvent(_ context.Context, eventID string, limit int) ([]domain.RefundOrderInfo, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var results []domain.RefundOrderInfo
	for _, order := range m.orders {
		if order.EventID == eventID && (order.Status == "PAID" || order.Status == "ISSUED" || order.Status == "REFUNDING") {
			info := domain.RefundOrderInfo{
				OrderID:    order.ID,
				EventID:    order.EventID,
				IdentityID: order.IdentityID,
				Status:     order.Status,
				TotalCents: order.TotalCents,
				Currency:   order.Currency,
				CreatedAt:  order.CreatedAt,
				UpdatedAt:  order.UpdatedAt,
			}

			// Tim payment tuong ung neu co
			for _, p := range m.payments {
				if p.OrderID == order.ID {
					info.PaymentID = p.ID
					info.Provider = p.Provider
					info.ProviderTxnID = p.ProviderTxnID
					break
				}
			}

			results = append(results, info)
		}
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].CreatedAt.Before(results[j].CreatedAt)
	})

	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}

	return results, nil
}

// MarkOrderRefundingTx cap nhat trang thai don hang sang REFUNDING.
func (m *MemoryRepository) MarkOrderRefundingTx(_ context.Context, orderID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	order, exists := m.orders[orderID]
	if !exists {
		return domain.ErrOrderNotFound
	}

	if order.Status == "REFUNDED" {
		return nil // Already refunded
	}
	if order.Status != "PAID" && order.Status != "ISSUED" && order.Status != "REFUNDING" {
		return domain.ErrOrderNotRefundable
	}

	order.Status = "REFUNDING"
	order.UpdatedAt = m.now()
	m.orders[orderID] = order
	return nil
}

// ProcessRefundSuccessTx cap nhat trang thai orders -> REFUNDED, payments -> REFUNDED trong mot transaction (Quy tac 1).
func (m *MemoryRepository) ProcessRefundSuccessTx(_ context.Context, params domain.ProcessRefundParams) (domain.ProcessRefundResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	order, exists := m.orders[params.OrderID]
	if !exists {
		return domain.ProcessRefundResult{}, domain.ErrOrderNotFound
	}

	// Idempotent: neu don da o trang thai REFUNDED truoc do
	if order.Status == "REFUNDED" {
		return domain.ProcessRefundResult{
			OrderID:         order.ID,
			PaymentID:       params.PaymentID,
			AlreadyDone:     true,
			TicketsRefunded: 1,
			Status:          "REFUNDED",
			Message:         "don hang da duoc hoan tien truoc do (idempotent)",
		}, nil
	}

	now := m.now()
	order.Status = "REFUNDED"
	order.UpdatedAt = now
	m.orders[order.ID] = order

	// Cap nhat payment
	for id, p := range m.payments {
		if p.OrderID == order.ID || (params.PaymentID != "" && p.ID == params.PaymentID) {
			p.Status = domain.StatusRefunded
			if p.RawPayload == nil {
				p.RawPayload = make(map[string]any)
			}
			p.RawPayload["refund_txn_id"] = params.RefundTxnID
			p.RawPayload["refund_reason"] = params.Reason
			p.RawPayload["refunded_at"] = now.Format(time.RFC3339)
			m.payments[id] = p
		}
	}

	// Ghi nhan outbox event payment.refunded
	outboxEvent := map[string]any{
		"aggregate_type": "payment",
		"aggregate_id":   order.ID,
		"routing_key":    "payment.refunded",
		"payload": domain.OutboxPaymentRefundedPayload{
			OrderID:     order.ID,
			EventID:     order.EventID,
			IdentityID:  order.IdentityID,
			PaymentID:   params.PaymentID,
			Provider:    string(params.Provider),
			RefundTxnID: params.RefundTxnID,
			AmountCents: params.AmountCents,
			Currency:    order.Currency,
			Reason:      params.Reason,
			RefundedAt:  now,
		},
		"trace_id":   params.TraceID,
		"created_at": now,
	}
	m.outbox = append(m.outbox, outboxEvent)

	return domain.ProcessRefundResult{
		OrderID:         order.ID,
		PaymentID:       params.PaymentID,
		AlreadyDone:     false,
		TicketsRefunded: 1,
		Status:          "REFUNDED",
		Message:         "hoan tien thanh cong, da cap nhat trang thai va phat outbox",
	}, nil
}

// ProcessRefundFailedTx ghi nhan trang thai hoan tien that bai hoac can retry.
func (m *MemoryRepository) ProcessRefundFailedTx(_ context.Context, orderID string, paymentID string, reason string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for id, p := range m.payments {
		if p.OrderID == orderID || (paymentID != "" && p.ID == paymentID) {
			if p.RawPayload == nil {
				p.RawPayload = make(map[string]any)
			}
			p.RawPayload["refund_last_error"] = reason
			p.RawPayload["refund_attempt_at"] = m.now().Format(time.RFC3339)
			m.payments[id] = p
		}
	}
	return nil
}
