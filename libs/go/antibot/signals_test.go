package antibot_test

import (
	"strings"
	"testing"

	"github.com/eventflow/eventflow/libs/go/antibot"
)

// validSignals is a well-formed v1 payload (contracts 1, R2); every field inside its range and human-like:
// no rule fires on it with a browser UA and a neutral state.
const validSignals = `{"v":1,"rt":3500,"pe":25,"se":6,"ke":2,"iv":[310,120,840,95,2200,460,180,705,330],` +
	`"sq":"0123456789abcdef","sn":12,"sb":6,"fp":"device-abc-12345","wd":false,"gl":true,"fc":12,` +
	`"tz":"Asia/Ho_Chi_Minh","tzo":420}`

// padTo returns a syntactically valid v1 payload of exactly n bytes (unknown field "x" carries the padding).
func padTo(t *testing.T, n int) []byte {
	t.Helper()
	head := `{"v":1,"pe":3,"x":"`
	tail := `"}`
	fill := n - len(head) - len(tail)
	if fill < 0 {
		t.Fatalf("padTo(%d): too small", n)
	}
	return []byte(head + strings.Repeat("a", fill) + tail)
}

// AC-3: a valid payload decodes with no invalid reason and is not "missing".
func TestDecodeSignals_AC3_ValidPayload(t *testing.T) {
	s, reason := antibot.DecodeSignals([]byte(validSignals))
	if reason != "" {
		t.Fatalf("reason = %q, want empty", reason)
	}
	if s.Missing() {
		t.Fatal("valid payload reported as missing")
	}
}

// AC-3: unknown fields are ignored, the payload stays valid.
func TestDecodeSignals_AC3_UnknownFieldsIgnored(t *testing.T) {
	raw := strings.TrimSuffix(validSignals, "}") + `,"zzz":{"nested":[1,2,3]},"extra":"x"}`
	s, reason := antibot.DecodeSignals([]byte(raw))
	if reason != "" || s.Missing() {
		t.Fatalf("unknown field invalidated payload: reason=%q missing=%v", reason, s.Missing())
	}
}

// AC-3 (R2, D15): whole-payload failures map to a fixed reason and count as missing.
func TestDecodeSignals_AC3_InvalidPayloadReasons(t *testing.T) {
	cases := []struct {
		name string
		raw  []byte
		want antibot.InvalidReason
	}{
		{"nil (signals absent)", nil, "missing"},
		{"empty", []byte(""), "missing"},
		{"json null", []byte(`null`), "missing"},
		{"string", []byte(`"abc"`), "missing"},
		{"array", []byte(`[1,2,3]`), "missing"},
		{"number", []byte(`42`), "missing"},
		{"bool", []byte(`true`), "missing"},
		{"empty object (demo.sh / k6)", []byte(`{}`), "bad_version"},
		{"v absent", []byte(`{"rt":3500,"pe":25}`), "bad_version"},
		{"v 2", []byte(strings.Replace(validSignals, `"v":1`, `"v":2`, 1)), "bad_version"},
		{"v 0", []byte(strings.Replace(validSignals, `"v":1`, `"v":0`, 1)), "bad_version"},
		{"v string", []byte(strings.Replace(validSignals, `"v":1`, `"v":"1"`, 1)), "bad_version"},
		{"truncated json", []byte(`{"v":1,"rt":`), "bad_json"},
		{"garbage", []byte(`not json at all`), "bad_json"},
		{"over 4 KiB", padTo(t, 4097), "too_large"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s, reason := antibot.DecodeSignals(c.raw)
			if reason != c.want {
				t.Fatalf("reason = %q, want %q", reason, c.want)
			}
			if !s.Missing() {
				t.Fatal("invalid payload must count as missing")
			}
		})
	}
}

