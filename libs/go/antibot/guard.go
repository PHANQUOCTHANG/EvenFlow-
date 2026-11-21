package antibot

import (
	"context"
	"errors"
	"math/rand/v2"
	"net"
	"net/netip"
	"sync"
	"time"
)

// Guard runs decode + Redis state + rules + score for one join. It never fails a join because of its own
// failure (D7): a slow or broken store degrades to stateless rules, a panic passes.
type Guard struct {
	cfg   Config
	store StateStore
	sink  Sink
	m     *Metrics
	br    breaker
}

// NewGuard builds a guard. store, sink and m may be nil.
func NewGuard(cfg Config, store StateStore, sink Sink, m *Metrics) *Guard {
	if m == nil {
		m = NewMetrics(nil)
	}
	cfg.index()
	return &Guard{cfg: cfg, store: store, sink: sink, m: m,
		br: breaker{n: cfg.BreakerN, window: cfg.BreakerWindow, open: cfg.BreakerOpen}}
}

// Config returns the guard's configuration.
func (g *Guard) Config() Config { return g.cfg }

// Metrics returns the guard's collectors.
func (g *Guard) Metrics() *Metrics { return g.m }

// Decide evaluates one join. It is safe for concurrent use.
func (g *Guard) Decide(ctx context.Context, in Input) (d Decision) {
	start := time.Now()
	defer func() {
		if r := recover(); r != nil {
			d = Decision{Action: ActionPass, Applied: ActionPass, Tags: []string{TagDegraded}}
			if g.cfg.Mode == ModeShadow {
				d.Tags = []string{TagModeShadow, TagDegraded}
			}
			g.safeInc(func() { g.m.Degraded.WithLabelValues(DegradedPanic).Inc() })
		}
		g.safeInc(func() {
			g.m.DecideSeconds.Observe(time.Since(start).Seconds())
			g.m.Decisions.WithLabelValues(string(d.Action), string(d.Applied), string(g.cfg.Mode)).Inc()
		})
	}()
	d = g.decide(ctx, in)
	return d
}

func (g *Guard) safeInc(f func()) {
	defer func() { _ = recover() }()
	f()
}

func (g *Guard) decide(ctx context.Context, in Input) Decision {
	sig, reason := DecodeSignals(in.RawSignals)
	if in.BodyErr != "" {
		reason = in.BodyErr
		sig = missingSignals(reason)
	}
	if reason != "" {
		g.m.SignalsInvalid.WithLabelValues(string(reason)).Inc()
	}
	fp := ""
	if sig.FP != "" {
		fp = FPHash(g.cfg.HashKey, sig.FP)
	}
	prefix := NetPrefix(in.ClientIP)

	st := g.observe(ctx, in, fp, prefix, sig.SQ)
	d := evaluate(&g.cfg, &in, &sig, &st, fp, prefix)

	for _, id := range d.Fired {
		g.m.RuleFired.WithLabelValues(string(id)).Inc()
	}
	if st.NetSuppressed {
		g.m.NetSuppressed.Inc()
	}
	for _, t := range d.Tags {
		if t == TagCapped {
			g.m.Capped.Inc()
		}
	}
	if g.sink != nil && (d.Action != ActionPass || (g.cfg.PassSample > 0 && rand.Float64() < g.cfg.PassSample)) {
		g.sink.Offer(in, d)
	}
	return d
}

// observe calls the store under the Redis budget and the breaker. Failure returns an unknown state.
func (g *Guard) observe(ctx context.Context, in Input, fp string, prefix netip.Prefix, sq string) State {
	unknown := State{CardIdentities: -1}
	if g.store == nil {
		return unknown
	}
	// X-1 / R-1: the buyer already went away (tab closed, reload). Nothing is wrong with Redis: do not call it,
	// do not count a Redis failure and do not feed the breaker.
	if ctx.Err() != nil {
		return unknown
	}
	now := time.Now()
	probe, ok := g.br.allow(now)
	if !ok {
		g.m.Degraded.WithLabelValues(DegradedBreakerOpen).Inc()
		return unknown
	}
	timeout := g.cfg.RedisTimeout
	if timeout <= 0 {
		timeout = 4 * time.Millisecond
	}
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	st, err := g.store.Observe(cctx, in, fp, prefix, sq)
	if err == nil {
		g.br.success(probe)
		st.Known = true
		return st
	}
	if ctx.Err() != nil { // the parent was cancelled while waiting: not a Redis failure (X-1)
		g.br.abandon(probe)
		return unknown
	}
	kind := failHard
	reason := DegradedRedisError
	switch {
	case errors.Is(err, ErrPoolExhausted):
		// X-2: every connection of the antibot pool is busy. That is local pressure (a burst of joins), not a
		// Redis outage, so it is counted but never opens the breaker.
		kind, reason = failPool, DegradedRedisTimeout
	case isTimeout(err) || errors.Is(cctx.Err(), context.DeadlineExceeded):
		kind, reason = failSlow, DegradedRedisTimeout
	}
	g.m.Degraded.WithLabelValues(reason).Inc()
	g.br.fail(time.Now(), kind, probe)
	return unknown
}

