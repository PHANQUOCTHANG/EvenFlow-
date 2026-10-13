package outbox

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/eventflow/eventflow/libs/go/mq"
)

// Unit test cho relay (spec AC-2, AC-3, AC-4, AC-6, AC-7; plan §6.2–§6.4, §7) voi fake DB/Publisher/Dialer.
//
// TEN EXPORT ma test gia dinh (plan §1.2 + §6.3 + §7):
//
//	// DB la phan cua *pgxpool.Pool ma relay dung (claim trong tx ngan, danh dau autocommit, dem dinh ky).
//	type DB interface {
//		Begin(ctx context.Context) (pgx.Tx, error)
//		Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
//		Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
//		QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
//	}
//	// Publisher: *mq.Publisher thoa man.
//	type Publisher interface {
//		Publish(ctx context.Context, exchange, routingKey string, msg mq.Message) error
//	}
//	// Dialer mo ket noi moi toi broker (dial, kenh, Declare topology, mq.NewPublisher); Closer dong ket noi.
//	type Dialer func(ctx context.Context) (Publisher, io.Closer, error)
//	type RelayConfig struct {
//		Exchange       string                // bat buoc
//		BatchSize      int                   // 0 -> 100; 1..1000
//		Interval       time.Duration         // 0 -> 500ms
//		Lease          time.Duration         // 0 -> 60s
//		BatchDeadline  time.Duration         // 0 -> 30s
//		PublishTimeout time.Duration         // 0 -> 10s; PublishTimeout <= BatchDeadline va
//		                                     // BatchDeadline + PublishTimeout + 2*5s (markTimeout) < Lease (plan §8 m1)
//		BaseBackoff    time.Duration         // 0 -> 1s
//		MaxBackoff     time.Duration         // 0 -> 5m; 0 < Base <= Max <= 1h
//		MaxAttempts    int                   // 0 -> 20; < 0 loi
//		DropUnroutable []string              // routing key duoc phep bo khi unroutable
//		Registerer     prometheus.Registerer // bat buoc
//	}
//	func NewRelay(db DB, dial Dialer, cfg RelayConfig) (*Relay, error)
//	// Run: vong lap toi khi ctx huy -> tra nil. Dial loi / ErrClosed -> dong, lui (1s..30s, jitter), dial lai;
//	// KHONG BAO GIO tra loi vi broker. Chi claim khi dang ket noi.
//	func (r *Relay) Run(ctx context.Context) error
//	// RunOnce: chua ket noi thi dial (loi -> tra loi, khong claim); claim mot lo, publish, danh dau.
//	// ErrClosed -> dong ket noi (Closer.Close), tra lease cac row chua xu ly, tra loi boc mq.ErrClosed.
//	// ctx huy -> danh dau phan da gui, tra lease phan con lai, tra loi boc context.Canceled.
//	func (r *Relay) RunOnce(ctx context.Context) (published int, err error)
//
// Metric (ten Prometheus): outbox_relay_connected (gauge 0/1), outbox_unroutable_total{routing_key},
// outbox_published_total{routing_key}.
//
// Hop dong SQL cua cac cau danh dau: xem fakes_test.go.

// --- NewRelay ---

func TestNewRelay_KiemTraCauHinh(t *testing.T) {
	db := newFakeDB(t)
	d := newFakeDialer()
	cases := []struct {
		ten string
		sua func(*RelayConfig)
		loi bool
	}{
		{"cau hinh test hop le", func(*RelayConfig) {}, false},
		{"toi thieu chi Exchange va Registerer, con lai mac dinh", func(c *RelayConfig) {
			*c = RelayConfig{Exchange: "x", Registerer: prometheus.NewRegistry()}
		}, false},
		{"thieu Exchange", func(c *RelayConfig) { c.Exchange = "" }, true},
		{"thieu Registerer", func(c *RelayConfig) { c.Registerer = nil }, true},
		{"BatchSize 1000 duoc", func(c *RelayConfig) { c.BatchSize = 1000 }, false},
		{"BatchSize 1001", func(c *RelayConfig) { c.BatchSize = 1001 }, true},
		{"BatchSize am", func(c *RelayConfig) { c.BatchSize = -1 }, true},
		{"PublishTimeout bang BatchDeadline duoc", func(c *RelayConfig) { c.PublishTimeout = c.BatchDeadline }, false},
		{"PublishTimeout lon hon BatchDeadline", func(c *RelayConfig) { c.PublishTimeout = c.BatchDeadline + time.Millisecond }, true},
		{"BatchDeadline bang Lease", func(c *RelayConfig) { c.BatchDeadline = c.Lease }, true},
		{"BatchDeadline lon hon Lease", func(c *RelayConfig) { c.BatchDeadline = c.Lease + time.Second }, true},
		{"BaseBackoff lon hon MaxBackoff", func(c *RelayConfig) { c.BaseBackoff = c.MaxBackoff + time.Millisecond }, true},
		{"BaseBackoff bang MaxBackoff duoc", func(c *RelayConfig) { c.BaseBackoff = c.MaxBackoff }, false},
		{"BaseBackoff am", func(c *RelayConfig) { c.BaseBackoff = -time.Second }, true},
		{"MaxBackoff 1h duoc", func(c *RelayConfig) { c.MaxBackoff = time.Hour }, false},
		{"MaxBackoff qua 1h", func(c *RelayConfig) { c.MaxBackoff = time.Hour + time.Nanosecond }, true},
		{"MaxAttempts am", func(c *RelayConfig) { c.MaxAttempts = -1 }, true},
		// plan §8 m1: lease phai bao ca lo, mot publish va hai cau danh dau (2 x 5s).
		{"Lease bang BatchDeadline + PublishTimeout + 10s", func(c *RelayConfig) {
			c.Lease = c.BatchDeadline + c.PublishTimeout + 10*time.Second
		}, true},
		{"Lease lon hon BatchDeadline + PublishTimeout + 10s duoc", func(c *RelayConfig) {
			c.Lease = c.BatchDeadline + c.PublishTimeout + 10*time.Second + time.Millisecond
		}, false},
		{"Lease 5s voi lo 1s (cu hop le, nay thieu cho danh dau)", func(c *RelayConfig) { c.Lease = 5 * time.Second }, true},
		{"chi dat Lease 45s, con lai mac dinh 30s + 10s + 10s", func(c *RelayConfig) {
			*c = RelayConfig{Exchange: "x", Registerer: prometheus.NewRegistry(), Lease: 45 * time.Second}
		}, true},
	}
	for _, tc := range cases {
		t.Run(tc.ten, func(t *testing.T) {
			cfg := testConfig()
			tc.sua(&cfg)
			r, err := NewRelay(db, d.dial, cfg)
			if tc.loi && err == nil {
				t.Fatalf("muon loi, nhan Relay %v", r)
			}
			if !tc.loi && err != nil {
				t.Fatalf("muon hop le, nhan loi %v", err)
			}
		})
	}
}

