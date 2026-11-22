package httpadapter_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/eventflow/eventflow/libs/go/antibot"
	"github.com/eventflow/eventflow/libs/go/authx"
	httpadapter "github.com/eventflow/eventflow/services/waitingroom/internal/adapter/http"
	"github.com/eventflow/eventflow/services/waitingroom/internal/domain"
)

// ---- fakes -----------------------------------------------------------------------------------------------------

type abCall struct {
	method  string // "Join" | "JoinLane" | "Revoke"
	ev, id  string
	lane    domain.Lane
	penalty int64
}

// laneStore implements QueueStore and the optional LaneStore (contracts 2, R2).
type laneStore struct {
	mu      sync.Mutex
	calls   []abCall
	state   domain.State // returned by Join / JoinLane; "" = QUEUED
	revoked bool
}

func (s *laneStore) result() httpadapter.JoinResult {
	st := s.state
	if st == "" {
		st = domain.StateQueued
	}
	if st == domain.State("BLOCKED") {
		return httpadapter.JoinResult{State: st, Rank: -1}
	}
	return httpadapter.JoinResult{State: st, Token: jti, Rank: 42, IsNew: true}
}

func (s *laneStore) Join(_ context.Context, ev, id, _ string) (httpadapter.JoinResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, abCall{method: "Join", ev: ev, id: id})
	return s.result(), nil
}

func (s *laneStore) JoinLane(_ context.Context, ev, id, _ string, lane domain.Lane, penalty int64) (httpadapter.JoinResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, abCall{method: "JoinLane", ev: ev, id: id, lane: lane, penalty: penalty})
	return s.result(), nil
}

func (s *laneStore) Revoke(_ context.Context, ev, id string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, abCall{method: "Revoke", ev: ev, id: id})
	return s.revoked, nil
}

func (s *laneStore) Status(context.Context, string, string) (domain.Position, error) {
	return domain.Position{State: domain.StateQueued}, nil
}

// plainStore has no LaneStore (fallback path of contracts 2).
type plainStore struct{ joins int }

func (p *plainStore) Join(context.Context, string, string, string) (httpadapter.JoinResult, error) {
	p.joins++
	return httpadapter.JoinResult{State: domain.StateQueued, Token: jti, Rank: 42, IsNew: true}, nil
}
func (p *plainStore) Status(context.Context, string, string) (domain.Position, error) {
	return domain.Position{State: domain.StateQueued}, nil
}

type fakeGuard struct {
	mu  sync.Mutex
	d   antibot.Decision
	got []antibot.Input
}

func (g *fakeGuard) Decide(_ context.Context, in antibot.Input) antibot.Decision {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.got = append(g.got, in)
	return g.d
}

func dec(action, applied string, tags ...string) antibot.Decision {
	return antibot.Decision{Action: antibot.Action(action), Applied: antibot.Action(applied), Score: 0.9,
		Fired: []antibot.RuleID{"R2", "R5"}, Tags: tags, FPHash: "0123456789abcdef0123456789abcdef",
		NetPrefix: netip.MustParsePrefix("203.0.113.0/24")}
}

// ---- helpers ---------------------------------------------------------------------------------------------------

const abUA = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"

const abSignals = `{"v":1,"rt":3500,"pe":25,"fp":"raw-device-fp-9999","wd":false}`

