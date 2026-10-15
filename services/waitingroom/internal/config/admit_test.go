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

func TestAdmitConfig_DecreaseCooldown(t *testing.T) {
	got, err := config.AdmitConfig(env(nil), "ev1", time.Time{})
	if err != nil || got.DecreaseCooldown != 5 {
		t.Fatalf("mac dinh: cooldown=%d err=%v, muon 5", got.DecreaseCooldown, err)
	}
	got, err = config.AdmitConfig(env(map[string]string{"ADMIT_DECREASE_COOLDOWN": "0"}), "ev1", time.Time{})
	if err != nil || got.DecreaseCooldown != 0 {
		t.Fatalf("0 = tat cua so cho: cooldown=%d err=%v", got.DecreaseCooldown, err)
	}
	if _, err := config.AdmitConfig(env(map[string]string{"ADMIT_DECREASE_COOLDOWN": "-1"}), "ev1", time.Time{}); err == nil {
		t.Fatal("cooldown am phai bi tu choi")
	}
}

func TestLeaderConfig(t *testing.T) {
	l, err := config.LeaderConfig(env(nil))
	if err != nil || l.TTL != 5*time.Second || l.RenewEvery != time.Second {
		t.Fatalf("mac dinh: %+v err=%v, muon 5s/1s", l, err)
	}
	l, err = config.LeaderConfig(env(map[string]string{"LEADER_TTL_SECONDS": "10", "LEADER_RENEW_SECONDS": "2.5"}))
	if err != nil || l.TTL != 10*time.Second || l.RenewEvery != 2500*time.Millisecond {
		t.Fatalf("ghi de: %+v err=%v", l, err)
	}
	bad := map[string]map[string]string{
		"khong phai so":                {"LEADER_TTL_SECONDS": "5s"},
		"ttl = 0":                      {"LEADER_TTL_SECONDS": "0"},
		"gia han am":                   {"LEADER_RENEW_SECONDS": "-1"},
		"gia han qua thua (> ttl / 2)": {"LEADER_TTL_SECONDS": "5", "LEADER_RENEW_SECONDS": "3"},
	}
	for name, m := range bad {
		t.Run(name, func(t *testing.T) {
			if _, err := config.LeaderConfig(env(m)); err == nil {
				t.Fatalf("%v phai bi tu choi", m)
			}
		})
	}
}

func TestHealthConfig(t *testing.T) {
	h, err := config.HealthConfig(env(nil))
	if err != nil || h.Static || h.MaxAge != 3*time.Second {
		t.Fatalf("mac dinh: %+v err=%v, muon signal / 3s", h, err)
	}
	h, err = config.HealthConfig(env(map[string]string{"ADMIT_HEALTH_MODE": "STATIC", "HEALTH_MAX_AGE_SECONDS": "10"}))
	if err != nil || !h.Static || h.MaxAge != 10*time.Second {
		t.Fatalf("static: %+v err=%v", h, err)
	}
	for name, m := range map[string]map[string]string{
		"che do la":     {"ADMIT_HEALTH_MODE": "off"},
		"tuoi khong so": {"HEALTH_MAX_AGE_SECONDS": "abc"},
		"tuoi = 0":      {"HEALTH_MAX_AGE_SECONDS": "0"},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := config.HealthConfig(env(m)); err == nil {
				t.Fatalf("%v phai bi tu choi", m)
			}
		})
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
