//go:build integration

package redisadapter_test

import (
	"fmt"
	"testing"
	"time"

	redisadapter "github.com/eventflow/eventflow/services/waitingroom/internal/adapter/redis"
	"github.com/eventflow/eventflow/services/waitingroom/internal/domain"
)

// R-2: a second BLOCK (revoke.lua) never extends the 24 h flag: the window starts at the first BLOCK.
func TestFix1_R2_SecondRevokeKeepsFlagTTL(t *testing.T) {
	e := setup(t)
	e.afterT0()
	e.join(t, "x", domain.LaneNormal)
	if _, err := e.q.Revoke(e.ctx, e.ev, "x"); err != nil {
		t.Fatal(err)
	}
	flag := "ab:{" + e.ev + "}:blk:x"
	e.rdb.Expire(e.ctx, flag, 1000*time.Second) // as if most of the 24 h had passed
	if _, err := e.q.Revoke(e.ctx, e.ev, "x"); err != nil {
		t.Fatal(err)
	}
	if ttl := e.rdb.TTL(e.ctx, flag).Val(); ttl > 1000*time.Second || ttl <= 0 {
		t.Fatalf("flag TTL after a second revoke = %s, want <= 1000 s (not reset to 24 h)", ttl)
	}
}

// R-3 / X-7: slow joins between T0 and the shuffle rank behind every pre-T0 slow lobby member.
func TestFix1_R3_SlowJoinBeforeShuffleRanksBehindSlowLobby(t *testing.T) {
	e := setup(t)
	e.beforeT0()
	for i := 1; i <= 5; i++ {
		e.join(t, fmt.Sprintf("pre%d", i), domain.LaneSlow)
	}
	e.afterT0() // T0 passed, the controller has not shuffled yet
	late := []string{e.join(t, "late1", domain.LaneSlow).Token, e.join(t, "late2", domain.LaneSlow).Token}
	e.shuffle(t, 99)
	var maxPre int64
	for i := 1; i <= 5; i++ {
		if sc := e.score(t, fmt.Sprintf("tok-pre%d", i)); sc > maxPre {
			maxPre = sc
		}
	}
	for _, tok := range late {
		if sc := e.score(t, tok); sc <= maxPre {
			t.Fatalf("post-T0 slow joiner %s score %d is not behind the slow lobby (max %d)", tok, sc, maxPre)
		}
	}
}

// R-8: the ops snapshot reports the slow-lane lobby separately.
func TestFix1_R8_OpsCountsSlowLobby(t *testing.T) {
	e := setup(t)
	e.beforeT0()
	e.join(t, "n1", domain.LaneNormal)
	e.join(t, "s1", domain.LaneSlow)
	e.join(t, "s2", domain.LaneSlow)
	st, err := redisadapter.NewOpsReader(e.rdb, nil).QueueOpsState(e.ctx, e.ev, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if st.Lobby != 1 || st.LobbySlow != 2 {
		t.Fatalf("ops lobby %d slow %d, want 1 and 2", st.Lobby, st.LobbySlow)
	}
}
