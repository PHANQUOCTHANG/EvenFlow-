package antibot_test

import (
	"encoding/json"
	"fmt"
	"math"
	"net/netip"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/eventflow/eventflow/libs/go/antibot"
)

const (
	chromeUA  = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"
	iphoneUA  = "Mozilla/5.0 (iPhone; CPU iPhone OS 18_1 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.1 Mobile/15E148 Safari/604.1"
	androidUA = "Mozilla/5.0 (Linux; Android 14; SM-A546E) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Mobile Safari/537.36"
	rawFP     = "device-abc-12345"
	rawIP     = "198.51.100.23"
)

// fixedNow is a winter date (no DST in Europe) so time-zone offsets in the tests are unambiguous.
var fixedNow = time.Date(2026, 12, 1, 12, 0, 0, 0, time.UTC)

// cfgWith loads a config in enforce mode with a known hash key, plus extra env pairs ("K", "V", ...).
func cfgWith(t testing.TB, kv ...string) antibot.Config {
	t.Helper()
	m := map[string]string{"ANTIBOT_MODE": "enforce", "ANTIBOT_HASH_KEY": goodKey}
	for i := 0; i+1 < len(kv); i += 2 {
		m[kv[i]] = kv[i+1]
	}
	cfg, err := antibot.LoadConfig(env(m))
	if err != nil {
		t.Fatalf("LoadConfig(%v): %v", m, err)
	}
	return cfg
}

// payload returns validSignals with fields set (value nil = delete the field).
func payload(t testing.TB, set map[string]any) []byte {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(validSignals), &m); err != nil {
		t.Fatal(err)
	}
	for k, v := range set {
		if v == nil {
			delete(m, k)
			continue
		}
		m[k] = v
	}
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// neutral is a warm, known state where no stateful rule fires.
func neutral() antibot.State {
	return antibot.State{FPIdentities: 1, NetIdentities: 1, SeqIdentities: 1, CardIdentities: -1, Known: true}
}

func input(raw []byte) antibot.Input {
	return antibot.Input{
		EventID: "ev-1", IdentityID: "id-1", DeviceID: "dev-1",
		UserAgent: chromeUA, ClientIP: netip.MustParseAddr(rawIP),
		RawSignals: raw, Now: fixedNow,
	}
}

// eval decodes raw (as Guard.Decide does) and runs the pure engine.
func eval(t testing.TB, cfg antibot.Config, in antibot.Input, st antibot.State) antibot.Decision {
	t.Helper()
	sig, _ := antibot.DecodeSignals(in.RawSignals)
	return antibot.Evaluate(cfg, in, sig, st)
}

func hasRule(d antibot.Decision, id string) bool { return slices.Contains(d.Fired, antibot.RuleID(id)) }
func hasTag(d antibot.Decision, tag string) bool { return slices.Contains(d.Tags, tag) }

func ids(d antibot.Decision) string {
	s := make([]string, len(d.Fired))
	for i, r := range d.Fired {
		s[i] = string(r)
	}
	return strings.Join(s, ",")
}

func sprintAll(v any) string { return fmt.Sprintf("%+v", v) }

func containsStr(s, sub string) bool { return strings.Contains(s, sub) }

func near(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

// metric returns the value of a counter / gauge, or the sample count of a histogram, with the given labels.
func metric(t testing.TB, reg *prometheus.Registry, name string, labels map[string]string) float64 {
	t.Helper()
	mfs, err := reg.Gather()
	if err != nil {
		t.Fatal(err)
	}
	var total float64
	for _, mf := range mfs {
		if mf.GetName() != name {
			continue
		}
	next:
		for _, m := range mf.GetMetric() {
			for k, v := range labels {
				found := false
				for _, lp := range m.GetLabel() {
					if lp.GetName() == k && lp.GetValue() == v {
						found = true
					}
				}
				if !found {
					continue next
				}
			}
			switch {
			case m.Counter != nil:
				total += m.GetCounter().GetValue()
			case m.Gauge != nil:
				total += m.GetGauge().GetValue()
			case m.Histogram != nil:
				total += float64(m.GetHistogram().GetSampleCount())
			}
		}
	}
	return total
}

// repeatIv returns n intervals alternating a, b.
func repeatIv(n int, a, b int) []int {
	out := make([]int, n)
	for i := range out {
		if i%2 == 0 {
			out[i] = a
		} else {
			out[i] = b
		}
	}
	return out
}
