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
	if g.store == nil {
		return State{CardIdentities: -1}
	}
	now := time.Now()
	if !g.br.allow(now) {
		g.m.Degraded.WithLabelValues(DegradedBreakerOpen).Inc()
		return State{CardIdentities: -1}
	}
	timeout := g.cfg.RedisTimeout
	if timeout <= 0 {
		timeout = 4 * time.Millisecond
	}
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	st, err := g.store.Observe(cctx, in, fp, prefix, sq)
	if err != nil {
		reason := DegradedRedisError
		if isTimeout(err) || errors.Is(cctx.Err(), context.DeadlineExceeded) {
			reason = DegradedRedisTimeout
		}
		g.m.Degraded.WithLabelValues(reason).Inc()
		g.br.fail(time.Now())
		return State{CardIdentities: -1}
	}
	st.Known = true
	return st
}

func isTimeout(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var ne net.Error
	return errors.As(err, &ne) && ne.Timeout()
}

// breaker opens after n failures within window and stays open for open (D7).
type breaker struct {
	n            int
	window, open time.Duration

	mu          sync.Mutex
	windowStart time.Time
	failures    int
	openUntil   time.Time
}

func (b *breaker) allow(now time.Time) bool {
	if b.n <= 0 {
		return true
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return !now.Before(b.openUntil)
}

func (b *breaker) fail(now time.Time) {
	if b.n <= 0 {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.windowStart.IsZero() || now.Sub(b.windowStart) > b.window {
		b.windowStart, b.failures = now, 0
	}
	b.failures++
	if b.failures >= b.n {
		b.openUntil = now.Add(b.open)
		b.windowStart, b.failures = time.Time{}, 0
	}
}
