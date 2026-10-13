package worker

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/eventflow/eventflow/services/ticketing/internal/port"
)

// --- fakes ---

type fakeSource struct {
	rows []port.InventoryRow
	err  error
}

func (f *fakeSource) ListInventory(context.Context, string) ([]port.InventoryRow, error) {
	return f.rows, f.err
}

type fakeStates struct {
	m   map[string]port.HoldState
	err error
}

func (f *fakeStates) HoldStates(_ context.Context, ids []string) (map[string]port.HoldState, error) {
	if f.err != nil {
		return nil, f.err
	}
	out := map[string]port.HoldState{}
	for _, id := range ids {
		if s, ok := f.m[id]; ok {
			out[id] = s
		}
	}
	return out, nil
}

type adjustCall struct {
	tt            string
	bucket, delta int
}

// fakeLedger mo phong Redis: ton kho + hash hold. AdjustInventory ap dung that su (khong am).
type fakeLedger struct {
	mu       sync.Mutex
	inv      map[string]map[int]int
	holds    map[string]port.RedisHold
	locked   bool // true: khoa dang do instance khac giu
	snapErr  error
	adjErr   error
	adjusted []adjustCall
}

func (f *fakeLedger) Snapshot(_ context.Context, _ string, tts []string) (port.RedisSnapshot, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.snapErr != nil {
		return port.RedisSnapshot{}, f.snapErr
	}
	s := port.RedisSnapshot{Inventory: map[string]map[int]int{}, Holds: map[string]port.RedisHold{}}
	for _, tt := range tts {
		s.Inventory[tt] = map[int]int{}
		for b, v := range f.inv[tt] {
			s.Inventory[tt][b] = v
		}
	}
	for id, h := range f.holds {
		s.Holds[id] = h
	}
	return s, nil
}

func (f *fakeLedger) AdjustInventory(_ context.Context, _, tt string, bucket, delta int) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.adjErr != nil {
		return 0, f.adjErr
	}
	f.adjusted = append(f.adjusted, adjustCall{tt, bucket, delta})
	if f.inv[tt] == nil {
		f.inv[tt] = map[int]int{}
	}
	n := f.inv[tt][bucket] + delta
	if n < 0 {
		n = 0
	}
	f.inv[tt][bucket] = n
	return n, nil
}

func (f *fakeLedger) TryReconcileLock(context.Context, string, time.Duration) (bool, error) {
	return !f.locked, nil
}

type spyReconMetrics struct {
	mu        sync.Mutex
	drift     map[string]int
	corrected map[string]int
}

func newSpyReconMetrics() *spyReconMetrics {
	return &spyReconMetrics{drift: map[string]int{}, corrected: map[string]int{}}
}
func (s *spyReconMetrics) InventoryDrift(tt string, d int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.drift[tt] = d
}
func (s *spyReconMetrics) DriftCorrected(dir string, n int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.corrected[dir] += n
}

// --- khung test ---

const (
	ev = "ev-1"
	tt = "tt-1"
)

type rig struct {
	src    *fakeSource
	states *fakeStates
	led    *fakeLedger
	spy    *spyReconMetrics
	rec    *InventoryReconciler
}

// newRig: mot hang ve, hai bucket, Postgres moi bucket con 50, Redis khop.
func newRig() *rig {
	r := &rig{
		src: &fakeSource{rows: []port.InventoryRow{
			{TicketTypeID: tt, Bucket: 0, Available: 50},
			{TicketTypeID: tt, Bucket: 1, Available: 50},
		}},
		states: &fakeStates{m: map[string]port.HoldState{}},
		led: &fakeLedger{
			inv:   map[string]map[int]int{tt: {0: 50, 1: 50}},
			holds: map[string]port.RedisHold{},
		},
		spy: newSpyReconMetrics(),
	}
	r.rec = NewInventoryReconciler(r.src, r.states, r.led, DefaultReconcilerConfig([]string{ev}),
		slog.New(slog.NewTextHandler(io.Discard, nil))).WithMetrics(r.spy)
	return r
}

func (r *rig) cycle(t *testing.T) {
	t.Helper()
	if err := r.rec.Reconcile(context.Background(), ev); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
}

func (r *rig) redis(bucket int) int { return r.led.inv[tt][bucket] }

// --- AC: sua do lech trong <= 2 chu ky va metric ve 0 ---

func TestReconciler_RedisThieu_SuaSauHaiChuKy_MetricVe0(t *testing.T) {
	r := newRig()
	r.led.inv[tt][0] = 43 // lech -7: Redis thieu 7 ve (vd mat mot lan tra kho)

	r.cycle(t)
	if len(r.led.adjusted) != 0 {
		t.Fatalf("lan thay dau tien khong duoc sua, nhung da sua: %v", r.led.adjusted)
	}
	if r.spy.drift[tt] != 7 {
		t.Fatalf("metric sau chu ky 1 = %d, muon 7", r.spy.drift[tt])
	}

	r.cycle(t)
	if r.redis(0) != 50 {
		t.Fatalf("redis bucket 0 = %d sau chu ky 2, muon 50", r.redis(0))
	}
	if r.spy.drift[tt] != 0 {
		t.Fatalf("metric sau khi sua = %d, muon 0", r.spy.drift[tt])
	}
	if r.spy.corrected["refill"] != 7 {
		t.Fatalf("refill = %d, muon 7", r.spy.corrected["refill"])
	}

	r.cycle(t)
	if len(r.led.adjusted) != 1 {
		t.Fatalf("sau khi khop khong duoc sua them: %v", r.led.adjusted)
	}
}

