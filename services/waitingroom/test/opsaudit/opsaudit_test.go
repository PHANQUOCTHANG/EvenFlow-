//go:build integration

// Package opsaudit kiem duong audit cua lenh ghi de (ops-warroom AC-10, AC-11, AC-12) voi Redis va
// Postgres that, dung dung ham NewAuditorFromEnv ma cmd/server goi.
package opsaudit

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/eventflow/eventflow/libs/go/testx"
	httpadapter "github.com/eventflow/eventflow/services/waitingroom/internal/adapter/http"
	postgresadapter "github.com/eventflow/eventflow/services/waitingroom/internal/adapter/postgres"
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

type rig struct {
	ctx  context.Context
	rdb  *redis.Client
	mux  *http.ServeMux
	logs *bytes.Buffer
	ev   string
	aud  app.Auditor
}

// setup dung server Ops voi auditor lay tu DATABASE_URL = dsn (qua NewAuditorFromEnv).
func setup(t *testing.T, dsn string) *rig {
	t.Helper()
	ctx := context.Background()
	rdb := redis.NewClient(&redis.Options{Addr: testx.Redis(t)})
	t.Cleanup(func() { _ = rdb.Close() })
	q, err := redisadapter.NewQueue(ctx, rdb, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	logs := &bytes.Buffer{}
	log := slog.New(slog.NewTextHandler(logs, nil))
	aud, closeAud := postgresadapter.NewAuditorFromEnv(ctx, func(k string) string {
		if k == "DATABASE_URL" {
			return dsn
		}
		return ""
	}, log)
	t.Cleanup(closeAud)
	mux := http.NewServeMux()
	httpadapter.NewOpsHandler(redisadapter.NewOpsReader(rdb, nil), app.NewOpsOverride(q, aud, log), log).Routes(mux)
	r := &rig{ctx: ctx, rdb: rdb, mux: mux, logs: logs, ev: newUUID(t), aud: aud}
	rdb.HSet(ctx, r.metaKey(), "sale_start_ms", "1")
	return r
}

func (r *rig) metaKey() string { return "wr:{" + r.ev + "}:meta" }

func (r *rig) call(t *testing.T, method, body string) (int, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(method, "/v1/ops/events/"+r.ev+"/admit-override", strings.NewReader(body))
	req.Header.Set("X-Identity-Id", actor)
	req.Header.Set("X-Identity-Role", "ops")
	req.Header.Set("X-Request-Id", "req-tichhop")
	rec := httptest.NewRecorder()
	r.mux.ServeHTTP(rec, req)
	var m map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &m)
	return rec.Code, m
}

func (r *rig) overrideFields() []any {
	return r.rdb.HMGet(r.ctx, r.metaKey(), "override", "override_rate").Val()
}

