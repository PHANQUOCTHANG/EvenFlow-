package antibot

import (
	"bytes"
	"encoding/json"
	"math"
)

// InvalidReason says why a whole signal payload counts as missing ("" = valid).
type InvalidReason string

// Whole-payload reasons (the label set of antibot_signals_invalid_total).
const (
	ReasonTooLarge   InvalidReason = "too_large"
	ReasonBadJSON    InvalidReason = "bad_json"
	ReasonMissing    InvalidReason = "missing"
	ReasonBadVersion InvalidReason = "bad_version"
)

// MaxSignalBytes is the body / payload cap (contracts 1).
const MaxSignalBytes = 4096

// OptInt is an optional integer field.
type OptInt struct {
	V  int
	OK bool
}

// OptBool is an optional boolean field.
type OptBool struct {
	V  bool
	OK bool
}

// Signals is a decoded v1 payload. Every field is optional: a wrong type or an out-of-range value leaves it
// absent (D15).
type Signals struct {
	Reason InvalidReason // "" = valid

	RT, PE, SE, KE OptInt
	IV             []int
	HasIV          bool
	SQ             string // "" = absent
	SN, SB         OptInt
	FP             string // "" = absent
	WD, GL         OptBool
	FC             OptInt
	TZ             string // "" = absent
	TZO            OptInt
}

// Missing reports one of the four whole-payload failures.
func (s Signals) Missing() bool { return s.Reason != "" }

func missingSignals(r InvalidReason) Signals { return Signals{Reason: r} }

// DecodeSignals decodes the raw "signals" value of the join body. It never panics.
func DecodeSignals(raw []byte) (Signals, InvalidReason) {
	if len(raw) > MaxSignalBytes {
		return missingSignals(ReasonTooLarge), ReasonTooLarge
	}
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return missingSignals(ReasonMissing), ReasonMissing
	}
	if !json.Valid(trimmed) {
		return missingSignals(ReasonBadJSON), ReasonBadJSON
	}
	if trimmed[0] != '{' {
		return missingSignals(ReasonMissing), ReasonMissing
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(trimmed, &m); err != nil {
		return missingSignals(ReasonBadJSON), ReasonBadJSON
	}
	if v, ok := decodeInt(m["v"], 0, 1000); !ok || v != 1 {
		return missingSignals(ReasonBadVersion), ReasonBadVersion
	}
	var s Signals
	s.RT = optInt(m["rt"], 0, 600000)
	s.PE = optInt(m["pe"], 0, 10000)
	s.SE = optInt(m["se"], 0, 10000)
	s.KE = optInt(m["ke"], 0, 10000)
	s.IV, s.HasIV = decodeIntervals(m["iv"])
	s.SQ = optString(m["sq"], isSeqDigest)
	s.SN = optInt(m["sn"], 12, 32)
	s.SB = optInt(m["sb"], 4, 41)
	s.FP = optString(m["fp"], isDeviceID)
	s.WD = optBool(m["wd"])
	s.GL = optBool(m["gl"])
	s.FC = optInt(m["fc"], 0, 20)
	s.TZ = optString(m["tz"], isZoneName)
	s.TZO = optInt(m["tzo"], -840, 840)
	return s, ""
}

func isNull(raw json.RawMessage) bool {
	return len(raw) == 0 || (len(raw) == 4 && string(raw) == "null")
}

// decodeInt reads a finite JSON number, rounds it to the nearest integer and checks [lo, hi].
func decodeInt(raw json.RawMessage, lo, hi int) (int, bool) {
	if isNull(raw) {
		return 0, false
	}
	c := raw[0]
	if c != '-' && (c < '0' || c > '9') {
		return 0, false
	}
	var f float64
	if err := json.Unmarshal(raw, &f); err != nil {
		return 0, false
	}
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return 0, false
	}
	f = math.Round(f)
	if f < float64(lo) || f > float64(hi) {
		return 0, false
	}
	return int(f), true
}

func optInt(raw json.RawMessage, lo, hi int) OptInt {
	v, ok := decodeInt(raw, lo, hi)
	return OptInt{V: v, OK: ok}
}

func optBool(raw json.RawMessage) OptBool {
	switch string(raw) {
	case "true":
		return OptBool{V: true, OK: true}
	case "false":
		return OptBool{V: false, OK: true}
	}
	return OptBool{}
}

func optString(raw json.RawMessage, valid func(string) bool) string {
	if isNull(raw) || raw[0] != '"' {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return ""
	}
	if !valid(s) {
		return ""
	}
	return s
}

// decodeIntervals reads iv: <= 32 integers in 0..60000; one bad item drops the whole array.
func decodeIntervals(raw json.RawMessage) ([]int, bool) {
	if isNull(raw) || raw[0] != '[' {
		return nil, false
	}
	var items []json.RawMessage
	if err := json.Unmarshal(raw, &items); err != nil || len(items) > 32 {
		return nil, false
	}
	out := make([]int, len(items))
	for i, it := range items {
		v, ok := decodeInt(it, 0, 60000)
		if !ok {
			return nil, false
		}
		out[i] = v
	}
	return out, true
}

// ^[0-9a-f]{16}$
func isSeqDigest(s string) bool {
	if len(s) != 16 {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

// ^[A-Za-z0-9-]{8,64}$
func isDeviceID(s string) bool {
	if len(s) < 8 || len(s) > 64 {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !isAlnum(c) && c != '-' {
			return false
		}
	}
	return true
}

// ^[A-Za-z_+\-/0-9]{1,64}$
func isZoneName(s string) bool {
	if len(s) < 1 || len(s) > 64 {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !isAlnum(c) && c != '_' && c != '+' && c != '-' && c != '/' {
			return false
		}
	}
	return true
}

func isAlnum(c byte) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}
