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
}

func NewLoginUseCase(repo domain.IdentityRepository, hasher domain.PasswordHasher, tokenGen domain.TokenGenerator) LoginUseCase {
	return &loginUseCase{
		repo:         repo,
		hasher:       hasher,
		tokenGen:     tokenGen,
	}
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
