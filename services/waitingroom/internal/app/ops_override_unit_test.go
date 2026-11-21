package app

// Unit test cho use case OpsOverride (EVF-60): kiem dau vao (ValidRate, ValidateReason, NormalizeReason),
// thu tu kiem tra -> ap dung -> audit, audit hong/panic va doc lai sau khi ap dung.

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"math"
	"strings"
	"sync"
	"testing"
	"time"
)

type opsFakeStore struct {
	mu sync.Mutex

	configured    bool
	configuredErr error

	rate       float64
	active     bool
	rateErr    error
	rateCalls  int
	readBackFn func(call int) (float64, bool, error)

	setErr   error
	clearErr error

	calls []string
}

func (s *opsFakeStore) RoomConfigured(_ context.Context, _ string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, "configured")
	return s.configured, s.configuredErr
}

func (s *opsFakeStore) RateOverride(_ context.Context, _ string) (float64, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rateCalls++
	s.calls = append(s.calls, "read")
	if s.readBackFn != nil {
		return s.readBackFn(s.rateCalls)
	}
	return s.rate, s.active, s.rateErr
}

func (s *opsFakeStore) SetRateOverride(_ context.Context, _ string, r float64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, "set")
	if s.setErr != nil {
		return s.setErr
	}
	s.rate, s.active = r, true
	return nil
}

func (s *opsFakeStore) ClearRateOverride(_ context.Context, _ string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, "clear")
	if s.clearErr != nil {
		return s.clearErr
	}
	s.rate, s.active = 0, false
	return nil
}

type opsFakeAuditor struct {
	store   *opsFakeStore
	err     error
	panicV  any
	entries []AuditEntry
}

func (a *opsFakeAuditor) Record(_ context.Context, e AuditEntry) error {
	if a.store != nil {
		a.store.mu.Lock()
		a.store.calls = append(a.store.calls, "audit")
		a.store.mu.Unlock()
	}
	if a.panicV != nil {
		panic(a.panicV)
	}
	a.entries = append(a.entries, e)
	return a.err
}

func newOpsUnderTest(store *opsFakeStore, aud Auditor) (*OpsOverride, *bytes.Buffer) {
	buf := &bytes.Buffer{}
	o := NewOpsOverride(store, aud, slog.New(slog.NewTextHandler(buf, nil)))
	o.now = func() time.Time { return time.Date(2026, 11, 21, 10, 0, 0, 0, time.FixedZone("X", 7*3600)) }
	return o, buf
}

func opsCmd(reason string) OverrideCommand {
	return OverrideCommand{ActorID: "ops-1", EventID: "ev-1", Reason: reason, RequestID: "req-1"}
}

func TestValidRate_Bounds(t *testing.T) {
	cases := []struct {
		rate float64
		want bool
	}{
		{0, true}, {1, true}, {MaxOverrideRate, true},
		{-1, false}, {MaxOverrideRate + 1, false}, {0.3, false}, {2.5, false},
		{math.NaN(), false}, {math.Inf(1), false}, {math.Inf(-1), false},
	}
	for _, c := range cases {
		if got := ValidRate(c.rate); got != c.want {
			t.Errorf("ValidRate(%v) = %v, want %v", c.rate, got, c.want)
		}
	}
}

func TestValidateReason_AllCauses(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		want    string
		wantErr error
	}{
		{"ok", "su co checkout", "su co checkout", nil},
		{"trim edge spaces and Cf", " \t\u200bok!\u00a0 ", "ok!", nil},
		{"invalid utf8", "ab\xffcd", "ab\xffcd", ErrReasonControl},
		{"control inside", "ab\x01cd", "ab\x01cd", ErrReasonControl},
		{"NEL is control not space", "abc\u0085", "abc\u0085", ErrReasonControl},
		{"bidi inside", "ab\u202ecd", "ab\u202ecd", ErrReasonBidi},
		{"bidi at edge not trimmed", "\u2066abc", "\u2066abc", ErrReasonBidi},
		{"too short", "ab", "ab", ErrReasonLength},
		{"empty", "   ", "", ErrReasonLength},
		{"combining marks not visible", "a\u0301\u0301", "a\u0301\u0301", ErrReasonLength},
		{"hangul filler not visible", "a\u3164\u3164\u2800", "a\u3164\u3164\u2800", ErrReasonLength},
		{"zwsp inside not visible", "a\u200bb", "a\u200bb", ErrReasonLength},
		{"exactly max", strings.Repeat("x", maxReasonLen), strings.Repeat("x", maxReasonLen), nil},
		{"too long", strings.Repeat("x", maxReasonLen+1), strings.Repeat("x", maxReasonLen+1), ErrReasonLength},
		{"vietnamese NFD ok", "Ta\u0323m du\u031b\u0300ng", "Ta\u0323m du\u031b\u0300ng", nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := ValidateReason(c.in)
			if !errors.Is(err, c.wantErr) || (c.wantErr == nil && err != nil) {
				t.Fatalf("err = %v, want %v", err, c.wantErr)
			}
			if got != c.want {
				t.Fatalf("reason = %q, want %q", got, c.want)
			}
			n, ok := NormalizeReason(c.in)
			if ok != (c.wantErr == nil) || n != c.want {
				t.Fatalf("NormalizeReason = (%q,%v), want (%q,%v)", n, ok, c.want, c.wantErr == nil)
			}
		})
	}
}

