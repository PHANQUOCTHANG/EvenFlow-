package antibot_test

import (
	"testing"

	"github.com/eventflow/eventflow/libs/go/antibot"
)

// Each rule fires exactly at its configured threshold (AC-4): at the threshold, just below, just above, absent.
// The baseline is validSignals + chromeUA + neutral state, on which nothing fires.

func TestEvaluate_AC4_BaselineFiresNothing(t *testing.T) {
	d := eval(t, cfgWith(t), input([]byte(validSignals)), neutral())
	if len(d.Fired) != 0 || d.Score != 0 || d.Action != antibot.Action("PASS") {
		t.Fatalf("baseline: fired=%s score=%v action=%s", ids(d), d.Score, d.Action)
	}
}

type ruleCase struct {
	name string
	set  map[string]any       // payload changes
	st   func(*antibot.State) // state changes
	in   func(*antibot.Input) // input changes
	cfg  []string             // extra env
	want bool
}

func runRule(t *testing.T, rule string, cases []ruleCase) {
	t.Helper()
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cfg := cfgWith(t, c.cfg...)
			st := neutral()
			if c.st != nil {
				c.st(&st)
			}
			in := input(payload(t, c.set))
			if c.in != nil {
				c.in(&in)
			}
			d := eval(t, cfg, in, st)
			if hasRule(d, rule) != c.want {
				t.Fatalf("%s fired=%v, want %v (fired %s)", rule, hasRule(d, rule), c.want, ids(d))
			}
		})
	}
}

// R1 client: >= 8 intervals with CV < 0.10 (strict).
func TestRule_AC4_R1Client(t *testing.T) {
	runRule(t, "R1", []ruleCase{
		{name: "8 intervals CV 0.09", set: map[string]any{"iv": repeatIv(8, 91, 109)}, want: true},
		{name: "8 intervals CV 0 (fixed rhythm)", set: map[string]any{"iv": repeatIv(8, 100, 100)}, want: true},
		{name: "7 intervals CV 0 (below min samples)", set: map[string]any{"iv": repeatIv(7, 100, 100)}, want: false},
		{name: "8 intervals CV exactly 0.10", set: map[string]any{"iv": repeatIv(8, 90, 110)}, want: false},
		{name: "8 intervals CV 0.12", set: map[string]any{"iv": repeatIv(8, 88, 112)}, want: false},
		{name: "32 intervals CV 0", set: map[string]any{"iv": repeatIv(32, 33, 33)}, want: true},
		{name: "iv absent", set: map[string]any{"iv": nil}, want: false},
		{name: "config min 10: 9 samples", set: map[string]any{"iv": repeatIv(9, 100, 100)}, cfg: []string{"ANTIBOT_RULES", "R1_CLIENT_MIN=10"}, want: false},
		{name: "config CV 0.15: CV 0.12", set: map[string]any{"iv": repeatIv(8, 88, 112)}, cfg: []string{"ANTIBOT_RULES", "R1_CLIENT_CV=0.15"}, want: true},
	})
}

// R1 server: >= 4 join gaps (joins made without a slot) with CV < 0.05; not evaluated when degraded.
func TestRule_AC4_R1Server(t *testing.T) {
	gaps := func(g ...int64) func(*antibot.State) { return func(s *antibot.State) { s.JoinGapsMs = g } }
	runRule(t, "R1", []ruleCase{
		{name: "4 equal gaps", st: gaps(1000, 1000, 1000, 1000), want: true},
		{name: "4 gaps CV 0.04", st: gaps(960, 1040, 960, 1040), want: true},
		{name: "3 equal gaps", st: gaps(1000, 1000, 1000), want: false},
		{name: "4 gaps CV exactly 0.05", st: gaps(950, 1050, 950, 1050), want: false},
		{name: "4 gaps CV 0.06", st: gaps(940, 1060, 940, 1060), want: false},
		{name: "8 equal gaps", st: gaps(500, 500, 500, 500, 500, 500, 500, 500), want: true},
		{name: "unknown state", st: func(s *antibot.State) { *s = antibot.State{JoinGapsMs: []int64{1000, 1000, 1000, 1000}} }, want: false},
	})
}

