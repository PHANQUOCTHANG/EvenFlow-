//go:build integration

package outbox

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/testcontainers/testcontainers-go"
	tcexec "github.com/testcontainers/testcontainers-go/exec"
	"github.com/testcontainers/testcontainers-go/modules/rabbitmq"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"

	"github.com/eventflow/eventflow/libs/go/mq"
	"github.com/eventflow/eventflow/libs/go/otelx"
	"github.com/eventflow/eventflow/libs/go/testx/pgtest"
)

// Test tich hop outbox + relay voi Postgres 16 (pgtest) va RabbitMQ 3.13 that (spec AC-3..AC-6, AC-8;
// plan §6.6, §7 R2, r4). Chay: go test -tags=integration ./outbox/... ; khong co Docker thi FAIL (khong skip).
//
// Ten export dua vao: Write, Event, Envelope, NewRelay, RelayConfig, Relay.Run, Relay.RunOnce, DB, Publisher,
// Dialer. Migration: migrations/0001_core.sql, migrations/0002_outbox_relay.sql (plan §6.1).

const brokerImage = "rabbitmq:3.13-management-alpine"

var (
	brokerOnce sync.Once
	brokerURL  string
	brokerErr  error
	brokerCtr  *rabbitmq.RabbitMQContainer
	seq        atomic.Int64
)

func TestMain(m *testing.M) {
	code := m.Run()
	pgtest.Terminate()
	if brokerCtr != nil {
		_ = testcontainers.TerminateContainer(brokerCtr)
	}
	os.Exit(code)
}

func broker(t *testing.T) string {
	t.Helper()
	brokerOnce.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()
		brokerCtr, brokerErr = rabbitmq.Run(ctx, brokerImage)
		if brokerErr != nil {
			return
		}
		brokerURL, brokerErr = brokerCtr.AmqpURL(ctx)
	})
	if brokerErr != nil {
		t.Fatalf("khong khoi dong duoc RabbitMQ (%s) — can Docker: %v", brokerImage, brokerErr)
	}
	return brokerURL
}

func rabbitctl(t *testing.T, args ...string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	code, out, err := brokerCtr.Exec(ctx, args, tcexec.Multiplexed())
	if err != nil {
		t.Fatalf("exec %v: %v", args, err)
	}
	b, _ := io.ReadAll(out)
	if code != 0 {
		t.Fatalf("exec %v: exit %d: %s", args, code, b)
	}
}

