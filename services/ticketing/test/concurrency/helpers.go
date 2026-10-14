//go:build integration

package concurrency

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/eventflow/eventflow/libs/go/testx"
	postgresadapter "github.com/eventflow/eventflow/services/ticketing/internal/adapter/postgres"
	redisadapter "github.com/eventflow/eventflow/services/ticketing/internal/adapter/redis"
	"github.com/eventflow/eventflow/services/ticketing/internal/port"
	"github.com/eventflow/eventflow/services/ticketing/internal/worker"
)

// identityCount la so khach nap san vao Postgres. Test dung SeedIdentity(n) voi n trong
// [0, identityCount): test 1 dung 10.000 goroutine, test 2 dung 5 vong x 2.000.
const identityCount = 10_000

// bucketCount khop ticket_types.bucket_count mac dinh cua migration va deploy/scripts/seed.sh.
const bucketCount = 32

// sweeperInterval ngan hon nhieu so voi production (1 giay) de dot quet cuoi cung ket thuc truoc
// phep kiem cuoi cua test het han: hai truy van TotalAvailable/TotalHeldOrSold khong cung
// snapshot, nen sweeper con chay giua chung se lam tong lech gia.
const sweeperInterval = 100 * time.Millisecond

// TestEnv la moi truong tich hop dung Postgres + Redis THAT qua testcontainers.
//
// Khong dung mock o day: bat bien chong oversell nam trong constraint cua
// Postgres va tinh atomic cua Lua script. Mock hai thu do di thi test chi con
// kiem tra chinh mock, va bug that se lot thang ra production.
//
// Repo va Gate la adapter THAT, dung cung cau hinh pool voi cmd/server/main.go. Moi test lay
// mot su kien moi (event_id ngau nhien) tren cung mot cap container, nen khong can truncate
// giua cac lan chay.
type TestEnv struct {
	Repo   port.Repository
	Gate   port.InventoryGate
	Logger *slog.Logger

	pool  *pgxpool.Pool
	admin *redis.Client // client rieng, timeout rong, de nap/kiem tra -- khong phai Gate
	repo  *postgresadapter.Repository
	gate  *redisadapter.Gate
}

var (
	identitiesOnce sync.Once
	identitiesErr  error
)

// setupEnv dung ha tang test va don dep sau khi xong.
//
// Container Postgres/Redis duoc dung chung cho ca tien trinh (xem libs/go/testx); moi lan goi
// chi tao pool/client moi va dong chung trong t.Cleanup.
func setupEnv(t *testing.T) *TestEnv {
	t.Helper()
	ctx := context.Background()

	dsn := testx.Postgres(t)
	addr := testx.Redis(t)

	pcfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatalf("doc DSN postgres: %v", err)
	}
	// Giong cmd/server/main.go. Pool nho la co y: nut co chai that su la Postgres, va test phai
	// chiu dung dieu kien do. Neu nong pool de test "xanh" thi bat bien khong con duoc do that.
	pcfg.MaxConns = 25
	pcfg.MinConns = 5
	pool, err := pgxpool.NewWithConfig(ctx, pcfg)
	if err != nil {
		t.Fatalf("mo pool postgres: %v", err)
	}
	t.Cleanup(pool.Close)

	// Giong cmd/server/main.go, ke ca timeout 200ms: mot request bi timeout duoi tai la ket qua
	// that, va bat bien 4 (khong loi he thong) la cam ket nghiep vu, khong phai nhieu can giau.
	rdb := redis.NewClient(&redis.Options{
		Addr:         addr,
		PoolSize:     256,
		MinIdleConns: 32,
		ReadTimeout:  200 * time.Millisecond,
		WriteTimeout: 200 * time.Millisecond,
	})
	t.Cleanup(func() { _ = rdb.Close() })

	admin := redis.NewClient(&redis.Options{Addr: addr})
	t.Cleanup(func() { _ = admin.Close() })

	// NewGate nap script bang chinh client co timeout 200ms. Ngay sau mot lan chay nang (don key
	// cua lan truoc) Redis co the cham hon the vai nhip -- do la nhieu cua khoi dong, khong phai
	// hanh vi dang do, nen thu lai vai lan thay vi de test do o buoc dung ha tang.
	var gate *redisadapter.Gate
	for attempt := 1; ; attempt++ {
		gate, err = redisadapter.NewGate(ctx, rdb)
		if err == nil {
			break
		}
		if attempt == 5 {
			t.Fatalf("khoi tao redis gate (sau %d lan thu): %v", attempt, err)
		}
		time.Sleep(200 * time.Millisecond)
	}
	repo := postgresadapter.NewRepository(pool)

	identitiesOnce.Do(func() { identitiesErr = seedIdentities(ctx, pool) })
	if identitiesErr != nil {
		t.Fatalf("nap identity: %v", identitiesErr)
	}

	return &TestEnv{
		Repo:  repo,
		Gate:  gate,
		pool:  pool,
		admin: admin,
		repo:  repo,
		gate:  gate,
		// Chi Warn tro len: sweeper log Info moi 100ms. Ghi ra stderr chu khong qua t.Log vi
		// sweeper con chay sau khi test ket thuc, va t.Log luc do la panic.
		Logger: slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn})),
	}
}

