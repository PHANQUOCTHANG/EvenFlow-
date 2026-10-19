// Package metricsadapter xuat so lieu nghiep vu cua payment-svc sang Prometheus.
//
// Ten metric khop docs/02 muc 6 va AC-6 cua task EV-153 (EVF-73):
// - payment_webhook_lag_seconds (p99 > 30s)
// - payment_reconciliation_total{outcome}
// - payment_reconciliation_duration_seconds
package metricsadapter

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Prom hien thuc port.ReconciliationMetrics su dung Prometheus client.
type Prom struct {
	webhookLag     *prometheus.HistogramVec
	reconcileTotal *prometheus.CounterVec
	reconcileDur   prometheus.Histogram
	reg            prometheus.Registerer
}

// New dang ky cac metric vao Prometheus registerer chi dinh.
func New(reg prometheus.Registerer) *Prom {
	if reg == nil {
		reg = prometheus.DefaultRegisterer
	}
	f := promauto.With(reg)

	lag := f.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "payment_webhook_lag_seconds",
		Help:    "Do tre nhan webhook tu cong thanh toan (tinh tu luc don sang PAYMENT_PENDING den khi duoc doi soat tu chua).",
		Buckets: []float64{10, 30, 60, 120, 300, 600, 900, 1800, 3600},
	}, []string{"outcome"})

	// Khoi tao cac nhan san de metrics xuat hien ngay tren Prometheus
	lag.WithLabelValues("healed_paid").Observe(0)

	total := f.NewCounterVec(prometheus.CounterOpts{
		Name: "payment_reconciliation_total",
		Help: "So luong don hang da duoc tien trinh doi soat xu ly theo tung outcome.",
	}, []string{"outcome"})

	total.WithLabelValues("healed_paid").Add(0)
	total.WithLabelValues("healed_cancelled").Add(0)
	total.WithLabelValues("still_pending").Add(0)
	total.WithLabelValues("skipped").Add(0)
	total.WithLabelValues("error").Add(0)

	dur := f.NewHistogram(prometheus.HistogramOpts{
		Name:    "payment_reconciliation_duration_seconds",
		Help:    "Thoi gian thuc thi mot chu ky doi soat don hang.",
		Buckets: []float64{0.01, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10},
	})

	return &Prom{
		reg:            reg,
		webhookLag:     lag,
		reconcileTotal: total,
		reconcileDur:   dur,
	}
}

// ObserveWebhookLag ghi nhan do tre webhook bang giay cho don hang da tu chua.
func (p *Prom) ObserveWebhookLag(d time.Duration) {
	if p == nil || p.webhookLag == nil {
		return
	}
	p.webhookLag.WithLabelValues("healed_paid").Observe(d.Seconds())
}

// IncReconciliation tang counter cho outcome tuong ung.
func (p *Prom) IncReconciliation(outcome string) {
	if p == nil || p.reconcileTotal == nil {
		return
	}
	p.reconcileTotal.WithLabelValues(outcome).Inc()
}

// ObserveReconciliationDuration ghi nhan thoi gian thuc thi cua chu ky doi soat.
func (p *Prom) ObserveReconciliationDuration(d time.Duration) {
	if p == nil || p.reconcileDur == nil {
		return
	}
	p.reconcileDur.Observe(d.Seconds())
}