func eventually(t *testing.T, timeout time.Duration, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("het %v ma van chua: %s", timeout, what)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func newUUID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// --- topology va dialer toi broker that ---

type itTopo struct {
	exchange, queue string
	top             mq.Topology
}

// newTopo tao exchange topic + queue quorum rieng cho test, bind theo cac key.
func newTopo(keys ...string) itTopo {
	p := fmt.Sprintf("ob%d", seq.Add(1))
	tt := itTopo{exchange: p + ".events", queue: p + ".q"}
	tt.top = mq.Topology{
		Exchanges: []mq.Exchange{{Name: tt.exchange, Kind: "topic", Durable: true}},
		Queues:    []mq.Queue{{Name: tt.queue, Durable: true, Args: map[string]any{"x-queue-type": "quorum"}}},
	}
	for _, k := range keys {
		tt.top.Bindings = append(tt.top.Bindings, mq.Binding{Source: tt.exchange, Destination: tt.queue, RoutingKey: k})
	}
	return tt
}

// brokerDialer la Dialer that: dial (giu net.Conn de dong cung), kenh topology + kenh publish, mq.NewPublisher.
type brokerDialer struct {
	url  string
	topo mq.Topology
	wrap func(Publisher) Publisher

	mu    sync.Mutex
	socks []net.Conn
	dials int
}

func (b *brokerDialer) dial(ctx context.Context) (Publisher, io.Closer, error) {
	b.mu.Lock()
	b.dials++
	b.mu.Unlock()
	conn, err := amqp.DialConfig(b.url, amqp.Config{
		Heartbeat: 5 * time.Second,
		Locale:    "en_US",
		Dial: func(network, addr string) (net.Conn, error) {
			c, err := net.DialTimeout(network, addr, 5*time.Second)
			if err == nil {
				b.mu.Lock()
				b.socks = append(b.socks, c)
				b.mu.Unlock()
			}
			return c, err
		},
	})
	if err != nil {
		return nil, nil, err
	}
	tch, err := conn.Channel()
	if err != nil {
		_ = conn.Close()
		return nil, nil, err
	}
	if err := mq.Declare(tch, b.topo); err != nil {
		_ = conn.Close()
		return nil, nil, err
	}
	_ = tch.Close()
	pch, err := conn.Channel()
	if err != nil {
		_ = conn.Close()
		return nil, nil, err
	}
	pub, err := mq.NewPublisher(pch)
	if err != nil {
		_ = conn.Close()
		return nil, nil, err
	}
	var p Publisher = pub
	if b.wrap != nil {
		p = b.wrap(p)
	}
	return p, conn, nil
}

// killTCP dong CUNG moi socket AMQP (khong gui connection.close) — mo phong tien trinh/mang chet.
func (b *brokerDialer) killTCP() {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, s := range b.socks {
		_ = s.Close()
	}
}

// pubFunc bien mot ham thanh Publisher (de boc publisher that).
type pubFunc func(ctx context.Context, exchange, key string, msg mq.Message) error

func (f pubFunc) Publish(ctx context.Context, exchange, key string, msg mq.Message) error {
	return f(ctx, exchange, key, msg)
}

func amqpConn(t *testing.T) *amqp.Connection {
	t.Helper()
	conn, err := amqp.Dial(broker(t))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

// declare khai bao topology truoc (de queue ton tai ngay ca khi relay chua dial).
func declare(t *testing.T, tt itTopo) {
	t.Helper()
	ch, err := amqpConn(t).Channel()
	if err != nil {
		t.Fatalf("channel: %v", err)
	}
	if err := mq.Declare(ch, tt.top); err != nil {
		t.Fatalf("declare: %v", err)
	}
}

// drain lay het message dang co trong queue.
func drain(t *testing.T, queue string) []amqp.Delivery {
	t.Helper()
	ch, err := amqpConn(t).Channel()
	if err != nil {
		t.Fatalf("channel: %v", err)
	}
	defer func() { _ = ch.Close() }()
	var out []amqp.Delivery
	for {
		d, ok, err := ch.Get(queue, true)
		if err != nil {
			t.Fatalf("get %s: %v", queue, err)
		}
		if !ok {
			return out
		}
		out = append(out, d)
	}
}

// countByEventID dem so lan moi event_id toi queue; kiem MessageId == envelope.event_id.
func countByEventID(t *testing.T, ds []amqp.Delivery) map[string]int {
	t.Helper()
	m := map[string]int{}
	for _, d := range ds {
		var env Envelope
		if err := json.Unmarshal(d.Body, &env); err != nil {
			t.Fatalf("body khong phai phong bi: %v (%s)", err, d.Body)
		}
		if d.MessageId != env.EventID {
			t.Errorf("MessageId %q khac envelope.event_id %q", d.MessageId, env.EventID)
		}
		m[env.EventID]++
	}
	return m
}

// assertAtLeastOnce: moi event_id toi >= 1 lan, so ban trung <= maxDup, khong co id la.
func assertAtLeastOnce(t *testing.T, ids []string, got map[string]int, maxDup int) {
	t.Helper()
	want := map[string]bool{}
	for _, id := range ids {
		want[id] = true
		if got[id] < 1 {
			t.Errorf("MAT event %s", id)
		}
	}
	dups := 0
	for id, n := range got {
		if !want[id] {
			t.Errorf("event la %s", id)
		}
		dups += n - 1
	}
	if dups > maxDup {
		t.Errorf("%d ban trung, muon <= %d (kich thuoc lo)", dups, maxDup)
	}
	t.Logf("%d event, %d ban trung", len(ids), dups)
}

// --- du lieu ---

func writeEvents(t *testing.T, ctx context.Context, pool *pgxpool.Pool, n int, key string) []string {
	t.Helper()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	ids := make([]string, 0, n)
	for i := 0; i < n; i++ {
		id, err := Write(ctx, tx, Event{AggregateType: "hold", AggregateID: newUUID(), RoutingKey: key,
			Data: map[string]any{"i": i, "hold_id": newUUID()}})
		if err != nil {
			t.Fatalf("Write: %v", err)
		}
		ids = append(ids, id)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit: %v", err)
	}
	return ids
}

func countWhere(t *testing.T, pool *pgxpool.Pool, where string, args ...any) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), "SELECT count(*) FROM outbox_events WHERE "+where, args...).Scan(&n); err != nil {
		t.Fatalf("count %s: %v", where, err)
	}
	return n
}

