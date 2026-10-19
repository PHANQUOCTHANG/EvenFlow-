package app

import (
	"context"
	crand "crypto/rand"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"regexp"
	"time"

	"github.com/eventflow/eventflow/libs/go/pii"
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
	devOTP   DevOTPSink
}

// DevOTPSink nhận OTP khi không có kênh gửi thật (SĐT chưa có SMS, email chưa cấu hình SMTP).
// Chỉ gắn ở môi trường dev: OTP KHÔNG BAO GIỜ được ghi vào log (EV-115).
type DevOTPSink func(identifier, otp string)

// RegisterOption tuỳ chỉnh use case đăng ký.
type RegisterOption func(*registerUseCase)

// WithDevOTPSink gắn nơi nhận OTP ở môi trường dev.
func WithDevOTPSink(s DevOTPSink) RegisterOption {
	return func(uc *registerUseCase) { uc.devOTP = s }
}

func NewRegisterUseCase(repo domain.IdentityRepository, hasher domain.PasswordHasher, redisClient redis.Client, notifier domain.OTPNotifier, opts ...RegisterOption) RegisterUseCase {
	uc := &registerUseCase{
		repo:     repo,
		hasher:   hasher,
		redis:    redisClient,
		notifier: notifier,
	}
	for _, o := range opts {
		o(uc)
	}
	return uc
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
	otpCode, err := newOTP()
	if err != nil {
		return err
	}

	// 3. Save OTP to Redis with 5 minutes TTL
	err = uc.redis.SetOTP(ctx, input.Identifier, otpCode, 5*time.Minute)
	if err != nil {
		return err
	}

	// 4. Publish Event or Send Email Directly
	if isEmail(input.Identifier) && uc.notifier != nil {
		slog.InfoContext(ctx, "gui OTP dang ky qua email", "email", input.Identifier)
		go func() {
			// Gửi trong goroutine để không block request
			err := uc.notifier.SendOTP(context.Background(), input.Identifier, otpCode)
			if err != nil {
				slog.Error("khong gui duoc email OTP", "email", input.Identifier, "err", err)
			}
		}()
	} else if uc.devOTP != nil {
		// SĐT chưa có SMS (Telegram bot đọc OTP từ Redis); dev thì đưa OTP ra kênh riêng.
		uc.devOTP(input.Identifier, otpCode)
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
		// EV-115: chỉ giữ số đã chuẩn hoá trong bộ nhớ; repository lưu HMAC + bản mã AES-GCM.
		phone, ok := pii.NormalizePhoneVN(input.Identifier)
		if !ok {
			return nil, ErrInvalidIdentifier
		}
		identity.Phone = &phone
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

// newOTP sinh mã 6 chữ số từ crypto/rand. math/rand gieo bằng thời gian thì ai biết lúc
// request được gửi có thể đoán lại mã.
func newOTP() (string, error) {
	n, err := crand.Int(crand.Reader, big.NewInt(1_000_000))
	if err != nil {
		return "", fmt.Errorf("sinh OTP: %w", err)
	}
	return fmt.Sprintf("%06d", n.Int64()), nil
}

func isValidIdentifier(id string) bool {
	// Basic regex for email or Vietnam phone number
	isEmail := regexp.MustCompile(`^[^\s@]+@[^\s@]+\.[^\s@]+$`).MatchString(id)
	_, isPhone := pii.NormalizePhoneVN(id)
	return isEmail || isPhone
}

func isEmail(id string) bool {
	return regexp.MustCompile(`^[^\s@]+@[^\s@]+\.[^\s@]+$`).MatchString(id)
}
