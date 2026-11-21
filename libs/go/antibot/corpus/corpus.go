// Package corpus generates the deterministic synthetic session corpus used by the anti-bot regression tests
// (AC-18, AC-19). It is test fixture code: it does not import the engine and knows nothing about rule weights or
// thresholds. Human behaviour comes from the distributions documented in
// libs/go/antibot/testdata/corpus/README.md; bot behaviour from the bot families described there.
//
// The corpus is a regression check, not proof of a low false-positive rate (spec Amendment 1, AC-18).
package corpus

import (
	"encoding/json"
	"fmt"
	"hash/fnv"
	"math"
	"math/rand/v2"
	"sort"
)

// DefaultSeed is the seed the committed tests use.
const DefaultSeed uint64 = 20261122

// EventID is the single on-sale event every session joins.
const EventID = "corpus-ev"

// Bot families gated by AC-18 / AC-19.
const (
	FamilyRawHTTP       = "raw_http"
	FamilyHeadless      = "headless"
	FamilyStealthRhythm = "stealth_rhythm"
	FamilyAccountFarm   = "account_farm"
	FamilyReplayPool    = "replay_proxy_pool"
)

// FamilyReplayResidential is informational only: replay over distinct residential networks scores R7 alone
// (CHALLENGE → pass until EV-176, contracts 3) and is not part of the >= 95% gate.
const FamilyReplayResidential = "replay_residential"

// GatedBotFamilies are the five families of AC-18.
var GatedBotFamilies = []string{FamilyRawHTTP, FamilyHeadless, FamilyStealthRhythm, FamilyAccountFarm, FamilyReplayPool}

// Join is one POST /queue/join of a session.
type Join struct {
	AtMs      int64           // offset from the corpus start (T0)
	UserAgent string          // User-Agent header
	IP        string          // first X-Forwarded-For element
	Signals   json.RawMessage // body "signals" value; nil = the field is absent
}

// Session is one identity's joins (one or more).
type Session struct {
	Family     string // bot family, or "human/<kind>"
	Bot        bool
	IdentityID string
	DeviceID   string // principal device id (session device, unique per identity)
	Joins      []Join
}

// Corpus holds the generated sessions.
type Corpus struct {
	Bots   []Session
	Humans []Session
}

// AllJoins returns every join with its session, ordered by time (ties by identity) for replay.
func (c Corpus) AllJoins() []SessionJoin {
	var out []SessionJoin
	for _, group := range [][]Session{c.Bots, c.Humans} {
		for i := range group {
			for j := range group[i].Joins {
				out = append(out, SessionJoin{Session: &group[i], Index: j})
			}
		}
	}
	sort.SliceStable(out, func(a, b int) bool {
		ja, jb := out[a].Join(), out[b].Join()
		if ja.AtMs != jb.AtMs {
			return ja.AtMs < jb.AtMs
		}
		return out[a].Session.IdentityID < out[b].Session.IdentityID
	})
	return out
}

// SessionJoin points at one join of a session.
type SessionJoin struct {
	Session *Session
	Index   int
}

// Join returns the join.
func (s SessionJoin) Join() Join { return s.Session.Joins[s.Index] }

// Generate builds the corpus deterministically from seed.
func Generate(seed uint64) Corpus {
	g := &gen{r: rand.New(rand.NewPCG(seed, seed^0x9e3779b97f4a7c15))}
	var c Corpus
	c.Bots = append(c.Bots, g.rawHTTP(200)...)
	c.Bots = append(c.Bots, g.headless(200)...)
	c.Bots = append(c.Bots, g.stealth(200)...)
	c.Bots = append(c.Bots, g.farm(200)...)
	c.Bots = append(c.Bots, g.replayPool(200)...)
	c.Bots = append(c.Bots, g.replayResidential(50)...)
	c.Humans = g.humans(5000)
	return c
}

// ---- generator internals -------------------------------------------------------------------------------------

type gen struct {
	r   *rand.Rand
	seq int
}

// openingSpikeMs: join time after T0. Opening spike shape (README "arrival"): 70% within ~20 s (exponential,
// mean 8 s), the rest spread uniformly over 4 minutes.
func (g *gen) openingSpikeMs() int64 {
	if g.r.Float64() < 0.7 {
		return int64(g.r.ExpFloat64() * 8000)
	}
	return int64(g.r.Float64() * 240000)
}

func (g *gen) id(prefix string) string {
	g.seq++
	return fmt.Sprintf("%s-%06d", prefix, g.seq)
}

