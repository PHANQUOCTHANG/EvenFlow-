package antibot_test

// Regression tests for fix round 1 (findings-review.json R-1/R-5, findings-challenge.json X-1/X-2, spec
// Amendment 2). New file: the frozen tests of 36aab08 stay byte-identical.

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"math"
	"net/netip"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/eventflow/eventflow/libs/go/antibot"
	"github.com/eventflow/eventflow/libs/go/antibot/corpus"
)

// scriptedStore returns errors from a queue, then the neutral state. block, when set, holds a call until released.
type scriptedStore struct {
	mu    sync.Mutex
	errs  []error
	calls int
	block chan struct{}
}

func (s *scriptedStore) Observe(ctx context.Context, _ antibot.Input, _ string, _ netip.Prefix, _ string) (antibot.State, error) {
	s.mu.Lock()
	s.calls++
	var err error
	if len(s.errs) > 0 {
		err, s.errs = s.errs[0], s.errs[1:]
	}
	block := s.block
	s.mu.Unlock()
	if block != nil {
		select {
		case <-block:
		case <-ctx.Done():
			return antibot.State{}, ctx.Err()
		}
	}
	if err != nil {
		return antibot.State{}, err
	}
	return neutral(), nil
}

func (s *scriptedStore) n() int { s.mu.Lock(); defer s.mu.Unlock(); return s.calls }

func (s *scriptedStore) push(err error, k int) {
	s.mu.Lock()
	for i := 0; i < k; i++ {
		s.errs = append(s.errs, err)
	}
	s.mu.Unlock()
}

// timeoutErr is a net.Error that reports a timeout immediately (a Redis stall seen by the client).
type timeoutErr struct{}

func (timeoutErr) Error() string   { return "i/o timeout" }
func (timeoutErr) Timeout() bool   { return true }
func (timeoutErr) Temporary() bool { return true }

func scripted(t testing.TB, st antibot.StateStore, kv ...string) guardEnv {
	t.Helper()
	reg := prometheus.NewRegistry()
	sink := &recSink{}
	return guardEnv{g: antibot.NewGuard(cfgWith(t, kv...), st, sink, antibot.NewMetrics(reg)), sink: sink, reg: reg}
}

func degradedTotal(t *testing.T, e guardEnv, reason string) float64 {
	t.Helper()
	return metric(t, e.reg, "antibot_degraded_total", map[string]string{"reason": reason})
}

// X-1 / R-1: a cancelled parent context (client went away) never counts as a Redis failure and never opens the
// breaker, so the next honest join still gets history rules.
func TestFix1_X1_CancelledParentIsNotARedisFailure(t *testing.T) {
	st := &scriptedStore{}
	e := scripted(t, st, "ANTIBOT_BREAKER", "20/1s,open=3s")
	g := e.g
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for i := 0; i < 40; i++ {
		g.Decide(ctx, input([]byte(validSignals)))
	}
	if st.n() != 0 {
		t.Fatalf("store called %d times for already-cancelled requests", st.n())
	}
	for _, r := range []string{"redis_error", "redis_timeout", "breaker_open"} {
		if v := degradedTotal(t, e, r); v != 0 {
			t.Fatalf("antibot_degraded_total{%s} = %v after client aborts, want 0", r, v)
		}
	}
	d := g.Decide(context.Background(), input([]byte(validSignals)))
	if hasTag(d, "DEGRADED") || st.n() != 1 {
		t.Fatalf("honest join after aborts: tags %v store calls %d, want history rules (not DEGRADED)", d.Tags, st.n())
	}
}

// X-1: the parent is cancelled while the store call is in flight: still not a Redis failure.
func TestFix1_X1_CancelDuringObserveIsNotCounted(t *testing.T) {
	st := &scriptedStore{block: make(chan struct{})}
	e := scripted(t, st, "ANTIBOT_REDIS_TIMEOUT_MS", "50", "ANTIBOT_BREAKER", "1/1s,open=3s")
	g := e.g
	ctx, cancel := context.WithCancel(context.Background())
	go func() { time.Sleep(5 * time.Millisecond); cancel() }()
	g.Decide(ctx, input([]byte(validSignals)))
	if v := degradedTotal(t, e, "redis_error") + degradedTotal(t, e, "redis_timeout"); v != 0 {
		t.Fatalf("client abort during Observe counted as %v Redis failures", v)
	}
	close(st.block)
	if d := g.Decide(context.Background(), input([]byte(validSignals))); hasTag(d, "DEGRADED") {
		t.Fatalf("breaker opened on a client abort (threshold 1): %v", d.Tags)
	}
}

