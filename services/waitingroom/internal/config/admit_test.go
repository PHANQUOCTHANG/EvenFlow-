package config_test

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/eventflow/eventflow/services/waitingroom/internal/app"
	"github.com/eventflow/eventflow/services/waitingroom/internal/config"
)

func env(m map[string]string) config.Getenv {
	return func(k string) string { return m[k] }
}

func TestAdmitConfig_KhongCoEnv_DungMacDinh(t *testing.T) {
	t0 := time.Date(2026, 10, 10, 20, 0, 0, 0, time.UTC)

	got, err := config.AdmitConfig(env(nil), "ev1", t0)
	if err != nil {
		t.Fatalf("loi bat ngo: %v", err)
	}
	want := app.DefaultAdmitConfig("ev1", t0)
	if got != want {
		t.Fatalf("muon mac dinh %+v, nhan %+v", want, got)
	}
}

func TestAdmitConfig_EnvGhiDeMacDinh(t *testing.T) {
	got, err := config.AdmitConfig(env(map[string]string{
		"ADMIT_MIN_RATE":    "2",
		"ADMIT_MAX_RATE":    "50",
		"ADMIT_STEP_UP":     "5",
		"ADMIT_BACKOFF":     "0.5",
		"ADMIT_TTL_SECONDS": "900",
	}), "ev1", time.Time{})
	if err != nil {
		t.Fatalf("loi bat ngo: %v", err)
	}
	if got.MinRate != 2 || got.MaxRate != 50 || got.StepUp != 5 || got.Backoff != 0.5 {
		t.Errorf("tham so AIMD khong khop env: %+v", got)
	}
	if got.AdmitTTL != 15*time.Minute {
		t.Errorf("AdmitTTL = %v, muon 15 phut (BR-Q7)", got.AdmitTTL)
	}
}

func TestAdmitConfig_GiaTriSai_PhaiTuChoiLucKhoiDong(t *testing.T) {
	cases := map[string]map[string]string{
		"khong phai so":     {"ADMIT_MAX_RATE": "nhanh"},
		"min = 0":           {"ADMIT_MIN_RATE": "0"},
		"max nho hon min":   {"ADMIT_MIN_RATE": "10", "ADMIT_MAX_RATE": "5"},
		"step up am":        {"ADMIT_STEP_UP": "-1"},
		"backoff = 1":       {"ADMIT_BACKOFF": "1"},
		"backoff lon hon 1": {"ADMIT_BACKOFF": "1.5"},
		"backoff = 0":       {"ADMIT_BACKOFF": "0"},
		"ttl am":            {"ADMIT_TTL_SECONDS": "-5"},
		"ttl khong phai so": {"ADMIT_TTL_SECONDS": "15m"},
	}
	for name, m := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := config.AdmitConfig(env(m), "ev1", time.Time{}); err == nil {
				t.Fatalf("cau hinh %v phai bi tu choi", m)
			}
		})
	}
}

func TestAdmitConfig_LoiNoiRoTenBien(t *testing.T) {
	_, err := config.AdmitConfig(env(map[string]string{"ADMIT_BACKOFF": "2"}), "ev1", time.Time{})
	if err == nil || !strings.Contains(err.Error(), "ADMIT_BACKOFF") {
		t.Fatalf("loi phai nhac ten bien sai, nhan %v", err)
	}
}

func TestSplitCSV(t *testing.T) {
	got := config.SplitCSV(" a, b ,,c,")
	if want := []string{"a", "b", "c"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("SplitCSV = %v, muon %v", got, want)
	}
	if got := config.SplitCSV("  "); len(got) != 0 {
		t.Fatalf("chuoi rong phai ra danh sach rong, nhan %v", got)
	}
}
