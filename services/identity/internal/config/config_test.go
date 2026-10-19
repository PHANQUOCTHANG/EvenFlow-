package config_test

import (
	"testing"

	"github.com/eventflow/eventflow/libs/go/pii"
	"github.com/eventflow/eventflow/services/identity/internal/config"
)

func TestLoad_DevDungKhoaPIIMacDinhHopLe(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x")
	t.Setenv("APP_ENV", "")
	t.Setenv("PII_ENC_KEY", "")
	t.Setenv("PII_HASH_KEY", "")
	c := config.Load()
	if !c.DevPIIKeys {
		t.Fatal("thieu khoa o dev phai danh dau DevPIIKeys")
	}
	if _, err := pii.NewProtector(c.PIIEncKey, c.PIIHashKey); err != nil {
		t.Fatalf("khoa dev phai hop le: %v", err)
	}
}

func TestLoad_ProductionBatBuocKhoa(t *testing.T) {
	for name, env := range map[string]map[string]string{
		"thieu khoa PII": {"JWT_SECRET": "s", "PII_ENC_KEY": "", "PII_HASH_KEY": ""},
		"thieu JWT":      {"JWT_SECRET": "", "PII_ENC_KEY": config.DevPIIEncKey, "PII_HASH_KEY": config.DevPIIHashKey},
	} {
		t.Run(name, func(t *testing.T) {
			t.Setenv("DATABASE_URL", "postgres://x")
			t.Setenv("APP_ENV", "production")
			for k, v := range env {
				t.Setenv(k, v)
			}
			defer func() {
				if recover() == nil {
					t.Fatal("production thieu khoa phai dung khoi dong")
				}
			}()
			config.Load()
		})
	}
}
