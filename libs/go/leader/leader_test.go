package leader

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// Test vong doi leader bang fake store: nghiep vu o day la "khi nao thi leader phai thoai vi",
// khong phai Lua (Lua duoc test tren Redis that o integration_test.go).

type fakeStore struct {
	mu sync.Mutex

	holder   string // khoa dang do ai giu
	epoch    int64
	renewErr error // != nil: moi lan renew loi nay
	released []string

	acquires, renews atomic.Int32
}

func (f *fakeStore) acquire(_ context.Context, id string, _ time.Duration) (int64, error) {
	f.acquires.Add(1)
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.holder != "" && f.holder != id {
		return 0, nil
	}
	if f.holder == "" {
		f.epoch++
	}
	f.holder = id
	return f.epoch, nil
}

func (f *fakeStore) renew(_ context.Context, id string, _ time.Duration) (bool, error) {
	f.renews.Add(1)
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.renewErr != nil {
		return false, f.renewErr
	}
	return f.holder == id, nil
}

func (f *fakeStore) release(_ context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.released = append(f.released, id)
	if f.holder == id {
		f.holder = ""
	}
	return nil
}

func (f *fakeStore) set(fn func(*fakeStore)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	fn(f)
}

func newTestElector(st store) *Elector {
	return &Elector{
		cfg: Config{LockKey: "l", EpochKey: "e", ID: "me", TTL: 200 * time.Millisecond, RenewEvery: 20 * time.Millisecond},
		st:  st,
		log: slog.New(slog.NewTextHandler(io.Discard, nil)),
		now: time.Now,
	}
}

func within(t *testing.T, d time.Duration, what string, ch <-chan struct{}) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(d):
		t.Fatalf("qua %v ma %s chua xay ra", d, what)
	}
}

func TestConfigValidate(t *testing.T) {
	ok := Config{LockKey: "l", EpochKey: "e", ID: "i", TTL: 5 * time.Second, RenewEvery: time.Second}
	if err := ok.validate(); err != nil {
		t.Fatalf("cau hinh hop le bi tu choi: %v", err)
	}
	cases := map[string]func(*Config){
		"thieu khoa":               func(c *Config) { c.LockKey = "" },
		"thieu epoch key":          func(c *Config) { c.EpochKey = "" },
		"thieu id":                 func(c *Config) { c.ID = "" },
		"ttl khong duong":          func(c *Config) { c.TTL = 0 },
		"gia han khong duong":      func(c *Config) { c.RenewEvery = 0 },
		"gia han qua thua > ttl/2": func(c *Config) { c.RenewEvery = 3 * time.Second },
	}
	for name, mut := range cases {
		t.Run(name, func(t *testing.T) {
			c := ok
			mut(&c)
			if err := c.validate(); err == nil {
				t.Fatalf("cau hinh sai phai bi tu choi: %+v", c)
			}
		})
	}
}

// Khoa dang do nguoi khac giu thi KHONG chay viec cua leader.
func TestRun_KhongLaLeaderThiKhongChayViec(t *testing.T) {
	st := &fakeStore{holder: "nguoi-khac", epoch: 7}
	e := newTestElector(st)
	var ran atomic.Bool

	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	_ = e.Run(ctx, func(context.Context, int64) error { ran.Store(true); return nil })

	if ran.Load() {
		t.Fatal("khong phai leader ma van chay viec cua leader")
	}
	if st.acquires.Load() < 2 {
		t.Errorf("phai thu lay khoa dinh ky, chi thu %d lan", st.acquires.Load())
	}
}

// Lay duoc khoa thi nhan dung epoch moi; epoch tang moi nhiem ky.
func TestRun_NhanEpochTangDanMoiNhiemKy(t *testing.T) {
	st := &fakeStore{}
	e := newTestElector(st)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var mu sync.Mutex
	var epochs []int64
	_ = e.Run(ctx, func(_ context.Context, epoch int64) error {
		mu.Lock()
		epochs = append(epochs, epoch)
		n := len(epochs)
		mu.Unlock()
		if n == 3 {
			cancel()
		}
		return nil // het viec: nhiem ky ket thuc, nha khoa, tranh lai
	})

	mu.Lock()
	defer mu.Unlock()
	if len(epochs) != 3 || epochs[0] != 1 || epochs[1] != 2 || epochs[2] != 3 {
		t.Fatalf("epoch = %v, muon [1 2 3]", epochs)
	}
	st.mu.Lock()
	defer st.mu.Unlock()
	if len(st.released) < 3 {
		t.Errorf("moi nhiem ky ket thuc phai nha khoa, chi nha %d lan", len(st.released))
	}
}

