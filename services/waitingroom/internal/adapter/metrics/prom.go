// Package metricsadapter xuat so lieu cua phong cho sang Prometheus.
//
// Ten metric khop deploy/observability/alerts.yml: alert "admit_rate = 0 trong
// khi hang cho con nguoi" doc dung hai gauge nay.
package metricsadapter

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Prom hien thuc app.AdmitMetrics.
type Prom struct {
	rate  *prometheus.GaugeVec
	depth *prometheus.GaugeVec
}

// New dang ky metric len reg. Chi goi mot lan cho moi registry.
func New(reg prometheus.Registerer) *Prom {
	f := promauto.With(reg)
	return &Prom{
		rate: f.NewGaugeVec(prometheus.GaugeOpts{
			Name: "waitingroom_admit_rate",
			Help: "So nguoi duoc tha vao mua ve moi giay, theo su kien.",
		}, []string{"event"}),
		depth: f.NewGaugeVec(prometheus.GaugeOpts{
			Name: "waitingroom_queue_depth",
			Help: "So nguoi dang cho trong hang, theo su kien.",
		}, []string{"event"}),
	}
}

// AdmitState cap nhat rate va do sau hang cho cua mot su kien.
func (p *Prom) AdmitState(eventID string, ratePerSec float64, queueDepth int64) {
	p.rate.WithLabelValues(eventID).Set(ratePerSec)
	p.depth.WithLabelValues(eventID).Set(float64(queueDepth))
}
