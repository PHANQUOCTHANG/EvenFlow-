package outbox

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"math/rand/v2"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/eventflow/eventflow/libs/go/mq"
	"github.com/eventflow/eventflow/libs/go/otelx"
)

const (
	markTimeout     = 5 * time.Second  // moi cau danh dau / tra lease / dem gauge
	gaugeRefresh    = 15 * time.Second // chu ky cap nhat outbox_pending / outbox_dead
	redialBase      = time.Second
	redialMax       = 30 * time.Second
	maxLastErrBytes = 1024
	maxBackoffExp   = 20
	maxAllowedBack  = time.Hour
	maxBatchSize    = 1000
)

// DB la phan cua *pgxpool.Pool ma relay dung.
type DB interface {
	Begin(ctx context.Context) (pgx.Tx, error)
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// Publisher la phan cua *mq.Publisher ma relay dung.
type Publisher interface {
	Publish(ctx context.Context, exchange, routingKey string, msg mq.Message) error
}

// Dialer mo ket noi moi toi broker (dial, mo kenh, khai bao topology, mq.NewPublisher). Closer dong ket noi.
// Loi tra ve khong duoc chua URL ket noi.
type Dialer func(ctx context.Context) (Publisher, io.Closer, error)

// RelayConfig cau hinh relay; gia tri 0 dung mac dinh (xem tung truong).
type RelayConfig struct {
	Exchange       string        // bat buoc
	BatchSize      int           // 0 -> 100; 1..1000
	Interval       time.Duration // 0 -> 500ms: cho giua hai lo khi hang doi trong
	Lease          time.Duration // 0 -> 60s: thoi gian row bi claim truoc khi relay khac duoc lay lai
	BatchDeadline  time.Duration // 0 -> 30s: han chot cua mot lo
	PublishTimeout time.Duration // 0 -> 10s: han chot moi message; PublishTimeout <= BatchDeadline, BatchDeadline + PublishTimeout + 10s < Lease
	BaseBackoff    time.Duration // 0 -> 1s
	MaxBackoff     time.Duration // 0 -> 5m; 0 < BaseBackoff <= MaxBackoff <= 1h
	MaxAttempts    int           // 0 -> 20; vuot thi row bi do
	// DropUnroutable: routing key duoc phep danh dau da gui khi khong co queue nhan.
	DropUnroutable []string
	Registerer     prometheus.Registerer // bat buoc
	Logger         *slog.Logger          // tuy chon; mac dinh slog.Default()
}

func (c *RelayConfig) applyDefaults() error {
	if c.Exchange == "" {
		return errors.New("outbox: thieu Exchange")
	}
	if c.Registerer == nil {
		return errors.New("outbox: thieu Registerer")
	}
	if c.BatchSize < 0 || c.BatchSize > maxBatchSize {
		return fmt.Errorf("outbox: BatchSize %d ngoai 1..%d", c.BatchSize, maxBatchSize)
	}
	if c.Interval < 0 || c.Lease < 0 || c.BatchDeadline < 0 || c.PublishTimeout < 0 || c.BaseBackoff < 0 || c.MaxBackoff < 0 {
		return errors.New("outbox: thoi gian khong duoc am")
	}
	if c.MaxAttempts < 0 {
		return errors.New("outbox: MaxAttempts am")
	}
	def := func(d *time.Duration, v time.Duration) {
		if *d == 0 {
			*d = v
		}
	}
	if c.BatchSize == 0 {
		c.BatchSize = 100
	}
	if c.MaxAttempts == 0 {
		c.MaxAttempts = 20
	}
	def(&c.Interval, 500*time.Millisecond)
	def(&c.Lease, 60*time.Second)
	def(&c.BatchDeadline, 30*time.Second)
	def(&c.PublishTimeout, 10*time.Second)
	def(&c.BaseBackoff, time.Second)
	def(&c.MaxBackoff, 5*time.Minute)
	if c.PublishTimeout > c.BatchDeadline || c.BatchDeadline+c.PublishTimeout+2*markTimeout >= c.Lease {
		return errors.New("outbox: can PublishTimeout <= BatchDeadline va BatchDeadline + PublishTimeout + 2*5s < Lease")
	}
	if c.BaseBackoff > c.MaxBackoff || c.MaxBackoff > maxAllowedBack {
		return errors.New("outbox: can 0 < BaseBackoff <= MaxBackoff <= 1h")
	}
	return nil
}

// Relay doc outbox_events va publish ra RabbitMQ.
type Relay struct {
	db   DB
	dial Dialer
	cfg  RelayConfig
	log  *slog.Logger
	m    *metrics
	drop map[string]struct{}

	mu     sync.Mutex // mot lo tai mot thoi diem
	pub    Publisher
	closer io.Closer
}

// NewRelay tao relay; db va dial bat buoc, cfg duoc kiem tra va dien mac dinh.
func NewRelay(db DB, dial Dialer, cfg RelayConfig) (*Relay, error) {
	if db == nil {
		return nil, errors.New("outbox: db nil")
	}
	if dial == nil {
		return nil, errors.New("outbox: dialer nil")
	}
	if err := cfg.applyDefaults(); err != nil {
		return nil, err
	}
	m, err := newMetrics(cfg.Registerer)
	if err != nil {
		return nil, err
	}
	log := cfg.Logger
	if log == nil {
		log = slog.Default()
	}
	drop := make(map[string]struct{}, len(cfg.DropUnroutable))
	for _, k := range cfg.DropUnroutable {
		drop[k] = struct{}{}
	}
	return &Relay{db: db, dial: dial, cfg: cfg, log: log, m: m, drop: drop}, nil
}

// Run chay vong lap cho toi khi ctx bi huy, roi tra nil. Loi broker / DB khong lam Run thoat:
// dial lai voi backoff 1s..30s (co jitter) va chi claim khi dang ket noi.
func (r *Relay) Run(ctx context.Context) error {
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		r.gaugeLoop(ctx)
	}()
	defer func() {
		wg.Wait()
		r.mu.Lock()
		r.closeConn()
		r.mu.Unlock()
	}()

	fails := 0
	for ctx.Err() == nil {
		r.mu.Lock()
		claimed, _, err := r.runBatch(ctx)
		r.mu.Unlock()
		switch {
		case err != nil:
			if ctx.Err() != nil {
				return nil
			}
			fails++
			r.log.Warn("outbox relay: lo that bai, thu lai sau", "error", sanitizeErr(err), "fails", fails)
			sleep(ctx, backoff(fails, redialBase, redialMax))
		case claimed < r.cfg.BatchSize:
			fails = 0
			sleep(ctx, r.cfg.Interval)
		default:
			fails = 0 // lo day: lay tiep ngay
		}
	}
	return nil
}

