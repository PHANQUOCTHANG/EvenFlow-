package app

import (
	"context"
	"log/slog"
	"math"
	"math/rand"
	"sync/atomic"
	"time"
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
	log     *slog.Logger
	cfg     AdmitConfig

	rate atomic.Uint64 // nguoi/giay * 1000, de luu float trong atomic
}

// Admitter la phan hang cho ma controller can (DIP -- khong phu thuoc Redis).
type Admitter interface {
	Admit(ctx context.Context, eventID string, batch int, ttl time.Duration) ([]string, error)
	SetAdmitRate(ctx context.Context, eventID string, ratePerSec float64) error
	QueueDepth(ctx context.Context, eventID string) (int64, error)
	Shuffle(ctx context.Context, eventID string, seed int64) (int64, error)
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
	}
}

func NewAdmitController(q Admitter, m HealthProbe, cfg AdmitConfig, log *slog.Logger) *AdmitController {
	c := &AdmitController{queue: q, metrics: m, cfg: cfg, log: log}
	c.setRate(cfg.MinRate)
	return c
}

// Run chay vong dieu khien. Chi goi sau khi da doat leader lock -- neu hai
// instance cung chay, moi giay se co gap doi so nguoi duoc tha vao.
func (c *AdmitController) Run(ctx context.Context) error {
	if err := c.waitForSaleStart(ctx); err != nil {
		return err
	}

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

func (c *AdmitController) tick(ctx context.Context) error {
	h, err := c.metrics.Health(ctx, c.cfg.EventID)
	if err != nil {
		// Khong do duoc suc khoe tang duoi -> lui ve mot cach than trong.
		// Mu ma van tha nguoi voi nhip cu la cach nhanh nhat de lam sap he thong.
		c.adjust(false)
	} else {
		c.adjust(c.healthy(h))
	}

	rate := c.Rate()
	if err := c.queue.SetAdmitRate(ctx, c.cfg.EventID, rate); err != nil {
		return err
	}

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

func (c *AdmitController) healthy(h Health) bool {
	return h.CheckoutP99 <= c.cfg.P99Budget &&
		h.ErrorRate <= c.cfg.ErrorBudget &&
		h.DBPoolUsage <= c.cfg.PoolBudget
}

// adjust hien thuc AIMD: khoe thi cong, met thi nhan.
//
// Bat doi xung nay la co y. Tang tuyen tinh giup do tim gioi han that su cua
// he thong ma khong vot qua. Giam theo he so nhan giup thoat khoi vung qua tai
// trong vai tick thay vi vai chuc tick -- vi khi tang duoi da qua tai thi moi
// giay cham tre deu lam hang doi don them.
func (c *AdmitController) adjust(healthy bool) {
	cur := c.Rate()
	var next float64
	if healthy {
		next = cur + c.cfg.StepUp
	} else {
		next = cur * c.cfg.Backoff
	}
	c.setRate(clamp(next, c.cfg.MinRate, c.cfg.MaxRate))
}

func (c *AdmitController) Rate() float64 {
	return float64(c.rate.Load()) / 1000.0
}

// SetRateOverride cho Ops ghi de thu cong (BR-Q5).
func (c *AdmitController) SetRateOverride(r float64) {
	c.setRate(clamp(r, 0, c.cfg.MaxRate))
	c.log.Warn("admit_rate bi ghi de thu cong", "event", c.cfg.EventID, "rate", r)
}

func (c *AdmitController) setRate(r float64) { c.rate.Store(uint64(r * 1000)) }

func clamp(v, lo, hi float64) float64 {
	return math.Max(lo, math.Min(hi, v))
}