// AC-3 (R2): {"v":1} with nothing else is valid but empty, not missing.
func TestDecodeSignals_AC3_VersionOnlyIsValidButEmpty(t *testing.T) {
	s, reason := antibot.DecodeSignals([]byte(`{"v":1}`))
	if reason != "" || s.Missing() {
		t.Fatalf(`{"v":1}: reason=%q missing=%v, want valid`, reason, s.Missing())
	}
}

// AC-3: the 4 KiB cap is inclusive: exactly 4096 bytes is still decoded.
func TestDecodeSignals_AC3_SizeCapBoundary(t *testing.T) {
	raw := padTo(t, 4096)
	if _, reason := antibot.DecodeSignals(raw); reason == "too_large" {
		t.Fatal("4096-byte payload rejected as too_large")
	}
	if _, reason := antibot.DecodeSignals(padTo(t, 64*1024)); reason != "too_large" {
		t.Fatalf("64 KiB payload: reason = %q, want too_large", reason)
	}
}

// AC-3: a field outside its range is treated as absent; the rest of the payload stays valid.
func TestDecodeSignals_AC3_OutOfRangeFieldDoesNotInvalidatePayload(t *testing.T) {
	bad := map[string][2]string{
		"rt over":   {`"rt":3500`, `"rt":600001`},
		"rt neg":    {`"rt":3500`, `"rt":-1`},
		"pe over":   {`"pe":25`, `"pe":10001`},
		"fc over":   {`"fc":12`, `"fc":21`},
		"tzo over":  {`"tzo":420`, `"tzo":841`},
		"sq upper":  {`"sq":"0123456789abcdef"`, `"sq":"0123456789ABCDEF"`},
		"sq short":  {`"sq":"0123456789abcdef"`, `"sq":"0123"`},
		"fp short":  {`"fp":"device-abc-12345"`, `"fp":"abc"`},
		"fp chars":  {`"fp":"device-abc-12345"`, `"fp":"device abc;drop"`},
		"tz chars":  {`"tz":"Asia/Ho_Chi_Minh"`, `"tz":"Asia/Ho Chi Minh<script>"`},
		"sn under":  {`"sn":12`, `"sn":11`},
		"sn over":   {`"sn":12`, `"sn":33`},
		"sb under":  {`"sb":6`, `"sb":3`},
		"sb over":   {`"sb":6`, `"sb":42`},
		"iv item":   {`"iv":[310,`, `"iv":[60001,`},
		"iv length": {`"iv":[310,120,840,95,2200,460,180,705,330]`, `"iv":[` + strings.TrimSuffix(strings.Repeat("100,", 33), ",") + `]`},
	}
	for name, r := range bad {
		t.Run(name, func(t *testing.T) {
			raw := strings.Replace(validSignals, r[0], r[1], 1)
			if raw == validSignals {
				t.Fatalf("fixture replacement %q did not apply", r[0])
			}
			s, reason := antibot.DecodeSignals([]byte(raw))
			if reason != "" || s.Missing() {
				t.Fatalf("one out-of-range field invalidated the payload: reason=%q missing=%v", reason, s.Missing())
			}
		})
	}
}

// AC-3 (R2, D15): a wrong-typed field makes only that field absent; the payload stays valid.
func TestDecodeSignals_AC3_WrongTypeDropsOnlyThatField(t *testing.T) {
	bad := map[string][2]string{
		"rt string": {`"rt":3500`, `"rt":"fast"`},
		"pe bool":   {`"pe":25`, `"pe":true`},
		"iv object": {`"iv":[310,120,840,95,2200,460,180,705,330]`, `"iv":{"a":1}`},
		"iv strs":   {`"iv":[310,`, `"iv":["310",`},
		"wd string": {`"wd":false`, `"wd":"false"`},
		"gl number": {`"gl":true`, `"gl":1`},
		"tz number": {`"tz":"Asia/Ho_Chi_Minh"`, `"tz":7`},
		"fp array":  {`"fp":"device-abc-12345"`, `"fp":["device-abc-12345"]`},
		"sq null":   {`"sq":"0123456789abcdef"`, `"sq":null`},
	}
	for name, r := range bad {
		t.Run(name, func(t *testing.T) {
			raw := strings.Replace(validSignals, r[0], r[1], 1)
			if raw == validSignals {
				t.Fatalf("fixture replacement %q did not apply", r[0])
			}
			s, reason := antibot.DecodeSignals([]byte(raw))
			if reason != "" || s.Missing() {
				t.Fatalf("wrong-typed field invalidated the payload: reason=%q missing=%v", reason, s.Missing())
			}
		})
	}
}

