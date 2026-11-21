package httpadapter_test

// ops-warroom fix round 2: R2-1/X2-1/X2-5 (reason rules), X2-2/R2-3 (audit the commanded action), X2-4 (strict keys).

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"testing"

	httpadapter "github.com/eventflow/eventflow/services/waitingroom/internal/adapter/http"
	"github.com/eventflow/eventflow/services/waitingroom/internal/app"
)

// R2-1/X2-1/X2-5: invisible-only reasons (Cf, combining marks alone, fillers, braille blank), bidi controls and
// control characters (incl. U+0085) are 400 with a detail naming the cause; nothing applied or audited.
func TestOpsRound2Fix_X21_InvisibleBidiOrControlReasonIs400(t *testing.T) {
	cases := []struct{ name, reason, want string }{
		{"RLO x3", `\u202e\u202e\u202e`, "bidi"},
		{"isolates", `\u2066\u2067\u2068\u2069`, "bidi"},
		{"LRM", `abc\u200e`, "bidi"},
		{"RLM", `\u200fabc`, "bidi"},
		{"ALM", `ab\u061cc`, "bidi"},
		{"bidi spoof", `resume \u202eesuap\u202c ok`, "bidi"},
		{"word joiner x3", `\u2060\u2060\u2060`, "3..200"},
		{"soft hyphen x3", `\u00ad\u00ad\u00ad`, "3..200"},
		{"combining acute x3", `\u0301\u0301\u0301`, "3..200"},
		{"enclosing mark x3", `\u20dd\u20dd\u20dd`, "3..200"},
		{"hangul fillers", `\u3164\u115f\u1160\uffa0`, "3..200"},
		{"braille blank x3", `\u2800\u2800\u2800`, "3..200"},
		{"variation selector x3", `\ufe0f\ufe0f\ufe0f`, "3..200"},
		{"tag chars x3", `\udb40\udc20\udb40\udc20\udb40\udc20`, "3..200"},
		{"mongolian VS x3", `\u180e\u180e\u180e`, "3..200"},
		{"invisible operators", `\u2061\u2062\u2063\u2064`, "3..200"},
		{"two letters + fillers", `ab\u3164\u2800`, "3..200"},
		{"NEL edges", `\u0085abc\u0085`, "dieu khien"},
		{"NEL inside", `ab\u0085c`, "dieu khien"},
		{"interior tab", `ab\tcd`, "dieu khien"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := newOpsRig(true)
			rec := put(r, `{"rate_per_sec":0,"reason":"`+c.reason+`"}`)
			if rec.Code != 400 || ptype(rec) != "/problems/invalid-override" {
				t.Fatalf("PUT %q: %d %s", c.reason, rec.Code, rec.Body)
			}
			if d, _ := decode(t, rec)["detail"].(string); !strings.Contains(d, c.want) {
				t.Fatalf("detail %q must name the cause (%q)", d, c.want)
			}
			rec = opsCall(r.mux, http.MethodDelete, ovPath(opsEvent), "ops", `{"reason":"`+c.reason+`"}`)
			if rec.Code != 400 {
				t.Fatalf("DELETE %q: %d %s", c.reason, rec.Code, rec.Body)
			}
			if r.store.writes() != 0 || len(r.aud.entries) != 0 {
				t.Fatalf("nothing applied/audited: writes=%d audits=%d", r.store.writes(), len(r.aud.entries))
			}
		})
	}
}

// R2-1: Vietnamese in NFC and NFD (combining marks) stays valid; marks are kept, non-bidi Cf at the edges trimmed.
func TestOpsRound2Fix_X21_VietnameseNFCAndNFDAccepted(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"Kh\u1ea9n c\u1ea5p", "Kh\u1ea9n c\u1ea5p"},                             // NFC
		{"Kha\u0302\u0309n ca\u0302\u0301p", "Kha\u0302\u0309n ca\u0302\u0301p"}, // NFD
		{"Kha\u0302\u0309n", "Kha\u0302\u0309n"},                                 // NFD, 4 visible + 2 marks
		{"\u2060 abc \u00ad", "abc"},
		{"a\u200db\u200cc", "a\u200db\u200cc"},
	} {
		got, err := app.ValidateReason(c.in)
		if err != nil || got != c.want {
			t.Fatalf("ValidateReason(%+q) = %+q, %v; want %+q", c.in, got, err, c.want)
		}
	}
	r := newOpsRig(true)
	if rec := put(r, `{"rate_per_sec":5,"reason":"Kha\u0302\u0309n ca\u0302\u0301p"}`); rec.Code != 200 {
		t.Fatalf("NFD reason must pass: %d %s", rec.Code, rec.Body)
	}
	if len(r.aud.entries) != 1 || r.aud.entries[0].Reason != "Kha\u0302\u0309n ca\u0302\u0301p" {
		t.Fatalf("audit reason must keep the combining marks: %+v", r.aud.entries)
	}
	if _, err := app.ValidateReason("ab\u0301\u0301"); !errors.Is(err, app.ErrReasonLength) {
		t.Fatalf("2 base letters + marks must fail with ErrReasonLength, got %v", err)
	}
}

// otherPutStore: another Ops' PUT lands between our HSET and our read-back.
type otherPutStore struct {
	fakeOverrideStore
	other float64
}

func (s *otherPutStore) SetRateOverride(ctx context.Context, ev string, rate float64) error {
	_ = s.fakeOverrideStore.SetRateOverride(ctx, ev, rate)
	return s.fakeOverrideStore.SetRateOverride(ctx, ev, s.other)
}