func TestNewRelay_ThieuDBHoacDialerLaLoi(t *testing.T) {
	d := newFakeDialer()
	if _, err := NewRelay(nil, d.dial, testConfig()); err == nil {
		t.Error("db nil phai loi")
	}
	if _, err := NewRelay(newFakeDB(t), nil, testConfig()); err == nil {
		t.Error("dialer nil phai loi")
	}
}

// Cau hinh mac dinh phai thoa bat bien thoi gian cua lease (PublishTimeout <= BatchDeadline < Lease).
func TestNewRelay_MacDinhThoaBatBienLease(t *testing.T) {
	cfg := RelayConfig{Exchange: "x", Registerer: prometheus.NewRegistry(), Lease: 61 * time.Second}
	if _, err := NewRelay(newFakeDB(t), newFakeDialer().dial, cfg); err != nil {
		t.Fatalf("chi dat Lease, phan con lai mac dinh: %v", err)
	}
}

// --- RunOnce: thanh cong, phong bi, trace ---

func TestRunOnce_ThanhCongPublishPhongBiVaDanhDau(t *testing.T) {
	newRecorder(t)
	traceHex := "4bf92f3577b34da6a3ce929d0e0e4736"
	traceparent := "00-" + traceHex + "-00f067aa0ba902b7-01"
	moi := mkRow(1, "ticketing.hold.released")
	moi.Headers = `{"traceparent":"` + traceparent + `"}`
	moi.TraceID = &traceHex
	// Row kieu cu (ghi truoc 0002): headers '{}', trace_id NULL, data con khoa event_id = id buoi dien.
	cu := mkRow(2, "ticketing.hold.created")
	cu.Payload = `{"order_id":"22222222-2222-4222-8222-222222222222","event_id":"33333333-3333-4333-8333-333333333333","quantity":2}`

	db := newFakeDB(t, moi, cu)
	d := newFakeDialer()
	cfg := testConfig()
	r := newTestRelay(t, db, d, cfg)

	n, err := r.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if n != 2 {
		t.Errorf("published = %d, muon 2", n)
	}
	calls := d.pub.snapshot()
	if len(calls) != 2 {
		t.Fatalf("Publish goi %d lan, muon 2", len(calls))
	}
	for _, c := range calls {
		row := moi
		if c.msg.MessageID == cu.EventID {
			row = cu
		} else if c.msg.MessageID != moi.EventID {
			t.Fatalf("MessageID = %q, muon event_id cua row", c.msg.MessageID)
		}
		if c.exchange != cfg.Exchange || c.key != row.RoutingKey {
			t.Errorf("publish toi %s/%s, muon %s/%s", c.exchange, c.key, cfg.Exchange, row.RoutingKey)
		}
		if c.msg.ContentType != "application/json" {
			t.Errorf("ContentType = %q", c.msg.ContentType)
		}
		if c.msg.Type != row.RoutingKey {
			t.Errorf("Type = %q, muon routing key", c.msg.Type)
		}
		if !c.msg.Timestamp.Equal(row.CreatedAt) {
			t.Errorf("Timestamp = %v, muon created_at %v", c.msg.Timestamp, row.CreatedAt)
		}
		for k, v := range c.msg.Headers {
			if _, ok := v.(string); !ok {
				t.Errorf("header %s co kieu %T, chi duoc la string", k, v)
			}
		}

		var env Envelope
		if err := json.Unmarshal(c.msg.Body, &env); err != nil {
			t.Fatalf("body khong phai Envelope: %v (%s)", err, c.msg.Body)
		}
		if env.EventID != row.EventID {
			t.Errorf("envelope event_id = %q, muon cot event_id %q", env.EventID, row.EventID)
		}
		if !env.OccurredAt.Equal(row.CreatedAt) {
			t.Errorf("occurred_at = %v, muon created_at %v", env.OccurredAt, row.CreatedAt)
		}
		if env.SchemaVersion != SchemaVersion || SchemaVersion != 1 {
			t.Errorf("schema_version = %d (hang %d), muon 1", env.SchemaVersion, SchemaVersion)
		}
		if env.Type != row.RoutingKey {
			t.Errorf("type = %q", env.Type)
		}
		if !jsonEqual(t, env.Data, []byte(row.Payload)) {
			t.Errorf("data = %s, muon dung payload %s", env.Data, row.Payload)
		}
		raw := decodeJSON(t, c.msg.Body)
		for k := range raw {
			switch k {
			case "event_id", "occurred_at", "trace_id", "schema_version", "type", "data":
			default:
				t.Errorf("phong bi co khoa la %q", k)
			}
		}
		if row.TraceID != nil {
			if env.TraceID != traceHex {
				t.Errorf("trace_id = %q, muon %q", env.TraceID, traceHex)
			}
			if c.msg.Headers["traceparent"] != traceparent {
				t.Errorf("header traceparent = %v, muon %q", c.msg.Headers["traceparent"], traceparent)
			}
			// AC-5: ctx publish duoc khoi phuc tu headers cua row.
			if c.traceID.String() != traceHex {
				t.Errorf("trace ID trong ctx publish = %s, muon %s", c.traceID, traceHex)
			}
		} else {
			if env.TraceID != "" {
				t.Errorf("row cu khong co trace_id, envelope trace_id = %q", env.TraceID)
			}
			if c.traceID.IsValid() {
				t.Errorf("row cu khong co trace, ctx publish lai co trace %s", c.traceID)
			}
		}
	}

	for _, row := range []fakeRow{moi, cu} {
		st := db.state(row.ID)
		if !st.published || st.unroutable || st.retries != 0 || st.parked {
			t.Errorf("row %d: trang thai %+v, muon da gui sach", row.ID, st)
		}
	}
	assertMarksGuarded(t, db)

	var claim *stmt
	for _, s := range db.all() {
		if s.kind == kindClaim {
			s := s
			claim = &s
		}
	}
	if claim == nil {
		t.Fatal("khong co cau claim")
	}
	if !strings.Contains(claim.sql, "order by next_attempt_at,id") {
		t.Errorf("claim phai ORDER BY next_attempt_at, id (khop idx_outbox_due): %s", claim.sql)
	}
	if !strings.Contains(claim.sql, "published_at is null") {
		t.Errorf("claim phai loc published_at IS NULL: %s", claim.sql)
	}

	g := gatherer(t, cfg)
	if v, _ := metricValue(t, g, "outbox_relay_connected", nil); v != 1 {
		t.Errorf("outbox_relay_connected = %v, muon 1 sau khi dial thanh cong", v)
	}
	if v, ok := metricValue(t, g, "outbox_published_total", nil); !ok || v != 2 {
		t.Errorf("outbox_published_total = %v (co %v), muon 2", v, ok)
	}
	if d.dialCount() != 1 {
		t.Errorf("dial %d lan, muon 1", d.dialCount())
	}
	// Lan thu hai dung lai ket noi, khong dial lai, khong publish lai.
	if n, err := r.RunOnce(context.Background()); err != nil || n != 0 {
		t.Errorf("RunOnce lan 2 = (%d, %v), muon (0, nil)", n, err)
	}
	if d.dialCount() != 1 || len(d.pub.snapshot()) != 2 {
		t.Errorf("lan 2: dial %d, publish %d — muon 1, 2", d.dialCount(), len(d.pub.snapshot()))
	}
}

