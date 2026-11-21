//go:build integration

// Package antibot runs AC-19: the sample bot script (the corpus bot families, sent as HTTP joins) and a human
// replay against the waitingroom join handler with real Redis and the real guard in enforce mode.
package antibot

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/redis/go-redis/v9"

	"github.com/eventflow/eventflow/libs/go/antibot"
	"github.com/eventflow/eventflow/libs/go/antibot/corpus"
	"github.com/eventflow/eventflow/libs/go/antibot/redisstate"
	"github.com/eventflow/eventflow/libs/go/authx"
	"github.com/eventflow/eventflow/libs/go/testx"
	httpadapter "github.com/eventflow/eventflow/services/waitingroom/internal/adapter/http"
	redisadapter "github.com/eventflow/eventflow/services/waitingroom/internal/adapter/redis"
	"github.com/eventflow/eventflow/services/waitingroom/internal/domain"
)

func TestMain(m *testing.M) {
	code := m.Run()
	testx.Shutdown()
	os.Exit(code)
}

var secret = []byte("integration-test-secret-16bytes+")

const penalty = int64(100000)

// store adapts *redisadapter.Queue to QueueStore + LaneStore.
type store struct{ q *redisadapter.Queue }

func conv(r redisadapter.JoinResult) httpadapter.JoinResult {
	return httpadapter.JoinResult{State: r.State, Token: r.Token, Rank: r.Rank, IsNew: r.IsNew}
}

func (s store) Join(ctx context.Context, ev, id, tok string) (httpadapter.JoinResult, error) {
	r, err := s.q.Join(ctx, ev, id, tok)
	return conv(r), err
}
func (s store) JoinLane(ctx context.Context, ev, id, tok string, lane domain.Lane, p int64) (httpadapter.JoinResult, error) {
	r, err := s.q.JoinLane(ctx, ev, id, tok, lane, p)
	return conv(r), err
}
func (s store) Revoke(ctx context.Context, ev, id string) (bool, error) {
	return s.q.Revoke(ctx, ev, id)
}
func (s store) Status(ctx context.Context, ev, tok string) (domain.Position, error) {
	return s.q.Status(ctx, ev, tok)
}

type nopSink struct{}

func (nopSink) Offer(antibot.Input, antibot.Decision) {}

type env struct {
	mux http.Handler
	rdb *redis.Client
	reg *prometheus.Registry
	ev  string
}

