//go:build integration

// Package leader chua test tich hop EV-142 tren Redis THAT: fencing cua cac script admit, bau
// leader giua nhieu controller, va nhip / ghi de song sot qua doi leader.
//
// Khong mock Redis o day: fencing nam trong Lua va phai chay tren chinh no.
package leader

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"

	libleader "github.com/eventflow/eventflow/libs/go/leader"
	"github.com/eventflow/eventflow/libs/go/testx"
	redisadapter "github.com/eventflow/eventflow/services/waitingroom/internal/adapter/redis"
	"github.com/eventflow/eventflow/services/waitingroom/internal/app"
	"github.com/eventflow/eventflow/services/waitingroom/internal/domain"
)

type env struct {
	rdb *redis.Client
	q   *redisadapter.Queue
	log *slog.Logger
}

func setup(t *testing.T) env {
	t.Helper()
	rdb := redis.NewClient(&redis.Options{Addr: testx.Redis(t)})
	t.Cleanup(func() { _ = rdb.Close() })
	q, err := redisadapter.NewQueue(context.Background(), rdb, time.Hour)
	if err != nil {
		t.Fatalf("khoi tao queue: %v", err)
	}
	return env{rdb: rdb, q: q, log: slog.New(slog.NewTextHandler(io.Discard, nil))}
}

func newEvent(t *testing.T) string {
	t.Helper()
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		t.Fatal(err)
	}
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	h := hex.EncodeToString(b[:])
	return h[0:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:32]
}

func key(ev, suffix string) string { return fmt.Sprintf("wr:{%s}:%s", ev, suffix) }

func (e env) seedQueue(t *testing.T, ev string, n int) {
	t.Helper()
	members := make([]redis.Z, n)
	for i := range members {
		members[i] = redis.Z{Score: float64(i + 1), Member: "tok-" + strconv.Itoa(i)}
	}
	if err := e.rdb.ZAdd(context.Background(), key(ev, "queue"), members...).Err(); err != nil {
		t.Fatal(err)
	}
}

func (e env) setEpoch(t *testing.T, ev string, epoch int64) {
	t.Helper()
	if err := e.rdb.Set(context.Background(), key(ev, "leader_epoch"), epoch, 0).Err(); err != nil {
		t.Fatal(err)
	}
}

