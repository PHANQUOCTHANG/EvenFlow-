package outbox

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"

	"github.com/eventflow/eventflow/libs/go/mq"
)

// Fake dung chung cho unit test cua relay (khong can DB/broker that).
//
// fakeDB dong vai *pgxpool.Pool o muc giao thuc SQL: no ghi lai moi cau lenh, phan loai theo noi dung
// (xem classify) va giu trang thai tung row. Hop dong SQL ma fake (va do do implementer) dua vao — lay tu
// plan §6.2, §6.4, §7:
//
//   - Claim: mot cau chua `FOR UPDATE SKIP LOCKED` (trong Begin/tx hoac goi thang), tra cac cot qua
//     `RETURNING <cot>, ...` — fake doc danh sach cot trong RETURNING va tra dung thu tu do (cot co the co
//     `::text`). Cot ho tro: id, event_id, aggregate_type, aggregate_id, routing_key, payload, headers,
//     trace_id, created_at, attempts, last_error, next_attempt_at, published_at.
//   - Moi cau danh dau la mot `UPDATE outbox_events ...` rieng (Exec tren DB, khong SendBatch), dieu kien
//     row la `id = $n` hoac `id = ANY($n)`, va CO `published_at IS NULL` trong WHERE:
//   - da gui:        SET published_at = now() ... (last_error NULL; unroutable: chuoi 'unroutable' trong
//     SQL hoac trong tham so)
//   - thu lai:       SET attempts = attempts + 1, last_error = $x, next_attempt_at = now() + ...
//   - do (park):     SET ... next_attempt_at = 'infinity' ... (hang so trong SQL; quyet dinh do nam o Go)
//   - tra lease:     SET next_attempt_at = now() WHERE id = ANY($1) AND published_at IS NULL
//     AND next_attempt_at <> 'infinity' (plan §8 m1: tra lease khong duoc go do row da bi relay khac do)
//   - Cau doc so lieu (count(*)) di qua QueryRow; fake tra 0.
//   - Moi cau dung ctx da huy thi fake tra ctx.Err() (nhu pgx) — nen danh dau phai chay duoi
//     context.WithoutCancel.

type stmtKind int

const (
	kindOther stmtKind = iota
	kindClaim
	kindPublished
	kindRetry
	kindPark
	kindRelease
)

func (k stmtKind) String() string {
	return [...]string{"khac", "claim", "da-gui", "thu-lai", "do", "tra-lease"}[k]
}

// normSQL chuan hoa SQL de so khop: chu thuong, gop khoang trang, bo khoang trang quanh toan tu.
func normSQL(s string) string {
	s = strings.ToLower(s)
	s = strings.Join(strings.Fields(s), " ")
	for _, op := range []string{"=", "+", "(", ")", ",", "<", ">"} {
		s = strings.ReplaceAll(s, " "+op, op)
		s = strings.ReplaceAll(s, op+" ", op)
	}
	return s
}

func classify(n string) stmtKind {
	switch {
	case strings.Contains(n, "skip locked"):
		return kindClaim
	case !strings.HasPrefix(n, "update"):
		return kindOther
	case strings.Contains(n, "published_at=now()"):
		return kindPublished
	case strings.Contains(n, "next_attempt_at='infinity'"):
		return kindPark
	case strings.Contains(n, "attempts=attempts+1"):
		return kindRetry
	case strings.Contains(n, "next_attempt_at=now()"):
		return kindRelease
	}
	return kindOther
}

var idParamRe = regexp.MustCompile(`\bid=(?:any\()?\$(\d+)`)

// rowIDs lay id row tu tham so duoc tham chieu boi `id = $n` / `id = ANY($n)`.
func rowIDs(n string, args []any) []int64 {
	var out []int64
	for _, m := range idParamRe.FindAllStringSubmatch(n, -1) {
		i, _ := strconv.Atoi(m[1])
		if i < 1 || i > len(args) {
			continue
		}
		out = append(out, toInt64s(args[i-1])...)
	}
	return out
}

func toInt64s(v any) []int64 {
	switch x := v.(type) {
	case int64:
		return []int64{x}
	case int:
		return []int64{int64(x)}
	case int32:
		return []int64{int64(x)}
	case []int64:
		return x
	case []int:
		out := make([]int64, len(x))
		for i, y := range x {
			out[i] = int64(y)
		}
		return out
	case []int32:
		out := make([]int64, len(x))
		for i, y := range x {
			out[i] = int64(y)
		}
		return out
	case pgtype.Int8:
		return []int64{x.Int64}
	}
	return nil
}

