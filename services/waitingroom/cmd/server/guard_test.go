package main

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/eventflow/eventflow/libs/go/antibot"
)

func quietLog() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

// P-15 / AC-13: ANTIBOT_MODE=off returns an untyped nil BotGuard, so the handler keeps develop's join.
func TestBuildGuard_AC13_OffIsUntypedNil(t *testing.T) {
	cfg, err := antibot.LoadConfig(func(k string) string { return map[string]string{"ANTIBOT_MODE": "off"}[k] })
	if err != nil {
		t.Fatal(err)
	}
	g, m, closeFn := buildGuard(context.Background(), cfg, "127.0.0.1:1", nil, prometheus.NewRegistry(), quietLog())
	defer closeFn()
	if g != nil || m != nil {
		t.Fatalf("off: guard %#v metrics %v, want untyped nil", g, m)
	}
}

// D7: shadow with Redis unreachable at startup still builds a guard; decisions degrade and pass.
func TestBuildGuard_D7_RedisDownStillBuilds(t *testing.T) {
	cfg, err := antibot.LoadConfig(func(string) string { return "" }) // shadow, dev key
	if err != nil {
		t.Fatal(err)
	}
	reg := prometheus.NewRegistry()
	g, m, closeFn := buildGuard(context.Background(), cfg, "127.0.0.1:1", nil, reg, quietLog())
	defer closeFn()
	if g == nil || m == nil {
		t.Fatal("shadow: no guard")
	}
	d := g.Decide(context.Background(), antibot.Input{EventID: "e", IdentityID: "i", UserAgent: "Mozilla/5.0",
		RawSignals: []byte(`{"v":1,"pe":3}`)})
	if d.Applied != antibot.ActionPass {
		t.Fatalf("applied %s, want PASS", d.Applied)
	}
	degraded := false
	for _, tag := range d.Tags {
		degraded = degraded || tag == antibot.TagDegraded
	}
	if !degraded {
		t.Fatalf("tags %v, want DEGRADED", d.Tags)
	}
}
