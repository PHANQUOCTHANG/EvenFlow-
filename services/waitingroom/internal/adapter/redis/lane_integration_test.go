//go:build integration

package redisadapter_test

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/eventflow/eventflow/libs/go/testx"
	redisadapter "github.com/eventflow/eventflow/services/waitingroom/internal/adapter/redis"
	"github.com/eventflow/eventflow/services/waitingroom/internal/domain"
)

func TestMain(m *testing.M) {
	code := m.Run()
	testx.Shutdown()
	os.Exit(code)
}

const (
	P        = int64(100000) // ANTIBOT_SLOW_LANE_PENALTY default; also shuffle.lua's default ARGV[3]
	offset   = int64(1_000_000_000_000)
	epochVal = 7
)

type env struct {
	q   *redisadapter.Queue
	rdb *redis.Client
	ev  string
	ctx context.Context
}

var n int

func setup(t *testing.T) *env {
	t.Helper()
	rdb := redis.NewClient(&redis.Options{Addr: testx.Redis(t)})
	t.Cleanup(func() { _ = rdb.Close() })
	q, err := redisadapter.NewQueue(context.Background(), rdb, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	n++
	return &env{q: q, rdb: rdb, ev: fmt.Sprintf("lane-%s-%d", strings.ReplaceAll(t.Name(), "/", "-"), n), ctx: context.Background()}
}

func (e *env) k(s string) string { return "wr:{" + e.ev + "}:" + s }

func (e *env) afterT0() { e.rdb.HSet(e.ctx, e.k("meta"), "sale_start_ms", "1") }
func (e *env) beforeT0() {
	e.rdb.HSet(e.ctx, e.k("meta"), "sale_start_ms", time.Now().Add(time.Hour).UnixMilli())
}

func (e *env) join(t *testing.T, id string, lane domain.Lane) redisadapter.JoinResult {
	t.Helper()
	r, err := e.q.JoinLane(e.ctx, e.ev, id, "tok-"+id, lane, P)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func (e *env) score(t *testing.T, tok string) int64 {
	t.Helper()
	s, err := e.rdb.ZScore(e.ctx, e.k("queue"), tok).Result()
	if err != nil {
		t.Fatalf("ZSCORE %s: %v", tok, err)
	}
	return int64(s)
}

func (e *env) rank(t *testing.T, tok string) int64 {
	t.Helper()
	r, err := e.rdb.ZRank(e.ctx, e.k("queue"), tok).Result()
	if err != nil {
		t.Fatalf("ZRANK %s: %v", tok, err)
	}
	return r + 1
}

func (e *env) status(t *testing.T, tok string) domain.State {
	t.Helper()
	p, err := e.q.Status(e.ctx, e.ev, tok)
	if err != nil {
		t.Fatal(err)
	}
	return p.State
}

func (e *env) shuffle(t *testing.T, seed int64) int64 {
	t.Helper()
	e.rdb.Set(e.ctx, e.k("leader_epoch"), epochVal, 0)
	n, err := e.q.Fenced(epochVal).Shuffle(e.ctx, e.ev, seed)
	if err != nil {
		t.Fatal(err)
	}
	return n
}

// AC-9 / P-4: JoinLane(normal) is the develop join (same state and score shape).
func TestLane_AC9_NormalLaneIsDevelop(t *testing.T) {
	e := setup(t)
	e.afterT0()
	a := e.join(t, "a", domain.LaneNormal)
	r, err := e.q.Join(e.ctx, e.ev, "b", "tok-b")
	if err != nil {
		t.Fatal(err)
	}
	if a.State != domain.StateQueued || r.State != domain.StateQueued || a.Rank != 1 || r.Rank != 2 {
		t.Fatalf("normal lane %+v / 4-arg %+v", a, r)
	}
	if e.score(t, "tok-a") != offset+1 || e.score(t, "tok-b") != offset+2 {
		t.Fatalf("scores %d %d", e.score(t, "tok-a"), e.score(t, "tok-b"))
	}
}

// AC-11: post-T0 slow score = 1e12 + seq + P + m (m = slow lobby size at shuffle, 0 without a shuffle).
func TestLane_AC11_SlowAfterT0IsPLater(t *testing.T) {
	e := setup(t)
	e.afterT0()
	for i := 0; i < 5; i++ {
		e.join(t, fmt.Sprintf("n%d", i), domain.LaneNormal)
	}
	s := e.join(t, "slow", domain.LaneSlow)
	if s.State != domain.StateQueued || s.Token != "tok-slow" || !s.IsNew {
		t.Fatalf("slow join %+v, want a normal-looking QUEUED join", s)
	}
	if got := e.score(t, "tok-slow"); got != offset+6+P {
		t.Fatalf("slow score %d, want 1e12+6+P", got)
	}
	for i := 0; i < 200; i++ {
		e.join(t, fmt.Sprintf("m%03d", i), domain.LaneNormal)
	}
	// behind every normal joiner with seq < 6+P, i.e. all 205 here
	if r := e.rank(t, "tok-slow"); r != 206 {
		t.Fatalf("slow rank %d, want 206 (behind all 205 normal joiners)", r)
	}
}

// AC-11 / BR-Q3: idempotent re-join returns the same token and rank in each lane; a slot never moves lanes.
func TestLane_AC11_IdempotentRejoinPerLane(t *testing.T) {
	e := setup(t)
	e.afterT0()
	n1 := e.join(t, "norm", domain.LaneNormal)
	s1 := e.join(t, "slow", domain.LaneSlow)
	scN, scS := e.score(t, n1.Token), e.score(t, s1.Token)
	for _, lane := range []domain.Lane{domain.LaneNormal, domain.LaneSlow} {
		r, err := e.q.JoinLane(e.ctx, e.ev, "norm", "other-tok", lane, P)
		if err != nil || r.Token != n1.Token || r.Rank != n1.Rank || r.IsNew {
			t.Fatalf("normal re-join via %s: %+v %v, want %+v", lane, r, err, n1)
		}
		r, err = e.q.JoinLane(e.ctx, e.ev, "slow", "other-tok", lane, P)
		if err != nil || r.Token != s1.Token || r.IsNew {
			t.Fatalf("slow re-join via %s: %+v %v", lane, r, err)
		}
	}
	if e.score(t, n1.Token) != scN || e.score(t, s1.Token) != scS {
		t.Fatal("re-join moved a slot")
	}
}

// AC-11: before T0 a slow join is LOBBY (lobby_slow), re-join is idempotent, status LOBBY.
func TestLane_AC11_SlowBeforeT0IsLobby(t *testing.T) {
	e := setup(t)
	e.beforeT0()
	r := e.join(t, "s", domain.LaneSlow)
	if r.State != domain.StateLobby || r.Rank != -1 {
		t.Fatalf("pre-T0 slow join %+v, want LOBBY -1", r)
	}
	if !e.rdb.SIsMember(e.ctx, e.k("lobby_slow"), r.Token).Val() || e.rdb.SIsMember(e.ctx, e.k("lobby"), r.Token).Val() {
		t.Fatal("slow member not in lobby_slow only")
	}
	if st := e.status(t, r.Token); st != domain.StateLobby {
		t.Fatalf("status %s, want LOBBY", st)
	}
	again := e.join(t, "s", domain.LaneNormal)
	if again.Token != r.Token || again.State != domain.StateLobby || again.IsNew {
		t.Fatalf("re-join %+v", again)
	}
}

// AC-11: after the lottery every slow member ranks behind all lottery winners and the first P post-T0 joiners;
// lottery_size counts the main lane only; lobby_slow is gone; slow_lottery_size = m.
func TestLane_AC11_ShufflePlacesSlowLobbyBehind(t *testing.T) {
	e := setup(t)
	e.beforeT0()
	for i := 0; i < 50; i++ {
		e.join(t, fmt.Sprintf("n%02d", i), domain.LaneNormal)
	}
	for i := 0; i < 10; i++ {
		e.join(t, fmt.Sprintf("s%02d", i), domain.LaneSlow)
	}
	e.shuffle(t, 42)
	if v := e.rdb.HGet(e.ctx, e.k("meta"), "lottery_size").Val(); v != "50" {
		t.Fatalf("lottery_size %q, want 50 (main lane only)", v)
	}
	if v := e.rdb.HGet(e.ctx, e.k("meta"), "slow_lottery_size").Val(); v != "10" {
		t.Fatalf("slow_lottery_size %q, want 10", v)
	}
	if e.rdb.Exists(e.ctx, e.k("lobby_slow")).Val() != 0 {
		t.Fatal("lobby_slow not deleted")
	}
	for i := 0; i < 10; i++ {
		tok := fmt.Sprintf("tok-s%02d", i)
		sc := e.score(t, tok)
		if sc < offset+P+1 || sc > offset+P+10 {
			t.Fatalf("slow lobby score %d, want 1e12+P+[1..10]", sc)
		}
		if st := e.status(t, tok); st != domain.StateQueued {
			t.Fatalf("slow member status %s after shuffle", st)
		}
	}
	// post-T0 slow joiners come after every pre-T0 slow member (P-13)
	e.afterT0()
	late := e.join(t, "late-slow", domain.LaneSlow)
	if e.score(t, late.Token) <= offset+P+10 {
		t.Fatalf("post-T0 slow score %d not behind the slow lobby", e.score(t, late.Token))
	}
	for i := 0; i < 50; i++ {
		if sc := e.score(t, fmt.Sprintf("tok-n%02d", i)); sc < 1 || sc > 50 {
			t.Fatalf("lottery winner score %d", sc)
		}
	}
	// a second shuffle is a no-op
	e.shuffle(t, 43)
	if e.score(t, "tok-s00") > offset+P+10 {
		t.Fatal("second shuffle moved the slow lobby")
	}
}

// AC-11 (R2, P-5): the slow lobby is placed even when the main lobby is empty.
func TestLane_AC11_ShuffleWithEmptyMainLobby(t *testing.T) {
	e := setup(t)
	e.beforeT0()
	for i := 0; i < 3; i++ {
		e.join(t, fmt.Sprintf("s%d", i), domain.LaneSlow)
	}
	e.shuffle(t, 1)
	for i := 0; i < 3; i++ {
		if st := e.status(t, fmt.Sprintf("tok-s%d", i)); st != domain.StateQueued {
			t.Fatalf("slow member %d stranded in %s with an empty main lobby", i, st)
		}
	}
	if e.rdb.HGet(e.ctx, e.k("meta"), "shuffled").Val() != "1" {
		t.Fatal("shuffled flag not set")
	}
}

// BR-Q1: within each lane every member is equally likely to take each position (chi-square, 1 000 runs).
func TestLane_BRQ1_FairnessPerLane(t *testing.T) {
	const runs, k = 1000, 5
	var cn, cs [k]int
	rdb := redis.NewClient(&redis.Options{Addr: testx.Redis(t)})
	t.Cleanup(func() { _ = rdb.Close() })
	q, err := redisadapter.NewQueue(context.Background(), rdb, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	for run := 0; run < runs; run++ {
		ev := fmt.Sprintf("fair-%d-%d", time.Now().UnixNano(), run)
		kk := func(s string) string { return "wr:{" + ev + "}:" + s }
		rdb.HSet(ctx, kk("meta"), "sale_start_ms", time.Now().Add(time.Hour).UnixMilli())
		for i := 0; i < k; i++ {
			if _, err := q.JoinLane(ctx, ev, fmt.Sprintf("n%d", i), fmt.Sprintf("tn%d", i), domain.LaneNormal, P); err != nil {
				t.Fatal(err)
			}
			if _, err := q.JoinLane(ctx, ev, fmt.Sprintf("s%d", i), fmt.Sprintf("ts%d", i), domain.LaneSlow, P); err != nil {
				t.Fatal(err)
			}
		}
		rdb.Set(ctx, kk("leader_epoch"), epochVal, 0)
		if _, err := q.Fenced(epochVal).Shuffle(ctx, ev, int64(run*7919+13)); err != nil {
			t.Fatal(err)
		}
		cn[int(rdb.ZScore(ctx, kk("queue"), "tn0").Val())-1]++
		cs[int(rdb.ZScore(ctx, kk("queue"), "ts0").Val()-float64(offset+P))-1]++
	}
	for name, c := range map[string][k]int{"normal": cn, "slow": cs} {
		exp := float64(runs) / k
		var chi float64
		for _, o := range c {
			chi += (float64(o) - exp) * (float64(o) - exp) / exp
		}
		t.Logf("%s lane positions of member 0: %v chi2=%.2f", name, c, chi)
		if chi > 13.28 { // df 4, p = 0.01
			t.Errorf("%s lane not uniform: chi2 %.2f > 13.28", name, chi)
		}
	}
}

// AC-12 (R2, D6): Revoke removes the slot from every structure, sets the 24 h flag atomically; old token UNKNOWN.
func TestLane_AC12_RevokeEverywhere(t *testing.T) {
	cases := []struct {
		name  string
		setup func(e *env, t *testing.T) string
	}{
		{"lobby", func(e *env, t *testing.T) string { e.beforeT0(); return e.join(t, "x", domain.LaneNormal).Token }},
		{"slow lobby", func(e *env, t *testing.T) string { e.beforeT0(); return e.join(t, "x", domain.LaneSlow).Token }},
		{"queue", func(e *env, t *testing.T) string { e.afterT0(); return e.join(t, "x", domain.LaneSlow).Token }},
		{"admitted", func(e *env, t *testing.T) string {
			e.afterT0()
			tok := e.join(t, "x", domain.LaneNormal).Token
			e.rdb.Set(e.ctx, e.k("leader_epoch"), epochVal, 0)
			if _, err := e.q.Fenced(epochVal).Admit(e.ctx, e.ev, 10, time.Minute); err != nil {
				t.Fatal(err)
			}
			if st := e.status(t, tok); st != domain.StateAdmitted {
				t.Fatalf("setup: %s", st)
			}
			return tok
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := setup(t)
			tok := c.setup(e, t)
			ok, err := e.q.Revoke(e.ctx, e.ev, "x")
			if err != nil || !ok {
				t.Fatalf("Revoke = %v, %v; want true", ok, err)
			}
			if st := e.status(t, tok); st != domain.StateUnknown {
				t.Fatalf("old token status %s, want UNKNOWN", st)
			}
			for _, key := range []string{"lobby", "lobby_slow"} {
				if e.rdb.SIsMember(e.ctx, e.k(key), tok).Val() {
					t.Errorf("token still in %s", key)
				}
			}
			for _, key := range []string{"queue", "admitted"} {
				if e.rdb.ZScore(e.ctx, e.k(key), tok).Err() != redis.Nil {
					t.Errorf("token still in %s", key)
				}
			}
			if e.rdb.HExists(e.ctx, e.k("tok"), "x").Val() || e.rdb.HExists(e.ctx, e.k("sess"), tok).Val() {
				t.Error("tok / sess entry not removed")
			}
			ttl := e.rdb.TTL(e.ctx, "ab:{"+e.ev+"}:blk:x").Val()
			if ttl < 86000*time.Second || ttl > 86400*time.Second {
				t.Errorf("block flag TTL %s, want 24 h", ttl)
			}
		})
	}
}

// AC-12: revoking an identity without a slot returns false and still sets the flag.
func TestLane_AC12_RevokeAbsent(t *testing.T) {
	e := setup(t)
	ok, err := e.q.Revoke(e.ctx, e.ev, "nobody")
	if err != nil || ok {
		t.Fatalf("Revoke absent = %v, %v; want false, nil", ok, err)
	}
	if e.rdb.Exists(e.ctx, "ab:{"+e.ev+"}:blk:nobody").Val() != 1 {
		t.Fatal("flag not set")
	}
}

// D6 (R2): while the flag exists, JoinLane refuses a new slot (BLOCKED); the 4-arg Join ignores it.
func TestLane_D6_FlagBlocksLanePathOnly(t *testing.T) {
	e := setup(t)
	e.afterT0()
	e.rdb.Set(e.ctx, "ab:{"+e.ev+"}:blk:bad", "1", 24*time.Hour)
	for _, lane := range []domain.Lane{domain.LaneNormal, domain.LaneSlow} {
		r, err := e.q.JoinLane(e.ctx, e.ev, "bad", "tok-bad", lane, P)
		if err != nil || r.State != domain.State("BLOCKED") || r.Token != "" {
			t.Fatalf("flagged JoinLane(%s) = %+v, %v; want BLOCKED without token", lane, r, err)
		}
	}
	if e.rdb.HExists(e.ctx, e.k("tok"), "bad").Val() {
		t.Fatal("flagged identity got a slot")
	}
	r, err := e.q.Join(e.ctx, e.ev, "bad", "tok-bad")
	if err != nil || r.State != domain.StateQueued {
		t.Fatalf("4-arg Join with flag = %+v, %v; want unchanged develop behaviour", r, err)
	}
}

// D12: an existing slot is returned before the flag check (BR-Q3).
func TestLane_D12_ExistingSlotBeforeFlag(t *testing.T) {
	e := setup(t)
	e.afterT0()
	first := e.join(t, "h", domain.LaneNormal)
	e.rdb.Set(e.ctx, "ab:{"+e.ev+"}:blk:h", "1", time.Hour)
	r := e.join(t, "h", domain.LaneSlow)
	if r.Token != first.Token {
		t.Fatalf("existing slot not returned: %+v", r)
	}
}
