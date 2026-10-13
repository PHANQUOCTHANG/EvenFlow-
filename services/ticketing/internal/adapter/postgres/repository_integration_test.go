//go:build integration

package postgresadapter_test

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	"github.com/eventflow/eventflow/libs/go/testx/pgtest"
	postgresadapter "github.com/eventflow/eventflow/services/ticketing/internal/adapter/postgres"
	"github.com/eventflow/eventflow/services/ticketing/internal/domain"
	"github.com/eventflow/eventflow/services/ticketing/internal/port"
)

// Test tich hop Repository ghi outbox trong CUNG transaction nghiep vu (spec AC-1, AC-2, AC-5;
// plan §1.3, §6.4, §7 r3) tren Postgres 16 that (libs/go/testx/pgtest, migrations 0001 + 0002).
// Chay: go test -tags=integration ./internal/adapter/postgres/... ; khong co Docker thi FAIL.
//
// Hop dong:
//   - PersistHold ghi dung 1 row outbox ticketing.hold.created: aggregate_type 'order', aggregate_id = order_id,
//     payload (CHI data) co dung cac khoa order_id, hold_id, sale_event_id, expires_at, quantity — khong co
//     event_id, khong co identity_id/PII. Transaction rollback -> khong co row.
//   - ReleaseHold (lan dau tra true) ghi dung 1 row ticketing.hold.released: aggregate_type 'hold',
//     aggregate_id = hold id dang chuan (co gach, RETURNING h.id::text), data co hold_id (dang chuan) va
//     sale_event_id. Lan goi lai (false) khong ghi them. Loi giua chung -> rollback ca row outbox.

func TestMain(m *testing.M) {
	code := m.Run()
	pgtest.Terminate()
	os.Exit(code)
}

func uuid4() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// hex32 la dang HoldID cua Redis (32 hex, khong gach).
func hex32() string { return strings.ReplaceAll(uuid4(), "-", "") }

type fixture struct {
	pool                              *pgxpool.Pool
	repo                              *postgresadapter.Repository
	eventID, ticketTypeID, identityID string
}

func setup(t *testing.T, available int) fixture {
	t.Helper()
	ctx := context.Background()
	pool := pgtest.New(t)
	f := fixture{pool: pool, repo: postgresadapter.NewRepository(pool)}
	f.identityID = newIdentity(t, pool)
	if err := pool.QueryRow(ctx, `
		INSERT INTO events (organizer_id, slug, title, starts_at, sale_start_at, status)
		VALUES ($1, $2, 'Hoa nhac', now() + interval '7 days', now() - interval '1 hour', 'ON_SALE')
		RETURNING id::text`, f.identityID, "ev-"+hex32()).Scan(&f.eventID); err != nil {
		t.Fatalf("seed event: %v", err)
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO ticket_types (event_id, name, price_cents, quota, bucket_count)
		VALUES ($1, 'GA', 100000, 100, 1) RETURNING id::text`, f.eventID).Scan(&f.ticketTypeID); err != nil {
		t.Fatalf("seed ticket type: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO ticket_inventory (ticket_type_id, bucket, capacity, available) VALUES ($1, 0, 100, $2)`,
		f.ticketTypeID, available); err != nil {
		t.Fatalf("seed inventory: %v", err)
	}
	return f
}

func newIdentity(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	var id string
	if err := pool.QueryRow(context.Background(),
		`INSERT INTO identities (email, password_hash) VALUES ($1, 'x') RETURNING id::text`,
		hex32()+"@test.local").Scan(&id); err != nil {
		t.Fatalf("seed identity: %v", err)
	}
	return id
}

func (f fixture) params(identityID string, qty int) port.PersistHoldParams {
	return port.PersistHoldParams{
		EventID: f.eventID, TicketTypeID: f.ticketTypeID, IdentityID: identityID, HoldID: hex32(),
		Bucket: 0, Quantity: qty, UnitCents: 100000,
		ExpiresAt: time.Now().Add(10 * time.Minute).UTC().Truncate(time.Microsecond),
	}
}