// --- RunOnce: loi tam tang attempts (R1) ---

func TestRunOnce_ErrNackedTangAttemptsGhiLastError(t *testing.T) {
	a, b := mkRow(1, "ticketing.hold.released"), mkRow(2, "ticketing.hold.released")
	db := newFakeDB(t, a, b)
	d := newFakeDialer()
	d.pub.on[a.EventID] = func(context.Context) error { return fmt.Errorf("publish: %w", mq.ErrNacked) }
	r := newTestRelay(t, db, d, testConfig())

	if _, err := r.RunOnce(context.Background()); err != nil {
		t.Fatalf("nack la loi cua mot row, RunOnce khong duoc tra loi: %v", err)
	}
	sa := db.state(a.ID)
	if sa.retries != 1 || sa.published || sa.parked {
		t.Errorf("row nack: %+v, muon attempts+1, chua gui, khong do", sa)
	}
	if !contains(sa.errs, "nack") {
		t.Errorf("last_error phai chua loi nack, nhan %q", sa.errs)
	}
	if sb := db.state(b.ID); !sb.published {
		t.Errorf("row khac khong duoc bi chan: %+v", sb)
	}
	if d.closer.count() != 0 {
		t.Error("nack khong duoc dong ket noi")
	}
	assertMarksGuarded(t, db)
}

func TestRunOnce_HetPublishTimeoutTangAttemptsKhiKenhConMo(t *testing.T) {
	a, b := mkRow(1, "ticketing.hold.released"), mkRow(2, "ticketing.hold.released")
	db := newFakeDB(t, a, b)
	d := newFakeDialer()
	d.pub.on[a.EventID] = func(ctx context.Context) error {
		<-ctx.Done() // confirm khong bao gio toi
		return fmt.Errorf("mq: cho confirm: %w", ctx.Err())
	}
	cfg := testConfig()
	cfg.PublishTimeout = 50 * time.Millisecond
	r := newTestRelay(t, db, d, cfg)

	start := time.Now()
	if _, err := r.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if el := time.Since(start); el > 900*time.Millisecond {
		t.Errorf("RunOnce mat %v; PublishTimeout 50ms khong co hieu luc", el)
	}
	if sa := db.state(a.ID); sa.retries != 1 || sa.published || sa.releases != 0 {
		t.Errorf("row het PublishTimeout: %+v, muon attempts+1 (khong tra lease)", sa)
	}
	if sb := db.state(b.ID); !sb.published {
		t.Errorf("row sau row timeout van phai duoc gui: %+v", sb)
	}
	assertMarksGuarded(t, db)
}

// --- AC-6: unroutable ---

func TestRunOnce_UnroutableTrongDanhSachDuocDanhDauBo(t *testing.T) {
	a := mkRow(1, "ticketing.hold.created")
	db := newFakeDB(t, a)
	d := newFakeDialer()
	d.pub.on[a.EventID] = func(context.Context) error { return fmt.Errorf("x: %w", mq.ErrUnroutable) }
	cfg := testConfig()
	r := newTestRelay(t, db, d, cfg)

	if _, err := r.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	st := db.state(a.ID)
	if !st.published || !st.unroutable || st.retries != 0 {
		t.Errorf("row unroutable trong DropUnroutable: %+v, muon published_at dat + last_error 'unroutable'", st)
	}
	v, ok := metricValue(t, gatherer(t, cfg), "outbox_unroutable_total", map[string]string{"routing_key": a.RoutingKey})
	if !ok || v != 1 {
		t.Errorf("outbox_unroutable_total{routing_key=%s} = %v (co %v), muon 1", a.RoutingKey, v, ok)
	}
	assertMarksGuarded(t, db)
}

func TestRunOnce_UnroutableNgoaiDanhSachThuLai(t *testing.T) {
	a := mkRow(1, "ticketing.hold.released")
	db := newFakeDB(t, a)
	d := newFakeDialer()
	d.pub.on[a.EventID] = func(context.Context) error { return fmt.Errorf("x: %w", mq.ErrUnroutable) }
	r := newTestRelay(t, db, d, testConfig())

	if _, err := r.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	st := db.state(a.ID)
	if st.published {
		t.Fatalf("key ngoai DropUnroutable bi danh dau da gui = mat event: %+v", st)
	}
	if st.retries != 1 {
		t.Errorf("unroutable ngoai danh sach phai la loi tam (attempts+1), nhan %+v", st)
	}
	assertMarksGuarded(t, db)
}

// --- R1: ErrClosed / ctx huy khong tang attempts, tra lease ---

func TestRunOnce_ErrClosedTraLeaseKhongTangAttemptsRoiDialLai(t *testing.T) {
	rows := []fakeRow{
		mkRow(1, "ticketing.hold.released"), mkRow(2, "ticketing.hold.released"), mkRow(3, "ticketing.hold.released"),
		mkRow(4, "ticketing.hold.released"), mkRow(5, "ticketing.hold.released"),
	}
	db := newFakeDB(t, rows...)
	d := newFakeDialer()
	closedOnce := true
	d.pub.on[rows[2].EventID] = func(context.Context) error {
		if closedOnce {
			closedOnce = false
			return fmt.Errorf("publish: %w", mq.ErrClosed)
		}
		return nil
	}
	cfg := testConfig()
	r := newTestRelay(t, db, d, cfg)

	_, err := r.RunOnce(context.Background())
	if !errors.Is(err, mq.ErrClosed) {
		t.Fatalf("RunOnce = %v, muon loi boc mq.ErrClosed", err)
	}
	for _, row := range rows[:2] {
		if st := db.state(row.ID); !st.published {
			t.Errorf("row %d gui truoc khi kenh dong phai duoc danh dau: %+v", row.ID, st)
		}
	}
	for _, row := range rows[2:] {
		st := db.state(row.ID)
		if st.published || st.retries != 0 || st.parked {
			t.Errorf("row %d: %+v — ErrClosed khong duoc tang attempts", row.ID, st)
		}
		if st.releases != 1 {
			t.Errorf("row %d: lease phai duoc tra ngay (releases=%d)", row.ID, st.releases)
		}
	}
	for _, c := range d.pub.snapshot() {
		if c.msg.MessageID == rows[3].EventID || c.msg.MessageID == rows[4].EventID {
			t.Errorf("publish tiep %s sau khi kenh da dong", c.msg.MessageID)
		}
	}
	if d.closer.count() != 1 {
		t.Errorf("ket noi phai duoc dong mot lan, nhan %d", d.closer.count())
	}
	g := gatherer(t, cfg)
	if v, _ := metricValue(t, g, "outbox_relay_connected", nil); v != 0 {
		t.Errorf("outbox_relay_connected = %v sau ErrClosed, muon 0", v)
	}
	assertMarksGuarded(t, db)

	// Lan sau: dial lai va gui not cac row da tra lease.
	if _, err := r.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce sau khi dial lai: %v", err)
	}
	if d.dialCount() != 2 {
		t.Errorf("dial %d lan, muon 2", d.dialCount())
	}
	for _, row := range rows {
		if st := db.state(row.ID); !st.published {
			t.Errorf("row %d chua duoc gui sau khi dial lai: %+v", row.ID, st)
		}
	}
}

