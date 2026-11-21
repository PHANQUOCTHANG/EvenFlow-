//go:build integration

package redisstate_test

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/netip"
	"os"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/redis/go-redis/v9"

	"github.com/eventflow/eventflow/libs/go/antibot"
	"github.com/eventflow/eventflow/libs/go/antibot/redisstate"
	"github.com/eventflow/eventflow/libs/go/testx"
)

func TestMain(m *testing.M) {
	code := m.Run()
	testx.Shutdown()
	os.Exit(code)
}

var hashKey = base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef"))

func cfg(t testing.TB, kv ...string) antibot.Config {
	t.Helper()
	m := map[string]string{"ANTIBOT_MODE": "enforce", "ANTIBOT_HASH_KEY": hashKey}
	for i := 0; i+1 < len(kv); i += 2 {
		m[kv[i]] = kv[i+1]
	}
	c, err := antibot.LoadConfig(func(k string) string { return m[k] })
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// counter is a go-redis hook counting commands sent to Redis (pipelines count each command).
type counter struct{ n atomic.Int64 }

func (c *counter) DialHook(next redis.DialHook) redis.DialHook { return next }
func (c *counter) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, cmd redis.Cmder) error { c.n.Add(1); return next(ctx, cmd) }
}
func (c *counter) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return func(ctx context.Context, cmds []redis.Cmder) error { c.n.Add(int64(len(cmds))); return next(ctx, cmds) }
}

type env struct {
	rdb   *redis.Client
	store *redisstate.Store
	cnt   *counter
	ev    string
	ctx   context.Context
}

// client uses the contracted antibot client options (contracts 4, R2).
func client(t testing.TB, timeout time.Duration) *redis.Client {
	rdb := redis.NewClient(&redis.Options{Addr: testx.Redis(t), ReadTimeout: timeout, WriteTimeout: timeout,
		PoolTimeout: timeout, ContextTimeoutEnabled: true, MaxRetries: -1, PoolSize: 128, MinIdleConns: 32, DialTimeout: 50 * time.Millisecond})
	t.Cleanup(func() { _ = rdb.Close() })
	return rdb
}

func setup(t testing.TB, kv ...string) *env {
	t.Helper()
	rdb := client(t, time.Second)
	c := &counter{}
	ctx := context.Background()
	st, err := redisstate.New(ctx, rdb, cfg(t, kv...))
	if err != nil {
		t.Fatal(err)
	}
	rdb.AddHook(c)
	return &env{rdb: rdb, store: st, cnt: c, ev: "abst-" + strings.ReplaceAll(t.Name(), "/", "-"), ctx: ctx}
}

var t0 = time.Date(2026, 12, 1, 12, 0, 0, 0, time.UTC)

func (e *env) in(id string, at time.Duration) antibot.Input {
	return antibot.Input{EventID: e.ev, IdentityID: id, DeviceID: "dev-" + id, UserAgent: "Mozilla/5.0",
		ClientIP: netip.MustParseAddr("203.0.113.77"), Now: t0.Add(at)}
}

var pfx = netip.MustParsePrefix("203.0.113.0/24")

const fph = "0123456789abcdef0123456789abcdef"

func (e *env) observe(t testing.TB, id string, at time.Duration, fp string, net netip.Prefix, sq string) antibot.State {
	t.Helper()
	st, err := e.store.Observe(e.ctx, e.in(id, at), fp, net, sq)
	if err != nil {
		t.Fatal(err)
	}
	return st
}

func (e *env) key(suffix string) string { return "ab:{" + e.ev + "}:" + suffix }

// AC-6: distinct identities are counted, repeats are not; state is known; R6 unknown.
func TestStore_AC6_CountsDistinctIdentities(t *testing.T) {
	e := setup(t)
	for i, id := range []string{"a", "b", "a", "c", "d"} {
		st := e.observe(t, id, time.Duration(i)*time.Second, fph, pfx, "00000000000000aa")
		if !st.Known || st.CardIdentities != -1 {
			t.Fatalf("state %+v: want Known, CardIdentities -1", st)
		}
	}
	st := e.observe(t, "d", 6*time.Second, fph, pfx, "00000000000000aa")
	if st.FPIdentities != 4 || st.NetIdentities != 4 || st.SeqIdentities != 4 {
		t.Fatalf("counts fp=%d net=%d seq=%d, want 4/4/4", st.FPIdentities, st.NetIdentities, st.SeqIdentities)
	}
}