func itConfig(exchange string) RelayConfig {
	cfg := testConfig()
	cfg.Exchange = exchange
	cfg.BatchSize = 10
	cfg.Interval = 50 * time.Millisecond
	cfg.Lease = 3 * time.Second
	cfg.BatchDeadline = 2 * time.Second
	cfg.PublishTimeout = time.Second
	cfg.BaseBackoff = 50 * time.Millisecond
	cfg.MaxBackoff = 500 * time.Millisecond
	cfg.MaxAttempts = 20
	return cfg
}

// runRelay chay Run trong goroutine; stop huy ctx va cho Run tra ve.
func runRelay(t *testing.T, r *Relay) (stop func()) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- r.Run(ctx) }()
	var once sync.Once
	stop = func() {
		once.Do(func() {
			cancel()
			select {
			case err := <-done:
				if err != nil {
					t.Errorf("Run = %v, muon nil khi huy ctx", err)
				}
			case <-time.After(30 * time.Second):
				t.Error("Run khong tra ve sau khi huy ctx")
			}
		})
	}
	t.Cleanup(stop)
	return stop
}

// --- AC-3 ---

func TestTichHop_AC3_PublishVaDanhDau(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.New(t)
	tt := newTopo("ticketing.hold.released")
	declare(t, tt)
	ids := writeEvents(t, ctx, pool, 25, "ticketing.hold.released")

	d := &brokerDialer{url: broker(t), topo: tt.top}
	r, err := NewRelay(pool, d.dial, itConfig(tt.exchange))
	if err != nil {
		t.Fatalf("NewRelay: %v", err)
	}
	total := 0
	for i := 0; i < 10; i++ {
		n, err := r.RunOnce(ctx)
		if err != nil {
			t.Fatalf("RunOnce: %v", err)
		}
		total += n
		if n == 0 {
			break
		}
	}
	if total != 25 {
		t.Errorf("published tong %d, muon 25", total)
	}
	if n := countWhere(t, pool, "published_at IS NULL"); n != 0 {
		t.Errorf("%d row chua co published_at", n)
	}
	if n := countWhere(t, pool, "attempts <> 0 OR last_error IS NOT NULL"); n != 0 {
		t.Errorf("%d row co attempts/last_error du publish thanh cong", n)
	}
	ds := drain(t, tt.queue)
	got := countByEventID(t, ds)
	assertAtLeastOnce(t, ids, got, 0)
	for _, dl := range ds {
		if dl.ContentType != "application/json" || dl.Type != "ticketing.hold.released" || dl.DeliveryMode != amqp.Persistent {
			t.Errorf("property sai: content-type %q type %q mode %d", dl.ContentType, dl.Type, dl.DeliveryMode)
		}
		var env Envelope
		_ = json.Unmarshal(dl.Body, &env)
		if env.SchemaVersion != 1 || env.Type != "ticketing.hold.released" || env.OccurredAt.IsZero() {
			t.Errorf("phong bi sai: %+v", env)
		}
		if m := decodeJSON(t, env.Data); m["hold_id"] == nil {
			t.Errorf("data mat truong: %s", env.Data)
		}
	}
}

// --- AC-4: dong cung AMQP giua lo, relay chet, relay moi gui not ---

func TestTichHop_AC4_DongCungAMQPGiuaLoRoiKhoiDongLai(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.New(t)
	tt := newTopo("ticketing.hold.released")
	declare(t, tt)
	ids := writeEvents(t, ctx, pool, 30, "ticketing.hold.released")

	ctx1, cancel1 := context.WithCancel(ctx)
	defer cancel1()
	var ok atomic.Int32
	d1 := &brokerDialer{url: broker(t), topo: tt.top}
	d1.wrap = func(p Publisher) Publisher {
		return pubFunc(func(c context.Context, ex, key string, msg mq.Message) error {
			err := p.Publish(c, ex, key, msg)
			if err == nil && ok.Add(1) == 5 {
				d1.killTCP() // giua lo 1 (lo 10 row)
				cancel1()    // tien trinh relay chet
			}
			return err
		})
	}
	cfg := itConfig(tt.exchange)
	r1, err := NewRelay(pool, d1.dial, cfg)
	if err != nil {
		t.Fatalf("NewRelay 1: %v", err)
	}
	done := make(chan error, 1)
	go func() { done <- r1.Run(ctx1) }()
	select {
	case <-done:
	case <-time.After(60 * time.Second):
		t.Fatal("relay 1 khong dung")
	}
	if n := countWhere(t, pool, "published_at IS NULL"); n == 0 {
		t.Fatal("tien de sai: relay 1 da gui het truoc khi chet")
	}

	cfg2 := itConfig(tt.exchange)
	d2 := &brokerDialer{url: broker(t), topo: tt.top}
	r2, err := NewRelay(pool, d2.dial, cfg2)
	if err != nil {
		t.Fatalf("NewRelay 2: %v", err)
	}
	stop := runRelay(t, r2)
	eventually(t, 60*time.Second, "moi row co published_at", func() bool {
		return countWhere(t, pool, "published_at IS NULL") == 0
	})
	stop()
	assertAtLeastOnce(t, ids, countByEventID(t, drain(t, tt.queue)), cfg.BatchSize)
}

