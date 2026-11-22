package antibot

import "github.com/prometheus/client_golang/prometheus"

// Metrics are the collectors of contracts 6. No event or identity labels.
type Metrics struct {
	Decisions      *prometheus.CounterVec // action, applied, mode
	RuleFired      *prometheus.CounterVec // rule
	DecideSeconds  prometheus.Histogram
	Degraded       *prometheus.CounterVec // reason
	SignalsInvalid *prometheus.CounterVec // reason
	Dropped        prometheus.Counter
	NonUUID        prometheus.Counter
	SlotRevoked    prometheus.Counter
	NetSuppressed  prometheus.Counter
	Capped         prometheus.Counter
}

// Degraded reasons.
const (
	DegradedRedisTimeout = "redis_timeout"
	DegradedRedisError   = "redis_error"
	DegradedBreakerOpen  = "breaker_open"
	DegradedPanic        = "panic"
)

// NewMetrics creates and registers the collectors on reg (nil = not registered).
func NewMetrics(reg prometheus.Registerer) *Metrics {
	m := &Metrics{
		Decisions: prometheus.NewCounterVec(prometheus.CounterOpts{Name: "antibot_decisions_total",
			Help: "Anti-bot decisions by tier, applied effect and mode."}, []string{"action", "applied", "mode"}),
		RuleFired: prometheus.NewCounterVec(prometheus.CounterOpts{Name: "antibot_rule_fired_total",
			Help: "Anti-bot rules fired."}, []string{"rule"}),
		DecideSeconds: prometheus.NewHistogram(prometheus.HistogramOpts{Name: "antibot_decide_seconds",
			Help:    "Latency of Guard.Decide (decode + state + rules + score).",
			Buckets: []float64{0.00025, 0.0005, 0.001, 0.002, 0.003, 0.004, 0.005, 0.0075, 0.01}}),
		Degraded: prometheus.NewCounterVec(prometheus.CounterOpts{Name: "antibot_degraded_total",
			Help: "Anti-bot decisions made without Redis state, or after a panic."}, []string{"reason"}),
		SignalsInvalid: prometheus.NewCounterVec(prometheus.CounterOpts{Name: "antibot_signals_invalid_total",
			Help: "Join bodies whose signal payload counts as missing."}, []string{"reason"}),
		Dropped: prometheus.NewCounter(prometheus.CounterOpts{Name: "antibot_decisions_dropped_total",
			Help: "bot_decisions rows dropped (buffer full, failed batch or no pool)."}),
		NonUUID: prometheus.NewCounter(prometheus.CounterOpts{Name: "antibot_decisions_nonuuid_total",
			Help: "bot_decisions rows whose event or identity id was not a UUID (stored as NULL)."}),
		SlotRevoked: prometheus.NewCounter(prometheus.CounterOpts{Name: "antibot_slot_revoked_total",
			Help: "Queue slots revoked by a BLOCK."}),
		NetSuppressed: prometheus.NewCounter(prometheus.CounterOpts{Name: "antibot_net_suppressed_total",
			Help: "Joins whose network prefix holds too large a share of the event's joins (R4N not evaluated)."}),
		Capped: prometheus.NewCounter(prometheus.CounterOpts{Name: "antibot_capped_total",
			Help: "BLOCK-tier decisions capped at SLOW_LANE for lack of a positive sign."}),
	}
	if reg != nil {
		reg.MustRegister(m.Decisions, m.RuleFired, m.DecideSeconds, m.Degraded, m.SignalsInvalid, m.Dropped,
			m.NonUUID, m.SlotRevoked, m.NetSuppressed, m.Capped)
	}
	return m
}
