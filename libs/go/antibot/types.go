// Package antibot is the in-process anti-bot engine of the waiting room (spec antibot-phase1, D1): a lenient
// signal decoder, the rule table R1-R7, noisy-OR scoring with tiers, and a Guard that adds Redis state under a
// hard latency budget. Wire shapes, keys and defaults: docs/workflow/antibot-phase1/contracts.md.
package antibot

import (
	"context"
	"encoding/json"
	"net/netip"
	"time"
)

// RuleID names a rule. Future rules (EV-178) start at weight 0 until a human approves them.
type RuleID string

// Registered rules (contracts 2).
const (
	R1  RuleID = "R1"
	R2  RuleID = "R2"
	R3  RuleID = "R3"
	R4D RuleID = "R4D"
	R4N RuleID = "R4N"
	R5  RuleID = "R5"
	R5W RuleID = "R5W"
	R6  RuleID = "R6"
	R7  RuleID = "R7"
)

// allRules is the rule table order (sorted, so Fired comes out sorted).
var allRules = [...]RuleID{R1, R2, R3, R4D, R4N, R5, R5W, R6, R7}

// Action is a tier or the applied effect. PASS is stored as 'ALLOW' in bot_decisions.
type Action string

// Actions.
const (
	ActionPass      Action = "PASS"
	ActionChallenge Action = "CHALLENGE"
	ActionSlowLane  Action = "SLOW_LANE"
	ActionBlock     Action = "BLOCK"
)

// Mode is ANTIBOT_MODE.
type Mode string

// Modes.
const (
	ModeOff     Mode = "off"
	ModeShadow  Mode = "shadow"
	ModeEnforce Mode = "enforce"
)

// Decision tags.
const (
	TagModeShadow = "MODE_SHADOW"
	TagDegraded   = "DEGRADED"
	TagSticky     = "STICKY"
	TagCapped     = "CAPPED"
	TagRejoin     = "REJOIN"
)

// Input is one join as seen by the engine.
type Input struct {
	EventID, IdentityID, DeviceID string
	UserAgent                     string
	ClientIP                      netip.Addr      // zero = unknown
	RawSignals                    json.RawMessage // nil = missing
	BodyErr                       InvalidReason   // set by the handler for too_large / bad_json
	Now                           time.Time
}

// State is what observe.lua returns for one join.
type State struct {
	JoinGapsMs     []int64 // R1-server (only joins made without a slot)
	FPIdentities   int     // R4D, includes caller
	NetIdentities  int     // R4N
	NetSuppressed  bool    // prefix share of event joins over R4N_SHARE
	SeqIdentities  int     // R7
	CardIdentities int     // R6, -1 = unknown (always in phase 1)
	HasSlot        bool    // R2: HEXISTS wr:{ev}:tok identity
	Sticky         bool    // ab:{ev}:blk:{identity} exists
	Known          bool    // false when degraded
}

// Decision is the engine output.
type Decision struct {
	Action    Action   // tier from the rounded score, after the D11 cap
	Applied   Action   // what the handler does (mode, fallback, D12)
	Score     float64  // 3 decimals; 1.000 for STICKY
	Fired     []RuleID // sorted
	Tags      []string // MODE_SHADOW DEGRADED STICKY CAPPED REJOIN
	FPHash    string   // 32 hex, "" if no fp
	NetPrefix netip.Prefix
}

// StateStore reads and writes the Redis state of one join in one script call.
type StateStore interface {
	Observe(ctx context.Context, in Input, fpHash string, net netip.Prefix, sq string) (State, error)
}

// Sink receives decisions for persistence. Offer must not block.
type Sink interface {
	Offer(Input, Decision)
}
