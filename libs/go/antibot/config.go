package antibot

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

// devHashKey is used when ANTIBOT_HASH_KEY is unset outside production (same pattern as QUEUE_TOKEN_SECRET).
var devHashKey = []byte("eventflow-antibot-dev-key-000000")

// RuleParams are the ANTIBOT_RULES thresholds (contracts 3).
type RuleParams struct {
	R1ClientCV       float64
	R1ClientMin      int
	R1ServerCV       float64
	R1ServerMin      int
	R2Ms             int
	R4DMax           int
	R4NMax           int
	R4NShare         float64
	R4NShareMinJoins int
	R4WindowS        int
	R5FontsMin       int
	R6Max            int
	R7MinIDs         int
	R7MinEvents      int
	R7MinBuckets     int
	R7WindowS        int
}

// Config is the validated antibot configuration, read once at startup.
type Config struct {
	Mode            Mode
	DevKey          bool // ANTIBOT_HASH_KEY was unset and the dev key is used
	HashKey         []byte
	Weights         map[RuleID]float64
	Tiers           [3]float64 // CHALLENGE, SLOW_LANE, BLOCK lower bounds
	Fallback        Action     // CHALLENGE applies this: PASS or SLOW_LANE
	SlowLanePenalty int64
	Rules           RuleParams
	RedisTimeout    time.Duration
	BreakerN        int
	BreakerWindow   time.Duration
	BreakerOpen     time.Duration
	PassSample      float64
	UADeny          []string // lower case; a match is a strong R5 sign
	UAWeak          []string // lower case; a match is one weak R5W sign (Amendment 2, X-5)

	weights [len(allRules)]float64
}

// DefaultWeights (contracts 3).
func DefaultWeights() map[RuleID]float64 {
	return map[RuleID]float64{R1: 0.50, R2: 0.40, R3: 0.40, R4D: 0.60, R4N: 0.25, R5: 0.80, R5W: 0.45, R6: 0.60, R7: 0.50}
}

// DefaultRules (contracts 3).
func DefaultRules() RuleParams {
	return RuleParams{R1ClientCV: 0.10, R1ClientMin: 8, R1ServerCV: 0.05, R1ServerMin: 4, R2Ms: 120, R4DMax: 3,
		R4NMax: 30, R4NShare: 0.05, R4NShareMinJoins: 1000, R4WindowS: 300, R5FontsMin: 3, R6Max: 3, R7MinIDs: 3,
		R7MinEvents: 12, R7MinBuckets: 4, R7WindowS: 900}
}

// DefaultUADeny is the built-in User-Agent deny list (case-insensitive substrings).
var DefaultUADeny = []string{"HeadlessChrome", "PhantomJS", "python-requests", "python-httpx", "aiohttp", "curl/",
	"Wget", "Go-http-client", "okhttp", "node-fetch", "axios/", "Scrapy", "Puppeteer", "Playwright", "Selenium"}

// DefaultUAWeak is the built-in weak User-Agent list: generic HTTP libraries that legitimate first-party or test
// clients also use (Amendment 2, X-5), so a match alone never blocks. Empty until TCR-6 moves the four
// libraries out of DefaultUADeny (the frozen rule tests still expect them there).
var DefaultUAWeak = []string{}

// DefaultSlowLanePenalty is ANTIBOT_SLOW_LANE_PENALTY's default (also shuffle.lua's default).
const DefaultSlowLanePenalty int64 = 100000

// DefaultConfig returns the defaults in shadow mode with the dev key.
func DefaultConfig() Config {
	c := Config{
		Mode:            ModeShadow,
		DevKey:          true,
		HashKey:         devHashKey,
		Weights:         DefaultWeights(),
		Tiers:           [3]float64{0.30, 0.60, 0.85},
		Fallback:        ActionPass,
		SlowLanePenalty: DefaultSlowLanePenalty,
		Rules:           DefaultRules(),
		RedisTimeout:    4 * time.Millisecond,
		BreakerN:        20,
		BreakerWindow:   time.Second,
		BreakerOpen:     3 * time.Second,
		PassSample:      0.01,
	}
	for _, s := range DefaultUADeny {
		c.UADeny = append(c.UADeny, strings.ToLower(s))
	}
	for _, s := range DefaultUAWeak {
		c.UAWeak = append(c.UAWeak, strings.ToLower(s))
	}
	c.index()
	return c
}

func (c *Config) index() {
	for i, id := range allRules {
		c.weights[i] = c.Weights[id]
	}
}

