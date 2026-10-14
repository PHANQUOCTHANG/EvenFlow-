package app

import (
	"context"
	"errors"
	"log/slog"
	"math"
	"math/rand"
	"sync/atomic"
	"time"

	"github.com/eventflow/eventflow/services/waitingroom/internal/domain"
)

// AdmitController quyet dinh moi giay duoc tha bao nhieu nguoi vao mua ve.
//
// Day la trai tim cua chien luoc chiu tai. Y tuong: KHONG cho phep tai duoi
// (checkout + Postgres) bi quyet dinh boi so nguoi dang xep hang. Thay vao do,
// do thong luong thuc te cua tang duoi va chi tha vao dung ngan ay nguoi.
//
// Ket qua: Postgres luon thay mot dong tai gan nhu hang so, du hang cho co
// 30 nghin hay 3 trieu nguoi. Spike bi hap thu tron ven o tang Redis.
//
// Thuat toan la AIMD (Additive Increase, Multiplicative Decrease) -- cung ho
// voi TCP congestion control, va vi ly do tuong tu: tang cham de do tim tran,
// giam nhanh de khong lam sap tang duoi.
type AdmitController struct {
	queue   Admitter
	metrics HealthProbe
	obs     AdmitMetrics // nil = khong xuat metric
	log     *slog.Logger
	cfg     AdmitConfig

	store StateStore // nil = chi giu trang thai trong RAM (test, chay don le)

	rate atomic.Uint64 // nguoi/giay * 1000, de luu float trong atomic

	// cooldown la so tick con phai giu nguyen rate sau mot lan giam. Chi tick() cham vao no.
	cooldown int
	// overrideFromStore: co ghi de dang bat den tu StateStore (khac voi SetRateOverride trong RAM),
	// de biet khi Ops bo ghi de trong store thi duoc tra quyen cho AIMD.
	overrideFromStore bool

	// override bat khi Ops ghi de thu cong (BR-Q5). Trong luc bat, tick KHONG chay AIMD:
	// gia tri Ops dat duoc giu nguyen cho toi khi ClearRateOverride.
	override atomic.Bool
}

// Admitter la phan hang cho ma controller can (DIP -- khong phu thuoc Redis).
type Admitter interface {
	Admit(ctx context.Context, eventID string, batch int, ttl time.Duration) ([]string, error)
	SetAdmitRate(ctx context.Context, eventID string, ratePerSec float64) error
	QueueDepth(ctx context.Context, eventID string) (int64, error)
	Shuffle(ctx context.Context, eventID string, seed int64) (int64, error)
}

// StateStore la trang thai cua controller can song sot qua doi leader va restart (EV-142).
//
// Controller o RAM thi leader moi bat dau lai tu san 5 nguoi/giay va quen luon lenh "tam dung
// khan cap" cua Ops -- cai sau la loi an toan: van tu mo lai dung luc dang co su co.
type StateStore interface {
	// AdmitRate la nhip admit leader truoc da ghi; ok=false neu chua tung ghi.
	AdmitRate(ctx context.Context, eventID string) (rate float64, ok bool, err error)
	// RateOverride la lenh ghi de cua Ops; active=false neu khong co.
	RateOverride(ctx context.Context, eventID string) (rate float64, active bool, err error)
}

// AdjustMetrics (tuy chon, nhan qua AdmitMetrics) nhan ly do moi tick dieu chinh nhip, de van hanh
// thay vi sao rate dang giam / dung yen: "increase" | "decrease" | "hold" | "override".
type AdjustMetrics interface {
	AdmitAdjust(eventID, action string)
}

// HealthProbe cung cap tin hieu suc khoe cua tang duoi.
type HealthProbe interface {
	// Health tra ve tinh trang checkout trong cua so vua roi.
	Health(ctx context.Context, eventID string) (Health, error)
}

// Health la tin hieu backpressure do duoc tu tang duoi.
type Health struct {
	CheckoutP99   time.Duration // do tre p99 cua luong tao hold
	ErrorRate     float64       // 0..1
	DBPoolUsage   float64       // 0..1
	HoldsInFlight int64
}