// RunOnce xu ly mot lo: dial neu chua ket noi, claim, publish, danh dau. Tra so event da publish.
func (r *Relay) RunOnce(ctx context.Context) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, n, err := r.runBatch(ctx)
	return n, err
}

type outboxRow struct {
	id         int64
	eventID    string
	routingKey string
	payload    []byte
	headers    []byte
	traceID    *string
	createdAt  time.Time
	attempts   int
}

const claimSQL = `UPDATE outbox_events SET next_attempt_at = now() + make_interval(secs => $1)
WHERE id IN (
  SELECT id FROM outbox_events
  WHERE published_at IS NULL AND next_attempt_at <= now()
  ORDER BY next_attempt_at, id
  LIMIT $2
  FOR UPDATE SKIP LOCKED)
RETURNING id, event_id::text, routing_key, payload, headers, trace_id, created_at, attempts`

const (
	setStatementTimeoutSQL = `SET LOCAL statement_timeout = '5s'`
	markPublishedSQL       = `UPDATE outbox_events SET published_at = now(), last_error = NULL WHERE id = $1 AND published_at IS NULL`
	markUnroutableSQL      = `UPDATE outbox_events SET published_at = now(), last_error = 'unroutable' WHERE id = $1 AND published_at IS NULL`
	markRetrySQL           = `UPDATE outbox_events SET attempts = attempts + 1, last_error = $2, next_attempt_at = now() + make_interval(secs => $3) WHERE id = $1 AND published_at IS NULL`
	markParkSQL            = `UPDATE outbox_events SET attempts = attempts + 1, last_error = $2, next_attempt_at = 'infinity' WHERE id = $1 AND published_at IS NULL`
	releaseSQL             = `UPDATE outbox_events SET next_attempt_at = now() WHERE id = ANY($1) AND published_at IS NULL AND next_attempt_at <> 'infinity'`
	gaugeSQL               = `SELECT count(*) FILTER (WHERE next_attempt_at <> 'infinity'), count(*) FILTER (WHERE next_attempt_at = 'infinity') FROM outbox_events WHERE published_at IS NULL`
)