// R2: rt < R2_MS (120).
func TestRule_AC4_R2(t *testing.T) {
	runRule(t, "R2", []ruleCase{
		{name: "rt 119", set: map[string]any{"rt": 119}, want: true},
		{name: "rt 120", set: map[string]any{"rt": 120}, want: false},
		{name: "rt 121", set: map[string]any{"rt": 121}, want: false},
		{name: "rt 0", set: map[string]any{"rt": 0}, want: true},
		{name: "rt absent", set: map[string]any{"rt": nil}, want: false},
		{name: "rt out of range", set: map[string]any{"rt": 600001}, want: false},
		{name: "config 200: rt 150", set: map[string]any{"rt": 150}, cfg: []string{"ANTIBOT_RULES", "R2_MS=200"}, want: true},
	})
}

// R3: pointer + scroll + key counts all 0 (or all absent, `{"v":1}`), or the payload is missing.
func TestRule_AC4_R3(t *testing.T) {
	zero := map[string]any{"pe": 0, "se": 0, "ke": 0}
	runRule(t, "R3", []ruleCase{
		{name: "all zero", set: zero, want: true},
		{name: "one pointer event (the click)", set: map[string]any{"pe": 1, "se": 0, "ke": 0}, want: false},
		{name: "keyboard only (Enter)", set: map[string]any{"pe": 0, "se": 0, "ke": 1}, want: false},
		{name: "scroll only", set: map[string]any{"pe": 0, "se": 1, "ke": 0}, want: false},
		{name: "missing payload", in: func(in *antibot.Input) { in.RawSignals = nil }, want: true},
		{name: "bad json", in: func(in *antibot.Input) { in.RawSignals = []byte(`{"v":`) }, want: true},
	})
}

// R3 + R5 on `{"v":1}` (contracts 1 R2, human decision 2026-11-22): valid but empty → R3 fires, R5 does not, 0.40.
func TestRule_AC4_VersionOnlyScoresR3Only(t *testing.T) {
	d := eval(t, cfgWith(t), input([]byte(`{"v":1}`)), neutral())
	if ids(d) != "R3" || !near(d.Score, 0.40) || d.Action != antibot.Action("CHALLENGE") || d.Applied != antibot.Action("PASS") {
		t.Fatalf(`{"v":1}: fired=%s score=%v action=%s applied=%s, want R3 0.40 CHALLENGE/PASS`, ids(d), d.Score, d.Action, d.Applied)
	}
}

// R4D: distinct identities on one fp hash > R4D_MAX (3).
func TestRule_AC4_R4D(t *testing.T) {
	fp := func(n int) func(*antibot.State) { return func(s *antibot.State) { s.FPIdentities = n } }
	runRule(t, "R4D", []ruleCase{
		{name: "4 identities", st: fp(4), want: true},
		{name: "3 identities", st: fp(3), want: false},
		{name: "2 identities", st: fp(2), want: false},
		{name: "64 identities (cap)", st: fp(64), want: true},
		{name: "no fp in payload", set: map[string]any{"fp": nil}, st: fp(10), want: false},
		{name: "unknown state", st: func(s *antibot.State) { *s = antibot.State{FPIdentities: 10, CardIdentities: -1} }, want: false},
		{name: "config max 5: 5 identities", st: fp(5), cfg: []string{"ANTIBOT_RULES", "R4D_MAX=5"}, want: false},
	})
}

// R4N: distinct identities on one network prefix > R4N_MAX (30), unless the prefix is suppressed.
func TestRule_AC4_R4N(t *testing.T) {
	net := func(n int) func(*antibot.State) { return func(s *antibot.State) { s.NetIdentities = n } }
	runRule(t, "R4N", []ruleCase{
		{name: "31 identities", st: net(31), want: true},
		{name: "30 identities", st: net(30), want: false},
		{name: "29 identities", st: net(29), want: false},
		{name: "suppressed prefix", st: func(s *antibot.State) { s.NetIdentities = 64; s.NetSuppressed = true }, want: false},
		{name: "unknown state", st: func(s *antibot.State) { *s = antibot.State{NetIdentities: 64, CardIdentities: -1} }, want: false},
	})
}