func stringArgs(args []any) []string {
	var out []string
	for _, a := range args {
		switch x := a.(type) {
		case string:
			out = append(out, x)
		case *string:
			if x != nil {
				out = append(out, *x)
			}
		case pgtype.Text:
			if x.Valid {
				out = append(out, x.String)
			}
		case []byte:
			out = append(out, string(x))
		}
	}
	return out
}

type stmt struct {
	sql  string // da chuan hoa
	kind stmtKind
	args []any
	ids  []int64
	inTx bool
}

// fakeRow la mot row outbox_events trong fake.
type fakeRow struct {
	ID            int64
	EventID       string
	AggregateType string
	AggregateID   string
	RoutingKey    string
	Payload       string // JSON
	Headers       string // JSON
	TraceID       *string
	CreatedAt     time.Time
	Attempts      int
}

type rowState struct {
	due        bool
	published  bool
	unroutable bool
	parked     bool
	retries    int
	releases   int
	errs       []string
}

type fakeDB struct {
	t        *testing.T
	mu       sync.Mutex
	rows     []fakeRow
	st       map[int64]*rowState
	stmts    []stmt
	claims   int
	commits  int
	failMark func(kind stmtKind, ids []int64) error // khac nil: tiem loi cho cau danh dau
}

func newFakeDB(t *testing.T, rows ...fakeRow) *fakeDB {
	db := &fakeDB{t: t, rows: rows, st: map[int64]*rowState{}}
	for _, r := range rows {
		db.st[r.ID] = &rowState{due: true}
	}
	return db
}

func (db *fakeDB) Begin(ctx context.Context) (pgx.Tx, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return &fakeTx{db: db}, nil
}

func (db *fakeDB) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	return db.exec(ctx, false, sql, args)
}

func (db *fakeDB) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	return db.query(ctx, false, sql, args)
}

func (db *fakeDB) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	return db.queryRow(ctx, false, sql, args)
}

func (db *fakeDB) record(inTx bool, sql string, args []any) stmt {
	n := normSQL(sql)
	s := stmt{sql: n, kind: classify(n), args: args, inTx: inTx}
	s.ids = rowIDs(n, args)
	db.stmts = append(db.stmts, s)
	return s
}

func (db *fakeDB) exec(ctx context.Context, inTx bool, sql string, args []any) (pgconn.CommandTag, error) {
	if err := ctx.Err(); err != nil {
		return pgconn.CommandTag{}, err
	}
	db.mu.Lock()
	defer db.mu.Unlock()
	s := db.record(inTx, sql, args)
	switch s.kind {
	case kindOther:
		return pgconn.NewCommandTag("SET"), nil
	case kindClaim:
		db.t.Errorf("claim phai doc ket qua (Query), khong dung Exec: %s", s.sql)
		return pgconn.NewCommandTag("UPDATE 0"), nil
	}
	if db.failMark != nil {
		if err := db.failMark(s.kind, s.ids); err != nil {
			return pgconn.CommandTag{}, err
		}
	}
	if len(s.ids) == 0 {
		db.t.Errorf("cau danh dau khong co dieu kien id = $n / id = ANY($n): %s", s.sql)
	}
	errs := stringArgs(args)
	affected := 0
	for _, id := range s.ids {
		r := db.st[id]
		if r == nil || r.published { // AND published_at IS NULL
			continue
		}
		affected++
		r.errs = append(r.errs, errs...)
		switch s.kind {
		case kindPublished:
			r.published = true
			r.unroutable = strings.Contains(s.sql, "'unroutable'") || contains(errs, "unroutable")
		case kindRetry:
			r.retries++
		case kindPark:
			r.parked = true
			if strings.Contains(s.sql, "attempts=attempts+1") {
				r.retries++
			}
		case kindRelease:
			if r.parked {
				if strings.Contains(s.sql, "next_attempt_at<>'infinity'") {
					affected--
					continue // dieu kien SQL loai row da do
				}
				r.parked = false // SQL thieu dieu kien: go do nham
			}
			r.releases++
			r.due = true
		}
	}
	return pgconn.NewCommandTag(fmt.Sprintf("UPDATE %d", affected)), nil
}

