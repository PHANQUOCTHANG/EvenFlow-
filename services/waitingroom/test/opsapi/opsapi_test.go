//go:build integration

// Package opsapi kiem Ops API cua phong cho (ops-warroom AC-1, AC-2, AC-4, AC-6, AC-7) voi Redis that:
// pipeline doc trang thai, lenh ghi de qua HTTP va admit controller that phan ung voi ghi de.
package opsapi

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/eventflow/eventflow/libs/go/loadsignal"
	"github.com/eventflow/eventflow/libs/go/testx"
	httpadapter "github.com/eventflow/eventflow/services/waitingroom/internal/adapter/http"
	redisadapter "github.com/eventflow/eventflow/services/waitingroom/internal/adapter/redis"
	"github.com/eventflow/eventflow/services/waitingroom/internal/app"
)

const actor = "99999999-9999-4999-8999-999999999999"

func newUUID(t *testing.T) string {
	t.Helper()
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	h := hex.EncodeToString(b)
	return h[0:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:]
}

type memAuditor struct {
	mu      sync.Mutex
	entries []app.AuditEntry
}

func (a *memAuditor) Record(_ context.Context, e app.AuditEntry) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.entries = append(a.entries, e)
	return nil
}

type rig struct {
	ctx context.Context
	rdb *redis.Client
	q   *redisadapter.Queue
	mux *http.ServeMux
	aud *memAuditor
	ev  string
}

func setup(t *testing.T) *rig {
	t.Helper()
	ctx := context.Background()
	rdb := redis.NewClient(&redis.Options{Addr: testx.Redis(t)})
	t.Cleanup(func() { _ = rdb.Close() })
	q, err := redisadapter.NewQueue(ctx, rdb, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	aud := &memAuditor{}
	checkout := redisadapter.NewLoadSignalCheckout(loadsignal.NewReader(rdb, "loadsignal:test-"+newUUID(t), 3*time.Second))
	mux := http.NewServeMux()
	httpadapter.NewOpsHandler(redisadapter.NewOpsReader(rdb, checkout), app.NewOpsOverride(q, aud, log), log).Routes(mux)
	return &rig{ctx: ctx, rdb: rdb, q: q, mux: mux, aud: aud, ev: newUUID(t)}
}

func (r *rig) key(suffix string) string { return "wr:{" + r.ev + "}:" + suffix }

func (r *rig) call(t *testing.T, method, path, body string) (int, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("X-Identity-Id", actor)
	req.Header.Set("X-Identity-Role", "ops")
	rec := httptest.NewRecorder()
	r.mux.ServeHTTP(rec, req)
	var m map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &m)
	return rec.Code, m
}

func (r *rig) getQueue(t *testing.T) (int, map[string]any) {
	return r.call(t, http.MethodGet, "/v1/ops/events/"+r.ev+"/queue", "")
}

func (r *rig) put(t *testing.T, rate int) (int, map[string]any) {
	return r.call(t, http.MethodPut, "/v1/ops/events/"+r.ev+"/admit-override",
		fmt.Sprintf(`{"rate_per_sec":%d,"reason":"kiem thu tich hop"}`, rate))
}

func (r *rig) del(t *testing.T) (int, map[string]any) {
	return r.call(t, http.MethodDelete, "/v1/ops/events/"+r.ev+"/admit-override", `{"reason":"kiem thu tich hop"}`)
}

