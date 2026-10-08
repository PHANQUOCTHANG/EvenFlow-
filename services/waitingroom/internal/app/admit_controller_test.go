package app

// Test viet tu nghiep vu (docs/01-nghiep-vu.md §2.2):
//   - BR-Q1: khong admit ai truoc sale_start; lottery (Shuffle) chay dung T0 truoc moi lan admit.
//   - BR-Q5: admit_rate tu dieu chinh theo tin hieu backpressure, luon trong [MinRate, MaxRate];
//            Ops ghi de thu cong.
//   - BR-Q7: suat admit co TTL 15 phut.

import (
	"context"
	"errors"
	"math"
	"sync"
	"testing"
	"time"
)

const waitLimit = 2 * time.Second // tran an toan cho cac test co goroutine; binh thuong xong trong vai ms

func mustTick(t *testing.T, c *AdmitController) {
	t.Helper()
	if err := c.tick(context.Background()); err != nil {
		t.Fatalf("tick: %v", err)
	}
}

// --- Cau hinh mac dinh --------------------------------------------------------

func TestDefaultAdmitConfig_AdmitTTLIs15Minutes(t *testing.T) {
	// BR-Q7: suat admit co TTL 15 phut.
	cfg := DefaultAdmitConfig("evt-1", time.Unix(0, 0))
	if cfg.AdmitTTL != 15*time.Minute {
		t.Errorf("AdmitTTL = %v, muon 15m (BR-Q7)", cfg.AdmitTTL)
	}
}

func TestDefaultAdmitConfig_CarriesEventAndSaleStart(t *testing.T) {
	start := time.Date(2026, 10, 8, 20, 0, 0, 0, time.UTC)
	cfg := DefaultAdmitConfig("evt-42", start)
	if cfg.EventID != "evt-42" || !cfg.SaleStartAt.Equal(start) {
		t.Errorf("cfg = %+v, muon EventID evt-42 va SaleStartAt %v", cfg, start)
	}
}

func TestDefaultAdmitConfig_RateBoundsAreSane(t *testing.T) {
	// BR-Q5: rate tu dieu chinh trong mot bien co nghia: san duong (hang cho khong dung han),
	// tran lon hon san, tang khi khoe (StepUp > 0), giam khi met (0 < Backoff < 1).
	cfg := DefaultAdmitConfig("evt-1", time.Now())
	if cfg.MinRate <= 0 {
		t.Errorf("MinRate = %v, phai > 0", cfg.MinRate)
	}
	if cfg.MaxRate <= cfg.MinRate {
		t.Errorf("MaxRate %v phai > MinRate %v", cfg.MaxRate, cfg.MinRate)
	}
	if cfg.StepUp <= 0 {
		t.Errorf("StepUp = %v, phai > 0", cfg.StepUp)
	}
	if cfg.Backoff <= 0 || cfg.Backoff >= 1 {
		t.Errorf("Backoff = %v, phai trong (0, 1)", cfg.Backoff)
	}
	if cfg.Tick <= 0 {
		t.Errorf("Tick = %v, phai > 0", cfg.Tick)
	}
	if cfg.P99Budget <= 0 || cfg.ErrorBudget <= 0 || cfg.PoolBudget <= 0 || cfg.PoolBudget > 1 {
		t.Errorf("nguong backpressure khong hop ly: %+v", cfg)
	}
}

// --- BR-Q5: tu dieu chinh admit_rate ---------------------------------------------

func TestController_StartsConservativelyAtFloor(t *testing.T) {
	// Chua do duoc gi ve tang duoi thi bat dau tu san, khong vot len.
	c, _, _ := newTestController(goodHealth, nil)
	if c.Rate() != c.cfg.MinRate {
		t.Errorf("rate ban dau = %v, muon MinRate %v", c.Rate(), c.cfg.MinRate)
	}
}

func TestTick_HealthySystemIncreasesRate(t *testing.T) {
	c, _, _ := newTestController(goodHealth, nil)
	before := c.Rate()
	mustTick(t, c)
	if after := c.Rate(); after <= before {
		t.Errorf("he thong khoe: rate %v -> %v, muon tang", before, after)
	}
}

