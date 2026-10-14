package app

// EV-142 -- controller duoi quyen mot leader co fencing:
//   - BR-Q5: nhip admit phai song sot qua doi leader; lenh ghi de cua Ops (ke ca tam dung = 0)
//     khong duoc mat khi doi leader.
//   - Leader cu bi fencing chan phai dung ngay, khong tha them ai.
//   - Sau moi lan giam nhip, cho mot cua so de tin hieu tre kip phan anh (chong dao dong rang cua).

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/eventflow/eventflow/services/waitingroom/internal/domain"
)

// fakeStore gia lap StateStore (Redis): trang thai ma leader truoc de lai.
type fakeStore struct {
	mu sync.Mutex

	rate    float64
	rateOK  bool
	rateErr error

	override     float64
	overrideOn   bool
	overrideErr  error
	overrideSeen int
}

func (s *fakeStore) AdmitRate(context.Context, string) (float64, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.rate, s.rateOK, s.rateErr
}

func (s *fakeStore) RateOverride(context.Context, string) (float64, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.overrideSeen++
	return s.override, s.overrideOn, s.overrideErr
}

func (s *fakeStore) setOverride(on bool, rate float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.overrideOn, s.override = on, rate
}

// fencedQueue la fakeQueue ma lenh ghi nao cung bi fencing chan: mo phong leader cu tinh lai sau
// khi mot leader moi da len.
type fencedQueue struct {
	*fakeQueue
	failAdmit, failShuffle, failSetRate bool
}

func (f *fencedQueue) Admit(ctx context.Context, ev string, n int, ttl time.Duration) ([]string, error) {
	if f.failAdmit {
		f.fakeQueue.mu.Lock()
		f.fakeQueue.admits = append(f.fakeQueue.admits, admitCall{at: time.Now(), batch: n, ttl: ttl})
		f.fakeQueue.mu.Unlock()
		return nil, fmt.Errorf("admit batch: %w", domain.ErrFenced)
	}
	return f.fakeQueue.Admit(ctx, ev, n, ttl)
}

func (f *fencedQueue) Shuffle(ctx context.Context, ev string, seed int64) (int64, error) {
	if f.failShuffle {
		return 0, fmt.Errorf("shuffle lobby: %w", domain.ErrFenced)
	}
	return f.fakeQueue.Shuffle(ctx, ev, seed)
}

func (f *fencedQueue) SetAdmitRate(ctx context.Context, ev string, r float64) error {
	if f.failSetRate {
		return fmt.Errorf("set admit rate: %w", domain.ErrFenced)
	}
	return f.fakeQueue.SetAdmitRate(ctx, ev, r)
}

func leaderTestConfig() AdmitConfig {
	cfg := DefaultAdmitConfig("evt-1", time.Now().Add(-time.Minute))
	cfg.Tick = 5 * time.Millisecond
	return cfg
}

func runUntil(t *testing.T, c *AdmitController, d time.Duration) error {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), d)
	defer cancel()
	return c.Run(ctx)
}

// --- Fencing: leader cu dung ngay ----------------------------------------------------------

func TestRun_BiFencingKhiAdmit_ThoatNgayVaKhongThaThem(t *testing.T) {
	q := &fencedQueue{fakeQueue: newFakeQueue(), failAdmit: true}
	c := NewAdmitController(q, &fakeHealth{h: goodHealth}, leaderTestConfig(), discardLog())

	start := time.Now()
	err := runUntil(t, c, 2*time.Second)

	if !errors.Is(err, domain.ErrFenced) {
		t.Fatalf("Run tra ve %v, muon domain.ErrFenced (de leader loop thoai vi)", err)
	}
	if time.Since(start) > time.Second {
		t.Errorf("thoai vi mat %v, muon ngay lap tuc", time.Since(start))
	}
	if _, admits, _ := q.snapshot(); len(admits) != 1 {
		t.Errorf("sau khi bi chan van thu admit %d lan, muon dung 1", len(admits))
	}
}

