package antibot_test

import (
	"math"
	"net/netip"
	"slices"
	"testing"

	"github.com/eventflow/eventflow/libs/go/antibot"
)

const (
	pass      = antibot.Action("PASS")
	challenge = antibot.Action("CHALLENGE")
	slowLane  = antibot.Action("SLOW_LANE")
	block     = antibot.Action("BLOCK")
)

// Contracts 3 "Default outcomes" (R2): every row, with default config in enforce.
func TestScore_AC8_DefaultOutcomesTable(t *testing.T) {
	weak2 := map[string]any{"gl": false, "fc": 2} // R5W (privacy browser)
	merge := func(ms ...map[string]any) map[string]any {
		out := map[string]any{}
		for _, m := range ms {
			for k, v := range m {
				out[k] = v
			}
		}
		return out
	}
	fast := map[string]any{"rt": 80}
	cases := []struct {
		name    string
		raw     []byte // nil = validSignals unless missing
		missing bool   // send no signals at all
		ua      string
		st      func(*antibot.State)
		fired   string
		score   float64
		tol     float64
		action  antibot.Action
		applied antibot.Action
		tags    []string
		notTags []string
	}{
		{name: "missing payload, browser UA", missing: true, fired: "R3,R5", score: 0.88, action: slowLane, applied: slowLane, tags: []string{"CAPPED"}},
		{name: "missing payload, identity holds a slot", missing: true, st: func(s *antibot.State) { s.HasSlot = true },
			fired: "R3,R5", score: 0.88, action: slowLane, applied: pass, tags: []string{"CAPPED", "REJOIN"}},
		{name: "raw HTTP python-requests", missing: true, ua: "python-requests/2.32.3", fired: "R3,R5", score: 0.88, action: block, applied: block, notTags: []string{"CAPPED"}},
		{name: "headless default", raw: payload(t, map[string]any{"wd": true, "rt": 40, "pe": 1, "se": 0, "ke": 0}), fired: "R2,R5", score: 0.88, action: block, applied: block},
		{name: "stealth fixed rhythm", raw: payload(t, map[string]any{"rt": 90, "iv": repeatIv(10, 100, 100)}), fired: "R1,R2", score: 0.70, action: slowLane, applied: slowLane},
		{name: "account farm", st: func(s *antibot.State) { s.FPIdentities = 4; s.NetIdentities = 31 }, fired: "R4D,R4N", score: 0.70, action: slowLane, applied: slowLane},
		{name: "shared fp only", st: func(s *antibot.State) { s.FPIdentities = 4 }, fired: "R4D", score: 0.60, action: slowLane, applied: slowLane},
		{name: "replay small proxy pool", st: func(s *antibot.State) { s.SeqIdentities = 3; s.NetIdentities = 31 }, fired: "R4N,R7", score: 0.625, action: slowLane, applied: slowLane},
		{name: "replay distinct networks", st: func(s *antibot.State) { s.SeqIdentities = 3 }, fired: "R7", score: 0.50, action: challenge, applied: pass},
		{name: "human on CGNAT /24", st: func(s *antibot.State) { s.NetIdentities = 40 }, fired: "R4N", score: 0.25, action: pass, applied: pass},
		{name: "cluster + fast click", raw: payload(t, fast), st: func(s *antibot.State) { s.NetIdentities = 40 }, fired: "R2,R4N", score: 0.55, action: challenge, applied: pass},
		{name: "cluster + privacy browser", raw: payload(t, weak2), st: func(s *antibot.State) { s.NetIdentities = 40 }, fired: "R4N,R5W", score: 0.588, tol: 0.0011, action: challenge, applied: pass},
		{name: "cluster + fast + privacy", raw: payload(t, merge(fast, weak2)), st: func(s *antibot.State) { s.NetIdentities = 40 }, fired: "R2,R4N,R5W", score: 0.7525, tol: 0.0006, action: slowLane, applied: slowLane},
		{name: "one weak: R2", raw: payload(t, fast), fired: "R2", score: 0.40, action: challenge, applied: pass},
		{name: "one weak: R5W", raw: payload(t, weak2), fired: "R5W", score: 0.45, action: challenge, applied: pass},
		{name: "one weak: R1", raw: payload(t, map[string]any{"iv": repeatIv(8, 33, 33)}), fired: "R1", score: 0.50, action: challenge, applied: pass},
		{name: "fast click + privacy browser", raw: payload(t, merge(fast, weak2)), fired: "R2,R5W", score: 0.67, action: slowLane, applied: slowLane},
		{name: "R1 + R7", raw: payload(t, map[string]any{"iv": repeatIv(8, 33, 33)}), st: func(s *antibot.State) { s.SeqIdentities = 3 }, fired: "R1,R7", score: 0.75, action: slowLane, applied: slowLane},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			raw := c.raw
			if raw == nil && !c.missing {
				raw = []byte(validSignals)
			}
			in := input(raw)
			if c.ua != "" {
				in.UserAgent = c.ua
			}
			st := neutral()
			if c.st != nil {
				c.st(&st)
			}
			d := eval(t, cfgWith(t), in, st)
			tol := c.tol
			if tol == 0 {
				tol = 1e-9
			}
			if ids(d) != c.fired {
				t.Errorf("fired = %s, want %s", ids(d), c.fired)
			}
			if math.Abs(d.Score-c.score) > tol {
				t.Errorf("score = %v, want %v", d.Score, c.score)
			}
			if d.Action != c.action || d.Applied != c.applied {
				t.Errorf("action/applied = %s/%s, want %s/%s", d.Action, d.Applied, c.action, c.applied)
			}
			for _, tag := range c.tags {
				if !hasTag(d, tag) {
					t.Errorf("tags %v lack %s", d.Tags, tag)
				}
			}
			for _, tag := range c.notTags {
				if hasTag(d, tag) {
					t.Errorf("tags %v must not contain %s", d.Tags, tag)
				}
			}
		})
	}
}