func TestTick_SustainedHealthReachesButNeverExceedsMax(t *testing.T) {
	c, _, _ := newTestController(goodHealth, nil)
	prev := c.Rate()
	for i := 0; i < 200; i++ {
		mustTick(t, c)
		cur := c.Rate()
		if cur > c.cfg.MaxRate {
			t.Fatalf("tick %d: rate %v vuot MaxRate %v", i, cur, c.cfg.MaxRate)
		}
		if cur < prev {
			t.Fatalf("tick %d: he thong khoe ma rate giam %v -> %v", i, prev, cur)
		}
		prev = cur
	}
	if c.Rate() != c.cfg.MaxRate {
		t.Errorf("sau 200 tick khoe: rate = %v, muon cham tran %v", c.Rate(), c.cfg.MaxRate)
	}
}

func TestTick_BackpressureDecreasesRate(t *testing.T) {
	// Moi tin hieu backpressure rieng le (p99 cham, loi cao, pool DB gan day) deu phai lam giam rate.
	cases := map[string]Health{
		"p99 vuot ngan sach":   badP99,
		"ty le loi cao":        badErrors,
		"pool DB gan can kiet": badPool,
	}
	for name, h := range cases {
		t.Run(name, func(t *testing.T) {
			c, _, _ := newTestController(goodHealth, nil)
			c.SetRateOverride(300) // dua rate len cao de thay ro su suy giam
			before := c.Rate()
			c.metrics.(*fakeHealth).set(h, nil)
			mustTick(t, c)
			after := c.Rate()
			if after >= before {
				t.Errorf("%s: rate %v -> %v, muon giam", name, before, after)
			}
			if after < c.cfg.MinRate {
				t.Errorf("%s: rate %v duoi san %v", name, after, c.cfg.MinRate)
			}
		})
	}
}

func TestTick_SustainedBackpressureFloorsAtMinRateAndKeepsTrickling(t *testing.T) {
	// BR-Q5 + y do thiet ke: qua tai keo dai thi rate ve san, khong bao gio duoi san / ve 0,
	// va van tha nho giot de hang cho khong dung han.
	c, q, _ := newTestController(badP99, nil)
	c.SetRateOverride(c.cfg.MaxRate)
	prev := c.Rate()
	for i := 0; i < 100; i++ {
		mustTick(t, c)
		cur := c.Rate()
		if cur < c.cfg.MinRate {
			t.Fatalf("tick %d: rate %v duoi MinRate %v", i, cur, c.cfg.MinRate)
		}
		if cur > prev {
			t.Fatalf("tick %d: dang qua tai ma rate tang %v -> %v", i, prev, cur)
		}
		prev = cur
	}
	if c.Rate() != c.cfg.MinRate {
		t.Errorf("sau 100 tick qua tai: rate = %v, muon ve san %v", c.Rate(), c.cfg.MinRate)
	}
	_, admits, _ := q.snapshot()
	last := admits[len(admits)-1]
	if last.batch < 1 {
		t.Errorf("o san van phai tha it nhat 1 nguoi/tick, batch = %d", last.batch)
	}
}

func TestTick_UnknownHealthIsTreatedAsBackpressure(t *testing.T) {
	// Khong do duoc suc khoe tang duoi -> khong duoc tang rate "mu".
	c, _, _ := newTestController(goodHealth, errors.New("prometheus timeout"))
	c.SetRateOverride(200)
	before := c.Rate()
	mustTick(t, c)
	if after := c.Rate(); after >= before {
		t.Errorf("health loi: rate %v -> %v, muon giam (than trong)", before, after)
	}
}

func TestTick_RecoversAfterBackpressureClears(t *testing.T) {
	c, _, m := newTestController(badPool, nil)
	c.SetRateOverride(200)
	mustTick(t, c)
	low := c.Rate()
	m.set(goodHealth, nil)
	mustTick(t, c)
	if c.Rate() <= low {
		t.Errorf("het suc ep: rate %v -> %v, muon tang tro lai", low, c.Rate())
	}
}

