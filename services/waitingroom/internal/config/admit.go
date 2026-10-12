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
	}
	return nil
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