// --- AC-4 + R2: da publish nhung chua danh dau (cat ket noi DB cua relay) ---

// deadDB boc pool cua relay: sau khi `dead` bat, moi thao tac DB loi (tien trinh da chet).
type deadDB struct {
	pool *pgxpool.Pool
	dead atomic.Bool
}

var errDead = errors.New("relay da chet")

func (d *deadDB) Begin(ctx context.Context) (pgx.Tx, error) {
	if d.dead.Load() {
		return nil, errDead
	}
	return d.pool.Begin(ctx)
}

func (d *deadDB) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	if d.dead.Load() {
		return pgconn.CommandTag{}, errDead
	}
	return d.pool.Exec(ctx, sql, args...)
}

func (d *deadDB) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	if d.dead.Load() {
		return nil, errDead
	}
	return d.pool.Query(ctx, sql, args...)
}

func (d *deadDB) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	if d.dead.Load() {
		return errRow{errDead}
	}
	return d.pool.QueryRow(ctx, sql, args...)
}

func TestTichHop_AC4_R2_DaPublishChuaDanhDauThiGuiLai(t *testing.T) {
	ctx := context.Background()
	admin := pgtest.New(t)
	tt := newTopo("ticketing.hold.released")
	declare(t, tt)
	ids := writeEvents(t, ctx, admin, 30, "ticketing.hold.released")

	const app = "outbox-relay-r2"
	pcfg, err := pgxpool.ParseConfig(admin.Config().ConnString())
	if err != nil {
		t.Fatalf("parse config: %v", err)
	}
	pcfg.ConnConfig.RuntimeParams["application_name"] = app
	relayPool, err := pgxpool.NewWithConfig(ctx, pcfg)
	if err != nil {
		t.Fatalf("pool relay: %v", err)
	}
	defer relayPool.Close()
	db := &deadDB{pool: relayPool}

	ctx1, cancel1 := context.WithCancel(ctx)
	defer cancel1()
	var ok atomic.Int32
	var lastSent atomic.Value
	d1 := &brokerDialer{url: broker(t), topo: tt.top}
	d1.wrap = func(p Publisher) Publisher {
		return pubFunc(func(c context.Context, ex, key string, msg mq.Message) error {
			err := p.Publish(c, ex, key, msg)
			if err == nil && ok.Add(1) == 5 {
				// Broker da ack message thu 5; cat ket noi DB cua relay truoc khi no kip danh dau.
				if _, terr := admin.Exec(context.Background(),
					`SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE application_name = $1`, app); terr != nil {
					t.Errorf("pg_terminate_backend: %v", terr)
				}
				db.dead.Store(true)
				lastSent.Store(msg.MessageID)
				cancel1()
			}
			return err
		})
	}
	cfg := itConfig(tt.exchange)
	r1, err := NewRelay(db, d1.dial, cfg)
	if err != nil {
		t.Fatalf("NewRelay 1: %v", err)
	}
	done := make(chan error, 1)
	go func() { done <- r1.Run(ctx1) }()
	select {
	case <-done:
	case <-time.After(60 * time.Second):
		t.Fatal("relay 1 khong dung")
	}
	sent, _ := lastSent.Load().(string)
	if sent == "" {
		t.Fatal("tien de sai: chua publish du 5 message")
	}
	if n := countWhere(t, admin, "event_id = $1 AND published_at IS NULL", sent); n != 1 {
		t.Fatalf("tien de sai: message %s da toi broker phai CHUA duoc danh dau", sent)
	}

	// Relay moi (pool moi, lease ngan): sau khi lease het han, gui lai phan chua danh dau.
	r2, err := NewRelay(admin, (&brokerDialer{url: broker(t), topo: tt.top}).dial, itConfig(tt.exchange))
	if err != nil {
		t.Fatalf("NewRelay 2: %v", err)
	}
	stop := runRelay(t, r2)
	eventually(t, 60*time.Second, "moi row co published_at", func() bool {
		return countWhere(t, admin, "published_at IS NULL") == 0
	})
	stop()
	got := countByEventID(t, drain(t, tt.queue))
	if got[sent] < 1 {
		t.Errorf("message %s (da publish, chua danh dau) khong co trong queue", sent)
	}
	assertAtLeastOnce(t, ids, got, cfg.BatchSize)
}