func TestTick_RateAlwaysWithinBoundsUnderMixedSignals(t *testing.T) {
	// BR-Q5: du tin hieu dao chieu lien tuc, rate luon nam trong [MinRate, MaxRate].
	c, _, m := newTestController(goodHealth, nil)
	signals := []Health{goodHealth, goodHealth, badP99, goodHealth, badErrors, badPool, goodHealth}
	for i := 0; i < 500; i++ {
		var err error
		if i%13 == 0 {
			err = errFake
		}
		m.set(signals[i%len(signals)], err)
		mustTick(t, c)
		if r := c.Rate(); r < c.cfg.MinRate || r > c.cfg.MaxRate {
			t.Fatalf("tick %d: rate %v ngoai [%v, %v]", i, r, c.cfg.MinRate, c.cfg.MaxRate)
		}
	}
}

func TestTick_PublishesRateAndAdmitsThatManyPerSecondWith15mTTL(t *testing.T) {
	// BR-Q5: rate dang ap dung phai duoc cong bo (de client tinh ETA) va dung la so nguoi
	// duoc tha moi giay. BR-Q7: moi suat admit mang TTL 15 phut.
	c, q, _ := newTestController(goodHealth, nil)
	for i := 0; i < 5; i++ {
		mustTick(t, c)
		_, admits, rates := q.snapshot()
		published := rates[len(rates)-1]
		if published != c.Rate() {
			t.Errorf("tick %d: cong bo rate %v, controller dang dung %v", i, published, c.Rate())
		}
		got := admits[len(admits)-1]
		want := int(math.Round(published * c.cfg.Tick.Seconds()))
		if got.batch != want {
			t.Errorf("tick %d: batch = %d, muon %d (rate %v/s x tick %v)", i, got.batch, want, published, c.cfg.Tick)
		}
		if got.ttl != 15*time.Minute {
			t.Errorf("tick %d: TTL admit = %v, muon 15m (BR-Q7)", i, got.ttl)
		}
	}
}

func TestTick_ReportsQueueFailures(t *testing.T) {
	t.Run("cong bo rate loi", func(t *testing.T) {
		c, q, _ := newTestController(goodHealth, nil)
		q.setRateErrN = 1
		if err := c.tick(context.Background()); err == nil {
			t.Error("SetAdmitRate loi ma tick khong bao loi")
		}
	})
	t.Run("admit loi", func(t *testing.T) {
		c, q, _ := newTestController(goodHealth, nil)
		q.admitErrN = 1
		if err := c.tick(context.Background()); err == nil {
			t.Error("Admit loi ma tick khong bao loi")
		}
	})
}

func TestTick_ZeroRateAdmitsNobody(t *testing.T) {
	// Rate 0 (Ops tam dung) -> trong tick do khong duoc tha ai.
	cfg := DefaultAdmitConfig("evt-1", time.Now().Add(-time.Minute))
	cfg.MinRate = 0
	q := newFakeQueue()
	c := NewAdmitController(q, &fakeHealth{h: badP99}, cfg, discardLog())
	mustTick(t, c)
	if _, admits, _ := q.snapshot(); len(admits) != 0 {
		t.Errorf("rate 0 ma van admit %d lo", len(admits))
	}
}

// --- BR-Q5: Ops ghi de thu cong -------------------------------------------------

func TestSetRateOverride_TakesEffectImmediately(t *testing.T) {
	c, _, _ := newTestController(goodHealth, nil)
	c.SetRateOverride(42)
	if c.Rate() != 42 {
		t.Errorf("sau khi Ops ghi de 42: rate = %v", c.Rate())
	}
}

func TestSetRateOverride_ZeroPausesAdmission(t *testing.T) {
	// Ops phai dat duoc 0 de tam dung (khong bi ep len san).
	c, _, _ := newTestController(goodHealth, nil)
	c.SetRateOverride(0)
	if c.Rate() != 0 {
		t.Errorf("Ops ghi de 0: rate = %v, muon 0", c.Rate())
	}
}

func TestSetRateOverride_NeverNegativeNorAboveCeiling(t *testing.T) {
	// Rate am la vo nghia; tran MaxRate bao ve tang duoi.
	c, _, _ := newTestController(goodHealth, nil)
	c.SetRateOverride(-50)
	if c.Rate() < 0 {
		t.Errorf("ghi de -50: rate = %v am", c.Rate())
	}
	c.SetRateOverride(c.cfg.MaxRate * 10)
	if c.Rate() > c.cfg.MaxRate {
		t.Errorf("ghi de vuot tran: rate = %v > MaxRate %v", c.Rate(), c.cfg.MaxRate)
	}
}

