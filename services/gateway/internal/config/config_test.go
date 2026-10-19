package config_test

import (
	"testing"

	"github.com/eventflow/eventflow/services/gateway/internal/config"
)

func TestLoad_MacDinhChoDev(t *testing.T) {
	t.Setenv("JWT_SECRET", "")
	t.Setenv("APP_ENV", "")
	c, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if !c.DevSecret || c.JWTSecret != config.DevJWTSecret {
		t.Fatal("thieu JWT_SECRET o dev phai dung secret mac dinh va danh dau DevSecret")
	}
	if c.HTTPAddr != ":8080" || c.Upstreams[config.Identity].String() != "http://localhost:8083" {
		t.Fatalf("mac dinh sai: %s %s", c.HTTPAddr, c.Upstreams[config.Identity])
	}
	if r := c.Rates[config.RateQueuePoll]; r.PerSecond != 0.5 || r.Burst != 3 {
		t.Fatalf("rate queue_poll mac dinh sai: %+v", r)
	}
}

func TestLoad_ProductionBatBuocJWTSecret(t *testing.T) {
	t.Setenv("JWT_SECRET", "")
	t.Setenv("APP_ENV", "production")
	if _, err := config.Load(); err == nil {
		t.Fatal("production thieu JWT_SECRET phai loi")
	}
}

func TestLoad_DocBienMoiTruong(t *testing.T) {
	t.Setenv("JWT_SECRET", "s")
	t.Setenv("EVENT_URL", "http://event:8084")
	t.Setenv("CORS_ALLOWED_ORIGINS", "https://a.example, https://b.example")
	t.Setenv("RATE_DEFAULT", "50:100")
	c, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if c.Upstreams[config.Event].Host != "event:8084" {
		t.Fatalf("EVENT_URL = %s", c.Upstreams[config.Event])
	}
	if len(c.AllowedOrigins) != 2 || c.AllowedOrigins[1] != "https://b.example" {
		t.Fatalf("origins = %v", c.AllowedOrigins)
	}
	if r := c.Rates[config.RateDefault]; r.PerSecond != 50 || r.Burst != 100 {
		t.Fatalf("RATE_DEFAULT = %+v", r)
	}
}

func TestLoad_TuChoiCauHinhHong(t *testing.T) {
	for k, v := range map[string]string{
		"TICKETING_URL":   "khong-phai-url",
		"RATE_QUEUE_POLL": "nhanh",
		"RATE_AUTH":       "0:5",
		"RATE_DEFAULT":    "5:0",
	} {
		t.Run(k, func(t *testing.T) {
			t.Setenv(k, v)
			if _, err := config.Load(); err == nil {
				t.Fatalf("%s=%q phai loi", k, v)
			}
		})
	}
}
