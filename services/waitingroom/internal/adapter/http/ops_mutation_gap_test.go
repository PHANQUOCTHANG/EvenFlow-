package httpadapter_test

// Mutation gaps M12, M17, M21 (ops-warroom verification):
//   M12: contract 3.4 "one audit row per successful apply": an apply that fails must not be audited.
//   M17: contract 3.4: apply and audit run on a context detached from the request, so a client disconnect
//        in the middle of the call cannot cancel them.
//   M21: decodeStrict rejects trailing bytes after the JSON body.

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	httpadapter "github.com/eventflow/eventflow/services/waitingroom/internal/adapter/http"
	"github.com/eventflow/eventflow/services/waitingroom/internal/app"
)

// applyStore succeeds on every read; SetRateOverride runs hook (if set) and then fails with applyErr (if set).
type applyStore struct {
	mu       sync.Mutex
	applyErr error
	hook     func(ctx context.Context) error
	sets     []float64
}

func (s *applyStore) RoomConfigured(context.Context, string) (bool, error) { return true, nil }

func (s *applyStore) RateOverride(context.Context, string) (float64, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.sets) == 0 {
		return 0, false, nil
	}
	return s.sets[len(s.sets)-1], true, nil
}

func (s *applyStore) SetRateOverride(ctx context.Context, _ string, r float64) error {
	if s.hook != nil {
		if err := s.hook(ctx); err != nil {
			return err
		}
	}
	if s.applyErr != nil {
		return s.applyErr
	}
	s.mu.Lock()
	s.sets = append(s.sets, r)
	s.mu.Unlock()
	return nil
}

func (s *applyStore) ClearRateOverride(context.Context, string) error { return nil }

// ctxAuditor fails if the context it receives is already cancelled.
type ctxAuditor struct {
	mu      sync.Mutex
	entries []app.AuditEntry
}

func (a *ctxAuditor) Record(ctx context.Context, e app.AuditEntry) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	a.mu.Lock()
	a.entries = append(a.entries, e)
	a.mu.Unlock()
	return nil
}

func (a *ctxAuditor) rows() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.entries)
}

func applyMux(store app.OverrideStore, aud app.Auditor) *http.ServeMux {
	log := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	mux := http.NewServeMux()
	httpadapter.NewOpsHandler(&fakeState{}, app.NewOpsOverride(store, aud, log), log).Routes(mux)
	return mux
}

func opsPut(ctx context.Context, mux http.Handler, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPut, ovPath(opsEvent), strings.NewReader(body)).WithContext(ctx)
	req.Header.Set("X-Identity-Id", opsActor)
	req.Header.Set("X-Identity-Role", "ops")
	req.Header.Set("X-Request-Id", "req-123")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func TestOpsPut_ApplyFailureIsNotAudited(t *testing.T) {
	store := &applyStore{applyErr: errBoom}
	aud := &ctxAuditor{}
	rec := opsPut(context.Background(), applyMux(store, aud), `{"rate_per_sec":50,"reason":"redis down test"}`)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status %d, want 503: %s", rec.Code, rec.Body)
	}
	if n := aud.rows(); n != 0 {
		t.Fatalf("audit rows = %d after a failed apply, want 0", n)
	}
}

func TestOpsPut_RequestCancelledMidApplyStillAppliesAndAudits(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	store := &applyStore{hook: func(c context.Context) error {
		cancel() // client disconnects while the Redis write is in flight
		return c.Err()
	}}
	aud := &ctxAuditor{}
	rec := opsPut(ctx, applyMux(store, aud), `{"rate_per_sec":0,"reason":"emergency pause"}`)
	if ctx.Err() == nil {
		t.Fatal("test setup: request context was not cancelled")
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, want 200: %s", rec.Code, rec.Body)
	}
	if len(store.sets) != 1 || store.sets[0] != 0 {
		t.Fatalf("store sets = %v, want [0]", store.sets)
	}
	if m := decode(t, rec); m["audit"] != app.AuditRecorded {
		t.Fatalf("audit = %v, want recorded", m["audit"])
	}
	if n := aud.rows(); n != 1 {
		t.Fatalf("audit rows = %d, want 1", n)
	}
}

func TestOpsPut_TrailingBytesAfterJSONIs400(t *testing.T) {
	for _, body := range []string{
		`{"rate_per_sec":1,"reason":"abc"} x`,
		`{"rate_per_sec":1,"reason":"abc"}{"rate_per_sec":2,"reason":"abc"}`,
	} {
		store := &applyStore{}
		aud := &ctxAuditor{}
		rec := opsPut(context.Background(), applyMux(store, aud), body)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("body %q: status %d, want 400", body, rec.Code)
		}
		if len(store.sets) != 0 || aud.rows() != 0 {
			t.Fatalf("body %q: applied=%v audited=%d, want nothing", body, store.sets, aud.rows())
		}
	}
}
