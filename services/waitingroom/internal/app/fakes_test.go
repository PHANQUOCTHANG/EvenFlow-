package app

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"time"
)

// Fake viet tay cho Admitter va HealthProbe -- khong can Redis, khong can thu vien mock.

type admitCall struct {
	at    time.Time
	batch int
	ttl   time.Duration
}

type shuffleCall struct {
	at   time.Time
	seed int64
}

type fakeQueue struct {
	mu sync.Mutex

	shuffles []shuffleCall
	admits   []admitCall
	rates    []float64

	shuffleErr error
	// admitErrN / setRateErrN: so lan dau tien tra loi (mo phong su co tam thoi).
	admitErrN   int
	setRateErrN int

	admitted chan struct{} // bao hieu moi lan Admit duoc goi (khong chan)
}

func newFakeQueue() *fakeQueue {
	return &fakeQueue{admitted: make(chan struct{}, 1024)}
}

func (f *fakeQueue) Admit(_ context.Context, _ string, batch int, ttl time.Duration) ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.admits = append(f.admits, admitCall{at: time.Now(), batch: batch, ttl: ttl})
	select {
	case f.admitted <- struct{}{}:
	default:
	}
	if f.admitErrN > 0 {
		f.admitErrN--
		return nil, errFake
	}
	out := make([]string, batch)
	for i := range out {
		out[i] = "tok"
	}
	return out, nil
}

func (f *fakeQueue) SetAdmitRate(_ context.Context, _ string, r float64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.rates = append(f.rates, r)
	if f.setRateErrN > 0 {
		f.setRateErrN--
		return errFake
	}
	return nil
}

func (f *fakeQueue) QueueDepth(context.Context, string) (int64, error) { return 0, nil }

func (f *fakeQueue) Shuffle(_ context.Context, _ string, seed int64) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.shuffles = append(f.shuffles, shuffleCall{at: time.Now(), seed: seed})
	if f.shuffleErr != nil {
		return 0, f.shuffleErr
	}
	return 1000, nil
}

func (f *fakeQueue) snapshot() (shuffles []shuffleCall, admits []admitCall, rates []float64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]shuffleCall(nil), f.shuffles...),
		append([]admitCall(nil), f.admits...),
		append([]float64(nil), f.rates...)
}

type fakeErr string

func (e fakeErr) Error() string { return string(e) }

const errFake = fakeErr("loi gia lap")

// fakeHealth tra ve tin hieu suc khoe co the doi giua cac tick.
type fakeHealth struct {
	mu  sync.Mutex
	h   Health
	err error
}

func (f *fakeHealth) Health(context.Context, string) (Health, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.h, f.err
}

func (f *fakeHealth) set(h Health, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.h, f.err = h, err
}

// Tin hieu dien hinh, dat ro ngoai/trong nguong mac dinh de khong phu thuoc bien.
var (
	goodHealth = Health{CheckoutP99: 100 * time.Millisecond, ErrorRate: 0.001, DBPoolUsage: 0.30}
	badP99     = Health{CheckoutP99: 3 * time.Second, ErrorRate: 0.001, DBPoolUsage: 0.30}
	badErrors  = Health{CheckoutP99: 100 * time.Millisecond, ErrorRate: 0.20, DBPoolUsage: 0.30}
	badPool    = Health{CheckoutP99: 100 * time.Millisecond, ErrorRate: 0.001, DBPoolUsage: 0.99}
)

func discardLog() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

// newTestController dung cau hinh mac dinh (Tick 1s) -- cac test goi tick() truc tiep
// nen khong phai cho dong ho that.
func newTestController(h Health, herr error) (*AdmitController, *fakeQueue, *fakeHealth) {
	q := newFakeQueue()
	m := &fakeHealth{h: h, err: herr}
	cfg := DefaultAdmitConfig("evt-1", time.Now().Add(-time.Minute))
	return NewAdmitController(q, m, cfg, discardLog()), q, m
}