func TestSetRateOverride_IsSafeConcurrentlyWithTicks(t *testing.T) {
	// Ops goi ghi de tu goroutine HTTP trong khi vong dieu khien dang chay (kiem bang -race).
	c, _, _ := newTestController(goodHealth, nil)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			c.SetRateOverride(float64(i))
			_ = c.Rate()
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			_ = c.tick(context.Background())
		}
	}()
	wg.Wait()
	if r := c.Rate(); r < 0 || r > c.cfg.MaxRate {
		t.Errorf("rate %v ngoai [0, %v]", r, c.cfg.MaxRate)
	}
}

// --- BR-Q1: vong dieu khien va T0 ------------------------------------------------

// fastConfig: tick 5ms, san 200/s => moi tick tha ~1 nguoi, de Run chay that ma test van nhanh.
func fastConfig(saleStart time.Time) AdmitConfig {
	cfg := DefaultAdmitConfig("evt-1", saleStart)
	cfg.Tick = 5 * time.Millisecond
	cfg.MinRate = 200
	cfg.MaxRate = 400
	return cfg
}

func runAsync(ctx context.Context, c *AdmitController) <-chan error {
	done := make(chan error, 1)
	go func() { done <- c.Run(ctx) }()
	return done
}

func waitDone(t *testing.T, done <-chan error) error {
	t.Helper()
	select {
	case err := <-done:
		return err
	case <-time.After(waitLimit):
		t.Fatal("Run khong dung sau khi context bi huy")
		return nil
	}
}

func TestRun_NobodyAdmittedBeforeSaleStart(t *testing.T) {
	// BR-Q1: truoc T0 khong ai co rank, khong xao tron, khong admit.
	q := newFakeQueue()
	c := NewAdmitController(q, &fakeHealth{h: goodHealth}, fastConfig(time.Now().Add(time.Hour)), discardLog())

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	err := waitDone(t, runAsync(ctx, c))
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("Run truoc T0 ket thuc voi %v, muon context.DeadlineExceeded", err)
	}
	shuffles, admits, rates := q.snapshot()
	if len(shuffles) != 0 || len(admits) != 0 || len(rates) != 0 {
		t.Errorf("truoc T0 ma da co: %d shuffle, %d admit, %d set-rate", len(shuffles), len(admits), len(rates))
	}
}

func TestRun_LotteryAtT0ThenAdmitsOnlyAfterSaleStart(t *testing.T) {
	// BR-Q1: dung T0 moi xao tron LOBBY mot lan, va moi lan admit deu sau T0 va sau lottery.
	saleStart := time.Now().Add(40 * time.Millisecond)
	q := newFakeQueue()
	c := NewAdmitController(q, &fakeHealth{h: goodHealth}, fastConfig(saleStart), discardLog())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := runAsync(ctx, c)

	for got := 0; got < 3; got++ {
		select {
		case <-q.admitted:
		case <-time.After(waitLimit):
			t.Fatal("qua T0 ma khong ai duoc admit")
		}
	}
	cancel()
	if err := waitDone(t, done); !errors.Is(err, context.Canceled) {
		t.Errorf("Run ket thuc voi %v, muon context.Canceled", err)
	}

	shuffles, admits, _ := q.snapshot()
	if len(shuffles) != 1 {
		t.Fatalf("lottery phai chay dung 1 lan tai T0, thuc te %d", len(shuffles))
	}
	if shuffles[0].at.Before(saleStart) {
		t.Errorf("lottery chay truoc T0 (%v truoc gio)", saleStart.Sub(shuffles[0].at))
	}
	for i, a := range admits {
		if a.at.Before(saleStart) {
			t.Errorf("admit #%d truoc T0", i)
		}
		if a.at.Before(shuffles[0].at) {
			t.Errorf("admit #%d truoc khi lottery xong -- rank chua duoc cap cong bang", i)
		}
		if a.ttl != 15*time.Minute {
			t.Errorf("admit #%d TTL = %v, muon 15m", i, a.ttl)
		}
	}
}

