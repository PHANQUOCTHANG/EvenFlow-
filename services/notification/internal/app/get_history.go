package app

import (
	"context"
	"fmt"
	"strings"

	"github.com/eventflow/eventflow/services/notification/internal/domain"
	"github.com/eventflow/eventflow/services/notification/internal/port"
)

type GetNotificationHistoryUseCase struct {
	store port.NotificationStore
}

func NewGetNotificationHistoryUseCase(store port.NotificationStore) *GetNotificationHistoryUseCase {
	return &GetNotificationHistoryUseCase{
		store: store,
	}
}

func (uc *GetNotificationHistoryUseCase) Execute(ctx context.Context, orderID string) ([]domain.Notification, error) {
	orderID = strings.TrimSpace(orderID)
	if orderID == "" {
		return nil, fmt.Errorf("%w: empty orderID", domain.ErrInvalidEventPayload)
	}

	return uc.store.GetHistoryByOrderID(ctx, orderID)
}