// X-2: a burst of timeouts within a few ms (shuffle.lua blocking Redis at T0) degrades those joins one by one
// but does not open the breaker, so history rules come back on the next call.
func TestFix1_X2_ShortStallDoesNotOpenBreaker(t *testing.T) {
	st := &scriptedStore{}
	st.push(timeoutErr{}, 60)
	e := scripted(t, st, "ANTIBOT_BREAKER", "20/1s,open=3s")
	g := e.g
	for i := 0; i < 60; i++ {
		g.Decide(context.Background(), input([]byte(validSignals)))
	}
	if v := degradedTotal(t, e, "redis_timeout"); v != 60 {
		t.Fatalf("redis_timeout = %v, want 60", v)
	}
	if v := degradedTotal(t, e, "breaker_open"); v != 0 {
		t.Fatalf("breaker opened on a short stall (breaker_open = %v)", v)
	}
	if d := g.Decide(context.Background(), input([]byte(validSignals))); hasTag(d, "DEGRADED") || st.n() != 61 {
		t.Fatalf("after the stall: tags %v calls %d, want Redis used again", d.Tags, st.n())
	}
}

// X-2: slowness that lasts (failures spanning >= 250 ms) does open the breaker.
func TestFix1_X2_LastingSlownessOpensBreaker(t *testing.T) {
	st := &scriptedStore{}
	st.push(timeoutErr{}, 25)
	e := scripted(t, st, "ANTIBOT_BREAKER", "20/1s,open=3s")
	g := e.g
	for i := 0; i < 20; i++ {
		g.Decide(context.Background(), input([]byte(validSignals)))
	}
	time.Sleep(300 * time.Millisecond)
	g.Decide(context.Background(), input([]byte(validSignals))) // 21st failure, span >= 250 ms
	before := st.n()
	if d := g.Decide(context.Background(), input([]byte(validSignals))); !hasTag(d, "DEGRADED") || st.n() != before {
		t.Fatalf("breaker did not open after lasting slowness: calls %d->%d tags %v", before, st.n(), d.Tags)
	}
	if v := degradedTotal(t, e, "breaker_open"); v != 1 {
		t.Fatalf("breaker_open = %v, want 1", v)
	}
}

// X-2: pool exhaustion (local pressure, not a Redis outage) is counted but never opens the breaker.
func TestFix1_X2_PoolExhaustionNeverOpensBreaker(t *testing.T) {
	st := &scriptedStore{}
	st.push(fmt.Errorf("%w: redis: connection pool timeout", antibot.ErrPoolExhausted), 100)
	e := scripted(t, st, "ANTIBOT_BREAKER", "5/1s,open=3s")
	g := e.g
	for i := 0; i < 100; i++ {
		g.Decide(context.Background(), input([]byte(validSignals)))
		if i == 50 {
			time.Sleep(300 * time.Millisecond)
		}
	}
	if v := degradedTotal(t, e, "breaker_open"); v != 0 || st.n() != 100 {
		t.Fatalf("pool exhaustion opened the breaker: breaker_open=%v calls=%d", v, st.n())
	}
	if v := degradedTotal(t, e, "redis_timeout"); v != 100 {
		t.Fatalf("pool exhaustion counted as redis_timeout %v times, want 100", v)
	}
}