func TestRun_LotterySeedIsNotPredictable(t *testing.T) {
	// BR-Q1: ket qua xao tron khong doan truoc duoc -> hai lan chay khong dung chung seed.
	seeds := make(map[int64]bool)
	for i := 0; i < 5; i++ {
		q := newFakeQueue()
		q.shuffleErr = errFake // dung ngay sau lottery cho nhanh
		c := NewAdmitController(q, &fakeHealth{h: goodHealth}, fastConfig(time.Now().Add(-time.Second)), discardLog())
		_ = c.Run(context.Background())
		shuffles, _, _ := q.snapshot()
		if len(shuffles) != 1 {
			t.Fatalf("lan %d: %d shuffle", i, len(shuffles))
		}
		seeds[shuffles[0].seed] = true
	}
	if len(seeds) < 5 {
		t.Errorf("5 lan lottery chi co %d seed khac nhau -- seed doan truoc duoc", len(seeds))
	}
}

func TestRun_LotteryFailureStopsBeforeAnyAdmission(t *testing.T) {
	// BR-Q1: lottery that bai thi khong duoc admit theo thu tu vao (mat cong bang).
	q := newFakeQueue()
	q.shuffleErr = errFake
	c := NewAdmitController(q, &fakeHealth{h: goodHealth}, fastConfig(time.Now().Add(-time.Second)), discardLog())

	ctx, cancel := context.WithTimeout(context.Background(), waitLimit)
	defer cancel()
	err := c.Run(ctx)
	if !errors.Is(err, errFake) {
		t.Errorf("Run = %v, muon loi lottery", err)
	}
	if _, admits, _ := q.snapshot(); len(admits) != 0 {
		t.Errorf("lottery loi ma van admit %d lo", len(admits))
	}
}

func TestRun_StopsWhenContextCancelled(t *testing.T) {
	q := newFakeQueue()
	c := NewAdmitController(q, &fakeHealth{h: goodHealth}, fastConfig(time.Now().Add(-time.Second)), discardLog())
	ctx, cancel := context.WithCancel(context.Background())
	done := runAsync(ctx, c)
	select {
	case <-q.admitted:
	case <-time.After(waitLimit):
		t.Fatal("vong dieu khien khong chay")
	}
	cancel()
	if err := waitDone(t, done); !errors.Is(err, context.Canceled) {
		t.Errorf("Run = %v, muon context.Canceled", err)
	}
	_, before, _ := q.snapshot()
	// Sau khi Run tra ve thi khong con tick nao nua.
	time.Sleep(4 * c.cfg.Tick)
	if _, after, _ := q.snapshot(); len(after) != len(before) {
		t.Errorf("Run da dung ma van admit them %d lo", len(after)-len(before))
	}
}

func TestRun_TransientTickFailureDoesNotStopTheQueue(t *testing.T) {
	// Mot tick loi (Redis chap chon) khong duoc lam chet vong dieu khien -- hang cho dung han
	// con te hon admit lech nhip.
	q := newFakeQueue()
	q.admitErrN = 2
	q.setRateErrN = 2
	c := NewAdmitController(q, &fakeHealth{h: goodHealth}, fastConfig(time.Now().Add(-time.Second)), discardLog())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := runAsync(ctx, c)

	// 2 tick dau loi o SetAdmitRate, 2 tick sau loi o Admit; can thay them lan admit thanh cong.
	for got := 0; got < 4; got++ {
		select {
		case <-q.admitted:
		case <-time.After(waitLimit):
			t.Fatalf("vong dieu khien dung sau su co tam thoi (moi thay %d lan admit)", got)
		}
	}
	cancel()
	if err := waitDone(t, done); !errors.Is(err, context.Canceled) {
		t.Errorf("Run = %v, muon context.Canceled", err)
	}
}

func TestRun_OverrideFromOpsWhileRunningIsRaceFree(t *testing.T) {
	q := newFakeQueue()
	c := NewAdmitController(q, &fakeHealth{h: goodHealth}, fastConfig(time.Now().Add(-time.Second)), discardLog())
	ctx, cancel := context.WithCancel(context.Background())
	done := runAsync(ctx, c)
	for i := 0; i < 50; i++ {
		c.SetRateOverride(float64(200 + i))
		_ = c.Rate()
	}
	select {
	case <-q.admitted:
	case <-time.After(waitLimit):
		t.Fatal("vong dieu khien khong chay")
	}
	cancel()
	_ = waitDone(t, done)
}
