//go:build integration

package postgres_test

// Test tich hop frontend-real-api AC-26: EventRepository.ListPublic(ctx, limit) (ten gia dinh,
// xem adapter/http/public_events_contract_test.go). Dung chung TestMain/testx cua event_repo_integration_test.go.

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/eventflow/eventflow/libs/go/testx"
	"github.com/eventflow/eventflow/services/event/internal/adapter/postgres"
	"github.com/eventflow/eventflow/services/event/internal/domain"
)

func seedPublicEvent(t *testing.T, ctx context.Context, repo interface {
	Create(context.Context, *domain.Event) error
	Update(context.Context, *domain.Event) error
	AddTier(context.Context, *domain.TicketTier) error
}, org uuid.UUID, status domain.EventStatus, saleStartIn time.Duration, withTier bool) *domain.Event {
	t.Helper()
	now := time.Now().UTC()
	evt, err := domain.NewEvent(org, "pub-"+uuid.NewString()[:8], "Pub "+string(status), "d", "v",
		now.Add(saleStartIn+72*time.Hour), now.Add(saleStartIn), nil)
	if err != nil {
		t.Fatalf("NewEvent: %v", err)
	}
	if err := repo.Create(ctx, evt); err != nil {
		t.Fatalf("Create: %v", err)
	}
	evt.Status = status
	if err := repo.Update(ctx, evt); err != nil {
		t.Fatalf("Update status: %v", err)
	}
	if withTier {
		tier, err := domain.NewTicketTier(evt.ID, "General", 650000, 100)
		if err != nil {
			t.Fatal(err)
		}
		if err := repo.AddTier(ctx, tier); err != nil {
			t.Fatalf("AddTier: %v", err)
		}
	}
	return evt
}

func TestTichHop_ListPublic_LocTrangThaiSapXepVaTiers(t *testing.T) {
	ctx := context.Background()
	db, err := sql.Open("postgres", testx.Postgres(t))
	if err != nil {
		t.Fatalf("mo postgres: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	org := uuid.New()
	if _, err := db.Exec(`INSERT INTO identities (id, email, password_hash, role) VALUES ($1, $2, 'x', 'organizer')`,
		org, org.String()+"@test.local"); err != nil {
		t.Fatalf("tao organizer: %v", err)
	}
	repo := postgres.NewEventRepository(db)

	late := seedPublicEvent(t, ctx, repo, org, domain.StatusScheduled, 96*time.Hour, true)
	early := seedPublicEvent(t, ctx, repo, org, domain.StatusOnSale, 1*time.Hour, true)
	mid := seedPublicEvent(t, ctx, repo, org, domain.StatusSoldOut, 24*time.Hour, false)
	hidden := map[uuid.UUID]bool{}
	for _, st := range []domain.EventStatus{
		domain.StatusDraft, domain.StatusPendingReview, domain.StatusRejected,
		domain.StatusClosed, domain.StatusCompleted, domain.StatusCancelled,
	} {
		hidden[seedPublicEvent(t, ctx, repo, org, st, 2*time.Hour, true).ID] = true
	}

	list, err := repo.ListPublic(ctx, 100)
	if err != nil {
		t.Fatalf("ListPublic: %v", err)
	}
	if len(list) > 100 {
		t.Fatalf("vuot gioi han 100: %d", len(list))
	}

	// Chi xet su kien cua organizer nay (DB dung chung voi test khac).
	var mine []*domain.Event
	var prev time.Time
	for i, e := range list {
		if hidden[e.ID] {
			t.Fatalf("trang thai khong cong khai lot vao: %s", e.Status)
		}
		switch e.Status {
		case domain.StatusScheduled, domain.StatusOnSale, domain.StatusSoldOut:
		default:
			t.Fatalf("trang thai khong cong khai: %s", e.Status)
		}
		if i > 0 && e.SaleStartAt.Before(prev) {
			t.Fatalf("khong sap xep sale_start_at ASC tai vi tri %d", i)
		}
		prev = e.SaleStartAt
		if e.OrganizerID == org {
			mine = append(mine, e)
		}
	}
	if len(mine) != 3 {
		t.Fatalf("muon 3 su kien cong khai cua organizer, got %d", len(mine))
	}
	if mine[0].ID != early.ID || mine[1].ID != mid.ID || mine[2].ID != late.ID {
		t.Fatalf("thu tu sai: %s, %s, %s", mine[0].Status, mine[1].Status, mine[2].Status)
	}
	if len(mine[0].Tiers) != 1 || mine[0].Tiers[0].PriceCents != 650000 {
		t.Errorf("thieu tiers cho su kien ON_SALE: %+v", mine[0].Tiers)
	}
	if len(mine[1].Tiers) != 0 {
		t.Errorf("su kien khong co tier phai tra Tiers rong: %+v", mine[1].Tiers)
	}
}

func TestTichHop_ListPublic_CatTheoLimit(t *testing.T) {
	ctx := context.Background()
	db, err := sql.Open("postgres", testx.Postgres(t))
	if err != nil {
		t.Fatalf("mo postgres: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	org := uuid.New()
	if _, err := db.Exec(`INSERT INTO identities (id, email, password_hash, role) VALUES ($1, $2, 'x', 'organizer')`,
		org, org.String()+"@test.local"); err != nil {
		t.Fatalf("tao organizer: %v", err)
	}
	repo := postgres.NewEventRepository(db)
	for i := 0; i < 3; i++ {
		seedPublicEvent(t, ctx, repo, org, domain.StatusOnSale, time.Duration(i+1)*time.Hour, false)
	}
	list, err := repo.ListPublic(ctx, 2)
	if err != nil {
		t.Fatalf("ListPublic: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("limit 2 phai tra dung 2, got %d", len(list))
	}
}