// X-2: half-open lets exactly one probe through; a failed probe re-opens with a doubled open time; a
// successful probe closes the breaker.
func TestFix1_X2_HalfOpenProbe(t *testing.T) {
	st := &scriptedStore{}
	down := errors.New("connection refused")
	st.push(down, 5)
	e := scripted(t, st, "ANTIBOT_BREAKER", "5/1s,open=200ms")
	g := e.g
	for i := 0; i < 5; i++ {
		g.Decide(context.Background(), input([]byte(validSignals)))
	}
	time.Sleep(250 * time.Millisecond)

	// one probe in flight: concurrent calls do not reach Redis
	st.mu.Lock()
	st.block = make(chan struct{})
	st.errs = []error{down}
	st.mu.Unlock()
	done := make(chan struct{})
	go func() { g.Decide(context.Background(), input([]byte(validSignals))); close(done) }()
	time.Sleep(20 * time.Millisecond)
	for i := 0; i < 5; i++ {
		g.Decide(context.Background(), input([]byte(validSignals)))
	}
	if st.n() != 6 {
		t.Fatalf("half-open let %d calls through, want exactly one probe", st.n()-5)
	}
	st.mu.Lock()
	close(st.block)
	st.block = nil
	st.mu.Unlock()
	<-done

	// failed probe -> open again for 2 x 200 ms
	time.Sleep(250 * time.Millisecond)
	g.Decide(context.Background(), input([]byte(validSignals)))
	if st.n() != 6 {
		t.Fatalf("breaker re-opened for less than the doubled time: calls %d", st.n())
	}
	time.Sleep(250 * time.Millisecond)
	if d := g.Decide(context.Background(), input([]byte(validSignals))); st.n() != 7 || hasTag(d, "DEGRADED") {
		t.Fatalf("successful probe did not close the breaker: calls %d tags %v", st.n(), d.Tags)
	}
	if d := g.Decide(context.Background(), input([]byte(validSignals))); st.n() != 8 || hasTag(d, "DEGRADED") {
		t.Fatalf("breaker not closed after the probe: calls %d tags %v", st.n(), d.Tags)
	}
}

// R-5: production refuses every committed dev key; dev still accepts the compose / local value.
func TestFix1_R5_ProductionRefusesDevKeys(t *testing.T) {
	committed := "devAntibotHashKeyLocalClusterOnlyNeverProdA=" // deploy/compose, overlays/local/secrets.dev.env
	builtin := base64.StdEncoding.EncodeToString([]byte("eventflow-antibot-dev-key-000000"))
	bytesDev := base64.StdEncoding.EncodeToString([]byte("0123456789-DEV-ANTIBOT-0123456789")) // marker in the decoded bytes, any case
	for _, k := range []string{committed, builtin, bytesDev} {
		_, err := antibot.LoadConfig(env(map[string]string{"APP_ENV": "production", "ANTIBOT_HASH_KEY": k}))
		if err == nil || !strings.Contains(err.Error(), "ANTIBOT_HASH_KEY") {
			t.Errorf("production accepted dev key %q (err %v)", k, err)
		}
	}
	if _, err := antibot.LoadConfig(env(map[string]string{"ANTIBOT_HASH_KEY": committed})); err != nil {
		t.Fatalf("development rejected the committed dev key: %v", err)
	}
	if _, err := antibot.LoadConfig(env(map[string]string{"APP_ENV": "production", "ANTIBOT_HASH_KEY": goodKey})); err != nil {
		t.Fatalf("production rejected a real key: %v", err)
	}
}

// ---- Amendment 2 revision (X-4, X-5; X-6 withdrawn): now the built-in defaults (TCR-6 approved) -------------

// amendment2 restates the approved configuration explicitly; it equals the built-in defaults, so the tests below
// also pin the defaults.
var amendment2 = []string{
	"ANTIBOT_WEIGHTS", "R5=0.85,R4D=0.60",
	"ANTIBOT_UA_DENY", "HeadlessChrome,PhantomJS,python-requests,python-httpx,aiohttp,curl/,Wget,Scrapy,Puppeteer,Playwright,Selenium",
	"ANTIBOT_UA_WEAK", "okhttp,Go-http-client,axios/,node-fetch",
}

func cfgA2(t testing.TB, kv ...string) antibot.Config {
	return cfgWith(t, append(append([]string{}, amendment2...), kv...)...)
}

