package store

import (
	"context"
	"sync"
	"time"

	"github.com/eventflow/eventflow/services/notification/internal/domain"
	"github.com/eventflow/eventflow/services/notification/internal/port"
)

// MemoryNotificationStore luu tru thong bao trong RAM, ho tro thread-safe qua RWMutex.
type MemoryNotificationStore struct {
	mu           sync.RWMutex
	dedupRecords map[string]domain.Notification
	orderHistory map[string][]domain.Notification
}

var _ port.NotificationStore = (*MemoryNotificationStore)(nil)

func NewMemoryNotificationStore() *MemoryNotificationStore {
	return &MemoryNotificationStore{
		dedupRecords: make(map[string]domain.Notification),
		orderHistory: make(map[string][]domain.Notification),
	}
}

func (s *MemoryNotificationStore) CheckOrSetDedup(ctx context.Context, dedupKey string, notif domain.Notification) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	existing, exists := s.dedupRecords[dedupKey]
	if exists {
		// Neu da thanh cong hoac dang pending xu ly -> da xu ly (skip duplicate)
		if existing.Status == domain.StatusSent || existing.Status == domain.StatusPending {
			return true, nil
		}
	}

	notif.Status = domain.StatusPending
	if notif.CreatedAt.IsZero() {
		notif.CreatedAt = time.Now()
	}
	s.dedupRecords[dedupKey] = notif
	s.orderHistory[notif.OrderID] = append(s.orderHistory[notif.OrderID], notif)

	return false, nil
}

func (s *MemoryNotificationStore) MarkSent(ctx context.Context, dedupKey string, notifID string, sentAt time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	notif, exists := s.dedupRecords[dedupKey]
	if !exists {
		return domain.ErrNotificationNotFound
	}

	notif.Status = domain.StatusSent
	notif.SentAt = &sentAt
	s.dedupRecords[dedupKey] = notif

	// Cap nhat trong orderHistory
	list := s.orderHistory[notif.OrderID]
	for i := range list {
		if list[i].ID == notifID || list[i].DedupKey == dedupKey {
			list[i].Status = domain.StatusSent
			list[i].SentAt = &sentAt
		}
	}
	return nil
}

func (s *MemoryNotificationStore) MarkFailed(ctx context.Context, dedupKey string, notifID string, reason string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	notif, exists := s.dedupRecords[dedupKey]
	if !exists {
		return domain.ErrNotificationNotFound
	}

	notif.Status = domain.StatusFailed
	notif.ErrorReason = reason
	s.dedupRecords[dedupKey] = notif

	list := s.orderHistory[notif.OrderID]
	for i := range list {
		if list[i].ID == notifID || list[i].DedupKey == dedupKey {
			list[i].Status = domain.StatusFailed
			list[i].ErrorReason = reason
		}
	}
	return nil
}

func (s *MemoryNotificationStore) GetHistoryByOrderID(ctx context.Context, orderID string) ([]domain.Notification, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	list, exists := s.orderHistory[orderID]
	if !exists || len(list) == 0 {
		return []domain.Notification{}, nil
	}

	result := make([]domain.Notification, len(list))
	copy(result, list)
	return result, nil
}
