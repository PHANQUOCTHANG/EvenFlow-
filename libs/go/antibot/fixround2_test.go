package antibot_test

// Regression tests for fix round 2 (re-review R2-2, R2-3; verification mutant M16). New file.

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"net/netip"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/eventflow/eventflow/libs/go/antibot"
)

// step is the scripted outcome of one Observe call: wait for release (if set, ignoring the context, so a call can
// stay in flight as long as the test needs), then return err.
type step struct {
	release chan struct{}
	err     error
}

// stepStore plays one step per call; calls past the script succeed at once.
type stepStore struct {
	mu    sync.Mutex
	steps []step
	calls int
}

func (s *stepStore) Observe(_ context.Context, _ antibot.Input, _ string, _ netip.Prefix, _ string) (antibot.State, error) {
	s.mu.Lock()
	var st step
	if s.calls < len(s.steps) {
		st = s.steps[s.calls]
	}
	s.calls++
	s.mu.Unlock()
	if st.release != nil {
		<-st.release // ignores ctx on purpose: keeps the call in flight past the budget
	}
	if st.err != nil {
		return antibot.State{}, st.err
	}
	return neutral(), nil
}

func (s *stepStore) n() int { s.mu.Lock(); defer s.mu.Unlock(); return s.calls }

var errDown = errors.New("connection refused")

func decide(e guardEnv) antibot.Decision {
	return e.g.Decide(context.Background(), input([]byte(validSignals)))
}

// M16: while the half-open probe is in flight (the store holds it past the budget), concurrent calls do not reach Redis.
func TestFix2_M16_OneProbeDuringRealHalfOpen(t *testing.T) {
	probe := make(chan struct{})
	st := &stepStore{steps: []step{{err: errDown}, {release: probe}}}
	e := scripted(t, st, "ANTIBOT_REDIS_TIMEOUT_MS", "50", "ANTIBOT_BREAKER", "1/1s,open=100ms")
	decide(e) // hard error: open
	time.Sleep(130 * time.Millisecond)
	done := make(chan antibot.Decision)
	go func() { done <- decide(e) }() // the probe, held by the store
	time.Sleep(30 * time.Millisecond)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); decide(e) }()
	}
	wg.Wait()
	if st.n() != 2 {
		t.Fatalf("%d calls reached Redis during one half-open probe, want 2 (trip + probe)", st.n())
	}
	if v := degradedTotal(t, e, "breaker_open"); v != 8 {
		t.Fatalf("breaker_open = %v during the probe, want 8", v)
	}
	close(probe)
	if d := <-done; hasTag(d, "DEGRADED") {
		t.Fatalf("successful probe degraded: %v", d.Tags)
	}
	if d := decide(e); hasTag(d, "DEGRADED") || st.n() != 3 {
		t.Fatalf("breaker not closed after the probe: calls %d tags %v", st.n(), d.Tags)
	}
}

// R2-3: a call admitted while closed that finishes during half-open does not move the breaker; only the probe does.
func TestFix2_R23_NonProbeResultIgnoredWhileHalfOpen(t *testing.T) {
	early, probe := make(chan struct{}), make(chan struct{})
	st := &stepStore{steps: []step{{release: early}, {err: errDown}, {release: probe, err: errDown}}}
	e := scripted(t, st, "ANTIBOT_REDIS_TIMEOUT_MS", "50", "ANTIBOT_BREAKER", "1/1s,open=100ms")

	earlyDone := make(chan struct{})
	go func() { decide(e); close(earlyDone) }() // call 1: admitted while closed, still in flight
	time.Sleep(20 * time.Millisecond)
	decide(e) // call 2: hard error -> open
	time.Sleep(130 * time.Millisecond)
	probeDone := make(chan struct{})
	go func() { decide(e); close(probeDone) }() // call 3: the half-open probe, held
	time.Sleep(20 * time.Millisecond)

	close(early) // call 1 succeeds now: must NOT close the half-open breaker
	<-earlyDone
	before := st.n()
	if d := decide(e); st.n() != before || !hasTag(d, "DEGRADED") {
		t.Fatalf("a late non-probe success closed the breaker: calls %d->%d tags %v", before, st.n(), d.Tags)
	}
	close(probe) // the probe fails: open again
	<-probeDone
	if d := decide(e); st.n() != before || !hasTag(d, "DEGRADED") {
		t.Fatalf("breaker not re-opened by the failed probe: calls %d->%d tags %v", before, st.n(), d.Tags)
	}
}

// R2-3: a non-probe failure that lands while half-open does not re-trip the breaker (which would double the next
// open time); only the probe's failure re-opens it, for 2 x 100 ms.
func TestFix2_R23_NonProbeFailureIgnoredWhileHalfOpen(t *testing.T) {
	early, probe := make(chan struct{}), make(chan struct{})
	st := &stepStore{steps: []step{{release: early, err: errDown}, {err: errDown}, {release: probe, err: errDown}}}
	e := scripted(t, st, "ANTIBOT_REDIS_TIMEOUT_MS", "50", "ANTIBOT_BREAKER", "1/1s,open=100ms")
	earlyDone := make(chan struct{})
	go func() { decide(e); close(earlyDone) }()
	time.Sleep(20 * time.Millisecond)
	decide(e) // open for 100 ms
	time.Sleep(130 * time.Millisecond)
	probeDone := make(chan struct{})
	go func() { decide(e); close(probeDone) }() // probe, held
	time.Sleep(20 * time.Millisecond)
	close(early) // late non-probe failure: ignored
	<-earlyDone
	close(probe) // probe fails: open for 200 ms (one doubling, not two)
	<-probeDone
	time.Sleep(260 * time.Millisecond)
	before := st.n()
	decide(e)
	if st.n() != before+1 {
		t.Fatalf("no probe 260 ms after the failed probe: the late non-probe failure also doubled the open time")
	}
}

// R2-2: production accepts a random key even when its text or bytes contain "dev"; it refuses committed dev markers
// in any case.
func TestFix2_R22_RandomKeyWithDevIsAccepted(t *testing.T) {
	var found string
	for i := 0; i < 200000 && found == ""; i++ {
		b := make([]byte, 32)
		_, _ = rand.Read(b)
		k := base64.StdEncoding.EncodeToString(b)
		if strings.Contains(k, "dev") {
			found = k
		}
	}
	if found == "" {
		found = base64.StdEncoding.EncodeToString([]byte("x-dev-key-made-of-random-bytes!!"))
	}
	if _, err := antibot.LoadConfig(env(map[string]string{"APP_ENV": "production", "ANTIBOT_HASH_KEY": found})); err != nil {
		t.Fatalf("production refused a random key %q containing dev: %v", found, err)
	}
	for _, k := range []string{
		"DEVANTIBOThashKeyLocalClusterOnlyNeverProdA=",
		base64.StdEncoding.EncodeToString([]byte("key-for-NeverProd-use-0123456789")),
		base64.StdEncoding.EncodeToString([]byte("eventflow-antibot-dev-key-000000")),
	} {
		if _, err := antibot.LoadConfig(env(map[string]string{"APP_ENV": "production", "ANTIBOT_HASH_KEY": k})); err == nil {
			t.Errorf("production accepted dev-marked key %q", k)
		}
	}
}
