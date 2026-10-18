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