type outboxRow struct {
	EventID, AggregateType, AggregateID string
	Payload                             map[string]any
	Headers                             map[string]string
	TraceID                             string
}

func outboxRows(t *testing.T, pool *pgxpool.Pool, key string) []outboxRow {
	t.Helper()
	rows, err := pool.Query(context.Background(), `
		SELECT event_id::text, aggregate_type, aggregate_id::text, payload, headers, coalesce(trace_id, '')
		  FROM outbox_events WHERE routing_key = $1 ORDER BY id`, key)
	if err != nil {
		t.Fatalf("doc outbox: %v", err)
	}
	defer rows.Close()
	var out []outboxRow
	for rows.Next() {
		var r outboxRow
		var payload, headers []byte
		if err := rows.Scan(&r.EventID, &r.AggregateType, &r.AggregateID, &payload, &headers, &r.TraceID); err != nil {
			t.Fatalf("scan outbox: %v", err)
		}
		if err := json.Unmarshal(payload, &r.Payload); err != nil {
			t.Fatalf("payload: %v", err)
		}
		if err := json.Unmarshal(headers, &r.Headers); err != nil {
			t.Fatalf("headers phai la object chuoi: %v (%s)", err, headers)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func assertKeys(t *testing.T, what string, m map[string]any, want ...string) {
	t.Helper()
	if len(m) != len(want) {
		t.Errorf("%s co khoa %v, muon dung %v", what, keysOf(m), want)
	}
	for _, k := range want {
		if _, ok := m[k]; !ok {
			t.Errorf("%s thieu khoa %s: %v", what, k, m)
		}
	}
	for _, cam := range []string{"event_id", "identity_id"} {
		if _, ok := m[cam]; ok {
			t.Errorf("%s khong duoc co khoa %s (O4 / PII): %v", what, cam, m)
		}
	}
}

func keysOf(m map[string]any) []string {
	var ks []string
	for k := range m {
		ks = append(ks, k)
	}
	return ks
}

func TestTichHop_PersistHoldGhiOutboxHoldCreatedCungTransaction(t *testing.T) {
	f := setup(t, 100)
	p := f.params(f.identityID, 2)
	hv, err := f.repo.PersistHold(context.Background(), p)
	if err != nil {
		t.Fatalf("PersistHold: %v", err)
	}
	rows := outboxRows(t, f.pool, "ticketing.hold.created")
	if len(rows) != 1 {
		t.Fatalf("co %d row ticketing.hold.created, muon 1", len(rows))
	}
	r := rows[0]
	if r.EventID == "" || r.EventID == f.eventID {
		t.Errorf("event_id = %q phai la UUID cua message, khong phai id su kien", r.EventID)
	}
	if r.AggregateType != "order" || r.AggregateID != hv.OrderID {
		t.Errorf("aggregate = %s/%s, muon order/%s", r.AggregateType, r.AggregateID, hv.OrderID)
	}
	assertKeys(t, "data hold.created", r.Payload, "order_id", "hold_id", "sale_event_id", "expires_at", "quantity")
	if r.Payload["order_id"] != hv.OrderID || r.Payload["hold_id"] != hv.HoldID {
		t.Errorf("order_id/hold_id = %v/%v, muon %s/%s", r.Payload["order_id"], r.Payload["hold_id"], hv.OrderID, hv.HoldID)
	}
	if r.Payload["sale_event_id"] != f.eventID {
		t.Errorf("sale_event_id = %v, muon %s", r.Payload["sale_event_id"], f.eventID)
	}
	if r.Payload["quantity"] != float64(2) {
		t.Errorf("quantity = %v", r.Payload["quantity"])
	}
	exp, _ := r.Payload["expires_at"].(string)
	if ts, err := time.Parse(time.RFC3339Nano, exp); err != nil || !ts.Equal(p.ExpiresAt) {
		t.Errorf("expires_at = %q, muon %v", exp, p.ExpiresAt)
	}
	if !strings.Contains(hv.HoldID, "-") {
		t.Errorf("HoldID tra ve phai dang chuan co gach, nhan %q", hv.HoldID)
	}
}

func TestTichHop_PersistHoldRollbackThiKhongCoRowOutbox(t *testing.T) {
	f := setup(t, 100)
	ctx := context.Background()
	if _, err := f.repo.PersistHold(ctx, f.params(f.identityID, 1)); err != nil {
		t.Fatalf("PersistHold lan 1: %v", err)
	}
	// Cung identity + su kien: uq_active_order_per_identity_event chan o buoc tao don -> rollback ca tx.
	_, err := f.repo.PersistHold(ctx, f.params(f.identityID, 1))
	if !errors.Is(err, domain.ErrPerIdentityLimit) {
		t.Fatalf("PersistHold lan 2 = %v, muon ErrPerIdentityLimit", err)
	}
	// Het ton kho: chan ngay buoc tru kho.
	_, err = f.repo.PersistHold(ctx, f.params(newIdentity(t, f.pool), 1000))
	if !errors.Is(err, domain.ErrInventoryExhausted) {
		t.Fatalf("PersistHold qua ton kho = %v, muon ErrInventoryExhausted", err)
	}
	if n := len(outboxRows(t, f.pool, "ticketing.hold.created")); n != 1 {
		t.Errorf("co %d row hold.created, muon 1 (transaction loi khong duoc de lai row)", n)
	}
	var avail int
	if err := f.pool.QueryRow(ctx, `SELECT available FROM ticket_inventory WHERE ticket_type_id = $1`, f.ticketTypeID).Scan(&avail); err != nil {
		t.Fatal(err)
	}
	if avail != 99 {
		t.Errorf("ton kho %d, muon 99 (chi hold dau tien duoc tru)", avail)
	}
}

// AC-5: trace cua request tao hold duoc luu cung row.
func TestTichHop_PersistHoldLuuTraceCuaRequest(t *testing.T) {
	tp := sdktrace.NewTracerProvider()
	prevTP, prevProp := otel.GetTracerProvider(), otel.GetTextMapPropagator()
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.TraceContext{})
	t.Cleanup(func() {
		otel.SetTracerProvider(prevTP)
		otel.SetTextMapPropagator(prevProp)
		_ = tp.Shutdown(context.Background())
	})

	f := setup(t, 100)
	ctx, span := otel.Tracer("test").Start(context.Background(), "POST /holds")
	defer span.End()
	if _, err := f.repo.PersistHold(ctx, f.params(f.identityID, 1)); err != nil {
		t.Fatalf("PersistHold: %v", err)
	}
	r := outboxRows(t, f.pool, "ticketing.hold.created")[0]
	want := span.SpanContext().TraceID().String()
	if r.TraceID != want {
		t.Errorf("trace_id = %q, muon %s", r.TraceID, want)
	}
	if !strings.Contains(r.Headers["traceparent"], want) {
		t.Errorf("headers = %v, muon traceparent cua trace %s", r.Headers, want)
	}
}

func TestTichHop_ReleaseHoldGhiOutboxHoldReleasedMotLan(t *testing.T) {
	f := setup(t, 100)
	ctx := context.Background()
	p := f.params(f.identityID, 2)
	hv, err := f.repo.PersistHold(ctx, p)
	if err != nil {
		t.Fatalf("PersistHold: %v", err)
	}

	// Sweeper truyen HoldID dang Redis (32 hex, khong gach).
	ok, err := f.repo.ReleaseHold(ctx, p.HoldID)
	if err != nil || !ok {
		t.Fatalf("ReleaseHold = (%v, %v), muon (true, nil)", ok, err)
	}
	ok, err = f.repo.ReleaseHold(ctx, p.HoldID)
	if err != nil || ok {
		t.Fatalf("ReleaseHold lan 2 = (%v, %v), muon (false, nil)", ok, err)
	}

	rows := outboxRows(t, f.pool, "ticketing.hold.released")
	if len(rows) != 1 {
		t.Fatalf("co %d row ticketing.hold.released, muon dung 1 (BR-O7)", len(rows))
	}
	r := rows[0]
	if r.AggregateType != "hold" || r.AggregateID != hv.HoldID {
		t.Errorf("aggregate = %s/%s, muon hold/%s (dang chuan co gach)", r.AggregateType, r.AggregateID, hv.HoldID)
	}
	if r.Payload["hold_id"] != hv.HoldID {
		t.Errorf("data.hold_id = %v, muon %s (dang chuan, khong phai 32 hex)", r.Payload["hold_id"], hv.HoldID)
	}
	if r.Payload["sale_event_id"] != f.eventID {
		t.Errorf("data.sale_event_id = %v, muon %s", r.Payload["sale_event_id"], f.eventID)
	}
	// plan §8 m8: released_at lay tu DB (now() cua transaction, cung gia tri voi seat_holds.released_at),
	// khong phai dong ho Go — so khop toi micro giay.
	var dbReleased time.Time
	if err := f.pool.QueryRow(ctx, `SELECT released_at FROM seat_holds WHERE id = $1::uuid`, hv.HoldID).Scan(&dbReleased); err != nil {
		t.Fatalf("doc seat_holds.released_at: %v", err)
	}
	ra, _ := r.Payload["released_at"].(string)
	if got, err := time.Parse(time.RFC3339Nano, ra); err != nil {
		t.Errorf("data.released_at = %q khong phai RFC3339: %v", ra, err)
	} else if !got.Truncate(time.Microsecond).Equal(dbReleased.Truncate(time.Microsecond)) {
		t.Errorf("data.released_at = %v, muon dung seat_holds.released_at %v (thoi gian DB)", got, dbReleased)
	}
	for _, cam := range []string{"event_id", "identity_id"} {
		if _, ok := r.Payload[cam]; ok {
			t.Errorf("data hold.released khong duoc co %s: %v", cam, r.Payload)
		}
	}
	if r.EventID == "" {
		t.Error("row thieu event_id")
	}
}

// Loi giua transaction ReleaseHold (tra kho vi pham inv_available_le_capacity) -> rollback ca row outbox.
func TestTichHop_ReleaseHoldLoiThiKhongCoRowOutbox(t *testing.T) {
	f := setup(t, 100)
	ctx := context.Background()
	p := f.params(f.identityID, 2)
	if _, err := f.repo.PersistHold(ctx, p); err != nil {
		t.Fatalf("PersistHold: %v", err)
	}
	if _, err := f.pool.Exec(ctx, `UPDATE ticket_inventory SET available = capacity WHERE ticket_type_id = $1`, f.ticketTypeID); err != nil {
		t.Fatal(err)
	}
	if ok, err := f.repo.ReleaseHold(ctx, p.HoldID); err == nil {
		t.Fatalf("ReleaseHold = (%v, nil), muon loi tra kho hai lan", ok)
	}
	if n := len(outboxRows(t, f.pool, "ticketing.hold.released")); n != 0 {
		t.Errorf("co %d row hold.released du transaction loi", n)
	}
	var released int
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM seat_holds WHERE released_at IS NOT NULL`).Scan(&released); err != nil {
		t.Fatal(err)
	}
	if released != 0 {
		t.Error("hold bi danh dau released du transaction loi")
	}
}

func TestTichHop_ReleaseHoldDaConsumeKhongGhiOutbox(t *testing.T) {
	f := setup(t, 100)
	ctx := context.Background()
	p := f.params(f.identityID, 1)
	hv, err := f.repo.PersistHold(ctx, p)
	if err != nil {
		t.Fatalf("PersistHold: %v", err)
	}
	if _, err := f.pool.Exec(ctx, `UPDATE seat_holds SET consumed_at = now() WHERE id = $1::uuid`, hv.HoldID); err != nil {
		t.Fatal(err)
	}
	if ok, err := f.repo.ReleaseHold(ctx, p.HoldID); err != nil || ok {
		t.Fatalf("ReleaseHold hold da thanh toan = (%v, %v), muon (false, nil)", ok, err)
	}
	if n := len(outboxRows(t, f.pool, "ticketing.hold.released")); n != 0 {
		t.Errorf("hold khong duoc nha ma van co %d row hold.released", n)
	}
}