// AC-3 (R2, D15): a wrong-typed field is absent, so a rule fed only by it is "not evaluated".
func TestDecodeSignals_AC3_WrongTypeFieldIsAbsentForRules(t *testing.T) {
	cfg := cfgWith(t)
	// wd as a string must not count as the strong sign "wd true".
	d := eval(t, cfg, input(payload(t, map[string]any{"wd": "true"})), neutral())
	if hasRule(d, "R5") {
		t.Fatalf(`"wd":"true" fired R5 (fired %s)`, ids(d))
	}
	// rt as a string is absent: R2 not evaluated.
	d = eval(t, cfg, input(payload(t, map[string]any{"rt": "50"})), neutral())
	if hasRule(d, "R2") {
		t.Fatalf(`"rt":"50" fired R2 (fired %s)`, ids(d))
	}
}

// AC-3 (R2, D15): finite numbers are rounded to the nearest integer before the range check.
func TestDecodeSignals_AC3_NumbersRounded(t *testing.T) {
	cfg := cfgWith(t)
	cases := []struct {
		rt   any
		want bool // R2 fires (rt < 120 after rounding)
	}{
		{119.4, true},  // -> 119
		{119.6, false}, // -> 120
		{1.19e2, true}, // exponent form, 119
		{-0.4, true},   // -> 0, in range
	}
	for _, c := range cases {
		d := eval(t, cfg, input(payload(t, map[string]any{"rt": c.rt})), neutral())
		if hasRule(d, "R2") != c.want {
			t.Errorf("rt=%v: R2 fired=%v, want %v (fired %s)", c.rt, hasRule(d, "R2"), c.want, ids(d))
		}
	}
	if _, reason := antibot.DecodeSignals(payload(t, map[string]any{"rt": 600000.4})); reason != "" {
		t.Fatalf("rt 600000.4: reason %q", reason)
	}
}

// AC-4: the decoder never panics; any reported reason implies missing; reasons stay in the label set.
func FuzzDecodeSignals(f *testing.F) {
	for _, seed := range []string{validSignals, `{}`, `{"v":1}`, `{"v":1,"iv":[1,2,3]}`, `{"v":"1"}`,
		`{"v":1,"rt":1e309}`, `{"v":1,"rt":-1e308}`, `{"v":1,"iv":[-1]}`, `{"v":1,"sq":null}`, `[`, "\x00\xff",
		`{"v":1,"fp":"` + strings.Repeat("a", 5000) + `"}`, `{"v":1,"iv":[1.5,2.5,null]}`, `{"v":1,"v":2}`} {
		f.Add([]byte(seed))
	}
	f.Fuzz(func(t *testing.T, raw []byte) {
		s, reason := antibot.DecodeSignals(raw)
		if reason != "" && !s.Missing() {
			t.Fatalf("reason %q but not missing", reason)
		}
		if reason == "" && s.Missing() {
			t.Fatal("missing without a reason")
		}
		switch reason {
		case "", "too_large", "bad_json", "bad_version", "missing":
		default:
			t.Fatalf("unknown reason %q (metric label cardinality)", reason)
		}
		if len(raw) > 4096 && reason != "too_large" {
			t.Fatalf("%d bytes accepted, want too_large", len(raw))
		}
	})
}