func contains(ss []string, sub string) bool {
	for _, s := range ss {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

var limitRe = regexp.MustCompile(`limit \$(\d+)`)

func (db *fakeDB) query(ctx context.Context, inTx bool, sql string, args []any) (pgx.Rows, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	db.mu.Lock()
	defer db.mu.Unlock()
	s := db.record(inTx, sql, args)
	if s.kind != kindClaim {
		return &fakeRows{}, nil
	}
	db.claims++
	limit := -1
	if m := limitRe.FindStringSubmatch(s.sql); m != nil {
		i, _ := strconv.Atoi(m[1])
		if i >= 1 && i <= len(args) {
			if v := toInt64s(args[i-1]); len(v) == 1 {
				limit = int(v[0])
			}
		}
	}
	cols := returningCols(s.sql)
	if len(cols) == 0 {
		db.t.Errorf("claim khong co RETURNING: %s", s.sql)
	}
	out := &fakeRows{cols: cols}
	for _, r := range db.rows {
		st := db.st[r.ID]
		if !st.due || st.published || st.parked {
			continue
		}
		if limit >= 0 && len(out.data) >= limit {
			break
		}
		st.due = false
		out.data = append(out.data, r)
	}
	return out, nil
}

func (db *fakeDB) queryRow(ctx context.Context, inTx bool, sql string, args []any) pgx.Row {
	if err := ctx.Err(); err != nil {
		return errRow{err}
	}
	n := normSQL(sql)
	if classify(n) == kindClaim {
		rows, err := db.query(ctx, inTx, sql, args)
		if err != nil {
			return errRow{err}
		}
		return rowFromRows{rows}
	}
	db.mu.Lock()
	db.record(inTx, sql, args)
	db.mu.Unlock()
	return zeroRow{}
}

// --- truy van trang thai cho test ---

// setParked gia lap mot relay khac vua do row id (next_attempt_at = 'infinity').
func (db *fakeDB) setParked(id int64) {
	db.mu.Lock()
	defer db.mu.Unlock()
	db.st[id].parked = true
	db.st[id].due = false
}

// add them row vao fake (vd. event moi duoc ghi giua hai lo).
func (db *fakeDB) add(r fakeRow) {
	db.mu.Lock()
	defer db.mu.Unlock()
	db.rows = append(db.rows, r)
	db.st[r.ID] = &rowState{due: true}
}

func (db *fakeDB) state(id int64) rowState {
	db.mu.Lock()
	defer db.mu.Unlock()
	return *db.st[id]
}

func (db *fakeDB) claimCount() int {
	db.mu.Lock()
	defer db.mu.Unlock()
	return db.claims
}

func (db *fakeDB) all() []stmt {
	db.mu.Lock()
	defer db.mu.Unlock()
	return append([]stmt(nil), db.stmts...)
}

// assertMarksGuarded: moi cau danh dau phai co `published_at IS NULL` (plan §7 r1, R1).
func assertMarksGuarded(t *testing.T, db *fakeDB) {
	t.Helper()
	for _, s := range db.all() {
		switch s.kind {
		case kindPublished, kindRetry, kindPark, kindRelease:
			where := s.sql
			if i := strings.Index(where, " where "); i >= 0 {
				where = where[i:]
			}
			if !strings.Contains(where, "published_at is null") {
				t.Errorf("cau %s thieu `AND published_at IS NULL` trong WHERE: %s", s.kind, s.sql)
			}
			if s.kind == kindRelease && !strings.Contains(where, "next_attempt_at<>'infinity'") {
				t.Errorf("cau tra lease thieu `AND next_attempt_at <> 'infinity'` (plan §8 m1): %s", s.sql)
			}
		}
	}
}

// --- fakeTx ---

// fakeTx nhung pgx.Tx (nil) — chi cac phuong thuc relay can duoc hien thuc.
type fakeTx struct {
	pgx.Tx
	db   *fakeDB
	done bool
}

func (tx *fakeTx) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	return tx.db.exec(ctx, true, sql, args)
}

func (tx *fakeTx) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	return tx.db.query(ctx, true, sql, args)
}

func (tx *fakeTx) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	return tx.db.queryRow(ctx, true, sql, args)
}

