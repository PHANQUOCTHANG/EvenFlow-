package antibot_test

import (
	"encoding/json"
	"fmt"
	"math"
	"net/netip"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/eventflow/eventflow/libs/go/antibot"
	"github.com/eventflow/eventflow/libs/go/antibot/corpus"
)

// emulator mirrors observe.lua (contracts 4, R2 + plan review round 2 N-2) in memory, and the queue's slot /
// block-flag bookkeeping, so the corpus can be replayed through the pure engine with realistic state.
type emulator struct {
	slots   map[string]antibot.Action // identity -> lane of its slot
	blocked map[string]bool
	jt      map[string][]int64 // newest first
	fp      map[string]map[string]int64
	net     map[string]map[string]int64
	sq      map[string]map[string]int64
	jc      map[int64]int
	nj      map[string]map[int64]int
}

const (
	emuR4Window  = 300_000
	emuR7Window  = 900_000
	emuZCap      = 64
	emuShare     = 0.05
	emuShareMinJ = 1000
)

func newEmulator() *emulator {
	return &emulator{slots: map[string]antibot.Action{}, blocked: map[string]bool{}, jt: map[string][]int64{},
		fp: map[string]map[string]int64{}, net: map[string]map[string]int64{}, sq: map[string]map[string]int64{},
		jc: map[int64]int{}, nj: map[string]map[int64]int{}}
}

func zadd(z map[string]map[string]int64, key, id string, now, window int64) int {
	m := z[key]
	if m == nil {
		m = map[string]int64{}
		z[key] = m
	}
	m[id] = now
	for k, v := range m {
		if v < now-window {
			delete(m, k)
		}
	}
	if len(m) > emuZCap { // keep newest 64
		type kv struct {
			k string
			v int64
		}
		all := make([]kv, 0, len(m))
		for k, v := range m {
			all = append(all, kv{k, v})
		}
		sort.Slice(all, func(i, j int) bool { return all[i].v > all[j].v })
		for _, e := range all[emuZCap:] {
			delete(m, e.k)
		}
	}
	return len(m)
}

func (e *emulator) observe(id, fp string, prefix netip.Prefix, sq string, now int64) antibot.State {
	_, hasSlot := e.slots[id]
	if e.blocked[id] {
		return antibot.State{Sticky: true, HasSlot: hasSlot, Known: true, CardIdentities: -1}
	}
	st := antibot.State{HasSlot: hasSlot, Known: true, CardIdentities: -1}
	if !hasSlot {
		l := e.jt[id]
		if len(l) > 0 && now-l[0] > 300_000 {
			l = nil
		}
		l = append([]int64{now}, l...)
		if len(l) > 9 {
			l = l[:9]
		}
		e.jt[id] = l
		for i := 0; i+1 < len(l); i++ {
			st.JoinGapsMs = append(st.JoinGapsMs, l[i]-l[i+1])
		}
	}
	if fp != "" {
		st.FPIdentities = zadd(e.fp, fp, id, now, emuR4Window)
	}
	minute := now / 60_000
	e.jc[minute]++
	if prefix.IsValid() {
		p := prefix.String()
		st.NetIdentities = zadd(e.net, p, id, now, emuR4Window)
		if e.nj[p] == nil {
			e.nj[p] = map[int64]int{}
		}
		e.nj[p][minute]++
		var pj, ej int
		for m := minute - 4; m <= minute; m++ {
			pj += e.nj[p][m]
			ej += e.jc[m]
		}
		st.NetSuppressed = ej >= emuShareMinJ && float64(pj)/float64(ej) > emuShare
	}
	if sq != "" {
		st.SeqIdentities = zadd(e.sq, sq, id, now, emuR7Window)
	}
	return st
}

// apply records the queue effect of an enforce decision.
func (e *emulator) apply(id string, d antibot.Decision) {
	switch d.Applied {
	case block:
		delete(e.slots, id)
		e.blocked[id] = true
	default:
		if _, ok := e.slots[id]; !ok {
			e.slots[id] = d.Applied
		}
	}
}

type rawFields struct {
	FP string `json:"fp"`
	SQ string `json:"sq"`
}

func fieldsOf(raw json.RawMessage) rawFields {
	var f rawFields
	_ = json.Unmarshal(raw, &f)
	return f
}

type outcome struct {
	result   antibot.Action // BLOCK if any join BLOCK, else first join's Applied (contracts 8.2)
	decisive antibot.Decision
	blocks   []antibot.Decision
}

