//go:build integration

package postgresadapter_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/eventflow/eventflow/libs/go/testx"
	postgresadapter "github.com/eventflow/eventflow/services/ticketing/internal/adapter/postgres"
	"github.com/eventflow/eventflow/services/ticketing/internal/domain"
	"github.com/eventflow/eventflow/services/ticketing/internal/port"
)

func TestMain(m *testing.M) {
	if os.Getenv("TESTCONTAINERS_RYUK_DISABLED") == "" {
		_ = os.Setenv("TESTCONTAINERS_RYUK_DISABLED", "true")
	}
	code := m.Run()
	testx.Shutdown()
	os.Exit(code)
}

func setupPostgresTest(t *testing.T) (*postgresadapter.Repository, *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	dsn := testx.Postgres(t)

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("failed to open pgxpool: %v", err)
	}
	t.Cleanup(pool.Close)

	repo := postgresadapter.NewRepository(pool)
	return repo, pool
}

type testFixture struct {
	EventID      string
	TicketTypeID string
	IdentityID   string
	OrganizerID  string
}

func seedTestEvent(t *testing.T, pool *pgxpool.Pool, quota int) testFixture {
	t.Helper()
	ctx := context.Background()

	orgID := uuid.New().String()
	buyerID := uuid.New().String()
	eventID := uuid.New().String()
	ticketTypeID := uuid.New().String()

	const insOrg = `
		INSERT INTO identities (id, email, password_hash, role, otp_verified_at)
		VALUES ($1::uuid, $2, 'pass-hash', 'organizer', now())`
	if _, err := pool.Exec(ctx, insOrg, orgID, "org-"+orgID+"@test.com"); err != nil {
		t.Fatalf("seed organizer: %v", err)
	}

	const insBuyer = `
		INSERT INTO identities (id, email, password_hash, role, otp_verified_at)
		VALUES ($1::uuid, $2, 'pass-hash', 'buyer', now())`
	if _, err := pool.Exec(ctx, insBuyer, buyerID, "buyer-"+buyerID+"@test.com"); err != nil {
		t.Fatalf("seed buyer: %v", err)
	}

	const insEvent = `
		INSERT INTO events (id, organizer_id, slug, title, starts_at, sale_start_at, status, max_per_identity, max_per_order, hold_ttl_seconds)
		VALUES ($1::uuid, $2::uuid, $3, 'Integration Test Event', now() + interval '7 days', now(), 'ON_SALE', 4, 4, 600)`
	if _, err := pool.Exec(ctx, insEvent, eventID, orgID, "event-"+eventID); err != nil {
		t.Fatalf("seed event: %v", err)
	}

	const insType = `
		INSERT INTO ticket_types (id, event_id, name, price_cents, quota, bucket_count)
		VALUES ($1::uuid, $2::uuid, 'VIP', 500000, $3, 32)`
	if _, err := pool.Exec(ctx, insType, ticketTypeID, eventID, quota); err != nil {
		t.Fatalf("seed ticket type: %v", err)
	}

	repo := postgresadapter.NewRepository(pool)
	if err := repo.InitInventory(ctx, ticketTypeID, quota, 32); err != nil {
		t.Fatalf("seed inventory: %v", err)
	}

	return testFixture{
		EventID:      eventID,
		TicketTypeID: ticketTypeID,
		IdentityID:   buyerID,
		OrganizerID:  orgID,
	}
}