// AC-8: score = 1 - prod(1 - w) rounded to 3 decimals; no fired rule = 0 and PASS.
func TestScore_AC8_NoisyOrRounded(t *testing.T) {
	cases := []struct {
		weights string
		set     map[string]any
		want    float64
	}{
		{"R2=0.1234", map[string]any{"rt": 50}, 0.123},
		{"R2=0.1236", map[string]any{"rt": 50}, 0.124},
		{"R2=0.1,R5W=0.2", map[string]any{"rt": 50, "gl": false, "fc": 0}, 0.28},
		{"R2=0.5,R5W=0.5", map[string]any{"rt": 50, "gl": false, "fc": 0}, 0.75},
		{"R2=0", map[string]any{"rt": 50}, 0},
	}
	for _, c := range cases {
		d := eval(t, cfgWith(t, "ANTIBOT_WEIGHTS", c.weights), input(payload(t, c.set)), neutral())
		if !near(d.Score, c.want) {
			t.Errorf("weights %s fired %s: score %v, want %v", c.weights, ids(d), d.Score, c.want)
		}
		if r := math.Round(d.Score*1000) / 1000; r != d.Score {
			t.Errorf("score %v not rounded to 3 decimals", d.Score)
		}
	}
	d := eval(t, cfgWith(t), input([]byte(validSignals)), neutral())
	if d.Score != 0 || d.Action != pass || len(d.Fired) != 0 {
		t.Fatalf("no rule: score %v action %s fired %s", d.Score, d.Action, ids(d))
	}
}