// LoadConfig reads the ANTIBOT_* variables. Every fault names its variable (AC-16). ANTIBOT_MODE=off skips the
// rest: the guard is not wired.
func LoadConfig(getenv func(string) string) (Config, error) {
	c := DefaultConfig()
	switch m := Mode(strings.TrimSpace(getenv("ANTIBOT_MODE"))); m {
	case "":
	case ModeOff, ModeShadow, ModeEnforce:
		c.Mode = m
	default:
		return Config{}, fmt.Errorf("ANTIBOT_MODE: %q is not off, shadow or enforce", m)
	}
	if c.Mode == ModeOff {
		return c, nil
	}
	if err := c.loadKey(getenv("ANTIBOT_HASH_KEY"), strings.TrimSpace(getenv("APP_ENV")) == "production"); err != nil {
		return Config{}, err
	}
	steps := []struct {
		name string
		fn   func(string) error
	}{
		{"ANTIBOT_WEIGHTS", c.loadWeights},
		{"ANTIBOT_TIERS", c.loadTiers},
		{"ANTIBOT_CHALLENGE_FALLBACK", c.loadFallback},
		{"ANTIBOT_SLOW_LANE_PENALTY", c.loadPenalty},
		{"ANTIBOT_RULES", c.loadRules},
		{"ANTIBOT_REDIS_TIMEOUT_MS", c.loadTimeout},
		{"ANTIBOT_BREAKER", c.loadBreaker},
		{"ANTIBOT_PASS_SAMPLE", c.loadSample},
		{"ANTIBOT_UA_DENY", c.loadUADeny},
		{"ANTIBOT_UA_WEAK", c.loadUAWeak},
	}
	for _, s := range steps {
		v := strings.TrimSpace(getenv(s.name))
		if v == "" {
			continue
		}
		if err := s.fn(v); err != nil {
			return Config{}, fmt.Errorf("%s: %w", s.name, err)
		}
	}
	c.index()
	return c, nil
}

// SlowLanePenaltyFromEnv reads only ANTIBOT_SLOW_LANE_PENALTY (the controller needs nothing else).
func SlowLanePenaltyFromEnv(getenv func(string) string) (int64, error) {
	c := Config{SlowLanePenalty: DefaultSlowLanePenalty}
	if v := strings.TrimSpace(getenv("ANTIBOT_SLOW_LANE_PENALTY")); v != "" {
		if err := c.loadPenalty(v); err != nil {
			return 0, fmt.Errorf("ANTIBOT_SLOW_LANE_PENALTY: %w", err)
		}
	}
	return c.SlowLanePenalty, nil
}

func (c *Config) loadKey(v string, production bool) error {
	v = strings.TrimSpace(v)
	if v == "" {
		if production {
			return fmt.Errorf("ANTIBOT_HASH_KEY: required when APP_ENV=production and ANTIBOT_MODE is not off")
		}
		c.HashKey, c.DevKey = devHashKey, true
		return nil
	}
	key, err := base64.StdEncoding.DecodeString(v)
	if err != nil {
		if key, err = base64.RawStdEncoding.DecodeString(v); err != nil {
			return fmt.Errorf("ANTIBOT_HASH_KEY: not valid base64")
		}
	}
	if len(key) < 32 {
		return fmt.Errorf("ANTIBOT_HASH_KEY: %d bytes, want at least 32", len(key))
	}
	// R-5: a public dev key would let anyone recompute the stored device_fp hashes (D10). Every committed dev
	// value carries the marker "dev" (in the text or in the decoded bytes), so production refuses it.
	if production && (bytes.Equal(key, devHashKey) || strings.Contains(v, "dev") || bytes.Contains(key, []byte("dev"))) {
		return fmt.Errorf("ANTIBOT_HASH_KEY: a dev key (contains \"dev\") is not allowed when APP_ENV=production")
	}
	c.HashKey, c.DevKey = key, false
	return nil
}

// pairs splits "K=V,K=V" (empty entries ignored).
func pairs(v string) ([][2]string, error) {
	var out [][2]string
	for _, item := range strings.Split(v, ",") {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		k, val, ok := strings.Cut(item, "=")
		k, val = strings.TrimSpace(k), strings.TrimSpace(val)
		if !ok || k == "" || val == "" {
			return nil, fmt.Errorf("%q is not KEY=VALUE", item)
		}
		out = append(out, [2]string{k, val})
	}
	return out, nil
}

func parseFloat(v string) (float64, error) {
	f, err := strconv.ParseFloat(v, 64)
	if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
		return 0, fmt.Errorf("%q is not a number", v)
	}
	return f, nil
}

func (c *Config) loadWeights(v string) error {
	ps, err := pairs(v)
	if err != nil {
		return err
	}
	w := DefaultWeights()
	for _, p := range ps {
		id := RuleID(p[0])
		if _, ok := w[id]; !ok {
			return fmt.Errorf("unknown rule %q", p[0])
		}
		f, err := parseFloat(p[1])
		if err != nil {
			return fmt.Errorf("%s: %w", p[0], err)
		}
		if f < 0 || f >= 1 {
			return fmt.Errorf("%s=%s outside [0,1)", p[0], p[1])
		}
		w[id] = f
	}
	c.Weights = w
	return nil
}

func (c *Config) loadTiers(v string) error {
	parts := strings.Split(v, ",")
	if len(parts) != 3 {
		return fmt.Errorf("%q: want three cut-offs", v)
	}
	var t [3]float64
	for i, p := range parts {
		f, err := parseFloat(strings.TrimSpace(p))
		if err != nil {
			return err
		}
		if f <= 0 || f >= 1 {
			return fmt.Errorf("%v outside (0,1)", f)
		}
		if i > 0 && f <= t[i-1] {
			return fmt.Errorf("%q is not strictly increasing", v)
		}
		t[i] = f
	}
	c.Tiers = t
	return nil
}

