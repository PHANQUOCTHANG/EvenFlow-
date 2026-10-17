package postgres

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/eventflow/eventflow/services/payment/internal/domain"
)

// MemoryRepository cung cap kho luu tru in-memory phuc vu chay local va unit test.
type MemoryRepository struct {
	mu       sync.RWMutex
	orders   map[string]domain.OrderView
	payments map[string]domain.Payment
	outbox   []map[string]any
}

func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{
		orders:   make(map[string]domain.OrderView),
		payments: make(map[string]domain.Payment),
		outbox:   make([]map[string]any, 0),
	}
}

func (m *MemoryRepository) SeedOrder(order domain.OrderView) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.orders[order.ID] = order
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