func TestRunOnce_HuyCtxVanDanhDauPhanDaGuiVaTraLeasePhanCon(t *testing.T) {
	rows := []fakeRow{
		mkRow(1, "ticketing.hold.released"), mkRow(2, "ticketing.hold.released"), mkRow(3, "ticketing.hold.released"),
		mkRow(4, "ticketing.hold.released"), mkRow(5, "ticketing.hold.released"),
	}
	db := newFakeDB(t, rows...)
	d := newFakeDialer()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	// SIGTERM toi ngay sau khi broker da ack row 2: danh dau row 2 van phai thanh cong (WithoutCancel).
	d.pub.on[rows[1].EventID] = func(context.Context) error {
		cancel()
		return nil
	}
	r := newTestRelay(t, db, d, testConfig())

	_, err := r.RunOnce(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("RunOnce = %v, muon loi boc context.Canceled", err)
	}
	for _, row := range rows[:2] {
		if st := db.state(row.ID); !st.published {
			t.Errorf("row %d da gui phai duoc danh dau du ctx bi huy (WithoutCancel): %+v", row.ID, st)
		}
	}
	for _, row := range rows[2:] {
		st := db.state(row.ID)
		if st.published || st.retries != 0 || st.parked {
			t.Errorf("row %d: %+v — huy ctx khong duoc tang attempts", row.ID, st)
		}
		if st.releases != 1 {
			t.Errorf("row %d: lease phai duoc tra ngay ca khi ctx da huy (releases=%d)", row.ID, st.releases)
		}
	}
	if n := len(d.pub.snapshot()); n != 2 {
		t.Errorf("Publish goi %d lan, muon 2 (khong publish sau khi ctx huy)", n)
	}
	assertMarksGuarded(t, db)
}

// --- MaxAttempts: do row ---

func TestRunOnce_LanThuThuMaxAttemptsThatBaiThiDoRow(t *testing.T) {
	sapDo := mkRow(1, "ticketing.hold.released")
	sapDo.Attempts = 2 // MaxAttempts 3: lan nay that bai -> attempts = 3 -> do
	conLai := mkRow(2, "ticketing.hold.released")
	db := newFakeDB(t, sapDo, conLai)
	d := newFakeDialer()
	nack := func(context.Context) error { return fmt.Errorf("x: %w", mq.ErrNacked) }
	d.pub.on[sapDo.EventID] = nack
	d.pub.on[conLai.EventID] = nack
	cfg := testConfig()
	cfg.MaxAttempts = 3
	r := newTestRelay(t, db, d, cfg)

	if _, err := r.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	s1 := db.state(sapDo.ID)
	if !s1.parked || s1.published {
		t.Errorf("row toi MaxAttempts phai bi do (next_attempt_at = 'infinity'), nhan %+v", s1)
	}
	if !contains(s1.errs, "nack") {
		t.Errorf("row bi do phai giu last_error, nhan %q", s1.errs)
	}
	s2 := db.state(conLai.ID)
	if s2.parked || s2.retries != 1 {
		t.Errorf("row con duoi MaxAttempts: %+v, muon thu lai (khong do)", s2)
	}
	assertMarksGuarded(t, db)
}

// --- r7: panic trong mot row ---

func TestRunOnce_PanicTrongMotRowDuocRecoverThanhLoi(t *testing.T) {
	a, b, c := mkRow(1, "ticketing.hold.released"), mkRow(2, "ticketing.hold.released"), mkRow(3, "ticketing.hold.released")
	db := newFakeDB(t, a, b, c)
	d := newFakeDialer()
	d.pub.on[b.EventID] = func(context.Context) error { panic("du lieu hong") }
	r := newTestRelay(t, db, d, testConfig())

	func() {
		defer func() {
			if p := recover(); p != nil {
				t.Fatalf("panic lot ra khoi RunOnce: %v", p)
			}
		}()
		_, _ = r.RunOnce(context.Background())
	}()
	sb := db.state(b.ID)
	if sb.retries != 1 || sb.published {
		t.Errorf("row panic phai duoc coi la loi dung message (attempts+1): %+v", sb)
	}
	if len(sb.errs) == 0 || sb.errs[0] == "" {
		t.Errorf("row panic phai co last_error, nhan %q", sb.errs)
	}
	for _, row := range []fakeRow{a, c} {
		if st := db.state(row.ID); !st.published {
			t.Errorf("row %d khong duoc bi row panic chan: %+v", row.ID, st)
		}
	}
	assertMarksGuarded(t, db)
}

// --- M1: loi DB khi danh dau -> dung lo ---

func TestRunOnce_LoiDBKhiDanhDauThiDungLo(t *testing.T) {
	rows := []fakeRow{mkRow(1, "ticketing.hold.released"), mkRow(2, "ticketing.hold.released"),
		mkRow(3, "ticketing.hold.released"), mkRow(4, "ticketing.hold.released")}
	db := newFakeDB(t, rows...)
	db.failMark = func(kind stmtKind, ids []int64) error {
		if kind == kindPublished && len(ids) == 1 && ids[0] == rows[1].ID {
			return errTest
		}
		return nil
	}
	d := newFakeDialer()
	r := newTestRelay(t, db, d, testConfig())

	_, err := r.RunOnce(context.Background())
	if err == nil {
		t.Error("loi DB khi danh dau phai lam RunOnce tra loi")
	}
	if n := len(d.pub.snapshot()); n != 2 {
		t.Errorf("Publish goi %d lan, muon 2 (dung lo ngay sau loi DB)", n)
	}
	for _, row := range rows[2:] {
		if st := db.state(row.ID); st.retries != 0 || st.published {
			t.Errorf("row %d chua xu ly bi doi trang thai: %+v", row.ID, st)
		}
	}
}

// --- r1: het BatchDeadline -> dung, tra lease phan con lai, khong tang attempts ---