// R5 strong signs: UA deny list (case-insensitive substring), empty UA, wd true, missing payload.
func TestRule_AC4_R5Strong(t *testing.T) {
	ua := func(u string) func(*antibot.Input) { return func(in *antibot.Input) { in.UserAgent = u } }
	cases := []ruleCase{
		{name: "empty UA", in: ua(""), want: true},
		{name: "wd true", set: map[string]any{"wd": true}, want: true},
		{name: "wd false", set: map[string]any{"wd": false}, want: false},
		{name: "missing payload", in: func(in *antibot.Input) { in.RawSignals = nil }, want: true},
		{name: "browser UA", in: ua(chromeUA), want: false},
		{name: "case-insensitive", in: ua("PYTHON-Requests/2.32"), want: true},
		{name: "custom list replaces built-in (built-in entry no longer matches)", in: ua("curl/8.5.0"), cfg: []string{"ANTIBOT_UA_DENY", "MyBot,EvilCrawler"}, want: false},
		{name: "custom list entry", in: ua("Mozilla/5.0 evilcrawler/1.0"), cfg: []string{"ANTIBOT_UA_DENY", "MyBot,EvilCrawler"}, want: true},
	}
	for _, deny := range []string{"HeadlessChrome/131.0", "PhantomJS/2.1", "python-requests/2.31", "python-httpx/0.27",
		"Python/3.12 aiohttp/3.9", "curl/8.5.0", "Wget/1.21", "Go-http-client/1.1", "okhttp/4.12", "node-fetch/1.0",
		"axios/1.7.2", "Scrapy/2.11", "Puppeteer", "Playwright/1.48", "Selenium/4.25"} {
		cases = append(cases, ruleCase{name: "deny " + deny, in: ua(deny), want: true})
	}
	runRule(t, "R5", cases)
}

// R5W: >= 2 weak signs (gl false, fc < 3 except on mobile UA, tz/tzo disagree) and no strong sign.
func TestRule_AC4_R5Weak(t *testing.T) {
	ua := func(u string) func(*antibot.Input) { return func(in *antibot.Input) { in.UserAgent = u } }
	cases := []struct {
		name    string
		set     map[string]any
		in      func(*antibot.Input)
		r5, r5w bool
	}{
		{"gl false only", map[string]any{"gl": false}, nil, false, false},
		{"fc 2 only", map[string]any{"fc": 2}, nil, false, false},
		{"tz mismatch only", map[string]any{"tzo": -300}, nil, false, false},
		{"gl false + fc 2", map[string]any{"gl": false, "fc": 2}, nil, false, true},
		{"gl false + fc 3 (at threshold, not weak)", map[string]any{"gl": false, "fc": 3}, nil, false, false},
		{"fc 2 + tz mismatch", map[string]any{"fc": 2, "tzo": -300}, nil, false, true},
		{"gl false + tz mismatch", map[string]any{"gl": false, "tzo": 0}, nil, false, true},
		{"all three weak", map[string]any{"gl": false, "fc": 0, "tzo": 0}, nil, false, true},
		{"DST-aware zone agrees (Berlin winter +60)", map[string]any{"gl": false, "tz": "Europe/Berlin", "tzo": 60}, nil, false, false},
		{"DST-aware zone disagrees (Berlin winter +120)", map[string]any{"gl": false, "tz": "Europe/Berlin", "tzo": 120}, nil, false, true},
		{"invalid zone is not evaluated", map[string]any{"gl": false, "tz": "Mars/Olympus_Mons", "tzo": 0}, nil, false, false},
		{"iPhone: fc not evaluated", map[string]any{"gl": false, "fc": 0}, ua(iphoneUA), false, false},
		{"Android: fc not evaluated", map[string]any{"gl": false, "fc": 0}, ua(androidUA), false, false},
		{"mobile: gl false + tz mismatch still weak x2", map[string]any{"gl": false, "fc": 0, "tzo": -300}, ua(androidUA), false, true},
		{"strong + 2 weak = R5 only", map[string]any{"wd": true, "gl": false, "fc": 0}, nil, true, false},
		{"weak signs absent", map[string]any{"gl": nil, "fc": nil, "tz": nil, "tzo": nil}, nil, false, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			in := input(payload(t, c.set))
			if c.in != nil {
				c.in(&in)
			}
			d := eval(t, cfgWith(t), in, neutral())
			if hasRule(d, "R5") != c.r5 || hasRule(d, "R5W") != c.r5w {
				t.Fatalf("R5=%v R5W=%v, want %v %v (fired %s)", hasRule(d, "R5"), hasRule(d, "R5W"), c.r5, c.r5w, ids(d))
			}
		})
	}
}

