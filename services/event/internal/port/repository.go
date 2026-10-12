package port

import (
	"context"

	"github.com/eventflow/eventflow/services/event/internal/domain"
	"github.com/google/uuid"
)

// EventRepository định nghĩa giao tiếp lưu trữ dữ liệu cho Event và TicketTier.
type EventRepository interface {
	Create(ctx context.Context, event *domain.Event) error
	GetByID(ctx context.Context, id uuid.UUID) (*domain.Event, error)
	GetBySlug(ctx context.Context, slug string) (*domain.Event, error)
	ListByOrganizer(ctx context.Context, organizerID uuid.UUID) ([]*domain.Event, error)
	Update(ctx context.Context, event *domain.Event) error

	AddTier(ctx context.Context, tier *domain.TicketTier) error
	UpdateTier(ctx context.Context, tier *domain.TicketTier) error
	GetTiersByEventID(ctx context.Context, eventID uuid.UUID) ([]domain.TicketTier, error)
}