type AdmitConfig struct {
	EventID     string
	SaleStartAt time.Time

	MinRate  float64 // san: luon nho giot de hang cho khong bao gio dung han
	MaxRate  float64 // tran: bao ve tang duoi ke ca khi moi chi so deu dep
	StepUp   float64 // cong them moi tick khi khoe (additive increase)
	Backoff  float64 // he so nhan khi co suc ep (multiplicative decrease)
	Tick     time.Duration
	AdmitTTL time.Duration // BR-Q7: 15 phut

	// Nguong coi la "dang chiu suc ep".
	P99Budget   time.Duration
	ErrorBudget float64
	PoolBudget  float64

	// DecreaseCooldown la so tick giu nguyen rate sau mot lan giam.
	//
	// Nguoi vua duoc tha vao khong mua ngay: ho mat 30 giay den 2 phut de chon ve, nen tai tang
	// duoi dang thay la hau qua cua quyet dinh tu truoc do. Phan ung lien tuc voi tin hieu tre
	// do thi giam, giam, giam (van qua tai vi nguoi cu van dang vao) roi vot len, tao dao dong
	// rang cua. Cho mot nhip de lan giam truoc kip co tac dung.
	DecreaseCooldown int
}

func DefaultAdmitConfig(eventID string, saleStart time.Time) AdmitConfig {
	return AdmitConfig{
		EventID:     eventID,
		SaleStartAt: saleStart,
		MinRate:     5,
		MaxRate:     500,
		StepUp:      10,
		Backoff:     0.7,
		Tick:        time.Second,
		AdmitTTL:    15 * time.Minute,
		P99Budget:   800 * time.Millisecond,
		ErrorBudget: 0.02,
		PoolBudget:  0.80,

		DecreaseCooldown: 5,
	}
}

// WithStateStore gan noi luu trang thai ben vung (Redis). Khong goi thi controller chi giu trong RAM.
func (c *AdmitController) WithStateStore(s StateStore) *AdmitController {
	c.store = s
	return c
}

// AdmitMetrics nhan so lieu moi tick cua controller (cho dashboard va alert
// "admit_rate = 0 trong khi hang cho con nguoi").
type AdmitMetrics interface {
	AdmitState(eventID string, ratePerSec float64, queueDepth int64)
}

// WithMetrics gan noi nhan so lieu. Khong goi thi controller khong doc them gi tu
// Redis moi tick.
func (c *AdmitController) WithMetrics(m AdmitMetrics) *AdmitController {
	c.obs = m
	return c
}

func NewAdmitController(q Admitter, m HealthProbe, cfg AdmitConfig, log *slog.Logger) *AdmitController {
	c := &AdmitController{queue: q, metrics: m, cfg: cfg, log: log}
	c.setRate(cfg.MinRate)
	return c
}

// Run chay vong dieu khien. Chi goi sau khi da doat leader lock (libs/go/leader) va queue phai
// la ban da gan epoch: neu hai instance cung chay, moi giay se co gap doi so nguoi duoc tha vao,
// va fencing la thu chan leader cu o phia Redis. Gap domain.ErrFenced, Run tra ve ngay.
func (c *AdmitController) Run(ctx context.Context) error {
	if err := c.waitForSaleStart(ctx); err != nil {
		return err
	}
	c.restore(ctx)

	// Lottery tai T0 (BR-Q1). Seed lay tu server de ket qua khong doan truoc duoc.
	n, err := c.queue.Shuffle(ctx, c.cfg.EventID, time.Now().UnixNano()+rand.Int63())
	if err != nil {
		return err
	}
	c.log.Info("lottery hoan tat", "event", c.cfg.EventID, "so_nguoi", n)

	t := time.NewTicker(c.cfg.Tick)
	defer t.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.C:
			if err := c.tick(ctx); err != nil {
				if errors.Is(err, domain.ErrFenced) {
					c.log.Warn("bi fencing chan, thoai vi", "event", c.cfg.EventID)
					return err
				}
				// Mot tick loi khong duoc lam chet vong dieu khien: hang cho dung
				// han con te hon la admit hoi lech nhip.
				c.log.Error("tick that bai", "event", c.cfg.EventID, "err", err)
			}
		}
	}
}