var corpusStart = time.Date(2026, 12, 1, 12, 0, 0, 0, time.UTC)

func replayCorpus(t testing.TB, cfg antibot.Config, c corpus.Corpus) map[*corpus.Session]*outcome {
	t.Helper()
	emu := newEmulator()
	out := map[*corpus.Session]*outcome{}
	for _, sj := range c.AllJoins() {
		s, j := sj.Session, sj.Join()
		ip, _ := netip.ParseAddr(j.IP)
		in := antibot.Input{EventID: corpus.EventID, IdentityID: s.IdentityID, DeviceID: s.DeviceID,
			UserAgent: j.UserAgent, ClientIP: ip, RawSignals: j.Signals, Now: corpusStart.Add(time.Duration(j.AtMs) * time.Millisecond)}
		sig, _ := antibot.DecodeSignals(j.Signals)
		f := fieldsOf(j.Signals)
		var prefix netip.Prefix
		if ip.IsValid() {
			prefix = antibot.NetPrefix(ip)
		}
		fp, sq := "", ""
		if !sig.Missing() {
			fp, sq = f.FP, f.SQ
		}
		st := emu.observe(s.IdentityID, fp, prefix, sq, j.AtMs)
		d := antibot.Evaluate(cfg, in, sig, st)
		emu.apply(s.IdentityID, d)
		o := out[s]
		if o == nil {
			o = &outcome{result: d.Applied, decisive: d}
			out[s] = o
		}
		if d.Applied == block {
			o.blocks = append(o.blocks, d)
			if o.result != block {
				o.result, o.decisive = block, d
			}
		}
	}
	return out
}

func demoted(a antibot.Action) bool { return a == slowLane || a == block }

type rates struct {
	botsCaught, botsTotal, humansDemoted, humansTotal int
}

func (r rates) bot() float64   { return float64(r.botsCaught) / float64(r.botsTotal) }
func (r rates) human() float64 { return float64(r.humansDemoted) / float64(r.humansTotal) }

func gated(family string) bool {
	for _, f := range corpus.GatedBotFamilies {
		if f == family {
			return true
		}
	}
	return false
}

func summarize(c corpus.Corpus, res map[*corpus.Session]*outcome) rates {
	var r rates
	for i := range c.Bots {
		s := &c.Bots[i]
		if !gated(s.Family) {
			continue
		}
		r.botsTotal++
		if demoted(res[s].result) {
			r.botsCaught++
		}
	}
	for i := range c.Humans {
		r.humansTotal++
		if demoted(res[&c.Humans[i]].result) {
			r.humansDemoted++
		}
	}
	return r
}

