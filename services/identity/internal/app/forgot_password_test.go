package app_test

import (
	"context"
	"errors"
	"testing"

	"github.com/eventflow/eventflow/services/identity/internal/app"
	"github.com/eventflow/eventflow/services/identity/internal/domain"
	"github.com/google/uuid"
)

func TestForgotPasswordUseCase_RequestResetOTP(t *testing.T) {
	ctx := context.Background()

	t.Run("invalid identifier", func(t *testing.T) {
		uc := app.NewForgotPasswordUseCase(newFakeRepo(), &fakePasswordHasher{}, newFakeRedis(), &fakeNotifier{})
		err := uc.RequestResetOTP(ctx, "invalid")
		if !errors.Is(err, app.ErrInvalidIdentifier) {
			t.Errorf("expected ErrInvalidIdentifier, got %v", err)
		}
	})

	t.Run("account not found", func(t *testing.T) {
		repo := newFakeRepo()
		uc := app.NewForgotPasswordUseCase(repo, &fakePasswordHasher{}, newFakeRedis(), &fakeNotifier{})
		err := uc.RequestResetOTP(ctx, "nonexistent@example.com")
		if err == nil || err.Error() != "tài khoản không tồn tại" {
			t.Errorf("expected tài khoản không tồn tại error, got %v", err)
		}
	})

	t.Run("repo error", func(t *testing.T) {
		repo := newFakeRepo()
		repo.findErr = errors.New("db find failed")
		uc := app.NewForgotPasswordUseCase(repo, &fakePasswordHasher{}, newFakeRedis(), &fakeNotifier{})
		err := uc.RequestResetOTP(ctx, "test@example.com")
		if err == nil || err.Error() != "db find failed" {
			t.Errorf("expected db find failed error, got %v", err)
		}
	})

	t.Run("rate limit error", func(t *testing.T) {
		repo := newFakeRepo()
		email := "test@example.com"
		repo.identities[email] = &domain.Identity{Email: &email}
		redisClient := newFakeRedis()
		redisClient.rateErr = errors.New("redis rate error")
		uc := app.NewForgotPasswordUseCase(repo, &fakePasswordHasher{}, redisClient, &fakeNotifier{})
		err := uc.RequestResetOTP(ctx, email)
		if err == nil || err.Error() != "redis rate error" {
			t.Errorf("expected redis rate error, got %v", err)
		}
	})

	t.Run("rate limit exceeded", func(t *testing.T) {
		repo := newFakeRepo()
		email := "test@example.com"
		repo.identities[email] = &domain.Identity{Email: &email}
		redisClient := newFakeRedis()
		redisClient.rateLimits["rate:reset_otp:"+email] = 3 // next is 4 > 3
		uc := app.NewForgotPasswordUseCase(repo, &fakePasswordHasher{}, redisClient, &fakeNotifier{})
		err := uc.RequestResetOTP(ctx, email)
		if !errors.Is(err, app.ErrRateLimited) {
			t.Errorf("expected ErrRateLimited, got %v", err)
		}
	})

	t.Run("redis set OTP error", func(t *testing.T) {
		repo := newFakeRepo()
		email := "test@example.com"
		repo.identities[email] = &domain.Identity{Email: &email}
		redisClient := newFakeRedis()
		redisClient.setOTPErr = errors.New("redis set error")
		uc := app.NewForgotPasswordUseCase(repo, &fakePasswordHasher{}, redisClient, &fakeNotifier{})
		err := uc.RequestResetOTP(ctx, email)
		if err == nil || err.Error() != "redis set error" {
			t.Errorf("expected redis set error, got %v", err)
		}
	})

	t.Run("success email", func(t *testing.T) {
		repo := newFakeRepo()
		email := "test@example.com"
		repo.identities[email] = &domain.Identity{Email: &email}
		redisClient := newFakeRedis()
		notifier := &fakeNotifier{}
		uc := app.NewForgotPasswordUseCase(repo, &fakePasswordHasher{}, redisClient, notifier)
		err := uc.RequestResetOTP(ctx, email)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("success phone", func(t *testing.T) {
		repo := newFakeRepo()
		phone := "0987654321"
		repo.identities[phone] = &domain.Identity{PhoneHash: &phone}
		redisClient := newFakeRedis()
		uc := app.NewForgotPasswordUseCase(repo, &fakePasswordHasher{}, redisClient, nil)
		err := uc.RequestResetOTP(ctx, phone)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestForgotPasswordUseCase_ResetPassword(t *testing.T) {
	ctx := context.Background()

	t.Run("brute force protection", func(t *testing.T) {
		redisClient := newFakeRedis()
		redisClient.rateLimits["rate:verify_reset_otp:test@example.com"] = 5 // next is 6 > 5
		redisClient.otpStore["reset_test@example.com"] = "123456"
		uc := app.NewForgotPasswordUseCase(newFakeRepo(), &fakePasswordHasher{}, redisClient, &fakeNotifier{})

		err := uc.ResetPassword(ctx, app.ResetPasswordInput{
			Identifier:  "test@example.com",
			OTP:         "123456",
			NewPassword: "newpassword123",
		})
		if err == nil {
			t.Fatal("expected brute force error")
		}
		if _, exists := redisClient.otpStore["reset_test@example.com"]; exists {
			t.Errorf("expected OTP to be deleted after brute force limit")
		}
	})

	t.Run("password too short", func(t *testing.T) {
		redisClient := newFakeRedis()
		uc := app.NewForgotPasswordUseCase(newFakeRepo(), &fakePasswordHasher{}, redisClient, &fakeNotifier{})
		err := uc.ResetPassword(ctx, app.ResetPasswordInput{
			Identifier:  "test@example.com",
			OTP:         "123456",
			NewPassword: "short",
		})
		if !errors.Is(err, app.ErrPasswordTooShort) {
			t.Errorf("expected ErrPasswordTooShort, got %v", err)
		}
	})

	t.Run("invalid or expired OTP", func(t *testing.T) {
		redisClient := newFakeRedis()
		redisClient.otpStore["reset_test@example.com"] = "123456"
		uc := app.NewForgotPasswordUseCase(newFakeRepo(), &fakePasswordHasher{}, redisClient, &fakeNotifier{})
		err := uc.ResetPassword(ctx, app.ResetPasswordInput{
			Identifier:  "test@example.com",
			OTP:         "wrong",
			NewPassword: "newpassword123",
		})
		if !errors.Is(err, app.ErrInvalidOTP) {
			t.Errorf("expected ErrInvalidOTP, got %v", err)
		}
	})

	t.Run("user find error", func(t *testing.T) {
		redisClient := newFakeRedis()
		redisClient.otpStore["reset_test@example.com"] = "123456"
		repo := newFakeRepo()
		repo.findErr = errors.New("user not found in db")
		uc := app.NewForgotPasswordUseCase(repo, &fakePasswordHasher{}, redisClient, &fakeNotifier{})

		err := uc.ResetPassword(ctx, app.ResetPasswordInput{
			Identifier:  "test@example.com",
			OTP:         "123456",
			NewPassword: "newpassword123",
		})
		if err == nil || err.Error() != "user not found in db" {
			t.Errorf("expected user not found error, got %v", err)
		}
	})

	t.Run("hasher error", func(t *testing.T) {
		redisClient := newFakeRedis()
		redisClient.otpStore["reset_test@example.com"] = "123456"
		repo := newFakeRepo()
		email := "test@example.com"
		repo.identities[email] = &domain.Identity{ID: uuid.New(), Email: &email}
		hasher := &fakePasswordHasher{hashErr: errors.New("hash error")}
		uc := app.NewForgotPasswordUseCase(repo, hasher, redisClient, &fakeNotifier{})

		err := uc.ResetPassword(ctx, app.ResetPasswordInput{
			Identifier:  email,
			OTP:         "123456",
			NewPassword: "newpassword123",
		})
		if err == nil || err.Error() != "hash error" {
			t.Errorf("expected hash error, got %v", err)
		}
	})

	t.Run("repo update password error", func(t *testing.T) {
		redisClient := newFakeRedis()
		redisClient.otpStore["reset_test@example.com"] = "123456"
		repo := newFakeRepo()
		email := "test@example.com"
		repo.identities[email] = &domain.Identity{ID: uuid.New(), Email: &email}
		repo.updateErr = errors.New("update password failed")
		uc := app.NewForgotPasswordUseCase(repo, &fakePasswordHasher{}, redisClient, &fakeNotifier{})

		err := uc.ResetPassword(ctx, app.ResetPasswordInput{
			Identifier:  email,
			OTP:         "123456",
			NewPassword: "newpassword123",
		})
		if err == nil || err.Error() != "update password failed" {
			t.Errorf("expected update password failed error, got %v", err)
		}
	})

	t.Run("successful password reset", func(t *testing.T) {
		redisClient := newFakeRedis()
		redisClient.otpStore["reset_test@example.com"] = "123456"
		repo := newFakeRepo()
		email := "test@example.com"
		userID := uuid.New()
		repo.identities[email] = &domain.Identity{ID: userID, Email: &email, PasswordHash: "old_hash"}
		uc := app.NewForgotPasswordUseCase(repo, &fakePasswordHasher{}, redisClient, &fakeNotifier{})

		err := uc.ResetPassword(ctx, app.ResetPasswordInput{
			Identifier:  email,
			OTP:         "123456",
			NewPassword: "brandNewPassword123",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if repo.identities[email].PasswordHash != "hashed:brandNewPassword123" {
			t.Errorf("expected updated password hash, got %s", repo.identities[email].PasswordHash)
		}
	})
}
