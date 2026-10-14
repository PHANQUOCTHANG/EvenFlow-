// Package config nap va kiem tra cau hinh luc khoi dong (12-factor).
//
// Cau hinh sai thi dung ngay luc khoi dong: mot controller chay voi tham so vo
// ly (vd backoff >= 1 khien rate khong bao gio giam) se chi lo ra dung luc he
// thong bi qua tai.
package config

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/eventflow/eventflow/services/waitingroom/internal/app"
)

// Getenv la ham doc bien moi truong; tach ra de test khong phai dung os.Setenv.
type Getenv func(string) string

// AdmitConfig dung cau hinh cho admit controller cua mot su kien tu env.
// Bien nao khong co thi dung gia tri mac dinh cua app.DefaultAdmitConfig.
func AdmitConfig(getenv Getenv, eventID string, saleStart time.Time) (app.AdmitConfig, error) {
	cfg := app.DefaultAdmitConfig(eventID, saleStart)

	var err error
	set := func(name string, dst *float64) {
		if err != nil {
			return
		}
		raw := strings.TrimSpace(getenv(name))
		if raw == "" {
			return
		}
		v, perr := strconv.ParseFloat(raw, 64)
		if perr != nil {
			err = fmt.Errorf("%s=%q khong phai so: %w", name, raw, perr)
			return
		}
		*dst = v
	}

	set("ADMIT_MIN_RATE", &cfg.MinRate)
	set("ADMIT_MAX_RATE", &cfg.MaxRate)
	set("ADMIT_STEP_UP", &cfg.StepUp)
	set("ADMIT_BACKOFF", &cfg.Backoff)

	var cooldown float64 = float64(cfg.DecreaseCooldown)
	set("ADMIT_DECREASE_COOLDOWN", &cooldown)
	cfg.DecreaseCooldown = int(cooldown)

	var ttlSeconds float64
	set("ADMIT_TTL_SECONDS", &ttlSeconds)
	if err != nil {
		return app.AdmitConfig{}, err
	}
	if ttlSeconds != 0 {
		cfg.AdmitTTL = time.Duration(ttlSeconds * float64(time.Second))
	}

	if err := validate(cfg); err != nil {
		return app.AdmitConfig{}, err
	}
	return cfg, nil
}

func validate(c app.AdmitConfig) error {
	switch {
	case c.MinRate <= 0:
		return fmt.Errorf("ADMIT_MIN_RATE phai > 0 (hien %v): hang cho khong duoc dung han", c.MinRate)
	case c.MaxRate < c.MinRate:
		return fmt.Errorf("ADMIT_MAX_RATE (%v) phai >= ADMIT_MIN_RATE (%v)", c.MaxRate, c.MinRate)
	case c.StepUp <= 0:
		return fmt.Errorf("ADMIT_STEP_UP phai > 0 (hien %v)", c.StepUp)
	case c.Backoff <= 0 || c.Backoff >= 1:
		return fmt.Errorf("ADMIT_BACKOFF phai trong (0,1) (hien %v): >= 1 thi rate khong bao gio giam", c.Backoff)
	case c.AdmitTTL <= 0:
		return fmt.Errorf("ADMIT_TTL_SECONDS phai > 0 (hien %v)", c.AdmitTTL)
	case c.DecreaseCooldown < 0:
		return fmt.Errorf("ADMIT_DECREASE_COOLDOWN phai >= 0 (hien %d); 0 = tat cua so cho", c.DecreaseCooldown)
	}
	return nil
}

// Leader la thoi gian cua lease bau leader.
type Leader struct {
	TTL        time.Duration // lease; leader chet thi nguoi khac len sau toi da TTL
	RenewEvery time.Duration // chu ky gia han / thu lay khoa
}

// LeaderConfig doc LEADER_TTL_SECONDS (mac dinh 5) va LEADER_RENEW_SECONDS (mac dinh 1).
//
// Ngan hon thi failover nhanh hon nhung de mat khoa oan khi GC hay mang cham; 5s / 1s cho phep
// mat hai lan gia han lien tiep ma van giu khoa.
func LeaderConfig(getenv Getenv) (Leader, error) {
	l := Leader{TTL: 5 * time.Second, RenewEvery: time.Second}
	read := func(name string, dst *time.Duration) error {
		raw := strings.TrimSpace(getenv(name))
		if raw == "" {
			return nil
		}
		v, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			return fmt.Errorf("%s=%q khong phai so: %w", name, raw, err)
		}
		*dst = time.Duration(v * float64(time.Second))
		return nil
	}
	if err := read("LEADER_TTL_SECONDS", &l.TTL); err != nil {
		return Leader{}, err
	}
	if err := read("LEADER_RENEW_SECONDS", &l.RenewEvery); err != nil {
		return Leader{}, err
	}
	switch {
	case l.TTL <= 0 || l.RenewEvery <= 0:
		return Leader{}, fmt.Errorf("LEADER_TTL_SECONDS (%v) va LEADER_RENEW_SECONDS (%v) phai > 0", l.TTL, l.RenewEvery)
	case l.RenewEvery*2 > l.TTL:
		return Leader{}, fmt.Errorf("LEADER_RENEW_SECONDS (%v) phai <= LEADER_TTL_SECONDS/2 (%v): "+
			"gia han it nhat hai lan moi lease, neu khong mot lan mat goi la mat khoa", l.RenewEvery, l.TTL/2)
	}
	return l, nil
}

// Health la nguon tin hieu suc khoe cua tang duoi.
type Health struct {
	// Static = true: tin hieu gia "luon khoe" -- CHI cho local khong co ticketing. Rate se leo toi
	// tran bat ke Postgres ra sao, nen production khong bao gio duoc dat.
	Static bool
	// MaxAge la tuoi toi da cua mot anh chup suc khoe con duoc tin.
	MaxAge time.Duration
}

// HealthConfig doc ADMIT_HEALTH_MODE (signal | static, mac dinh signal) va HEALTH_MAX_AGE_SECONDS
// (mac dinh 3).
func HealthConfig(getenv Getenv) (Health, error) {
	h := Health{MaxAge: 3 * time.Second}
	switch mode := strings.ToLower(strings.TrimSpace(getenv("ADMIT_HEALTH_MODE"))); mode {
	case "", "signal":
	case "static":
		h.Static = true
	default:
		return Health{}, fmt.Errorf("ADMIT_HEALTH_MODE=%q khong hop le (signal | static)", mode)
	}
	if raw := strings.TrimSpace(getenv("HEALTH_MAX_AGE_SECONDS")); raw != "" {
		v, err := strconv.ParseFloat(raw, 64)
		if err != nil || v <= 0 {
			return Health{}, fmt.Errorf("HEALTH_MAX_AGE_SECONDS=%q phai la so > 0", raw)
		}
		h.MaxAge = time.Duration(v * float64(time.Second))
	}
	return h, nil
}

// SplitCSV tach danh sach phan cach bang dau phay, bo phan tu rong.
func SplitCSV(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