func abMux(store httpadapter.QueueStore, g httpadapter.BotGuard, logBuf *bytes.Buffer) http.Handler {
	mux := http.NewServeMux()
	log := slog.New(slog.NewTextHandler(logBuf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	httpadapter.New(store, g, log).WithQueueTokens(secret, true).Routes(mux)
	return mux
}

func abJoin(mux http.Handler, body string, hdr map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/v1/events/e1/queue/join", strings.NewReader(body))
	for k, v := range verifiedBuyer(map[string]string{"User-Agent": abUA, "X-Forwarded-For": "203.0.113.77, 10.0.0.1"}) {
		req.Header.Set(k, v)
	}
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func bodyKeys(t *testing.T, rec *httptest.ResponseRecorder) (map[string]any, []string) {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
		t.Fatalf("body %q: %v", rec.Body.String(), err)
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return m, keys
}

func jtiOf(t *testing.T, m map[string]any) string {
	t.Helper()
	tok, _ := m["queue_token"].(string)
	c, err := authx.VerifyQueueToken(secret, tok, time.Now())
	if err != nil {
		t.Fatalf("queue_token does not verify: %v", err)
	}
	return c.JTI
}

func joins(s *laneStore) []abCall {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []abCall
	for _, c := range s.calls {
		if c.method != "Revoke" {
			out = append(out, c)
		}
	}
	return out
}

func revokes(s *laneStore) []abCall {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []abCall
	for _, c := range s.calls {
		if c.method == "Revoke" {
			out = append(out, c)
		}
	}
	return out
}

var developKeys = []string{"is_new", "poll_after_ms", "queue_token", "rank", "state"}

// ---- tests -----------------------------------------------------------------------------------------------------

// AC-13 off: guard not wired (untyped nil) → develop behaviour, 4-arg Join, develop response shape.
func TestAntibotJoin_AC13_OffIsDevelop(t *testing.T) {
	s := &laneStore{}
	var logs bytes.Buffer
	rec := abJoin(abMux(s, nil, &logs), `{"signals":`+abSignals+`}`, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	m, keys := bodyKeys(t, rec)
	if strings.Join(keys, ",") != strings.Join(developKeys, ",") {
		t.Fatalf("keys %v, want %v", keys, developKeys)
	}
	if jtiOf(t, m) != jti {
		t.Fatal("token not bound to the store's jti")
	}
	if js := joins(s); len(js) != 1 || js[0].method != "Join" {
		t.Fatalf("store calls %+v, want one 4-arg Join", s.calls)
	}
}

// AC-9: PASS in enforce behaves as develop: same response, one join in the normal lane, no revoke.
func TestAntibotJoin_AC9_PassIsDevelop(t *testing.T) {
	sOff, sPass := &laneStore{}, &laneStore{}
	var logs bytes.Buffer
	off := abJoin(abMux(sOff, nil, &logs), `{"signals":`+abSignals+`}`, nil)
	g := &fakeGuard{d: antibot.Decision{Action: "PASS", Applied: "PASS"}}
	got := abJoin(abMux(sPass, g, &logs), `{"signals":`+abSignals+`}`, nil)
	if got.Code != off.Code {
		t.Fatalf("status %d, develop %d", got.Code, off.Code)
	}
	mOff, _ := bodyKeys(t, off)
	mGot, keys := bodyKeys(t, got)
	if strings.Join(keys, ",") != strings.Join(developKeys, ",") {
		t.Fatalf("keys %v", keys)
	}
	for _, k := range []string{"state", "rank", "is_new", "poll_after_ms"} {
		if mGot[k] != mOff[k] {
			t.Errorf("%s = %v, develop %v", k, mGot[k], mOff[k])
		}
	}
	if jtiOf(t, mGot) != jtiOf(t, mOff) {
		t.Error("token jti differs from develop")
	}
	js := joins(sPass)
	if len(js) != 1 || (js[0].method == "JoinLane" && js[0].lane != domain.LaneNormal) {
		t.Fatalf("store calls %+v, want one normal-lane join", js)
	}
	if len(revokes(sPass)) != 0 {
		t.Fatal("PASS revoked a slot")
	}
}

// AC-13 shadow: computed BLOCK, applied PASS → joins normally.
func TestAntibotJoin_AC13_ShadowProceeds(t *testing.T) {
	s := &laneStore{}
	var logs bytes.Buffer
	g := &fakeGuard{d: dec("BLOCK", "PASS", "MODE_SHADOW")}
	rec := abJoin(abMux(s, g, &logs), `{"signals":`+abSignals+`}`, nil)
	if rec.Code != http.StatusOK || len(joins(s)) != 1 || len(revokes(s)) != 0 {
		t.Fatalf("shadow: status %d calls %+v", rec.Code, s.calls)
	}
	if js := joins(s); js[0].method == "JoinLane" && js[0].lane != domain.LaneNormal {
		t.Fatalf("shadow used lane %q", js[0].lane)
	}
}

// AC-11: SLOW_LANE → 200 with the same fields and state, JoinLane(slow, P > 0).
func TestAntibotJoin_AC11_SlowLaneSameResponse(t *testing.T) {
	sPass, sSlow := &laneStore{}, &laneStore{}
	var logs bytes.Buffer
	pass := abJoin(abMux(sPass, &fakeGuard{d: antibot.Decision{Action: "PASS", Applied: "PASS"}}, &logs), `{"signals":`+abSignals+`}`, nil)
	slow := abJoin(abMux(sSlow, &fakeGuard{d: dec("SLOW_LANE", "SLOW_LANE")}, &logs), `{"signals":`+abSignals+`}`, nil)
	if slow.Code != http.StatusOK {
		t.Fatalf("slow lane status %d", slow.Code)
	}
	mp, kp := bodyKeys(t, pass)
	ms, ks := bodyKeys(t, slow)
	if strings.Join(kp, ",") != strings.Join(ks, ",") {
		t.Fatalf("slow keys %v, pass keys %v", ks, kp)
	}
	if mp["state"] != ms["state"] || mp["is_new"] != ms["is_new"] {
		t.Fatalf("slow body %v differs in state from pass %v", ms, mp)
	}
	js := joins(sSlow)
	if len(js) != 1 || js[0].method != "JoinLane" || js[0].lane != domain.LaneSlow || js[0].penalty <= 0 {
		t.Fatalf("store calls %+v, want JoinLane(slow, P>0)", js)
	}
	if strings.Contains(slow.Body.String(), "SLOW") || strings.Contains(slow.Body.String(), "R2") {
		t.Fatalf("response reveals the decision: %s", slow.Body)
	}
}

// Contracts 2 (R2, P-4): store without LaneStore → SLOW_LANE falls back to Join (+ warning).
func TestAntibotJoin_P4_SlowLaneFallbackWithoutLaneStore(t *testing.T) {
	s := &plainStore{}
	var logs bytes.Buffer
	rec := abJoin(abMux(s, &fakeGuard{d: dec("SLOW_LANE", "SLOW_LANE")}, &logs), `{"signals":`+abSignals+`}`, nil)
	if rec.Code != http.StatusOK || s.joins != 1 {
		t.Fatalf("status %d joins %d, want 200 and one Join", rec.Code, s.joins)
	}
	if !strings.Contains(strings.ToLower(logs.String()), "level=warn") {
		t.Fatalf("no warning logged for the fallback: %s", logs.String())
	}
}

// AC-12: BLOCK → 403 /problems/request-denied, generic, revoke, no join, no token.
func TestAntibotJoin_AC12_BlockDeniesAndRevokes(t *testing.T) {
	s := &laneStore{revoked: true}
	var logs bytes.Buffer
	rec := abJoin(abMux(s, &fakeGuard{d: dec("BLOCK", "BLOCK")}, &logs), `{"signals":`+abSignals+`}`, nil)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status %d, want 403", rec.Code)
	}
	if problemType(rec) != httpadapter.ProblemRequestDenied || httpadapter.ProblemRequestDenied != "/problems/request-denied" {
		t.Fatalf("problem type %q", problemType(rec))
	}
	b := rec.Body.String()
	for _, leak := range []string{"R2", "R5", "score", "0.9", "queue_token", "SLOW", "BLOCK", "0123456789abcdef"} {
		if strings.Contains(b, leak) {
			t.Errorf("403 body leaks %q: %s", leak, b)
		}
	}
	if len(joins(s)) != 0 {
		t.Fatalf("BLOCK still joined: %+v", s.calls)
	}
	rv := revokes(s)
	if len(rv) != 1 || rv[0].ev != "e1" || rv[0].id != userID {
		t.Fatalf("revoke calls %+v, want one for (e1, %s)", rv, userID)
	}
}

// Contracts 2 (R2): BLOCK without LaneStore → 403 only.
func TestAntibotJoin_P4_BlockWithoutLaneStore(t *testing.T) {
	s := &plainStore{}
	var logs bytes.Buffer
	rec := abJoin(abMux(s, &fakeGuard{d: dec("BLOCK", "BLOCK")}, &logs), `{"signals":`+abSignals+`}`, nil)
	if rec.Code != http.StatusForbidden || problemType(rec) != "/problems/request-denied" || s.joins != 0 {
		t.Fatalf("status %d type %q joins %d", rec.Code, problemType(rec), s.joins)
	}
}

// Contracts 2 (R2, D6): JoinLane reporting BLOCKED (flag set by a concurrent BLOCK) → 403 request-denied.
func TestAntibotJoin_D6_JoinLaneBlocked(t *testing.T) {
	s := &laneStore{state: domain.State("BLOCKED")}
	var logs bytes.Buffer
	rec := abJoin(abMux(s, &fakeGuard{d: dec("SLOW_LANE", "SLOW_LANE")}, &logs), `{"signals":`+abSignals+`}`, nil)
	if rec.Code != http.StatusForbidden || problemType(rec) != "/problems/request-denied" {
		t.Fatalf("status %d type %q, want 403 request-denied", rec.Code, problemType(rec))
	}
	if strings.Contains(rec.Body.String(), "queue_token") {
		t.Fatal("token issued for a BLOCKED join")
	}
}

// Contracts 1/2: the handler builds Input from the principal, headers and the raw signals.
func TestAntibotJoin_Contracts_InputBuiltFromRequest(t *testing.T) {
	s := &laneStore{}
	g := &fakeGuard{d: antibot.Decision{Action: "PASS", Applied: "PASS"}}
	var logs bytes.Buffer
	longUA := strings.Repeat("u", 700)
	before := time.Now()
	abJoin(abMux(s, g, &logs), `{"signals":`+abSignals+`,"other":1}`, map[string]string{"User-Agent": longUA})
	if len(g.got) != 1 {
		t.Fatalf("guard called %d times", len(g.got))
	}
	in := g.got[0]
	if in.EventID != "e1" || in.IdentityID != userID || in.DeviceID != deviceID {
		t.Errorf("ids %q %q %q", in.EventID, in.IdentityID, in.DeviceID)
	}
	if len(in.UserAgent) != 512 {
		t.Errorf("UserAgent length %d, want first 512 bytes", len(in.UserAgent))
	}
	if in.ClientIP != netip.MustParseAddr("203.0.113.77") {
		t.Errorf("ClientIP %v, want first X-Forwarded-For element", in.ClientIP)
	}
	var a, b any
	_ = json.Unmarshal([]byte(abSignals), &a)
	if err := json.Unmarshal(in.RawSignals, &b); err != nil || !jsonEqual(a, b) {
		t.Errorf("RawSignals %s, want the body's signals value", in.RawSignals)
	}
	if in.BodyErr != "" {
		t.Errorf("BodyErr %q for a valid body", in.BodyErr)
	}
	if in.Now.Before(before.Add(-time.Second)) || in.Now.After(time.Now().Add(time.Second)) {
		t.Errorf("Now %v not the request time", in.Now)
	}
}

func jsonEqual(a, b any) bool {
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	return bytes.Equal(x, y)
}

// Contracts 1: client IP from the first X-Forwarded-For element; absent / garbage → zero.
func TestAntibotJoin_Contracts_ClientIP(t *testing.T) {
	for xff, want := range map[string]netip.Addr{
		"2001:db8::1, 10.0.0.1": netip.MustParseAddr("2001:db8::1"),
		" 198.51.100.4 ":        netip.MustParseAddr("198.51.100.4"),
		"not-an-ip, 1.2.3.4":    {},
	} {
		g := &fakeGuard{d: antibot.Decision{Action: "PASS", Applied: "PASS"}}
		var logs bytes.Buffer
		abJoin(abMux(&laneStore{}, g, &logs), `{"signals":{"v":1}}`, map[string]string{"X-Forwarded-For": xff})
		if g.got[0].ClientIP != want {
			t.Errorf("XFF %q: ClientIP %v, want %v", xff, g.got[0].ClientIP, want)
		}
	}
	g := &fakeGuard{d: antibot.Decision{Action: "PASS", Applied: "PASS"}}
	var logs bytes.Buffer
	req := httptest.NewRequest(http.MethodPost, "/v1/events/e1/queue/join", strings.NewReader(`{"signals":{"v":1}}`))
	for k, v := range verifiedBuyer(nil) {
		req.Header.Set(k, v)
	}
	abMux(&laneStore{}, g, &logs).ServeHTTP(httptest.NewRecorder(), req)
	if g.got[0].ClientIP.IsValid() {
		t.Errorf("no XFF: ClientIP %v, want zero", g.got[0].ClientIP)
	}
}

// AC-3: oversize / malformed / signal-less bodies are not rejected; the guard sees them as missing.
func TestAntibotJoin_AC3_BadBodiesStillJoin(t *testing.T) {
	big := `{"signals":{"v":1,"x":"` + strings.Repeat("a", 5000) + `"}}`
	cases := []struct {
		name, body string
		bodyErr    antibot.InvalidReason
		rawNil     bool
	}{
		{"over 4 KiB", big, "too_large", true},
		{"bad json", `{"signals":{"v":1,`, "bad_json", true},
		{"no signals field", `{}`, "", true},
		{"empty body", ``, "", true},
		{"signals {} (demo.sh / k6)", `{"signals":{}}`, "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := &laneStore{}
			g := &fakeGuard{d: antibot.Decision{Action: "PASS", Applied: "PASS"}}
			var logs bytes.Buffer
			rec := abJoin(abMux(s, g, &logs), c.body, nil)
			if rec.Code != http.StatusOK {
				t.Fatalf("status %d, want 200 (body never rejected for its signals)", rec.Code)
			}
			if len(g.got) != 1 {
				t.Fatalf("guard calls %d", len(g.got))
			}
			in := g.got[0]
			if c.bodyErr != "" && in.BodyErr != c.bodyErr {
				t.Errorf("BodyErr %q, want %q", in.BodyErr, c.bodyErr)
			}
			if c.rawNil && in.RawSignals != nil {
				t.Errorf("RawSignals %s, want nil", in.RawSignals)
			}
			if sig, _ := antibot.DecodeSignals(in.RawSignals); in.BodyErr == "" && !sig.Missing() {
				t.Errorf("guard input is neither BodyErr nor missing: %s", in.RawSignals)
			}
		})
	}
	// guard off: an oversize body still joins (develop ignored the body)
	var logs bytes.Buffer
	if rec := abJoin(abMux(&laneStore{}, nil, &logs), big, nil); rec.Code != http.StatusOK {
		t.Fatalf("off + oversize body: %d", rec.Code)
	}
}

// AC-15: no raw fingerprint, UA or full IP in the handler's log lines, for any outcome.
func TestAntibotJoin_AC15_LogsHaveNoRawValues(t *testing.T) {
	var logs bytes.Buffer
	for _, d := range []antibot.Decision{dec("BLOCK", "BLOCK"), dec("SLOW_LANE", "SLOW_LANE"), dec("BLOCK", "PASS", "MODE_SHADOW")} {
		abJoin(abMux(&laneStore{}, &fakeGuard{d: d}, &logs), `{"signals":`+abSignals+`}`, nil)
		abJoin(abMux(&plainStore{}, &fakeGuard{d: d}, &logs), `{"signals":`+abSignals+`}`, nil)
	}
	for _, raw := range []string{"raw-device-fp-9999", "203.0.113.77", "Chrome/131"} {
		if strings.Contains(logs.String(), raw) {
			t.Errorf("log contains %q:\n%s", raw, logs.String())
		}
	}
}

// domain.Lane values are the Lua lane codes (contracts 8.5).
func TestLane_Contracts_Values(t *testing.T) {
	if domain.LaneNormal != domain.Lane("n") || domain.LaneSlow != domain.Lane("s") {
		t.Fatalf("lanes %q %q, want n / s", domain.LaneNormal, domain.LaneSlow)
	}
	var _ httpadapter.LaneStore = (*laneStore)(nil)
}