// Amendment 2 worked examples (contracts 3, revision 4).
func TestFix1_Amendment2_WorkedExamples(t *testing.T) {
	pe1 := []byte(`{"v":1,"pe":1}`)
	cases := []struct {
		name   string
		raw    []byte
		ua     string
		st     func(*antibot.State)
		fired  string
		score  float64
		action antibot.Action
		capped bool
	}{
		{"X-4 python-requests + forged pe:1", pe1, "python-requests/2", nil, "R5", 0.85, block, false},
		{"X-4 wd true alone", payload(t, map[string]any{"wd": true}), "", nil, "R5", 0.85, block, false},
		{"missing payload, browser", nil, "", nil, "R3,R5", 0.91, slowLane, true},
		{"raw HTTP python-requests", nil, "python-requests/2.32.3", nil, "R3,R5", 0.91, block, false},
		{"X-5 okhttp, no signals", nil, "okhttp/4.12", nil, "R3,R5", 0.91, slowLane, true},
		{"X-5 okhttp, valid signals", []byte(validSignals), "okhttp/4.12", nil, "", 0, pass, false},
		{"X-5 okhttp + privacy browser sign", payload(t, map[string]any{"gl": false}), "okhttp/4.12", nil, "R5W", 0.45, challenge, false},
		{"headless default", payload(t, map[string]any{"wd": true, "rt": 40, "pe": 1, "se": 0, "ke": 0}), "", nil, "R2,R5", 0.91, block, false},
		{"shared fp only (X-6 withdrawn, R4D 0.60)", []byte(validSignals), "", func(s *antibot.State) { s.FPIdentities = 4 }, "R4D", 0.60, slowLane, false},
		{"account farm", []byte(validSignals), "", func(s *antibot.State) { s.FPIdentities = 4; s.NetIdentities = 31 }, "R4D,R4N", 0.70, slowLane, false},
		{"shared fp + privacy browser", payload(t, map[string]any{"gl": false, "fc": 2}), "", func(s *antibot.State) { s.FPIdentities = 4 }, "R4D,R5W", 0.78, slowLane, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			in := input(c.raw)
			if c.ua != "" {
				in.UserAgent = c.ua
			}
			st := neutral()
			if c.st != nil {
				c.st(&st)
			}
			d := eval(t, cfgA2(t), in, st)
			if dd := eval(t, cfgWith(t), in, st); dd.Score != d.Score || dd.Action != d.Action {
				t.Fatalf("built-in defaults differ from Amendment 2: %v %s vs %v %s", dd.Score, dd.Action, d.Score, d.Action)
			}
			if ids(d) != c.fired || math.Abs(d.Score-c.score) > 0.0011 || d.Action != c.action || hasTag(d, "CAPPED") != c.capped {
				t.Fatalf("fired %s score %v action %s tags %v, want %s %v %s capped=%v", ids(d), d.Score, d.Action, d.Tags, c.fired, c.score, c.action, c.capped)
			}
		})
	}
}

// Amendment 2 corpus: per-family table; the AC-18 gate holds with the approved configuration.
func TestFix1_Amendment2_Corpus(t *testing.T) {
	c := corpus.Generate(corpus.DefaultSeed)
	res := replayCorpus(t, cfgA2(t), c)
	fam := map[string][2]int{}
	for i := range c.Bots {
		s := &c.Bots[i]
		f := fam[s.Family]
		f[0]++
		if demoted(res[s].result) {
			f[1]++
		}
		fam[s.Family] = f
	}
	var b strings.Builder
	for _, f := range append(append([]string{}, corpus.GatedBotFamilies...), corpus.FamilyReplayResidential) {
		fmt.Fprintf(&b, "\n%-22s %d/%d demoted", f, fam[f][1], fam[f][0])
	}
	r := summarize(c, res)
	t.Logf("Amendment 2:%s\nbots caught %.2f%% (%d/%d), humans demoted %.2f%% (%d/%d)", b.String(),
		100*r.bot(), r.botsCaught, r.botsTotal, 100*r.human(), r.humansDemoted, r.humansTotal)
	if r.bot() < 0.95 || r.human() >= 0.01 {
		t.Fatalf("Amendment 2 misses the AC-18 gate: bots %.2f%% humans %.2f%%", 100*r.bot(), 100*r.human())
	}
}
