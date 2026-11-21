package postgresadapter

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/eventflow/eventflow/libs/go/antibot"
)

// Writer limits (contracts 5).
const (
	decisionBuffer     = 10000
	decisionBatch      = 500
	decisionFlushEvery = 200 * time.Millisecond
	decisionFlushLimit = 2 * time.Second
)

type decisionRow struct {
	eventID, identityID any // string UUID or nil
	fp, ip              any // string or nil
	score               float64
	action              string
	fired               []string
}

// DecisionWriter persists anti-bot decisions into bot_decisions asynchronously (contracts 5, AC-14). It
// implements antibot.Sink: Offer never blocks the request; a full buffer drops the row and counts it. A nil pool
// disables the writer (every row is dropped and counted).
type DecisionWriter struct {
	pool *pgxpool.Pool
	m    *antibot.Metrics

	ch      chan decisionRow
	stop    chan struct{}
	done    chan struct{}
	closed  atomic.Bool
	closeMu sync.Once
}

// NewDecisionWriter starts the writer goroutine (when pool is not nil).
func NewDecisionWriter(pool *pgxpool.Pool, m *antibot.Metrics) *DecisionWriter {
	if m == nil {
		m = antibot.NewMetrics(nil)
	}
	w := &DecisionWriter{pool: pool, m: m, ch: make(chan decisionRow, decisionBuffer),
		stop: make(chan struct{}), done: make(chan struct{})}
	if pool == nil {
		close(w.done)
		return w
	}
	go w.loop()
	return w
}

// Offer queues one decision; it never blocks.
func (w *DecisionWriter) Offer(in antibot.Input, d antibot.Decision) {
	if w.pool == nil || w.closed.Load() {
		w.m.Dropped.Inc()
		return
	}
	row := w.toRow(in, d)
	select {
	case w.ch <- row:
	default:
		w.m.Dropped.Inc()
	}
}

func (w *DecisionWriter) toRow(in antibot.Input, d antibot.Decision) decisionRow {
	r := decisionRow{score: d.Score, action: dbAction(d.Action)}
	nonUUID := false
	r.eventID, nonUUID = uuidOrNil(in.EventID, nonUUID)
	r.identityID, nonUUID = uuidOrNil(in.IdentityID, nonUUID)
	if nonUUID {
		w.m.NonUUID.Inc()
	}
	if d.FPHash != "" {
		r.fp = d.FPHash
	}
	if d.NetPrefix.IsValid() {
		r.ip = d.NetPrefix.String()
	}
	r.fired = make([]string, 0, len(d.Fired)+len(d.Tags))
	for _, id := range d.Fired {
		r.fired = append(r.fired, string(id))
	}
	r.fired = append(r.fired, d.Tags...)
	return r
}

// dbAction maps the tier to the column's value set (PASS is stored as ALLOW).
func dbAction(a antibot.Action) string {
	switch a {
	case antibot.ActionChallenge, antibot.ActionSlowLane, antibot.ActionBlock:
		return string(a)
	default:
		return "ALLOW"
	}
}

func uuidOrNil(s string, flagged bool) (any, bool) {
	if isUUID(s) {
		return strings.ToLower(s), flagged
	}
	return nil, true
}

func isUUID(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch i {
		case 8, 13, 18, 23:
			if c != '-' {
				return false
			}
		default:
			if (c < '0' || c > '9') && (c < 'a' || c > 'f') && (c < 'A' || c > 'F') {
				return false
			}
		}
	}
	return true
}

// Close stops the writer and flushes what is buffered (budget 2 s, or ctx if shorter).
func (w *DecisionWriter) Close(ctx context.Context) error {
	w.closeMu.Do(func() {
		w.closed.Store(true)
		close(w.stop)
	})
	select {
	case <-w.done:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("bot_decisions flush: %w", ctx.Err())
	}
}

func (w *DecisionWriter) loop() {
	defer close(w.done)
	for !w.run() {
	}
}

// run is the writer loop; it returns true when stopped, false after a recovered panic (the loop restarts).
func (w *DecisionWriter) run() (stopped bool) {
	batch := make([]decisionRow, 0, decisionBatch)
	defer func() {
		if r := recover(); r != nil {
			w.m.Dropped.Add(float64(len(batch)))
			stopped = false
		}
	}()
	ticker := time.NewTicker(decisionFlushEvery)
	defer ticker.Stop()
	for {
		select {
		case r := <-w.ch:
			batch = append(batch, r)
			if len(batch) >= decisionBatch {
				w.flush(context.Background(), batch)
				batch = batch[:0]
			}
		case <-ticker.C:
			if len(batch) > 0 {
				w.flush(context.Background(), batch)
				batch = batch[:0]
			}
		case <-w.stop:
			ctx, cancel := context.WithTimeout(context.Background(), decisionFlushLimit)
			defer cancel()
			for drained := false; !drained; {
				select {
				case r := <-w.ch:
					batch = append(batch, r)
					if len(batch) >= decisionBatch {
						w.flush(ctx, batch)
						batch = batch[:0]
					}
				default:
					drained = true
				}
			}
			if len(batch) > 0 {
				w.flush(ctx, batch)
				batch = batch[:0]
			}
			return true
		}
	}
}

// flush writes one multi-row INSERT in the simple protocol (PgBouncer transaction pooling safe). A failed batch
// is dropped and counted.
func (w *DecisionWriter) flush(ctx context.Context, batch []decisionRow) {
	if len(batch) == 0 {
		return
	}
	if ctx.Err() != nil {
		w.m.Dropped.Add(float64(len(batch)))
		return
	}
	var sb strings.Builder
	sb.WriteString("INSERT INTO bot_decisions (event_id, identity_id, device_fp, ip, score, action, fired_rules) VALUES ")
	args := make([]any, 0, 1+len(batch)*7)
	args = append(args, pgx.QueryExecModeSimpleProtocol)
	for i, r := range batch {
		if i > 0 {
			sb.WriteByte(',')
		}
		n := i * 7
		fmt.Fprintf(&sb, "($%d::uuid,$%d::uuid,$%d::text,$%d::inet,$%d::numeric,$%d::text,$%d::text[])",
			n+1, n+2, n+3, n+4, n+5, n+6, n+7)
		args = append(args, r.eventID, r.identityID, r.fp, r.ip, r.score, r.action, r.fired)
	}
	ctx, cancel := context.WithTimeout(ctx, decisionFlushLimit)
	defer cancel()
	if _, err := w.pool.Exec(ctx, sb.String(), args...); err != nil {
		w.m.Dropped.Add(float64(len(batch)))
	}
}
