//go:build integration

package worker_test

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/eventflow/eventflow/libs/go/testx"
	postgresadapter "github.com/eventflow/eventflow/services/ticketing/internal/adapter/postgres"
	redisadapter "github.com/eventflow/eventflow/services/ticketing/internal/adapter/redis"
	"github.com/eventflow/eventflow/services/ticketing/internal/domain"
	"github.com/eventflow/eventflow/services/ticketing/internal/port"
	"github.com/eventflow/eventflow/services/ticketing/internal/worker"
)

func TestMain(m *testing.M) {
	if os.Getenv("TESTCONTAINERS_RYUK_DISABLED") == "" {
		_ = os.Setenv("TESTCONTAINERS_RYUK_DISABLED", "true")
	}
	code := m.Run()
	testx.Shutdown()
	os.Exit(code)
}

type sweeperIntegrationEnv struct {
	pool *pgxpool.Pool
	rdb  *redis.Client
	repo *postgresadapter.Repository
	gate *redisadapter.Gate
}

func setupSweeperIntegrationEnv(t *testing.T) *sweeperIntegrationEnv {
	t.Helper()
	ctx := context.Background()

	dsn := testx.Postgres(t)
	addr := testx.Redis(t)

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("failed to connect postgres: %v", err)
	}
	t.Cleanup(pool.Close)

	rdb := redis.NewClient(&redis.Options{Addr: addr})
	t.Cleanup(func() { _ = rdb.Close() })

	gate, err := redisadapter.NewGate(ctx, rdb)
	if err != nil {
		t.Fatalf("failed to init redis gate: %v", err)
	}

	repo := postgresadapter.NewRepository(pool)
	return &sweeperIntegrationEnv{
		pool: pool,
		rdb:  rdb,
		repo: repo,
		gate: gate,
	}
}

type sweeperFixture struct {
	EventID      string
	TicketTypeID string
	IdentityIDs  []string
}

func seedSweeperEvent(t *testing.T, env *sweeperIntegrationEnv, quota int, buyerCount int) sweeperFixture {
	t.Helper()
	ctx := context.Background()

	orgID := uuid.New().String()
	eventID := uuid.New().String()
	ticketTypeID := uuid.New().String()

	const insOrg = `INSERT INTO identities (id, email, password_hash, role) VALUES ($1::uuid, $2, 'pass', 'organizer')`
	if _, err := env.pool.Exec(ctx, insOrg, orgID, "org-"+orgID+"@test.com"); err != nil {
		t.Fatalf("seed org: %v", err)
	}

	buyerIDs := make([]string, buyerCount)
	for i := 0; i < buyerCount; i++ {
		bID := uuid.New().String()
		buyerIDs[i] = bID
		const insBuyer = `INSERT INTO identities (id, email, password_hash, role) VALUES ($1::uuid, $2, 'pass', 'buyer')`
		if _, err := env.pool.Exec(ctx, insBuyer, bID, fmt.Sprintf("buyer-%s@test.com", bID)); err != nil {
			t.Fatalf("seed buyer: %v", err)
		}
	}

	const insEvent = `INSERT INTO events (id, organizer_id, slug, title, starts_at, sale_start_at, status, max_per_identity, max_per_order, hold_ttl_seconds)
		VALUES ($1::uuid, $2::uuid, $3, 'Sweeper Event', now() + interval '7 days', now(), 'ON_SALE', 4, 4, 600)`
	if _, err := env.pool.Exec(ctx, insEvent, eventID, orgID, "event-"+eventID); err != nil {
		t.Fatalf("seed event: %v", err)
	}

	const insType = `INSERT INTO ticket_types (id, event_id, name, price_cents, quota, bucket_count)
		VALUES ($1::uuid, $2::uuid, 'Standard', 100000, $3, 32)`
	if _, err := env.pool.Exec(ctx, insType, ticketTypeID, eventID, quota); err != nil {
		t.Fatalf("seed ticket type: %v", err)
	}

	if err := env.repo.InitInventory(ctx, ticketTypeID, quota, 32); err != nil {
		t.Fatalf("seed postgres inventory: %v", err)
	}

	rows, err := env.repo.ListInventory(ctx, eventID)
	if err != nil {
		t.Fatalf("list inventory: %v", err)
	}
	perBucket := make(map[int]int, len(rows))
	for _, r := range rows {
		perBucket[r.Bucket] = r.Available
	}
	if err := env.gate.SeedInventory(ctx, eventID, ticketTypeID, perBucket); err != nil {
		t.Fatalf("seed redis inventory: %v", err)
	}

	return sweeperFixture{
		EventID:      eventID,
		TicketTypeID: ticketTypeID,
		IdentityIDs:  buyerIDs,
	}
}

type integrationSpyPub struct {
	mu   sync.Mutex
	keys []string
	msgs []map[string]any
}

func (p *integrationSpyPub) Publish(_ context.Context, key string, payload any) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.keys = append(p.keys, key)
	if m, ok := payload.(map[string]any); ok {
		p.msgs = append(p.msgs, m)
	}
	return nil
}

