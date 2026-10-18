//go:build integration

package worker_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "github.com/lib/pq"
	amqp "github.com/rabbitmq/amqp091-go"

	"github.com/eventflow/eventflow/libs/go/mq"
	"github.com/eventflow/eventflow/libs/go/testx"
	"github.com/eventflow/eventflow/services/event/internal/adapter/postgres"
	"github.com/eventflow/eventflow/services/event/internal/app"
	"github.com/eventflow/eventflow/services/event/internal/worker"
)

func TestMain(m *testing.M) {
	if os.Getenv("TESTCONTAINERS_RYUK_DISABLED") == "" {
		_ = os.Setenv("TESTCONTAINERS_RYUK_DISABLED", "true")
	}
	code := m.Run()
	testx.Shutdown()
	os.Exit(code)
}

type env struct {
	db  *sql.DB
	ch  *amqp.Channel
	pub *mq.Publisher
}

func setup(t *testing.T) env {
	t.Helper()
	db, err := sql.Open("postgres", testx.Postgres(t))
	if err != nil {
		t.Fatalf("mo postgres: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Exec("DELETE FROM outbox_events"); err != nil {
		t.Fatalf("don outbox: %v", err)
	}

	conn, err := amqp.Dial(testx.RabbitMQ(t))
	if err != nil {
		t.Fatalf("ket noi rabbitmq: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	ch, err := conn.Channel()
	if err != nil {
		t.Fatalf("mo kenh: %v", err)
	}
	if err := mq.Declare(ch, mq.AITopology()); err != nil {
		t.Fatalf("khai bao topology AI: %v", err)
	}
	if _, err := ch.QueuePurge("ai.moderation.q", false); err != nil {
		t.Fatalf("purge: %v", err)
	}
	pubCh, err := conn.Channel()
	if err != nil {
		t.Fatalf("mo kenh publish: %v", err)
	}
	pub, err := mq.NewPublisher(pubCh)
	if err != nil {
		t.Fatalf("publisher: %v", err)
	}
	return env{db: db, ch: ch, pub: pub}
}

func runRelay(t *testing.T, e env) func() {
	t.Helper()
	cfg := worker.OutboxRelayConfig{Interval: 20 * time.Millisecond, Batch: 10}
	relay, err := worker.NewOutboxRelay(e.db, e.pub, worker.DefaultRoutes(), cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("NewOutboxRelay: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); _ = relay.Run(ctx) }()
	return func() { cancel(); <-done }
}

func insertRow(t *testing.T, db *sql.DB, aggType, key string) int64 {
	t.Helper()
	var id int64
	err := db.QueryRow(`INSERT INTO outbox_events (aggregate_type, aggregate_id, routing_key, payload)
		VALUES ($1, $2, $3, '{}') RETURNING id`, aggType, uuid.New(), key).Scan(&id)
	if err != nil {
		t.Fatalf("chen outbox: %v", err)
	}
	return id
}

func publishedAt(t *testing.T, db *sql.DB, id int64) *time.Time {
	t.Helper()
	var ts *time.Time
	if err := db.QueryRow("SELECT published_at FROM outbox_events WHERE id = $1", id).Scan(&ts); err != nil {
		t.Fatalf("doc outbox %d: %v", id, err)
	}
	return ts
}

// Luong that: nop duyet qua repository Postgres -> relay -> ai.direct -> ai.moderation.q.
// Truoc ban sua, job nam yen trong Postgres (khong co relay) va neu co relay thi cung bi
// tra ve vi routing key "ai.moderation.q" khong khop binding "moderation".
func TestTichHop_NopDuyet_JobToiHangKiemDuyet(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	repo := postgres.NewEventRepository(e.db)

	organizerID := uuid.New()
	// events.organizer_id tham chieu identities: tao nguoi to chuc truoc.
	if _, err := e.db.Exec(`INSERT INTO identities (id, email, password_hash, role)
		VALUES ($1, $2, 'x', 'organizer')`, organizerID, organizerID.String()+"@test.local"); err != nil {
		t.Fatalf("tao organizer: %v", err)
	}
	createUC := app.NewCreateEventUseCase(repo)
	evt, err := createUC.Execute(ctx, app.CreateEventInput{
		OrganizerID: organizerID,
		Title:       "Dem nhac tich hop",
		StartsAt:    time.Now().Add(72 * time.Hour),
		SaleStartAt: time.Now().Add(24 * time.Hour),
	})
	if err != nil {
		t.Fatalf("tao su kien: %v", err)
	}
	if _, err := app.NewManageTiersUseCase(repo).AddTier(ctx, app.AddTierInput{
		EventID: evt.ID, OrganizerID: organizerID, Name: "GA", PriceCents: 100000, Quota: 64,
	}); err != nil {
		t.Fatalf("them hang ve: %v", err)
	}
	// Dong cua ticketing dung TRUOC, nhieu hon mot lo (Batch = 10): khong loc aggregate thi
	// lo nao cung toan dong ticketing, bi bo qua, va job kiem duyet khong bao gio toi luot.
	var orderID int64
	for i := 0; i < 12; i++ {
		orderID = insertRow(t, e.db, "order", "ticketing.hold.created")
	}
	if _, err := app.NewTransitionStateUseCase(repo).SubmitForReview(ctx, evt.ID, organizerID); err != nil {
		t.Fatalf("nop duyet: %v", err)
	}
	stop := runRelay(t, e)
	defer stop()

	var got amqp.Delivery
	deadline := time.Now().Add(5 * time.Second)
	for {
		d, ok, err := e.ch.Get("ai.moderation.q", true)
		if err != nil {
			t.Fatalf("get: %v", err)
		}
		if ok {
			got = d
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("job kiem duyet khong toi ai.moderation.q")
		}
		time.Sleep(20 * time.Millisecond)
	}
	stop()

	var job map[string]any
	if err := json.Unmarshal(got.Body, &job); err != nil {
		t.Fatalf("body khong phai JSON: %v", err)
	}
	if job["event_id"] != evt.ID.String() || job["event_title"] != "Dem nhac tich hop" {
		t.Errorf("payload sai: %v", job)
	}
	if got.ContentType != "application/json" || got.MessageId == "" || got.Timestamp.IsZero() {
		t.Errorf("thieu property: content_type=%q message_id=%q ts=%v", got.ContentType, got.MessageId, got.Timestamp)
	}
	id, _ := strconv.ParseInt(got.MessageId, 10, 64)
	if publishedAt(t, e.db, id) == nil {
		t.Error("dong outbox da phat phai co published_at")
	}
	if publishedAt(t, e.db, orderID) != nil {
		t.Error("relay event khong duoc danh dau dong cua ticketing")
	}
}

func TestTichHop_RoutingKeyLa_KhongChanDongSau(t *testing.T) {
	e := setup(t)
	khongTuyen := insertRow(t, e.db, "event", "khong.co.tuyen")
	ok := insertRow(t, e.db, "event", app.ModerationRoutingKey)

	stop := runRelay(t, e)
	defer stop()

	deadline := time.Now().Add(5 * time.Second)
	for publishedAt(t, e.db, ok) == nil {
		if time.Now().After(deadline) {
			t.Fatal("dong hop le bi chan boi dong co routing key khong co tuyen")
		}
		time.Sleep(20 * time.Millisecond)
	}
	stop()
	if publishedAt(t, e.db, khongTuyen) != nil {
		t.Error("dong khong co tuyen khong duoc danh dau da phat")
	}
}