// AC-6: R4 window (300 s) and R7 window (900 s) expire old identities; the clock is Input.Now (contracts 8.1).
func TestStore_AC6_WindowsUseInputNow(t *testing.T) {
	e := setup(t)
	for _, id := range []string{"a", "b", "c"} {
		e.observe(t, id, 0, fph, pfx, "00000000000000bb")
	}
	st := e.observe(t, "z", 301*time.Second, fph, pfx, "00000000000000bb")
	if st.FPIdentities != 1 || st.NetIdentities != 1 {
		t.Fatalf("after 301 s fp=%d net=%d, want 1/1", st.FPIdentities, st.NetIdentities)
	}
	if st.SeqIdentities != 4 {
		t.Fatalf("sq within 900 s: %d, want 4", st.SeqIdentities)
	}
	st = e.observe(t, "y", 1202*time.Second, fph, pfx, "00000000000000bb")
	if st.SeqIdentities != 2 { // z (301 s) still inside, a/b/c expired
		t.Fatalf("sq after 1202 s: %d, want 2", st.SeqIdentities)
	}
}

// AC-6: caps: ZSETs keep the newest 64, the join-time list 9 (8 gaps).
func TestStore_AC6_Caps(t *testing.T) {
	e := setup(t)
	for i := 0; i < 70; i++ {
		e.observe(t, fmt.Sprintf("id-%02d", i), time.Duration(i)*time.Millisecond, fph, pfx, "00000000000000cc")
	}
	for _, k := range []string{"fp:" + fph, "net:" + pfx.String(), "sq:00000000000000cc"} {
		if n := e.rdb.ZCard(e.ctx, e.key(k)).Val(); n > 64 {
			t.Errorf("%s holds %d members, cap 64", k, n)
		}
	}
	var st antibot.State
	for i := 0; i < 12; i++ {
		st = e.observe(t, "rhythm", time.Duration(i)*time.Second, "", netip.Prefix{}, "")
	}
	if n := e.rdb.LLen(e.ctx, e.key("jt:rhythm")).Val(); n != 9 {
		t.Errorf("jt list length %d, want 9", n)
	}
	if len(st.JoinGapsMs) != 8 || st.JoinGapsMs[0] != 1000 {
		t.Errorf("gaps %v, want 8 gaps of 1000 ms", st.JoinGapsMs)
	}
}

// AC-6: TTLs from contracts 4.
func TestStore_AC6_TTLs(t *testing.T) {
	e := setup(t)
	e.observe(t, "a", 0, fph, pfx, "00000000000000dd")
	check := func(pattern string, max time.Duration) {
		keys := e.rdb.Keys(e.ctx, e.key(pattern)).Val()
		if len(keys) == 0 {
			t.Errorf("no key %s", pattern)
		}
		for _, k := range keys {
			ttl := e.rdb.TTL(e.ctx, k).Val()
			if ttl <= 0 || ttl > max {
				t.Errorf("%s TTL %s, want (0, %s]", k, ttl, max)
			}
		}
	}
	check("jt:a", 300*time.Second)
	check("fp:*", 600*time.Second)
	check("net:*", 600*time.Second)
	check("sq:*", 1800*time.Second)
	check("jc:*", 360*time.Second)
	check("nj:*", 360*time.Second)
}

// AC-6 / AC-15: every key is under ab:{ev}: (one slot) and no key carries a raw fingerprint or full IP.
func TestStore_AC6_AC15_KeysAreScopedAndPrivate(t *testing.T) {
	e := setup(t)
	e.observe(t, "a", 0, fph, pfx, "00000000000000ee")
	e.observe(t, "b", time.Second, fph, netip.MustParsePrefix("2001:db8:1:2::/64"), "")
	keys := e.rdb.Keys(e.ctx, "*"+e.ev+"*").Val()
	if len(keys) == 0 {
		t.Fatal("no keys written")
	}
	for _, k := range keys {
		if !strings.HasPrefix(k, e.key("")) {
			t.Errorf("key %q outside ab:{ev}:", k)
		}
		for _, raw := range []string{"203.0.113.77", "device-", "Mozilla"} {
			if strings.Contains(k, raw) {
				t.Errorf("key %q contains raw value %q", k, raw)
			}
		}
	}
}

// AC-6: absent inputs are skipped (no key written).
func TestStore_AC6_AbsentInputsSkipped(t *testing.T) {
	e := setup(t)
	st := e.observe(t, "a", 0, "", netip.Prefix{}, "")
	if st.FPIdentities != 0 || st.NetIdentities != 0 || st.SeqIdentities != 0 {
		t.Fatalf("counts for absent inputs: %+v", st)
	}
	for _, p := range []string{"fp:*", "net:*", "sq:*"} {
		if n := len(e.rdb.Keys(e.ctx, e.key(p)).Val()); n != 0 {
			t.Errorf("%d %s keys for absent input", n, p)
		}
	}
}

