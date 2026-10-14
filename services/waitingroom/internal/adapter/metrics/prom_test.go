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

// EV-142: van hanh phai thay AI dang cam van (leader) va vi sao rate doi (adjust).
func TestProm_LeaderVaAdjust(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := metricsadapter.New(reg)

	m.LeaderState("ev-a", true, 7)
	m.LeaderState("ev-b", false, 0)
	m.LeaderState("ev-a", false, 7) // mat quyen: gauge ve 0, epoch cu van de tham chieu
	m.LeaderState("ev-a", true, 8)
	m.AdmitAdjust("ev-a", "decrease")
	m.AdmitAdjust("ev-a", "decrease")
	m.AdmitAdjust("ev-a", "hold")

	families, err := reg.Gather()
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]float64{}
	for _, f := range families {
		for _, mt := range f.GetMetric() {
			name := f.GetName()
			for _, l := range mt.GetLabel() {
				name += "/" + l.GetValue()
			}
			switch {
			case mt.Gauge != nil:
				got[name] = mt.GetGauge().GetValue()
			case mt.Counter != nil:
				got[name] = mt.GetCounter().GetValue()
			}
		}
	}
	want := map[string]float64{
		"waitingroom_admit_leader/ev-a":                1,
		"waitingroom_admit_leader/ev-b":                0,
		"waitingroom_admit_leader_epoch/ev-a":          8,
		"waitingroom_admit_adjust_total/decrease/ev-a": 2,
		"waitingroom_admit_adjust_total/hold/ev-a":     1,
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %v, muon %v", k, got[k], v)
		}
	}
}