// organizerID la nguoi to chuc cua moi su kien test.
const organizerID = "00000000-0000-4000-8000-0000000000a0"

// seedIdentities nap san identityCount khach mot lan. Phai lam truoc khi goroutine chay:
// SeedIdentity va AdmitToken duoc goi trong 10.000 goroutine, noi t.Fatal khong duoc phep
// (FailNow phai chay o goroutine cua test).
func seedIdentities(ctx context.Context, pool *pgxpool.Pool) error {
	const q = `
		INSERT INTO identities (id, email, password_hash, role, otp_verified_at)
		SELECT ('00000000-0000-4000-8000-' || lpad(n::text, 12, '0'))::uuid,
		       'load' || n || '@eventflow.test', '!', 'buyer', now()
		  FROM generate_series(0, $1::int - 1) AS n
		ON CONFLICT DO NOTHING`
	if _, err := pool.Exec(ctx, q, identityCount); err != nil {
		return fmt.Errorf("nap buyer: %w", err)
	}
	const org = `
		INSERT INTO identities (id, email, password_hash, role, otp_verified_at)
		VALUES ($1::uuid, 'organizer@eventflow.test', '!', 'organizer', now())
		ON CONFLICT DO NOTHING`
	if _, err := pool.Exec(ctx, org, organizerID); err != nil {
		return fmt.Errorf("nap organizer: %w", err)
	}
	return nil
}