func TestRunOnce_HetBatchDeadlineTraLeasePhanCon(t *testing.T) {
	var rows []fakeRow
	for i := int64(1); i <= 10; i++ {
		rows = append(rows, mkRow(i, "ticketing.hold.released"))
	}
	db := newFakeDB(t, rows...)
	d := newFakeDialer()
	for _, row := range rows {
		d.pub.on[row.EventID] = func(context.Context) error { // cham, khong theo ctx, nhung thanh cong
			time.Sleep(60 * time.Millisecond)
			return nil
		}
	}
	cfg := testConfig()
	cfg.PublishTimeout = 100 * time.Millisecond
	cfg.BatchDeadline = 150 * time.Millisecond
	cfg.Lease = 11 * time.Second // > 150ms + 100ms + 2*5s (plan §8 m1)
	r := newTestRelay(t, db, d, cfg)

	_, _ = r.RunOnce(context.Background())
	sent, released := 0, 0
	for _, row := range rows {
		st := db.state(row.ID)
		if st.retries != 0 || st.parked {
			t.Errorf("row %d: %+v — het han lo khong duoc tang attempts", row.ID, st)
		}
		switch {
		case st.published:
			sent++
		case st.releases == 1:
			released++
		default:
			t.Errorf("row %d khong duoc gui cung khong duoc tra lease: %+v", row.ID, st)
		}
		if d.pub.published(row.EventID) > 0 && !st.published {
			t.Errorf("row %d da publish thanh cong nhung khong duoc danh dau", row.ID)
		}
	}
	if sent == 0 || sent == len(rows) {
		t.Errorf("da gui %d/%d row; han lo 150ms voi 60ms/row phai cat giua chung", sent, len(rows))
	}
	assertMarksGuarded(t, db)
}

// --- B1/r6: khong ket noi thi khong claim ---

func TestRunOnce_DialLoiThiKhongClaim(t *testing.T) {
	db := newFakeDB(t, mkRow(1, "ticketing.hold.released"))
	d := newFakeDialer()
	d.err = errors.New("dial tcp: connection refused")
	cfg := testConfig()
	r := newTestRelay(t, db, d, cfg)

	if _, err := r.RunOnce(context.Background()); err == nil {
		t.Error("dial loi phai tra loi")
	}
	if db.claimCount() != 0 {
		t.Errorf("claim %d lan khi chua ket noi, muon 0", db.claimCount())
	}
	if st := db.state(1); st.retries != 0 {
		t.Errorf("dial loi khong duoc tang attempts: %+v", st)
	}
	if v, _ := metricValue(t, gatherer(t, cfg), "outbox_relay_connected", nil); v != 0 {
		t.Errorf("outbox_relay_connected = %v, muon 0", v)
	}
}

func TestRun_DialerLuonLoiKhongTraVeToiKhiHuyCtx(t *testing.T) {
	db := newFakeDB(t, mkRow(1, "ticketing.hold.released"))
	d := newFakeDialer()
	d.err = errors.New("dial tcp: connection refused")
	cfg := testConfig()
	r := newTestRelay(t, db, d, cfg)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- r.Run(ctx) }()

	select {
	case err := <-done:
		t.Fatalf("Run tra ve %v khi broker khong toi duoc; worker se chet theo (B1)", err)
	case <-time.After(1500 * time.Millisecond):
	}
	if d.dialCount() < 1 {
		t.Error("Run phai thu dial")
	}
	if db.claimCount() != 0 {
		t.Errorf("claim %d lan khi chua ket noi, muon 0", db.claimCount())
	}
	if v, _ := metricValue(t, gatherer(t, cfg), "outbox_relay_connected", nil); v != 0 {
		t.Errorf("outbox_relay_connected = %v, muon 0", v)
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Run sau khi huy ctx tra %v, muon nil", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Run khong tra ve sau khi huy ctx (backoff dial phai theo ctx)")
	}
}

func TestRun_HuyCtxTraNilVaDongKetNoi(t *testing.T) {
	row := mkRow(1, "ticketing.hold.released")
	db := newFakeDB(t, row)
	d := newFakeDialer()
	r := newTestRelay(t, db, d, testConfig())

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- r.Run(ctx) }()
	deadline := time.Now().Add(2 * time.Second)
	for !db.state(row.ID).published {
		if time.Now().After(deadline) {
			t.Fatal("Run khong gui row trong 2s")
		}
		time.Sleep(5 * time.Millisecond)
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Run = %v, muon nil", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Run khong tra ve sau khi huy ctx")
	}
	if d.closer.count() != 1 {
		t.Errorf("ket noi phai duoc dong khi Run ket thuc, Close goi %d lan", d.closer.count())
	}
}

// --- backoff (plan §6.4, §7 r2) ---
//
// SEAM KHONG EXPORT gia dinh:
//
//	// backoff tra ve do lui cho lan thu lai sau `attempts` lan that bai:
//	// min(base * 2^clamp(attempts-1, 0, 20), max), nhan jitter ngau nhien trong [0.8, 1.2].
//	func backoff(attempts int, base, max time.Duration) time.Duration

func TestBackoff_LuyThuaCoTranVaJitter(t *testing.T) {
	base, max := time.Second, 5*time.Minute
	cases := []struct {
		attempts  int
		danhNghia time.Duration
	}{
		{0, time.Second}, // so mu kep ve 0, khong dich am
		{1, time.Second},
		{2, 2 * time.Second},
		{5, 16 * time.Second},
		{9, 256 * time.Second},
		{10, max},            // 512s > 5m -> tran
		{21, max},            // so mu kep 20
		{math.MaxInt32, max}, // khong tran so
		{-5, time.Second},
	}
	for _, tc := range cases {
		var minSeen, maxSeen time.Duration
		for i := 0; i < 300; i++ {
			got := backoff(tc.attempts, base, max)
			lo := time.Duration(float64(tc.danhNghia) * 0.8)
			hi := time.Duration(float64(tc.danhNghia) * 1.2)
			if got < lo || got > hi || got <= 0 {
				t.Fatalf("backoff(%d) = %v, ngoai [%v, %v]", tc.attempts, got, lo, hi)
			}
			if i == 0 || got < minSeen {
				minSeen = got
			}
			if got > maxSeen {
				maxSeen = got
			}
		}
		if minSeen == maxSeen {
			t.Errorf("backoff(%d) khong co jitter (luon %v)", tc.attempts, minSeen)
		}
	}
}

// So mu kep o 20 (khong chi nho tran): base 1ns, max 1h -> tran tren danh nghia la 2^20 ns.
func TestBackoff_SoMuKepO20(t *testing.T) {
	nominal := time.Duration(1 << 20)
	for _, a := range []int{21, 22, 43, 64, math.MaxInt32} {
		got := backoff(a, time.Nanosecond, time.Hour)
		if got < nominal*8/10 || got > nominal*12/10 {
			t.Errorf("backoff(%d, 1ns, 1h) = %v, muon ~%v (so mu kep 20)", a, got, nominal)
		}
	}
}

func TestBackoff_MaxMotGioKhongTran(t *testing.T) {
	for _, a := range []int{20, 21, 64, math.MaxInt32} {
		got := backoff(a, time.Hour, time.Hour)
		if got <= 0 || got > time.Duration(float64(time.Hour)*1.2) {
			t.Errorf("backoff(%d, 1h, 1h) = %v", a, got)
		}
	}
}

// --- sanitizeErr (plan §6.4, review M2) ---
//
// SEAM KHONG EXPORT gia dinh:
//
//	// sanitizeErr tra chuoi luu vao last_error: err.Error() da strings.ToValidUTF8, bo \x00, cat <= 1024 byte
//	// (khong cat giua mot ky tu UTF-8).
//	func sanitizeErr(err error) string

