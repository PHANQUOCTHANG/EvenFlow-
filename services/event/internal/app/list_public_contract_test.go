package app_test

import (
	"context"

	"github.com/eventflow/eventflow/services/event/internal/domain"
)

// frontend-real-api AC-26: port.EventRepository them
//
//	ListPublic(ctx context.Context, limit int) ([]*domain.Event, error)
//
// (ten gia dinh, xem public_events_contract_test.go). Fake cua package app_test phai thoa interface moi;
// dinh nghia o file test moi de khong sua fakes_test.go. Loc/sap xep/gioi han kiem o test tich hop postgres.
func (r *fakeEventRepo) ListPublic(ctx context.Context, limit int) ([]*domain.Event, error) {
	var list []*domain.Event
	for _, evt := range r.events {
		switch evt.Status {
		case domain.StatusScheduled, domain.StatusOnSale, domain.StatusSoldOut:
			list = append(list, evt)
		}
	}
	if limit > 0 && len(list) > limit {
		list = list[:limit]
	}
	return list, nil
}
