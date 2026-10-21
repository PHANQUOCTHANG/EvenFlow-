//go:build integration

package postgres_test

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "github.com/lib/pq"

	"github.com/eventflow/eventflow/libs/go/testx"
	"github.com/eventflow/eventflow/services/event/internal/adapter/postgres"
	"github.com/eventflow/eventflow/services/event/internal/domain"
	"github.com/eventflow/eventflow/services/event/internal/port"
)

func TestMain(m *testing.M) {
	if os.Getenv("TESTCONTAINERS_RYUK_DISABLED") == "" {
		_ = os.Setenv("TESTCONTAINERS_RYUK_DISABLED", "true")
	}
	code := m.Run()
	testx.Shutdown()
	os.Exit(code)
}

// Bat bien 1: su kien doi trang thai va job outbox di cung mot transaction. Message thu hai
// hong (payload khong marshal duoc) -> KHONG dong nao duoc ghi, ke ca su kien lan message
// thu nhat da ghi truoc do trong cung transaction.
func TestTichHop_UpdateWithOutbox_HongGiuaChungThiKhongGhiGi(t *testing.T) {
	ctx := context.Background()
	db, err := sql.Open("postgres", testx.Postgres(t))
	if err != nil {
		t.Fatalf("mo postgres: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Exec("DELETE FROM outbox_events"); err != nil {
		t.Fatalf("don outbox: %v", err)
	}

	organizerID := uuid.New()
	if _, err := db.Exec(`INSERT INTO identities (id, email, password_hash, role) VALUES ($1, $2, 'x', 'organizer')`,
		organizerID, organizerID.String()+"@test.local"); err != nil {
		t.Fatalf("tao organizer: %v", err)
	}
	evt, err := domain.NewEvent(organizerID, "tx-"+organizerID.String()[:8], "Tx", "", "",
		time.Now().Add(72*time.Hour), time.Now().Add(24*time.Hour), nil)
	if err != nil {
		t.Fatalf("NewEvent: %v", err)
	}
	repo := postgres.NewEventRepository(db)
	if err := repo.Create(ctx, evt); err != nil {
		t.Fatalf("Create: %v", err)
	}

	evt.Status = domain.StatusPendingReview
	good := port.OutboxMessage{AggregateType: "event", AggregateID: evt.ID, RoutingKey: "moderation", Payload: map[string]any{"ok": 1}}
	bad := port.OutboxMessage{AggregateType: "event", AggregateID: evt.ID, RoutingKey: "moderation", Payload: make(chan int)}
	if err := repo.UpdateWithOutbox(ctx, evt, good, bad); err == nil {
		t.Fatal("payload khong marshal duoc phai loi")
	}

	var status string
	if err := db.QueryRow("SELECT status FROM events WHERE id = $1", evt.ID).Scan(&status); err != nil {
		t.Fatalf("doc su kien: %v", err)
	}
	if status != string(domain.StatusDraft) {
		t.Errorf("su kien khong duoc doi trang thai khi transaction hong, got %s", status)
	}
	var n int
	if err := db.QueryRow("SELECT count(*) FROM outbox_events WHERE aggregate_id = $1", evt.ID).Scan(&n); err != nil {
		t.Fatalf("dem outbox: %v", err)
	}
	if n != 0 {
		t.Errorf("khong duoc con dong outbox nao khi transaction hong, got %d", n)
	}

	// Duong thuan: ca hai cung duoc ghi.
	if err := repo.UpdateWithOutbox(ctx, evt, good); err != nil {
		t.Fatalf("UpdateWithOutbox: %v", err)
	}
	_ = db.QueryRow("SELECT status FROM events WHERE id = $1", evt.ID).Scan(&status)
	_ = db.QueryRow("SELECT count(*) FROM outbox_events WHERE aggregate_id = $1", evt.ID).Scan(&n)
	if status != string(domain.StatusPendingReview) || n != 1 {
		t.Errorf("duong thuan: status=%s outbox=%d", status, n)
	}
}

// Danh sach cong khai chi gom SCHEDULED/ON_SALE/SOLD_OUT, gan mo ban nhat truoc, ton trong limit.
func TestTichHop_ListPublic_LocTrangThaiVaThuTu(t *testing.T) {
	ctx := context.Background()
	db, err := sql.Open("postgres", testx.Postgres(t))
	if err != nil {
		t.Fatalf("mo postgres: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	org := uuid.New()
	if _, err := db.Exec(`INSERT INTO identities (id, email, password_hash, role) VALUES ($1, $2, 'x', 'organizer')`,
		org, org.String()+"@test.local"); err != nil {
		t.Fatal(err)
	}
	// Truoc moi su kien khac trong DB dung chung cua test: T0 rat som de dung dau danh sach.
	base := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	add := func(st domain.EventStatus, offset time.Duration) uuid.UUID {
		id := uuid.New()
		_, err := db.Exec(`INSERT INTO events (id, organizer_id, slug, title, starts_at, sale_start_at, status)
			VALUES ($1, $2, $3, 't', $4, $5, $6)`, id, org, "p-"+id.String(), base.Add(240*time.Hour), base.Add(offset), string(st))
		if err != nil {
			t.Fatalf("seed %s: %v", st, err)
		}
		return id
	}
	soldOut := add(domain.StatusSoldOut, 3*time.Hour)
	onSale := add(domain.StatusOnSale, time.Hour)
	scheduled := add(domain.StatusScheduled, 2*time.Hour)
	add(domain.StatusDraft, 0)
	add(domain.StatusPendingReview, 0)
	add(domain.StatusCancelled, 0)

	got, err := postgres.NewEventRepository(db).ListPublic(ctx, 3)
	if err != nil {
		t.Fatal(err)
	}
	want := []uuid.UUID{onSale, scheduled, soldOut}
	if len(got) != 3 {
		t.Fatalf("tra %d su kien, muon 3", len(got))
	}
	for i, e := range got {
		if e.ID != want[i] {
			t.Errorf("vi tri %d = %s (%s), muon %s", i, e.ID, e.Status, want[i])
		}
	}
}

// Chi SCHEDULED da den gio duoc mo; goi lai khong doi gi them (hai instance cung chay van an toan).
func TestTichHop_OpenDueSales_ChiMoSuKienDenGio(t *testing.T) {
	ctx := context.Background()
	db, err := sql.Open("postgres", testx.Postgres(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	org := uuid.New()
	if _, err := db.Exec(`INSERT INTO identities (id, email, password_hash, role) VALUES ($1, $2, 'x', 'organizer')`,
		org, org.String()+"@test.local"); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2030, 1, 1, 20, 0, 0, 0, time.UTC)
	add := func(st domain.EventStatus, t0 time.Time) uuid.UUID {
		id := uuid.New()
		if _, err := db.Exec(`INSERT INTO events (id, organizer_id, slug, title, starts_at, sale_start_at, status)
			VALUES ($1, $2, $3, 't', $4, $5, $6)`, id, org, "o-"+id.String(), t0.Add(24*time.Hour), t0, string(st)); err != nil {
			t.Fatal(err)
		}
		return id
	}
	due := add(domain.StatusScheduled, now.Add(-time.Second))
	exact := add(domain.StatusScheduled, now)
	later := add(domain.StatusScheduled, now.Add(time.Minute))
	pending := add(domain.StatusPendingReview, now.Add(-time.Hour))

	repo := postgres.NewSaleScheduleRepository(db)
	ids, err := repo.OpenDueSales(ctx, now)
	if err != nil {
		t.Fatal(err)
	}
	opened := map[uuid.UUID]bool{}
	for _, id := range ids {
		opened[id] = true
	}
	if !opened[due] || !opened[exact] || opened[later] || opened[pending] {
		t.Fatalf("mo sai: %v", ids)
	}
	status := func(id uuid.UUID) string {
		var s string
		_ = db.QueryRow(`SELECT status FROM events WHERE id = $1`, id).Scan(&s)
		return s
	}
	if status(due) != "ON_SALE" || status(later) != "SCHEDULED" || status(pending) != "PENDING_REVIEW" {
		t.Errorf("trang thai sau khi mo: %s %s %s", status(due), status(later), status(pending))
	}
	again, err := repo.OpenDueSales(ctx, now)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range again {
		if id == due || id == exact {
			t.Errorf("goi lai mo lai su kien %s", id)
		}
	}
}
