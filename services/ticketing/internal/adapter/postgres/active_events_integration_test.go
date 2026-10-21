//go:build integration

package postgresadapter_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
)

// Chi su kien ON_SALE, SOLD_OUT, va SCHEDULED mo ban trong khoang nhin truoc duoc phuc vu.
func TestListSellableEvents_LocTheoTrangThaiVaKhoangNhinTruoc(t *testing.T) {
	repo, pool := setupPostgresTest(t)
	ctx := context.Background()
	org := uuid.New().String()
	if _, err := pool.Exec(ctx, `INSERT INTO identities (id, email, password_hash, role) VALUES ($1::uuid, $2, 'x', 'organizer')`,
		org, "org-"+org+"@test.com"); err != nil {
		t.Fatal(err)
	}
	add := func(status string, saleIn time.Duration) string {
		id := uuid.New().String()
		_, err := pool.Exec(ctx, `INSERT INTO events (id, organizer_id, slug, title, starts_at, sale_start_at, status)
			VALUES ($1::uuid, $2::uuid, $3, 't', now() + interval '30 days', now() + $4::interval, $5)`,
			id, org, "s-"+id, (saleIn.Truncate(time.Second)).String(), status)
		if err != nil {
			t.Fatalf("seed %s: %v", status, err)
		}
		return id
	}
	want := map[string]bool{
		add("ON_SALE", -time.Hour):       true,
		add("SOLD_OUT", -time.Hour):      true,
		add("SCHEDULED", 2*time.Hour):    true,
		add("SCHEDULED", 48*time.Hour):   false, // ngoai khoang 24 gio
		add("DRAFT", time.Hour):          false,
		add("PENDING_REVIEW", time.Hour): false,
		add("CANCELLED", -time.Hour):     false,
		add("COMPLETED", -time.Hour):     false,
	}

	got, err := repo.ListSellableEvents(ctx, 24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, e := range got {
		seen[e.ID] = true
		if e.SaleStartAt.IsZero() {
			t.Errorf("%s thieu sale_start_at", e.ID)
		}
	}
	for id, in := range want {
		if seen[id] != in {
			t.Errorf("su kien %s: co trong ket qua = %v, muon %v", id, seen[id], in)
		}
	}
}
