package postgres

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/eventflow/eventflow/services/payment/internal/domain"
)

// MemoryRepository cung cap kho luu tru in-memory phuc vu chay local va unit test.
type MemoryRepository struct {
	mu       sync.RWMutex
	orders   map[string]domain.OrderView
	payments map[string]domain.Payment
}

func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{
		orders:   make(map[string]domain.OrderView),
		payments: make(map[string]domain.Payment),
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