func TestRun_BiFencingKhiGhiNhip_CungThoaiVi(t *testing.T) {
	q := &fencedQueue{fakeQueue: newFakeQueue(), failSetRate: true}
	c := NewAdmitController(q, &fakeHealth{h: goodHealth}, leaderTestConfig(), discardLog())

	if err := runUntil(t, c, 2*time.Second); !errors.Is(err, domain.ErrFenced) {
		t.Fatalf("Run tra ve %v, muon domain.ErrFenced", err)
	}
	if _, admits, _ := q.snapshot(); len(admits) != 0 {
		t.Errorf("khong ghi duoc nhip thi khong duoc tha ai, nhung da admit %d lan", len(admits))
	}
}

// Leader cu khong duoc xao tron lai hang cho cua leader moi (BR-Q1).
func TestRun_BiFencingKhiShuffle_KhongTiepTucVaoVongAdmit(t *testing.T) {
	q := &fencedQueue{fakeQueue: newFakeQueue(), failShuffle: true}
	c := NewAdmitController(q, &fakeHealth{h: goodHealth}, leaderTestConfig(), discardLog())

	if err := runUntil(t, c, 2*time.Second); !errors.Is(err, domain.ErrFenced) {
		t.Fatalf("Run tra ve %v, muon domain.ErrFenced", err)
	}
	if _, admits, _ := q.snapshot(); len(admits) != 0 {
		t.Errorf("bi fencing luc shuffle ma van admit %d lan", len(admits))
	}
}

// Loi thuong (khong phai fencing) KHONG lam chet vong dieu khien -- giu hanh vi cu.
func TestRun_LoiThuong_VanChayTiep(t *testing.T) {
	q := newFakeQueue()
	q.admitErrN = 3
	c := NewAdmitController(q, &fakeHealth{h: goodHealth}, leaderTestConfig(), discardLog())

	err := runUntil(t, c, 150*time.Millisecond)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Run tra ve %v, muon DeadlineExceeded (chay den het gio)", err)
	}
	if _, admits, _ := q.snapshot(); len(admits) < 5 {
		t.Errorf("chi admit %d lan: vong dieu khien bi dung sau loi thuong", len(admits))
	}
}

// --- Nhip song sot qua doi leader -----------------------------------------------------------

func TestRestore_TiepTucNhipCuCuaLeaderTruoc(t *testing.T) {
	cases := []struct {
		name  string
		store *fakeStore
		want  float64
	}{
		{"tiep tuc nhip da luu", &fakeStore{rate: 300, rateOK: true}, 300},
		{"nhip luu vuot tran bi keo ve tran", &fakeStore{rate: 9999, rateOK: true}, 500},
		{"chua tung ghi thi bat dau tu san", &fakeStore{}, 5},
		{"nhip luu 0 (rac) thi bat dau tu san", &fakeStore{rate: 0, rateOK: true}, 5},
		{"khong doc duoc thi bat dau tu san", &fakeStore{rateErr: errFake}, 5},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := NewAdmitController(newFakeQueue(), &fakeHealth{h: goodHealth}, leaderTestConfig(), discardLog()).
				WithStateStore(tc.store)
			c.restore(context.Background())
			if c.Rate() != tc.want {
				t.Fatalf("rate = %v, muon %v", c.Rate(), tc.want)
			}
		})
	}
}

// --- Ghi de cua Ops song sot qua doi leader (Q5.5) ------------------------------------------

// "Tam dung khan cap" = ghi de 0. Leader moi (controller moi, RAM trong) phai thay lenh do va
// KHONG tha ai, du suc khoe he thong dang rat dep.
func TestSyncOverride_TamDungKhanCapSongSotQuaLeaderMoi(t *testing.T) {
	store := &fakeStore{overrideOn: true, override: 0}
	q := newFakeQueue()
	c := NewAdmitController(q, &fakeHealth{h: goodHealth}, leaderTestConfig(), discardLog()).WithStateStore(store)

	for i := 0; i < 20; i++ {
		mustTick(t, c)
	}
	if c.Rate() != 0 {
		t.Fatalf("rate = %v, muon 0 (Ops dang tam dung)", c.Rate())
	}
	if _, admits, _ := q.snapshot(); len(admits) != 0 {
		t.Fatalf("Ops tam dung ma van admit %d lan", len(admits))
	}
}