// SeedEvent tao mot su kien dang ON_SALE voi quota cho truoc, da chia bucket
// va da nap ton kho tuong ung vao Redis.
//
// Dong thoi nap san token admit cho moi khach trong [0, identityCount), nen AdmitToken khong
// can I/O.
func (e *TestEnv) SeedEvent(t *testing.T, quota int) (eventID, ticketTypeID string) {
	t.Helper()
	ctx := context.Background()

	eventID = newUUID(t)
	ticketTypeID = newUUID(t)

	const insEvent = `
		INSERT INTO events (id, organizer_id, slug, title, starts_at, sale_start_at, status)
		VALUES ($1::uuid, $2::uuid, $3, 'Oversell gate', now() + interval '30 days', now(), 'ON_SALE')`
	if _, err := e.pool.Exec(ctx, insEvent, eventID, organizerID, "oversell-"+eventID); err != nil {
		t.Fatalf("tao su kien: %v", err)
	}

	const insType = `
		INSERT INTO ticket_types (id, event_id, name, price_cents, quota, bucket_count)
		VALUES ($1::uuid, $2::uuid, 'Standard', 50000000, $3, $4)`
	if _, err := e.pool.Exec(ctx, insType, ticketTypeID, eventID, quota, bucketCount); err != nil {
		t.Fatalf("tao hang ve: %v", err)
	}

	// Cong thuc chia bucket giong deploy/scripts/seed.sh: chia deu, phan du don vao bucket dau.
	const insInv = `
		INSERT INTO ticket_inventory (ticket_type_id, bucket, capacity, available)
		SELECT $1::uuid, b,
		       $2::int / $3::int + CASE WHEN b < $2::int % $3::int THEN 1 ELSE 0 END,
		       $2::int / $3::int + CASE WHEN b < $2::int % $3::int THEN 1 ELSE 0 END
		  FROM generate_series(0, $3::int - 1) AS b`
	if _, err := e.pool.Exec(ctx, insInv, ticketTypeID, quota, bucketCount); err != nil {
		t.Fatalf("nap ton kho postgres: %v", err)
	}

	// Nap Redis tu chinh Postgres, qua cung duong ma worker production dung.
	rows, err := e.repo.ListInventory(ctx, eventID)
	if err != nil {
		t.Fatalf("doc ton kho: %v", err)
	}
	perBucket := make(map[int]int, len(rows))
	for _, r := range rows {
		perBucket[r.Bucket] = r.Available
	}
	if err := e.gate.SeedInventory(ctx, eventID, ticketTypeID, perBucket); err != nil {
		t.Fatalf("nap ton kho redis: %v", err)
	}

	// Token admit: ZSET `wr:{ev}:admitted`, member = token, score = han (ms). Mot lenh ZADD voi
	// nhieu member de khong ton 10.000 round-trip.
	expiry := float64(time.Now().Add(time.Hour).UnixMilli())
	members := make([]redis.Z, identityCount)
	for n := range members {
		members[n] = redis.Z{Score: expiry, Member: tokenOf(identityID(n))}
	}
	admitted := fmt.Sprintf("wr:{%s}:admitted", eventID)
	if err := e.admin.ZAdd(ctx, admitted, members...).Err(); err != nil {
		t.Fatalf("nap token admit: %v", err)
	}

	// Container Redis song suot tien trinh: don key cua su kien nay de 200 lan chay khong de lai
	// hang trieu member.
	t.Cleanup(func() { e.deleteEventKeys(eventID) })

	return eventID, ticketTypeID
}

func (e *TestEnv) deleteEventKeys(eventID string) {
	ctx := context.Background()
	iter := e.admin.Scan(ctx, 0, "*{"+eventID+"}*", 100).Iterator()
	for iter.Next(ctx) {
		_ = e.admin.Del(ctx, iter.Val()).Err()
	}
}

// SeedIdentity tra ve id cua khach thu n. Khong I/O: khach da duoc nap san trong setupEnv.
//
// Panic khi n ngoai [0, identityCount): ham nay chay trong goroutine, noi t.Fatal khong duoc
// phep, va mot id ngoai khoang se chi hien ra sau do thanh loi khoa ngoai kho doc.
func (e *TestEnv) SeedIdentity(_ *testing.T, n int) string {
	if n < 0 || n >= identityCount {
		panic(fmt.Sprintf("SeedIdentity(%d): ngoai khoang [0, %d)", n, identityCount))
	}
	return identityID(n)
}

// AdmitToken cap mot queue token da o trang thai ADMITTED, de test tap trung
// vao ton kho thay vi vao phong cho.
//
// Token da duoc nap san o SeedEvent nen ham nay khong I/O; no chi tinh ra dung token do.
func (e *TestEnv) AdmitToken(_ *testing.T, _, identityID string) string {
	return tokenOf(identityID)
}

// TotalAvailable cong ton kho con lai tren toan bo 32 bucket trong Postgres.
func (e *TestEnv) TotalAvailable(t *testing.T, ticketTypeID string) int {
	t.Helper()
	return e.queryInt(t, `SELECT COALESCE(SUM(available), 0)::int FROM ticket_inventory WHERE ticket_type_id = $1::uuid`,
		ticketTypeID)
}

