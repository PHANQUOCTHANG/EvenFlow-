package httpadapter_test

import (
	"bytes"
	"net/http"
	"testing"
)

// R-2: a STICKY decision (the flag is already set by the first BLOCK) answers 403 without calling Revoke again,
// so a retry never touches the flag or its 24 h TTL.
func TestFix1_R2_StickyDoesNotRevokeAgain(t *testing.T) {
	s := &laneStore{revoked: true}
	var logs bytes.Buffer
	d := dec("BLOCK", "BLOCK", "STICKY")
	d.Fired = nil
	d.Score = 1
	rec := abJoin(abMux(s, &fakeGuard{d: d}, &logs), `{"signals":`+abSignals+`}`, nil)
	if rec.Code != http.StatusForbidden || problemType(rec) != "/problems/request-denied" {
		t.Fatalf("sticky: status %d type %q, want 403 request-denied", rec.Code, problemType(rec))
	}
	if n := len(revokes(s)); n != 0 {
		t.Fatalf("sticky called Revoke %d times, want 0", n)
	}
	if n := len(joins(s)); n != 0 {
		t.Fatalf("sticky joined: %+v", s.calls)
	}
}
