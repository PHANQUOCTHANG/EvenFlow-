package httpadapter_test

// Mutation gap T04 (ops-warroom verification): a not-found result is cached for the 2 s window, so a loop of
// requests for an unknown event cannot hammer the store (ttlCache doc comment).

import (
	"testing"
)

const opsUnknownEv = "77777777-7777-4777-8777-777777777777"

func TestOpsCache_NotFoundIsCachedInsideTTL(t *testing.T) {
	r := baseReader()
	mux := newOpsMux(r)
	for i := 0; i < 3; i++ {
		if rec := opsGet(mux, invPath(opsUnknownEv), opsUser, "ops"); rec.Code != 404 {
			t.Fatalf("call %d: status %d, want 404", i, rec.Code)
		}
	}
	if n := r.invCalls.Load(); n != 1 {
		t.Fatalf("store calls for unknown event inside TTL = %d, want 1 (not-found must be cached)", n)
	}
}