func (c *AdmitController) waitForSaleStart(ctx context.Context) error {
	d := time.Until(c.cfg.SaleStartAt)
	if d <= 0 {
		return nil
	}
	c.log.Info("cho toi gio mo ban", "event", c.cfg.EventID, "con", d.String())
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(d):
		return nil
	}
}

// restore nap lai nhip leader truoc da ghi, de doi leader khong keo nhip ve san. Loi doc chi duoc
// log: bat dau tu san la huong an toan.
func (c *AdmitController) restore(ctx context.Context) {
	if c.store == nil {
		return
	}
	r, ok, err := c.store.AdmitRate(ctx, c.cfg.EventID)
	switch {
	case err != nil:
		c.log.Warn("khong doc duoc nhip admit cu, bat dau tu san", "event", c.cfg.EventID, "err", err)
	case ok && r > 0:
		c.setRate(clamp(r, c.cfg.MinRate, c.cfg.MaxRate))
		c.log.Info("tiep tuc nhip admit cua leader truoc", "event", c.cfg.EventID, "rate", c.Rate())
	}
}

// syncOverride dong bo lenh ghi de cua Ops tu store vao controller moi tick (<= 1 tick = hieu luc
// trong <= 2 giay nhu AC cua EV-147). Loi doc giu nguyen trang thai hien tai.
func (c *AdmitController) syncOverride(ctx context.Context) {
	if c.store == nil {
		return
	}
	r, active, err := c.store.RateOverride(ctx, c.cfg.EventID)
	switch {
	case err != nil:
		c.log.Warn("khong doc duoc lenh ghi de, giu trang thai hien tai", "event", c.cfg.EventID, "err", err)
	case active:
		// Bat co TRUOC khi ghi rate, cung thu tu voi SetRateOverride: xen ke nao cung ket thuc bang
		// gia tri cua Ops.
		c.override.Store(true)
		c.setRate(clamp(r, 0, c.cfg.MaxRate))
		c.overrideFromStore = true
	case c.overrideFromStore:
		// Ops bo ghi de trong store: tra quyen cho AIMD. Chi go co do chinh store dat, khong dong
		// vao SetRateOverride trong RAM.
		c.override.Store(false)
		c.overrideFromStore = false
		c.log.Warn("Ops bo ghi de admit_rate, tra ve tu dieu chinh", "event", c.cfg.EventID, "rate", c.Rate())
	}
}

func (c *AdmitController) noteAdjust(action string) {
	if m, ok := c.obs.(AdjustMetrics); ok {
		m.AdmitAdjust(c.cfg.EventID, action)
	}
}

func (c *AdmitController) tick(ctx context.Context) error {
	c.syncOverride(ctx)

	// Ops dang ghi de (BR-Q5): giu nguyen rate cua Ops, khong tu chinh. Ban cu van chay
	// AIMD tu gia tri ghi de, nen 1 giay sau no da bi thay: Ops dat 0 de dung khan cap ma
	// 3 tick sau van tha 60 nguoi.
	if !c.override.Load() {
		h, err := c.metrics.Health(ctx, c.cfg.EventID)
		if err != nil {
			// Khong do duoc suc khoe tang duoi -> lui ve mot cach than trong.
			// Mu ma van tha nguoi voi nhip cu la cach nhanh nhat de lam sap he thong.
			c.noteAdjust(c.adjust(false))
		} else {
			c.noteAdjust(c.adjust(c.healthy(h)))
		}
	} else {
		c.noteAdjust("override")
	}

	rate := c.Rate()
	if err := c.queue.SetAdmitRate(ctx, c.cfg.EventID, rate); err != nil {
		return err
	}
	c.observe(ctx, rate)

	batch := int(math.Round(rate * c.cfg.Tick.Seconds()))
	if batch <= 0 {
		return nil
	}

	tokens, err := c.queue.Admit(ctx, c.cfg.EventID, batch, c.cfg.AdmitTTL)
	if err != nil {
		return err
	}
	if len(tokens) > 0 {
		c.log.Debug("da admit", "event", c.cfg.EventID, "so_luong", len(tokens), "rate", rate)
	}
	return nil
}

