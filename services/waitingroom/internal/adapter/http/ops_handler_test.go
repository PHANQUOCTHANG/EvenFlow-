package httpadapter_test

// ops-warroom AC-1..AC-6, AC-8..AC-13 (waitingroom ops API), black box over HTTP with hand-written fakes.
//
// ASSUMED INTERFACES (names taken from plan.md WP1; shapes chosen by the test designer, implementer must match):
//   package app:
//     type OpsStateReader interface { QueueOpsState(ctx, eventID string, now time.Time) (app.OpsState, error) }
//     type OpsState struct {
//       Lobby, Queued, AdmittedActive int64; AdmitRatePerSec float64
//       Override app.OverrideState                     // {Active bool; RatePerSec *float64}
//       SoldOut, StockZero bool
//       Controller app.ControllerState                 // {LeaderPresent bool; Epoch int64}
//       Checkout app.CheckoutState                     // {Signal string; Pods int; P99Ms, ErrorRate, DBPoolUsage *float64}
//     }
//     var ErrRoomNotConfigured error                    // reader/store return it when meta.sale_start_ms is absent
//     type OverrideStore interface {
//       RoomConfigured(ctx, eventID string) (bool, error)
//       RateOverride(ctx, eventID string) (rate float64, active bool, err error)
//       SetRateOverride(ctx, eventID string, ratePerSec float64) error
//       ClearRateOverride(ctx, eventID string) error
//     }
//     type AuditEntry struct { ActorType, ActorID, Action, Subject, Reason string; Metadata map[string]any }
//     type Auditor interface { Record(ctx context.Context, e app.AuditEntry) error }
//     func NewOpsOverride(store OverrideStore, auditor Auditor, log *slog.Logger) *OpsOverride  (nil auditor => AC-12)
//   package httpadapter:
//     func NewOpsHandler(state app.OpsStateReader, ov *app.OpsOverride, log *slog.Logger) *OpsHandler
//     func (h *OpsHandler) Routes(mux *http.ServeMux)
//   Identity is read from X-Identity-Id / X-Identity-Role (authx.PrincipalFromHeaders).
//   The pgx Auditor and the Redis OpsStateReader are covered by integration tests in the implementer's WP (docker).

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	httpadapter "github.com/eventflow/eventflow/services/waitingroom/internal/adapter/http"
	"github.com/eventflow/eventflow/services/waitingroom/internal/app"
)

const (
	opsEvent = "11111111-1111-4111-8111-111111111111"
	opsActor = "99999999-9999-4999-8999-999999999999"
)

var errBoom = errors.New("boom")

type fakeState struct {
	st  app.OpsState
	err error
}

func (f *fakeState) QueueOpsState(context.Context, string, time.Time) (app.OpsState, error) {
	return f.st, f.err
}

type fakeOverrideStore struct {
	mu         sync.Mutex
	configured bool
	active     bool
	rate       float64
	err        error
	sets       []float64
	clears     int
}

func (f *fakeOverrideStore) RoomConfigured(context.Context, string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.configured, f.err
}

func (f *fakeOverrideStore) RateOverride(context.Context, string) (float64, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.rate, f.active, f.err
}

func (f *fakeOverrideStore) SetRateOverride(_ context.Context, _ string, r float64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return f.err
	}
	f.sets = append(f.sets, r)
	f.active, f.rate = true, r
	return nil
}

func (f *fakeOverrideStore) ClearRateOverride(context.Context, string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return f.err
	}
	f.clears++
	f.active, f.rate = false, 0
	return nil
}

func (f *fakeOverrideStore) writes() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.sets) + f.clears
}

type fakeAuditor struct {
	mu      sync.Mutex
	entries []app.AuditEntry
	err     error
	block   bool // block until ctx is done (simulates hung Postgres)
	sawDL   bool
}

func (f *fakeAuditor) Record(ctx context.Context, e app.AuditEntry) error {
	f.mu.Lock()
	_, f.sawDL = ctx.Deadline()
	f.mu.Unlock()
	if f.block {
		<-ctx.Done()
		return ctx.Err()
	}
	if f.err != nil {
		return f.err
	}
	f.mu.Lock()
	f.entries = append(f.entries, e)
	f.mu.Unlock()
	return nil
}

