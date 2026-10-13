package outbox

import (
	"fmt"

	"github.com/prometheus/client_golang/prometheus"
)

type metrics struct {
	connected   prometheus.Gauge
	published   *prometheus.CounterVec
	unroutable  *prometheus.CounterVec
	publishErrs *prometheus.CounterVec
	lag         prometheus.Histogram
	pending     prometheus.Gauge
	dead        prometheus.Gauge
}

func newMetrics(reg prometheus.Registerer) (*metrics, error) {
	m := &metrics{
		connected: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "outbox_relay_connected",
			Help: "1 neu relay dang ket noi toi broker, 0 neu khong.",
		}),
		published: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "outbox_published_total",
			Help: "So event da publish va duoc broker xac nhan.",
		}, []string{"routing_key"}),
		unroutable: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "outbox_unroutable_total",
			Help: "So event khong co queue nhan va bi bo (DropUnroutable).",
		}, []string{"routing_key"}),
		publishErrs: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "outbox_publish_errors_total",
			Help: "So lan publish loi lam tang attempts cua row.",
		}, []string{"routing_key"}),
		lag: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:    "outbox_publish_lag_seconds",
			Help:    "Do tre tu luc ghi outbox toi luc publish thanh cong.",
			Buckets: []float64{0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30, 60, 300, 1800},
		}),
		pending: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "outbox_pending",
			Help: "So row chua publish (khong tinh row bi do), cap nhat dinh ky.",
		}),
		dead: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "outbox_dead",
			Help: "So row bi do (next_attempt_at = infinity) cho van hanh xu ly, cap nhat dinh ky.",
		}),
	}
	all := []prometheus.Collector{m.connected, m.published, m.unroutable, m.publishErrs, m.lag, m.pending, m.dead}
	for i, c := range all {
		if err := reg.Register(c); err != nil {
			for _, done := range all[:i] {
				reg.Unregister(done)
			}
			return nil, fmt.Errorf("outbox: dang ky metric: %w", err)
		}
	}
	return m, nil
}