// AC-6: all state for one decision in ONE Redis command (EVALSHA).
func TestStore_AC6_OneScriptCall(t *testing.T) {
	e := setup(t)
	e.observe(t, "warm", 0, fph, pfx, "00000000000000ff")
	before := e.cnt.n.Load()
	e.observe(t, "a", time.Second, fph, pfx, "00000000000000ff")
	if n := e.cnt.n.Load() - before; n != 1 {
		t.Fatalf("Observe sent %d commands, want 1", n)
	}
}

// D12: slot holders are reported (HEXISTS wr:{ev}:tok) and their joins do not feed R1-server.
func TestStore_D12_HasSlotAndNoJoinTimes(t *testing.T) {
	e := setup(t)
	e.rdb.HSet(e.ctx, "wr:{"+e.ev+"}:tok", "holder", "tok-1")
	for i := 0; i < 5; i++ {
		st := e.observe(t, "holder", time.Duration(i)*time.Second, "", netip.Prefix{}, "")
		if !st.HasSlot || len(st.JoinGapsMs) != 0 {
			t.Fatalf("holder state %+v, want HasSlot and no gaps", st)
		}
	}
	if n := e.rdb.Exists(e.ctx, e.key("jt:holder")).Val(); n != 0 {
		t.Fatal("join times recorded for a slot holder")
	}
}

// AC-12: the block flag makes the state sticky.
func TestStore_AC12_Sticky(t *testing.T) {
	e := setup(t)
	e.rdb.Set(e.ctx, e.key("blk:bad"), "1", 24*time.Hour)
	if st := e.observe(t, "bad", 0, fph, pfx, ""); !st.Sticky {
		t.Fatalf("state %+v, want Sticky", st)
	}
	if st := e.observe(t, "good", 0, fph, pfx, ""); st.Sticky {
		t.Fatal("other identity sticky")
	}
}

// N-2 (plan review round 2): prefix share = prefix joins / event joins over 5 minute buckets.
func TestStore_N2_NetSuppression(t *testing.T) {
	for _, c := range []struct {
		heavy int
		want  bool
	}{{2000, true}, {500, false}} {
		t.Run(fmt.Sprintf("%d_of_20000", c.heavy), func(t *testing.T) {
			e := setup(t)
			every := 20000 / c.heavy
			var last antibot.State
			for i := 0; i < 20000; i++ {
				at := time.Duration(i) * 10 * time.Millisecond // 200 s, inside the window
				id := fmt.Sprintf("u%05d", i)
				if i%every == 0 {
					last = e.observe(t, id, at, "", pfx, "")
					continue
				}
				other := netip.PrefixFrom(netip.AddrFrom4([4]byte{10, byte(i >> 8), byte(i), 0}), 24)
				e.observe(t, id, at, "", other, "")
			}
			if last.NetSuppressed != c.want {
				t.Fatalf("%d of 20000 joins from one prefix: suppressed=%v, want %v", c.heavy, last.NetSuppressed, c.want)
			}
		})
	}
}

// ---- guard with the real store -------------------------------------------------------------------------------

type nopSink struct{}

func (nopSink) Offer(antibot.Input, antibot.Decision) {}

const sig = `{"v":1,"rt":3500,"pe":25,"se":6,"ke":2,"iv":[310,120,840,95,2200,460,180,705,330],"fp":"device-abc-12345","wd":false,"gl":true,"fc":12,"tz":"Asia/Ho_Chi_Minh","tzo":420}`

func guard(t testing.TB, rdb *redis.Client, reg *prometheus.Registry) *antibot.Guard {
	t.Helper()
	c := cfg(t)
	st, err := redisstate.New(context.Background(), rdb, c)
	if err != nil {
		t.Fatal(err)
	}
	return antibot.NewGuard(c, st, nopSink{}, antibot.NewMetrics(reg))
}

func degraded(reg *prometheus.Registry, reason string) float64 {
	mfs, _ := reg.Gather()
	for _, mf := range mfs {
		if mf.GetName() != "antibot_degraded_total" {
			continue
		}
		for _, m := range mf.GetMetric() {
			for _, l := range m.GetLabel() {
				if l.GetName() == "reason" && l.GetValue() == reason {
					return m.GetCounter().GetValue()
				}
			}
		}
	}
	return 0
}