// TotalHeldOrSold dem so ve dang bi giu hoac da ban.
func (e *TestEnv) TotalHeldOrSold(t *testing.T, ticketTypeID string) int {
	t.Helper()
	// released_at IS NULL gom ca hold dang giu lan hold da consume (da ban).
	return e.queryInt(t, `SELECT COALESCE(SUM(quantity), 0)::int FROM seat_holds
		WHERE ticket_type_id = $1::uuid AND released_at IS NULL`, ticketTypeID)
}

// InventoryDrift do do lech giua Redis va Postgres. Phai luon bang 0 khi he
// thong o trang thai nghi.
//
// Co dau: duong nghia la Redis dang PHONG so voi Postgres (nguy co oversell neu Postgres
// khong chan), am la Redis bao thieu (ban thieu tam thoi).
func (e *TestEnv) InventoryDrift(t *testing.T, eventID, ticketTypeID string) int {
	t.Helper()
	ctx := context.Background()

	vals, err := e.admin.HVals(ctx, fmt.Sprintf("inv:{%s}:tt:%s", eventID, ticketTypeID)).Result()
	if err != nil {
		t.Fatalf("doc ton kho redis: %v", err)
	}
	var redisTotal int
	for _, v := range vals {
		n, err := strconv.Atoi(v)
		if err != nil {
			t.Fatalf("gia tri ton kho redis khong phai so: %q", v)
		}
		redisTotal += n
	}
	return redisTotal - e.TotalAvailable(t, ticketTypeID)
}

func (e *TestEnv) SetHoldTTL(t *testing.T, eventID string, ttl time.Duration) {
	t.Helper()
	// GetSaleConfig doc lai hold_ttl_seconds o moi request nen doi la co hieu luc ngay.
	secs := int(ttl / time.Second)
	if secs < 1 {
		t.Fatalf("SetHoldTTL: ttl %v duoi 1 giay (cot luu theo giay)", ttl)
	}
	if _, err := e.pool.Exec(context.Background(),
		`UPDATE events SET hold_ttl_seconds = $2, updated_at = now() WHERE id = $1::uuid`,
		eventID, secs); err != nil {
		t.Fatalf("doi hold TTL: %v", err)
	}
}

// RunSweeper chay HoldSweeper THAT cho toi khi ctx bi huy.
//
// Pool/client chi dong sau khi test ket thuc (t.Cleanup), luc ctx cua test da huy; pgxpool.Close
// cho cac truy van dang chay xong nen khong cat ngang mot lan tra kho.
func (e *TestEnv) RunSweeper(ctx context.Context, eventID string) {
	cfg := worker.SweeperConfig{
		EventIDs: []string{eventID},
		Interval: sweeperInterval,
		Batch:    500,
	}
	_ = worker.NewHoldSweeper(e.gate, e.repo, noopPublisher{}, cfg, e.Logger).Run(ctx)
}

// noopPublisher bo qua su kien ticketing.hold.released: bat bien dang do la ton kho, khong phai
// viec phat su kien (outbox la EVF-36).
type noopPublisher struct{}

func (noopPublisher) Publish(context.Context, string, any) error { return nil }

var _ port.Publisher = noopPublisher{}

func (e *TestEnv) queryInt(t *testing.T, sql string, args ...any) int {
	t.Helper()
	var n int
	if err := e.pool.QueryRow(context.Background(), sql, args...).Scan(&n); err != nil {
		t.Fatalf("truy van %q: %v", sql, err)
	}
	return n
}

// identityID la UUID xac dinh cua khach thu n, khop voi seedIdentities.
func identityID(n int) string {
	return fmt.Sprintf("00000000-0000-4000-8000-%012d", n)
}

func tokenOf(identityID string) string { return "tok-" + identityID }

func newUUID(t *testing.T) string {
	t.Helper()
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		t.Fatalf("sinh uuid: %v", err)
	}
	b[6] = b[6]&0x0f | 0x40 // version 4
	b[8] = b[8]&0x3f | 0x80 // variant RFC 4122
	h := hex.EncodeToString(b[:])
	return h[0:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:32]
}