func (g *gen) hex16() string { return fmt.Sprintf("%016x", g.r.Uint64()) }

func (g *gen) lognormal(median, sigma float64) float64 {
	return median * math.Exp(sigma*g.r.NormFloat64())
}

func clampInt(v, lo, hi int) int { return max(lo, min(hi, v)) }

// randomIPv4 picks an address from a large public-looking space (distinct /24s are overwhelmingly likely).
func (g *gen) randomIPv4() string {
	return fmt.Sprintf("%d.%d.%d.%d", 11+g.r.IntN(200), g.r.IntN(256), g.r.IntN(256), 1+g.r.IntN(254))
}

func (g *gen) randomIPv6() string {
	return fmt.Sprintf("2001:db8:%x:%x::%x", g.r.IntN(65536), g.r.IntN(65536), 1+g.r.IntN(65535))
}

func (g *gen) ipIn24(net string) string { return fmt.Sprintf("%s.%d", net, 1+g.r.IntN(254)) }

var desktopUAs = []string{
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36",
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36 Edg/130.0.0.0",
	"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.1 Safari/605.1.15",
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:132.0) Gecko/20100101 Firefox/132.0",
	"Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36",
}

var mobileUAs = []string{
	"Mozilla/5.0 (Linux; Android 14; SM-A546E) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Mobile Safari/537.36",
	"Mozilla/5.0 (iPhone; CPU iPhone OS 18_1 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.1 Mobile/15E148 Safari/604.1",
	"Mozilla/5.0 (Linux; Android 13; Redmi Note 12) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Mobile Safari/537.36",
}

// zones: buyer time zones (README "time zone"): mostly Vietnam, some overseas buyers.
var zones = []struct {
	tz  string
	tzo int // offset on the corpus date (December, northern winter)
	w   float64
}{
	{"Asia/Ho_Chi_Minh", 420, 0.80}, {"Asia/Bangkok", 420, 0.05}, {"Asia/Singapore", 480, 0.04},
	{"Asia/Tokyo", 540, 0.03}, {"Australia/Sydney", 660, 0.02}, {"Europe/Berlin", 60, 0.03},
	{"America/Los_Angeles", -480, 0.02}, {"UTC", 0, 0.01},
}

func (g *gen) zone() (string, int) {
	x := g.r.Float64()
	for _, z := range zones {
		if x < z.w {
			return z.tz, z.tzo
		}
		x -= z.w
	}
	return zones[0].tz, zones[0].tzo
}

// snapshot is the v1 payload (contracts 1) as the web collector would send it.
type snapshot struct {
	V   int    `json:"v"`
	RT  int    `json:"rt"`
	PE  int    `json:"pe"`
	SE  int    `json:"se"`
	KE  int    `json:"ke"`
	IV  []int  `json:"iv,omitempty"`
	SQ  string `json:"sq,omitempty"`
	SN  int    `json:"sn,omitempty"`
	SB  int    `json:"sb,omitempty"`
	FP  string `json:"fp,omitempty"`
	WD  bool   `json:"wd"`
	GL  bool   `json:"gl"`
	FC  int    `json:"fc"`
	TZ  string `json:"tz,omitempty"`
	TZO int    `json:"tzo"`
}

func (s snapshot) raw() json.RawMessage {
	b, err := json.Marshal(s)
	if err != nil {
		panic(err)
	}
	return b
}

// discrete events → iv (last 32 intervals), sq / sn / sb exactly as contracts 1 (R2) define them.
type event struct {
	kind string // "p" pointerdown/touchstart, "k" keydown, "s" first scroll of a burst
	gap  int    // ms since the previous discrete event (0 for the first)
}

func fold(rt int, evs []event, s *snapshot) {
	var iv []int
	for i, e := range evs {
		if i > 0 {
			iv = append(iv, clampInt(e.gap, 0, 60000))
		}
	}
	if len(iv) > 32 {
		iv = iv[len(iv)-32:]
	}
	s.IV = iv
	last := evs
	if len(last) > 32 {
		last = last[len(last)-32:]
	}
	buckets := map[int]bool{}
	h := fnv.New64a()
	rtb := min(rt/250, 40)
	fmt.Fprintf(h, "rtb:%d", rtb)
	buckets[rtb] = true
	for _, e := range last {
		b := min(e.gap/50, 40)
		buckets[b] = true
		fmt.Fprintf(h, "|%s:%d", e.kind, b)
	}
	if len(last) >= 12 && len(buckets) >= 4 {
		s.SQ = fmt.Sprintf("%016x", h.Sum64())
		s.SN = len(last)
		s.SB = min(len(buckets), 41)
	}
}

