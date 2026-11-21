//go:build integration

package postgresadapter_test

import (
	"context"
	"fmt"
	"net/netip"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/eventflow/eventflow/libs/go/antibot"
	"github.com/eventflow/eventflow/libs/go/testx"
	postgresadapter "github.com/eventflow/eventflow/services/waitingroom/internal/adapter/postgres"
)

func TestMain(m *testing.M) {
	code := m.Run()
	testx.Shutdown()
	os.Exit(code)
}

func pool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	p, err := pgxpool.New(context.Background(), testx.Postgres(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(p.Close)
	return p
}

type row struct {
	event, identity, fp, ip *string
	score                   float64
	action                  string
	fired                   []string
	all                     string
}

func rowsFor(t *testing.T, p *pgxpool.Pool, identity string) []row {
	t.Helper()
	rs, err := p.Query(context.Background(), `SELECT event_id::text, identity_id::text, device_fp, host(ip) || '/' || masklen(ip),
		score::float8, action, fired_rules, b::text FROM bot_decisions b WHERE identity_id::text = $1 OR (identity_id IS NULL AND device_fp = $1) ORDER BY id`, identity)
	if err != nil {
		t.Fatal(err)
	}
	defer rs.Close()
	var out []row
	for rs.Next() {
		var r row
		if err := rs.Scan(&r.event, &r.identity, &r.fp, &r.ip, &r.score, &r.action, &r.fired, &r.all); err != nil {
			t.Fatal(err)
		}
		out = append(out, r)
	}
	return out
}

const rawFP = "raw-device-fp-777777"

func in(ev, id string) antibot.Input {
	return antibot.Input{EventID: ev, IdentityID: id, DeviceID: "33333333-3333-4333-8333-333333333333",
		UserAgent: "Mozilla/5.0 Chrome/131", ClientIP: netip.MustParseAddr("203.0.113.77"),
		RawSignals: []byte(`{"v":1,"fp":"` + rawFP + `"}`), Now: time.Now()}
}

func closeW(t *testing.T, w *postgresadapter.DecisionWriter) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2500*time.Millisecond)
	defer cancel()
	start := time.Now()
	if err := w.Close(ctx); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if el := time.Since(start); el > 2500*time.Millisecond {
		t.Fatalf("shutdown flush took %s (budget 2 s)", el)
	}
}

// AC-14 / contracts 5: columns, PASS → ALLOW, tags in fired_rules, prefix INET, STICKY score 1.000.
func TestDecisionWriter_AC14_RowShape(t *testing.T) {
	p := pool(t)
	reg := prometheus.NewRegistry()
	w := postgresadapter.NewDecisionWriter(p, antibot.NewMetrics(reg))
	ev := "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	id := fmt.Sprintf("bbbbbbbb-bbbb-4bbb-8bbb-%012d", time.Now().UnixNano()%1e12)
	fph := "0123456789abcdef0123456789abcdef"
	w.Offer(in(ev, id), antibot.Decision{Action: "BLOCK", Applied: "BLOCK", Score: 0.88, Fired: []antibot.RuleID{"R2", "R5"},
		FPHash: fph, NetPrefix: netip.MustParsePrefix("203.0.113.0/24")})
	w.Offer(in(ev, id), antibot.Decision{Action: "PASS", Applied: "PASS", Score: 0.25, Fired: []antibot.RuleID{"R4N"}, Tags: []string{"MODE_SHADOW"},
		NetPrefix: netip.MustParsePrefix("2001:db8:1:2::/64")})
	w.Offer(in(ev, id), antibot.Decision{Action: "BLOCK", Applied: "BLOCK", Score: 1, Tags: []string{"STICKY"}})
	w.Offer(in(ev, id), antibot.Decision{Action: "SLOW_LANE", Applied: "PASS", Score: 0.88, Fired: []antibot.RuleID{"R3", "R5"}, Tags: []string{"CAPPED", "REJOIN"}})
	closeW(t, w)

	rs := rowsFor(t, p, id)
	if len(rs) != 4 {
		t.Fatalf("rows = %d, want 4", len(rs))
	}
	if *rs[0].event != ev || *rs[0].identity != id || rs[0].fp == nil || *rs[0].fp != fph || *rs[0].ip != "203.0.113.0/24" ||
		rs[0].score != 0.88 || rs[0].action != "BLOCK" || strings.Join(rs[0].fired, ",") != "R2,R5" {
		t.Errorf("row 0 %+v", rs[0])
	}
	if rs[1].action != "ALLOW" || strings.Join(rs[1].fired, ",") != "R4N,MODE_SHADOW" || rs[1].fp != nil || *rs[1].ip != "2001:db8:1:2::/64" {
		t.Errorf("row 1 action %s fired %v ip %v", rs[1].action, rs[1].fired, rs[1].ip)
	}
	if rs[2].score != 1 || strings.Join(rs[2].fired, ",") != "STICKY" || rs[2].ip != nil {
		t.Errorf("row 2 (sticky) score %v fired %v", rs[2].score, rs[2].fired)
	}
	if rs[3].action != "SLOW_LANE" || strings.Join(rs[3].fired, ",") != "R3,R5,CAPPED,REJOIN" {
		t.Errorf("row 3 action %s (tier, not applied) fired %v", rs[3].action, rs[3].fired)
	}
	// AC-15: no raw fingerprint, UA or full IP in any column
	for i, r := range rs {
		for _, raw := range []string{rawFP, "203.0.113.77", "Chrome", "Mozilla"} {
			if strings.Contains(r.all, raw) {
				t.Errorf("row %d contains raw %q: %s", i, raw, r.all)
			}
		}
	}
}