// AC-8: tiers from the rounded score, lower bound inclusive (0.30 / 0.60 / 0.85). R2 alone is block-eligible.
func TestScore_AC8_TierBoundariesLowerInclusive(t *testing.T) {
	cases := []struct {
		w    string
		want antibot.Action
	}{
		{"0.299", pass}, {"0.2996", challenge}, {"0.30", challenge}, {"0.301", challenge},
		{"0.599", challenge}, {"0.60", slowLane}, {"0.601", slowLane},
		{"0.849", slowLane}, {"0.8496", block}, {"0.85", block}, {"0.851", block},
	}
	for _, c := range cases {
		d := eval(t, cfgWith(t, "ANTIBOT_WEIGHTS", "R2="+c.w), input(payload(t, map[string]any{"rt": 50})), neutral())
		if d.Action != c.want {
			t.Errorf("R2=%s: score %v tier %s, want %s", c.w, d.Score, d.Action, c.want)
		}
	}
	// configured cut-offs move the tiers
	d := eval(t, cfgWith(t, "ANTIBOT_TIERS", "0.10,0.20,0.95"), input(payload(t, map[string]any{"rt": 50})), neutral())
	if d.Action != slowLane {
		t.Fatalf("tiers 0.10,0.20,0.95 with 0.40: %s, want SLOW_LANE", d.Action)
	}
}

// D11: BLOCK needs a positive sign; otherwise SLOW_LANE + CAPPED.
func TestScore_AC8_BlockEligibilityCap(t *testing.T) {
	// R3 + R5(missing) + R4N = 1 - 0.6*0.2*0.75 = 0.91: no positive sign -> capped.
	st := neutral()
	st.NetIdentities = 40
	d := eval(t, cfgWith(t), input(nil), st)
	if d.Action != slowLane || !hasTag(d, "CAPPED") {
		t.Fatalf("missing + R4N: action %s tags %v, want SLOW_LANE CAPPED", d.Action, d.Tags)
	}
	// R3 + R5W + R4N + R5? use weights to push non-eligible rules over 0.85
	d = eval(t, cfgWith(t, "ANTIBOT_WEIGHTS", "R5W=0.95"), input(payload(t, map[string]any{"gl": false, "fc": 0})), neutral())
	if d.Action != slowLane || !hasTag(d, "CAPPED") {
		t.Fatalf("R5W alone at 0.95: action %s tags %v, want SLOW_LANE CAPPED", d.Action, d.Tags)
	}
	// each positive sign makes BLOCK reachable
	for _, c := range []struct {
		name string
		in   antibot.Input
		st   func(*antibot.State)
	}{
		{"R1", input(payload(t, map[string]any{"iv": repeatIv(8, 50, 50)})), nil},
		{"R2", input(payload(t, map[string]any{"rt": 10})), nil},
		{"R4D", input([]byte(validSignals)), func(s *antibot.State) { s.FPIdentities = 9 }},
		{"R6", input([]byte(validSignals)), func(s *antibot.State) { s.CardIdentities = 9 }},
		{"R7", input([]byte(validSignals)), func(s *antibot.State) { s.SeqIdentities = 9 }},
		{"R5 wd", input(payload(t, map[string]any{"wd": true})), nil},
	} {
		st := neutral()
		if c.st != nil {
			c.st(&st)
		}
		cfg := cfgWith(t, "ANTIBOT_WEIGHTS", "R1=0.9,R2=0.9,R4D=0.9,R6=0.9,R7=0.9,R5=0.9")
		d := eval(t, cfg, c.in, st)
		if d.Action != block || hasTag(d, "CAPPED") {
			t.Errorf("%s at 0.9: action %s tags %v, want BLOCK", c.name, d.Action, d.Tags)
		}
	}
	// missing payload + a positive server-side sign (R1-server rhythm) is block-eligible: R1+R3+R5 = 0.94
	st = neutral()
	st.JoinGapsMs = []int64{1000, 1000, 1000, 1000}
	d = eval(t, cfgWith(t), input(nil), st)
	if d.Action != block || ids(d) != "R1,R3,R5" {
		t.Fatalf("missing + R1-server: %s %s (score %v), want BLOCK R1,R3,R5", d.Action, ids(d), d.Score)
	}
}

