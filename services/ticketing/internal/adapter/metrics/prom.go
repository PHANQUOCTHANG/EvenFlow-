// Package metricsadapter xuat so lieu nghiep vu cua ticketing sang Prometheus.
//
// Ten metric khop deploy/observability/alerts.yml va docs/02 muc 6: doi ten o day
// la lam cau alert im lang.
package metricsadapter

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Prom hien thuc port.HoldMetrics va worker.SweeperMetrics.
type Prom struct {
	holdDuration *prometheus.HistogramVec
	guard        prometheus.Counter
	released     prometheus.Counter
	reg          prometheus.Registerer
}

// New dang ky metric len reg. Chi goi mot lan cho moi registry.
func New(reg prometheus.Registerer) *Prom {
	f := promauto.With(reg)
	return &Prom{
		reg: reg,
		holdDuration: f.NewHistogramVec(prometheus.HistogramOpts{
			Name: "ticketing_hold_duration_seconds",
			Help: "Thoi gian tao hold, theo ket qua nghiep vu.",
			// Duong nong qua Redis + Postgres: p99 can do duoc o muc vai chuc ms.
			Buckets: []float64{.001, .0025, .005, .01, .025, .05, .1, .25, .5, 1, 2.5},
		}, []string{"outcome"}),
		guard: f.NewCounter(prometheus.CounterOpts{
			Name: "ticketing_oversell_guard_rejections_total",
			Help: "So lan Postgres chan mot hold ma Redis da cho qua (Redis lech so).",
		}),
		released: f.NewCounter(prometheus.CounterOpts{
			Name: "ticketing_holds_released_total",
			Help: "So hold het han da duoc sweeper tra kho.",
		}),
	}
}

// HoldAttempt ghi mot lan tao hold.
func (p *Prom) HoldAttempt(outcome string, d time.Duration) {
	p.holdDuration.WithLabelValues(outcome).Observe(d.Seconds())
}

// OversellGuardRejected ghi mot lan Postgres chan mot hold ma Redis da cho qua.
func (p *Prom) OversellGuardRejected() { p.guard.Inc() }

// HoldsReleased cong so hold vua tra kho.
func (p *Prom) HoldsReleased(n int) { p.released.Add(float64(n)) }

// RegisterPoolUsage dang ky gauge muc dung pool Postgres (0..1), doc luc scrape.
// Day la mot trong ba tin hieu backpressure cua admit controller.
func (p *Prom) RegisterPoolUsage(usage func() float64) {
	promauto.With(p.reg).NewGaugeFunc(prometheus.GaugeOpts{
		Name: "ticketing_db_pool_usage",
		Help: "Ti le ket noi Postgres dang duoc dung (0..1).",
	}, usage)
}
