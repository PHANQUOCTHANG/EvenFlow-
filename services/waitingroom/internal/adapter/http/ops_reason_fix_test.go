package httpadapter_test

// ops-warroom fix round 1: R-1 + X-4 (reason with control or invisible characters).

import (
	"net/http"
	"testing"
)

// R-1 + X-4: reasons with control characters or without visible characters are 400 and nothing is applied.
func TestOpsWriteFix_R1_X4_ReasonControlOrInvisibleIs400(t *testing.T) {
	reasons := []string{
		`abc\u0000`,
		`ab\u0007c`,
		`ab\nc`,
		`\u200b\u200b\u200b`,
		`\u200c\u200d\ufeff`,
		`\u00a0\u3000\u2003\u200b`,
		`a\u200b\u200bb`,
	}
	for _, reason := range reasons {
		t.Run(reason, func(t *testing.T) {
			r := newOpsRig(true)
			rec := put(r, `{"rate_per_sec":5,"reason":"`+reason+`"}`)
			if rec.Code != 400 || ptype(rec) != "/problems/invalid-override" {
				t.Fatalf("PUT reason %q: status %d %s", reason, rec.Code, rec.Body)
			}
			rec = opsCall(r.mux, http.MethodDelete, ovPath(opsEvent), "ops", `{"reason":"`+reason+`"}`)
			if rec.Code != 400 {
				t.Fatalf("DELETE reason %q: status %d %s", reason, rec.Code, rec.Body)
			}
			if r.store.writes() != 0 || len(r.aud.entries) != 0 {
				t.Fatalf("nothing must be applied/audited, writes=%d audits=%d", r.store.writes(), len(r.aud.entries))
			}
		})
	}
	// Visible characters around zero-width ones still count: three visible runes pass.
	r := newOpsRig(true)
	if rec := put(r, `{"rate_per_sec":5,"reason":"a\u200bbc"}`); rec.Code != 200 {
		t.Fatalf("3 visible chars must pass: %d %s", rec.Code, rec.Body)
	}
}
