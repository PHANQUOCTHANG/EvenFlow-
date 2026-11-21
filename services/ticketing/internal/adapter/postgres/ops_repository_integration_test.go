//go:build integration

package postgresadapter_test

// ops-warroom AC-14 / AC-15 / AC-16: SQL definitions of contracts §4 against real Postgres (needs Docker).
// ASSUMED: *postgresadapter.Repository implements port.OpsReader (plan WP2: ops_repository.go) with
//   EventExists / EventOwner / TierInventory / EventSales as listed in ../http/ops_handler_test.go.
// Fixture: one event, tier A (price 100000, quota 100) and tier B (price 200000, quota 50).
//   orders (distinct buyers because of uq_active_order_per_identity_event):
//     PAID 3xA (vnpay SUCCEEDED), ISSUED 2xA (momo SUCCEEDED), PAID 1xB (vnpay SUCCEEDED),
//     REFUNDING 4xA, REFUNDED 5xA, CANCELLED 6xA, EXPIRED 7xA, HELD 8xA, PAYMENT_PENDING 9xA   -> not sold
//   seat_holds on A: open 5 (HELD order), released 11, consumed 13, open-but-expired 2 (PAYMENT_PENDING order) -> held 7;
//     open 4 on a PAID order (hold not yet consumed) -> NOT held (counted in paid)
//   tickets on A: ISSUED 2, CHECKED_IN 1, VOID 4, REFUNDED 3

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/eventflow/eventflow/services/ticketing/internal/domain"
)

type opsFixture struct {
	event, organizer, tierA, tierB string
}

func seedOps(t *testing.T, pool *pgxpool.Pool) opsFixture {
	t.Helper()
	ctx := context.Background()
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, q, args...); err != nil {
			t.Fatalf("seed: %v\n%s", err, q)
		}
	}
	f := opsFixture{event: uuid.NewString(), organizer: uuid.NewString(), tierA: uuid.NewString(), tierB: uuid.NewString()}
	exec(`INSERT INTO identities (id, email, password_hash, role) VALUES ($1::uuid, $2, 'x', 'organizer')`, f.organizer, f.organizer+"@t.com")
	exec(`INSERT INTO events (id, organizer_id, slug, title, starts_at, sale_start_at, status, max_per_identity, max_per_order)
		VALUES ($1::uuid, $2::uuid, $3, 'Ev', now() + interval '9 days', now(), 'ON_SALE', 4, 4)`, f.event, f.organizer, "ev-"+f.event)
	exec(`INSERT INTO ticket_types (id, event_id, name, price_cents, quota, bucket_count) VALUES ($1::uuid, $2::uuid, 'A', 100000, 100, 2)`, f.tierA, f.event)
	exec(`INSERT INTO ticket_types (id, event_id, name, price_cents, quota, bucket_count) VALUES ($1::uuid, $2::uuid, 'B', 200000, 50, 2)`, f.tierB, f.event)
	exec(`INSERT INTO ticket_inventory (ticket_type_id, bucket, capacity, available) VALUES ($1::uuid,0,50,30),($1::uuid,1,50,20)`, f.tierA)
	exec(`INSERT INTO ticket_inventory (ticket_type_id, bucket, capacity, available) VALUES ($1::uuid,0,25,25),($1::uuid,1,25,25)`, f.tierB)

	order := func(tier, status string, qty int, provider string) string {
		buyer := uuid.NewString()
		exec(`INSERT INTO identities (id, email, password_hash, role) VALUES ($1::uuid, $2, 'x', 'buyer')`, buyer, buyer+"@t.com")
		oid := uuid.NewString()
		price := int64(100000)
		if tier == f.tierB {
			price = 200000
		}
		exec(`INSERT INTO orders (id, event_id, identity_id, status, total_cents, expires_at) VALUES ($1::uuid,$2::uuid,$3::uuid,$4,$5,$6)`,
			oid, f.event, buyer, status, price*int64(qty), time.Now().Add(time.Hour))
		exec(`INSERT INTO order_items (order_id, ticket_type_id, bucket, quantity, unit_cents) VALUES ($1::uuid,$2::uuid,0,$3,$4)`, oid, tier, qty, price)
		if provider != "" {
			exec(`INSERT INTO payments (order_id, provider, provider_txn_id, amount_cents, status) VALUES ($1::uuid,$2,$3,$4,'SUCCEEDED')`,
				oid, provider, uuid.NewString(), price*int64(qty))
		}
		return oid
	}
	paid := order(f.tierA, "PAID", 3, "vnpay")
	order(f.tierA, "ISSUED", 2, "momo")
	order(f.tierB, "PAID", 1, "vnpay")
	for i, s := range []string{"REFUNDING", "REFUNDED", "CANCELLED", "EXPIRED", "HELD", "PAYMENT_PENDING"} {
		order(f.tierA, s, 4+i, "")
	}

	hold := func(qty int, expires time.Time, released, consumed bool, orderStatus string) {
		oid := order(f.tierA, orderStatus, 1, "") // carrier order
		q := `INSERT INTO seat_holds (order_id, ticket_type_id, bucket, quantity, expires_at, released_at, consumed_at)
			VALUES ($1::uuid,$2::uuid,0,$3,$4, CASE WHEN $5 THEN now() END, CASE WHEN $6 THEN now() END)`
		exec(q, oid, f.tierA, qty, expires, released, consumed)
	}
	// held counts open holds only while their order is HELD / PAYMENT_PENDING (contracts rev 2).
	hold(5, time.Now().Add(time.Minute), false, false, "HELD")
	hold(11, time.Now().Add(time.Minute), true, false, "HELD")
	hold(13, time.Now().Add(time.Minute), false, true, "HELD")
	hold(2, time.Now().Add(-time.Hour), false, false, "PAYMENT_PENDING") // expired but unswept: still held
	// PAID order whose hold is not yet consumed: counted in paid/sold, NOT in held (no double count).
	hold(4, time.Now().Add(time.Minute), false, false, "PAID")

	for _, s := range []struct {
		status string
		n      int
	}{{"ISSUED", 2}, {"CHECKED_IN", 1}, {"VOID", 4}, {"REFUNDED", 3}} {
		for i := 0; i < s.n; i++ {
			exec(`INSERT INTO tickets (order_id, ticket_type_id, jti, qr_sig, status) VALUES ($1::uuid,$2::uuid,$3,'sig',$4)`,
				paid, f.tierA, uuid.NewString(), s.status)
		}
	}
	return f
}

