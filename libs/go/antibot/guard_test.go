package antibot_test

import (
	"context"
	"errors"
	"net/netip"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/eventflow/eventflow/libs/go/antibot"
)

// fakeStore is an antibot.StateStore with scripted behaviour.
type fakeStore struct {
	mu    sync.Mutex
	st    antibot.State
	err   error
	hang  bool // block until ctx is done, then return ctx.Err()
	boom  bool // panic
	calls int
	fp    string
	net   netip.Prefix
	sq    string
}

func (f *fakeStore) Observe(ctx context.Context, in antibot.Input, fpHash string, net netip.Prefix, sq string) (antibot.State, error) {
	f.mu.Lock()
	f.calls++
	f.fp, f.net, f.sq = fpHash, net, sq
	st, err, hang, boom := f.st, f.err, f.hang, f.boom
	f.mu.Unlock()
	if boom {
		panic("store exploded")
	}
	if hang {
		<-ctx.Done()
		return antibot.State{}, ctx.Err()
	}
	return st, err
}

func (f *fakeStore) n() int { f.mu.Lock(); defer f.mu.Unlock(); return f.calls }

type recSink struct {
	mu  sync.Mutex
	got []antibot.Decision
}

func (s *recSink) Offer(_ antibot.Input, d antibot.Decision) {
	s.mu.Lock()
	s.got = append(s.got, d)
	s.mu.Unlock()
}

func (s *recSink) n() int { s.mu.Lock(); defer s.mu.Unlock(); return len(s.got) }

type guardEnv struct {
	g     *antibot.Guard
	store *fakeStore
	sink  *recSink
	reg   *prometheus.Registry
}

func newGuard(t testing.TB, st *fakeStore, kv ...string) guardEnv {
	t.Helper()
	if st == nil {
		st = &fakeStore{st: neutral()}
	}
	reg := prometheus.NewRegistry()
	sink := &recSink{}
	g := antibot.NewGuard(cfgWith(t, kv...), st, sink, antibot.NewMetrics(reg))
	return guardEnv{g: g, store: st, sink: sink, reg: reg}
}

// AC-6 / AC-15: Decide decodes the raw signals and hands the store only the HMAC, the prefix and sq.
func TestGuard_AC6_ObserveGetsHashPrefixAndSq(t *testing.T) {
	e := newGuard(t, nil)
	e.g.Decide(context.Background(), input([]byte(validSignals)))
	if e.store.n() != 1 {
		t.Fatalf("Observe calls = %d, want 1", e.store.n())
	}
	if e.store.fp != antibot.FPHash([]byte("0123456789abcdef0123456789abcdef"), rawFP) {
		t.Errorf("fp passed to store = %q, want HMAC", e.store.fp)
	}
	if e.store.net.String() != "198.51.100.0/24" {
		t.Errorf("prefix passed to store = %s", e.store.net)
	}
	if e.store.sq != "0123456789abcdef" {
		t.Errorf("sq passed to store = %q", e.store.sq)
	}
}

// AC-7: Redis slower than the budget → stateless rules only, DEGRADED, redis_timeout; Decide returns fast.
func TestGuard_AC7_TimeoutDegrades(t *testing.T) {
	st := &fakeStore{hang: true}
	e := newGuard(t, st, "ANTIBOT_REDIS_TIMEOUT_MS", "4")
	start := time.Now()
	d := e.g.Decide(context.Background(), input(payload(t, map[string]any{"rt": 50})))
	if el := time.Since(start); el > 100*time.Millisecond {
		t.Fatalf("Decide took %s with a hanging store (budget 4 ms)", el)
	}
	if !hasTag(d, "DEGRADED") || !hasRule(d, "R2") {
		t.Fatalf("decision %+v, want DEGRADED with stateless R2", d)
	}
	if v := metric(t, e.reg, "antibot_degraded_total", map[string]string{"reason": "redis_timeout"}); v != 1 {
		t.Fatalf("antibot_degraded_total{redis_timeout} = %v", v)
	}
}