func TestReconciler_RedisPhong_BotDi(t *testing.T) {
	r := newRig()
	r.led.inv[tt][1] = 58 // +8: Redis phong

	r.cycle(t)
	r.cycle(t)

	if r.redis(1) != 50 {
		t.Fatalf("redis bucket 1 = %d, muon 50", r.redis(1))
	}
	if r.spy.corrected["drain"] != 8 {
		t.Fatalf("drain = %d, muon 8", r.spy.corrected["drain"])
	}
}

func TestReconciler_RedisPhongKhongBaoGioXuongAm(t *testing.T) {
	r := newRig()
	r.src.rows = []port.InventoryRow{{TicketTypeID: tt, Bucket: 0, Available: 0}}
	r.led.inv[tt] = map[int]int{0: 3}

	r.cycle(t)
	r.cycle(t)

	if r.redis(0) != 0 {
		t.Fatalf("redis = %d, muon 0", r.redis(0))
	}
}

// --- hold dang bay khong phai la lech ---

func TestReconciler_HoldDangBay_KhongPhaiLech(t *testing.T) {
	r := newRig()
	// Redis da tru 3 cho hold h1; Postgres chua commit (khong co trong states).
	r.led.inv[tt][0] = 47
	r.led.holds["h1"] = port.RedisHold{TicketTypeID: tt, Bucket: 0, Quantity: 3}

	r.cycle(t)
	r.cycle(t)
	r.cycle(t)

	if len(r.led.adjusted) != 0 {
		t.Fatalf("hold dang bay bi coi la lech: %v", r.led.adjusted)
	}
	if r.spy.drift[tt] != 0 {
		t.Fatalf("metric = %d, muon 0", r.spy.drift[tt])
	}
}

func TestReconciler_HoldDaGhiPostgres_HaiTangCungTru(t *testing.T) {
	r := newRig()
	// Postgres da tru 2 (available=48), Redis cung tru 2.
	r.src.rows[0].Available = 48
	r.led.inv[tt][0] = 48
	r.led.holds["h1"] = port.RedisHold{TicketTypeID: tt, Bucket: 0, Quantity: 2}
	r.states.m["h1"] = port.HoldState{}

	r.cycle(t)
	r.cycle(t)

	if len(r.led.adjusted) != 0 || r.spy.drift[tt] != 0 {
		t.Fatalf("adjusted=%v drift=%d", r.led.adjusted, r.spy.drift[tt])
	}
}

func TestReconciler_PostgresDaTraRedisChuaKip_KhongPhaiLech(t *testing.T) {
	r := newRig()
	// Sweeper da tra kho o Postgres (available 50) nhung Redis chua release: Redis con thap 2.
	r.led.inv[tt][0] = 48
	r.led.holds["h1"] = port.RedisHold{TicketTypeID: tt, Bucket: 0, Quantity: 2}
	r.states.m["h1"] = port.HoldState{Released: true}

	r.cycle(t)
	r.cycle(t)

	if len(r.led.adjusted) != 0 {
		t.Fatalf("hold cho release bi coi la lech: %v", r.led.adjusted)
	}
}

func TestReconciler_HoldDaConsume_KhongPhaiLech(t *testing.T) {
	r := newRig()
	r.src.rows[0].Available = 49
	r.led.inv[tt][0] = 49
	r.led.holds["h1"] = port.RedisHold{TicketTypeID: tt, Bucket: 0, Quantity: 1}
	r.states.m["h1"] = port.HoldState{Consumed: true}

	r.cycle(t)
	r.cycle(t)

	if len(r.led.adjusted) != 0 {
		t.Fatalf("adjusted=%v", r.led.adjusted)
	}
}

func TestReconciler_LechThatCongHoldDangBay_ChiSuaPhanLech(t *testing.T) {
	r := newRig()
	// Hold dang bay 3 (Redis 47 la dung) cong them mat 5 ve that -> Redis 42.
	r.led.inv[tt][0] = 42
	r.led.holds["h1"] = port.RedisHold{TicketTypeID: tt, Bucket: 0, Quantity: 3}

	r.cycle(t)
	r.cycle(t)

	if r.redis(0) != 47 {
		t.Fatalf("redis = %d, muon 47 (50 tru 3 hold dang bay)", r.redis(0))
	}
}

// --- nhieu thoang qua khong duoc sua ---