func TestSyncOverride_GiaTriGhiDeBiChanTaiTran(t *testing.T) {
	store := &fakeStore{overrideOn: true, override: 99999}
	c := NewAdmitController(newFakeQueue(), &fakeHealth{h: goodHealth}, leaderTestConfig(), discardLog()).WithStateStore(store)
	mustTick(t, c)
	if c.Rate() != c.cfg.MaxRate {
		t.Fatalf("rate = %v, muon bi chan o MaxRate %v", c.Rate(), c.cfg.MaxRate)
	}
}

func TestSyncOverride_OpsBoGhiDe_TraQuyenChoAIMD(t *testing.T) {
	store := &fakeStore{overrideOn: true, override: 0}
	c := NewAdmitController(newFakeQueue(), &fakeHealth{h: goodHealth}, leaderTestConfig(), discardLog()).WithStateStore(store)
	mustTick(t, c)
	if c.Rate() != 0 {
		t.Fatalf("rate = %v, muon 0", c.Rate())
	}

	store.setOverride(false, 0)
	mustTick(t, c) // Ops bo ghi de: tick nay AIMD keo ve [Min, Max] roi tang
	mustTick(t, c)
	if c.Rate() <= c.cfg.MinRate {
		t.Fatalf("rate = %v: AIMD phai chay lai va tang tu san %v", c.Rate(), c.cfg.MinRate)
	}
}

// Lenh ghi de dat TRUC TIEP tren controller (SetRateOverride) khong duoc bi store "khong co ghi de"
// go di: store chi go co do chinh no dat.
func TestSyncOverride_StoreKhongGoGhiDeTrongRAM(t *testing.T) {
	store := &fakeStore{}
	c := NewAdmitController(newFakeQueue(), &fakeHealth{h: goodHealth}, leaderTestConfig(), discardLog()).WithStateStore(store)
	c.SetRateOverride(40)
	for i := 0; i < 5; i++ {
		mustTick(t, c)
	}
	if c.Rate() != 40 {
		t.Fatalf("rate = %v, muon giu 40 cua Ops", c.Rate())
	}
}

// Khong doc duoc store thi giu nguyen trang thai: dang tam dung thi van tam dung.
func TestSyncOverride_LoiDoc_GiuNguyenTrangThai(t *testing.T) {
	store := &fakeStore{overrideOn: true, override: 0}
	c := NewAdmitController(newFakeQueue(), &fakeHealth{h: goodHealth}, leaderTestConfig(), discardLog()).WithStateStore(store)
	mustTick(t, c)

	store.mu.Lock()
	store.overrideErr = errFake
	store.mu.Unlock()
	for i := 0; i < 5; i++ {
		mustTick(t, c)
	}
	if c.Rate() != 0 {
		t.Fatalf("rate = %v: loi doc store khong duoc tu mo van", c.Rate())
	}
}

// --- Cooldown chong dao dong ---------------------------------------------------------------

func TestAdjust_SauMoiLanGiamThiGiuMotCuaSo(t *testing.T) {
	c, _, _ := newTestController(badP99, nil)
	c.setRate(400)
	cd := c.cfg.DecreaseCooldown

	mustTick(t, c)
	first := c.Rate()
	if first >= 400 {
		t.Fatalf("lan dau: rate %v, muon giam tu 400", first)
	}
	for i := 0; i < cd; i++ {
		mustTick(t, c)
		if c.Rate() != first {
			t.Fatalf("tick %d trong cua so: rate %v, muon giu %v (khong giam chong len nhau)", i, c.Rate(), first)
		}
	}
	mustTick(t, c)
	if c.Rate() >= first {
		t.Fatalf("het cua so ma van qua tai: rate %v, muon giam tiep tu %v", c.Rate(), first)
	}
}

// O san roi thi khong co gi de cho: phuc hoi khong bi tri hoan.
func TestAdjust_ODuoiSan_KhongBatCuaSo(t *testing.T) {
	c, _, m := newTestController(badP99, nil)
	for i := 0; i < 10; i++ { // da o san
		mustTick(t, c)
	}
	if c.Rate() != c.cfg.MinRate {
		t.Fatalf("rate = %v, muon o san %v", c.Rate(), c.cfg.MinRate)
	}
	m.set(goodHealth, nil)
	mustTick(t, c)
	if c.Rate() <= c.cfg.MinRate {
		t.Fatalf("o san ma het suc ep: rate %v, muon tang ngay", c.Rate())
	}
}

