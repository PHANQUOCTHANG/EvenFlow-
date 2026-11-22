package antibot_test

import (
	"encoding/base64"
	"strings"
	"testing"

	"github.com/eventflow/eventflow/libs/go/antibot"
)

// goodKey is base64 of 32 bytes (contracts 3: >= 32 bytes).
var goodKey = base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef"))

func env(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

// AC-16: with nothing set, defaults load (mode shadow) and the dev key is flagged.
func TestLoadConfig_AC16_DefaultsInDev(t *testing.T) {
	cfg, err := antibot.LoadConfig(env(nil))
	if err != nil {
		t.Fatalf("defaults rejected: %v", err)
	}
	if cfg.Mode != antibot.Mode("shadow") {
		t.Fatalf("default mode = %q, want shadow", cfg.Mode)
	}
	if !cfg.DevKey {
		t.Fatal("missing ANTIBOT_HASH_KEY in dev must use the dev key and set DevKey")
	}
}

// AC-16: production with a valid key loads and does not use the dev key.
func TestLoadConfig_AC16_ProductionWithKey(t *testing.T) {
	cfg, err := antibot.LoadConfig(env(map[string]string{"APP_ENV": "production", "ANTIBOT_HASH_KEY": goodKey}))
	if err != nil {
		t.Fatalf("valid production config rejected: %v", err)
	}
	if cfg.DevKey {
		t.Fatal("DevKey set although ANTIBOT_HASH_KEY was given")
	}
}

// AC-16: every invalid value fails and the error names the variable.
func TestLoadConfig_AC16_InvalidValueNamesVariable(t *testing.T) {
	short := base64.StdEncoding.EncodeToString([]byte("only-16-bytes!!!"))
	cases := []struct {
		name, variable, value string
		extra                 map[string]string
	}{
		{"mode", "ANTIBOT_MODE", "block-everyone", nil},
		{"weight not number", "ANTIBOT_WEIGHTS", "R2=abc", nil},
		{"weight one", "ANTIBOT_WEIGHTS", "R2=1.0", nil},
		{"weight negative", "ANTIBOT_WEIGHTS", "R2=-0.1", nil},
		{"weight unknown rule", "ANTIBOT_WEIGHTS", "R99=0.1", nil},
		{"weight malformed", "ANTIBOT_WEIGHTS", "R2", nil},
		{"weight legacy R4 (split into R4D/R4N)", "ANTIBOT_WEIGHTS", "R4=0.6", nil},
		{"breaker garbage", "ANTIBOT_BREAKER", "lots", nil},
		{"breaker zero calls", "ANTIBOT_BREAKER", "0/1s,open=3s", nil},
		{"breaker too many calls", "ANTIBOT_BREAKER", "10001/1s,open=3s", nil},
		{"breaker window too short", "ANTIBOT_BREAKER", "20/50ms,open=3s", nil},
		{"breaker open too long", "ANTIBOT_BREAKER", "20/1s,open=61s", nil},
		{"rules legacy R4_FP_MAX", "ANTIBOT_RULES", "R4_FP_MAX=3", nil},
		{"rules share out of range", "ANTIBOT_RULES", "R4N_SHARE=1.5", nil},
		{"rules CV zero", "ANTIBOT_RULES", "R1_CLIENT_CV=0", nil},
		{"rules count fractional", "ANTIBOT_RULES", "R4D_MAX=2.5", nil},
		{"tiers not increasing", "ANTIBOT_TIERS", "0.60,0.30,0.85", nil},
		{"tiers equal", "ANTIBOT_TIERS", "0.30,0.30,0.85", nil},
		{"tiers zero", "ANTIBOT_TIERS", "0,0.60,0.85", nil},
		{"tiers one", "ANTIBOT_TIERS", "0.30,0.60,1", nil},
		{"tiers count", "ANTIBOT_TIERS", "0.30,0.60", nil},
		{"tiers text", "ANTIBOT_TIERS", "low,mid,high", nil},
		{"fallback", "ANTIBOT_CHALLENGE_FALLBACK", "block", nil},
		{"penalty zero", "ANTIBOT_SLOW_LANE_PENALTY", "0", nil},
		{"penalty huge", "ANTIBOT_SLOW_LANE_PENALTY", "1000000001", nil},
		{"penalty text", "ANTIBOT_SLOW_LANE_PENALTY", "many", nil},
		{"rules unknown key", "ANTIBOT_RULES", "NOPE=1", nil},
		{"rules not number", "ANTIBOT_RULES", "R2_MS=fast", nil},
		{"rules negative", "ANTIBOT_RULES", "R2_MS=-5", nil},
		{"rules zero", "ANTIBOT_RULES", "R4_WINDOW_S=0", nil},
		{"timeout zero", "ANTIBOT_REDIS_TIMEOUT_MS", "0", nil},
		{"timeout over", "ANTIBOT_REDIS_TIMEOUT_MS", "51", nil},
		{"sample over", "ANTIBOT_PASS_SAMPLE", "1.5", nil},
		{"sample negative", "ANTIBOT_PASS_SAMPLE", "-0.01", nil},
		{"key missing in production", "ANTIBOT_HASH_KEY", "", map[string]string{"APP_ENV": "production"}},
		{"key short in production", "ANTIBOT_HASH_KEY", short, map[string]string{"APP_ENV": "production"}},
		{"key not base64 in production", "ANTIBOT_HASH_KEY", "%%%not-base64%%%", map[string]string{"APP_ENV": "production"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := map[string]string{"ANTIBOT_MODE": "enforce", c.variable: c.value}
			if c.variable == "ANTIBOT_MODE" {
				m["ANTIBOT_MODE"] = c.value
			}
			for k, v := range c.extra {
				m[k] = v
			}
			_, err := antibot.LoadConfig(env(m))
			if err == nil {
				t.Fatalf("%s=%q accepted", c.variable, c.value)
			}
			if !strings.Contains(err.Error(), c.variable) {
				t.Fatalf("error %q does not name %s", err, c.variable)
			}
		})
	}
}

// AC-16: the same faults are also fatal in shadow (the default), not only in enforce.
func TestLoadConfig_AC16_ShadowAlsoValidates(t *testing.T) {
	_, err := antibot.LoadConfig(env(map[string]string{"ANTIBOT_TIERS": "0.9,0.5,0.1"}))
	if err == nil || !strings.Contains(err.Error(), "ANTIBOT_TIERS") {
		t.Fatalf("shadow with bad tiers: err = %v", err)
	}
}

// AC-16: ANTIBOT_MODE=off does not validate the rest (the guard is not wired at all).
func TestLoadConfig_AC16_OffSkipsValidation(t *testing.T) {
	cfg, err := antibot.LoadConfig(env(map[string]string{
		"ANTIBOT_MODE": "off", "APP_ENV": "production", "ANTIBOT_WEIGHTS": "R2=abc", "ANTIBOT_TIERS": "x",
	}))
	if err != nil {
		t.Fatalf("off mode failed on unused settings: %v", err)
	}
	if cfg.Mode != antibot.Mode("off") {
		t.Fatalf("mode = %q", cfg.Mode)
	}
}

// AC-16: the three documented modes load as themselves.
func TestLoadConfig_AC16_ModesAccepted(t *testing.T) {
	for _, m := range []string{"off", "shadow", "enforce"} {
		cfg, err := antibot.LoadConfig(env(map[string]string{"ANTIBOT_MODE": m}))
		if err != nil || cfg.Mode != antibot.Mode(m) {
			t.Errorf("mode %q: cfg.Mode=%q err=%v", m, cfg.Mode, err)
		}
	}
}

// Contracts 3 (R2): partial ANTIBOT_WEIGHTS / ANTIBOT_RULES lists merge with the defaults; valid values load.
func TestLoadConfig_AC16_PartialListsMergeAndValidValuesLoad(t *testing.T) {
	for k, v := range map[string]string{
		"ANTIBOT_WEIGHTS":            "R2=0.30,R4N=0",
		"ANTIBOT_RULES":              "R2_MS=150,R4N_SHARE=0.10",
		"ANTIBOT_BREAKER":            "5/500ms,open=2s",
		"ANTIBOT_TIERS":              "0.2,0.5,0.9",
		"ANTIBOT_CHALLENGE_FALLBACK": "slow_lane",
		"ANTIBOT_SLOW_LANE_PENALTY":  "1000000000",
		"ANTIBOT_REDIS_TIMEOUT_MS":   "50",
		"ANTIBOT_PASS_SAMPLE":        "1",
		"ANTIBOT_UA_DENY":            "MyBot,Other",
	} {
		if _, err := antibot.LoadConfig(env(map[string]string{"ANTIBOT_MODE": "enforce", k: v})); err != nil {
			t.Errorf("%s=%q rejected: %v", k, v, err)
		}
	}
}