// ---- humans ---------------------------------------------------------------------------------------------------

// Human kinds and their prevalence (README "population"). Kinds are not exclusive with the shared-network and
// household overlays applied afterwards.
func (g *gen) humans(n int) []Session {
	out := make([]Session, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, g.human())
	}
	g.overlayCampus(out)
	g.overlayHouseholds(out)
	return out
}

func (g *gen) human() Session {
	mobile := g.r.Float64() < 0.62 // README: share of mobile web traffic in Vietnam
	kind := "desktop"
	ua := desktopUAs[g.r.IntN(len(desktopUAs))]
	if mobile {
		kind = "mobile"
		ua = mobileUAs[g.r.IntN(len(mobileUAs))]
	}
	privacy := g.r.Float64() < 0.05 // README: privacy-hardened browsers (Brave shields, Firefox RFP, Tor)
	keyboardOnly := !mobile && g.r.Float64() < 0.02
	autorepeat := !mobile && g.r.Float64() < 0.005 // Enter held on the button: keydown auto-repeat ~30-35 ms
	waitedOnPage := g.r.Float64() < 0.70           // opened the event page before T0 and waited

	// rt: render → join click.
	var rt float64
	switch {
	case waitedOnPage:
		rt = 5000 + g.r.Float64()*900000 // waited seconds to 15 min; capped below by the range check
	case g.r.Float64() < 0.03:
		rt = g.lognormal(170, 0.35) // README: pre-positioned pointer, click on render ("fast clicker")
		kind += "/fast"
	default:
		rt = g.lognormal(2600, 0.8) // README: visual search + Fitts pointing after the page renders
	}
	rtI := clampInt(int(rt), 0, 600000)

	// discrete events before the click: count ~ geometric, gaps lognormal (README "inter-event").
	nEv := int(g.r.ExpFloat64() * 10)
	var evs []event
	pe, se, ke := 0, 0, 0
	for j := 0; j < nEv; j++ {
		gap := int(g.lognormal(420, 0.9))
		k := "p"
		switch x := g.r.Float64(); {
		case keyboardOnly || x < 0.25:
			k = "k"
			ke++
		case x < 0.55:
			k = "s"
			se++
		default:
			pe++
		}
		evs = append(evs, event{kind: k, gap: gap})
	}
	if autorepeat {
		kind += "/autorepeat"
		for j := 0; j < 20+g.r.IntN(20); j++ {
			evs = append(evs, event{kind: "k", gap: 30 + g.r.IntN(6)})
			ke++
		}
	}
	// sampled moves / scroll repeats feed counts only (D14)
	if !keyboardOnly {
		pe += int(g.r.ExpFloat64() * 30)
		se += int(g.r.ExpFloat64() * 5)
	}
	// the join click itself (recorded before the snapshot). Auto-repeat users hold Enter on the focused, still
	// disabled button until it enables at T0: the join is triggered by one more repeat.
	clickGap := int(g.lognormal(600, 0.8))
	if autorepeat {
		evs = append(evs, event{kind: "k", gap: 30 + g.r.IntN(6)})
		ke++
	} else if keyboardOnly {
		evs = append(evs, event{kind: "k", gap: clickGap})
		ke++
		kind += "/keyboard"
	} else {
		evs = append(evs, event{kind: "p", gap: clickGap})
		pe++
	}

	tz, tzo := g.zone()
	gl := true
	fc := clampInt(int(g.r.NormFloat64()*2.5+11), 0, 20) // README "fonts": desktop detects most of the list
	if mobile {
		fc = clampInt(int(g.r.NormFloat64()*1.5+3), 0, 20) // mobile ships few of the desktop fonts
	}
	if privacy {
		kind += "/privacy"
		if g.r.Float64() < 0.5 {
			gl = false
		}
		fc = clampInt(int(g.r.NormFloat64()*1.5+2), 0, 20) // font allow-lists report few fonts
		if g.r.Float64() < 0.3 {
			tz, tzo = "UTC", 0 // RFP reports UTC consistently
		}
	}
	if g.r.Float64() < 0.004 { // README: spoofing extensions report a zone that disagrees with the offset
		tzo = (tzo + 60*(1+g.r.IntN(5))) % 840
		kind += "/tzspoof"
	}

	s := snapshot{V: 1, RT: rtI, PE: min(pe, 10000), SE: min(se, 10000), KE: min(ke, 10000),
		FP: "h" + g.hex16(), GL: gl, FC: fc, TZ: tz, TZO: tzo}
	fold(rtI, evs, &s)

	ip := g.randomIPv4()
	if g.r.Float64() < 0.2 {
		ip = g.randomIPv6()
	}
	at := g.openingSpikeMs()
	sess := Session{Family: "human/" + kind, IdentityID: g.id("hum"), DeviceID: g.id("hdev")}
	sess.Joins = append(sess.Joins, Join{AtMs: at, UserAgent: ua, IP: ip, Signals: s.raw()})
	// README "re-join": 10% re-join 1-3 times (token expiry, second tab); signals are the stored snapshot.
	if g.r.Float64() < 0.10 {
		for k := 0; k < 1+g.r.IntN(3); k++ {
			at += 2000 + int64(g.r.ExpFloat64()*30000)
			sess.Joins = append(sess.Joins, Join{AtMs: at, UserAgent: ua, IP: ip, Signals: s.raw()})
		}
	}
	return sess
}

