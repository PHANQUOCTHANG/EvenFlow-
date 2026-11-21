package postgresadapter_test

import (
	"context"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/eventflow/eventflow/libs/go/antibot"
	postgresadapter "github.com/eventflow/eventflow/services/waitingroom/internal/adapter/postgres"
)

func counterValue(t *testing.T, reg *prometheus.Registry, name string) float64 {
	t.Helper()
	mfs, err := reg.Gather()
	if err != nil {
		t.Fatal(err)
	}
	for _, mf := range mfs {
		if mf.GetName() == name {
			var v float64
			for _, m := range mf.GetMetric() {
				v += m.GetCounter().GetValue()
			}
			return v
		}
	}
	return 0
}

// AC-14 (contracts 5): without DATABASE_URL the writer is disabled: every row counts as dropped, Offer never blocks.
func TestDecisionWriter_AC14_NoPoolDropsAndCounts(t *testing.T) {
	reg := prometheus.NewRegistry()
	w := postgresadapter.NewDecisionWriter(nil, antibot.NewMetrics(reg))
	var _ antibot.Sink = w
	start := time.Now()
	for i := 0; i < 1000; i++ {
		w.Offer(antibot.Input{EventID: "e", IdentityID: "i"}, antibot.Decision{Action: "BLOCK", Applied: "BLOCK", Score: 0.9, Fired: []antibot.RuleID{"R2"}})
	}
	if el := time.Since(start); el > 200*time.Millisecond {
		t.Fatalf("1000 Offers took %s without a pool", el)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = w.Close(ctx)
	if v := counterValue(t, reg, "antibot_decisions_dropped_total"); v != 1000 {
		t.Fatalf("antibot_decisions_dropped_total = %v, want 1000", v)
	}
}