func (e env) queueLen(t *testing.T, ev string) int64 {
	t.Helper()
	n, err := e.rdb.ZCard(context.Background(), key(ev, "queue")).Result()
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func (e env) admittedLen(t *testing.T, ev string) int64 {
	t.Helper()
	n, err := e.rdb.ZCard(context.Background(), key(ev, "admitted")).Result()
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func (e env) metaRate(t *testing.T, ev string) float64 {
	t.Helper()
	v, err := e.rdb.HGet(context.Background(), key(ev, "meta"), "admit_rate").Int64()
	if errors.Is(err, redis.Nil) {
		return -1
	}
	if err != nil {
		t.Fatal(err)
	}
	return float64(v) / 1000
}

type okHealth struct{}

func (okHealth) Health(context.Context, string) (app.Health, error) { return app.Health{}, nil }

// fixedRateConfig: nhip co dinh (Min = Max) de so nguoi duoc tha tinh duoc bang so hoc.
func fixedRateConfig(ev string, rate float64, tick time.Duration) app.AdmitConfig {
	cfg := app.DefaultAdmitConfig(ev, time.Now().Add(-time.Second))
	cfg.MinRate, cfg.MaxRate, cfg.Tick = rate, rate, tick
	return cfg
}

func eventually(t *testing.T, d time.Duration, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("qua %v ma %s chua xay ra", d, what)
}

// --- Fencing o tung script ----------------------------------------------------------------

// Epoch cu bi script tu choi va KHONG thay doi gi; epoch hien tai ghi binh thuong.
func TestTichHop_Fencing_AdmitCuBiChan(t *testing.T) {
	e := setup(t)
	ctx, ev := context.Background(), newEvent(t)
	e.seedQueue(t, ev, 10)
	e.setEpoch(t, ev, 2)

	_, err := e.q.Fenced(1).Admit(ctx, ev, 3, time.Minute)
	if !errors.Is(err, domain.ErrFenced) {
		t.Fatalf("leader cu: muon ErrFenced, nhan %v", err)
	}
	if e.queueLen(t, ev) != 10 || e.admittedLen(t, ev) != 0 {
		t.Fatalf("leader cu bi chan ma van doi hang cho: queue=%d admitted=%d",
			e.queueLen(t, ev), e.admittedLen(t, ev))
	}

	got, err := e.q.Fenced(2).Admit(ctx, ev, 3, time.Minute)
	if err != nil || len(got) != 3 {
		t.Fatalf("leader hien tai: %v, %v, muon 3 nguoi", got, err)
	}
	if e.queueLen(t, ev) != 7 || e.admittedLen(t, ev) != 3 {
		t.Fatalf("queue=%d admitted=%d, muon 7 / 3", e.queueLen(t, ev), e.admittedLen(t, ev))
	}
}

// Chua ai tung la leader (khong co epoch key) thi moi epoch deu bi chan: khong co duong tat de
// ghi ma khong qua bau cu.
func TestTichHop_Fencing_ChuaCoLeaderThiMoiEpochBiChan(t *testing.T) {
	e := setup(t)
	ctx, ev := context.Background(), newEvent(t)
	e.seedQueue(t, ev, 5)

	for _, epoch := range []int64{0, 1, 7} {
		if epoch == 0 {
			// epoch 0 khop "chua co key" -- khong phai leader hop le, nhung script chi so sanh so.
			// Bau cu luon cap epoch >= 1, nen noi goi that khong bao gio co epoch 0.
			continue
		}
		if _, err := e.q.Fenced(epoch).Admit(ctx, ev, 1, time.Minute); !errors.Is(err, domain.ErrFenced) {
			t.Fatalf("epoch %d khi chua co leader: muon ErrFenced, nhan %v", epoch, err)
		}
	}
	if e.queueLen(t, ev) != 5 {
		t.Fatalf("queue = %d, muon nguyen 5", e.queueLen(t, ev))
	}
}

// Leader cu khong duoc xao tron hang cho (BR-Q1); leader moi xao dung mot lan du co goi lai.
func TestTichHop_Fencing_ShuffleCuBiChan_MoiChiMotLan(t *testing.T) {
	e := setup(t)
	ctx, ev := context.Background(), newEvent(t)
	lobby := make([]any, 20)
	for i := range lobby {
		lobby[i] = "lob-" + strconv.Itoa(i)
	}
	if err := e.rdb.SAdd(ctx, key(ev, "lobby"), lobby...).Err(); err != nil {
		t.Fatal(err)
	}
	e.setEpoch(t, ev, 2)

	if _, err := e.q.Fenced(1).Shuffle(ctx, ev, 42); !errors.Is(err, domain.ErrFenced) {
		t.Fatalf("shuffle cua leader cu: muon ErrFenced, nhan %v", err)
	}
	if e.queueLen(t, ev) != 0 {
		t.Fatalf("leader cu bi chan ma van cap thu tu: queue = %d", e.queueLen(t, ev))
	}
	if v, _ := e.rdb.HGet(ctx, key(ev, "meta"), "shuffled").Result(); v == "1" {
		t.Fatal("co 'shuffled' bi dat boi leader cu")
	}

	if n, err := e.q.Fenced(2).Shuffle(ctx, ev, 42); err != nil || n != 20 {
		t.Fatalf("leader moi shuffle: n=%d err=%v, muon 20", n, err)
	}
	if n, err := e.q.Fenced(2).Shuffle(ctx, ev, 43); err != nil || n != 0 {
		t.Fatalf("shuffle lan hai: n=%d err=%v, muon 0 (no-op)", n, err)
	}
}

// Leader cu khong the de len nhip cua leader moi; nhip do doc lai duoc de tiep tuc.
func TestTichHop_Fencing_NhipCuBiChanVaDocLaiDuoc(t *testing.T) {
	e := setup(t)
	ctx, ev := context.Background(), newEvent(t)

	if _, ok, err := e.q.Fenced(1).AdmitRate(ctx, ev); err != nil || ok {
		t.Fatalf("chua tung ghi: ok=%v err=%v, muon ok=false", ok, err)
	}

	e.setEpoch(t, ev, 2)
	if err := e.q.Fenced(2).SetAdmitRate(ctx, ev, 120.5); err != nil {
		t.Fatal(err)
	}
	if err := e.q.Fenced(1).SetAdmitRate(ctx, ev, 5); !errors.Is(err, domain.ErrFenced) {
		t.Fatalf("leader cu ghi nhip: muon ErrFenced, nhan %v", err)
	}

	rate, ok, err := e.q.Fenced(3).AdmitRate(ctx, ev) // leader ke tiep doc lai
	if err != nil || !ok || rate != 120.5 {
		t.Fatalf("nhip = %v ok=%v err=%v, muon 120.5", rate, ok, err)
	}
}

func TestTichHop_Override_DatVaBo(t *testing.T) {
	e := setup(t)
	ctx, ev := context.Background(), newEvent(t)
	fq := e.q.Fenced(1)

	if _, active, err := fq.RateOverride(ctx, ev); err != nil || active {
		t.Fatalf("chua dat: active=%v err=%v", active, err)
	}
	if err := e.q.SetRateOverride(ctx, ev, 0); err != nil {
		t.Fatal(err)
	}
	if r, active, err := fq.RateOverride(ctx, ev); err != nil || !active || r != 0 {
		t.Fatalf("tam dung: rate=%v active=%v err=%v, muon 0/true", r, active, err)
	}
	if err := e.q.SetRateOverride(ctx, ev, 37.5); err != nil {
		t.Fatal(err)
	}
	if r, active, _ := fq.RateOverride(ctx, ev); !active || r != 37.5 {
		t.Fatalf("ghi de 37.5: rate=%v active=%v", r, active)
	}
	if err := e.q.ClearRateOverride(ctx, ev); err != nil {
		t.Fatal(err)
	}
	if _, active, _ := fq.RateOverride(ctx, ev); active {
		t.Fatal("da bo ghi de ma van active")
	}
}

// --- Nhieu controller, mot leader -------------------------------------------------------------

// runInstance chay mot instance controller day du (bau leader + vong admit) cho den khi ctx huy.
// leaderOf nhan id instance moi lan no thanh leader; maxActive la so leader dong thoi lon nhat.
type cluster struct {
	e         env
	ev        string
	cfg       app.AdmitConfig
	active    atomic.Int32
	maxActive atomic.Int32
	mu        sync.Mutex
	terms     []string // id cua tung nhiem ky, theo thu tu
	epochs    []int64
}

func (c *cluster) runInstance(t *testing.T, ctx context.Context, id string) {
	t.Helper()
	el, err := libleader.New(c.e.rdb, libleader.Config{
		LockKey:    key(c.ev, "leader"),
		EpochKey:   key(c.ev, "leader_epoch"),
		ID:         id,
		TTL:        600 * time.Millisecond,
		RenewEvery: 100 * time.Millisecond,
	}, c.e.log)
	if err != nil {
		t.Fatal(err)
	}
	_ = el.Run(ctx, func(ctx context.Context, epoch int64) error {
		n := c.active.Add(1)
		defer c.active.Add(-1)
		for {
			m := c.maxActive.Load()
			if n <= m || c.maxActive.CompareAndSwap(m, n) {
				break
			}
		}
		c.mu.Lock()
		c.terms = append(c.terms, id)
		c.epochs = append(c.epochs, epoch)
		c.mu.Unlock()

		fq := c.e.q.Fenced(epoch)
		return app.NewAdmitController(fq, okHealth{}, c.cfg, c.e.log).WithStateStore(fq).Run(ctx)
	})
}

func (c *cluster) lastTerm() (id string, epoch int64, ok bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.terms) == 0 {
		return "", 0, false
	}
	return c.terms[len(c.terms)-1], c.epochs[len(c.epochs)-1], true
}

// Hai controller cung chay: nhip tha chi bang MOT controller, khong gap doi. Day la dung loi ma
// EV-142 sinh ra de sua (BR-Q5): hai van mo song song thi Postgres nhan gap doi.
func TestTichHop_HaiController_ChiMotLeaderThaNguoi(t *testing.T) {
	e := setup(t)
	ev := newEvent(t)
	e.seedQueue(t, ev, 3000)

	const rate = 50.0
	cl := &cluster{e: e, ev: ev, cfg: fixedRateConfig(ev, rate, 100*time.Millisecond)}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	var wg sync.WaitGroup
	for _, id := range []string{"A", "B"} {
		wg.Add(1)
		go func() { defer wg.Done(); cl.runInstance(t, ctx, id) }()
	}
	wg.Wait()

	admitted := 3000 - e.queueLen(t, ev)
	t.Logf("3 giay, nhip %v/s: tha %d nguoi, toi da %d leader dong thoi", rate, admitted, cl.maxActive.Load())

	if cl.maxActive.Load() != 1 {
		t.Fatalf("co luc %d leader chay dong thoi, muon 1", cl.maxActive.Load())
	}
	// Mot leader: ~ 3s * 50 = 150 tru thoi gian bau cu dau tien. Hai leader se ~300.
	if admitted > 190 {
		t.Fatalf("tha %d nguoi trong 3 giay o nhip %v/s: vuot nhip cua MOT controller (hai van mo?)", admitted, rate)
	}
	if admitted < 80 {
		t.Fatalf("chi tha %d nguoi: leader khong chay het cong suat", admitted)
	}
	if got := e.admittedLen(t, ev); got != admitted {
		t.Fatalf("admitted=%d nhung queue giam %d: co nguoi duoc tha trung hoac mat", got, admitted)
	}
}

// Leader tat em: instance con lai len ngay voi epoch lon hon va TIEP TUC tha, khong ai bi tha
// trung, khong ai bi bo quen.
func TestTichHop_LeaderTatEm_NguoiKhacTiepQuan(t *testing.T) {
	e := setup(t)
	ev := newEvent(t)
	e.seedQueue(t, ev, 3000)

	cl := &cluster{e: e, ev: ev, cfg: fixedRateConfig(ev, 50, 100*time.Millisecond)}
	ctxA, cancelA := context.WithCancel(context.Background())
	ctxB, cancelB := context.WithCancel(context.Background())
	defer cancelA()
	defer cancelB()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() { defer wg.Done(); cl.runInstance(t, ctxA, "A") }()
	eventually(t, 3*time.Second, "A tro thanh leader", func() bool { _, _, ok := cl.lastTerm(); return ok })
	wg.Add(1)
	go func() { defer wg.Done(); cl.runInstance(t, ctxB, "B") }()

	eventually(t, 3*time.Second, "A bat dau tha nguoi", func() bool { return e.admittedLen(t, ev) > 10 })
	cancelA() // tat em
	before := e.admittedLen(t, ev)

	eventually(t, 3*time.Second, "B len lam leader", func() bool {
		id, _, _ := cl.lastTerm()
		return id == "B"
	})
	_, epoch, _ := cl.lastTerm()
	if epoch < 2 {
		t.Fatalf("epoch cua leader moi = %d, muon >= 2", epoch)
	}
	eventually(t, 3*time.Second, "B tiep tuc tha nguoi", func() bool { return e.admittedLen(t, ev) > before+20 })

	cancelB()
	wg.Wait()
	if got, admitted := e.admittedLen(t, ev), 3000-e.queueLen(t, ev); got != admitted {
		t.Fatalf("admitted=%d nhung queue giam %d: co nguoi tha trung hoac mat khi doi leader", got, admitted)
	}
	if cl.maxActive.Load() != 1 {
		t.Fatalf("co luc %d leader chay dong thoi", cl.maxActive.Load())
	}
}

// Kich ban cot loi: leader cu "dung hinh" (van chay controller voi epoch cu) khi leader moi da
// len. Lenh ghi tiep theo cua no bi FENCED: controller dung ngay va KHONG tha them ai.
func TestTichHop_LeaderCuDungHinh_ControllerDungNgayKhongThaThem(t *testing.T) {
	e := setup(t)
	ev := newEvent(t)
	e.seedQueue(t, ev, 5000)
	e.setEpoch(t, ev, 1)

	fq := e.q.Fenced(1)
	c := app.NewAdmitController(fq, okHealth{}, fixedRateConfig(ev, 100, 50*time.Millisecond), e.log).WithStateStore(fq)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- c.Run(ctx) }()

	eventually(t, 2*time.Second, "leader cu dang tha nguoi", func() bool { return e.admittedLen(t, ev) > 20 })

	e.setEpoch(t, ev, 2) // leader moi len (leader cu khong hay biet)
	bumped := time.Now()

	select {
	case err := <-done:
		if !errors.Is(err, domain.ErrFenced) {
			t.Fatalf("Run tra ve %v, muon domain.ErrFenced", err)
		}
		if d := time.Since(bumped); d > time.Second {
			t.Errorf("mat %v moi thoai vi, muon trong ~1 tick", d)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("controller cu van chay sau khi bi fencing")
	}

	frozen := e.admittedLen(t, ev)
	time.Sleep(300 * time.Millisecond)
	if e.admittedLen(t, ev) != frozen {
		t.Fatalf("leader cu van tha them nguoi sau khi bi fencing: %d -> %d", frozen, e.admittedLen(t, ev))
	}
	if r := e.metaRate(t, ev); r != 100 {
		t.Errorf("nhip trong meta = %v: leader cu khong duoc de len", r)
	}
}

// Q5.5 + Q5.6: doi leader giua cao diem khong keo nhip ve san, va lenh tam dung cua Ops van hieu
// luc voi leader moi.
func TestTichHop_DoiLeader_TiepTucNhipVaGiuTamDungCuaOps(t *testing.T) {
	e := setup(t)
	ctx, ev := context.Background(), newEvent(t)
	e.seedQueue(t, ev, 50_000)

	cfg := app.DefaultAdmitConfig(ev, time.Now().Add(-time.Second))
	cfg.Tick, cfg.MinRate, cfg.MaxRate, cfg.StepUp = 20*time.Millisecond, 5, 400, 25

	// Leader A leo nhip.
	e.setEpoch(t, ev, 1)
	fqA := e.q.Fenced(1)
	actx, cancelA := context.WithCancel(ctx)
	doneA := make(chan error, 1)
	go func() {
		doneA <- app.NewAdmitController(fqA, okHealth{}, cfg, e.log).WithStateStore(fqA).Run(actx)
	}()
	eventually(t, 5*time.Second, "leader A leo nhip >= 150", func() bool { return e.metaRate(t, ev) >= 150 })
	cancelA()
	<-doneA
	last := e.metaRate(t, ev)

	// Leader B len: phai tiep tuc tu nhip cua A, khong ve san. B chi tang 1/tick: neu khong khoi
	// phuc duoc thi no bat dau tu san 5 va phai mat hang tram tick moi cham toi `last`.
	cfgB := cfg
	cfgB.StepUp = 1
	e.setEpoch(t, ev, 2)
	fqB := e.q.Fenced(2)
	bctx, cancelB := context.WithCancel(ctx)
	defer cancelB()
	doneB := make(chan error, 1)
	go func() {
		doneB <- app.NewAdmitController(fqB, okHealth{}, cfgB, e.log).WithStateStore(fqB).Run(bctx)
	}()
	// Lan ghi dau tien cua B (gia tri doi khoi `last`) phai da la nhip cu cong them, khong phai san.
	eventually(t, 3*time.Second, "leader B ghi nhip dau tien", func() bool {
		return e.metaRate(t, ev) != last
	})
	if r := e.metaRate(t, ev); r < last {
		t.Fatalf("nhip dau tien cua leader moi = %v, thap hon nhip cu %v: khong tiep tuc tu nhip truoc", r, last)
	}

	// Ops tam dung khan cap: hieu luc trong <= 2 giay.
	if err := e.q.SetRateOverride(ctx, ev, 0); err != nil {
		t.Fatal(err)
	}
	eventually(t, 2*time.Second, "nhip ve 0", func() bool { return e.metaRate(t, ev) == 0 })
	time.Sleep(100 * time.Millisecond) // tick dang bay
	frozen := e.queueLen(t, ev)
	time.Sleep(300 * time.Millisecond)
	if e.queueLen(t, ev) != frozen {
		t.Fatalf("Ops tam dung ma van tha: queue %d -> %d", frozen, e.queueLen(t, ev))
	}

	// Leader moi nua (C) len khi dang tam dung: van phai dung.
	cancelB()
	<-doneB
	e.setEpoch(t, ev, 3)
	fqC := e.q.Fenced(3)
	cctx, cancelC := context.WithCancel(ctx)
	defer cancelC()
	doneC := make(chan error, 1)
	go func() {
		doneC <- app.NewAdmitController(fqC, okHealth{}, cfg, e.log).WithStateStore(fqC).Run(cctx)
	}()
	time.Sleep(400 * time.Millisecond)
	if e.queueLen(t, ev) != frozen {
		t.Fatalf("leader moi tu mo van khi Ops dang tam dung: queue %d -> %d", frozen, e.queueLen(t, ev))
	}

	// Ops bo ghi de: tha lai.
	if err := e.q.ClearRateOverride(ctx, ev); err != nil {
		t.Fatal(err)
	}
	eventually(t, 3*time.Second, "tha lai sau khi bo ghi de", func() bool { return e.queueLen(t, ev) < frozen })
	cancelC()
	<-doneC
}
