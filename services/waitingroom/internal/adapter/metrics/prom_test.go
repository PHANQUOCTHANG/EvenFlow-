package metricsadapter_test

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"

	metricsadapter "github.com/eventflow/eventflow/services/waitingroom/internal/adapter/metrics"
)

func TestProm_AdmitState_TheoTungSuKien(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := metricsadapter.New(reg)

	m.AdmitState("ev-a", 42.5, 1000)
	m.AdmitState("ev-b", 0, 7)
	m.AdmitState("ev-a", 50, 900) // cap nhat de len gia tri cu

	families, err := reg.Gather()
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]float64{}
	for _, f := range families {
		for _, mt := range f.GetMetric() {
			got[f.GetName()+"/"+mt.GetLabel()[0].GetValue()] = mt.GetGauge().GetValue()
		}
	}
	want := map[string]float64{
		"waitingroom_admit_rate/ev-a":  50,
		"waitingroom_queue_depth/ev-a": 900,
		"waitingroom_admit_rate/ev-b":  0,
		"waitingroom_queue_depth/ev-b": 7,
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %v, muon %v", k, got[k], v)
		}
	}
}