// AC-1 + AC-2 (mutation 6): mot pipeline doc dung tung key; admitted da het han khong duoc dem.
func TestTichHop_OpsQueue_AC1_AC2_SnapshotCountsOnlyActiveAdmitted(t *testing.T) {
	r := setup(t)
	now := time.Now().UnixMilli()
	r.rdb.SAdd(r.ctx, r.key("lobby"), "l1", "l2")
	r.rdb.ZAdd(r.ctx, r.key("queue"), redis.Z{Score: 1, Member: "q1"}, redis.Z{Score: 2, Member: "q2"}, redis.Z{Score: 3, Member: "q3"})
	r.rdb.ZAdd(r.ctx, r.key("admitted"),
		redis.Z{Score: float64(now + 600_000), Member: "a1"},
		redis.Z{Score: float64(now + 600_000), Member: "a2"},
		redis.Z{Score: float64(now - 1_000), Member: "x1"},
		redis.Z{Score: float64(now - 5_000), Member: "x2"},
		redis.Z{Score: float64(now - 60_000), Member: "x3"},
	)
	r.rdb.HSet(r.ctx, r.key("meta"), "sale_start_ms", "1", "admit_rate", "50000",
		"override", "1", "override_rate", "0", "sold_out", "1")
	r.rdb.Set(r.ctx, r.key("leader"), "ctl-1", time.Minute)
	r.rdb.Set(r.ctx, r.key("leader_epoch"), 7, 0)

	code, m := r.getQueue(t)
	if code != http.StatusOK {
		t.Fatalf("status %d: %v", code, m)
	}
	if m["lobby"] != float64(2) || m["queued"] != float64(3) || m["admitted_active"] != float64(2) {
		t.Fatalf("counters: lobby=%v queued=%v admitted_active=%v (expired must not count)", m["lobby"], m["queued"], m["admitted_active"])
	}
	if m["admit_rate_per_sec"] != float64(50) || m["paused"] != true || m["sold_out"] != true || m["stock_zero"] != false {
		t.Fatalf("rate/flags: %v", m)
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
	if co["signal"] != "none" || co["p99_ms"] != nil {
		t.Fatalf("checkout without any pod must be none: %v", co)
	}
}

// AC-1 gia tri mac dinh: thieu admit_rate / epoch / leader -> 0, false; khong ghi de -> rate null.
func TestTichHop_OpsQueue_AC1_DefaultsWhenKeysAbsent(t *testing.T) {
	r := setup(t)
	r.rdb.HSet(r.ctx, r.key("meta"), "sale_start_ms", "1")
	code, m := r.getQueue(t)
	if code != http.StatusOK {
		t.Fatalf("status %d: %v", code, m)
	}
	ov := m["override"].(map[string]any)
	ctl := m["controller"].(map[string]any)
	if m["admit_rate_per_sec"] != float64(0) || ov["active"] != false || ov["rate_per_sec"] != nil ||
		ctl["leader_present"] != false || ctl["epoch"] != float64(0) || m["paused"] != false {
		t.Fatalf("defaults: %v", m)
	}
}

// AC-4: phong chua cau hinh (thieu sale_start_ms) -> 404 cho doc va ghi, Redis khong doi.
func TestTichHop_OpsAPI_AC4_UnconfiguredRoomIs404(t *testing.T) {
	r := setup(t)
	if code, m := r.getQueue(t); code != http.StatusNotFound || m["type"] != "/problems/not-found" {
		t.Fatalf("GET: %d %v", code, m)
	}
	if code, _ := r.put(t, 5); code != http.StatusNotFound {
		t.Fatalf("PUT: %d", code)
	}
	if n := r.rdb.Exists(r.ctx, r.key("meta")).Val(); n != 0 {
		t.Fatal("PUT on an unconfigured room wrote meta")
	}
}

// AC-6 / AC-8 qua Redis that: PUT ghi override="1", override_rate=rate*1000; DELETE xoa ca hai, lap lai van 200.
func TestTichHop_OpsOverride_AC6_AC8_StoredInMeta(t *testing.T) {
	r := setup(t)
	r.rdb.HSet(r.ctx, r.key("meta"), "sale_start_ms", "1")
	code, m := r.put(t, 120)
	if code != http.StatusOK || m["audit"] != "recorded" {
		t.Fatalf("PUT: %d %v", code, m)
	}
	vals := r.rdb.HMGet(r.ctx, r.key("meta"), "override", "override_rate").Val()
	if vals[0] != "1" || vals[1] != "120000" {
		t.Fatalf("meta = %v", vals)
	}
	if ov := m["override"].(map[string]any); ov["rate_per_sec"] != float64(120) {
		t.Fatalf("response override: %v", ov)
	}
	for i := 0; i < 2; i++ {
		if code, m := r.del(t); code != http.StatusOK || m["paused"] != false {
			t.Fatalf("DELETE %d: %d %v", i, code, m)
		}
	}
	if n := r.rdb.HExists(r.ctx, r.key("meta"), "override").Val(); n {
		t.Fatal("override still present after DELETE")
	}
	if got := len(r.aud.entries); got != 3 {
		t.Fatalf("audit entries = %d, want 3", got)
	}
}

type healthyProbe struct{}

func (healthyProbe) Health(context.Context, string) (app.Health, error) { return app.Health{}, nil }

// AC-7: controller that + Redis. Sau PUT 0, admitted khong tang nua sau tick dang chay + 1 tick;
// sau DELETE, admit chay lai (rate > 0, admitted tang) trong DecreaseCooldown + 2 tick.
func TestTichHop_OpsOverride_AC7_RealControllerFollowsPauseAndResume(t *testing.T) {
	r := setup(t)
	r.rdb.HSet(r.ctx, r.key("meta"), "sale_start_ms", "1")
	for i := 0; i < 2000; i++ {
		if _, err := r.q.Join(r.ctx, r.ev, fmt.Sprintf("id-%04d", i), fmt.Sprintf("tok-%04d", i)); err != nil {
			t.Fatal(err)
		}
	}
	const epoch = 1
	if err := r.rdb.Set(r.ctx, r.key("leader_epoch"), epoch, 0).Err(); err != nil {
		t.Fatal(err)
	}

	cfg := app.DefaultAdmitConfig(r.ev, time.Now().Add(-time.Second))
	cfg.Tick = 200 * time.Millisecond
	tick := cfg.Tick
	fq := r.q.Fenced(epoch)
	ctl := app.NewAdmitController(fq, healthyProbe{}, cfg, slog.New(slog.NewTextHandler(io.Discard, nil))).WithStateStore(fq)
	ctx, cancel := context.WithCancel(r.ctx)
	done := make(chan struct{})
	go func() { defer close(done); _ = ctl.Run(ctx) }()
	t.Cleanup(func() { cancel(); <-done })

	admitted := func() int64 { return r.rdb.ZCard(r.ctx, r.key("admitted")).Val() }
	waitFor := func(d time.Duration, cond func() bool) bool {
		deadline := time.Now().Add(d)
		for time.Now().Before(deadline) {
			if cond() {
				return true
			}
			time.Sleep(tick / 4)
		}
		return cond()
	}
	if !waitFor(10*tick, func() bool { return admitted() > 0 }) {
		t.Fatal("controller never admitted anyone before the pause")
	}

	if code, m := r.put(t, 0); code != http.StatusOK || m["paused"] != true {
		t.Fatalf("PUT 0: %d %v", code, m)
	}
	// Tick dang chay + 1 tick (cong mot chut bien do lich).
	time.Sleep(2*tick + tick/2)
	frozen := admitted()
	time.Sleep(4 * tick)
	if got := admitted(); got != frozen {
		t.Fatalf("admitted grew while paused: %d -> %d", frozen, got)
	}
	if v := r.rdb.HGet(r.ctx, r.key("meta"), "admit_rate").Val(); v != "0" {
		t.Fatalf("meta.admit_rate while paused = %q, want 0", v)
	}

	if code, m := r.del(t); code != http.StatusOK || m["paused"] != false {
		t.Fatalf("DELETE: %d %v", code, m)
	}
	resumeWithin := time.Duration(cfg.DecreaseCooldown+2)*tick + tick/2
	if !waitFor(resumeWithin, func() bool { return admitted() > frozen }) {
		t.Fatalf("admission did not resume within %v after DELETE (admitted=%d)", resumeWithin, admitted())
	}
	if v := r.rdb.HGet(r.ctx, r.key("meta"), "admit_rate").Val(); v == "0" || v == "" {
		t.Fatalf("meta.admit_rate after resume = %q, want > 0", v)
	}
}
