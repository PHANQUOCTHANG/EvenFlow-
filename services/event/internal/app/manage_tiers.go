package app

import (
	"context"

	"github.com/eventflow/eventflow/services/event/internal/domain"
	"github.com/eventflow/eventflow/services/event/internal/port"
	"github.com/google/uuid"
)

type AddTierInput struct {
	EventID     uuid.UUID `json:"event_id"`
	OrganizerID uuid.UUID `json:"organizer_id"`
	Name        string    `json:"name"`
	PriceCents  int64     `json:"price_cents"`
	Quota       int       `json:"quota"`
}

type UpdateTierQuotaInput struct {
	EventID     uuid.UUID `json:"event_id"`
	OrganizerID uuid.UUID `json:"organizer_id"`
	TierID      uuid.UUID `json:"tier_id"`
	NewQuota    int       `json:"new_quota"`
}

type ManageTiersUseCase interface {
	AddTier(ctx context.Context, input AddTierInput) (*domain.TicketTier, error)
	UpdateTierQuota(ctx context.Context, input UpdateTierQuotaInput) (*domain.TicketTier, error)
}

type manageTiersUseCase struct {
	repo port.EventRepository
}

func NewManageTiersUseCase(repo port.EventRepository) ManageTiersUseCase {
	return &manageTiersUseCase{repo: repo}
}

func (uc *manageTiersUseCase) AddTier(ctx context.Context, input AddTierInput) (*domain.TicketTier, error) {
	evt, err := uc.repo.GetByID(ctx, input.EventID)
	if err != nil {
		return nil, err
	}

	if evt.OrganizerID != input.OrganizerID {
		return nil, domain.ErrUnauthorizedOrganizer
	}

	tier, err := domain.NewTicketTier(input.EventID, input.Name, input.PriceCents, input.Quota)
	if err != nil {
		return nil, err
	}

	if err := evt.AddTier(*tier); err != nil {
		return nil, err
	}

	if err := uc.repo.AddTier(ctx, tier); err != nil {
		return nil, err
	}

	return tier, nil
}

// UpdateTierQuota cập nhật số lượng vé, bảo đảm Business Rule BR-E3.
func (uc *manageTiersUseCase) UpdateTierQuota(ctx context.Context, input UpdateTierQuotaInput) (*domain.TicketTier, error) {
	evt, err := uc.repo.GetByID(ctx, input.EventID)
	if err != nil {
		return nil, err
	}

	if evt.OrganizerID != input.OrganizerID {
		return nil, domain.ErrUnauthorizedOrganizer
	}

	// Tự động kiểm tra BR-E3: cấm giảm quota nếu ON_SALE
	if err := evt.UpdateTierQuota(input.TierID, input.NewQuota); err != nil {
		return nil, err
	}

	var updatedTier *domain.TicketTier
	for _, t := range evt.Tiers {
		if t.ID == input.TierID {
			updatedTier = &t
			break
		}
	}

	if updatedTier == nil {
		return nil, domain.ErrTierNotFound
	}

	if err := uc.repo.UpdateTier(ctx, updatedTier); err != nil {
		return nil, err
	}

	// Cập nhật trạng thái sự kiện nếu chuyển từ SOLD_OUT sang ON_SALE
	_ = uc.repo.Update(ctx, evt)

	return updatedTier, nil
}
