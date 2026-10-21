package app_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/eventflow/eventflow/services/event/internal/app"
	"github.com/eventflow/eventflow/services/event/internal/domain"
)

// Khach an danh khong keo duoc ca bang: limit mac dinh 50, tran 100.
func TestListPublic_KepLimit(t *testing.T) {
	repo := newFakeRepo()
	for i := 0; i < 150; i++ {
		id := uuid.New()
		repo.events[id] = &domain.Event{ID: id, Status: domain.StatusOnSale}
	}
	uc := app.NewGetEventUseCase(repo)
	for limit, want := range map[int]int{0: app.DefaultPublicLimit, -5: app.DefaultPublicLimit, 3: 3, 500: app.MaxPublicLimit} {
		got, err := uc.ListPublic(context.Background(), limit)
		if err != nil || len(got) != want {
			t.Errorf("limit %d: %d su kien (%v), muon %d", limit, len(got), err, want)
		}
	}
}