func TestSanitizeErr_LamSachLastError(t *testing.T) {
	got := sanitizeErr(errors.New("loi \x00giua\xff\xfe cuoi"))
	if strings.ContainsRune(got, 0) {
		t.Errorf("con byte NUL: %q", got)
	}
	if !isValidUTF8(got) {
		t.Errorf("UTF-8 khong hop le: %q", got)
	}
	if !strings.Contains(got, "giua") || !strings.Contains(got, "cuoi") {
		t.Errorf("mat noi dung loi: %q", got)
	}

	long := strings.Repeat("ố", 2000) // 3 byte moi ky tu
	got = sanitizeErr(errors.New(long))
	if len(got) > 1024 || len(got) < 1000 {
		t.Errorf("do dai %d, muon cat ve <= 1024 (va khong cat qua tay)", len(got))
	}
	if !isValidUTF8(got) {
		t.Error("cat giua mot ky tu UTF-8")
	}
	if s := sanitizeErr(errors.New("ngan")); s != "ngan" {
		t.Errorf("loi ngan bi doi: %q", s)
	}
}

func isValidUTF8(s string) bool { return utf8.ValidString(s) }

// Bao dam *pgxpool.Pool thoa DB va *mq.Publisher thoa Publisher (bat loi chu ky som).
var (
	_ DB        = (*pgxpool.Pool)(nil)
	_ Publisher = (*mq.Publisher)(nil)
	_ Dialer    = func(context.Context) (Publisher, io.Closer, error) { return nil, nil, nil }
)

// --- plan §8 (code review) ---

// m1: ctx cua lan publish bi chan boi han chot lo — PublishTimeout khong duoc keo dai qua BatchDeadline.
func TestRunOnce_PublishBiCatTaiHanChotLo(t *testing.T) {
	a, b := mkRow(1, "ticketing.hold.released"), mkRow(2, "ticketing.hold.released")
	db := newFakeDB(t, a, b)
	d := newFakeDialer()
	d.pub.on[a.EventID] = func(context.Context) error { // cham nhung thanh cong, khong theo ctx
		time.Sleep(300 * time.Millisecond)
		return nil
	}
	var deadline time.Time
	var hasDeadline bool
	d.pub.on[b.EventID] = func(ctx context.Context) error {
		deadline, hasDeadline = ctx.Deadline()
		<-ctx.Done()
		return fmt.Errorf("mq: cho confirm: %w", ctx.Err())
	}
	cfg := testConfig()
	cfg.BatchDeadline = 400 * time.Millisecond
	cfg.PublishTimeout = 400 * time.Millisecond // row b bat dau ~300ms: tu han cua no la ~700ms
	r := newTestRelay(t, db, d, cfg)

	start := time.Now()
	_, _ = r.RunOnce(context.Background())
	el := time.Since(start)
	if !hasDeadline {
		t.Fatal("ctx publish khong co han chot")
	}
	if lim := start.Add(cfg.BatchDeadline + 100*time.Millisecond); deadline.After(lim) {
		t.Errorf("han ctx publish = start+%v, muon <= start+%v (han chot lo)", deadline.Sub(start), cfg.BatchDeadline)
	}
	if el > 650*time.Millisecond {
		t.Errorf("RunOnce mat %v; publish cuoi vuot han chot lo 400ms", el)
	}
	if st := db.state(a.ID); !st.published {
		t.Errorf("row a da gui truoc han chot phai duoc danh dau: %+v", st)
	}
	if st := db.state(b.ID); st.published || st.parked {
		t.Errorf("row b bi cat tai han chot: %+v, muon chua gui va khong do", st)
	}
	// plan §9 M23/M47: het han lo giua publish khong tinh attempts va tra lease ngay.
	if st := db.state(b.ID); st.retries != 0 || st.releases != 1 {
		t.Errorf("row b het han lo: %+v, muon attempts khong doi va lease duoc tra (releases=1)", st)
	}
	assertMarksGuarded(t, db)
}

// m1: tra lease khong duoc go do mot row ma relay khac vua do (next_attempt_at = 'infinity').
func TestRunOnce_TraLeaseKhongGoDoRowDaDo(t *testing.T) {
	rows := []fakeRow{mkRow(1, "ticketing.hold.released"), mkRow(2, "ticketing.hold.released"), mkRow(3, "ticketing.hold.released")}
	db := newFakeDB(t, rows...)
	d := newFakeDialer()
	d.pub.on[rows[0].EventID] = func(context.Context) error {
		db.setParked(rows[2].ID) // relay khac (lease cu het han) da claim lai row 3 va do no
		return fmt.Errorf("publish: %w", mq.ErrClosed)
	}
	r := newTestRelay(t, db, d, testConfig())

	if _, err := r.RunOnce(context.Background()); !errors.Is(err, mq.ErrClosed) {
		t.Fatalf("RunOnce = %v, muon ErrClosed", err)
	}
	if st := db.state(rows[2].ID); !st.parked || st.due {
		t.Errorf("row da do bi tra lease lam song lai: %+v", st)
	}
	for _, row := range rows[:2] {
		if st := db.state(row.ID); st.releases != 1 {
			t.Errorf("row %d chua do phai duoc tra lease: %+v", row.ID, st)
		}
	}
	assertMarksGuarded(t, db)
}

// m3: loi publish khong phan loai (khong phai ErrClosed/ctx) tinh la mot lan thu, ghi last_error.
func TestRunOnce_LoiKhongPhanLoaiTangAttempts(t *testing.T) {
	a, b := mkRow(1, "ticketing.hold.released"), mkRow(2, "ticketing.hold.released")
	db := newFakeDB(t, a, b)
	d := newFakeDialer()
	d.pub.on[a.EventID] = func(context.Context) error { return errors.New("loi la chua tung gap") }
	r := newTestRelay(t, db, d, testConfig())

	if _, err := r.RunOnce(context.Background()); err != nil {
		t.Fatalf("loi cua mot row khong duoc lam RunOnce tra loi: %v", err)
	}
	sa := db.state(a.ID)
	if sa.retries != 1 || sa.published || sa.releases != 0 {
		t.Errorf("row loi khong phan loai: %+v, muon attempts+1", sa)
	}
	if !contains(sa.errs, "loi la chua tung gap") {
		t.Errorf("last_error = %q, muon chua loi goc", sa.errs)
	}
	if d.closer.count() != 0 {
		t.Error("loi khong phan loai khong duoc dong ket noi")
	}
	if sb := db.state(b.ID); !sb.published {
		t.Errorf("row khac bi chan: %+v", sb)
	}
	assertMarksGuarded(t, db)
}