// AC-10: mot lenh ap dung = dung mot dong audit_logs voi cac cot mong doi.
func TestTichHop_OpsAudit_AC10_OneRowPerApply(t *testing.T) {
	dsn := testx.Postgres(t)
	r := setup(t, dsn)
	if r.aud == nil {
		t.Fatal("auditor must be configured for a reachable database")
	}
	if code, m := r.call(t, http.MethodPut, `{"rate_per_sec":0,"reason":"  su co thanh toan  "}`); code != 200 || m["audit"] != "recorded" {
		t.Fatalf("PUT: %d %v", code, m)
	}
	if code, m := r.call(t, http.MethodDelete, `{"reason":"da xu ly"}`); code != 200 || m["audit"] != "recorded" {
		t.Fatalf("DELETE: %d %v", code, m)
	}

	pool, err := pgxpool.New(r.ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	rows, err := pool.Query(r.ctx,
		`SELECT actor_type, actor_id, action, reason, metadata FROM audit_logs WHERE subject = $1 ORDER BY id`,
		"event:"+r.ev)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	type row struct {
		actorType, actorID, action, reason string
		meta                               map[string]any
	}
	var got []row
	for rows.Next() {
		var x row
		var raw []byte
		if err := rows.Scan(&x.actorType, &x.actorID, &x.action, &x.reason, &raw); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(raw, &x.meta); err != nil {
			t.Fatalf("metadata is not JSON: %v", err)
		}
		got = append(got, x)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("audit rows = %d, want 2 (one per apply)", len(got))
	}
	p := got[0]
	if p.actorType != "human" || p.actorID != actor || p.action != "ADMIT_PAUSED" || p.reason != "su co thanh toan" {
		t.Fatalf("pause row: %+v", p)
	}
	if p.meta["rate_per_sec"] != float64(0) || p.meta["request_id"] != "req-tichhop" {
		t.Fatalf("pause metadata: %v", p.meta)
	}
	if prev, _ := p.meta["previous"].(map[string]any); prev == nil || prev["active"] != false {
		t.Fatalf("pause metadata.previous: %v", p.meta["previous"])
	}
	c := got[1]
	if c.action != "ADMIT_OVERRIDE_CLEARED" {
		t.Fatalf("clear row: %+v", c)
	}
	if prev, _ := c.meta["previous"].(map[string]any); prev == nil || prev["active"] != true || prev["rate_per_sec"] != float64(0) {
		t.Fatalf("clear metadata.previous: %v", c.meta["previous"])
	}
}

// AC-11 / AC-12 (MAJOR-2): DATABASE_URL hop le nhung DB khong toi duoc -> auditor van co; lenh van ap dung,
// tra 200 audit:"failed" va mot dong log ERROR.
func TestTichHop_OpsAudit_AC11_UnreachableDatabaseStillApplies(t *testing.T) {
	r := setup(t, "postgres://u:p@127.0.0.1:1/x?connect_timeout=1")
	if r.aud == nil {
		t.Fatal("an unreachable but parsable DATABASE_URL must keep the auditor (D2 path)")
	}
	code, m := r.call(t, http.MethodPut, `{"rate_per_sec":0,"reason":"khan cap"}`)
	if code != http.StatusOK || m["audit"] != "failed" || m["paused"] != true {
		t.Fatalf("PUT: %d %v", code, m)
	}
	if v := r.overrideFields(); v[0] != "1" || v[1] != "0" {
		t.Fatalf("override not applied: %v", v)
	}
	if n := strings.Count(r.logs.String(), "level=ERROR"); n != 1 {
		t.Fatalf("ERROR lines = %d, want 1:\n%s", n, r.logs.String())
	}
}

// AC-12: DATABASE_URL thieu / rong / khong parse duoc -> 503 audit-unavailable, Redis khong doi, doc van chay.
func TestTichHop_OpsAudit_AC12_MissingOrBadURLFailsClosed(t *testing.T) {
	for _, dsn := range []string{"", "   ", "::not a url"} {
		r := setup(t, dsn)
		if r.aud != nil {
			t.Fatalf("DATABASE_URL=%q: auditor must be a plain nil interface", dsn)
		}
		for _, c := range []struct{ method, body string }{
			{http.MethodPut, `{"rate_per_sec":0,"reason":"abc"}`},
			{http.MethodDelete, `{"reason":"abc"}`},
		} {
			code, m := r.call(t, c.method, c.body)
			if code != http.StatusServiceUnavailable || m["type"] != "/problems/audit-unavailable" {
				t.Fatalf("DATABASE_URL=%q %s: %d %v", dsn, c.method, code, m)
			}
		}
		if v := r.overrideFields(); v[0] != nil || v[1] != nil {
			t.Fatalf("DATABASE_URL=%q: Redis changed without an audit store: %v", dsn, v)
		}
		req := httptest.NewRequest(http.MethodGet, "/v1/ops/events/"+r.ev+"/queue", nil)
		req.Header.Set("X-Identity-Id", actor)
		req.Header.Set("X-Identity-Role", "ops")
		rec := httptest.NewRecorder()
		r.mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("DATABASE_URL=%q: read endpoint must keep working: %d", dsn, rec.Code)
		}
	}
}
