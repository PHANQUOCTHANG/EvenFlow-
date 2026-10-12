package app_test

import (
	"context"
	"errors"
	"testing"

	"github.com/eventflow/eventflow/services/identity/internal/app"
	"github.com/eventflow/eventflow/services/identity/internal/domain"
)

func TestRegisterUseCase_RequestOTP(t *testing.T) {
	ctx := context.Background()

	t.Run("invalid identifier", func(t *testing.T) {
		uc := app.NewRegisterUseCase(newFakeRepo(), &fakePasswordHasher{}, newFakeRedis(), &fakeNotifier{})
		err := uc.RequestOTP(ctx, app.RequestOTPInput{Identifier: "invalid_id"})
		if !errors.Is(err, app.ErrInvalidIdentifier) {
			t.Errorf("expected ErrInvalidIdentifier, got %v", err)
		}
	})

	t.Run("existing user returns ErrIdentityExists", func(t *testing.T) {
		repo := newFakeRepo()
		email := "test@example.com"
		repo.identities[email] = &domain.Identity{Email: &email}
		uc := app.NewRegisterUseCase(repo, &fakePasswordHasher{}, newFakeRedis(), &fakeNotifier{})
		err := uc.RequestOTP(ctx, app.RequestOTPInput{Identifier: email})
		if !errors.Is(err, domain.ErrIdentityExists) {
			t.Errorf("expected ErrIdentityExists, got %v", err)
		}
	})

	t.Run("repo error returns err", func(t *testing.T) {
		repo := newFakeRepo()
		repo.findErr = errors.New("db error")
		uc := app.NewRegisterUseCase(repo, &fakePasswordHasher{}, newFakeRedis(), &fakeNotifier{})
		err := uc.RequestOTP(ctx, app.RequestOTPInput{Identifier: "test@example.com"})
		if err == nil || err.Error() != "db error" {
			t.Errorf("expected db error, got %v", err)
		}
	})

	t.Run("rate limit error", func(t *testing.T) {
		repo := newFakeRepo()
		redisClient := newFakeRedis()
		redisClient.rateErr = errors.New("redis connection error")
		uc := app.NewRegisterUseCase(repo, &fakePasswordHasher{}, redisClient, &fakeNotifier{})
		err := uc.RequestOTP(ctx, app.RequestOTPInput{Identifier: "test@example.com"})
		if err == nil || err.Error() != "redis connection error" {
			t.Errorf("expected redis connection error, got %v", err)
		}
	})

	t.Run("rate limit exceeded", func(t *testing.T) {
		repo := newFakeRepo()
		redisClient := newFakeRedis()
		redisClient.rateLimits["rate:otp:test@example.com"] = 3 // next will be 4 > 3
		uc := app.NewRegisterUseCase(repo, &fakePasswordHasher{}, redisClient, &fakeNotifier{})
		err := uc.RequestOTP(ctx, app.RequestOTPInput{Identifier: "test@example.com"})
		if !errors.Is(err, app.ErrRateLimited) {
			t.Errorf("expected ErrRateLimited, got %v", err)
		}
	})

	t.Run("redis set OTP error", func(t *testing.T) {
		repo := newFakeRepo()
		redisClient := newFakeRedis()
		redisClient.setOTPErr = errors.New("redis set error")
		uc := app.NewRegisterUseCase(repo, &fakePasswordHasher{}, redisClient, &fakeNotifier{})
		err := uc.RequestOTP(ctx, app.RequestOTPInput{Identifier: "test@example.com"})
		if err == nil || err.Error() != "redis set error" {
			t.Errorf("expected redis set error, got %v", err)
		}
	})

	t.Run("success email", func(t *testing.T) {
		repo := newFakeRepo()
		redisClient := newFakeRedis()
		notifier := &fakeNotifier{}
		uc := app.NewRegisterUseCase(repo, &fakePasswordHasher{}, redisClient, notifier)
		err := uc.RequestOTP(ctx, app.RequestOTPInput{Identifier: "test@example.com"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("success phone", func(t *testing.T) {
		repo := newFakeRepo()
		redisClient := newFakeRedis()
		uc := app.NewRegisterUseCase(repo, &fakePasswordHasher{}, redisClient, nil)
		err := uc.RequestOTP(ctx, app.RequestOTPInput{Identifier: "0987654321"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if redisClient.otpStore["0987654321"] == "" {
			t.Errorf("expected OTP to be stored for phone")
		}
	})
}

func TestRegisterUseCase_VerifyOTP(t *testing.T) {
	ctx := context.Background()

	t.Run("brute force protection", func(t *testing.T) {
		redisClient := newFakeRedis()
		redisClient.rateLimits["rate:verify_otp:test@example.com"] = 5 // next is 6 > 5
		redisClient.otpStore["test@example.com"] = "123456"
		uc := app.NewRegisterUseCase(newFakeRepo(), &fakePasswordHasher{}, redisClient, &fakeNotifier{})

		_, err := uc.VerifyOTP(ctx, app.VerifyOTPInput{
			Identifier: "test@example.com",
			Password:   "password123",
			OTP:        "123456",
		})
		if err == nil {
			t.Fatal("expected error due to brute force lock")
		}
		if _, exists := redisClient.otpStore["test@example.com"]; exists {
			t.Errorf("expected OTP to be deleted after brute force limit")
		}
	})

	t.Run("invalid or expired OTP", func(t *testing.T) {
		redisClient := newFakeRedis()
		redisClient.otpStore["test@example.com"] = "123456"
		uc := app.NewRegisterUseCase(newFakeRepo(), &fakePasswordHasher{}, redisClient, &fakeNotifier{})

		_, err := uc.VerifyOTP(ctx, app.VerifyOTPInput{
			Identifier: "test@example.com",
			Password:   "password123",
			OTP:        "654321", // Mismatch
		})
		if !errors.Is(err, app.ErrInvalidOTP) {
			t.Errorf("expected ErrInvalidOTP, got %v", err)
		}
	})

	t.Run("password too short", func(t *testing.T) {
		redisClient := newFakeRedis()
		redisClient.otpStore["test@example.com"] = "123456"
		uc := app.NewRegisterUseCase(newFakeRepo(), &fakePasswordHasher{}, redisClient, &fakeNotifier{})

		_, err := uc.VerifyOTP(ctx, app.VerifyOTPInput{
			Identifier: "test@example.com",
			Password:   "short",
			OTP:        "123456",
		})
		if !errors.Is(err, app.ErrPasswordTooShort) {
			t.Errorf("expected ErrPasswordTooShort, got %v", err)
		}
	})

	t.Run("hasher error", func(t *testing.T) {
		redisClient := newFakeRedis()
		redisClient.otpStore["test@example.com"] = "123456"
		hasher := &fakePasswordHasher{hashErr: errors.New("hash failed")}
		uc := app.NewRegisterUseCase(newFakeRepo(), hasher, redisClient, &fakeNotifier{})

		_, err := uc.VerifyOTP(ctx, app.VerifyOTPInput{
			Identifier: "test@example.com",
			Password:   "password123",
			OTP:        "123456",
		})
		if err == nil || err.Error() != "hash failed" {
			t.Errorf("expected hash failed error, got %v", err)
		}
	})

	t.Run("repo save error", func(t *testing.T) {
		redisClient := newFakeRedis()
		redisClient.otpStore["test@example.com"] = "123456"
		repo := newFakeRepo()
		repo.saveErr = errors.New("db save error")
		uc := app.NewRegisterUseCase(repo, &fakePasswordHasher{}, redisClient, &fakeNotifier{})

		_, err := uc.VerifyOTP(ctx, app.VerifyOTPInput{
			Identifier: "test@example.com",
			Password:   "password123",
			OTP:        "123456",
		})
		if err == nil || err.Error() != "db save error" {
			t.Errorf("expected db save error, got %v", err)
		}
	})

	t.Run("successful registration with email", func(t *testing.T) {
		redisClient := newFakeRedis()
		redisClient.otpStore["test@example.com"] = "123456"
		repo := newFakeRepo()
		uc := app.NewRegisterUseCase(repo, &fakePasswordHasher{}, redisClient, &fakeNotifier{})

		ident, err := uc.VerifyOTP(ctx, app.VerifyOTPInput{
			Identifier: "test@example.com",
			Password:   "password123",
			OTP:        "123456",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ident == nil || ident.Email == nil || *ident.Email != "test@example.com" {
			t.Errorf("unexpected registered identity: %v", ident)
		}
		if ident.Role != domain.RoleBuyer {
			t.Errorf("expected default role buyer, got %s", ident.Role)
		}
		if _, exists := redisClient.otpStore["test@example.com"]; exists {
			t.Errorf("expected OTP to be deleted after successful verification")
		}
	})

	t.Run("successful registration with phone", func(t *testing.T) {
		redisClient := newFakeRedis()
		phone := "0987654321"
		redisClient.otpStore[phone] = "123456"
		repo := newFakeRepo()
		uc := app.NewRegisterUseCase(repo, &fakePasswordHasher{}, redisClient, &fakeNotifier{})

		ident, err := uc.VerifyOTP(ctx, app.VerifyOTPInput{
			Identifier: phone,
			Password:   "password123",
			OTP:        "123456",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ident == nil || ident.PhoneHash == nil || *ident.PhoneHash != phone {
			t.Errorf("unexpected registered phone identity: %v", ident)
		}
	})
}