func TestReconciler_NhieuThoangQua_KhongBaoGioDuocSua(t *testing.T) {
	r := newRig()

	r.led.inv[tt][0] = 49 // thay lech -1 o chu ky 1
	r.cycle(t)
	r.led.inv[tt][0] = 50 // chu ky 2 da khop
	r.cycle(t)
	r.led.inv[tt][0] = 49 // lai lech -1 thoang qua o chu ky 3
	r.cycle(t)
	r.led.inv[tt][0] = 50
	r.cycle(t)

	if len(r.led.adjusted) != 0 {
		t.Fatalf("nhieu thoang qua bi sua: %v", r.led.adjusted)
	}
}

func TestReconciler_DoiDau_KhongSua(t *testing.T) {
	r := newRig()

	r.led.inv[tt][0] = 48 // -2
	r.cycle(t)
	r.led.inv[tt][0] = 52 // +2
	r.cycle(t)

	if len(r.led.adjusted) != 0 {
		t.Fatalf("doi dau van bi sua: %v", r.led.adjusted)
	}
}

func TestReconciler_HaiLanLechKhacNhau_SuaPhanChacChan(t *testing.T) {
	r := newRig()

	r.led.inv[tt][0] = 42 // -8
	r.cycle(t)
	r.led.inv[tt][0] = 43 // -7 (mot hold bay vua tra ve 1)
	r.cycle(t)

	// Chi sua min(8,7)=7 -> Redis 50; phan thua (neu con) de chu ky sau.
	if r.redis(0) != 50 {
		t.Fatalf("redis = %d, muon 50", r.redis(0))
	}
	if got := r.led.adjusted[0].delta; got != 7 {
		t.Fatalf("delta = %d, muon 7 (nho hon trong hai lan)", got)
	}
}

// --- an toan van hanh ---

func TestReconciler_KhoaDoiSoatDoInstanceKhac_BoQua(t *testing.T) {
	r := newRig()
	r.led.inv[tt][0] = 40
	r.led.locked = true

	r.cycle(t)
	r.cycle(t)

	if len(r.led.adjusted) != 0 {
		t.Fatalf("khong giu khoa nhung van sua: %v", r.led.adjusted)
	}
}

func TestReconciler_SuKienKhongCoTonKho_KhongSuaGi(t *testing.T) {
	r := newRig()
	r.src.rows = nil
	r.led.inv[tt][0] = 7

	r.cycle(t)
	r.cycle(t)

	if len(r.led.adjusted) != 0 || r.redis(0) != 7 {
		t.Fatalf("adjusted=%v redis=%d", r.led.adjusted, r.redis(0))
	}
}

func TestReconciler_LoiDocRedis_KhongSuaVaTraLoi(t *testing.T) {
	r := newRig()
	r.led.inv[tt][0] = 40
	r.led.snapErr = errors.New("redis timeout")

	if err := r.rec.Reconcile(context.Background(), ev); err == nil {
		t.Fatal("muon loi, nhan nil")
	}
	if len(r.led.adjusted) != 0 {
		t.Fatalf("adjusted=%v", r.led.adjusted)
	}
}

func TestReconciler_LoiDocPostgres_KhongSua(t *testing.T) {
	r := newRig()
	r.led.inv[tt][0] = 40
	r.states.err = errors.New("postgres chet")

	r.cycle2err(t)
	r.cycle2err(t)

	if len(r.led.adjusted) != 0 {
		t.Fatalf("adjusted=%v", r.led.adjusted)
	}
}

func (r *rig) cycle2err(t *testing.T) {
	t.Helper()
	if err := r.rec.Reconcile(context.Background(), ev); err == nil {
		t.Fatal("muon loi, nhan nil")
	}
}

func TestReconciler_LoiSua_GiuDoLechDeThuLai(t *testing.T) {
	r := newRig()
	r.led.inv[tt][0] = 44 // -6
	r.cycle(t)
	r.led.adjErr = errors.New("redis timeout")
	r.cycle(t) // sua that bai
	if r.spy.drift[tt] != 6 {
		t.Fatalf("metric = %d, muon 6 (chua sua duoc)", r.spy.drift[tt])
	}

	r.led.adjErr = nil
	r.cycle(t) // thu lai thanh cong, vi do lech van duoc ghi nho
	if r.redis(0) != 50 {
		t.Fatalf("redis = %d, muon 50", r.redis(0))
	}
}

func TestReconciler_ReconcileAll_LoiMotSuKienKhongChanSuKienKhac(t *testing.T) {
	r := newRig()
	r.rec.cfg.EventIDs = []string{"ev-bad", ev}
	calls := 0
	src := &perEventSource{inner: r.src, fail: "ev-bad", calls: &calls}
	r.rec.src = src
	r.led.inv[tt][0] = 40

	r.rec.ReconcileAll(context.Background())
	r.rec.ReconcileAll(context.Background())

	if r.redis(0) != 50 {
		t.Fatalf("su kien tot khong duoc sua vi su kien loi: redis=%d", r.redis(0))
	}
}

type perEventSource struct {
	inner *fakeSource
	fail  string
	calls *int
}

func (p *perEventSource) ListInventory(ctx context.Context, eventID string) ([]port.InventoryRow, error) {
	*p.calls++
	if eventID == p.fail {
		return nil, errors.New("loi su kien")
	}
	return p.inner.ListInventory(ctx, eventID)
}