// Mo phong dung tieu chi Jira: tiem do tre -> giam <= 10s; het do tre -> phuc hoi <= 60s;
// khong dao dong. 1 tick = 1 giay.
func TestAdjust_TieuChiNghiemThu_GiamTrong10sPhucHoiTrong60s(t *testing.T) {
	c, _, m := newTestController(goodHealth, nil)
	for i := 0; i < 200; i++ { // khoe lau: leo toi tran
		mustTick(t, c)
	}
	peak := c.Rate()
	if peak != c.cfg.MaxRate {
		t.Fatalf("khong leo toi tran: %v", peak)
	}

	m.set(badP99, nil) // tiem do tre
	var secsToHalve int
	for sec := 1; sec <= 10; sec++ {
		mustTick(t, c)
		if c.Rate() <= peak*0.7+0.001 {
			secsToHalve = sec
			break
		}
	}
	if secsToHalve == 0 {
		t.Fatalf("sau 10 giay do tre, rate van %v (dinh %v): khong giam", c.Rate(), peak)
	}

	// Giu do tre 90 giay roi bo: tu tran 500 ve san can 13 lan giam x 6 tick (1 giam + 5 cho) = 78 giay.
	for i := 0; i < 90; i++ {
		mustTick(t, c)
	}
	if c.Rate() != c.cfg.MinRate {
		t.Fatalf("sau 90 giay do tre rate = %v, muon ve san %v", c.Rate(), c.cfg.MinRate)
	}

	m.set(goodHealth, nil) // het do tre
	var recovered int
	for sec := 1; sec <= 60; sec++ {
		mustTick(t, c)
		if c.Rate() >= peak {
			recovered = sec
			break
		}
	}
	if recovered == 0 {
		t.Fatalf("sau 60 giay het do tre rate chi %v (dinh %v): phuc hoi qua cham", c.Rate(), peak)
	}
}

// Khong dao dong: khi suc ep dung yen, so lan DOI CHIEU (tang <-> giam) phai nho.
func TestAdjust_KhongDaoDongKhiSucEpKhongDoi(t *testing.T) {
	c, _, m := newTestController(goodHealth, nil)
	for i := 0; i < 200; i++ {
		mustTick(t, c)
	}
	m.set(badP99, nil)
	prev := c.Rate()
	var dir, flips int
	for i := 0; i < 120; i++ {
		mustTick(t, c)
		cur := c.Rate()
		d := 0
		switch {
		case cur > prev:
			d = 1
		case cur < prev:
			d = -1
		}
		if d != 0 {
			if dir != 0 && d != dir {
				flips++
			}
			dir = d
		}
		prev = cur
	}
	if flips > 0 {
		t.Fatalf("rate doi chieu %d lan duoi suc ep khong doi, muon 0", flips)
	}
}

// --- Metric ly do dieu chinh ------------------------------------------------------------------

type adjustSpy struct {
	spyAdmitMetrics
	mu      sync.Mutex
	actions []string
}

func (s *adjustSpy) AdmitAdjust(_ string, action string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.actions = append(s.actions, action)
}

func TestAdjust_XuatLyDoMoiTick(t *testing.T) {
	c, _, m := newTestController(badP99, nil)
	c.setRate(200)
	spy := &adjustSpy{}
	c.WithMetrics(spy)

	mustTick(t, c) // giam
	mustTick(t, c) // giu (cua so cho)
	m.set(goodHealth, nil)
	for i := 0; i < c.cfg.DecreaseCooldown; i++ {
		mustTick(t, c)
	}
	mustTick(t, c) // tang

	spy.mu.Lock()
	defer spy.mu.Unlock()
	if len(spy.actions) < 3 || spy.actions[0] != "decrease" || spy.actions[1] != "hold" ||
		spy.actions[len(spy.actions)-1] != "increase" {
		t.Fatalf("hanh dong = %v, muon bat dau decrease, hold va ket thuc increase", spy.actions)
	}
}