func TestOverrideStatePaused(t *testing.T) {
	zero, one := 0.0, 1.0
	if !(OverrideState{Active: true, RatePerSec: &zero}).Paused() {
		t.Fatal("active rate 0 must be paused")
	}
	for _, s := range []OverrideState{{Active: true, RatePerSec: &one}, {Active: false, RatePerSec: &zero}, {Active: true}} {
		if s.Paused() {
			t.Fatalf("%+v must not be paused", s)
		}
	}
}

func TestNewOpsOverride_NilLoggerUsesDefault(t *testing.T) {
	if o := NewOpsOverride(&opsFakeStore{}, nil, nil); o.log == nil || o.now == nil {
		t.Fatal("logger/clock must be defaulted")
	}
}

func TestSet_RejectsInvalidInputBeforeTouchingStore(t *testing.T) {
	store := &opsFakeStore{configured: true}
	o, _ := newOpsUnderTest(store, &opsFakeAuditor{store: store})
	if _, err := o.Set(context.Background(), opsCmd("ly do hop le"), 0.5); !errors.Is(err, ErrInvalidOverride) {
		t.Fatalf("rate 0.5: err = %v", err)
	}
	_, err := o.Set(context.Background(), opsCmd("x"), 5)
	if !errors.Is(err, ErrInvalidOverride) || !errors.Is(err, ErrReasonLength) {
		t.Fatalf("short reason: err = %v", err)
	}
	if len(store.calls) != 0 {
		t.Fatalf("store touched: %v", store.calls)
	}
}

func TestSet_AppliesThenAudits(t *testing.T) {
	store := &opsFakeStore{configured: true, rate: 7, active: true}
	aud := &opsFakeAuditor{store: store}
	o, _ := newOpsUnderTest(store, aud)

	res, err := o.Set(context.Background(), opsCmd("  tang nhip  "), 50)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"configured", "read", "set", "read", "audit"}
	if strings.Join(store.calls, ",") != strings.Join(want, ",") {
		t.Fatalf("calls = %v, want %v", store.calls, want)
	}
	if !res.Active || res.RatePerSec == nil || *res.RatePerSec != 50 || res.Paused || res.Audit != AuditRecorded {
		t.Fatalf("res = %+v", res)
	}
	if res.AppliedAt.Location() != time.UTC {
		t.Fatalf("AppliedAt not UTC: %v", res.AppliedAt)
	}
	if len(aud.entries) != 1 {
		t.Fatalf("audit entries = %d", len(aud.entries))
	}
	e := aud.entries[0]
	if e.Action != ActionOverrideSet || e.ActorType != "human" || e.ActorID != "ops-1" || e.Subject != "event:ev-1" || e.Reason != "tang nhip" {
		t.Fatalf("entry = %+v", e)
	}
	prev, ok := e.Metadata["previous"].(previousOverride)
	if !ok || !prev.Active || prev.RatePerSec == nil || *prev.RatePerSec != 7 {
		t.Fatalf("previous = %+v", e.Metadata["previous"])
	}
	if e.Metadata["rate_per_sec"] != 50.0 || e.Metadata["request_id"] != "req-1" {
		t.Fatalf("metadata = %+v", e.Metadata)
	}
}