// runBatch tra (so row claim, so row publish, loi). Gia dinh r.mu dang duoc giu.
func (r *Relay) runBatch(ctx context.Context) (claimed, published int, err error) {
	if err := ctx.Err(); err != nil {
		return 0, 0, fmt.Errorf("outbox: %w", err)
	}
	if r.pub != nil && r.connDead() {
		r.log.Warn("outbox relay: ket noi broker da chet, dial lai")
		r.closeConn()    // gauge outbox_relay_connected = 0
		return 0, 0, nil // khong claim trong lan nay; lan sau dial lai
	}
	if r.pub == nil {
		if err := r.connect(ctx); err != nil {
			return 0, 0, err
		}
	}
	rows, err := r.claim(ctx)
	if err != nil {
		return 0, 0, err
	}
	if len(rows) == 0 {
		return 0, 0, nil
	}
	claimed = len(rows)
	deadline := time.Now().Add(r.cfg.BatchDeadline)

	for i, row := range rows {
		if cerr := ctx.Err(); cerr != nil {
			r.release(ctx, rows[i:])
			return claimed, published, fmt.Errorf("outbox: %w", cerr)
		}
		if !time.Now().Before(deadline) {
			r.release(ctx, rows[i:])
			return claimed, published, nil
		}
		res := r.publishRow(ctx, row, deadline)
		var markErr error
		switch res.kind {
		case resPublished:
			if markErr = r.exec(ctx, markPublishedSQL, row.id); markErr == nil {
				published++
				r.m.published.WithLabelValues(row.routingKey).Inc()
				r.m.lag.Observe(time.Since(row.createdAt).Seconds())
			}
		case resUnroutable:
			if markErr = r.exec(ctx, markUnroutableSQL, row.id); markErr == nil {
				r.m.unroutable.WithLabelValues(row.routingKey).Inc()
				r.log.Warn("outbox relay: event khong co queue nhan, bo qua", "routing_key", row.routingKey, "event_id", row.eventID)
			}
		case resBatchEnd:
			r.release(ctx, rows[i:])
			return claimed, published, nil
		case resRetry:
			markErr = r.markFailure(ctx, row, res.err)
		case resClosed:
			r.closeConn()
			r.release(ctx, rows[i:])
			return claimed, published, fmt.Errorf("outbox: kenh amqp dong: %w", res.err)
		case resCanceled:
			r.release(ctx, rows[i:])
			return claimed, published, fmt.Errorf("outbox: %w", ctx.Err())
		}
		if markErr != nil {
			r.release(ctx, rows[i+1:])
			return claimed, published, fmt.Errorf("outbox: danh dau row %d: %w", row.id, markErr)
		}
	}
	return claimed, published, nil
}

func (r *Relay) connect(ctx context.Context) error {
	pub, closer, err := r.dial(ctx)
	if err != nil {
		r.m.connected.Set(0)
		return fmt.Errorf("outbox: dial broker: %w", err)
	}
	if pub == nil {
		if closer != nil {
			_ = closer.Close()
		}
		r.m.connected.Set(0)
		return errors.New("outbox: dialer tra publisher nil")
	}
	r.pub, r.closer = pub, closer
	r.m.connected.Set(1)
	return nil
}

// connDead tra true neu closer hoac publisher (neu ho tro IsClosed, vd *amqp091.Connection) bao da dong.
func (r *Relay) connDead() bool {
	type closedChecker interface{ IsClosed() bool }
	if c, ok := r.closer.(closedChecker); ok && c.IsClosed() {
		return true
	}
	if c, ok := r.pub.(closedChecker); ok && c.IsClosed() {
		return true
	}
	return false
}

func (r *Relay) closeConn() {
	if r.closer != nil {
		_ = r.closer.Close()
	}
	r.pub, r.closer = nil, nil
	r.m.connected.Set(0)
}