func TestOpsRepository_AC14_TierInventoryDefinitions(t *testing.T) {
	repo, pool := setupPostgresTest(t)
	f := seedOps(t, pool)
	rows, err := repo.TierInventory(context.Background(), f.event)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0].Name != "A" || rows[1].Name != "B" { // ordered by price_cents, name
		t.Fatalf("rows = %+v", rows)
	}
	a, b := rows[0], rows[1]
	if a.Quota != 100 || a.Available != 50 {
		t.Errorf("A quota/available = %d/%d, want 100/50", a.Quota, a.Available)
	}
	if a.Held != 7 { // open 5 + open-but-expired 2; released/consumed excluded
		t.Errorf("A held = %d, want 7", a.Held)
	}
	if a.Paid != 4 { // PAID orders only: 3 + 1 (the PAID carrier order of the unconsumed hold); ISSUED excluded
		t.Errorf("A paid = %d, want 4", a.Paid)
	}
	if a.Issued != 3 { // ISSUED 2 + CHECKED_IN 1; VOID/REFUNDED excluded
		t.Errorf("A issued = %d, want 3", a.Issued)
	}
	if b.Quota != 50 || b.Available != 50 || b.Held != 0 || b.Paid != 1 || b.Issued != 0 {
		t.Errorf("B = %+v", b)
	}
}

func TestOpsRepository_AC14_EventExistsAndEmptyTiers(t *testing.T) {
	repo, pool := setupPostgresTest(t)
	f := seedOps(t, pool)
	ctx := context.Background()
	if ok, err := repo.EventExists(ctx, f.event); err != nil || !ok {
		t.Fatalf("exists = %v %v", ok, err)
	}
	if ok, err := repo.EventExists(ctx, uuid.NewString()); err != nil || ok {
		t.Fatalf("unknown exists = %v %v", ok, err)
	}
	empty := uuid.NewString()
	if _, err := pool.Exec(ctx, `INSERT INTO events (id, organizer_id, slug, title, starts_at, sale_start_at, status, max_per_identity, max_per_order)
		VALUES ($1::uuid,$2::uuid,$3,'E',now()+interval '9 days',now(),'DRAFT',4,4)`, empty, f.organizer, "e-"+empty); err != nil {
		t.Fatal(err)
	}
	rows, err := repo.TierInventory(ctx, empty)
	if err != nil || len(rows) != 0 {
		t.Fatalf("empty event: %v %v", rows, err)
	}
}

func TestOpsRepository_AC15_SalesOnlyPaidAndIssuedOrders(t *testing.T) {
	repo, pool := setupPostgresTest(t)
	f := seedOps(t, pool)
	s, err := repo.EventSales(context.Background(), f.event)
	if err != nil {
		t.Fatal(err)
	}
	if s.PaidOrders != 4 {
		t.Errorf("paid orders = %d, want 4 (PAID,ISSUED,PAID,PAID-carrier)", s.PaidOrders)
	}
	if len(s.Tiers) != 2 {
		t.Fatalf("tiers %+v", s.Tiers)
	}
	a, b := s.Tiers[0], s.Tiers[1]
	if a.Sold != 6 || a.RevenueCents != 600000 { // 3+2+1 tickets x 100000; REFUNDING/REFUNDED/... excluded
		t.Errorf("A sold/revenue = %d/%d, want 6/600000", a.Sold, a.RevenueCents)
	}
	if a.ActiveHolds != 7 || a.PriceCents != 100000 || a.Quota != 100 {
		t.Errorf("A holds/price/quota = %d/%d/%d", a.ActiveHolds, a.PriceCents, a.Quota)
	}
	if b.Sold != 1 || b.RevenueCents != 200000 {
		t.Errorf("B sold/revenue = %d/%d", b.Sold, b.RevenueCents)
	}
	if len(s.Payments) != 2 {
		t.Fatalf("payments %+v", s.Payments)
	}
	// sorted by amount desc: vnpay 300000+200000=500000 (2 orders), momo 200000 (1 order)
	if s.Payments[0].Provider != "vnpay" || s.Payments[0].Orders != 2 || s.Payments[0].AmountCents != 500000 {
		t.Errorf("payments[0] = %+v", s.Payments[0])
	}
	if s.Payments[1].Provider != "momo" || s.Payments[1].Orders != 1 || s.Payments[1].AmountCents != 200000 {
		t.Errorf("payments[1] = %+v", s.Payments[1])
	}
}

func TestOpsRepository_AC16_EventOwner(t *testing.T) {
	repo, pool := setupPostgresTest(t)
	f := seedOps(t, pool)
	got, err := repo.EventOwner(context.Background(), f.event)
	if err != nil || got != f.organizer {
		t.Fatalf("owner = %q %v", got, err)
	}
	if _, err := repo.EventOwner(context.Background(), uuid.NewString()); err != domain.ErrEventNotFound {
		t.Fatalf("unknown event err = %v, want ErrEventNotFound", err)
	}
}