// AC-7: a store error → DEGRADED, redis_error; stateful rules are not evaluated from the garbage state.
func TestGuard_AC7_ErrorDegrades(t *testing.T) {
	st := &fakeStore{st: antibot.State{FPIdentities: 99, NetIdentities: 99, Known: true}, err: errors.New("READONLY")}
	e := newGuard(t, st)
	d := e.g.Decide(context.Background(), input([]byte(validSignals)))
	if !hasTag(d, "DEGRADED") || hasRule(d, "R4D") || hasRule(d, "R4N") {
		t.Fatalf("decision %+v, want DEGRADED without stateful rules", d)
	}
	if v := metric(t, e.reg, "antibot_degraded_total", map[string]string{"reason": "redis_error"}); v != 1 {
		t.Fatalf("antibot_degraded_total{redis_error} = %v", v)
	}
}

// AC-7: a panic inside antibot → PASS and antibot_degraded_total{panic}.
func TestGuard_AC7_PanicPasses(t *testing.T) {
	e := newGuard(t, &fakeStore{boom: true})
	var d antibot.Decision
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("panic escaped Decide: %v", r)
			}
		}()
		d = e.g.Decide(context.Background(), input(payload(t, map[string]any{"wd": true, "rt": 1})))
	}()
	if d.Applied != pass {
		t.Fatalf("applied after panic = %s, want PASS", d.Applied)
	}
	if v := metric(t, e.reg, "antibot_degraded_total", map[string]string{"reason": "panic"}); v != 1 {
		t.Fatalf("antibot_degraded_total{panic} = %v", v)
	}
}

// AC-7 / D7: after N degraded calls in the window the breaker skips Redis (breaker_open), then closes again.
func TestGuard_AC7_BreakerOpensAndCloses(t *testing.T) {
	st := &fakeStore{err: errors.New("down")}
	e := newGuard(t, st, "ANTIBOT_BREAKER", "5/1s,open=200ms")
	for i := 0; i < 5; i++ {
		e.g.Decide(context.Background(), input([]byte(validSignals)))
	}
	if st.n() != 5 {
		t.Fatalf("store calls before breaker = %d", st.n())
	}
	for i := 0; i < 10; i++ {
		d := e.g.Decide(context.Background(), input([]byte(validSignals)))
		if !hasTag(d, "DEGRADED") {
			t.Fatalf("breaker-open decision not DEGRADED: %+v", d)
		}
	}
	if st.n() != 5 {
		t.Fatalf("store called %d times while breaker open, want 5", st.n())
	}
	if v := metric(t, e.reg, "antibot_degraded_total", map[string]string{"reason": "breaker_open"}); v != 10 {
		t.Fatalf("antibot_degraded_total{breaker_open} = %v, want 10", v)
	}
	time.Sleep(300 * time.Millisecond)
	st.mu.Lock()
	st.err, st.st = nil, neutral()
	st.mu.Unlock()
	d := e.g.Decide(context.Background(), input([]byte(validSignals)))
	if st.n() != 6 || hasTag(d, "DEGRADED") {
		t.Fatalf("breaker did not close: calls %d decision %+v", st.n(), d)
	}
}

// AC-3 (contracts 8.4): Decide counts antibot_signals_invalid_total{reason}, also for the handler's BodyErr.
func TestGuard_AC3_InvalidSignalsMetric(t *testing.T) {
	e := newGuard(t, nil)
	ctx := context.Background()
	e.g.Decide(ctx, input(nil))
	e.g.Decide(ctx, input([]byte(`{"v":`)))
	e.g.Decide(ctx, input([]byte(`{"v":7}`)))
	big := input(nil)
	big.BodyErr = "too_large"
	e.g.Decide(ctx, big)
	e.g.Decide(ctx, input([]byte(validSignals)))
	for reason, want := range map[string]float64{"missing": 1, "bad_json": 1, "bad_version": 1, "too_large": 1} {
		if v := metric(t, e.reg, "antibot_signals_invalid_total", map[string]string{"reason": reason}); v != want {
			t.Errorf("antibot_signals_invalid_total{%s} = %v, want %v", reason, v, want)
		}
	}
	d := e.g.Decide(ctx, big)
	if !hasRule(d, "R3") || !hasRule(d, "R5") {
		t.Fatalf("BodyErr too_large must count as missing (R3+R5): fired %s", ids(d))
	}
}

