package antibot

import (
	"math"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	_ "time/tzdata" // R5W zone checks must work in images without a zoneinfo database
)

// ruleSet is the outcome of the rule table for one join.
type ruleSet struct {
	fired [len(allRules)]bool
	// blockEligible: a positive sign fired (D11): R1, R2, R4D, R6, R7, or R5 from a non-missing strong sign.
	blockEligible bool
}

func (r *ruleSet) fire(id RuleID) {
	for i, x := range allRules {
		if x == id {
			r.fired[i] = true
			return
		}
	}
}

// evaluateRules runs R1-R7. A rule whose input is absent is not evaluated, except R3/R5 for a missing payload
// and R3 for absent counts (contracts 1, N-6). Stateful rules need a known state (D7).
func evaluateRules(cfg *Config, in *Input, sig *Signals, st *State) ruleSet {
	var rs ruleSet
	p := &cfg.Rules
	missing := sig.Missing()

	// R1 machine rhythm: client intervals or server join gaps.
	r1 := false
	if sig.HasIV && len(sig.IV) >= p.R1ClientMin {
		if cv, ok := cvInts(sig.IV, true); ok && cv < p.R1ClientCV {
			r1 = true
		}
	}
	if !r1 && st.Known && len(st.JoinGapsMs) >= p.R1ServerMin {
		if cv, ok := cvInt64s(st.JoinGapsMs); ok && cv < p.R1ServerCV {
			r1 = true
		}
	}
	if r1 {
		rs.fire(R1)
		rs.blockEligible = true
	}

	// R2 too fast.
	if sig.RT.OK && sig.RT.V < p.R2Ms {
		rs.fire(R2)
		rs.blockEligible = true
	}

	// R3 no interaction: all counts absent or 0, or the payload is missing.
	if missing || (sig.PE.V == 0 && sig.SE.V == 0 && sig.KE.V == 0) {
		rs.fire(R3)
	}

	// R4D shared device, R4N shared network.
	if st.Known && sig.FP != "" && st.FPIdentities > p.R4DMax {
		rs.fire(R4D)
		rs.blockEligible = true
	}
	if st.Known && !st.NetSuppressed && st.NetIdentities > p.R4NMax {
		rs.fire(R4N)
	}

	// R5 strong sign, else R5W from >= 2 weak signs.
	realStrong := in.UserAgent == "" || uaDenied(cfg.UADeny, in.UserAgent) || (sig.WD.OK && sig.WD.V)
	switch {
	case realStrong:
		rs.fire(R5)
		rs.blockEligible = true
	case missing:
		rs.fire(R5)
	default:
		weak := 0
		if sig.GL.OK && !sig.GL.V {
			weak++
		}
		if sig.FC.OK && sig.FC.V < p.R5FontsMin && !mobileUA(in.UserAgent) {
			weak++
		}
		if sig.TZ != "" && sig.TZO.OK && zoneDisagrees(sig.TZ, sig.TZO.V, in.Now) {
			weak++
		}
		if weak >= 2 {
			rs.fire(R5W)
		}
	}

	// R6 shared card (dormant: CardIdentities is -1 in phase 1).
	if st.Known && st.CardIdentities > p.R6Max {
		rs.fire(R6)
		rs.blockEligible = true
	}

	// R7 replayed sequence.
	if st.Known && sig.SQ != "" && sig.SN.OK && sig.SN.V >= p.R7MinEvents && sig.SB.OK && sig.SB.V >= p.R7MinBuckets &&
		st.SeqIdentities >= p.R7MinIDs {
		rs.fire(R7)
		rs.blockEligible = true
	}
	return rs
}

// cvInts is the coefficient of variation (population stdev / mean). A zero mean counts as CV 0 when zeroOK.
func cvInts(xs []int, zeroOK bool) (float64, bool) {
	var sum float64
	for _, x := range xs {
		sum += float64(x)
	}
	mean := sum / float64(len(xs))
	if mean <= 0 {
		return 0, zeroOK && mean == 0
	}
	var ss float64
	for _, x := range xs {
		d := float64(x) - mean
		ss += d * d
	}
	return math.Sqrt(ss/float64(len(xs))) / mean, true
}

func cvInt64s(xs []int64) (float64, bool) {
	var sum float64
	for _, x := range xs {
		sum += float64(x)
	}
	mean := sum / float64(len(xs))
	if mean <= 0 || math.IsNaN(mean) || math.IsInf(mean, 0) {
		return 0, false
	}
	var ss float64
	for _, x := range xs {
		d := float64(x) - mean
		ss += d * d
	}
	return math.Sqrt(ss/float64(len(xs))) / mean, true
}

// uaDenied is a case-insensitive substring match against the (lower-case) deny list.
func uaDenied(deny []string, ua string) bool {
	lower := strings.ToLower(ua)
	for _, d := range deny {
		if strings.Contains(lower, d) {
			return true
		}
	}
	return false
}

func mobileUA(ua string) bool {
	return strings.Contains(ua, "Android") || strings.Contains(ua, "iPhone") || strings.Contains(ua, "iPad") ||
		strings.Contains(ua, "Mobile")
}

// zone cache: only valid zones are stored, at most maxZones entries.
const maxZones = 600

var (
	zoneCache sync.Map // string -> *time.Location
	zoneCount atomic.Int64
)

func loadZone(name string) (*time.Location, bool) {
	if v, ok := zoneCache.Load(name); ok {
		return v.(*time.Location), true
	}
	if name == "Local" || name == "" {
		return nil, false // the server's zone says nothing about the client
	}
	loc, err := time.LoadLocation(name)
	if err != nil {
		return nil, false
	}
	if zoneCount.Load() < maxZones {
		if _, loaded := zoneCache.LoadOrStore(name, loc); !loaded {
			zoneCount.Add(1)
		}
	}
	return loc, true
}

// zoneDisagrees reports that the IANA zone's offset at now differs from tzo (minutes east of UTC).
// An unknown zone is not evaluated.
func zoneDisagrees(name string, tzo int, now time.Time) bool {
	loc, ok := loadZone(name)
	if !ok {
		return false
	}
	if now.IsZero() {
		now = time.Now()
	}
	_, off := now.In(loc).Zone()
	return off/60 != tzo
}
