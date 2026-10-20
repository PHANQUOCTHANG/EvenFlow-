package authx_test

import (
	"errors"
	"testing"

	"github.com/eventflow/eventflow/libs/go/authx"
)

func TestQueueTokenSecretFromEnv(t *testing.T) {
	t.Run("dev thieu secret dung mac dinh", func(t *testing.T) {
		t.Setenv("QUEUE_TOKEN_SECRET", "")
		t.Setenv("APP_ENV", "")
		s, dev, err := authx.QueueTokenSecretFromEnv()
		if err != nil || !dev || string(s) != authx.DevQueueTokenSecret {
			t.Fatalf("%q dev=%v err=%v", s, dev, err)
		}
	})
	t.Run("production thieu secret bi tu choi", func(t *testing.T) {
		t.Setenv("QUEUE_TOKEN_SECRET", "")
		t.Setenv("APP_ENV", "Production")
		if _, _, err := authx.QueueTokenSecretFromEnv(); err == nil {
			t.Fatal("phai loi")
		}
	})
	t.Run("secret ngan bi tu choi", func(t *testing.T) {
		t.Setenv("QUEUE_TOKEN_SECRET", "short")
		if _, _, err := authx.QueueTokenSecretFromEnv(); !errors.Is(err, authx.ErrInvalidSecret) {
			t.Fatalf("muon ErrInvalidSecret, nhan %v", err)
		}
	})
	t.Run("secret hop le", func(t *testing.T) {
		t.Setenv("QUEUE_TOKEN_SECRET", "a-real-secret-0123456789")
		t.Setenv("APP_ENV", "production")
		s, dev, err := authx.QueueTokenSecretFromEnv()
		if err != nil || dev || string(s) != "a-real-secret-0123456789" {
			t.Fatalf("%q dev=%v err=%v", s, dev, err)
		}
	})
}