// AC-18: regression check on the synthetic corpus with default config (enforce).
func TestCorpus_AC18_DefaultConfig(t *testing.T) {
	c := corpus.Generate(corpus.DefaultSeed)
	if len(c.Humans) < 5000 {
		t.Fatalf("humans = %d, want >= 5000", len(c.Humans))
	}
	fam := map[string]int{}
	for _, b := range c.Bots {
		fam[b.Family]++
	}
	n := 0
	for _, f := range corpus.GatedBotFamilies {
		n += fam[f]
	}
	if n < 1000 || len(corpus.GatedBotFamilies) != 5 {
		t.Fatalf("gated bots = %d in %d families, want >= 1000 in 5", n, len(corpus.GatedBotFamilies))
	}

	cfg := cfgWith(t)
	res := replayCorpus(t, cfg, c)

	// per-family confusion table
	type row struct{ n, pass, slow, blk int }
	byFam := map[string]*row{}
	ruleBots, ruleHumans := map[antibot.RuleID]int{}, map[antibot.RuleID]int{}
	for i := range c.Bots {
		s := &c.Bots[i]
		r := byFam[s.Family]
		if r == nil {
			r = &row{}
			byFam[s.Family] = r
		}
		r.n++
		switch res[s].result {
		case block:
			r.blk++
		case slowLane:
			r.slow++
		default:
			r.pass++
		}
		for _, id := range res[s].decisive.Fired {
			ruleBots[id]++
		}
	}
	humanKinds := map[string]*row{}
	for i := range c.Humans {
		s := &c.Humans[i]
		k := strings.SplitN(strings.TrimPrefix(s.Family, "human/"), "/", 2)[0]
		for _, tag := range []string{"fast", "privacy", "cgnat", "household", "autorepeat", "keyboard", "tzspoof"} {
			if strings.Contains(s.Family, tag) {
				k = tag
			}
		}
		r := humanKinds[k]
		if r == nil {
			r = &row{}
			humanKinds[k] = r
		}
		r.n++
		switch res[s].result {
		case block:
			r.blk++
		case slowLane:
			r.slow++
		default:
			r.pass++
		}
		for _, id := range res[s].decisive.Fired {
			ruleHumans[id]++
		}
	}
	var b strings.Builder
	fmt.Fprintf(&b, "\n%-22s %6s %6s %6s %6s %8s\n", "family", "n", "PASS", "SLOW", "BLOCK", "demoted")
	print := func(name string, r *row) {
		fmt.Fprintf(&b, "%-22s %6d %6d %6d %6d %7.1f%%\n", name, r.n, r.pass, r.slow, r.blk, 100*float64(r.slow+r.blk)/float64(r.n))
	}
	for _, f := range append(append([]string{}, corpus.GatedBotFamilies...), corpus.FamilyReplayResidential) {
		if r := byFam[f]; r != nil {
			print(f, r)
		}
	}
	keys := make([]string, 0, len(humanKinds))
	for k := range humanKinds {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		print("human/"+k, humanKinds[k])
	}
	fmt.Fprintf(&b, "\n%-6s %8s %8s   (sessions whose deciding decision fired the rule)\n", "rule", "bots", "humans")
	for _, id := range []antibot.RuleID{"R1", "R2", "R3", "R4D", "R4N", "R5", "R5W", "R6", "R7"} {
		fmt.Fprintf(&b, "%-6s %8d %8d\n", id, ruleBots[id], ruleHumans[id])
	}
	t.Log(b.String())

	r := summarize(c, res)
	t.Logf("bots caught %.2f%% (%d/%d), humans demoted %.2f%% (%d/%d)", 100*r.bot(), r.botsCaught, r.botsTotal, 100*r.human(), r.humansDemoted, r.humansTotal)
	if r.bot() < 0.95 {
		t.Errorf("bots caught %.2f%%, want >= 95%%", 100*r.bot())
	}
	if r.human() >= 0.01 {
		t.Errorf("humans demoted %.2f%%, want < 1%%", 100*r.human())
	}
	// every BLOCK lists >= 1 fired rule (or is a STICKY follow-up of one that did)
	for s, o := range res {
		for _, d := range o.blocks {
			if len(d.Fired) == 0 && !hasTag(d, "STICKY") {
				t.Errorf("%s %s: BLOCK without a fired rule: %+v", s.Family, s.IdentityID, d)
			}
		}
	}
}

// P-7: the human corpus has members on both sides of every threshold a human can reach.
func TestCorpus_AC18_HumansOnBothSidesOfThresholds(t *testing.T) {
	c := corpus.Generate(corpus.DefaultSeed)
	type sig struct {
		RT *int   `json:"rt"`
		IV []int  `json:"iv"`
		GL *bool  `json:"gl"`
		FC *int   `json:"fc"`
		FP string `json:"fp"`
	}
	var rtBelow, rtAbove, cvBelow, cvAbove, fcBelow, fcAbove, glOff int
	fpCount := map[string]int{}
	netCount := map[string]map[string]bool{}
	for _, h := range c.Humans {
		var s sig
		_ = json.Unmarshal(h.Joins[0].Signals, &s)
		if s.RT != nil {
			if *s.RT < 120 {
				rtBelow++
			} else if *s.RT < 400 {
				rtAbove++
			}
		}
		if len(s.IV) >= 8 {
			if cv(s.IV) < 0.10 {
				cvBelow++
			} else {
				cvAbove++
			}
		}
		if s.FC != nil && !strings.Contains(h.Joins[0].UserAgent, "Mobile") {
			if *s.FC < 3 {
				fcBelow++
			} else {
				fcAbove++
			}
		}
		if s.GL != nil && !*s.GL {
			glOff++
		}
		fpCount[s.FP]++
		ip, _ := netip.ParseAddr(h.Joins[0].IP)
		p := antibot.NetPrefix(ip).String()
		if netCount[p] == nil {
			netCount[p] = map[string]bool{}
		}
		netCount[p][h.IdentityID] = true
	}
	var fpOver, fpAtOrUnder2, netOver int
	for _, n := range fpCount {
		if n > 3 {
			fpOver++
		} else if n >= 2 {
			fpAtOrUnder2++
		}
	}
	for _, ids := range netCount {
		if len(ids) > 30 {
			netOver++
		}
	}
	t.Logf("rt<120:%d rt 120-400:%d | ivCV<0.10:%d >=:%d | fc<3:%d >=3:%d | gl off:%d | fp shared >3:%d 2-3:%d | /24 >30 ids:%d",
		rtBelow, rtAbove, cvBelow, cvAbove, fcBelow, fcAbove, glOff, fpOver, fpAtOrUnder2, netOver)
	for name, v := range map[string]int{"rt<120": rtBelow, "rt near above": rtAbove, "iv CV<0.10": cvBelow, "iv CV>=0.10": cvAbove,
		"fc<3 desktop": fcBelow, "fc>=3 desktop": fcAbove, "gl off": glOff, "fp shared by >3": fpOver, "fp shared by 2-3": fpAtOrUnder2, "/24 with >30": netOver} {
		if v == 0 {
			t.Errorf("no human population for %q", name)
		}
	}
	t.Log("not reachable by humans by construction: R1-server (needs >= 4 joins without a slot), R6 (dormant), R7 (needs a shared digest)")
}