func setup(t *testing.T, ev string) *env {
	t.Helper()
	addr := testx.Redis(t)
	rdb := redis.NewClient(&redis.Options{Addr: addr})
	t.Cleanup(func() { _ = rdb.Close() })
	q, err := redisadapter.NewQueue(context.Background(), rdb, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	key := base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef"))
	m := map[string]string{"ANTIBOT_MODE": "enforce", "ANTIBOT_HASH_KEY": key}
	cfg, err := antibot.LoadConfig(func(k string) string { return m[k] })
	if err != nil {
		t.Fatal(err)
	}
	abc := redis.NewClient(&redis.Options{Addr: addr, ReadTimeout: 50 * time.Millisecond, WriteTimeout: 50 * time.Millisecond,
		ContextTimeoutEnabled: true, MaxRetries: -1, PoolSize: 128})
	t.Cleanup(func() { _ = abc.Close() })
	st, err := redisstate.New(context.Background(), abc, cfg)
	if err != nil {
		t.Fatal(err)
	}
	reg := prometheus.NewRegistry()
	g := antibot.NewGuard(cfg, st, nopSink{}, antibot.NewMetrics(reg))
	mux := http.NewServeMux()
	httpadapter.New(store{q}, g, slog.New(slog.NewTextHandler(io.Discard, nil))).WithQueueTokens(secret, true).Routes(mux)
	rdb.HSet(context.Background(), "wr:{"+ev+"}:meta", "sale_start_ms", "1") // after T0
	return &env{mux: mux, rdb: rdb, reg: reg, ev: ev}
}

// join posts one corpus join and returns the lane outcome observed in Redis.
func (e *env) join(t *testing.T, s *corpus.Session, j corpus.Join) antibot.Action {
	t.Helper()
	body := `{}`
	if j.Signals != nil {
		b, _ := json.Marshal(map[string]json.RawMessage{"signals": j.Signals})
		body = string(b)
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/events/"+e.ev+"/queue/join", strings.NewReader(body))
	req.Header.Set(authx.HeaderIdentityID, asUUID(s.IdentityID))
	req.Header.Set(authx.HeaderIdentityRole, "buyer")
	req.Header.Set(authx.HeaderIdentityVerified, "true")
	req.Header.Set(authx.HeaderDeviceID, asUUID(s.DeviceID))
	req.Header.Set("User-Agent", j.UserAgent)
	req.Header.Set("X-Forwarded-For", j.IP)
	rec := httptest.NewRecorder()
	e.mux.ServeHTTP(rec, req)
	switch rec.Code {
	case http.StatusForbidden:
		var p struct{ Type string }
		_ = json.Unmarshal(rec.Body.Bytes(), &p)
		if p.Type != "/problems/request-denied" {
			t.Fatalf("403 type %q", p.Type)
		}
		return "BLOCK"
	case http.StatusOK:
	default:
		t.Fatalf("join status %d: %s", rec.Code, rec.Body)
	}
	var r struct {
		QueueToken string `json:"queue_token"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &r)
	c, err := authx.VerifyQueueToken(secret, r.QueueToken, time.Now())
	if err != nil {
		t.Fatalf("token: %v", err)
	}
	sc, err := e.rdb.ZScore(context.Background(), "wr:{"+e.ev+"}:queue", c.JTI).Result()
	if err != nil {
		t.Fatalf("slot of %s: %v", s.IdentityID, err)
	}
	if int64(sc) > 1_000_000_000_000+penalty {
		return "SLOW_LANE"
	}
	return "PASS"
}

func run(t *testing.T, e *env, sessions []*corpus.Session, joins []corpus.SessionJoin) map[*corpus.Session]antibot.Action {
	want := map[*corpus.Session]bool{}
	for _, s := range sessions {
		want[s] = true
	}
	res := map[*corpus.Session]antibot.Action{}
	for _, sj := range joins {
		if !want[sj.Session] {
			continue
		}
		a := e.join(t, sj.Session, sj.Join())
		prev, seen := res[sj.Session]
		switch {
		case a == "BLOCK":
			res[sj.Session] = "BLOCK"
		case !seen:
			res[sj.Session] = a
		default:
			res[sj.Session] = prev
		}
	}
	return res
}

// AC-19: >= 95% of the bot script's sessions end SLOW_LANE or BLOCK in enforce; the human replay stays < 1%.
func TestAC19_BotScriptAndHumanReplay(t *testing.T) {
	c := corpus.Generate(corpus.DefaultSeed)
	joins := c.AllJoins()
	e := setup(t, fmt.Sprintf("ac19-%d", time.Now().UnixNano()))

	var all []*corpus.Session
	for i := range c.Bots {
		all = append(all, &c.Bots[i])
	}
	for i := range c.Humans {
		all = append(all, &c.Humans[i])
	}
	res := run(t, e, all, joins)

	var bots, caught, humans, demoted int
	perFam := map[string][2]int{}
	for i := range c.Bots {
		s := &c.Bots[i]
		gated := false
		for _, f := range corpus.GatedBotFamilies {
			gated = gated || f == s.Family
		}
		d := res[s] == "SLOW_LANE" || res[s] == "BLOCK"
		pf := perFam[s.Family]
		pf[0]++
		if d {
			pf[1]++
		}
		perFam[s.Family] = pf
		if !gated {
			continue
		}
		bots++
		if d {
			caught++
		}
	}
	for i := range c.Humans {
		humans++
		if r := res[&c.Humans[i]]; r == "SLOW_LANE" || r == "BLOCK" {
			demoted++
		}
	}
	for f, v := range perFam {
		t.Logf("%-20s %d/%d demoted", f, v[1], v[0])
	}
	t.Logf("bots caught %d/%d, humans demoted %d/%d", caught, bots, demoted, humans)
	if float64(caught)/float64(bots) < 0.95 {
		t.Errorf("bot script: %.2f%% demoted, want >= 95%%", 100*float64(caught)/float64(bots))
	}
	if float64(demoted)/float64(humans) >= 0.01 {
		t.Errorf("human replay: %.2f%% demoted, want < 1%%", 100*float64(demoted)/float64(humans))
	}
}

// AC-12 end to end: a BLOCK revokes, later joins within 24 h stay BLOCK even with clean signals.
func TestAC12_StickyEndToEnd(t *testing.T) {
	e := setup(t, fmt.Sprintf("ac12-%d", time.Now().UnixNano()))
	s := &corpus.Session{IdentityID: "55555555-5555-4555-8555-555555555555", DeviceID: "66666666-6666-4666-8666-666666666666"}
	clean := json.RawMessage(`{"v":1,"rt":4000,"pe":20,"se":3,"ke":1,"fp":"clean-device-01","wd":false,"gl":true,"fc":12,"tz":"Asia/Ho_Chi_Minh","tzo":420}`)
	ua := "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"
	if got := e.join(t, s, corpus.Join{UserAgent: ua, IP: "198.51.100.10", Signals: clean}); got != "PASS" {
		t.Fatalf("clean first join: %s", got)
	}
	bad := json.RawMessage(`{"v":1,"rt":20,"pe":1,"wd":true}`)
	if got := e.join(t, s, corpus.Join{UserAgent: ua, IP: "198.51.100.10", Signals: bad}); got != "BLOCK" {
		t.Fatalf("headless re-join: %s, want BLOCK (revoke)", got)
	}
	if n := e.rdb.HExists(context.Background(), "wr:{"+e.ev+"}:tok", s.IdentityID).Val(); n {
		t.Fatal("slot not revoked")
	}
	if got := e.join(t, s, corpus.Join{UserAgent: ua, IP: "198.51.100.10", Signals: clean}); got != "BLOCK" {
		t.Fatalf("clean join after BLOCK: %s, want sticky BLOCK", got)
	}
}

// AC-3 end to end: Decide counts antibot_signals_invalid_total for bodies the handler passes through.
func TestAC3_InvalidSignalsCountedEndToEnd(t *testing.T) {
	e := setup(t, fmt.Sprintf("ac3-%d", time.Now().UnixNano()))
	s := &corpus.Session{IdentityID: "77777777-7777-4777-8777-777777777777", DeviceID: "88888888-8888-4888-8888-888888888888"}
	ua := "Mozilla/5.0 Chrome/131"
	e.join(t, s, corpus.Join{UserAgent: ua, IP: "198.51.100.11", Signals: json.RawMessage(`{}`)})
	mfs, _ := e.reg.Gather()
	var total float64
	for _, mf := range mfs {
		if mf.GetName() == "antibot_signals_invalid_total" {
			for _, m := range mf.GetMetric() {
				total += m.GetCounter().GetValue()
			}
		}
	}
	if total != 1 {
		t.Fatalf("antibot_signals_invalid_total = %v, want 1", total)
	}
}

// asUUID maps a corpus id (e.g. "hum-000001") to a distinct, stable UUID v4 so the gateway identity headers pass
// authx.PrincipalFromHeaders (TCR-2). Ids that already look like UUIDs are returned unchanged.
func asUUID(id string) string {
	if len(id) == 36 && id[8] == '-' && id[13] == '-' && id[18] == '-' && id[23] == '-' {
		return id
	}
	sum := sha256.Sum256([]byte(id))
	b := sum[:16]
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