// overlayCampus puts two clusters of 40 humans each on one carrier / campus /24 within 5 minutes
// (contracts 8.3) and a handful of pairs on small office /24s.
func (g *gen) overlayCampus(hs []Session) {
	for c, net := range []string{"100.64.17", "100.80.3"} {
		for i := 0; i < 40; i++ {
			h := &hs[c*40+i]
			for j := range h.Joins {
				h.Joins[j].IP = g.ipIn24(net)
				h.Joins[j].AtMs = int64(g.r.Float64() * 280000)
			}
			h.Family += "/cgnat"
		}
	}
}

// overlayHouseholds: 3% of humans share a device fingerprint with family members (README "household"):
// household size 2 (80%), 3 (15%), 4 (5%). Sizes come from household data, not from R4D.
func (g *gen) overlayHouseholds(hs []Session) {
	start := 200 // after the campus clusters
	target := len(hs) * 3 / 100
	i := start
	for i < start+target && i < len(hs) {
		size := 2
		if x := g.r.Float64(); x < 0.05 {
			size = 4
		} else if x < 0.20 {
			size = 3
		}
		var fp string
		for k := 0; k < size && i < len(hs); k++ {
			h := &hs[i]
			var m map[string]any
			_ = json.Unmarshal(h.Joins[0].Signals, &m)
			if k == 0 {
				fp, _ = m["fp"].(string)
			}
			m["fp"] = fp
			b, _ := json.Marshal(m)
			for j := range h.Joins {
				h.Joins[j].Signals = b
			}
			h.Family += "/household"
			i++
		}
	}
}

// ---- bots -----------------------------------------------------------------------------------------------------

var scriptUAs = []string{"python-requests/2.32.3", "curl/8.5.0", "Go-http-client/1.1", "axios/1.7.2", "python-httpx/0.27.0", "okhttp/4.12.0"}

// rawHTTP: scripts that POST the join without a browser: no signals (body {} or {"signals":{}}).
func (g *gen) rawHTTP(n int) []Session {
	var out []Session
	for i := 0; i < n; i++ {
		var sig json.RawMessage
		if i%2 == 0 {
			sig = json.RawMessage(`{}`)
		}
		s := Session{Family: FamilyRawHTTP, Bot: true, IdentityID: g.id("bot"), DeviceID: g.id("bdev")}
		at := int64(g.r.IntN(3000))
		for k := 0; k < 1+g.r.IntN(5); k++ { // retry loop
			s.Joins = append(s.Joins, Join{AtMs: at, UserAgent: scriptUAs[i%len(scriptUAs)], IP: g.ipIn24(fmt.Sprintf("45.%d.%d", 10+i%4, 7)), Signals: sig})
			at += 500
		}
		out = append(out, s)
	}
	return out
}

// headless: default Puppeteer/Playwright: element.click() (no pointerdown before the snapshot except the
// synthetic click), navigator.webdriver true, near-instant click after render.
func (g *gen) headless(n int) []Session {
	var out []Session
	for i := 0; i < n; i++ {
		ua := "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) HeadlessChrome/131.0.0.0 Safari/537.36"
		if i%5 >= 3 {
			ua = desktopUAs[0] // UA patched, webdriver flag not
		}
		rt := 20 + g.r.IntN(90)
		s := snapshot{V: 1, RT: rt, PE: 1, FP: "b" + g.hex16(), WD: true, GL: true, FC: 3 + g.r.IntN(3), TZ: "UTC", TZO: 0}
		out = append(out, Session{Family: FamilyHeadless, Bot: true, IdentityID: g.id("bot"), DeviceID: g.id("bdev"),
			Joins: []Join{{AtMs: int64(g.r.IntN(2000)), UserAgent: ua, IP: g.ipIn24(fmt.Sprintf("34.%d.88", 100+i%8)), Signals: s.raw()}}})
	}
	return out
}