// Khoa bi nguoi khac chiem (renew tra false): ctx cua viec phai bi huy NGAY, va KHONG nha khoa
// (khoa luc nay khong con la cua minh).
func TestRun_MatKhoaThiHuyViecVaKhongNhaKhoaCuaNguoiKhac(t *testing.T) {
	st := &fakeStore{}
	e := newTestElector(st)
	started, lost := make(chan struct{}), make(chan struct{})
	var startOnce, lostOnce sync.Once

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		_ = e.Run(ctx, func(c context.Context, _ int64) error {
			startOnce.Do(func() { close(started) })
			<-c.Done()
			lostOnce.Do(func() { close(lost) })
			return c.Err()
		})
	}()

	within(t, time.Second, "bat dau lam leader", started)
	st.set(func(f *fakeStore) { f.holder = "nguoi-moi" }) // nguoi khac len sau khi lease het han
	within(t, time.Second, "huy viec khi mat khoa", lost)

	cancel()
	time.Sleep(30 * time.Millisecond)
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.holder != "nguoi-moi" {
		t.Fatalf("khoa cua nguoi moi bi nha nham, holder = %q", st.holder)
	}
}

// Redis loi thoang qua (mot hai lan gia han) thi KHONG thoai vi; loi keo dai toi gan het lease thi
// phai thoai vi, vi sau thoi diem do nguoi khac co the da len.
func TestRun_LoiGiaHan_ThoangQuaThiGiu_KeoDaiThiThoaiVi(t *testing.T) {
	st := &fakeStore{}
	e := newTestElector(st)
	started, lost := make(chan struct{}), make(chan struct{})
	var startOnce, lostOnce sync.Once

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		_ = e.Run(ctx, func(c context.Context, _ int64) error {
			startOnce.Do(func() { close(started) })
			<-c.Done()
			lostOnce.Do(func() { close(lost) })
			return nil
		})
	}()
	within(t, time.Second, "bat dau lam leader", started)

	// Loi trong 60ms (< TTL - RenewEvery = 180ms): van la leader.
	st.set(func(f *fakeStore) { f.renewErr = errors.New("redis timeout") })
	time.Sleep(60 * time.Millisecond)
	select {
	case <-lost:
		t.Fatal("loi thoang qua khong duoc lam mat quyen leader")
	default:
	}
	st.set(func(f *fakeStore) { f.renewErr = nil })
	time.Sleep(60 * time.Millisecond) // gia han thanh cong tro lai: dong ho lastOK duoc dat lai

	// Loi keo dai: phai thoai vi trong khoang TTL.
	st.set(func(f *fakeStore) { f.renewErr = errors.New("redis timeout") })
	within(t, time.Second, "thoai vi khi loi keo dai", lost)
}

// Tat tien trinh (ctx cha bi huy) thi huy viec, doi viec xong, roi nha khoa de leader moi len ngay.
func TestRun_TatEm_NhaKhoaDeFailoverNgay(t *testing.T) {
	st := &fakeStore{}
	e := newTestElector(st)
	started := make(chan struct{})
	var startOnce sync.Once

	ctx, cancel := context.WithCancel(context.Background())
	finished := make(chan error, 1)
	go func() {
		finished <- e.Run(ctx, func(c context.Context, _ int64) error {
			startOnce.Do(func() { close(started) })
			<-c.Done()
			return c.Err()
		})
	}()
	within(t, time.Second, "bat dau lam leader", started)
	cancel()

	select {
	case err := <-finished:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Run tra ve %v, muon context.Canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Run khong tra ve sau khi ctx bi huy")
	}
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.holder != "" {
		t.Fatalf("tat em phai nha khoa, holder = %q", st.holder)
	}
}

func TestIsFenced(t *testing.T) {
	if !IsFenced(errors.New("FENCED")) || !IsFenced(ErrFenced) {
		t.Error("phai nhan ra FENCED va ErrFenced")
	}
	if IsFenced(nil) || IsFenced(errors.New("loi khac")) {
		t.Error("loi khac khong duoc coi la FENCED")
	}
}