func TestSet_ZeroIsPause(t *testing.T) {
	store := &opsFakeStore{configured: true}
	aud := &opsFakeAuditor{}
	o, _ := newOpsUnderTest(store, aud)
	res, err := o.Set(context.Background(), opsCmd("tam dung khan cap"), 0)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Active || !res.Paused || res.RatePerSec == nil || *res.RatePerSec != 0 {
		t.Fatalf("res = %+v", res)
	}
	if aud.entries[0].Action != ActionPaused {
		t.Fatalf("action = %s", aud.entries[0].Action)
	}
	prev := aud.entries[0].Metadata["previous"].(previousOverride)
	if prev.Active || prev.RatePerSec != nil {
		t.Fatalf("previous = %+v", prev)
	}
}

func TestSet_ReadBackReportsStoredState(t *testing.T) {
	// Doc lai thay gia tri khac (mot PUT khac chen vao): phan hoi theo gia tri luu, audit theo lenh da gui.
	store := &opsFakeStore{configured: true, readBackFn: func(call int) (float64, bool, error) {
		if call == 1 {
			return 0, false, nil
		}
		return 30, true, nil
	}}
	aud := &opsFakeAuditor{}
	o, _ := newOpsUnderTest(store, aud)
	res, err := o.Set(context.Background(), opsCmd("ly do"), 10)
	if err != nil {
		t.Fatal(err)
	}
	if *res.RatePerSec != 30 || aud.entries[0].Metadata["rate_per_sec"] != 10.0 {
		t.Fatalf("res rate %v, audit rate %v", *res.RatePerSec, aud.entries[0].Metadata["rate_per_sec"])
	}

	// Doc lai thay ghi de da bi bo: active:false.
	store2 := &opsFakeStore{configured: true, readBackFn: func(call int) (float64, bool, error) { return 0, false, nil }}
	o2, _ := newOpsUnderTest(store2, &opsFakeAuditor{})
	res2, err := o2.Set(context.Background(), opsCmd("ly do"), 10)
	if err != nil {
		t.Fatal(err)
	}
	if res2.Active || res2.RatePerSec != nil || res2.Paused {
		t.Fatalf("res2 = %+v", res2)
	}
}

func TestSet_ReadBackErrorFallsBackToSentRate(t *testing.T) {
	store := &opsFakeStore{configured: true, readBackFn: func(call int) (float64, bool, error) {
		if call == 1 {
			return 0, false, nil
		}
		return 0, false, errors.New("redis down")
	}}
	o, logs := newOpsUnderTest(store, &opsFakeAuditor{})
	res, err := o.Set(context.Background(), opsCmd("ly do"), 12)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Active || *res.RatePerSec != 12 || res.Audit != AuditRecorded {
		t.Fatalf("res = %+v", res)
	}
	if !strings.Contains(logs.String(), "level=WARN") {
		t.Fatalf("missing warn log: %s", logs.String())
	}
}

func TestSet_PrecheckFailures(t *testing.T) {
	boom := errors.New("boom")
	cases := []struct {
		name    string
		store   *opsFakeStore
		aud     Auditor
		wantErr error
	}{
		{"no auditor", &opsFakeStore{configured: true}, nil, ErrAuditUnavailable},
		{"configured error", &opsFakeStore{configuredErr: boom}, &opsFakeAuditor{}, boom},
		{"not configured", &opsFakeStore{}, &opsFakeAuditor{}, ErrRoomNotConfigured},
		{"read current error", &opsFakeStore{configured: true, rateErr: boom}, &opsFakeAuditor{}, boom},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			o, _ := newOpsUnderTest(c.store, c.aud)
			if _, err := o.Set(context.Background(), opsCmd("ly do"), 5); !errors.Is(err, c.wantErr) {
				t.Fatalf("Set err = %v, want %v", err, c.wantErr)
			}
			if _, err := o.Clear(context.Background(), opsCmd("ly do")); !errors.Is(err, c.wantErr) {
				t.Fatalf("Clear err = %v, want %v", err, c.wantErr)
			}
			for _, call := range c.store.calls {
				if call == "set" || call == "clear" {
					t.Fatalf("applied despite precheck failure: %v", c.store.calls)
				}
			}
		})
	}
}

func TestSet_ApplyErrorSkipsAudit(t *testing.T) {
	boom := errors.New("hset failed")
	store := &opsFakeStore{configured: true, setErr: boom}
	aud := &opsFakeAuditor{}
	o, _ := newOpsUnderTest(store, aud)
	if _, err := o.Set(context.Background(), opsCmd("ly do"), 5); !errors.Is(err, boom) {
		t.Fatalf("err = %v", err)
	}
	if len(aud.entries) != 0 {
		t.Fatal("audited a failed apply")
	}
}