// AC-17: decisions, fired rules, decide latency and capped counters.
func TestGuard_AC17_DecisionMetrics(t *testing.T) {
	e := newGuard(t, nil, "ANTIBOT_MODE", "shadow")
	e.g.Decide(context.Background(), input(payload(t, map[string]any{"rt": 50})))
	e.g.Decide(context.Background(), input(nil))
	if v := metric(t, e.reg, "antibot_decisions_total", map[string]string{"action": "CHALLENGE", "applied": "PASS", "mode": "shadow"}); v != 1 {
		t.Errorf("antibot_decisions_total{CHALLENGE,PASS,shadow} = %v", v)
	}
	if v := metric(t, e.reg, "antibot_rule_fired_total", map[string]string{"rule": "R2"}); v != 1 {
		t.Errorf("antibot_rule_fired_total{R2} = %v", v)
	}
	if v := metric(t, e.reg, "antibot_decide_seconds", nil); v != 2 {
		t.Errorf("antibot_decide_seconds count = %v", v)
	}
	if v := metric(t, e.reg, "antibot_capped_total", nil); v != 1 {
		t.Errorf("antibot_capped_total = %v", v)
	}
}

// AC-14: score >= 0.30 always reaches the sink; PASS only through the sample.
func TestGuard_AC14_SinkSelection(t *testing.T) {
	e := newGuard(t, nil, "ANTIBOT_PASS_SAMPLE", "0")
	for i := 0; i < 50; i++ {
		e.g.Decide(context.Background(), input([]byte(validSignals)))
	}
	if e.sink.n() != 0 {
		t.Fatalf("PASS rows offered with sample 0: %d", e.sink.n())
	}
	e.g.Decide(context.Background(), input(payload(t, map[string]any{"rt": 50})))   // 0.40
	e.g.Decide(context.Background(), input(payload(t, map[string]any{"wd": true}))) // 0.80
	if e.sink.n() != 2 {
		t.Fatalf("scored rows offered = %d, want 2", e.sink.n())
	}

	all := newGuard(t, nil, "ANTIBOT_PASS_SAMPLE", "1")
	for i := 0; i < 20; i++ {
		all.g.Decide(context.Background(), input([]byte(validSignals)))
	}
	if all.sink.n() != 20 {
		t.Fatalf("sample 1: offered %d of 20", all.sink.n())
	}

	def := newGuard(t, nil)
	for i := 0; i < 20000; i++ {
		def.g.Decide(context.Background(), input([]byte(validSignals)))
	}
	if n := def.sink.n(); n < 100 || n > 320 {
		t.Fatalf("default 1%% sample offered %d of 20000, want about 200", n)
	}
}

// AC-12 / AC-14: sticky from the store → BLOCK {STICKY} score 1, offered to the sink.
func TestGuard_AC12_StickyBlocks(t *testing.T) {
	e := newGuard(t, &fakeStore{st: antibot.State{Sticky: true, Known: true, CardIdentities: -1}})
	d := e.g.Decide(context.Background(), input([]byte(validSignals)))
	if d.Applied != block || len(d.Fired) != 0 || !hasTag(d, "STICKY") || d.Score != 1 {
		t.Fatalf("sticky: %+v", d)
	}
	if e.sink.n() != 1 {
		t.Fatalf("sticky not offered to sink")
	}
}

// AC-13: shadow never applies anything but PASS, even for BLOCK-tier decisions.
func TestGuard_AC13_ShadowAlwaysPasses(t *testing.T) {
	e := newGuard(t, nil, "ANTIBOT_MODE", "shadow")
	d := e.g.Decide(context.Background(), input(payload(t, map[string]any{"wd": true, "rt": 1})))
	if d.Action != block || d.Applied != pass || !hasTag(d, "MODE_SHADOW") {
		t.Fatalf("shadow: %+v", d)
	}
}

// AC-15: no raw fingerprint, UA or IP in the decision Decide returns.
func TestGuard_AC15_NoRawValuesInDecision(t *testing.T) {
	e := newGuard(t, nil)
	d := e.g.Decide(context.Background(), input([]byte(validSignals)))
	dump := sprintAll(d)
	for _, raw := range []string{rawFP, rawIP, chromeUA} {
		if containsStr(dump, raw) {
			t.Errorf("decision contains %q", raw)
		}
	}
}

// AC-7: Decide is safe for concurrent use.
func TestGuard_AC7_ConcurrentDecide(t *testing.T) {
	e := newGuard(t, nil)
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				e.g.Decide(context.Background(), input([]byte(validSignals)))
			}
		}()
	}
	wg.Wait()
	if v := metric(t, e.reg, "antibot_decisions_total", nil); v != 6400 {
		t.Fatalf("decisions counted %v, want 6400", v)
	}
}