// D12: a slot holder keeps the slot unless the decision is a block-eligible BLOCK.
func TestScore_AC11_RejoinKeepsSlotUnlessBlock(t *testing.T) {
	st := neutral()
	st.HasSlot = true
	for _, c := range []struct {
		name string
		in   antibot.Input
	}{
		{"pass", input([]byte(validSignals))},
		{"challenge", input(payload(t, map[string]any{"rt": 50}))},
		{"slow lane", input(payload(t, map[string]any{"rt": 50, "iv": repeatIv(8, 100, 100)}))},
		{"capped", input(nil)},
	} {
		d := eval(t, cfgWith(t, "ANTIBOT_CHALLENGE_FALLBACK", "slow_lane"), c.in, st)
		if d.Applied != pass || !hasTag(d, "REJOIN") {
			t.Errorf("%s with slot: applied %s tags %v, want PASS REJOIN", c.name, d.Applied, d.Tags)
		}
	}
	d := eval(t, cfgWith(t), input(payload(t, map[string]any{"wd": true, "rt": 10})), st)
	if d.Action != block || d.Applied != block {
		t.Fatalf("eligible BLOCK with slot: %s/%s, want BLOCK/BLOCK", d.Action, d.Applied)
	}
}

// AC-12: sticky → BLOCK with only STICKY, score 1.000, rules not evaluated.
func TestScore_AC12_Sticky(t *testing.T) {
	st := neutral()
	st.Sticky = true
	d := eval(t, cfgWith(t), input([]byte(validSignals)), st)
	if d.Action != block || d.Applied != block || len(d.Fired) != 0 || !slices.Equal(d.Tags, []string{"STICKY"}) || d.Score != 1 {
		t.Fatalf("sticky: %+v", d)
	}
}

// AC-10 / AC-13: Applied from mode and fallback.
func TestScore_AC10_AC13_AppliedMatrix(t *testing.T) {
	tierInputs := map[antibot.Action]func() (antibot.Input, antibot.State){
		pass:      func() (antibot.Input, antibot.State) { return input([]byte(validSignals)), neutral() },
		challenge: func() (antibot.Input, antibot.State) { return input(payload(t, map[string]any{"rt": 50})), neutral() },
		slowLane: func() (antibot.Input, antibot.State) {
			st := neutral()
			st.FPIdentities = 4
			return input([]byte(validSignals)), st
		},
		block: func() (antibot.Input, antibot.State) {
			return input(payload(t, map[string]any{"wd": true, "rt": 10})), neutral()
		},
	}
	cases := []struct {
		mode, fallback string
		tier, applied  antibot.Action
	}{
		{"enforce", "pass", pass, pass},
		{"enforce", "pass", challenge, pass},
		{"enforce", "slow_lane", challenge, slowLane},
		{"enforce", "pass", slowLane, slowLane},
		{"enforce", "pass", block, block},
		{"shadow", "pass", pass, pass},
		{"shadow", "slow_lane", challenge, pass},
		{"shadow", "pass", slowLane, pass},
		{"shadow", "pass", block, pass},
	}
	for _, c := range cases {
		in, st := tierInputs[c.tier]()
		d := eval(t, cfgWith(t, "ANTIBOT_MODE", c.mode, "ANTIBOT_CHALLENGE_FALLBACK", c.fallback), in, st)
		if d.Action != c.tier || d.Applied != c.applied {
			t.Errorf("%s/%s tier %s: got %s/%s, want applied %s", c.mode, c.fallback, c.tier, d.Action, d.Applied, c.applied)
		}
		if (c.mode == "shadow") != hasTag(d, "MODE_SHADOW") {
			t.Errorf("%s: tags %v", c.mode, d.Tags)
		}
	}
}

