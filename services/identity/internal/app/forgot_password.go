package app

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"time"

	"github.com/eventflow/eventflow/services/identity/internal/adapter/redis"
	"github.com/eventflow/eventflow/services/identity/internal/domain"
)

type ResetPasswordInput struct {
	Identifier string
	OTP        string
	NewPassword string
}

type ForgotPasswordUseCase interface {
	RequestResetOTP(ctx context.Context, identifier string) error
	ResetPassword(ctx context.Context, input ResetPasswordInput) error
}

type forgotPasswordUseCase struct {
	repo     domain.IdentityRepository
	hasher   domain.PasswordHasher
	redis    redis.Client
	notifier domain.OTPNotifier
}

func NewForgotPasswordUseCase(repo domain.IdentityRepository, hasher domain.PasswordHasher, redisClient redis.Client, notifier domain.OTPNotifier) ForgotPasswordUseCase {
	return &forgotPasswordUseCase{
		repo:     repo,
		hasher:   hasher,
		redis:    redisClient,
		notifier: notifier,
	}
}

func (uc *forgotPasswordUseCase) RequestResetOTP(ctx context.Context, identifier string) error {
	if !isValidIdentifier(identifier) {
		return ErrInvalidIdentifier
	}

	// 1. Check if user exists
	_, err := uc.repo.FindByEmailOrPhone(ctx, identifier)
	if err != nil {
		if errors.Is(err, domain.ErrIdentityNotFound) {
			return errors.New("tài khoản không tồn tại")
		}
		return err // DB error
	}

	// 2. Rate Limiting (Anti-Spam) - using prefix "rate:reset_otp:"
	rateKey := fmt.Sprintf("rate:reset_otp:%s", identifier)
	count, err := uc.redis.IncrementRateLimit(ctx, rateKey, 5*time.Minute)
	if err != nil {
		return err
	}
	if count > 3 {
		return ErrRateLimited
	}

	// 3. Generate 6-digit OTP
	rand.Seed(time.Now().UnixNano())
	otpCode := fmt.Sprintf("%06d", rand.Intn(1000000))

	// 4. Save OTP to Redis with prefix "reset_"
	// Note: SetOTP appends "otp:" inside, so actual key will be "otp:reset_0987654321"
	err = uc.redis.SetOTP(ctx, "reset_"+identifier, otpCode, 5*time.Minute)
	if err != nil {
		return err
	}

	// 5. Publish Event or Send Email Directly
	if isEmail(identifier) && uc.notifier != nil {
		fmt.Printf("[INFO] Gửi OTP khôi phục mật khẩu qua Email tới: %s\n", identifier)
		go func() {
			err := uc.notifier.SendOTP(context.Background(), identifier, otpCode)
			if err != nil {
				fmt.Printf("[ERROR] Không thể gửi Email OTP: %v\n", err)
			}
		}()
	} else {
		// Mock (cho SĐT vì chưa tích hợp Twilio/Zalo)
		fmt.Printf("[MOCK] SENDING RESET OTP: %s to %s\n", otpCode, identifier)
	}

	return nil
}

func (uc *forgotPasswordUseCase) ResetPassword(ctx context.Context, input ResetPasswordInput) error {
	// 0. Anti Brute-force OTP
	verifyRateKey := fmt.Sprintf("rate:verify_reset_otp:%s", input.Identifier)
	count, _ := uc.redis.IncrementRateLimit(ctx, verifyRateKey, 5*time.Minute)
	if count > 5 {
		_ = uc.redis.DeleteOTP(ctx, "reset_"+input.Identifier)
		return fmt.Errorf("nhập sai quá nhiều lần, mã OTP đã bị vô hiệu hóa. Vui lòng lấy mã mới")
	}

	// 1. Check length
	if len(input.NewPassword) < 8 {
		return ErrPasswordTooShort
	}

	// 2. Get OTP from Redis (with "reset_" prefix)
	storedOTP, err := uc.redis.GetOTP(ctx, "reset_"+input.Identifier)
	if err != nil || storedOTP != input.OTP {
		return ErrInvalidOTP
	}

	// 3. Verify user exists
	identity, err := uc.repo.FindByEmailOrPhone(ctx, input.Identifier)
	if err != nil {
		return err
	}

	// 4. Hash new password
	hashed, err := uc.hasher.Hash(input.NewPassword)
	if err != nil {
		return err
	}

	// 5. Save to DB
	err = uc.repo.UpdatePassword(ctx, identity.ID, hashed)
	if err != nil {
		return err
	}

	// 6. Clean up OTP from Redis
	_ = uc.redis.DeleteOTP(ctx, "reset_"+input.Identifier)

	return nil
}