// ErrPoolExhausted is returned (wrapped) by a StateStore when no connection was free within the budget.
var ErrPoolExhausted = errors.New("antibot: state store pool exhausted")

func isTimeout(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var ne net.Error
	return errors.As(err, &ne) && ne.Timeout()
}

type failKind int

const (
	failHard failKind = iota // connection refused, READONLY, script error...: opens the breaker at once
	failSlow                 // over the budget: opens only when the slowness lasts (minSlowSpan)
	failPool                 // pool exhausted: never opens the breaker
)

// Breaker thresholds (X-2), documented in contracts 3 (ANTIBOT_BREAKER):
//   - closed -> open after n failures within window. Hard errors count at once. Timeouts count only once the
//     failures span at least minSlowSpan (min(250 ms, window/2)): a Redis stall of a few ms (shuffle.lua placing
//     a big lobby at T0) makes those joins DEGRADED one by one but does not turn history rules off for the whole
//     open period. Pool exhaustion never counts.
//   - open -> half-open after the open duration: exactly one request probes Redis, the others stay degraded.
//   - half-open -> closed on a successful probe; -> open again on a failed probe, with the open duration doubled
//     each time up to maxOpenFactor x the configured one.
const (
	maxSlowSpan   = 250 * time.Millisecond
	maxOpenFactor = 4
)

type breakerState int

const (
	brClosed breakerState = iota
	brOpen
	brHalfOpen
)

type breaker struct {
	n            int
	window, open time.Duration

	mu          sync.Mutex
	state       breakerState
	windowStart time.Time
	failures    int
	hard        bool
	openUntil   time.Time
	openFor     time.Duration
	probing     bool
}

func (b *breaker) minSlowSpan() time.Duration {
	if b.window/2 < maxSlowSpan {
		return b.window / 2
	}
	return maxSlowSpan
}

// allow reports whether a call may go to Redis; probe is true for the single half-open probe.
func (b *breaker) allow(now time.Time) (probe, ok bool) {
	if b.n <= 0 {
		return false, true
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	switch b.state {
	case brOpen:
		if now.Before(b.openUntil) {
			return false, false
		}
		b.state = brHalfOpen
		fallthrough
	case brHalfOpen:
		if b.probing {
			return false, false
		}
		b.probing = true
		return true, true
	}
	return false, true
}

func (b *breaker) success(probe bool) {
	if b.n <= 0 {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if probe || b.state == brHalfOpen {
		b.state, b.probing, b.openFor = brClosed, false, 0
		b.windowStart, b.failures, b.hard = time.Time{}, 0, false
	}
}

// abandon releases a probe whose caller went away without an answer from Redis.
func (b *breaker) abandon(probe bool) {
	if !probe || b.n <= 0 {
		return
	}
	b.mu.Lock()
	b.probing = false
	b.mu.Unlock()
}

func (b *breaker) fail(now time.Time, kind failKind, probe bool) {
	if b.n <= 0 {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if probe || b.state == brHalfOpen {
		if kind == failPool {
			b.probing = false
			return
		}
		b.openFor = min(b.openFor*2, b.open*maxOpenFactor)
		b.trip(now)
		return
	}
	if kind == failPool || b.state != brClosed {
		return
	}
	if b.windowStart.IsZero() || now.Sub(b.windowStart) > b.window {
		b.windowStart, b.failures, b.hard = now, 0, false
	}
	b.failures++
	b.hard = b.hard || kind == failHard
	if b.failures >= b.n && (b.hard || now.Sub(b.windowStart) >= b.minSlowSpan()) {
		b.openFor = b.open
		b.trip(now)
	}
}

func (b *breaker) trip(now time.Time) {
	if b.openFor <= 0 {
		b.openFor = b.open
	}
	b.state, b.probing = brOpen, false
	b.openUntil = now.Add(b.openFor)
	b.windowStart, b.failures, b.hard = time.Time{}, 0, false
}