func TestPersistHold_TruKhoDungBucket(t *testing.T) {
	repo, pool := setupPostgresTest(t)
	ctx := context.Background()
	fix := seedTestEvent(t, pool, 320) // 10 vé / bucket

	holdID := domain.NewID()
	expiresAt := time.Now().Add(10 * time.Minute)
	const targetBucket = 5
	const holdQty = 3

	// 1. Kiểm tra available trước khi hold
	var availBefore int
	err := pool.QueryRow(ctx,
		"SELECT available FROM ticket_inventory WHERE ticket_type_id = $1::uuid AND bucket = $2",
		fix.TicketTypeID, targetBucket).Scan(&availBefore)
	if err != nil {
		t.Fatalf("query inventory before: %v", err)
	}
	if availBefore != 10 {
		t.Fatalf("available before = %d, expected 10", availBefore)
	}

	// 2. PersistHold
	out, err := repo.PersistHold(ctx, port.PersistHoldParams{
		EventID:        fix.EventID,
		TicketTypeID:   fix.TicketTypeID,
		IdentityID:     fix.IdentityID,
		HoldID:         holdID,
		Bucket:         targetBucket,
		Quantity:       holdQty,
		UnitCents:      500000,
		ExpiresAt:      expiresAt,
		MaxPerIdentity: 4,
	})
	if err != nil {
		t.Fatalf("PersistHold failed: %v", err)
	}
	if out.OrderID == "" || out.HoldID == "" {
		t.Fatalf("expected non-empty OrderID and HoldID, got %+v", out)
	}

	// 3. Kiểm tra available sau khi hold: phải giảm đúng 3 vé ở bucket 5
	var availAfter int
	err = pool.QueryRow(ctx,
		"SELECT available FROM ticket_inventory WHERE ticket_type_id = $1::uuid AND bucket = $2",
		fix.TicketTypeID, targetBucket).Scan(&availAfter)
	if err != nil {
		t.Fatalf("query inventory after: %v", err)
	}
	if availAfter != availBefore-holdQty {
		t.Fatalf("available after = %d, expected %d", availAfter, availBefore-holdQty)
	}

	// 4. Kiểm tra các bucket khác không bị ảnh hưởng
	var otherBucketAvail int
	err = pool.QueryRow(ctx,
		"SELECT available FROM ticket_inventory WHERE ticket_type_id = $1::uuid AND bucket = 0",
		fix.TicketTypeID).Scan(&otherBucketAvail)
	if err != nil || otherBucketAvail != 10 {
		t.Fatalf("bucket 0 altered: avail=%d, err=%v", otherBucketAvail, err)
	}

	// 5. Kiểm tra Order & SeatHold đã tạo
	var orderStatus string
	var totalCents int64
	err = pool.QueryRow(ctx, "SELECT status, total_cents FROM orders WHERE id = $1::uuid", out.OrderID).Scan(&orderStatus, &totalCents)
	if err != nil || orderStatus != "HELD" || totalCents != 500000*holdQty {
		t.Fatalf("order not created properly: status=%s, total=%d, err=%v", orderStatus, totalCents, err)
	}

	var holdBucket, holdQuantity int
	var releasedAt, consumedAt *time.Time
	err = pool.QueryRow(ctx,
		"SELECT bucket, quantity, released_at, consumed_at FROM seat_holds WHERE order_id = $1::uuid",
		out.OrderID).Scan(&holdBucket, &holdQuantity, &releasedAt, &consumedAt)
	if err != nil || holdBucket != targetBucket || holdQuantity != holdQty || releasedAt != nil || consumedAt != nil {
		t.Fatalf("seat_hold record invalid: err=%v, bucket=%d, qty=%d, releasedAt=%v", err, holdBucket, holdQuantity, releasedAt)
	}
}

func TestPersistHold_InventoryExhausted_CheckConstraint(t *testing.T) {
	repo, pool := setupPostgresTest(t)
	ctx := context.Background()
	fix := seedTestEvent(t, pool, 32) // Mỗi bucket đúng 1 vé

	holdID := domain.NewID()
	expiresAt := time.Now().Add(10 * time.Minute)

	// Bucket 2 chỉ có 1 vé, thử hold 2 vé
	_, err := repo.PersistHold(ctx, port.PersistHoldParams{
		EventID:        fix.EventID,
		TicketTypeID:   fix.TicketTypeID,
		IdentityID:     fix.IdentityID,
		HoldID:         holdID,
		Bucket:         2,
		Quantity:       2,
		UnitCents:      500000,
		ExpiresAt:      expiresAt,
		MaxPerIdentity: 4,
	})
	if !errors.Is(err, domain.ErrInventoryExhausted) {
		t.Fatalf("expected ErrInventoryExhausted, got %v", err)
	}

	// Xác nhận rollback: không có order nào được tạo
	var orderCount int
	_ = pool.QueryRow(ctx, "SELECT count(*) FROM orders WHERE event_id = $1::uuid", fix.EventID).Scan(&orderCount)
	if orderCount != 0 {
		t.Fatalf("expected 0 orders after rollback, got %d", orderCount)
	}
}