func (tx *fakeTx) Commit(ctx context.Context) error {
	if tx.done {
		return pgx.ErrTxClosed
	}
	tx.done = true
	tx.db.mu.Lock()
	tx.db.commits++
	tx.db.mu.Unlock()
	return nil
}

func (tx *fakeTx) Rollback(context.Context) error {
	if tx.done {
		return pgx.ErrTxClosed
	}
	tx.done = true
	return nil
}

// --- fakeRows ---

var returningRe = regexp.MustCompile(`returning (.*)$`)

func returningCols(n string) []string {
	m := returningRe.FindStringSubmatch(n)
	if m == nil {
		return nil
	}
	var cols []string
	for _, part := range strings.Split(m[1], ",") {
		part = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(part), ";"))
		if i := strings.LastIndex(part, " as "); i >= 0 {
			part = part[:i]
		}
		if i := strings.LastIndex(part, "."); i >= 0 {
			part = part[i+1:]
		}
		cols = append(cols, part)
	}
	return cols
}

type fakeRows struct {
	cols   []string
	data   []fakeRow
	i      int
	closed bool
}

var typeMap = pgtype.NewMap()

func pgTime(t time.Time) string { return t.UTC().Format("2006-01-02 15:04:05.999999-07") }

// cell tra OID va gia tri dang text cua cot col (co the co ::text).
func cell(r fakeRow, col string) (uint32, []byte, string) {
	base, cast, _ := strings.Cut(col, "::")
	name := base
	var oid uint32
	var val []byte
	switch base {
	case "id":
		oid, val = pgtype.Int8OID, []byte(strconv.FormatInt(r.ID, 10))
	case "event_id":
		oid, val = pgtype.UUIDOID, []byte(r.EventID)
	case "aggregate_type":
		oid, val = pgtype.TextOID, []byte(r.AggregateType)
	case "aggregate_id":
		oid, val = pgtype.UUIDOID, []byte(r.AggregateID)
	case "routing_key":
		oid, val = pgtype.TextOID, []byte(r.RoutingKey)
	case "payload":
		oid, val = pgtype.JSONBOID, []byte(r.Payload)
	case "headers":
		oid, val = pgtype.JSONBOID, []byte(r.Headers)
	case "trace_id":
		oid = pgtype.TextOID
		if r.TraceID != nil {
			val = []byte(*r.TraceID)
		}
	case "created_at":
		oid, val = pgtype.TimestamptzOID, []byte(pgTime(r.CreatedAt))
	case "attempts":
		oid, val = pgtype.Int4OID, []byte(strconv.Itoa(r.Attempts))
	case "last_error", "published_at":
		oid = pgtype.TextOID
		if base == "published_at" {
			oid = pgtype.TimestamptzOID
		}
	case "next_attempt_at":
		oid, val = pgtype.TimestamptzOID, []byte(pgTime(time.Now()))
	default:
		oid = pgtype.TextOID
		val = []byte("?" + base)
	}
	if cast == "text" {
		oid = pgtype.TextOID
	}
	return oid, val, name
}

func (r *fakeRows) Close()     { r.closed = true }
func (r *fakeRows) Err() error { return nil }
func (r *fakeRows) CommandTag() pgconn.CommandTag {
	return pgconn.NewCommandTag("UPDATE " + strconv.Itoa(len(r.data)))
}
func (r *fakeRows) Conn() *pgx.Conn { return nil }

func (r *fakeRows) FieldDescriptions() []pgconn.FieldDescription {
	fds := make([]pgconn.FieldDescription, len(r.cols))
	for i, c := range r.cols {
		oid, _, name := cell(fakeRow{}, c)
		fds[i] = pgconn.FieldDescription{Name: name, DataTypeOID: oid, Format: pgtype.TextFormatCode}
	}
	return fds
}

func (r *fakeRows) Next() bool {
	if r.closed || r.i >= len(r.data) {
		r.closed = true
		return false
	}
	r.i++
	return true
}

func (r *fakeRows) cur() fakeRow { return r.data[r.i-1] }