// stealth: patched automation with a fixed-rhythm script (setInterval ~100 ms, ±2 ms timer jitter) and a fast click.
func (g *gen) stealth(n int) []Session {
	var out []Session
	for i := 0; i < n; i++ {
		var evs []event
		for k := 0; k < 10+g.r.IntN(12); k++ {
			kind := "p"
			if k%3 == 0 {
				kind = "k"
			}
			evs = append(evs, event{kind: kind, gap: 98 + g.r.IntN(5)})
		}
		rt := 60 + g.r.IntN(55)
		s := snapshot{V: 1, RT: rt, PE: 8 + g.r.IntN(5), KE: 4, FP: "b" + g.hex16(), GL: true, FC: 10 + g.r.IntN(5), TZ: "Asia/Ho_Chi_Minh", TZO: 420}
		fold(rt, evs, &s)
		out = append(out, Session{Family: FamilyStealthRhythm, Bot: true, IdentityID: g.id("bot"), DeviceID: g.id("bdev"),
			Joins: []Join{{AtMs: int64(g.r.IntN(5000)), UserAgent: desktopUAs[1], IP: g.randomIPv4(), Signals: s.raw()}}})
	}
	return out
}

// farm: many accounts driven from a few machines (shared device id) behind two /24s, human-like timing copied
// from a human generator draw (the farm operator clicks by hand or replays jittered timing).
func (g *gen) farm(n int) []Session {
	var out []Session
	devices := []string{"farmdev-" + g.hex16(), "farmdev-" + g.hex16(), "farmdev-" + g.hex16(), "farmdev-" + g.hex16()}
	for i := 0; i < n; i++ {
		h := g.human()
		var m map[string]any
		_ = json.Unmarshal(h.Joins[0].Signals, &m)
		m["fp"] = devices[i%len(devices)]
		b, _ := json.Marshal(m)
		out = append(out, Session{Family: FamilyAccountFarm, Bot: true, IdentityID: g.id("bot"), DeviceID: g.id("bdev"),
			Joins: []Join{{AtMs: int64(i) * 600, UserAgent: desktopUAs[0], IP: g.ipIn24([]string{"103.77.12", "103.77.13"}[i%2]), Signals: b}}})
	}
	return out
}

// replay: one recorded human session replayed verbatim by many identities (same sq) through a datacenter proxy /24.
func (g *gen) replayPool(n int) []Session {
	rec := g.recordedHuman()
	var out []Session
	for i := 0; i < n; i++ {
		s := rec
		s.FP = "b" + g.hex16()
		out = append(out, Session{Family: FamilyReplayPool, Bot: true, IdentityID: g.id("bot"), DeviceID: g.id("bdev"),
			Joins: []Join{{AtMs: int64(i) * 900, UserAgent: desktopUAs[0], IP: g.ipIn24("185.220.101"), Signals: s.raw()}}})
	}
	return out
}

// replayResidential: same replay through distinct residential networks (informational, not gated).
func (g *gen) replayResidential(n int) []Session {
	rec := g.recordedHuman()
	var out []Session
	for i := 0; i < n; i++ {
		s := rec
		s.FP = "b" + g.hex16()
		out = append(out, Session{Family: FamilyReplayResidential, Bot: true, IdentityID: g.id("bot"), DeviceID: g.id("bdev"),
			Joins: []Join{{AtMs: int64(i) * 1500, UserAgent: desktopUAs[2], IP: g.randomIPv4(), Signals: s.raw()}}})
	}
	return out
}

func (g *gen) recordedHuman() snapshot {
	var evs []event
	for k := 0; k < 24; k++ {
		evs = append(evs, event{kind: []string{"p", "s", "k"}[k%3], gap: int(g.lognormal(420, 0.9))})
	}
	rt := 6400
	s := snapshot{V: 1, RT: rt, PE: 30, SE: 8, KE: 8, GL: true, FC: 12, TZ: "Asia/Ho_Chi_Minh", TZO: 420}
	fold(rt, evs, &s)
	return s
}