// --- AC-4: RabbitMQ stop_app/start_app khi relay dang chay ---

func TestTichHop_AC4_RabbitMQDungRoiChayLaiKhongMatHoldReleased(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.New(t)
	tt := newTopo("ticketing.hold.released")
	declare(t, tt)
	d := &brokerDialer{url: broker(t), topo: tt.top}
	r, err := NewRelay(pool, d.dial, itConfig(tt.exchange))
	if err != nil {
		t.Fatalf("NewRelay: %v", err)
	}
	stop := runRelay(t, r)

	ids := writeEvents(t, ctx, pool, 10, "ticketing.hold.released")
	eventually(t, 30*time.Second, "lo dau duoc gui", func() bool {
		return countWhere(t, pool, "published_at IS NULL") == 0
	})

	rabbitctl(t, "rabbitmqctl", "stop_app")
	stopped := true
	t.Cleanup(func() {
		if stopped {
			rabbitctl(t, "rabbitmqctl", "start_app")
		}
	})
	ids = append(ids, writeEvents(t, ctx, pool, 10, "ticketing.hold.released")...)
	time.Sleep(3 * time.Second)
	if n := countWhere(t, pool, "published_at IS NULL"); n != 10 {
		t.Errorf("broker dung ma %d/10 row moi da duoc danh dau gui", 10-n)
	}
	rabbitctl(t, "rabbitmqctl", "start_app")
	stopped = false
	rabbitctl(t, "rabbitmqctl", "await_startup")
	ids = append(ids, writeEvents(t, ctx, pool, 5, "ticketing.hold.released")...)

	eventually(t, 120*time.Second, "relay tu dial lai va gui het", func() bool {
		return countWhere(t, pool, "published_at IS NULL") == 0
	})
	stop()
	if n := countWhere(t, pool, "next_attempt_at = 'infinity'"); n != 0 {
		t.Errorf("%d row bi do vi broker dung (R1: ErrClosed/dial loi khong tinh attempts)", n)
	}
	if n := countWhere(t, pool, "attempts > 0"); n != 0 {
		t.Logf("canh bao: %d row co attempts > 0 sau su co broker", n)
	}
	assertAtLeastOnce(t, ids, countByEventID(t, drain(t, tt.queue)), itConfig("").BatchSize)
}

// --- AC-5: trace xuyen suot ---

func TestTichHop_AC5_TraceIDGiuNguyenTuGhiOutboxToiConsumer(t *testing.T) {
	newRecorder(t)
	pool := pgtest.New(t)
	tt := newTopo("ticketing.hold.released")
	declare(t, tt)

	root, span := otel.Tracer("test").Start(context.Background(), "tao-hold")
	want := span.SpanContext().TraceID()
	tx, err := pool.Begin(root)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	id, err := Write(root, tx, Event{AggregateType: "hold", AggregateID: newUUID(),
		RoutingKey: "ticketing.hold.released", Data: map[string]string{"hold_id": "x"}})
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := tx.Commit(root); err != nil {
		t.Fatalf("commit: %v", err)
	}
	span.End()

	var traceCol string
	if err := pool.QueryRow(context.Background(), `SELECT coalesce(trace_id, '') FROM outbox_events WHERE event_id = $1`, id).Scan(&traceCol); err != nil {
		t.Fatalf("doc trace_id: %v", err)
	}
	if traceCol != want.String() {
		t.Errorf("cot trace_id = %q, muon %s", traceCol, want)
	}

	r, err := NewRelay(pool, (&brokerDialer{url: broker(t), topo: tt.top}).dial, itConfig(tt.exchange))
	if err != nil {
		t.Fatalf("NewRelay: %v", err)
	}
	if _, err := r.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	ds := drain(t, tt.queue)
	if len(ds) != 1 {
		t.Fatalf("queue co %d message, muon 1", len(ds))
	}
	var env Envelope
	_ = json.Unmarshal(ds[0].Body, &env)
	if env.TraceID != want.String() {
		t.Errorf("envelope.trace_id = %q, muon %s", env.TraceID, want)
	}
	cctx := otelx.ExtractAMQP(context.Background(), otelx.AMQPHeaders(ds[0].Headers))
	_, cspan := otel.Tracer("consumer").Start(cctx, "consume", trace.WithSpanKind(trace.SpanKindConsumer))
	defer cspan.End()
	if got := cspan.SpanContext().TraceID(); got != want {
		t.Errorf("span consumer trace %s, muon %s (trace ghi outbox)", got, want)
	}
}