// claim lay mot lo row trong mot transaction ngan va dat lease cho chung.
func (r *Relay) claim(ctx context.Context) ([]outboxRow, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("outbox: begin claim: %w", err)
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()
	if _, err := tx.Exec(ctx, setStatementTimeoutSQL); err != nil {
		return nil, fmt.Errorf("outbox: statement_timeout: %w", err)
	}
	q, err := tx.Query(ctx, claimSQL, r.cfg.Lease.Seconds(), r.cfg.BatchSize)
	if err != nil {
		return nil, fmt.Errorf("outbox: claim: %w", err)
	}
	var rows []outboxRow
	for q.Next() {
		var o outboxRow
		if err := q.Scan(&o.id, &o.eventID, &o.routingKey, &o.payload, &o.headers, &o.traceID, &o.createdAt, &o.attempts); err != nil {
			q.Close()
			return nil, fmt.Errorf("outbox: doc row claim: %w", err)
		}
		rows = append(rows, o)
	}
	q.Close()
	if err := q.Err(); err != nil {
		return nil, fmt.Errorf("outbox: claim: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("outbox: commit claim: %w", err)
	}
	slices.SortFunc(rows, func(a, b outboxRow) int {
		switch {
		case a.id < b.id:
			return -1
		case a.id > b.id:
			return 1
		}
		return 0
	})
	return rows, nil
}

type resultKind int

const (
	resPublished resultKind = iota
	resUnroutable
	resRetry    // tang attempts
	resClosed   // kenh dong: dong ket noi, tra lease
	resCanceled // ctx huy: tra lease
	resBatchEnd // het han lo giua luc publish: tra lease, khong tinh attempts
)

type result struct {
	kind resultKind
	err  error
}

// publishRow dung message tu cot, publish, phan loai ket qua (plan §7 R1). Panic duoc bat va tinh la loi dung message.
func (r *Relay) publishRow(ctx context.Context, row outboxRow, batchDeadline time.Time) (res result) {
	defer func() {
		if p := recover(); p != nil {
			res = result{resRetry, fmt.Errorf("panic khi xu ly row: %v", p)}
		}
	}()
	msg, err := buildMessage(row)
	if err != nil {
		return result{resRetry, err}
	}
	pctx := otelx.ExtractAMQP(ctx, otelx.AMQPHeaders(msg.Headers))
	// Han chot moi lan publish khong vuot han chot cua lo.
	limit := time.Now().Add(r.cfg.PublishTimeout)
	cappedByBatch := batchDeadline.Before(limit)
	if cappedByBatch {
		limit = batchDeadline
	}
	pctx, cancel := context.WithDeadline(pctx, limit)
	defer cancel()

	err = r.pub.Publish(pctx, r.cfg.Exchange, row.routingKey, msg)
	switch {
	case err == nil:
		return result{kind: resPublished}
	case errors.Is(err, mq.ErrClosed):
		return result{resClosed, err}
	case errors.Is(err, mq.ErrUnroutable):
		if _, ok := r.drop[row.routingKey]; ok {
			return result{kind: resUnroutable}
		}
		return result{resRetry, err}
	case errors.Is(err, mq.ErrNacked):
		return result{resRetry, err}
	case ctx.Err() != nil:
		return result{resCanceled, err}
	case cappedByBatch && errors.Is(err, context.DeadlineExceeded):
		return result{resBatchEnd, err}
	default: // het PublishTimeout khi kenh con mo, hoac loi khac cua mot message
		return result{resRetry, err}
	}
}

func buildMessage(row outboxRow) (mq.Message, error) {
	if !json.Valid(row.payload) {
		return mq.Message{}, errors.New("payload khong phai JSON hop le")
	}
	var hs map[string]string
	if len(row.headers) > 0 {
		if err := json.Unmarshal(row.headers, &hs); err != nil {
			return mq.Message{}, fmt.Errorf("headers khong hop le: %w", err)
		}
	}
	headers := make(map[string]any, len(hs))
	for k, v := range hs {
		headers[k] = v
	}
	env := Envelope{
		EventID:       row.eventID,
		OccurredAt:    row.createdAt,
		SchemaVersion: SchemaVersion,
		Type:          row.routingKey,
		Data:          json.RawMessage(row.payload),
	}
	if row.traceID != nil {
		env.TraceID = *row.traceID
	}
	body, err := json.Marshal(env)
	if err != nil {
		return mq.Message{}, fmt.Errorf("ma hoa phong bi: %w", err)
	}
	return mq.Message{
		Body:        body,
		ContentType: "application/json",
		Headers:     headers,
		MessageID:   row.eventID,
		Type:        row.routingKey,
		Timestamp:   row.createdAt,
	}, nil
}

// markFailure ghi loi tam (tang attempts) hoac do row neu da du MaxAttempts.
func (r *Relay) markFailure(ctx context.Context, row outboxRow, cause error) error {
	r.m.publishErrs.WithLabelValues(row.routingKey).Inc()
	next := row.attempts + 1
	msg := sanitizeErr(cause)
	if next >= r.cfg.MaxAttempts {
		r.log.Error("outbox relay: row bi do sau khi vuot MaxAttempts",
			"event_id", row.eventID, "routing_key", row.routingKey, "attempts", next, "error", msg)
		return r.exec(ctx, markParkSQL, row.id, msg)
	}
	delay := backoff(next, r.cfg.BaseBackoff, r.cfg.MaxBackoff)
	return r.exec(ctx, markRetrySQL, row.id, msg, delay.Seconds())
}

// exec chay cau danh dau o autocommit duoi WithoutCancel de SIGTERM khong lam mat dau da gui.
func (r *Relay) exec(ctx context.Context, sql string, args ...any) error {
	mctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), markTimeout)
	defer cancel()
	_, err := r.db.Exec(mctx, sql, args...)
	return err
}