func (p *integrationSpyPub) count() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.keys)
}

func TestHoldSweeper_Integration_EndToEnd(t *testing.T) {
	env := setupSweeperIntegrationEnv(t)
	ctx := context.Background()
	fix := seedSweeperEvent(t, env, 320, 1)

	holdID := domain.NewID()
	// TTL ngắn (50ms) để hold nhanh hết hạn
	const shortTTL = 50 * time.Millisecond
	holdRes, err := env.gate.Hold(ctx, port.HoldRequest{
		EventID:      fix.EventID,
		TicketTypeID: fix.TicketTypeID,
		IdentityID:   fix.IdentityIDs[0],
		HoldID:       holdID,
		Quantity:     2,
		TTL:          shortTTL,
		BucketCount:  32,
		StartBucket:  3,
	})
	if err != nil || holdRes.Status != port.HoldOK {
		t.Fatalf("gate.Hold failed: res=%+v, err=%v", holdRes, err)
	}

	// Persist vào Postgres
	expiresAt := time.Now().Add(shortTTL)
	out, err := env.repo.PersistHold(ctx, port.PersistHoldParams{
		EventID:        fix.EventID,
		TicketTypeID:   fix.TicketTypeID,
		IdentityID:     fix.IdentityIDs[0],
		HoldID:         holdID,
		Bucket:         holdRes.Bucket,
		Quantity:       2,
		UnitCents:      100000,
		ExpiresAt:      expiresAt,
		MaxPerIdentity: 4,
	})
	if err != nil {
		t.Fatalf("PersistHold failed: %v", err)
	}

	// Xác nhận kho trong Postgres giảm còn 8 ở bucket đó
	var availPg int
	_ = env.pool.QueryRow(ctx,
		"SELECT available FROM ticket_inventory WHERE ticket_type_id = $1::uuid AND bucket = $2",
		fix.TicketTypeID, holdRes.Bucket).Scan(&availPg)
	if availPg != 8 {
		t.Fatalf("available in Postgres before sweep = %d, expected 8", availPg)
	}

	// Chờ hold hết hạn trong Redis ZSET
	time.Sleep(100 * time.Millisecond)

	// Chạy sweeper
	pub := &integrationSpyPub{}
	cfg := worker.SweeperConfig{
		EventIDs: []string{fix.EventID},
		Interval: 20 * time.Millisecond,
		Batch:    100,
	}
	sweeper := worker.NewHoldSweeper(env.gate, env.repo, pub, cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))

	ctxSweep, cancelSweep := context.WithCancel(context.Background())
	defer cancelSweep()
	go func() { _ = sweeper.Run(ctxSweep) }()

	// Chờ sweeper quét và phát event
	deadline := time.Now().Add(2 * time.Second)
	for pub.count() == 0 {
		if time.Now().After(deadline) {
			t.Fatal("sweeper did not release hold within 2s")
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancelSweep()

	// 1. Kiểm tra Postgres: available đã được trả lại đủ 10
	var availPgAfter int
	_ = env.pool.QueryRow(ctx,
		"SELECT available FROM ticket_inventory WHERE ticket_type_id = $1::uuid AND bucket = $2",
		fix.TicketTypeID, holdRes.Bucket).Scan(&availPgAfter)
	if availPgAfter != 10 {
		t.Fatalf("available in Postgres after sweep = %d, expected 10", availPgAfter)
	}

	// 2. Kiểm tra Postgres: order status là EXPIRED
	var orderStatus string
	_ = env.pool.QueryRow(ctx, "SELECT status FROM orders WHERE id = $1::uuid", out.OrderID).Scan(&orderStatus)
	if orderStatus != "EXPIRED" {
		t.Fatalf("order status = %s, expected EXPIRED", orderStatus)
	}

	// 3. Kiểm tra Postgres: seat_holds.released_at IS NOT NULL
	var releasedAt *time.Time
	_ = env.pool.QueryRow(ctx, "SELECT released_at FROM seat_holds WHERE id = $1::uuid", out.HoldID).Scan(&releasedAt)
	if releasedAt == nil {
		t.Fatal("seat_holds.released_at is still nil")
	}

	// 4. Kiểm tra Publisher: đúng routing key và hold_id
	pub.mu.Lock()
	if len(pub.keys) != 1 || pub.keys[0] != "ticketing.hold.released" || pub.msgs[0]["hold_id"] != holdID {
		t.Fatalf("unexpected pub events: keys=%v, msgs=%v", pub.keys, pub.msgs)
	}
	pub.mu.Unlock()
}

func TestHoldSweeper_Integration_Idempotent_BRO7(t *testing.T) {
	env := setupSweeperIntegrationEnv(t)
	ctx := context.Background()
	fix := seedSweeperEvent(t, env, 320, 1)

	holdID := domain.NewID()
	const shortTTL = 50 * time.Millisecond
	holdRes, err := env.gate.Hold(ctx, port.HoldRequest{
		EventID:      fix.EventID,
		TicketTypeID: fix.TicketTypeID,
		IdentityID:   fix.IdentityIDs[0],
		HoldID:       holdID,
		Quantity:     3,
		TTL:          shortTTL,
		BucketCount:  32,
		StartBucket:  7,
	})
	if err != nil || holdRes.Status != port.HoldOK {
		t.Fatalf("gate.Hold failed: %v", err)
	}

	_, err = env.repo.PersistHold(ctx, port.PersistHoldParams{
		EventID:        fix.EventID,
		TicketTypeID:   fix.TicketTypeID,
		IdentityID:     fix.IdentityIDs[0],
		HoldID:         holdID,
		Bucket:         holdRes.Bucket,
		Quantity:       3,
		UnitCents:      100000,
		ExpiresAt:      time.Now().Add(shortTTL),
		MaxPerIdentity: 4,
	})
	if err != nil {
		t.Fatalf("PersistHold failed: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	pub := &integrationSpyPub{}
	cfg := worker.SweeperConfig{
		EventIDs: []string{fix.EventID},
		Interval: 10 * time.Millisecond,
		Batch:    100,
	}
	sweeper := worker.NewHoldSweeper(env.gate, env.repo, pub, cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))

	ctxSweep, cancelSweep := context.WithCancel(context.Background())
	defer cancelSweep()
	go func() { _ = sweeper.Run(ctxSweep) }()

	// Cho chạy qua nhiều vòng quét
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	cancelSweep()

	// Kiểm tra kho: đúng 10, không bị nhân đôi
	var avail int
	_ = env.pool.QueryRow(ctx,
		"SELECT available FROM ticket_inventory WHERE ticket_type_id = $1::uuid AND bucket = $2",
		fix.TicketTypeID, holdRes.Bucket).Scan(&avail)
	if avail != 10 {
		t.Fatalf("inventory double-credited: available = %d, expected 10", avail)
	}

	// Kiểm tra Publisher: chỉ phát đúng 1 lần
	if pub.count() != 1 {
		t.Fatalf("published count = %d, expected 1 (BR-O7)", pub.count())
	}
}

func TestHoldSweeper_Integration_ConcurrentSweepers(t *testing.T) {
	env := setupSweeperIntegrationEnv(t)
	ctx := context.Background()
	const count = 6
	fix := seedSweeperEvent(t, env, 320, count)

	const shortTTL = 50 * time.Millisecond
	for i := 0; i < count; i++ {
		hID := domain.NewID()
		hRes, err := env.gate.Hold(ctx, port.HoldRequest{
			EventID:      fix.EventID,
			TicketTypeID: fix.TicketTypeID,
			IdentityID:   fix.IdentityIDs[i],
			HoldID:       hID,
			Quantity:     1,
			TTL:          shortTTL,
			BucketCount:  32,
			StartBucket:  i % 32,
		})
		if err != nil || hRes.Status != port.HoldOK {
			t.Fatalf("hold %d failed: %v", i, err)
		}
		_, err = env.repo.PersistHold(ctx, port.PersistHoldParams{
			EventID:        fix.EventID,
			TicketTypeID:   fix.TicketTypeID,
			IdentityID:     fix.IdentityIDs[i],
			HoldID:         hID,
			Bucket:         hRes.Bucket,
			Quantity:       1,
			UnitCents:      100000,
			ExpiresAt:      time.Now().Add(shortTTL),
			MaxPerIdentity: 4,
		})
		if err != nil {
			t.Fatalf("persist %d failed: %v", i, err)
		}
	}

	time.Sleep(100 * time.Millisecond)

	pub := &integrationSpyPub{}
	cfg := worker.SweeperConfig{
		EventIDs: []string{fix.EventID},
		Interval: 10 * time.Millisecond,
		Batch:    100,
	}

	ctxSweep, cancelSweep := context.WithCancel(context.Background())
	defer cancelSweep()

	// 3 sweeper chạy song song
	var wg sync.WaitGroup
	for i := 0; i < 3; i++ {
		wg.Add(1)
		sw := worker.NewHoldSweeper(env.gate, env.repo, pub, cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
		go func() {
			defer wg.Done()
			_ = sw.Run(ctxSweep)
		}()
	}

	deadline := time.Now().Add(2 * time.Second)
	for pub.count() < count {
		if time.Now().After(deadline) {
			t.Fatalf("concurrent sweepers only released %d of %d holds", pub.count(), count)
		}
		time.Sleep(20 * time.Millisecond)
	}
	cancelSweep()
	wg.Wait()

	// Tổng kho Postgres phải về đúng 320
	var totalAvail int
	_ = env.pool.QueryRow(ctx,
		"SELECT coalesce(sum(available), 0) FROM ticket_inventory WHERE ticket_type_id = $1::uuid",
		fix.TicketTypeID).Scan(&totalAvail)
	if totalAvail != 320 {
		t.Fatalf("total available = %d, expected 320", totalAvail)
	}

	if pub.count() != count {
		t.Fatalf("published count = %d, expected %d", pub.count(), count)
	}
}