func (r *fakeRows) Scan(dest ...any) error {
	if len(dest) != len(r.cols) {
		return fmt.Errorf("fake: Scan %d dich cho %d cot RETURNING %v", len(dest), len(r.cols), r.cols)
	}
	row := r.cur()
	for i, c := range r.cols {
		if dest[i] == nil {
			continue
		}
		oid, val, _ := cell(row, c)
		if err := typeMap.Scan(oid, pgtype.TextFormatCode, val, dest[i]); err != nil {
			return fmt.Errorf("fake: scan cot %s vao %T: %w", c, dest[i], err)
		}
	}
	return nil
}

func (r *fakeRows) Values() ([]any, error) {
	row := r.cur()
	out := make([]any, len(r.cols))
	for i, c := range r.cols {
		oid, val, _ := cell(row, c)
		var v any
		if err := typeMap.Scan(oid, pgtype.TextFormatCode, val, &v); err != nil {
			return nil, err
		}
		out[i] = v
	}
	return out, nil
}

func (r *fakeRows) RawValues() [][]byte {
	row := r.cur()
	out := make([][]byte, len(r.cols))
	for i, c := range r.cols {
		_, out[i], _ = cell(row, c)
	}
	return out
}

type errRow struct{ err error }

func (r errRow) Scan(...any) error { return r.err }

type zeroRow struct{}

func (zeroRow) Scan(dest ...any) error {
	for _, d := range dest {
		if d == nil {
			continue
		}
		if err := typeMap.Scan(pgtype.Int8OID, pgtype.TextFormatCode, []byte("0"), d); err != nil {
			return err
		}
	}
	return nil
}

type rowFromRows struct{ rows pgx.Rows }

func (r rowFromRows) Scan(dest ...any) error {
	defer r.rows.Close()
	if !r.rows.Next() {
		return pgx.ErrNoRows
	}
	return r.rows.Scan(dest...)
}

// --- fake Publisher / Dialer ---

type pubCall struct {
	exchange, key string
	msg           mq.Message
	traceID       trace.TraceID
	deadConn      bool // ket noi (closer) da bao IsClosed() luc publish
}

// fakePublisher ghi lai moi lan Publish; hanh vi theo MessageID (event_id) qua `on`.
type fakePublisher struct {
	mu     sync.Mutex
	calls  []pubCall
	on     map[string]func(ctx context.Context) error
	closer *fakeCloser // ket noi hien tai (de ghi deadConn)
}

func (p *fakePublisher) Publish(ctx context.Context, exchange, key string, msg mq.Message) error {
	p.mu.Lock()
	dead := p.closer != nil && p.closer.IsClosed()
	p.calls = append(p.calls, pubCall{exchange: exchange, key: key, msg: msg,
		traceID: trace.SpanContextFromContext(ctx).TraceID(), deadConn: dead})
	f := p.on[msg.MessageID]
	p.mu.Unlock()
	if f != nil {
		return f(ctx)
	}
	return nil
}

func (p *fakePublisher) snapshot() []pubCall {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]pubCall(nil), p.calls...)
}

func (p *fakePublisher) published(eventID string) int {
	n := 0
	for _, c := range p.snapshot() {
		if c.msg.MessageID == eventID {
			n++
		}
	}
	return n
}

// fakeCloser la io.Closer cua ket noi gia; hien thuc them IsClosed() bool (seam plan §8 m4: relay hoi
// closer ket noi con song truoc moi lan claim).
type fakeCloser struct {
	mu     sync.Mutex
	closes int
	dead   bool
}

func (c *fakeCloser) IsClosed() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.dead
}

// kill danh dau ket noi da chet (broker dong ma relay chua publish gi).
func (c *fakeCloser) kill() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.dead = true
}

func (c *fakeCloser) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closes++
	return nil
}

func (c *fakeCloser) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.closes
}

type fakeDialer struct {
	mu     sync.Mutex
	dials  int
	err    error
	pub    *fakePublisher
	closer *fakeCloser
}

func newFakeDialer() *fakeDialer {
	c := &fakeCloser{}
	return &fakeDialer{pub: &fakePublisher{on: map[string]func(context.Context) error{}, closer: c}, closer: c}
}

func (d *fakeDialer) dial(ctx context.Context) (Publisher, io.Closer, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.dials++
	if d.err != nil {
		return nil, nil, d.err
	}
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	d.closer.mu.Lock()
	d.closer.dead = false // ket noi moi
	d.closer.mu.Unlock()
	return d.pub, d.closer, nil
}

