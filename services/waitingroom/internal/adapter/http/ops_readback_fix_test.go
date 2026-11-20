package httpadapter_test

// ops-warroom fix round 1: X-3 (PUT response built from the stored value read back).

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"testing"

	httpadapter "github.com/eventflow/eventflow/services/waitingroom/internal/adapter/http"
	"github.com/eventflow/eventflow/services/waitingroom/internal/app"
)

// raceStore: another Ops' DELETE lands right after our HSET.
type raceStore struct{ fakeOverrideStore }

func (r *raceStore) SetRateOverride(ctx context.Context, ev string, rate float64) error {
	_ = r.fakeOverrideStore.SetRateOverride(ctx, ev, rate)
	return r.fakeOverrideStore.ClearRateOverride(ctx, ev)
}

// X-3: the PUT response reports the stored state read back, even when it was cleared concurrently.
func TestOpsPutFix_X3_ResponseReflectsStoredStateAfterConcurrentClear(t *testing.T) {
	st := &raceStore{fakeOverrideStore{configured: true}}
	aud := &fakeAuditor{}
	log := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	mux := http.NewServeMux()
	httpadapter.NewOpsHandler(&fakeState{}, app.NewOpsOverride(st, aud, log), log).Routes(mux)
	rec := opsCall(mux, http.MethodPut, ovPath(opsEvent), "ops", `{"rate_per_sec":120,"reason":"abc"}`)
	if rec.Code != 200 {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	m := decode(t, rec)
	ov, _ := m["override"].(map[string]any)
	if ov == nil || ov["active"] != false || ov["rate_per_sec"] != nil {
		t.Fatalf("override must report the stored state (inactive): %v", m)
	}
	if m["paused"] != false {
		t.Fatalf("paused must be false when no override is stored: %v", m)
	}
}