func (c *Config) loadFallback(v string) error {
	switch v {
	case "pass":
		c.Fallback = ActionPass
	case "slow_lane":
		c.Fallback = ActionSlowLane
	default:
		return fmt.Errorf("%q is not pass or slow_lane", v)
	}
	return nil
}

func (c *Config) loadPenalty(v string) error {
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil || n < 1 || n > 1_000_000_000 {
		return fmt.Errorf("%q is not an integer in 1..1000000000", v)
	}
	c.SlowLanePenalty = n
	return nil
}

func (c *Config) loadRules(v string) error {
	ps, err := pairs(v)
	if err != nil {
		return err
	}
	r := DefaultRules()
	ints := map[string]*int{"R1_CLIENT_MIN": &r.R1ClientMin, "R1_SERVER_MIN": &r.R1ServerMin, "R2_MS": &r.R2Ms,
		"R4D_MAX": &r.R4DMax, "R4N_MAX": &r.R4NMax, "R4N_SHARE_MIN_JOINS": &r.R4NShareMinJoins,
		"R4_WINDOW_S": &r.R4WindowS, "R5_FONTS_MIN": &r.R5FontsMin, "R6_MAX": &r.R6Max, "R7_MIN_IDS": &r.R7MinIDs,
		"R7_MIN_EVENTS": &r.R7MinEvents, "R7_MIN_BUCKETS": &r.R7MinBuckets, "R7_WINDOW_S": &r.R7WindowS}
	fracs := map[string]*float64{"R1_CLIENT_CV": &r.R1ClientCV, "R1_SERVER_CV": &r.R1ServerCV, "R4N_SHARE": &r.R4NShare}
	for _, p := range ps {
		if dst, ok := ints[p[0]]; ok {
			n, err := strconv.Atoi(p[1])
			if err != nil || n <= 0 || n > 1_000_000_000 {
				return fmt.Errorf("%s=%s is not a positive integer", p[0], p[1])
			}
			*dst = n
			continue
		}
		if dst, ok := fracs[p[0]]; ok {
			f, err := parseFloat(p[1])
			if err != nil || f <= 0 || f >= 1 {
				return fmt.Errorf("%s=%s outside (0,1)", p[0], p[1])
			}
			*dst = f
			continue
		}
		return fmt.Errorf("unknown key %q", p[0])
	}
	c.Rules = r
	return nil
}

func (c *Config) loadTimeout(v string) error {
	n, err := strconv.Atoi(v)
	if err != nil || n < 1 || n > 50 {
		return fmt.Errorf("%q is not an integer in 1..50", v)
	}
	c.RedisTimeout = time.Duration(n) * time.Millisecond
	return nil
}

// loadBreaker parses "N/window,open=duration".
func (c *Config) loadBreaker(v string) error {
	head, tail, ok := strings.Cut(v, ",")
	if !ok {
		return fmt.Errorf("%q is not N/window,open=duration", v)
	}
	ns, ws, ok := strings.Cut(strings.TrimSpace(head), "/")
	if !ok {
		return fmt.Errorf("%q is not N/window,open=duration", v)
	}
	n, err := strconv.Atoi(strings.TrimSpace(ns))
	if err != nil || n < 1 || n > 10000 {
		return fmt.Errorf("calls %q not in 1..10000", ns)
	}
	window, err := breakerDuration(ws)
	if err != nil {
		return err
	}
	ok2, os, found := strings.Cut(strings.TrimSpace(tail), "=")
	if !found || strings.TrimSpace(ok2) != "open" {
		return fmt.Errorf("%q is not N/window,open=duration", v)
	}
	open, err := breakerDuration(os)
	if err != nil {
		return err
	}
	c.BreakerN, c.BreakerWindow, c.BreakerOpen = n, window, open
	return nil
}

func breakerDuration(s string) (time.Duration, error) {
	d, err := time.ParseDuration(strings.TrimSpace(s))
	if err != nil || d < 100*time.Millisecond || d > 60*time.Second {
		return 0, fmt.Errorf("duration %q not in 100ms..60s", s)
	}
	return d, nil
}

func (c *Config) loadSample(v string) error {
	f, err := parseFloat(v)
	if err != nil || f < 0 || f > 1 {
		return fmt.Errorf("%q not in [0,1]", v)
	}
	c.PassSample = f
	return nil
}

func (c *Config) loadUAWeak(v string) error {
	c.UAWeak = lowerList(v)
	return nil
}

func lowerList(v string) []string {
	var out []string
	for _, s := range strings.Split(v, ",") {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, strings.ToLower(s))
		}
	}
	return out
}

func (c *Config) loadUADeny(v string) error {
	var out []string
	for _, s := range strings.Split(v, ",") {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, strings.ToLower(s))
		}
	}
	c.UADeny = out
	return nil
}