func (d *fakeDialer) dialCount() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.dials
}

// --- cau hinh, du lieu, metric, trace ---

func testConfig() RelayConfig {
	return RelayConfig{
		Exchange:       "ticketing.events",
		BatchSize:      100,
		Interval:       10 * time.Millisecond,
		Lease:          15 * time.Second, // plan §8 m1: BatchDeadline + PublishTimeout + 2*5s < Lease
		BatchDeadline:  time.Second,
		PublishTimeout: 200 * time.Millisecond,
		BaseBackoff:    10 * time.Millisecond,
		MaxBackoff:     100 * time.Millisecond,
		MaxAttempts:    5,
		DropUnroutable: []string{"ticketing.hold.created"},
		Registerer:     prometheus.NewRegistry(),
	}
}

func newTestRelay(t *testing.T, db *fakeDB, d *fakeDialer, cfg RelayConfig) *Relay {
	t.Helper()
	r, err := NewRelay(db, d.dial, cfg)
	if err != nil {
		t.Fatalf("NewRelay: %v", err)
	}
	return r
}

var baseTime = time.Date(2026, 10, 13, 9, 30, 0, 123456000, time.UTC)

// mkRow tao row kieu moi (sau 0002) voi event_id xac dinh tu id.
func mkRow(id int64, key string) fakeRow {
	return fakeRow{
		ID:            id,
		EventID:       fmt.Sprintf("00000000-0000-4000-8000-%012d", id),
		AggregateType: "order",
		AggregateID:   "11111111-1111-4111-8111-111111111111",
		RoutingKey:    key,
		Payload:       fmt.Sprintf(`{"order_id":"11111111-1111-4111-8111-111111111111","n":%d}`, id),
		Headers:       `{}`,
		CreatedAt:     baseTime.Add(time.Duration(id) * time.Second),
	}
}

func metricValue(t *testing.T, g prometheus.Gatherer, name string, labels map[string]string) (float64, bool) {
	t.Helper()
	mfs, err := g.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	for _, mf := range mfs {
		if mf.GetName() != name {
			continue
		}
		var sum float64
		found := false
	metric:
		for _, m := range mf.GetMetric() {
			have := map[string]string{}
			for _, lp := range m.GetLabel() {
				have[lp.GetName()] = lp.GetValue()
			}
			for k, v := range labels {
				if have[k] != v {
					continue metric
				}
			}
			found = true
			sum += value(m)
		}
		return sum, found
	}
	return 0, false
}

func value(m *dto.Metric) float64 {
	switch {
	case m.GetCounter() != nil:
		return m.GetCounter().GetValue()
	case m.GetGauge() != nil:
		return m.GetGauge().GetValue()
	case m.GetUntyped() != nil:
		return m.GetUntyped().GetValue()
	case m.GetHistogram() != nil:
		return float64(m.GetHistogram().GetSampleCount())
	}
	return 0
}

func gatherer(t *testing.T, cfg RelayConfig) prometheus.Gatherer {
	t.Helper()
	g, ok := cfg.Registerer.(prometheus.Gatherer)
	if !ok {
		t.Fatal("Registerer cua test phai la *prometheus.Registry")
	}
	return g
}

// newRecorder dat TracerProvider + propagator TraceContext cho toi het test (chep tu mq/trace_test.go).
func newRecorder(t *testing.T) *tracetest.SpanRecorder {
	t.Helper()
	rec := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(rec))
	prevTP, prevProp := otel.GetTracerProvider(), otel.GetTextMapPropagator()
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{}, propagation.Baggage{}))
	t.Cleanup(func() {
		otel.SetTracerProvider(prevTP)
		otel.SetTextMapPropagator(prevProp)
		_ = tp.Shutdown(context.Background())
	})
	return rec
}

func decodeJSON(t *testing.T, b []byte) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("JSON khong hop le %q: %v", b, err)
	}
	return m
}

func jsonEqual(t *testing.T, a, b []byte) bool {
	t.Helper()
	var x, y any
	if json.Unmarshal(a, &x) != nil || json.Unmarshal(b, &y) != nil {
		return false
	}
	xb, _ := json.Marshal(x)
	yb, _ := json.Marshal(y)
	return string(xb) == string(yb)
}

var errTest = errors.New("loi gia lap")