// AC-7: Redis paused longer than the 4 ms budget → DEGRADED redis_timeout, Decide stays fast.
func TestGuard_AC7_RedisTimeoutDegrades(t *testing.T) {
	rdb := client(t, 4*time.Millisecond)
	reg := prometheus.NewRegistry()
	g := guard(t, rdb, reg)
	admin := client(t, time.Second)
	if err := admin.Do(context.Background(), "CLIENT", "PAUSE", 300, "ALL").Err(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { admin.Do(context.Background(), "CLIENT", "UNPAUSE") })
	start := time.Now()
	in := antibot.Input{EventID: "abt-timeout", IdentityID: "a", UserAgent: "Mozilla/5.0", RawSignals: []byte(sig), Now: time.Now()}
	d := g.Decide(context.Background(), in)
	if el := time.Since(start); el > 60*time.Millisecond {
		t.Fatalf("Decide took %s while Redis paused", el)
	}
	if !slices.Contains(d.Tags, "DEGRADED") || degraded(reg, "redis_timeout") != 1 {
		t.Fatalf("decision %+v, degraded{redis_timeout}=%v", d, degraded(reg, "redis_timeout"))
	}
}

// AC-7: Redis unreachable → DEGRADED redis_error, never an error to the caller.
func TestGuard_AC7_RedisDownDegrades(t *testing.T) {
	good := client(t, time.Second)
	reg := prometheus.NewRegistry()
	c := cfg(t)
	st, err := redisstate.New(context.Background(), good, c)
	if err != nil {
		t.Fatal(err)
	}
	// reuse the loaded store, then break the connection target
	_ = good.Close()
	g := antibot.NewGuard(c, st, nopSink{}, antibot.NewMetrics(reg))
	d := g.Decide(context.Background(), antibot.Input{EventID: "abt-down", IdentityID: "a", UserAgent: "Mozilla/5.0", RawSignals: []byte(sig), Now: time.Now()})
	if !slices.Contains(d.Tags, "DEGRADED") || degraded(reg, "redis_error")+degraded(reg, "redis_timeout") != 1 {
		t.Fatalf("decision %+v", d)
	}
}

func p99(ds []time.Duration) time.Duration {
	slices.Sort(ds)
	return ds[len(ds)*99/100]
}

// AC-5: warm Redis, 10 000 sequential Decide (decode + one script + rules + score): p99 < 5 ms.
func TestGuard_AC5_DecideP99Sequential(t *testing.T) {
	rdb := client(t, 4*time.Millisecond)
	g := guard(t, rdb, prometheus.NewRegistry())
	ctx := context.Background()
	for i := 0; i < 500; i++ {
		g.Decide(ctx, antibot.Input{EventID: "abt-p99", IdentityID: fmt.Sprintf("w%d", i), UserAgent: "Mozilla/5.0", RawSignals: []byte(sig), ClientIP: netip.MustParseAddr("198.51.100.7"), Now: time.Now()})
	}
	ds := make([]time.Duration, 10000)
	for i := range ds {
		in := antibot.Input{EventID: "abt-p99", IdentityID: fmt.Sprintf("id-%d", i%2000), UserAgent: "Mozilla/5.0",
			RawSignals: []byte(sig), ClientIP: netip.AddrFrom4([4]byte{198, 51, byte(i % 50), 7}), Now: time.Now()}
		start := time.Now()
		g.Decide(ctx, in)
		ds[i] = time.Since(start)
	}
	p := p99(ds)
	t.Logf("sequential Decide p99 = %s", p)
	if p >= 5*time.Millisecond {
		t.Fatalf("p99 %s, want < 5 ms", p)
	}
}

// AC-5 (R2): the same under 64 concurrent goroutines.
func TestGuard_AC5_DecideP99Concurrent(t *testing.T) {
	rdb := client(t, 4*time.Millisecond)
	g := guard(t, rdb, prometheus.NewRegistry())
	ctx := context.Background()
	const workers, per = 64, 160
	var mu sync.Mutex
	var all []time.Duration
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			local := make([]time.Duration, 0, per)
			for i := 0; i < per; i++ {
				in := antibot.Input{EventID: "abt-p99c", IdentityID: fmt.Sprintf("w%d-%d", w, i), UserAgent: "Mozilla/5.0",
					RawSignals: []byte(sig), ClientIP: netip.AddrFrom4([4]byte{198, 51, byte(w), 9}), Now: time.Now()}
				start := time.Now()
				g.Decide(ctx, in)
				local = append(local, time.Since(start))
			}
			mu.Lock()
			all = append(all, local...)
			mu.Unlock()
		}(w)
	}
	wg.Wait()
	p := p99(all)
	t.Logf("64 goroutines Decide p99 = %s", p)
	if p >= 5*time.Millisecond {
		t.Fatalf("concurrent p99 %s, want < 5 ms", p)
	}
}