func TestPersistHold_BR_O3_DupIdentity(t *testing.T) {
	repo, pool := setupPostgresTest(t)
	ctx := context.Background()
	fix := seedTestEvent(t, pool, 320)

	expiresAt := time.Now().Add(10 * time.Minute)

	// Hold 1 thành công
	_, err := repo.PersistHold(ctx, port.PersistHoldParams{
		EventID:        fix.EventID,
		TicketTypeID:   fix.TicketTypeID,
		IdentityID:     fix.IdentityID,
		HoldID:         domain.NewID(),
		Bucket:         0,
		Quantity:       1,
		UnitCents:      500000,
		ExpiresAt:      expiresAt,
		MaxPerIdentity: 4,
	})
	if err != nil {
		t.Fatalf("first hold failed: %v", err)
	}

	// Hold 2 cùng identity + event khi hold 1 còn HELD -> phải vi phạm uq_active_order_per_identity_event
	_, err = repo.PersistHold(ctx, port.PersistHoldParams{
		EventID:        fix.EventID,
		TicketTypeID:   fix.TicketTypeID,
		IdentityID:     fix.IdentityID,
		HoldID:         domain.NewID(),
		Bucket:         1,
		Quantity:       1,
		UnitCents:      500000,
		ExpiresAt:      expiresAt,
		MaxPerIdentity: 4,
	})
	if !errors.Is(err, domain.ErrPerIdentityLimit) {
		t.Fatalf("expected ErrPerIdentityLimit for duplicate active hold, got %v", err)
	}
}

func TestPersistHold_Outbox_GhiCungTransaction(t *testing.T) {
	repo, pool := setupPostgresTest(t)
	ctx := context.Background()
	fix := seedTestEvent(t, pool, 320)

	holdID := domain.NewID()
	expiresAt := time.Now().Add(10 * time.Minute)

	out, err := repo.PersistHold(ctx, port.PersistHoldParams{
		EventID:        fix.EventID,
		TicketTypeID:   fix.TicketTypeID,
		IdentityID:     fix.IdentityID,
		HoldID:         holdID,
		Bucket:         3,
		Quantity:       2,
		UnitCents:      500000,
		ExpiresAt:      expiresAt,
		MaxPerIdentity: 4,
	})
	if err != nil {
		t.Fatalf("PersistHold failed: %v", err)
	}

	// Kiểm tra bảng outbox_events
	var aggregateType, routingKey string
	var aggregateID string
	var rawPayload []byte
	var publishedAt *time.Time
	err = pool.QueryRow(ctx, `
		SELECT aggregate_type, aggregate_id::text, routing_key, payload, published_at
		  FROM outbox_events
		 WHERE aggregate_id = $1::uuid`, out.OrderID).Scan(&aggregateType, &aggregateID, &routingKey, &rawPayload, &publishedAt)
	if err != nil {
		t.Fatalf("query outbox_events failed: %v", err)
	}

	if aggregateType != "order" || aggregateID != out.OrderID || routingKey != "ticketing.hold.created" {
		t.Fatalf("outbox row invalid: type=%s, aggID=%s, routingKey=%s", aggregateType, aggregateID, routingKey)
	}
	if publishedAt != nil {
		t.Fatalf("outbox event should not be published yet, got %v", publishedAt)
	}

	var payloadMap map[string]any
	if err := json.Unmarshal(rawPayload, &payloadMap); err != nil {
		t.Fatalf("unmarshal outbox payload: %v", err)
	}
	if payloadMap["order_id"] != out.OrderID || payloadMap["event_id"] != fix.EventID || int(payloadMap["quantity"].(float64)) != 2 {
		t.Fatalf("outbox payload content mismatch: %+v", payloadMap)
	}
}

func TestReleaseHold_IdempotentBRO7(t *testing.T) {
	repo, pool := setupPostgresTest(t)
	ctx := context.Background()
	fix := seedTestEvent(t, pool, 320)

	holdID := domain.NewID()
	expiresAt := time.Now().Add(10 * time.Minute)
	const targetBucket = 7
	const holdQty = 4

	// Tạo hold
	out, err := repo.PersistHold(ctx, port.PersistHoldParams{
		EventID:        fix.EventID,
		TicketTypeID:   fix.TicketTypeID,
		IdentityID:     fix.IdentityID,
		HoldID:         holdID,
		Bucket:         targetBucket,
		Quantity:       holdQty,
		UnitCents:      500000,
		ExpiresAt:      expiresAt,
		MaxPerIdentity: 4,
	})
	if err != nil {
		t.Fatalf("PersistHold failed: %v", err)
	}

	// Lần 1: ReleaseHold thành công -> trả về (true, nil)
	released1, err := repo.ReleaseHold(ctx, out.HoldID)
	if err != nil {
		t.Fatalf("first ReleaseHold failed: %v", err)
	}
	if !released1 {
		t.Fatalf("expected released=true on first call, got false")
	}

	// Kiểm tra kho: available phải phục hồi lại đầy đủ 10 vé
	var availAfterFirst int
	_ = pool.QueryRow(ctx,
		"SELECT available FROM ticket_inventory WHERE ticket_type_id = $1::uuid AND bucket = $2",
		fix.TicketTypeID, targetBucket).Scan(&availAfterFirst)
	if availAfterFirst != 10 {
		t.Fatalf("available after first release = %d, expected 10", availAfterFirst)
	}

	// Kiểm tra order status đã là EXPIRED
	var orderStatus string
	_ = pool.QueryRow(ctx, "SELECT status FROM orders WHERE id = $1::uuid", out.OrderID).Scan(&orderStatus)
	if orderStatus != "EXPIRED" {
		t.Fatalf("expected order status EXPIRED, got %s", orderStatus)
	}

	// Lần 2: ReleaseHold gọi lại với cùng holdID -> IDEMPOTENT (BR-O7): trả về (false, nil)
	released2, err := repo.ReleaseHold(ctx, out.HoldID)
	if err != nil {
		t.Fatalf("second ReleaseHold failed: %v", err)
	}
	if released2 {
		t.Fatalf("expected released=false on second call, got true (double release!)")
	}

	// Kho KHÔNG được tăng thêm lần nữa!
	var availAfterSecond int
	_ = pool.QueryRow(ctx,
		"SELECT available FROM ticket_inventory WHERE ticket_type_id = $1::uuid AND bucket = $2",
		fix.TicketTypeID, targetBucket).Scan(&availAfterSecond)
	if availAfterSecond != 10 {
		t.Fatalf("available after second release = %d, expected 10 (kho bi cong 2 lan!)", availAfterSecond)
	}
}