// AC-14: Fired is sorted.
func TestScore_AC14_FiredSorted(t *testing.T) {
	st := neutral()
	st.FPIdentities, st.NetIdentities, st.SeqIdentities = 9, 99, 9
	d := eval(t, cfgWith(t), input(payload(t, map[string]any{"rt": 1, "wd": true, "iv": repeatIv(8, 5, 5)})), st)
	if !slices.IsSorted(d.Fired) || len(d.Fired) < 4 {
		t.Fatalf("fired not sorted or too few: %v", d.Fired)
	}
}

// AC-15: decision carries only the HMAC and the prefix; no raw fp, UA or IP anywhere in it.
func TestEvaluate_AC15_DecisionHasNoRawValues(t *testing.T) {
	cfg := cfgWith(t)
	in := input([]byte(validSignals))
	in.ClientIP = netip.MustParseAddr("203.0.113.77")
	d := eval(t, cfg, in, neutral())
	if d.FPHash != antibot.FPHash([]byte("0123456789abcdef0123456789abcdef"), rawFP) {
		t.Fatalf("FPHash %q is not HMAC(ANTIBOT_HASH_KEY, fp)", d.FPHash)
	}
	if d.NetPrefix.String() != "203.0.113.0/24" {
		t.Fatalf("NetPrefix %s", d.NetPrefix)
	}
	dump := sprintAll(d)
	for _, raw := range []string{rawFP, "203.0.113.77", "Chrome/131", "Asia/Ho_Chi_Minh"} {
		if containsStr(dump, raw) {
			t.Errorf("decision contains raw value %q: %s", raw, dump)
		}
	}
	d = eval(t, cfg, input(payload(t, map[string]any{"fp": nil})), neutral())
	if d.FPHash != "" {
		t.Fatalf("no fp: FPHash %q, want empty", d.FPHash)
	}
}

// AC-4 / AC-8: the engine never panics and its output is always well-formed.
func FuzzEvaluate(f *testing.F) {
	f.Add([]byte(validSignals), chromeUA, 1, 1, 1, -1, false, false, true, "enforce")
	f.Add([]byte(`{}`), "", 64, 64, 64, 64, true, true, true, "shadow")
	f.Add([]byte(nil), "curl/8", 0, 0, 0, 0, false, false, false, "enforce")
	f.Add([]byte(`{"v":1,"iv":[0,0,0,0,0,0,0,0],"rt":0,"tz":"Etc/GMT+14","tzo":-840}`), "x", 3, 31, 3, 4, true, false, true, "enforce")
	f.Fuzz(func(t *testing.T, raw []byte, ua string, fpN, netN, seqN, cardN int, hasSlot, sticky, known bool, mode string) {
		if mode != "shadow" {
			mode = "enforce"
		}
		cfg := cfgWith(t, "ANTIBOT_MODE", mode)
		in := input(raw)
		in.UserAgent = ua
		st := antibot.State{FPIdentities: fpN, NetIdentities: netN, SeqIdentities: seqN, CardIdentities: cardN,
			HasSlot: hasSlot, Sticky: sticky, Known: known, JoinGapsMs: []int64{int64(fpN), int64(netN), int64(seqN), int64(cardN)}}
		d := eval(t, cfg, in, st)
		if d.Score < 0 || d.Score > 1 || math.IsNaN(d.Score) || math.Round(d.Score*1000)/1000 != d.Score {
			t.Fatalf("bad score %v", d.Score)
		}
		switch d.Action {
		case pass, challenge, slowLane, block:
		default:
			t.Fatalf("bad action %q", d.Action)
		}
		if d.Action == block && len(d.Fired) == 0 && !hasTag(d, "STICKY") {
			t.Fatalf("BLOCK without rule or STICKY: %+v", d)
		}
		if mode == "shadow" && d.Applied != pass {
			t.Fatalf("shadow applied %s", d.Applied)
		}
		if !slices.IsSorted(d.Fired) {
			t.Fatalf("fired not sorted %v", d.Fired)
		}
		if d.FPHash != "" && !hex32.MatchString(d.FPHash) {
			t.Fatalf("FPHash %q", d.FPHash)
		}
	})
}
