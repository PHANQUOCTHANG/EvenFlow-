package httpadapter_test

import (
	"bytes"
	"net/http"
	"testing"

	"github.com/eventflow/eventflow/libs/go/antibot"
)

// M47 / AC-13: in shadow every applied PASS uses the develop 4-argument Join, never JoinLane (the lane path checks
// the 24 h block flag, which must not deny anyone in shadow, e.g. after an enforce -> shadow rollback).
func TestFix2_M47_ShadowUsesDevelopJoin(t *testing.T) {
	for _, d := range []antibot.Decision{
		dec("BLOCK", "PASS", "MODE_SHADOW"),
		dec("SLOW_LANE", "PASS", "MODE_SHADOW"),
		{Action: "PASS", Applied: "PASS", Tags: []string{"MODE_SHADOW"}},
		{Action: "CHALLENGE", Applied: "PASS", Tags: []string{"MODE_SHADOW", "REJOIN"}},
	} {
		s := &laneStore{}
		var logs bytes.Buffer
		rec := abJoin(abMux(s, &fakeGuard{d: d}, &logs), `{"signals":`+abSignals+`}`, nil)
		js := joins(s)
		if rec.Code != http.StatusOK || len(js) != 1 || js[0].method != "Join" || len(revokes(s)) != 0 {
			t.Fatalf("shadow %s/%s: status %d calls %+v, want one 4-arg Join", d.Action, d.Applied, rec.Code, s.calls)
		}
	}
	// enforce PASS (no MODE_SHADOW) goes through the lane path, so the flag check applies there
	s := &laneStore{}
	var logs bytes.Buffer
	abJoin(abMux(s, &fakeGuard{d: antibot.Decision{Action: "PASS", Applied: "PASS"}}, &logs), `{"signals":`+abSignals+`}`, nil)
	if js := joins(s); len(js) != 1 || js[0].method != "JoinLane" {
		t.Fatalf("enforce PASS calls %+v, want JoinLane(normal)", s.calls)
	}
}