// X2-2: the audit row records the commanded action and rate; the response reports the stored value (X-3).
func TestOpsRound2Fix_X22_AuditRecordsCommandNotReadBack(t *testing.T) {
	for _, c := range []struct {
		body        string
		mine, other float64
		action      string
	}{
		{`{"rate_per_sec":500,"reason":"my command"}`, 500, 0, app.ActionOverrideSet},
		{`{"rate_per_sec":0,"reason":"my command"}`, 0, 500, app.ActionPaused},
	} {
		st := &otherPutStore{fakeOverrideStore: fakeOverrideStore{configured: true}, other: c.other}
		aud := &fakeAuditor{}
		log := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
		mux := http.NewServeMux()
		httpadapter.NewOpsHandler(&fakeState{}, app.NewOpsOverride(st, aud, log), log).Routes(mux)
		rec := opsCall(mux, http.MethodPut, ovPath(opsEvent), "ops", c.body)
		if rec.Code != 200 {
			t.Fatalf("status %d %s", rec.Code, rec.Body)
		}
		m := decode(t, rec)
		ov, _ := m["override"].(map[string]any)
		if ov == nil || ov["rate_per_sec"] != c.other || m["paused"] != (c.other == 0) {
			t.Fatalf("response must report the stored value %v: %v", c.other, m)
		}
		if len(aud.entries) != 1 {
			t.Fatalf("want one audit row, got %d", len(aud.entries))
		}
		e := aud.entries[0]
		if e.Action != c.action || e.Metadata["rate_per_sec"] != c.mine {
			t.Fatalf("mine=%v: audit must record the command, got action=%s meta=%v", c.mine, e.Action, e.Metadata)
		}
	}
}

// readFailStore: the read-back after the apply fails (the precheck read succeeds).
type readFailStore struct {
	fakeOverrideStore
	n int
}

func (s *readFailStore) RateOverride(ctx context.Context, ev string) (float64, bool, error) {
	s.n++
	if s.n >= 2 {
		return 0, false, errors.New("redis down")
	}
	return s.fakeOverrideStore.RateOverride(ctx, ev)
}

// R2-3: when the read-back fails the response falls back to the commanded value and a WARN is logged.
func TestOpsRound2Fix_R23_ReadBackFailureFallsBackToCommandWithWarn(t *testing.T) {
	st := &readFailStore{fakeOverrideStore: fakeOverrideStore{configured: true}}
	aud := &fakeAuditor{}
	logs := &bytes.Buffer{}
	log := slog.New(slog.NewTextHandler(logs, nil))
	mux := http.NewServeMux()
	httpadapter.NewOpsHandler(&fakeState{}, app.NewOpsOverride(st, aud, log), log).Routes(mux)
	rec := opsCall(mux, http.MethodPut, ovPath(opsEvent), "ops", `{"rate_per_sec":7,"reason":"abc"}`)
	if rec.Code != 200 {
		t.Fatalf("status %d %s", rec.Code, rec.Body)
	}
	m := decode(t, rec)
	ov, _ := m["override"].(map[string]any)
	if ov == nil || ov["active"] != true || ov["rate_per_sec"] != float64(7) || m["paused"] != false || m["audit"] != "recorded" {
		t.Fatalf("response must fall back to the commanded value: %v", m)
	}
	if !strings.Contains(logs.String(), "level=WARN") {
		t.Fatalf("a WARN line is required, logs: %s", logs)
	}
	if len(aud.entries) != 1 || aud.entries[0].Metadata["rate_per_sec"] != float64(7) {
		t.Fatalf("audit must record the command: %+v", aud.entries)
	}
}

// X2-4: duplicate keys and keys that differ in case are 400; nothing applied.
func TestOpsRound2Fix_X24_DuplicateOrCaseMismatchedKeysAre400(t *testing.T) {
	puts := []string{
		`{"rate_per_sec":5,"rate_per_sec":0,"reason":"abc"}`,
		`{"rate_per_sec":0,"rate_per_sec":5,"reason":"abc"}`,
		`{"RATE_PER_SEC":0,"REASON":"abc"}`,
		`{"Rate_per_sec":0,"reason":"abc"}`,
		`{"rate_per_sec":5,"reason":"abcdef","reason":"   xyz  "}`,
		`[{"rate_per_sec":5,"reason":"abc"}]`,
		`null`,
	}
	for _, body := range puts {
		r := newOpsRig(true)
		if rec := put(r, body); rec.Code != 400 || r.store.writes() != 0 {
			t.Fatalf("PUT %s: %d writes=%d", body, rec.Code, r.store.writes())
		}
	}
	for _, body := range []string{`{"reason":"abc","reason":"xyz"}`, `{"Reason":"abc"}`} {
		r := newOpsRig(true)
		rec := opsCall(r.mux, http.MethodDelete, ovPath(opsEvent), "ops", body)
		if rec.Code != 400 || r.store.writes() != 0 {
			t.Fatalf("DELETE %s: %d writes=%d", body, rec.Code, r.store.writes())
		}
	}
	r := newOpsRig(true)
	if rec := put(r, ` { "reason" : "abc" , "rate_per_sec" : 5 } `); rec.Code != 200 {
		t.Fatalf("exact keys in any order must pass: %d %s", rec.Code, rec.Body)
	}
}
