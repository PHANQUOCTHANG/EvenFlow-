package app

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"regexp"
	"time"

	"github.com/eventflow/eventflow/services/identity/internal/adapter/redis"
	"github.com/eventflow/eventflow/services/identity/internal/domain"
	"github.com/google/uuid"
)

var (
	ErrInvalidIdentifier = errors.New("invalid email or phone format")
	ErrPasswordTooShort  = errors.New("password must be at least 8 characters")
	ErrRateLimited       = errors.New("too many requests, please try again later")
	ErrInvalidOTP        = errors.New("invalid or expired OTP")
)

type RequestOTPInput struct {
	Identifier string // Email or Phone
}

type VerifyOTPInput struct {
	Identifier string
	Password   string
	OTP        string
}

type RegisterUseCase interface {
	RequestOTP(ctx context.Context, input RequestOTPInput) error
	VerifyOTP(ctx context.Context, input VerifyOTPInput) (*domain.Identity, error)
}

type registerUseCase struct {
	repo     domain.IdentityRepository
	hasher   domain.PasswordHasher
	redis    redis.Client
	notifier domain.OTPNotifier
}

func NewRegisterUseCase(repo domain.IdentityRepository, hasher domain.PasswordHasher, redisClient redis.Client, notifier domain.OTPNotifier) RegisterUseCase {
	return &registerUseCase{
		repo:     repo,
		hasher:   hasher,
		redis:    redisClient,
		notifier: notifier,
	}
}

func (uc *registerUseCase) RequestOTP(ctx context.Context, input RequestOTPInput) error {
	if !isValidIdentifier(input.Identifier) {
		return ErrInvalidIdentifier
	}

	// 0. Check if user already exists
	existing, err := uc.repo.FindByEmailOrPhone(ctx, input.Identifier)
	if err != nil && !errors.Is(err, domain.ErrIdentityNotFound) {
		return err // DB error
	}
	if existing != nil {
		return domain.ErrIdentityExists
	}

	// 1. Rate Limiting (Anti-Spam)
	// Key format: rate:otp:0987654321
	rateKey := fmt.Sprintf("rate:otp:%s", input.Identifier)
	count, err := uc.redis.IncrementRateLimit(ctx, rateKey, 5*time.Minute)
	if err != nil {
		return err
	}
	if count > 3 {
		return ErrRateLimited
	}

	// 2. Generate 6-digit OTP
	rand.Seed(time.Now().UnixNano())
	otpCode := fmt.Sprintf("%06d", rand.Intn(1000000))

	// 3. Save OTP to Redis with 5 minutes TTL
	err = uc.redis.SetOTP(ctx, input.Identifier, otpCode, 5*time.Minute)
	if err != nil {
		return err
	}

	// 4. Publish Event or Send Email Directly
	if isEmail(input.Identifier) && uc.notifier != nil {
		fmt.Printf("[INFO] Gửi OTP thực tế qua Email tới: %s\n", input.Identifier)
		go func() {
			// Gửi trong goroutine để không block request
			err := uc.notifier.SendOTP(context.Background(), input.Identifier, otpCode)
			if err != nil {
				fmt.Printf("[ERROR] Không thể gửi Email OTP: %v\n", err)
			}
		}()
	} else {
		// Mock (cho SĐT vì chưa tích hợp Twilio/Zalo)
		fmt.Printf("[MOCK] SENDING OTP: %s to %s\n", otpCode, input.Identifier)
	}

	return nil
}

func (uc *registerUseCase) VerifyOTP(ctx context.Context, input VerifyOTPInput) (*domain.Identity, error) {
	// 0. Anti Brute-force OTP
	verifyRateKey := fmt.Sprintf("rate:verify_otp:%s", input.Identifier)
	count, _ := uc.redis.IncrementRateLimit(ctx, verifyRateKey, 5*time.Minute)
	if count > 5 {
		_ = uc.redis.DeleteOTP(ctx, input.Identifier)
		return nil, fmt.Errorf("nhập sai quá nhiều lần, mã OTP đã bị vô hiệu hóa. Vui lòng lấy mã mới")
	}

	// 1. Get OTP from Redis
	storedOTP, err := uc.redis.GetOTP(ctx, input.Identifier)
	if err != nil || storedOTP != input.OTP {
		return nil, ErrInvalidOTP
	}

	if len(input.Password) < 8 {
		return nil, ErrPasswordTooShort
	}

	// 2. Hash Password
	hashed, err := uc.hasher.Hash(input.Password)
	if err != nil {
		return nil, err
	}

	// 3. Create Identity in Database (using current schema for now)
	now := time.Now()
	identity := &domain.Identity{
		ID:           uuid.New(),
		PasswordHash: hashed,
		Role:         domain.RoleBuyer,
		CreatedAt:    now,
	}
	
	// Determine if email or phone (basic check for '@')
	if isEmail(input.Identifier) {
		identity.Email = &input.Identifier
	} else {
		// Mock hashing phone if needed, or store directly based on new schema
		identity.PhoneHash = &input.Identifier 
	}

	// 4. Save to DB
	err = uc.repo.Save(ctx, identity)
	if err != nil {
		return nil, err
	}

	// 5. Clean up OTP from Redis to prevent reuse
	_ = uc.redis.DeleteOTP(ctx, input.Identifier)

	return identity, nil
}

func isValidIdentifier(id string) bool {
	// Basic regex for email or Vietnam phone number
	isEmail := regexp.MustCompile(`^[^\s@]+@[^\s@]+\.[^\s@]+$`).MatchString(id)
	isPhone := regexp.MustCompile(`^(84|0[3|5|7|8|9])+([0-9]{8})\b$`).MatchString(id)
	return isEmail || isPhone
}

func isEmail(id string) bool {
	return regexp.MustCompile(`^[^\s@]+@[^\s@]+\.[^\s@]+$`).MatchString(id)
}
