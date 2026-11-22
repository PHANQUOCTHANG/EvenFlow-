package postgresadapter

import (
	"net/netip"
	"testing"

	"github.com/eventflow/eventflow/libs/go/antibot"
)

// M53 / contracts 5: the tier is stored, with PASS as 'ALLOW' (the column's CHECK has no 'PASS').
func TestFix2_M53_DBActionMapping(t *testing.T) {
	for in, want := range map[antibot.Action]string{
		antibot.ActionPass: "ALLOW", antibot.ActionChallenge: "CHALLENGE",
		antibot.ActionSlowLane: "SLOW_LANE", antibot.ActionBlock: "BLOCK", "": "ALLOW",
	} {
		if got := dbAction(in); got != want {
			t.Errorf("dbAction(%q) = %q, want %q", in, got, want)
		}
	}
	w := NewDecisionWriter(nil, antibot.NewMetrics(nil))
	r := w.toRow(antibot.Input{EventID: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", IdentityID: "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"},
		antibot.Decision{Action: "PASS", Applied: "PASS", Score: 0.25, Fired: []antibot.RuleID{"R4N"}, Tags: []string{"MODE_SHADOW"},
			NetPrefix: netip.MustParsePrefix("203.0.113.0/24")})
	if r.action != "ALLOW" || r.ip != "203.0.113.0/24" || r.fp != nil || len(r.fired) != 2 || r.fired[1] != "MODE_SHADOW" {
		t.Fatalf("row %+v", r)
	}
}
