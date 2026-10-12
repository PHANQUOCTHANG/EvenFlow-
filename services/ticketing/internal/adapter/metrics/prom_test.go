package metricsadapter_test

import (
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	metricsadapter "github.com/eventflow/eventflow/services/ticketing/internal/adapter/metrics"
)

// Kiem ten metric: alerts.yml viet cung ten nay, doi ten la lam alert im lang.

func gather(t *testing.T, reg *prometheus.Registry) map[string]float64 {
	t.Helper()
	families, err := reg.Gather()
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]float64{}
	for _, f := range families {
		for _, m := range f.GetMetric() {
			switch {
			case m.GetCounter() != nil:
				out[f.GetName()] += m.GetCounter().GetValue()
			case m.GetGauge() != nil:
				out[f.GetName()] += m.GetGauge().GetValue()
			case m.GetHistogram() != nil:
				out[f.GetName()+"_count"] += float64(m.GetHistogram().GetSampleCount())
			}
		}
	}
	return out
}

func TestProm_XuatDungTenMetricCuaAlert(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := metricsadapter.New(reg)
	m.RegisterPoolUsage(func() float64 { return 0.25 })

	m.HoldAttempt("ok", 5*time.Millisecond)
	m.HoldAttempt("sold_out", 2*time.Millisecond)
	m.OversellGuardRejected()
	m.OversellGuardRejected()
	m.HoldsReleased(3)

	got := gather(t, reg)
	want := map[string]float64{
		"ticketing_hold_duration_seconds_count":     2,
		"ticketing_oversell_guard_rejections_total": 2,
		"ticketing_holds_released_total":            3,
		"ticketing_db_pool_usage":                   0.25,
	}
	for name, v := range want {
		if got[name] != v {
			t.Errorf("%s = %v, muon %v", name, got[name], v)
		}
	}
}

func TestProm_HoldDuration_TachTheoKetQua(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := metricsadapter.New(reg)
	m.HoldAttempt("ok", time.Millisecond)
	m.HoldAttempt("ok", time.Millisecond)
	m.HoldAttempt("error", time.Millisecond)

	families, _ := reg.Gather()
	byOutcome := map[string]uint64{}
	for _, f := range families {
		if f.GetName() != "ticketing_hold_duration_seconds" {
			continue
		}
		for _, mt := range f.GetMetric() {
			byOutcome[mt.GetLabel()[0].GetValue()] = mt.GetHistogram().GetSampleCount()
		}
	}
	if byOutcome["ok"] != 2 || byOutcome["error"] != 1 {
		t.Errorf("theo ket qua = %v", byOutcome)
	}
}