func TestSet_ApplyUsesDetachedContext(t *testing.T) {
	store := &opsFakeStore{configured: true}
	aud := &opsFakeAuditor{}
	o, _ := newOpsUnderTest(store, aud)
	ctx, cancel := context.WithCancel(context.Background())
	o.store = &cancelOnSet{opsFakeStore: store, cancel: cancel}
	res, err := o.Set(ctx, opsCmd("ly do"), 5)
	if err != nil || res.Audit != AuditRecorded {
		t.Fatalf("res=%+v err=%v", res, err)
	}
}

// cancelOnSet huy context cua request ngay sau HSET va kiem context duoc truyen vao audit van song.
type cancelOnSet struct {
	*opsFakeStore
	cancel context.CancelFunc
}

func (c *cancelOnSet) SetRateOverride(ctx context.Context, ev string, r float64) error {
	c.cancel()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return c.opsFakeStore.SetRateOverride(ctx, ev, r)
}

func TestAudit_FailureReturnsFailedAndLogsError(t *testing.T) {
	store := &opsFakeStore{configured: true}
	o, logs := newOpsUnderTest(store, &opsFakeAuditor{err: errors.New("pg down")})
	res, err := o.Set(context.Background(), opsCmd("ly do"), 5)
	if err != nil {
		t.Fatal(err)
	}
	if res.Audit != AuditFailed || !res.Active {
		t.Fatalf("res = %+v", res)
	}
	out := logs.String()
	for _, want := range []string{"level=ERROR", "actor_id=ops-1", "action=ADMIT_OVERRIDE_SET", "subject=event:ev-1", "pg down"} {
		if !strings.Contains(out, want) {
			t.Fatalf("log missing %q: %s", want, out)
		}
	}
	if !store.active {
		t.Fatal("override must stay applied when audit fails")
	}
}

func TestAudit_PanicIsRecovered(t *testing.T) {
	store := &opsFakeStore{configured: true}
	o, logs := newOpsUnderTest(store, &opsFakeAuditor{panicV: "nil driver"})
	res, err := o.Clear(context.Background(), opsCmd("ly do"))
	if err != nil {
		t.Fatal(err)
	}
	if res.Audit != AuditFailed {
		t.Fatalf("res = %+v", res)
	}
	if !strings.Contains(logs.String(), "auditor panic: nil driver") {
		t.Fatalf("log = %s", logs.String())
	}
}

func TestClear_AppliesThenAudits(t *testing.T) {
	store := &opsFakeStore{configured: true, rate: 0, active: true}
	aud := &opsFakeAuditor{store: store}
	o, _ := newOpsUnderTest(store, aud)
	res, err := o.Clear(context.Background(), opsCmd("tra quyen AIMD"))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"configured", "read", "clear", "audit"}
	if strings.Join(store.calls, ",") != strings.Join(want, ",") {
		t.Fatalf("calls = %v, want %v", store.calls, want)
	}
	if res.Active || res.RatePerSec != nil || res.Audit != AuditRecorded || res.AppliedAt.IsZero() {
		t.Fatalf("res = %+v", res)
	}
	e := aud.entries[0]
	if e.Action != ActionOverrideCleared || e.Metadata["rate_per_sec"] != nil {
		t.Fatalf("entry = %+v", e)
	}
	prev := e.Metadata["previous"].(previousOverride)
	if !prev.Active || *prev.RatePerSec != 0 {
		t.Fatalf("previous = %+v", prev)
	}
}

func TestClear_Errors(t *testing.T) {
	store := &opsFakeStore{configured: true}
	o, _ := newOpsUnderTest(store, &opsFakeAuditor{})
	if _, err := o.Clear(context.Background(), opsCmd("a\u202ebc")); !errors.Is(err, ErrInvalidOverride) || !errors.Is(err, ErrReasonBidi) {
		t.Fatalf("bidi err = %v", err)
	}
	boom := errors.New("del failed")
	store.clearErr = boom
	aud := &opsFakeAuditor{}
	o2, _ := newOpsUnderTest(store, aud)
	if _, err := o2.Clear(context.Background(), opsCmd("ly do")); !errors.Is(err, boom) {
		t.Fatalf("err = %v", err)
	}
	if len(aud.entries) != 0 {
		t.Fatal("audited a failed clear")
	}
}