// release tra lease cho cac row chua xu ly (khong tang attempts); loi chi duoc log vi lease se tu het han.
func (r *Relay) release(ctx context.Context, rows []outboxRow) {
	if len(rows) == 0 {
		return
	}
	ids := make([]int64, len(rows))
	for i, row := range rows {
		ids[i] = row.id
	}
	// Cau SQL loai row 'infinity': khong bao gio gan lai (go do) mot row da bi do.
	if err := r.exec(ctx, releaseSQL, ids); err != nil {
		r.log.Warn("outbox relay: khong tra duoc lease, se het han tu nhien", "rows", len(ids), "error", sanitizeErr(err))
	}
}

func (r *Relay) gaugeLoop(ctx context.Context) {
	t := time.NewTicker(gaugeRefresh)
	defer t.Stop()
	for {
		r.refreshGauges(ctx)
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

func (r *Relay) refreshGauges(ctx context.Context) {
	qctx, cancel := context.WithTimeout(ctx, markTimeout)
	defer cancel()
	var pending, dead int64
	if err := r.db.QueryRow(qctx, gaugeSQL).Scan(&pending, &dead); err != nil {
		if ctx.Err() == nil {
			r.log.Warn("outbox relay: khong dem duoc outbox", "error", sanitizeErr(err))
		}
		return
	}
	r.m.pending.Set(float64(pending))
	r.m.dead.Set(float64(dead))
}

func sleep(ctx context.Context, d time.Duration) {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
	case <-t.C:
	}
}

// backoff tra do lui cho lan thu lai sau `attempts` lan that bai:
// min(base * 2^clamp(attempts-1, 0, 20), max) nhan jitter trong [0.8, 1.2].
func backoff(attempts int, base, ceiling time.Duration) time.Duration {
	exp := min(max(attempts-1, 0), maxBackoffExp)
	d := float64(base) * math.Pow(2, float64(exp))
	if d > float64(ceiling) {
		d = float64(ceiling)
	}
	d *= 0.8 + 0.4*rand.Float64()
	if d < 1 {
		return 1
	}
	return time.Duration(d)
}

// sanitizeErr dua loi ve chuoi an toan de luu last_error: UTF-8 hop le, khong NUL, <= 1024 byte.
func sanitizeErr(err error) string {
	if err == nil {
		return ""
	}
	s := strings.ToValidUTF8(err.Error(), "�")
	s = strings.ReplaceAll(s, "\x00", "")
	if len(s) > maxLastErrBytes {
		cut := maxLastErrBytes
		for cut > 0 && !utf8.RuneStart(s[cut]) {
			cut--
		}
		s = s[:cut]
	}
	return s
}
