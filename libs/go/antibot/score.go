package antibot

import (
	"math"
	"net/netip"
)

// Evaluate is the pure engine: rules, noisy-OR score, tier, D11 cap, D12 slot awareness, mode and fallback.
func Evaluate(cfg Config, in Input, sig Signals, st State) Decision {
	fp := ""
	if sig.FP != "" {
		fp = FPHash(cfg.HashKey, sig.FP)
	}
	return evaluate(&cfg, &in, &sig, &st, fp, NetPrefix(in.ClientIP))
}

func evaluate(cfg *Config, in *Input, sig *Signals, st *State, fp string, prefix netip.Prefix) Decision {
	d := Decision{FPHash: fp, NetPrefix: prefix}
	shadow := cfg.Mode == ModeShadow

	if st.Sticky {
		d.Action, d.Score = ActionBlock, 1
		d.Applied = ActionBlock
		if shadow {
			d.Applied = ActionPass
			d.Tags = []string{TagModeShadow, TagSticky}
		} else {
			d.Tags = []string{TagSticky}
		}
		return d
	}

	rs := evaluateRules(cfg, in, sig, st)
	prod := 1.0
	for i, id := range allRules {
		if rs.fired[i] {
			d.Fired = append(d.Fired, id)
			prod *= 1 - cfg.weightAt(i)
		}
	}
	d.Score = roundScore(1 - prod)
	d.Action = cfg.tier(d.Score)

	capped := false
	if d.Action == ActionBlock && !rs.blockEligible {
		d.Action, capped = ActionSlowLane, true
	}

	switch d.Action {
	case ActionChallenge:
		d.Applied = cfg.Fallback
		if d.Applied == "" {
			d.Applied = ActionPass
		}
	default:
		d.Applied = d.Action
	}
	rejoin := st.HasSlot && d.Action != ActionBlock
	if rejoin {
		d.Applied = ActionPass
	}
	if shadow {
		d.Applied = ActionPass
	}

	if shadow {
		d.Tags = append(d.Tags, TagModeShadow)
	}
	if !st.Known {
		d.Tags = append(d.Tags, TagDegraded)
	}
	if capped {
		d.Tags = append(d.Tags, TagCapped)
	}
	if rejoin {
		d.Tags = append(d.Tags, TagRejoin)
	}
	return d
}

func (c *Config) weightAt(i int) float64 {
	w := c.weights[i]
	if w == 0 && c.Weights != nil { // Config built by hand without index()
		w = c.Weights[allRules[i]]
	}
	if w < 0 || math.IsNaN(w) {
		return 0
	}
	if w > 1 {
		return 1
	}
	return w
}

func roundScore(s float64) float64 {
	if s <= 0 || math.IsNaN(s) {
		return 0
	}
	if s >= 1 {
		return 1
	}
	return math.Round(s*1000) / 1000
}

// tier maps the rounded score to a tier, lower bound inclusive.
func (c *Config) tier(score float64) Action {
	switch {
	case score >= c.Tiers[2]:
		return ActionBlock
	case score >= c.Tiers[1]:
		return ActionSlowLane
	case score >= c.Tiers[0]:
		return ActionChallenge
	default:
		return ActionPass
	}
}