// AC-14 (R2, P-14): non-UUID event / identity ids are stored as NULL and counted.
func TestDecisionWriter_AC14_NonUUIDAsNull(t *testing.T) {
	p := pool(t)
	reg := prometheus.NewRegistry()
	w := postgresadapter.NewDecisionWriter(p, antibot.NewMetrics(reg))
	marker := fmt.Sprintf("%032x", time.Now().UnixNano())[:32]
	w.Offer(in("not-a-uuid", "also-not"), antibot.Decision{Action: "BLOCK", Applied: "BLOCK", Score: 0.9, Fired: []antibot.RuleID{"R2"}, FPHash: marker})
	closeW(t, w)
	rs := rowsFor(t, p, marker)
	if len(rs) != 1 || rs[0].event != nil || rs[0].identity != nil {
		t.Fatalf("rows %+v, want one row with NULL ids", rs)
	}
	if v := counterValue(t, reg, "antibot_decisions_nonuuid_total"); v < 1 {
		t.Fatalf("antibot_decisions_nonuuid_total = %v", v)
	}
}

// AC-14: the request path never waits: 20 000 Offers return quickly; rows written + dropped = offered.
func TestDecisionWriter_AC14_BoundedBufferNeverBlocks(t *testing.T) {
	p := pool(t)
	reg := prometheus.NewRegistry()
	w := postgresadapter.NewDecisionWriter(p, antibot.NewMetrics(reg))
	id := fmt.Sprintf("cccccccc-cccc-4ccc-8ccc-%012d", time.Now().UnixNano()%1e12)
	const n = 20000
	start := time.Now()
	for i := 0; i < n; i++ {
		w.Offer(in("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", id), antibot.Decision{Action: "CHALLENGE", Applied: "PASS", Score: 0.4, Fired: []antibot.RuleID{"R2"}})
	}
	if el := time.Since(start); el > time.Second {
		t.Fatalf("%d Offers took %s: the request path waited for Postgres", n, el)
	}
	closeW(t, w)
	var written int
	if err := p.QueryRow(context.Background(), `SELECT count(*) FROM bot_decisions WHERE identity_id::text = $1`, id).Scan(&written); err != nil {
		t.Fatal(err)
	}
	dropped := counterValue(t, reg, "antibot_decisions_dropped_total")
	if float64(written)+dropped != n {
		t.Fatalf("written %d + dropped %v != %d offered", written, dropped, n)
	}
	t.Logf("written %d, dropped %v", written, dropped)
}