// R6: > 3 identities on one card hash; dormant in phase 1 (-1 = unknown never fires).
func TestRule_AC4_R6(t *testing.T) {
	card := func(n int) func(*antibot.State) { return func(s *antibot.State) { s.CardIdentities = n } }
	runRule(t, "R6", []ruleCase{
		{name: "4 identities", st: card(4), want: true},
		{name: "3 identities", st: card(3), want: false},
		{name: "unknown (-1, phase 1)", st: card(-1), want: false},
		{name: "zero-value state (unknown)", st: func(s *antibot.State) { *s = antibot.State{} }, want: false},
	})
}

// R7: sq seen for >= R7_MIN_IDS (3) identities in the window; sq needs sn >= 12 and sb >= 4.
func TestRule_AC4_R7(t *testing.T) {
	seq := func(n int) func(*antibot.State) { return func(s *antibot.State) { s.SeqIdentities = n } }
	runRule(t, "R7", []ruleCase{
		{name: "3 identities", st: seq(3), want: true},
		{name: "2 identities", st: seq(2), want: false},
		{name: "4 identities", st: seq(4), want: true},
		{name: "sq absent", set: map[string]any{"sq": nil}, st: seq(10), want: false},
		{name: "sn out of range (11)", set: map[string]any{"sn": 11}, st: seq(10), want: false},
		{name: "sb absent", set: map[string]any{"sb": nil}, st: seq(10), want: false},
		{name: "sb out of range (3)", set: map[string]any{"sb": 3}, st: seq(10), want: false},
		{name: "unknown state", st: func(s *antibot.State) { *s = antibot.State{SeqIdentities: 10, CardIdentities: -1} }, want: false},
		{name: "config min events 16: sn 12", st: seq(5), cfg: []string{"ANTIBOT_RULES", "R7_MIN_EVENTS=16"}, want: false},
		{name: "config min buckets 8: sb 6", st: seq(5), cfg: []string{"ANTIBOT_RULES", "R7_MIN_BUCKETS=8"}, want: false},
		{name: "config min ids 5: 4 identities", st: seq(4), cfg: []string{"ANTIBOT_RULES", "R7_MIN_IDS=5"}, want: false},
	})
}

// D7: with an unknown (degraded) state only stateless rules are evaluated.
func TestEvaluate_AC7_UnknownStateUsesStatelessRulesOnly(t *testing.T) {
	st := antibot.State{JoinGapsMs: []int64{1, 1, 1, 1, 1}, FPIdentities: 60, NetIdentities: 60, SeqIdentities: 60, CardIdentities: 60}
	in := input(payload(t, map[string]any{"rt": 50}))
	d := eval(t, cfgWith(t), in, st)
	for _, r := range []string{"R4D", "R4N", "R6", "R7"} {
		if hasRule(d, r) {
			t.Errorf("%s fired on unknown state", r)
		}
	}
	if !hasRule(d, "R2") {
		t.Errorf("stateless R2 not evaluated on unknown state (fired %s)", ids(d))
	}
}

// AC-4: rule IDs are exactly the registered set.
func TestEvaluate_AC4_OnlyKnownRuleIDs(t *testing.T) {
	known := map[string]bool{"R1": true, "R2": true, "R3": true, "R4D": true, "R4N": true, "R5": true, "R5W": true, "R6": true, "R7": true}
	st := antibot.State{JoinGapsMs: []int64{9, 9, 9, 9}, FPIdentities: 9, NetIdentities: 99, SeqIdentities: 9, CardIdentities: 9, Known: true}
	in := input(payload(t, map[string]any{"rt": 1, "pe": 0, "se": 0, "ke": 0, "wd": true, "iv": repeatIv(8, 5, 5)}))
	d := eval(t, cfgWith(t), in, st)
	if len(d.Fired) < 7 {
		t.Fatalf("expected most rules to fire, got %s", ids(d))
	}
	for _, r := range d.Fired {
		if !known[string(r)] {
			t.Errorf("unknown rule id %q", r)
		}
	}
}