// m4: ket noi bao da dong (Closer co IsClosed() bool) truoc khi claim -> khong claim, gauge 0, lan sau dial lai.
//
// SEAM gia dinh: io.Closer (hoac Publisher) ma Dialer tra ve co the hien thuc them `IsClosed() bool`
// (*amqp091.Connection co san); relay kiem truoc moi lan claim, neu true thi dong ket noi va dat
// outbox_relay_connected = 0. Chap nhan hai cach: (a) khong claim trong lan RunOnce do, lan sau dial lai;
// (b) dial lai ngay trong cung lan RunOnce roi moi claim. Bat bien chung: KHONG claim/publish tren ket noi chet.
func TestRunOnce_KetNoiDaDongTruocClaimThiKhongClaimVaDialLai(t *testing.T) {
	a := mkRow(1, "ticketing.hold.released")
	db := newFakeDB(t, a)
	d := newFakeDialer()
	cfg := testConfig()
	r := newTestRelay(t, db, d, cfg)
	g := gatherer(t, cfg)

	if _, err := r.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce 1: %v", err)
	}
	claims := db.claimCount()
	b := mkRow(2, "ticketing.hold.released")
	db.add(b)
	d.closer.kill() // broker dong ket noi khi relay dang ranh

	_, _ = r.RunOnce(context.Background())
	if d.closer.count() != 1 {
		t.Errorf("ket noi chet phai duoc dong (Close %d lan)", d.closer.count())
	}
	switch d.dialCount() {
	case 1: // (a) chua dial lai: khong duoc claim, gauge 0
		if db.claimCount() != claims {
			t.Errorf("claim khi ket noi da chet (%d -> %d lan)", claims, db.claimCount())
		}
		if v, _ := metricValue(t, g, "outbox_relay_connected", nil); v != 0 {
			t.Errorf("outbox_relay_connected = %v, muon 0 khi ket noi da dong", v)
		}
		if _, err := r.RunOnce(context.Background()); err != nil {
			t.Fatalf("RunOnce 3: %v", err)
		}
	case 2: // (b) da dial lai ngay trong lan nay
	default:
		t.Errorf("dial %d lan sau khi ket noi chet, muon 1 hoac 2", d.dialCount())
	}
	for _, c := range d.pub.snapshot() {
		if c.deadConn {
			t.Errorf("publish %s tren ket noi da chet", c.msg.MessageID)
		}
	}
	if d.dialCount() != 2 {
		t.Errorf("lan sau phai dial lai: dial %d lan, muon 2", d.dialCount())
	}
	if st := db.state(b.ID); !st.published {
		t.Errorf("row moi phai duoc gui sau khi dial lai: %+v", st)
	}
	if v, _ := metricValue(t, g, "outbox_relay_connected", nil); v != 1 {
		t.Errorf("outbox_relay_connected = %v sau khi dial lai, muon 1", v)
	}
}

// m8: metric published/lag chi ghi SAU khi danh dau thanh cong.
func TestRunOnce_DanhDauLoiThiKhongTangMetricPublished(t *testing.T) {
	a := mkRow(1, "ticketing.hold.released")
	db := newFakeDB(t, a)
	db.failMark = func(kind stmtKind, _ []int64) error {
		if kind == kindPublished {
			return errTest
		}
		return nil
	}
	d := newFakeDialer()
	cfg := testConfig()
	r := newTestRelay(t, db, d, cfg)

	n, err := r.RunOnce(context.Background())
	if err == nil {
		t.Error("danh dau loi phai lam RunOnce tra loi")
	}
	if n != 0 {
		t.Errorf("published = %d, muon 0 (chua danh dau duoc)", n)
	}
	g := gatherer(t, cfg)
	if v, _ := metricValue(t, g, "outbox_published_total", nil); v != 0 {
		t.Errorf("outbox_published_total = %v, muon 0 khi danh dau loi", v)
	}
	if v, _ := metricValue(t, g, "outbox_publish_lag_seconds", nil); v != 0 {
		t.Errorf("outbox_publish_lag_seconds co %v mau, muon 0 khi danh dau loi", v)
	}
}

// --- plan §9 (challenge vong 2) ---

// F1: cau thu lai / do khong duoc dung vao row ma relay khac da do trong luc relay nay publish (lease cu het han).
func TestRunOnce_ThuLaiVaDoKhongDungRowDaBiRelayKhacDo(t *testing.T) {
	retry := mkRow(1, "ticketing.hold.released")
	park := mkRow(2, "ticketing.hold.released")
	park.Attempts = 4 // MaxAttempts 5: lan that bai nay se do
	db := newFakeDB(t, retry, park)
	d := newFakeDialer()
	for _, row := range []fakeRow{retry, park} {
		id := row.ID
		d.pub.on[row.EventID] = func(context.Context) error {
			db.setParked(id) // relay khac vua do row nay
			return fmt.Errorf("x: %w", mq.ErrNacked)
		}
	}
	r := newTestRelay(t, db, d, testConfig())

	if _, err := r.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	for _, row := range []fakeRow{retry, park} {
		st := db.state(row.ID)
		if st.unparkedOrTouched || st.retries != 0 || !st.parked {
			t.Errorf("row %d da bi relay khac do nhung bi cau thu lai/do dung vao: %+v", row.ID, st)
		}
	}
	assertMarksGuarded(t, db)
}

// F3 + M42: lag tinh theo dong ho DB tra ve tu claim (khong theo dong ho Go).
func TestRunOnce_LagTheoDongHoDB(t *testing.T) {
	a := mkRow(1, "ticketing.hold.released")
	db := newFakeDB(t, a)
	db.dbClock = func() time.Time { return a.CreatedAt.Add(7 * time.Second) }
	d := newFakeDialer()
	cfg := testConfig()
	r := newTestRelay(t, db, d, cfg)

	if _, err := r.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	sum, n := histSum(t, gatherer(t, cfg), "outbox_publish_lag_seconds")
	if n != 1 {
		t.Fatalf("outbox_publish_lag_seconds co %d mau, muon 1", n)
	}
	if sum < 7 || sum > 8 {
		t.Errorf("lag = %.3fs, muon ~7s (db_now luc claim - created_at, cong thoi gian troi trong Go)", sum)
	}
}

// F3: dong ho DB lech lam lag am -> kep ve 0.
func TestRunOnce_LagAmKepVe0(t *testing.T) {
	a := mkRow(1, "ticketing.hold.released")
	db := newFakeDB(t, a)
	db.dbClock = func() time.Time { return a.CreatedAt.Add(-5 * time.Second) }
	d := newFakeDialer()
	cfg := testConfig()
	r := newTestRelay(t, db, d, cfg)

	if _, err := r.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	sum, n := histSum(t, gatherer(t, cfg), "outbox_publish_lag_seconds")
	if n != 1 || sum != 0 {
		t.Errorf("lag am: tong %.3f, %d mau — muon 1 mau gia tri 0", sum, n)
	}
}

