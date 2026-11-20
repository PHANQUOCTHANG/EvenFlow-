package httpadapter_test

// ops-warroom fix round 1: X-1 (rate_per_sec must be a JSON integer literal).

import (
	"testing"
)

// X-1 / TCR-4: rate_per_sec must be a plain integer literal: digits only, no sign, decimal point or exponent.
func TestOpsPutFix_X1_RateMustBeIntegerLiteral(t *testing.T) {
	for _, lit := range []string{`1e2`, `1E2`, `1e0`, `100.0`, `-0`, `0.0`, `1.0e1`, `10000.00`, `-0.0`, `01`, `1e-0`, `1E+2.0`} {
		r := newOpsRig(true)
		rec := put(r, `{"rate_per_sec":`+lit+`,"reason":"abc"}`)
		if rec.Code != 400 || r.store.writes() != 0 {
			t.Errorf("rate=%s: status %d writes=%d %s", lit, rec.Code, r.store.writes(), rec.Body)
		}
	}
	for _, lit := range []string{`0`, `1`, `100`, `10000`} {
		r := newOpsRig(true)
		if rec := put(r, `{"rate_per_sec":`+lit+`,"reason":"abc"}`); rec.Code != 200 {
			t.Errorf("rate=%s must pass: %d %s", lit, rec.Code, rec.Body)
		}
	}
}