func cv(xs []int) float64 {
	var sum float64
	for _, x := range xs {
		sum += float64(x)
	}
	mean := sum / float64(len(xs))
	if mean == 0 {
		return 0
	}
	var ss float64
	for _, x := range xs {
		ss += (float64(x) - mean) * (float64(x) - mean)
	}
	return math.Sqrt(ss/float64(len(xs))) / mean
}

// AC-18 (R2): ±20% threshold sensitivity table (printed, not asserted).
func TestCorpus_AC18_SensitivityTable(t *testing.T) {
	if testing.Short() {
		t.Skip("sensitivity table skipped in -short")
	}
	c := corpus.Generate(corpus.DefaultSeed)
	defaults := map[string]float64{"R1_CLIENT_CV": 0.10, "R2_MS": 120, "R4D_MAX": 3, "R4N_MAX": 30, "R5_FONTS_MIN": 3, "R7_MIN_IDS": 3}
	keys := make([]string, 0, len(defaults))
	for k := range defaults {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	fmt.Fprintf(&b, "\n%-14s %6s %8s %10s %12s\n", "threshold", "scale", "value", "bots %", "humans %")
	for _, k := range keys {
		for _, scale := range []float64{0.8, 1.0, 1.2} {
			v := defaults[k] * scale
			val := strconv.FormatFloat(v, 'f', 3, 64)
			if k != "R1_CLIENT_CV" {
				val = strconv.Itoa(max(1, int(math.Round(v))))
			}
			r := summarize(c, replayCorpus(t, cfgWith(t, "ANTIBOT_RULES", k+"="+val), c))
			fmt.Fprintf(&b, "%-14s %6.1f %8s %9.2f%% %11.2f%%\n", k, scale, val, 100*r.bot(), 100*r.human())
		}
	}
	t.Log(b.String())
}

// The generator is deterministic and independent of the engine.
func TestCorpus_GeneratorDeterministic(t *testing.T) {
	a, b := corpus.Generate(corpus.DefaultSeed), corpus.Generate(corpus.DefaultSeed)
	if !reflect.DeepEqual(a, b) {
		t.Fatal("same seed produced different corpora")
	}
	if reflect.DeepEqual(a, corpus.Generate(corpus.DefaultSeed+1)) {
		t.Fatal("different seed produced the same corpus")
	}
	for _, s := range append(append([]corpus.Session{}, a.Bots...), a.Humans...) {
		for _, j := range s.Joins {
			if len(j.Signals) > 2048 {
				t.Fatalf("%s: snapshot %d bytes > 2 KiB", s.Family, len(j.Signals))
			}
		}
	}
}

// AC-18 (R2): sessions recorded by the team with the real collector stay < 1% demoted.
func TestCorpus_AC18_RecordedSessions(t *testing.T) {
	files, _ := filepath.Glob(filepath.Join("testdata", "corpus", "recorded", "*.json"))
	if len(files) == 0 {
		t.Skip("PENDING (AC-18): no recorded sessions in testdata/corpus/recorded yet; the team records 50-100 with the real collector")
	}
	cfg := cfgWith(t)
	dem := 0
	for i, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		in := input(raw)
		in.IdentityID = fmt.Sprintf("rec-%d", i)
		if d := eval(t, cfg, in, neutral()); demoted(d.Applied) {
			dem++
			t.Logf("%s demoted: %s", filepath.Base(f), ids(d))
		}
	}
	if float64(dem)/float64(len(files)) >= 0.01 {
		t.Fatalf("recorded sessions demoted %d/%d, want < 1%%", dem, len(files))
	}
}
