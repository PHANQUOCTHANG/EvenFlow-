package app

import (
	"context"
	"time"

	"github.com/eventflow/eventflow/services/event/internal/domain"
	"github.com/eventflow/eventflow/services/event/internal/port"
	"github.com/google/uuid"
)

type TransitionStateUseCase interface {
	SubmitForReview(ctx context.Context, eventID, organizerID uuid.UUID) (*domain.Event, error)
	Approve(ctx context.Context, eventID uuid.UUID, now time.Time) (*domain.Event, error)
	Reject(ctx context.Context, eventID uuid.UUID) (*domain.Event, error)
	Publish(ctx context.Context, eventID uuid.UUID, now time.Time) (*domain.Event, error)
	Cancel(ctx context.Context, eventID, organizerID uuid.UUID) (*domain.Event, error)
}

type transitionStateUseCase struct {
	repo port.EventRepository
}

func NewTransitionStateUseCase(repo port.EventRepository) TransitionStateUseCase {
	return &transitionStateUseCase{repo: repo}
}

// SubmitForReview nộp sự kiện để duyệt, kiểm tra BR-E1.
func (uc *transitionStateUseCase) SubmitForReview(ctx context.Context, eventID, organizerID uuid.UUID) (*domain.Event, error) {
	evt, err := uc.repo.GetByID(ctx, eventID)
	if err != nil {
		return nil, err
	}

	if evt.OrganizerID != organizerID {
		return nil, domain.ErrUnauthorizedOrganizer
	}

	// Đảm bảo nạp đầy đủ tiers để kiểm tra BR-E1
	if len(evt.Tiers) == 0 {
		tiers, _ := uc.repo.GetTiersByEventID(ctx, eventID)
		evt.Tiers = tiers
	}

	if err := evt.SubmitForReview(); err != nil {
		return nil, err
	}

	if err := uc.repo.Update(ctx, evt); err != nil {
		return nil, err
	}

	return evt, nil
}

// Approve phê duyệt sự kiện, kiểm tra BR-E2.
func (uc *transitionStateUseCase) Approve(ctx context.Context, eventID uuid.UUID, now time.Time) (*domain.Event, error) {
	evt, err := uc.repo.GetByID(ctx, eventID)
	if err != nil {
		return nil, err
	}

	if err := evt.Approve(now); err != nil {
		return nil, err
	}

	if err := uc.repo.Update(ctx, evt); err != nil {
		return nil, err
	}

	return evt, nil
}

// Reject từ chối duyệt sự kiện.
func (uc *transitionStateUseCase) Reject(ctx context.Context, eventID uuid.UUID) (*domain.Event, error) {
	evt, err := uc.repo.GetByID(ctx, eventID)
	if err != nil {
		return nil, err
	}

	if err := evt.Reject(); err != nil {
		return nil, err
	}

	if err := uc.repo.Update(ctx, evt); err != nil {
		return nil, err
	}

	return evt, nil
}

// Publish kích hoạt mở bán sự kiện khi đến T0.
func (uc *transitionStateUseCase) Publish(ctx context.Context, eventID uuid.UUID, now time.Time) (*domain.Event, error) {
	evt, err := uc.repo.GetByID(ctx, eventID)
	if err != nil {
		return nil, err
	}

	if err := evt.OpenSale(now); err != nil {
		return nil, err
	}

	if err := uc.repo.Update(ctx, evt); err != nil {
		return nil, err
	}

	return evt, nil
}

// Cancel hủy sự kiện.
func (uc *transitionStateUseCase) Cancel(ctx context.Context, eventID, organizerID uuid.UUID) (*domain.Event, error) {
	evt, err := uc.repo.GetByID(ctx, eventID)
	if err != nil {
		return nil, err
	}

	if evt.OrganizerID != organizerID {
		return nil, domain.ErrUnauthorizedOrganizer
	}

	if err := evt.Cancel(); err != nil {
		return nil, err
	}

	if err := uc.repo.Update(ctx, evt); err != nil {
		return nil, err
	}

	return evt, nil
}