func TestInitInventory_Idempotent(t *testing.T) {
	repo, pool := setupPostgresTest(t)
	ctx := context.Background()

	orgID := uuid.New().String()
	eventID := uuid.New().String()
	ticketTypeID := uuid.New().String()

	const insOrg = `INSERT INTO identities (id, email, password_hash, role) VALUES ($1::uuid, $2, 'pass', 'organizer')`
	_, _ = pool.Exec(ctx, insOrg, orgID, "org-"+orgID+"@test.com")

	const insEvent = `INSERT INTO events (id, organizer_id, slug, title, starts_at, sale_start_at, status)
		VALUES ($1::uuid, $2::uuid, $3, 'Test Init', now(), now(), 'SCHEDULED')`
	_, _ = pool.Exec(ctx, insEvent, eventID, orgID, "slug-"+eventID)

	const insType = `INSERT INTO ticket_types (id, event_id, name, price_cents, quota, bucket_count)
		VALUES ($1::uuid, $2::uuid, 'Standard', 100000, 100, 32)`
	_, _ = pool.Exec(ctx, insType, ticketTypeID, eventID)

	// Lần 1: InitInventory
	err := repo.InitInventory(ctx, ticketTypeID, 100, 32)
	if err != nil {
		t.Fatalf("first InitInventory failed: %v", err)
	}

	var count int
	var sumCap, sumAvail int
	_ = pool.QueryRow(ctx,
		"SELECT count(*), coalesce(sum(capacity), 0), coalesce(sum(available), 0) FROM ticket_inventory WHERE ticket_type_id = $1::uuid",
		ticketTypeID).Scan(&count, &sumCap, &sumAvail)

	if count != 32 || sumCap != 100 || sumAvail != 100 {
		t.Fatalf("inventory distribution mismatch: count=%d, cap=%d, avail=%d", count, sumCap, sumAvail)
	}

	// Lần 2: Gọi lại không lỗi (ON CONFLICT DO NOTHING)
	err = repo.InitInventory(ctx, ticketTypeID, 100, 32)
	if err != nil {
		t.Fatalf("second InitInventory failed: %v", err)
	}
}

func TestGetSaleConfig(t *testing.T) {
	repo, pool := setupPostgresTest(t)
	ctx := context.Background()
	fix := seedTestEvent(t, pool, 100)

	// Event đang ON_SALE -> lấy được config
	cfg, err := repo.GetSaleConfig(ctx, fix.EventID, fix.TicketTypeID)
	if err != nil {
		t.Fatalf("GetSaleConfig failed: %v", err)
	}
	if cfg.PriceCents != 500000 || cfg.BucketCount != 32 || cfg.HoldTTLSeconds != 600 || cfg.MaxPerOrder != 4 || cfg.MaxPerIdentity != 4 {
		t.Fatalf("SaleConfig mismatch: %+v", cfg)
	}

	// Cập nhật event sang DRAFT -> GetSaleConfig phải trả ErrSoldOut
	_, _ = pool.Exec(ctx, "UPDATE events SET status = 'DRAFT' WHERE id = $1::uuid", fix.EventID)
	_, err = repo.GetSaleConfig(ctx, fix.EventID, fix.TicketTypeID)
	if !errors.Is(err, domain.ErrSoldOut) {
		t.Fatalf("expected ErrSoldOut for non-ON_SALE event, got %v", err)
	}
}

