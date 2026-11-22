//go:build integration

package redisstate_test

import (
	"testing"
	"time"
)

// R-4: re-joins of a slot holder do not increment the per-minute join counters (jc, nj), so one identity cannot
// loop joins to push its own /24 over R4N_SHARE.
func TestFix1_R4_RejoinsDoNotCountForShare(t *testing.T) {
	e := setup(t)
	e.rdb.HSet(e.ctx, "wr:{"+e.ev+"}:tok", "holder", "tok-1")
	for i := 0; i < 50; i++ {
		e.observe(t, "holder", time.Duration(i)*time.Second, "", pfx, "")
	}
	if n := len(e.rdb.Keys(e.ctx, e.key("jc:*")).Val()) + len(e.rdb.Keys(e.ctx, e.key("nj:*")).Val()); n != 0 {
		t.Fatalf("%d join-count keys written by slot-holder re-joins, want 0", n)
	}
	e.observe(t, "newcomer", time.Minute, "", pfx, "")
	minute := t0.Add(time.Minute).UnixMilli() / 60_000
	if v := e.rdb.Get(e.ctx, e.key("jc:"+itoa(minute))).Val(); v != "1" {
		t.Fatalf("first join counted %q, want 1", v)
	}
	if v := e.rdb.Get(e.ctx, e.key("nj:"+pfx.String()+":"+itoa(minute))).Val(); v != "1" {
		t.Fatalf("first join prefix count %q, want 1", v)
	}
}
