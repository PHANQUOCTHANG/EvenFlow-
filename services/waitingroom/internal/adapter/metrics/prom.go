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
	rate   *prometheus.GaugeVec
	depth  *prometheus.GaugeVec
	adjust *prometheus.CounterVec
	leader *prometheus.GaugeVec
	epoch  *prometheus.GaugeVec
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
		adjust: f.NewCounterVec(prometheus.CounterOpts{
			Name: "waitingroom_admit_adjust_total",
			Help: "So tick theo hanh dong dieu chinh nhip admit: increase, decrease, hold, override.",
		}, []string{"event", "action"}),
		leader: f.NewGaugeVec(prometheus.GaugeOpts{
			Name: "waitingroom_admit_leader",
			Help: "1 neu instance nay dang giu quyen admit cua su kien, 0 neu khong.",
		}, []string{"event"}),
		epoch: f.NewGaugeVec(prometheus.GaugeOpts{
			Name: "waitingroom_admit_leader_epoch",
			Help: "Epoch (fencing token) cua nhiem ky leader hien tai cua instance nay.",
		}, []string{"event"}),
	}
}

// AdmitAdjust dem mot tick theo hanh dong dieu chinh nhip (app.AdjustMetrics).
func (p *Prom) AdmitAdjust(eventID, action string) {
	p.adjust.WithLabelValues(eventID, action).Inc()
}

// LeaderState ghi lai instance nay co dang giu quyen admit cua su kien khong. Tong
// sum(waitingroom_admit_leader) theo su kien phai bang 1; > 1 keo dai la dau hieu cua so hai leader.
func (p *Prom) LeaderState(eventID string, isLeader bool, epoch int64) {
	v := 0.0
	if isLeader {
		v = 1
	}
	p.leader.WithLabelValues(eventID).Set(v)
	p.epoch.WithLabelValues(eventID).Set(float64(epoch))
}

// AdmitState cap nhat rate va do sau hang cho cua mot su kien.
func (p *Prom) AdmitState(eventID string, ratePerSec float64, queueDepth int64) {
	p.rate.WithLabelValues(eventID).Set(ratePerSec)
	p.depth.WithLabelValues(eventID).Set(float64(queueDepth))
}
