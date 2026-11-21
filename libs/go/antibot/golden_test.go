package antibot_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eventflow/eventflow/libs/go/antibot"
)

// AC-3 (R2): the shared golden snapshots (also produced / checked by apps/web/src/lib/antibot-signals.test.ts)
// decode as valid on the Go side, and none of these human snapshots fires a rule.
// Files named mobile_* are evaluated with a mobile UA.
func TestGolden_AC3_WebSnapshotsDecode(t *testing.T) {
	files, err := filepath.Glob(filepath.Join("testdata", "signals", "*.json"))
	if err != nil || len(files) < 3 {
		t.Fatalf("golden fixtures: %v (found %d)", err, len(files))
	}
	cfg := cfgWith(t)
	for _, f := range files {
		t.Run(filepath.Base(f), func(t *testing.T) {
			raw, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			sig, reason := antibot.DecodeSignals(raw)
			if reason != "" || sig.Missing() {
				t.Fatalf("reason %q missing %v", reason, sig.Missing())
			}
			in := input(raw)
			if strings.HasPrefix(filepath.Base(f), "mobile_") {
				in.UserAgent = androidUA
			}
			if d := antibot.Evaluate(cfg, in, sig, neutral()); len(d.Fired) != 0 {
				t.Fatalf("human golden snapshot fired %s", ids(d))
			}
		})
	}
}