// --- AC-6: unroutable theo danh sach ---

func TestTichHop_AC6_UnroutableChiBoKeyTrongDanhSach(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.New(t)
	tt := newTopo() // exchange + queue, KHONG binding nao
	declare(t, tt)
	created := writeEvents(t, ctx, pool, 1, "ticketing.hold.created")[0]
	released := writeEvents(t, ctx, pool, 1, "ticketing.hold.released")[0]

	cfg := itConfig(tt.exchange)
	r, err := NewRelay(pool, (&brokerDialer{url: broker(t), topo: tt.top}).dial, cfg)
	if err != nil {
		t.Fatalf("NewRelay: %v", err)
	}
	if _, err := r.RunOnce(ctx); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if n := countWhere(t, pool, "event_id = $1 AND published_at IS NOT NULL AND last_error = 'unroutable'", created); n != 1 {
		t.Error("ticketing.hold.created (trong DropUnroutable) phai co published_at va last_error = 'unroutable'")
	}
	if n := countWhere(t, pool, `event_id = $1 AND published_at IS NULL AND attempts = 1
		AND last_error IS NOT NULL AND next_attempt_at > now()`, released); n != 1 {
		t.Error("ticketing.hold.released (ngoai danh sach) phai la loi tam: chua gui, attempts = 1, co last_error, lui lich")
	}
	if v, ok := metricValue(t, gatherer(t, cfg), "outbox_unroutable_total",
		map[string]string{"routing_key": "ticketing.hold.created"}); !ok || v < 1 {
		t.Errorf("outbox_unroutable_total{ticketing.hold.created} = %v (co %v)", v, ok)
	}

	// Sua binding -> row bi lui lich duoc gui khi toi han.
	ch, err := amqpConn(t).Channel()
	if err != nil {
		t.Fatal(err)
	}
	if err := ch.QueueBind(tt.queue, "ticketing.hold.released", tt.exchange, false, nil); err != nil {
		t.Fatalf("bind: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE outbox_events SET next_attempt_at = now() WHERE event_id = $1`, released); err != nil {
		t.Fatal(err)
	}
	if _, err := r.RunOnce(ctx); err != nil {
		t.Fatalf("RunOnce sau khi sua binding: %v", err)
	}
	if n := countWhere(t, pool, "event_id = $1 AND published_at IS NOT NULL", released); n != 1 {
		t.Error("sau khi sua binding, event phai duoc gui")
	}
	assertAtLeastOnce(t, []string{released}, countByEventID(t, drain(t, tt.queue)), 0)
}

// --- SKIP LOCKED + lease: hai relay song song khong gui trung ---

func TestTichHop_HaiRelaySongSongKhongGuiTrungTrongMotLease(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.New(t)
	tt := newTopo("ticketing.hold.released")
	declare(t, tt)
	ids := writeEvents(t, ctx, pool, 200, "ticketing.hold.released")

	var counts [2]atomic.Int32
	for i := 0; i < 2; i++ {
		i := i
		d := &brokerDialer{url: broker(t), topo: tt.top}
		d.wrap = func(p Publisher) Publisher {
			return pubFunc(func(c context.Context, ex, key string, msg mq.Message) error {
				time.Sleep(5 * time.Millisecond)
				err := p.Publish(c, ex, key, msg)
				if err == nil {
					counts[i].Add(1)
				}
				return err
			})
		}
		cfg := itConfig(tt.exchange)
		cfg.Lease = time.Minute
		cfg.BatchDeadline = 30 * time.Second
		cfg.PublishTimeout = 5 * time.Second
		r, err := NewRelay(pool, d.dial, cfg)
		if err != nil {
			t.Fatalf("NewRelay %d: %v", i, err)
		}
		runRelay(t, r)
	}
	eventually(t, 60*time.Second, "moi row co published_at", func() bool {
		return countWhere(t, pool, "published_at IS NULL") == 0
	})
	if counts[0].Load() == 0 || counts[1].Load() == 0 {
		t.Errorf("ca hai relay phai cung lam viec (SKIP LOCKED): %d / %d", counts[0].Load(), counts[1].Load())
	}
	got := countByEventID(t, drain(t, tt.queue))
	for id, n := range got {
		if n != 1 {
			t.Errorf("event %s duoc gui %d lan trong mot lease", id, n)
		}
	}
	assertAtLeastOnce(t, ids, got, 0)
}

// --- AC-8: migration 0002 ---

// insertKieuCu la dung cau INSERT cua repository.go truoc EVF-36 (code cu phai chay duoc tren schema 0002).
const insertKieuCu = `
		INSERT INTO outbox_events (aggregate_type, aggregate_id, routing_key, payload)
		VALUES ('order', $1::uuid, 'ticketing.hold.created', $2)`

func TestTichHop_AC8_InsertKieuCuChayTrenSchema0002VaNhanEventID(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.NewEmpty(t)
	pgtest.ApplyUp(t, pool, pgtest.MigrationFile(t, "0001"))
	if _, err := pool.Exec(ctx, insertKieuCu, newUUID(), []byte(`{"event_id":"cu-truoc-0002"}`)); err != nil {
		t.Fatalf("insert tren 0001: %v", err)
	}
	pgtest.ApplyUp(t, pool, pgtest.MigrationFile(t, "0002"))
	if _, err := pool.Exec(ctx, insertKieuCu, newUUID(), []byte(`{"event_id":"cu-sau-0002"}`)); err != nil {
		t.Fatalf("INSERT kieu cu tren schema 0002 phai chay duoc (rollback chi la rut app): %v", err)
	}
	if n := countWhere(t, pool, "event_id IS NULL"); n != 0 {
		t.Errorf("%d row khong co event_id", n)
	}
	if n := countWhere(t, pool, `headers = '{}'::jsonb AND attempts = 0 AND last_error IS NULL
		AND next_attempt_at <= now() AND published_at IS NULL`); n != 2 {
		t.Errorf("chi %d/2 row co gia tri mac dinh dung", n)
	}
	var distinct int
	if err := pool.QueryRow(ctx, `SELECT count(DISTINCT event_id) FROM outbox_events`).Scan(&distinct); err != nil {
		t.Fatal(err)
	}
	if distinct != 2 {
		t.Errorf("event_id khong duy nhat: %d gia tri cho 2 row", distinct)
	}
	if _, err := pool.Exec(ctx, `UPDATE outbox_events SET event_id = (SELECT event_id FROM outbox_events LIMIT 1)`); err == nil {
		t.Error("rang buoc UNIQUE(event_id) khong co hieu luc")
	}
}

type schemaSnap struct {
	cols, indexes, constraints []string
}

func snapshot(t *testing.T, pool *pgxpool.Pool) schemaSnap {
	t.Helper()
	q := func(sql string) []string {
		rows, err := pool.Query(context.Background(), sql)
		if err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
		out, err := pgx.CollectRows(rows, pgx.RowTo[string])
		if err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
		sort.Strings(out)
		return out
	}
	return schemaSnap{
		cols: q(`SELECT column_name || ' ' || data_type || ' ' || is_nullable || ' ' || coalesce(column_default, '-')
			FROM information_schema.columns WHERE table_schema = 'public' AND table_name = 'outbox_events'`),
		indexes: q(`SELECT indexname || ' ' || indexdef FROM pg_indexes
			WHERE schemaname = 'public' AND tablename = 'outbox_events'`),
		constraints: q(`SELECT conname || ' ' || contype || ' ' || pg_get_constraintdef(oid) FROM pg_constraint
			WHERE conrelid = 'public.outbox_events'::regclass`),
	}
}

func (s schemaSnap) String() string {
	return "cot:\n  " + strings.Join(s.cols, "\n  ") + "\nindex:\n  " + strings.Join(s.indexes, "\n  ") +
		"\nconstraint:\n  " + strings.Join(s.constraints, "\n  ")
}

func TestTichHop_AC8_UpDownUpTraDungSchema0001(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.NewEmpty(t)
	f1, f2 := pgtest.MigrationFile(t, "0001"), pgtest.MigrationFile(t, "0002")
	pgtest.ApplyUp(t, pool, f1)
	s1 := snapshot(t, pool)
	for i := 0; i < 3; i++ {
		if _, err := pool.Exec(ctx, insertKieuCu, newUUID(), []byte(`{"i":1}`)); err != nil {
			t.Fatal(err)
		}
	}

	pgtest.ApplyUp(t, pool, f2)
	s2 := snapshot(t, pool)
	if s2.String() == s1.String() {
		t.Fatal("Up 0002 khong doi schema outbox_events")
	}
	for _, want := range []string{"event_id uuid NO", "headers jsonb NO", "attempts integer NO",
		"last_error text YES", "next_attempt_at timestamp with time zone NO"} {
		if !strings.Contains(strings.Join(s2.cols, "\n"), want) {
			t.Errorf("sau Up 0002 thieu cot %q:\n%s", want, s2)
		}
	}
	if !strings.Contains(strings.Join(s2.indexes, "\n"), "idx_outbox_due") ||
		strings.Contains(strings.Join(s2.indexes, "\n"), "idx_outbox_unpublished") {
		t.Errorf("sau Up 0002 phai co idx_outbox_due va khong con idx_outbox_unpublished:\n%s", s2)
	}

	pgtest.ApplyDown(t, pool, f2)
	if got := snapshot(t, pool); got.String() != s1.String() {
		t.Errorf("Down 0002 khong tra dung schema 0001.\nsau Down:\n%s\n\nschema 0001:\n%s", got, s1)
	}
	if n := countWhere(t, pool, "true"); n != 3 {
		t.Errorf("Down lam mat du lieu: con %d/3 row", n)
	}

	pgtest.ApplyUp(t, pool, f2)
	if got := snapshot(t, pool); got.String() != s2.String() {
		t.Errorf("Up lan 2 khac Up lan 1.\nlan 2:\n%s\n\nlan 1:\n%s", got, s2)
	}
	if n := countWhere(t, pool, "event_id IS NOT NULL"); n != 3 {
		t.Errorf("sau Up lan 2 chi %d/3 row co event_id", n)
	}
}

// M7: lock_timeout trong transaction -> migration bo cuoc nhanh thay vi xep hang chan ca he thong.
func TestTichHop_AC8_Migration0002BoCuocKhiBangDangBiKhoa(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.NewEmpty(t)
	f2 := pgtest.MigrationFile(t, "0002")
	pgtest.ApplyUp(t, pool, pgtest.MigrationFile(t, "0001"))

	lock, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := lock.Exec(ctx, `LOCK TABLE outbox_events IN ACCESS SHARE MODE`); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	err = pgtest.Apply(pool, f2, true)
	el := time.Since(start)
	_ = lock.Rollback(ctx)
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "55P03" {
		t.Fatalf("Up khi bang bi khoa = %v, muon lock_not_available (55P03)", err)
	}
	if el > 15*time.Second {
		t.Errorf("migration cho %v moi bo cuoc; lock_timeout 5s khong co hieu luc", el)
	}
	if err := pgtest.Apply(pool, f2, true); err != nil {
		t.Fatalf("Up sau khi nha khoa: %v", err)
	}
}

// M3: row ghi truoc 0002 (payload tho, event_id trong data la id buoi dien) van duoc boc dung phong bi.
func TestTichHop_AC8_RowCuTruoc0002DuocBocPhongBi(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.NewEmpty(t)
	pgtest.ApplyUp(t, pool, pgtest.MigrationFile(t, "0001"))
	concert := newUUID()
	payload := fmt.Sprintf(`{"order_id":%q,"event_id":%q,"quantity":2}`, newUUID(), concert)
	if _, err := pool.Exec(ctx, insertKieuCu, newUUID(), []byte(payload)); err != nil {
		t.Fatal(err)
	}
	pgtest.ApplyUp(t, pool, pgtest.MigrationFile(t, "0002"))
	var eventID string
	if err := pool.QueryRow(ctx, `SELECT event_id::text FROM outbox_events`).Scan(&eventID); err != nil {
		t.Fatal(err)
	}

	tt := newTopo("ticketing.hold.created")
	declare(t, tt)
	r, err := NewRelay(pool, (&brokerDialer{url: broker(t), topo: tt.top}).dial, itConfig(tt.exchange))
	if err != nil {
		t.Fatalf("NewRelay: %v", err)
	}
	if _, err := r.RunOnce(ctx); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	ds := drain(t, tt.queue)
	if len(ds) != 1 {
		t.Fatalf("queue co %d message, muon 1", len(ds))
	}
	var env Envelope
	if err := json.Unmarshal(ds[0].Body, &env); err != nil {
		t.Fatalf("body khong phai phong bi: %v", err)
	}
	if env.EventID != eventID || env.EventID == concert || ds[0].MessageId != eventID {
		t.Errorf("event_id phong bi %q / MessageId %q, muon cot event_id %q (khong phai id buoi dien %q)",
			env.EventID, ds[0].MessageId, eventID, concert)
	}
	if !jsonEqual(t, env.Data, []byte(payload)) || env.Type != "ticketing.hold.created" || env.SchemaVersion != 1 {
		t.Errorf("phong bi sai: %+v", env)
	}
}