// ListOrderHoldIDs tra id hold cua don (dung de don hold Redis sau khi phat ve); don khong ton tai
// hoac id sai dinh dang thi rong, khong loi.
func TestListOrderHoldIDs(t *testing.T) {
	repo, pool := setupPostgresTest(t)
	ctx := context.Background()
	fix := seedTestEvent(t, pool, 320)

	out, err := repo.PersistHold(ctx, port.PersistHoldParams{
		EventID: fix.EventID, TicketTypeID: fix.TicketTypeID, IdentityID: fix.IdentityID,
		HoldID: domain.NewID(), Bucket: 1, Quantity: 1, UnitCents: 1,
		ExpiresAt: time.Now().Add(10 * time.Minute), MaxPerIdentity: 4,
	})
	if err != nil {
		t.Fatal(err)
	}
	ids, err := repo.ListOrderHoldIDs(ctx, out.OrderID)
	if err != nil || len(ids) != 1 || ids[0] != out.HoldID {
		t.Fatalf("ListOrderHoldIDs = %v, %v; muon [%s]", ids, err, out.HoldID)
	}
	for _, bad := range []string{"khong-phai-uuid", "00000000-0000-4000-8000-000000000000"} {
		if got, err := repo.ListOrderHoldIDs(ctx, bad); err != nil || len(got) != 0 {
			t.Errorf("%q: %v, %v; muon rong", bad, got, err)
		}
	}
}

// EV-135, BR-O8: 50 may quet cung luc mot ve -> dung mot UPDATE co dieu kien doi duoc ISSUED -> CHECKED_IN;
// 49 may con lai nhan ErrTicketAlreadyUsed. Ve khac khong bi anh huong.
func TestUpdateTicketStatus_CheckInDungMotLan(t *testing.T) {
	repo, pool := setupPostgresTest(t)
	ctx := context.Background()
	fix := seedTestEvent(t, pool, 320)
	out, err := repo.PersistHold(ctx, port.PersistHoldParams{
		EventID: fix.EventID, TicketTypeID: fix.TicketTypeID, IdentityID: fix.IdentityID,
		HoldID: domain.NewID(), Bucket: 2, Quantity: 2, UnitCents: 1,
		ExpiresAt: time.Now().Add(10 * time.Minute), MaxPerIdentity: 4,
	})
	if err != nil {
		t.Fatal(err)
	}
	ids := []string{uuid.NewString(), uuid.NewString()}
	for i, id := range ids {
		if _, err := pool.Exec(ctx, `INSERT INTO tickets (id, order_id, ticket_type_id, jti, qr_sig, status)
			VALUES ($1::uuid, $2::uuid, $3::uuid, $4, 'sig', 'ISSUED')`, id, out.OrderID, fix.TicketTypeID,
			fmt.Sprintf("jti-%d-%s", i, id)); err != nil {
			t.Fatalf("seed ve: %v", err)
		}
	}

	const scanners = 50
	var wins, used atomic.Int32
	var wg sync.WaitGroup
	now := time.Now().UTC()
	for i := 0; i < scanners; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := repo.UpdateTicketStatus(ctx, ids[0], domain.TicketStatusIssued, domain.TicketStatusCheckedIn, &now)
			switch {
			case err == nil:
				wins.Add(1)
			case errors.Is(err, domain.ErrTicketAlreadyUsed):
				used.Add(1)
			default:
				t.Errorf("loi bat ngo: %v", err)
			}
		}()
	}
	wg.Wait()
	if wins.Load() != 1 || used.Load() != scanners-1 {
		t.Fatalf("thang %d, bi tu choi %d; muon 1 va %d", wins.Load(), used.Load(), scanners-1)
	}

	var st string
	var at *time.Time
	_ = pool.QueryRow(ctx, `SELECT status, checked_in_at FROM tickets WHERE id = $1::uuid`, ids[0]).Scan(&st, &at)
	if st != "CHECKED_IN" || at == nil {
		t.Errorf("ve 1 = %s, checked_in_at %v", st, at)
	}
	_ = pool.QueryRow(ctx, `SELECT status FROM tickets WHERE id = $1::uuid`, ids[1]).Scan(&st)
	if st != "ISSUED" {
		t.Errorf("ve 2 bi anh huong: %s", st)
	}
	if err := repo.UpdateTicketStatus(ctx, uuid.NewString(), domain.TicketStatusIssued, domain.TicketStatusCheckedIn, &now); !errors.Is(err, domain.ErrTicketNotFound) {
		t.Errorf("ve khong ton tai: %v, muon ErrTicketNotFound", err)
	}
}
