package main

import (
	"testing"
	"time"

	"github.com/eventflow/eventflow/libs/go/loadsignal"
	"github.com/eventflow/eventflow/services/ticketing/internal/port"
)

type nopHoldMetrics struct{ attempts int }

func (n *nopHoldMetrics) HoldAttempt(string, time.Duration) { n.attempts++ }
func (n *nopHoldMetrics) OversellGuardRejected()            {}

// Tin hieu suc khoe chi tinh yeu cau DA cham tang duoi; loi he thong moi la loi.
func TestSignalMetrics_ChiTinhYeuCauChamTangDuoi(t *testing.T) {
	rec := loadsignal.NewRecorder()
	inner := &nopHoldMetrics{}
	m := signalMetrics{HoldMetrics: inner, rec: rec}

	m.HoldAttempt(port.OutcomeOK, 20*time.Millisecond)
	m.HoldAttempt(port.OutcomeSoldOut, 10*time.Millisecond)
	m.HoldAttempt(port.OutcomeLimit, 10*time.Millisecond)
	m.HoldAttempt(port.OutcomeError, 3*time.Second)
	m.HoldAttempt(port.OutcomeNotAdmitted, time.Millisecond) // bi tu choi truoc khi cham DB

	s := rec.Snapshot("p", 0)
	if s.Requests != 4 {
		t.Errorf("requests = %d, muon 4 (khong tinh not_admitted)", s.Requests)
	}
	if s.Errors != 1 {
		t.Errorf("errors = %d, muon 1 (chi loi he thong; het ve va vuot han muc khong phai loi)", s.Errors)
	}
	if s.P99Ms != 3000 {
		t.Errorf("p99 = %v ms, muon 3000", s.P99Ms)
	}
	if inner.attempts != 5 {
		t.Errorf("metric Prometheus phai nhan du 5 lan, nhan %d", inner.attempts)
	}
}
