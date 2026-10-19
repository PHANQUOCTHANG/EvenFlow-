package app

import (
	"context"
	"errors"

	"github.com/eventflow/eventflow/services/identity/internal/domain"
)

var (
	ErrInvalidInput       = errors.New("identifier and password are required")
	ErrInvalidCredentials = errors.New("invalid email/phone or password")
)

type LoginInput struct {
	Identifier string // Email hoặc số điện thoại
	Password   string
	Device     *domain.DeviceInfo // tuy chon: gan thiet bi cho phien (EV-113)
}

type LoginOutput struct {
	Identity *domain.Identity
	Tokens   *domain.TokenPair
}

type LoginUseCase interface {
	Execute(ctx context.Context, input LoginInput) (*LoginOutput, error)
}

type loginUseCase struct {
	repo         domain.IdentityRepository
	hasher       domain.PasswordHasher
	tokenGen     domain.TokenGenerator
	devices      domain.DeviceRegistrar
}

// LoginOption tuỳ chỉnh use case đăng nhập.
type LoginOption func(*loginUseCase)

// WithDeviceRegistrar cho phép đăng nhập gắn thiết bị cho phiên (EV-113).
func WithDeviceRegistrar(d domain.DeviceRegistrar) LoginOption {
	return func(uc *loginUseCase) { uc.devices = d }
}

func NewLoginUseCase(repo domain.IdentityRepository, hasher domain.PasswordHasher, tokenGen domain.TokenGenerator, opts ...LoginOption) LoginUseCase {
	uc := &loginUseCase{
		repo:         repo,
		hasher:       hasher,
		tokenGen:     tokenGen,
	}
	for _, o := range opts {
		o(uc)
	}
	return uc
}

func (uc *loginUseCase) Execute(ctx context.Context, input LoginInput) (*LoginOutput, error) {
	if input.Identifier == "" || input.Password == "" {
		return nil, ErrInvalidInput
	}

	// 1. Tìm Identity theo Email hoặc Số điện thoại
	identity, err := uc.repo.FindByEmailOrPhone(ctx, input.Identifier)
	if err != nil {
		if errors.Is(err, domain.ErrIdentityNotFound) {
			// BẢO MẬT: Chống dò quét tài khoản (User Enumeration).
			return nil, ErrInvalidCredentials
		}
		return nil, err
	}

	// 2. Xác minh mật khẩu với bản băm Argon2id
	match, err := uc.hasher.Verify(input.Password, identity.PasswordHash)
	if err != nil {
		return nil, err
	}

	if !match {
		return nil, ErrInvalidCredentials
	}

	// 2b. Gắn thiết bị: access token mang claim "did", queue token (EV-137) cần nó.
	if input.Device != nil {
		if uc.devices == nil {
			return nil, errors.New("khong cau hinh dang ky thiet bi")
		}
		deviceID, err := uc.devices.Register(ctx, identity.ID, *input.Device)
		if err != nil {
			return nil, err
		}
		ctx = domain.ContextWithDevice(ctx, deviceID)
	}

	// 3. Mở phiên: access token 15 phút + refresh token xoay vòng (EV-112)
	tokens, err := uc.tokenGen.GenerateTokens(ctx, identity)
	if err != nil {
		return nil, err
	}

	return &LoginOutput{
		Identity: identity,
		Tokens:   tokens,
	}, nil
}