// M22 + M48: ctx huy trong luc publish (publish tra loi ctx) -> khong tinh attempts, tra lease ca row do.
func TestRunOnce_HuyCtxGiuaPublishKhongTinhAttempts(t *testing.T) {
	rows := []fakeRow{mkRow(1, "ticketing.hold.released"), mkRow(2, "ticketing.hold.released"), mkRow(3, "ticketing.hold.released")}
	db := newFakeDB(t, rows...)
	d := newFakeDialer()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	d.pub.on[rows[1].EventID] = func(pctx context.Context) error {
		cancel()
		<-pctx.Done()
		return fmt.Errorf("mq: cho confirm: %w", pctx.Err())
	}
	cfg := testConfig()
	r := newTestRelay(t, db, d, cfg)

	if _, err := r.RunOnce(ctx); !errors.Is(err, context.Canceled) {
		t.Errorf("RunOnce = %v, muon boc context.Canceled", err)
	}
	if st := db.state(rows[0].ID); !st.published {
		t.Errorf("row 1: %+v, muon da gui", st)
	}
	for _, row := range rows[1:] {
		st := db.state(row.ID)
		if st.retries != 0 || st.published || st.parked {
			t.Errorf("row %d: %+v — huy ctx giua publish khong duoc tinh attempts", row.ID, st)
		}
		if st.releases != 1 {
			t.Errorf("row %d: lease phai duoc tra (releases=%d)", row.ID, st.releases)
		}
	}
	if v, _ := metricValue(t, gatherer(t, cfg), "outbox_publish_errors_total", nil); v != 0 {
		t.Errorf("outbox_publish_errors_total = %v, muon 0 khi ctx huy", v)
	}
}

// M43: loi tam tang outbox_publish_errors_total{routing_key}.
func TestRunOnce_LoiTamTangPublishErrorsTotal(t *testing.T) {
	a := mkRow(1, "ticketing.hold.released")
	db := newFakeDB(t, a)
	d := newFakeDialer()
	d.pub.on[a.EventID] = func(context.Context) error { return fmt.Errorf("x: %w", mq.ErrNacked) }
	cfg := testConfig()
	r := newTestRelay(t, db, d, cfg)
	if _, err := r.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	v, ok := metricValue(t, gatherer(t, cfg), "outbox_publish_errors_total", map[string]string{"routing_key": a.RoutingKey})
	if !ok || v != 1 {
		t.Errorf("outbox_publish_errors_total{routing_key=%s} = %v (co %v), muon 1", a.RoutingKey, v, ok)
	}
}

// M36: claim chay trong transaction co SET LOCAL statement_timeout truoc cau claim.
func TestRunOnce_ClaimCoStatementTimeoutTrongTx(t *testing.T) {
	db := newFakeDB(t, mkRow(1, "ticketing.hold.released"))
	r := newTestRelay(t, db, newFakeDialer(), testConfig())
	if _, err := r.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	seenTimeout := false
	for _, s := range db.all() {
		if s.inTx && strings.HasPrefix(s.sql, "set local statement_timeout") {
			seenTimeout = true
		}
		if s.kind == kindClaim {
			if !s.inTx {
				t.Error("claim phai chay trong transaction (Begin)")
			}
			if !seenTimeout {
				t.Error("thieu SET LOCAL statement_timeout trong transaction truoc cau claim")
			}
			return
		}
	}
	t.Fatal("khong co cau claim")
}

// M29: SQL dem gauge: outbox_pending khong tinh row do, outbox_dead dem row 'infinity'.
func TestRun_GaugePendingKhongTinhRowDo(t *testing.T) {
	db := newFakeDB(t)
	r := newTestRelay(t, db, newFakeDialer(), testConfig())
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- r.Run(ctx) }()
	time.Sleep(200 * time.Millisecond)
	cancel()
	<-done

	var gauge string
	for _, s := range db.all() {
		if strings.Contains(s.sql, "count(") {
			gauge = s.sql
		}
	}
	if gauge == "" {
		t.Fatal("Run khong chay cau dem gauge (outbox_pending/outbox_dead) luc khoi dong")
	}
	if !strings.Contains(gauge, "published_at is null") {
		t.Errorf("gauge phai chi dem row chua gui: %s", gauge)
	}
	if !strings.Contains(gauge, "next_attempt_at<>'infinity'") || !strings.Contains(gauge, "next_attempt_at='infinity'") {
		t.Errorf("gauge phai tach row do ('infinity') khoi outbox_pending: %s", gauge)
	}
}

// M30: backoff dial lai tang dan (base, 2*base, 4*base, ... co jitter), khong co dinh.
//
// SEAM (do implementer dat o dau relay.go, khong export): Relay.redialBase, Relay.redialMax. Test dat base 100ms:
// dial o t=0, ~0.1s (0.08-0.12), ~0.3s (0.24-0.36), ~0.7s (0.56-0.84), lan thu 5 >= 1.2s -> 4 lan trong 1s;
// backoff co dinh 100ms se cho ~9-10 lan.
func TestRun_BackoffDialLaiTangDan(t *testing.T) {
	db := newFakeDB(t)
	d := newFakeDialer()
	d.err = errors.New("dial tcp: connection refused")
	r := newTestRelay(t, db, d, testConfig())
	r.redialBase, r.redialMax = 100*time.Millisecond, 10*time.Second
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- r.Run(ctx) }()
	time.Sleep(time.Second)
	n := d.dialCount()
	cancel()
	<-done
	if n < 3 || n > 5 {
		t.Errorf("dial %d lan trong 1s voi base 100ms, muon 3-5 (lui 0.1s, 0.2s, 0.4s, ...); co dinh se cho ~10", n)
	}
}

// M31: so lan that bai lien tiep duoc dat lai sau mot lo thanh cong (ke ca lo rong): loi sau do chi lui ~base.
// Dung seam Relay.redialBase = 200ms: dung thi lui 160-240ms, neu khong dat lai (fails = 2) se lui 320-480ms.
func TestRun_LoThanhCongDatLaiBackoff(t *testing.T) {
	db := newFakeDB(t)
	db.failClaim = func(n int) error {
		if n == 3 {
			return errTest
		}
		return nil
	}
	d := newFakeDialer()
	var dialN int
	var mu sync.Mutex
	inner := d.dial
	failingFirst := func(ctx context.Context) (Publisher, io.Closer, error) {
		mu.Lock()
		dialN++
		first := dialN == 1
		mu.Unlock()
		if first {
			return nil, nil, errors.New("dial tcp: connection refused") // fails = 1
		}
		return inner(ctx)
	}
	r, err := NewRelay(db, failingFirst, testConfig())
	if err != nil {
		t.Fatalf("NewRelay: %v", err)
	}
	r.redialBase, r.redialMax = 200*time.Millisecond, 10*time.Second
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- r.Run(ctx) }()
	deadline := time.Now().Add(5 * time.Second)
	for len(db.claimTimesSnap()) < 4 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	cancel()
	<-done
	ts := db.claimTimesSnap()
	if len(ts) < 4 {
		t.Fatalf("chi %d lan claim trong 5s", len(ts))
	}
	gap := ts[3].Sub(ts[2])
	if gap > 290*time.Millisecond {
		t.Errorf("sau loi claim (sau cac lo rong thanh cong) relay lui %v, muon ~200ms (fails phai duoc dat lai)", gap)
	}
	if gap < 100*time.Millisecond {
		t.Errorf("sau loi claim relay chi lui %v, muon ~200ms", gap)
	}
}