type opsRig struct {
	mux   *http.ServeMux
	state *fakeState
	store *fakeOverrideStore
	aud   *fakeAuditor
	logs  *bytes.Buffer
}

func newOpsRig(withAuditor bool) *opsRig {
	r := &opsRig{
		state: &fakeState{},
		store: &fakeOverrideStore{configured: true},
		aud:   &fakeAuditor{},
		logs:  &bytes.Buffer{},
	}
	log := slog.New(slog.NewTextHandler(r.logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	var aud app.Auditor
	if withAuditor {
		aud = r.aud
	}
	r.mux = http.NewServeMux()
	httpadapter.NewOpsHandler(r.state, app.NewOpsOverride(r.store, aud, log), log).Routes(r.mux)
	return r
}

func opsCall(mux http.Handler, method, path, role, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if role != "" {
		req.Header.Set("X-Identity-Id", opsActor)
		req.Header.Set("X-Identity-Role", role)
	}
	req.Header.Set("X-Request-Id", "req-123")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func ovPath(ev string) string { return "/v1/ops/events/" + ev + "/admit-override" }
func qPath(ev string) string  { return "/v1/ops/events/" + ev + "/queue" }

func pf(v float64) *float64 { return &v }

func decode(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
		t.Fatalf("body is not JSON: %v (%s)", err, rec.Body.String())
	}
	return m
}

func ptype(rec *httptest.ResponseRecorder) string {
	var p struct {
		Type string `json:"type"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &p)
	return p.Type
}

// ---------- AC-1 .. AC-5 : GET queue ----------

func TestOpsQueue_AC1_ShapeAndNoStore(t *testing.T) {
	r := newOpsRig(true)
	r.state.st = app.OpsState{
		Lobby: 0, Queued: 4210, AdmittedActive: 350, AdmitRatePerSec: 50,
		Override:   app.OverrideState{Active: true, RatePerSec: pf(0)},
		SoldOut:    false,
		Controller: app.ControllerState{LeaderPresent: true, Epoch: 7},
		Checkout:   app.CheckoutState{Signal: "ok", Pods: 3, P99Ms: pf(420), ErrorRate: pf(0.004), DBPoolUsage: pf(0.61)},
	}
	rec := opsCall(r.mux, http.MethodGet, qPath(opsEvent), "ops", "")
	if rec.Code != 200 {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	if cc := rec.Header().Get("Cache-Control"); cc != "no-store" {
		t.Fatalf("Cache-Control = %q", cc)
	}
	m := decode(t, rec)
	if m["event_id"] != opsEvent || m["queued"] != float64(4210) || m["admitted_active"] != float64(350) ||
		m["lobby"] != float64(0) || m["admit_rate_per_sec"] != float64(50) {
		t.Fatalf("counters wrong: %v", m)
	}
	if m["paused"] != true || m["sold_out"] != false || m["stock_zero"] != false {
		t.Fatalf("flags wrong: %v", m)
	}
	ov := m["override"].(map[string]any)
	if ov["active"] != true || ov["rate_per_sec"] != float64(0) {
		t.Fatalf("override: %v", ov)
	}
	ctl := m["controller"].(map[string]any)
	if ctl["leader_present"] != true || ctl["epoch"] != float64(7) {
		t.Fatalf("controller: %v", ctl)
	}
	co := m["checkout"].(map[string]any)
	if co["signal"] != "ok" || co["pods"] != float64(3) || co["p99_ms"] != float64(420) ||
		co["error_rate"] != 0.004 || co["db_pool_usage"] != 0.61 {
		t.Fatalf("checkout: %v", co)
	}
	if _, err := time.Parse(time.RFC3339Nano, m["generated_at"].(string)); err != nil {
		t.Fatalf("generated_at: %v", err)
	}
}

func TestOpsQueue_AC1_NoOverrideAndPausedOnlyWhenRateZero(t *testing.T) {
	r := newOpsRig(true)
	r.state.st = app.OpsState{Override: app.OverrideState{Active: false}}
	m := decode(t, opsCall(r.mux, http.MethodGet, qPath(opsEvent), "ops", ""))
	ov := m["override"].(map[string]any)
	if ov["active"] != false || ov["rate_per_sec"] != nil || m["paused"] != false {
		t.Fatalf("no override: %v", m)
	}
	r.state.st = app.OpsState{Override: app.OverrideState{Active: true, RatePerSec: pf(120)}}
	m = decode(t, opsCall(r.mux, http.MethodGet, qPath(opsEvent), "ops", ""))
	if m["paused"] != false {
		t.Fatalf("override 120 must not be paused: %v", m)
	}
}

func TestOpsQueue_AC3_NoSignalAndErrorStill200WithNulls(t *testing.T) {
	for _, sig := range []string{"none", "error"} {
		r := newOpsRig(true)
		r.state.st = app.OpsState{Checkout: app.CheckoutState{Signal: sig}}
		rec := opsCall(r.mux, http.MethodGet, qPath(opsEvent), "ops", "")
		if rec.Code != 200 {
			t.Fatalf("%s: status %d", sig, rec.Code)
		}
		co := decode(t, rec)["checkout"].(map[string]any)
		if co["signal"] != sig || co["p99_ms"] != nil || co["error_rate"] != nil || co["db_pool_usage"] != nil {
			t.Fatalf("%s: %v", sig, co)
		}
	}
}

func TestOpsQueue_AC4_Errors(t *testing.T) {
	r := newOpsRig(true)
	if rec := opsCall(r.mux, http.MethodGet, qPath("not-a-uuid"), "ops", ""); rec.Code != 400 {
		t.Fatalf("non-uuid: %d", rec.Code)
	}
	r.state.err = app.ErrRoomNotConfigured
	rec := opsCall(r.mux, http.MethodGet, qPath(opsEvent), "ops", "")
	if rec.Code != 404 || ptype(rec) != "/problems/not-found" {
		t.Fatalf("unconfigured: %d %s", rec.Code, ptype(rec))
	}
	r.state.err = errBoom
	if rec := opsCall(r.mux, http.MethodGet, qPath(opsEvent), "ops", ""); rec.Code != 503 {
		t.Fatalf("redis error: %d", rec.Code)
	}
}

func TestOpsQueue_AC5_RoleAndPrincipal(t *testing.T) {
	r := newOpsRig(true)
	for _, role := range []string{"buyer", "organizer", "moderator"} {
		rec := opsCall(r.mux, http.MethodGet, qPath(opsEvent), role, "")
		if rec.Code != 403 || ptype(rec) != "/problems/forbidden" {
			t.Fatalf("%s: %d %s", role, rec.Code, ptype(rec))
		}
	}
	if rec := opsCall(r.mux, http.MethodGet, qPath(opsEvent), "", ""); rec.Code != 401 {
		t.Fatalf("no principal: %d", rec.Code)
	}
}

// ---------- AC-6 / AC-10: PUT ----------

func put(r *opsRig, body string) *httptest.ResponseRecorder {
	return opsCall(r.mux, http.MethodPut, ovPath(opsEvent), "ops", body)
}

func TestOpsPut_AC6_AC10_AppliesAndAudits(t *testing.T) {
	r := newOpsRig(true)
	rec := put(r, `{"rate_per_sec":120,"reason":"  checkout p99 cao  "}`)
	if rec.Code != 200 {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("Cache-Control no-store missing")
	}
	if len(r.store.sets) != 1 || r.store.sets[0] != 120 {
		t.Fatalf("store sets = %v", r.store.sets)
	}
	m := decode(t, rec)
	ov := m["override"].(map[string]any)
	if m["event_id"] != opsEvent || ov["active"] != true || ov["rate_per_sec"] != float64(120) ||
		m["paused"] != false || m["audit"] != "recorded" {
		t.Fatalf("response: %v", m)
	}
	if _, err := time.Parse(time.RFC3339, m["applied_at"].(string)); err != nil {
		t.Fatalf("applied_at: %v", err)
	}
	if len(r.aud.entries) != 1 {
		t.Fatalf("audit rows = %d, want exactly 1", len(r.aud.entries))
	}
	e := r.aud.entries[0]
	if e.ActorType != "human" || e.ActorID != opsActor || e.Action != "ADMIT_OVERRIDE_SET" ||
		e.Subject != "event:"+opsEvent || e.Reason != "checkout p99 cao" {
		t.Fatalf("audit entry: %+v", e)
	}
	if e.Metadata["rate_per_sec"] != float64(120) && e.Metadata["rate_per_sec"] != 120 {
		t.Fatalf("metadata.rate_per_sec: %v", e.Metadata)
	}
	if e.Metadata["request_id"] != "req-123" {
		t.Fatalf("metadata.request_id: %v", e.Metadata)
	}
	prev, ok := e.Metadata["previous"]
	if !ok {
		t.Fatalf("metadata.previous missing: %v", e.Metadata)
	}
	pb, _ := json.Marshal(prev)
	if !strings.Contains(string(pb), `"active":false`) {
		t.Fatalf("previous = %s", pb)
	}
}

func TestOpsPut_AC10_PreviousReflectsExistingOverride(t *testing.T) {
	r := newOpsRig(true)
	r.store.active, r.store.rate = true, 30
	put(r, `{"rate_per_sec":60,"reason":"abc"}`)
	if len(r.aud.entries) != 1 {
		t.Fatalf("audit rows = %d", len(r.aud.entries))
	}
	pb, _ := json.Marshal(r.aud.entries[0].Metadata["previous"])
	if !strings.Contains(string(pb), `"active":true`) || !strings.Contains(string(pb), "30") {
		t.Fatalf("previous = %s", pb)
	}
}

func TestOpsPut_AC10_ZeroIsPausedAction(t *testing.T) {
	r := newOpsRig(true)
	rec := put(r, `{"rate_per_sec":0,"reason":"su co thanh toan"}`)
	if rec.Code != 200 {
		t.Fatalf("status %d", rec.Code)
	}
	m := decode(t, rec)
	if m["paused"] != true {
		t.Fatalf("paused: %v", m)
	}
	if len(r.aud.entries) != 1 || r.aud.entries[0].Action != "ADMIT_PAUSED" {
		t.Fatalf("audit = %+v", r.aud.entries)
	}
	if len(r.store.sets) != 1 || r.store.sets[0] != 0 {
		t.Fatalf("sets = %v", r.store.sets)
	}
}

func TestOpsPut_AC6_BoundariesAccepted(t *testing.T) {
	for _, rate := range []string{"0", "10000", "0.5", "1e2"} {
		r := newOpsRig(true)
		if rec := put(r, `{"rate_per_sec":`+rate+`,"reason":"abc"}`); rec.Code != 200 {
			t.Errorf("rate %s: %d", rate, rec.Code)
		}
	}
}

// ---------- AC-9: validation ----------

func TestOpsWrite_AC9_InvalidInputIs400AndNothingApplied(t *testing.T) {
	long := strings.Repeat("x", 201)
	cases := map[string]string{
		"missing rate":      `{"reason":"abc"}`,
		"string rate":       `{"rate_per_sec":"12","reason":"abc"}`,
		"null rate":         `{"rate_per_sec":null,"reason":"abc"}`,
		"negative":          `{"rate_per_sec":-1,"reason":"abc"}`,
		"too big":           `{"rate_per_sec":10001,"reason":"abc"}`,
		"huge":              `{"rate_per_sec":1e308,"reason":"abc"}`,
		"missing reason":    `{"rate_per_sec":5}`,
		"short reason":      `{"rate_per_sec":5,"reason":"ab"}`,
		"blank reason":      `{"rate_per_sec":5,"reason":"     "}`,
		"long reason":       `{"rate_per_sec":5,"reason":"` + long + `"}`,
		"non-string reason": `{"rate_per_sec":5,"reason":123}`,
		"unknown field":     `{"rate_per_sec":5,"reason":"abc","extra":1}`,
		"bad json":          `{"rate_per_sec":`,
		"empty body":        ``,
		"oversized body":    `{"rate_per_sec":5,"reason":"abc","pad":"` + strings.Repeat("a", 5000) + `"}`,
	}
	for name, body := range cases {
		r := newOpsRig(true)
		rec := put(r, body)
		if rec.Code != 400 || ptype(rec) != "/problems/invalid-override" {
			t.Errorf("PUT %s: %d %s", name, rec.Code, ptype(rec))
		}
		if r.store.writes() != 0 || len(r.aud.entries) != 0 {
			t.Errorf("PUT %s: state changed or audited", name)
		}
	}
	for _, body := range []string{`{}`, `{"reason":"ab"}`, `{"reason":"abc","extra":1}`, ``, `{"reason":"` + long + `"}`} {
		r := newOpsRig(true)
		rec := opsCall(r.mux, http.MethodDelete, ovPath(opsEvent), "ops", body)
		if rec.Code != 400 || ptype(rec) != "/problems/invalid-override" {
			t.Errorf("DELETE %q: %d %s", body, rec.Code, ptype(rec))
		}
		if r.store.writes() != 0 || len(r.aud.entries) != 0 {
			t.Errorf("DELETE %q: state changed or audited", body)
		}
	}
}

func TestOpsWrite_AC9_ReasonLengthBoundariesAccepted(t *testing.T) {
	for _, reason := range []string{"abc", strings.Repeat("x", 200)} {
		r := newOpsRig(true)
		if rec := put(r, `{"rate_per_sec":5,"reason":"`+reason+`"}`); rec.Code != 200 {
			t.Errorf("reason len %d: %d", len(reason), rec.Code)
		}
	}
}

// ---------- AC-8: DELETE ----------

func TestOpsDelete_AC8_AC10_ClearsIdempotentlyAndAudits(t *testing.T) {
	r := newOpsRig(true)
	r.store.active, r.store.rate = true, 0
	for i := 0; i < 2; i++ {
		rec := opsCall(r.mux, http.MethodDelete, ovPath(opsEvent), "ops", `{"reason":"su co da xu ly"}`)
		if rec.Code != 200 {
			t.Fatalf("call %d: %d %s", i, rec.Code, rec.Body)
		}
		m := decode(t, rec)
		ov := m["override"].(map[string]any)
		if ov["active"] != false || ov["rate_per_sec"] != nil || m["paused"] != false || m["audit"] != "recorded" {
			t.Fatalf("response: %v", m)
		}
	}
	if r.store.clears != 2 || r.store.active {
		t.Fatalf("clears=%d active=%v", r.store.clears, r.store.active)
	}
	if len(r.aud.entries) != 2 || r.aud.entries[0].Action != "ADMIT_OVERRIDE_CLEARED" ||
		r.aud.entries[0].Reason != "su co da xu ly" || r.aud.entries[0].ActorID != opsActor {
		t.Fatalf("audit: %+v", r.aud.entries)
	}
}

// ---------- AC-4 (write) / AC-5 / AC-13 ----------

func TestOpsWrite_AC4_RoomNotConfigured404_AndNonUUID400_AndRedisError503(t *testing.T) {
	r := newOpsRig(true)
	r.store.configured = false
	for _, rec := range []*httptest.ResponseRecorder{
		put(r, `{"rate_per_sec":5,"reason":"abc"}`),
		opsCall(r.mux, http.MethodDelete, ovPath(opsEvent), "ops", `{"reason":"abc"}`),
	} {
		if rec.Code != 404 || ptype(rec) != "/problems/not-found" {
			t.Fatalf("unconfigured: %d %s", rec.Code, ptype(rec))
		}
	}
	if r.store.writes() != 0 || len(r.aud.entries) != 0 {
		t.Fatal("unconfigured room was changed/audited")
	}
	if rec := opsCall(r.mux, http.MethodPut, ovPath("zzz"), "ops", `{"rate_per_sec":5,"reason":"abc"}`); rec.Code != 400 {
		t.Fatalf("non-uuid: %d", rec.Code)
	}
	r2 := newOpsRig(true)
	r2.store.err = errBoom
	rec := put(r2, `{"rate_per_sec":5,"reason":"abc"}`)
	if rec.Code != 503 {
		t.Fatalf("redis error: %d", rec.Code)
	}
	if len(r2.aud.entries) != 0 {
		t.Fatal("nothing applied => nothing audited")
	}
}

func TestOpsWrite_AC13_NonOpsForbiddenNothingAppliedOrAudited(t *testing.T) {
	for _, role := range []string{"buyer", "organizer", "moderator"} {
		for _, c := range []struct{ method, body string }{
			{http.MethodPut, `{"rate_per_sec":0,"reason":"abc"}`},
			{http.MethodDelete, `{"reason":"abc"}`},
		} {
			r := newOpsRig(true)
			rec := opsCall(r.mux, c.method, ovPath(opsEvent), role, c.body)
			if rec.Code != 403 || ptype(rec) != "/problems/forbidden" {
				t.Errorf("%s %s: %d %s", c.method, role, rec.Code, ptype(rec))
			}
			if r.store.writes() != 0 || len(r.aud.entries) != 0 {
				t.Errorf("%s %s: applied or audited", c.method, role)
			}
		}
	}
	r := newOpsRig(true)
	if rec := opsCall(r.mux, http.MethodPut, ovPath(opsEvent), "", `{"rate_per_sec":0,"reason":"abc"}`); rec.Code != 401 {
		t.Fatalf("no principal: %d", rec.Code)
	}
}

// ---------- AC-11: audit failure keeps the change ----------

func TestOpsWrite_AC11_AuditErrorKeepsOverrideAndLogsError(t *testing.T) {
	r := newOpsRig(true)
	r.aud.err = errBoom
	rec := put(r, `{"rate_per_sec":0,"reason":"khan cap"}`)
	if rec.Code != 200 {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	m := decode(t, rec)
	if m["audit"] != "failed" || m["paused"] != true {
		t.Fatalf("response: %v", m)
	}
	if !r.store.active || r.store.rate != 0 {
		t.Fatal("override must stay applied when the audit fails")
	}
	var errLines []string
	for _, l := range strings.Split(r.logs.String(), "\n") {
		if strings.Contains(l, "level=ERROR") {
			errLines = append(errLines, l)
		}
	}
	if len(errLines) != 1 {
		t.Fatalf("want exactly 1 ERROR line, got %d:\n%s", len(errLines), r.logs.String())
	}
	for _, want := range []string{opsActor, "ADMIT_PAUSED", "event:" + opsEvent, "khan cap", "req-123"} {
		if !strings.Contains(errLines[0], want) {
			t.Errorf("ERROR line lacks %q: %s", want, errLines[0])
		}
	}
}

func TestOpsWrite_AC11_AuditHangIsBoundedBy2s(t *testing.T) {
	r := newOpsRig(true)
	r.aud.block = true
	start := time.Now()
	rec := opsCall(r.mux, http.MethodDelete, ovPath(opsEvent), "ops", `{"reason":"abc"}`)
	el := time.Since(start)
	if rec.Code != 200 || decode(t, rec)["audit"] != "failed" {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	if !r.aud.sawDL {
		t.Fatal("auditor context had no deadline")
	}
	if el > 3500*time.Millisecond || el < 1500*time.Millisecond {
		t.Fatalf("hung audit took %v, want ~2s", el)
	}
}

// ---------- AC-12: no audit store ----------

func TestOpsWrite_AC12_NoAuditStoreFailsClosedButReadWorks(t *testing.T) {
	r := newOpsRig(false)
	for _, rec := range []*httptest.ResponseRecorder{
		put(r, `{"rate_per_sec":0,"reason":"abc"}`),
		opsCall(r.mux, http.MethodDelete, ovPath(opsEvent), "ops", `{"reason":"abc"}`),
	} {
		if rec.Code != 503 || ptype(rec) != "/problems/audit-unavailable" {
			t.Fatalf("%d %s", rec.Code, ptype(rec))
		}
	}
	if r.store.writes() != 0 {
		t.Fatal("Redis changed without an audit store")
	}
	r.state.st = app.OpsState{Queued: 3}
	if rec := opsCall(r.mux, http.MethodGet, qPath(opsEvent), "ops", ""); rec.Code != 200 {
		t.Fatalf("read endpoint must keep working: %d", rec.Code)
	}
}

// Two Ops writing at once: last write wins in Redis, both are audited.
func TestOpsWrite_ConcurrentWritersAreAllAudited(t *testing.T) {
	r := newOpsRig(true)
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			put(r, fmt.Sprintf(`{"rate_per_sec":%d,"reason":"writer %d"}`, i, i))
		}(i)
	}
	wg.Wait()
	r.aud.mu.Lock()
	n := len(r.aud.entries)
	r.aud.mu.Unlock()
	if n != 5 {
		t.Fatalf("audit rows = %d, want 5", n)
	}
}