// observe xuat metric; loi doc do sau hang cho khong duoc lam hong vong admit.
func (c *AdmitController) observe(ctx context.Context, rate float64) {
	if c.obs == nil {
		return
	}
	depth, err := c.queue.QueueDepth(ctx, c.cfg.EventID)
	if err != nil {
		c.log.Debug("khong doc duoc do sau hang cho", "event", c.cfg.EventID, "err", err)
		return
	}
	c.obs.AdmitState(c.cfg.EventID, rate, depth)
}

func (c *AdmitController) healthy(h Health) bool {
	return h.CheckoutP99 <= c.cfg.P99Budget &&
		h.ErrorRate <= c.cfg.ErrorBudget &&
		h.DBPoolUsage <= c.cfg.PoolBudget
}

// adjust hien thuc AIMD: khoe thi cong, met thi nhan. Tra ve hanh dong da lam
// ("increase" | "decrease" | "hold" | "override") de xuat metric.
//
// Bat doi xung nay la co y. Tang tuyen tinh giup do tim gioi han that su cua
// he thong ma khong vot qua. Giam theo he so nhan giup thoat khoi vung qua tai
// trong vai tick thay vi vai chuc tick -- vi khi tang duoi da qua tai thi moi
// giay cham tre deu lam hang doi don them.
//
// Sau moi lan giam co DecreaseCooldown tick giu nguyen (xem AdmitConfig) de tin hieu tre kip
// phan anh quyet dinh truoc, thay vi giam chong len nhau.
func (c *AdmitController) adjust(healthy bool) string {
	if c.cooldown > 0 {
		c.cooldown--
		return "hold"
	}
	old := c.rate.Load()
	cur := float64(old) / 1000.0
	var next float64
	if healthy {
		next = cur + c.cfg.StepUp
	} else {
		next = cur * c.cfg.Backoff
	}
	// Ops co the ghi de GIUA luc doc `old` va luc ghi ket qua. Khong duoc de AIMD de len
	// gia tri cua Ops: kiem co override lan nua, va chi ghi neu rate chua doi tu luc doc
	// (CompareAndSwap). SetRateOverride bat co TRUOC khi ghi rate, nen moi thu tu xen ke
	// deu ket thuc bang gia tri cua Ops.
	if c.override.Load() {
		return "override"
	}
	nextRaw := uint64(clamp(next, c.cfg.MinRate, c.cfg.MaxRate) * 1000)
	if !c.rate.CompareAndSwap(old, nextRaw) {
		return "override"
	}
	switch {
	case nextRaw < old:
		// Chi bat cooldown khi rate THUC SU giam: o san roi thi khong co gi de cho, va phuc hoi
		// khong bi tri hoan vo ich.
		c.cooldown = c.cfg.DecreaseCooldown
		return "decrease"
	case nextRaw > old:
		return "increase"
	default:
		return "hold"
	}
}

func (c *AdmitController) Rate() float64 {
	return float64(c.rate.Load()) / 1000.0
}

// SetRateOverride cho Ops ghi de thu cong (BR-Q5). Gia tri duoc GIU o moi tick sau do --
// ke ca 0 de tam dung khan cap -- cho toi khi ClearRateOverride. Van bi chan duoi 0 va
// tren MaxRate: Ops khong duoc vuot tran bao ve tang duoi.
func (c *AdmitController) SetRateOverride(r float64) {
	c.override.Store(true)
	c.setRate(clamp(r, 0, c.cfg.MaxRate))
	c.log.Warn("admit_rate bi ghi de thu cong", "event", c.cfg.EventID, "rate", r)
}

// ClearRateOverride tra quyen lai cho AIMD. Tick ke tiep tu chinh tiep tu rate hien tai
// (va bi keo ve [MinRate, MaxRate] -- nen sau mot lan tam dung 0/s, AIMD bat dau lai tu
// san MinRate chu khong dung im o 0).
func (c *AdmitController) ClearRateOverride() {
	c.override.Store(false)
	c.log.Warn("bo ghi de admit_rate, tra ve tu dieu chinh", "event", c.cfg.EventID, "rate", c.Rate())
}

func (c *AdmitController) setRate(r float64) { c.rate.Store(uint64(r * 1000)) }

func clamp(v, lo, hi float64) float64 {
	return math.Max(lo, math.Min(hi, v))
}
