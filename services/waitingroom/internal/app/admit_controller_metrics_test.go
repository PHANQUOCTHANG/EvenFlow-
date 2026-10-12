package app

import (
	"context"
	"sync"
	"testing"
	"time"
)

// Alert "admit_rate = 0 trong khi hang cho con nguoi" doc metric controller xuat
// ra, nen moi tick phai cong bo dung rate DANG DUNG de admit -- ke ca khi Ops
// ghi de.

type spyAdmitMetrics struct {
	mu    sync.Mutex
	calls []admitObs
}

type admitObs struct {
	event string
	rate  float64
	depth int64
}

func (s *spyAdmitMetrics) AdmitState(event string, rate float64, depth int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, admitObs{event, rate, depth})
}

func (s *spyAdmitMetrics) snapshot() []admitObs {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]admitObs(nil), s.calls...)
}

// depthQueue tra ve do sau hang cho tuy y (hoac loi) thay cho 0 cua fakeQueue.
type depthQueue struct {
	*fakeQueue
	depth int64
	err   error
}

func (d depthQueue) QueueDepth(context.Context, string) (int64, error) { return d.depth, d.err }

func TestTick_XuatRateVaDoSauHangCho(t *testing.T) {
	q := depthQueue{fakeQueue: newFakeQueue(), depth: 1234}
	cfg := DefaultAdmitConfig("evt-1", time.Now().Add(-time.Minute))
	spy := &spyAdmitMetrics{}
	c := NewAdmitController(q, &fakeHealth{h: goodHealth}, cfg, discardLog()).WithMetrics(spy)

	mustTick(t, c)

	got := spy.snapshot()
	if len(got) != 1 {
		t.Fatalf("muon 1 lan xuat metric moi tick, nhan %d", len(got))
	}
	if got[0].event != "evt-1" || got[0].depth != 1234 {
		t.Errorf("metric sai: %+v", got[0])
	}
	if got[0].rate != c.Rate() {
		t.Errorf("rate xuat ra (%v) phai la rate dang dung de admit (%v)", got[0].rate, c.Rate())
	}
}

func TestTick_OpsTamDung_MetricBaoRateBang0(t *testing.T) {
	q := depthQueue{fakeQueue: newFakeQueue(), depth: 500}
	cfg := DefaultAdmitConfig("evt-1", time.Now().Add(-time.Minute))
	spy := &spyAdmitMetrics{}
	c := NewAdmitController(q, &fakeHealth{h: goodHealth}, cfg, discardLog()).WithMetrics(spy)

	c.SetRateOverride(0)
	mustTick(t, c)

	got := spy.snapshot()
	if len(got) != 1 || got[0].rate != 0 || got[0].depth != 500 {
		t.Fatalf("tam dung phai hien ra la rate=0, depth=500 (de alert bat), nhan %+v", got)
	}
}

func TestTick_LoiDocDoSau_KhongLamHongVongAdmit(t *testing.T) {
	q := depthQueue{fakeQueue: newFakeQueue(), err: errFake}
	cfg := DefaultAdmitConfig("evt-1", time.Now().Add(-time.Minute))
	spy := &spyAdmitMetrics{}
	c := NewAdmitController(q, &fakeHealth{h: goodHealth}, cfg, discardLog()).WithMetrics(spy)

	mustTick(t, c) // mustTick fail neu tick tra loi

	if _, admits, _ := q.snapshot(); len(admits) == 0 {
		t.Error("loi do metric khong duoc chan viec admit")
	}
	if n := len(spy.snapshot()); n != 0 {
		t.Errorf("khong doc duoc do sau thi khong xuat so sai, nhung xuat %d lan", n)
	}
}

func TestTick_KhongGanMetrics_KhongDocThemTuQueue(t *testing.T) {
	q := depthQueue{fakeQueue: newFakeQueue(), err: errFake}
	cfg := DefaultAdmitConfig("evt-1", time.Now().Add(-time.Minute))
	c := NewAdmitController(q, &fakeHealth{h: goodHealth}, cfg, discardLog())

	mustTick(t, c)
	if _, admits, _ := q.snapshot(); len(admits) == 0 {
		t.Error("khong gan metrics van phai admit binh thuong")
	}
}
