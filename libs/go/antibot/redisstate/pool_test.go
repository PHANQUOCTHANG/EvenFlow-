package redisstate_test

import (
	"context"
	"encoding/base64"
	"errors"
	"net"
	"net/netip"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/redis/go-redis/v9"

	"github.com/eventflow/eventflow/libs/go/antibot"
	"github.com/eventflow/eventflow/libs/go/antibot/redisstate"
)

// silentServer accepts TCP connections and never answers (a Redis that hangs), no Docker needed.
func silentServer(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	var conns []net.Conn
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			conns = append(conns, c)
		}
	}()
	t.Cleanup(func() {
		_ = ln.Close()
		for _, c := range conns {
			_ = c.Close()
		}
	})
	return ln.Addr().String()
}

func unitCfg(t *testing.T, kv ...string) antibot.Config {
	t.Helper()
	m := map[string]string{"ANTIBOT_MODE": "enforce",
		"ANTIBOT_HASH_KEY": base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef"))}
	for i := 0; i+1 < len(kv); i += 2 {
		m[kv[i]] = kv[i+1]
	}
	c, err := antibot.LoadConfig(func(k string) string { return m[k] })
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// R2-1: with every connection busy, the real go-redis client reports pool exhaustion (not a plain deadline), the
// store wraps it as antibot.ErrPoolExhausted, and the guard counts it without opening the breaker.
func TestFix2_R21_PoolExhaustionIsClassified(t *testing.T) {
	cfg := unitCfg(t, "ANTIBOT_REDIS_TIMEOUT_MS", "20", "ANTIBOT_BREAKER", "1/1s,open=3s")
	opts := redisstate.ClientOptions(silentServer(t), cfg)
	if opts.PoolTimeout >= cfg.RedisTimeout {
		t.Fatalf("PoolTimeout %s not below the budget %s", opts.PoolTimeout, cfg.RedisTimeout)
	}
	opts.PoolSize, opts.MinIdleConns, opts.DialTimeout = 1, 0, time.Second
	opts.Protocol, opts.DisableIdentity = 2, true // no HELLO / CLIENT SETINFO on dial: the server never answers
	opts.ReadTimeout = -1                         // the holder below must keep the connection; Observe is bounded by its ctx
	rdb := redis.NewClient(opts)
	t.Cleanup(func() { _ = rdb.Close() })

	// hold the only connection: a blocking command (no read timeout) that the silent server never answers
	hold, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	go func() { _ = rdb.BLPop(hold, 0, "held").Err() }()
	time.Sleep(100 * time.Millisecond)

	st := redisstate.NewLazy(rdb, cfg)
	in := antibot.Input{EventID: "ev", IdentityID: "id", Now: time.Now()}
	ctx, c2 := context.WithTimeout(context.Background(), cfg.RedisTimeout)
	defer c2()
	_, err := st.Observe(ctx, in, "", netip.Prefix{}, "")
	if !errors.Is(err, antibot.ErrPoolExhausted) {
		t.Fatalf("Observe with an exhausted pool: %v, want antibot.ErrPoolExhausted", err)
	}

	reg := prometheus.NewRegistry()
	g := antibot.NewGuard(cfg, st, nil, antibot.NewMetrics(reg))
	for i := 0; i < 3; i++ {
		g.Decide(context.Background(), antibot.Input{EventID: "ev", IdentityID: "id", UserAgent: "Mozilla/5.0",
			RawSignals: []byte(`{"v":1,"pe":2}`), Now: time.Now()})
	}
	mfs, _ := reg.Gather()
	got := map[string]float64{}
	for _, mf := range mfs {
		if mf.GetName() != "antibot_degraded_total" {
			continue
		}
		for _, m := range mf.GetMetric() {
			got[m.GetLabel()[0].GetValue()] = m.GetCounter().GetValue()
		}
	}
	if got["breaker_open"] != 0 || got["redis_error"] != 0 || got["redis_timeout"] != 3 {
		t.Fatalf("degraded counters %v: pool exhaustion must count as redis_timeout and never open the breaker (threshold 1)", got)
	}
}
