package app_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/eventflow/eventflow/services/ticketing/internal/app"
	"github.com/eventflow/eventflow/services/ticketing/internal/domain"
	"github.com/eventflow/eventflow/services/ticketing/internal/port"
)

type mockRepo struct {
	initErr error
	calls   []initCall
}

type initCall struct {
	ticketTypeID string
	quota        int
	bucketCount  int
}

func (m *mockRepo) GetSaleConfig(context.Context, string, string) (port.SaleConfig, error) {
	return port.SaleConfig{}, nil
}
func (m *mockRepo) CountPurchased(context.Context, string, string) (int, error) { return 0, nil }
func (m *mockRepo) GetActiveHold(context.Context, string, string) (port.HoldView, error) {
	return port.HoldView{}, nil
}
func (m *mockRepo) PersistHold(context.Context, port.PersistHoldParams) (port.HoldView, error) {
	return port.HoldView{}, nil
}
func (m *mockRepo) ReleaseHold(context.Context, string) (bool, error) { return true, nil }

func (m *mockRepo) InitInventory(_ context.Context, tt string, quota, bucketCount int) error {
	m.calls = append(m.calls, initCall{tt, quota, bucketCount})
	return m.initErr
}

type mockSeeder struct {
	seedErr error
	calls   []seedCall
}

type seedCall struct {
	eventID      string
	ticketTypeID string
	buckets      map[int]int
}

func (m *mockSeeder) SeedInventory(_ context.Context, ev, tt string, b map[int]int) error {
	m.calls = append(m.calls, seedCall{ev, tt, b})
	return m.seedErr
}

func TestInventoryService_InitializeInventory_Success(t *testing.T) {
	repo := &mockRepo{}
	seeder := &mockSeeder{}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	svc := app.NewInventoryService(repo, seeder, logger)
	ctx := context.Background()

	err := svc.InitializeInventory(ctx, app.InitInventoryInput{
		EventID:      "event-1",
		TicketTypeID: "tt-vip",
		Quota:        100,
		BucketCount:  32,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(repo.calls) != 1 {
		t.Fatalf("expected 1 repo call, got %d", len(repo.calls))
	}
	if repo.calls[0].ticketTypeID != "tt-vip" || repo.calls[0].quota != 100 || repo.calls[0].bucketCount != 32 {
		t.Fatalf("unexpected repo call args: %+v", repo.calls[0])
	}

	if len(seeder.calls) != 1 {
		t.Fatalf("expected 1 seeder call, got %d", len(seeder.calls))
	}
	if seeder.calls[0].eventID != "event-1" || seeder.calls[0].ticketTypeID != "tt-vip" {
		t.Fatalf("unexpected seeder call args: %+v", seeder.calls[0])
	}

	// Invariant check: total tickets in seeder map == 100
	total := 0
	for _, qty := range seeder.calls[0].buckets {
		total += qty
	}
	if total != 100 {
		t.Fatalf("expected total 100 in seeder, got %d", total)
	}
}

func TestInventoryService_InitializeInventory_NegativeQuota(t *testing.T) {
	repo := &mockRepo{}
	seeder := &mockSeeder{}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	svc := app.NewInventoryService(repo, seeder, logger)
	ctx := context.Background()

	err := svc.InitializeInventory(ctx, app.InitInventoryInput{
		EventID:      "event-1",
		TicketTypeID: "tt-vip",
		Quota:        -5,
	})
	if !errors.Is(err, domain.ErrNegativeQuota) {
		t.Fatalf("expected ErrNegativeQuota, got %v", err)
	}
}

func TestInventoryService_InitializeInventory_RepoError(t *testing.T) {
	repo := &mockRepo{initErr: errors.New("db error")}
	seeder := &mockSeeder{}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	svc := app.NewInventoryService(repo, seeder, logger)
	ctx := context.Background()

	err := svc.InitializeInventory(ctx, app.InitInventoryInput{
		EventID:      "event-1",
		TicketTypeID: "tt-vip",
		Quota:        50,
	})
	if err == nil {
		t.Fatalf("expected error from repo, got nil")
	}
}

func TestInventoryService_InitializeInventory_SeederWarningDoesNotFail(t *testing.T) {
	repo := &mockRepo{}
	seeder := &mockSeeder{seedErr: errors.New("redis down")}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	svc := app.NewInventoryService(repo, seeder, logger)
	ctx := context.Background()

	// Even if redis seeding logs a warning, the operation succeeds because Postgres is truth
	err := svc.InitializeInventory(ctx, app.InitInventoryInput{
		EventID:      "event-1",
		TicketTypeID: "tt-vip",
		Quota:        50,
	})
	if err != nil {
		t.Fatalf("seeder failure should be non-fatal, got error: %v", err)
	}
}
