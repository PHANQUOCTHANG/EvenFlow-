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

type LoginUseCase interface {
	Execute(ctx context.Context, input LoginInput) (*domain.Identity, error)
}

type loginUseCase struct {
	repo   domain.IdentityRepository
	hasher domain.PasswordHasher
}

func NewLoginUseCase(repo domain.IdentityRepository, hasher domain.PasswordHasher) LoginUseCase {
	return &loginUseCase{
		repo:   repo,
		hasher: hasher,
	}
}

func (uc *loginUseCase) Execute(ctx context.Context, input LoginInput) (*domain.Identity, error) {
	if input.Identifier == "" || input.Password == "" {
		return nil, ErrInvalidInput
	}

	// 1. Tìm Identity theo Email hoặc Số điện thoại
	identity, err := uc.repo.FindByEmailOrPhone(ctx, input.Identifier)
	if err != nil {
		if errors.Is(err, domain.ErrIdentityNotFound) {
			// BẢO MẬT: Chống dò quét tài khoản (User Enumeration).
			// Dù không tìm thấy email, ta vẫn trả về lỗi chung là "Sai thông tin đăng nhập".
			// Không bao giờ báo cho hacker biết là "email này không tồn tại".
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
		// BẢO MẬT: Lỗi chung, giống hệt lỗi không tìm thấy email
		return nil, ErrInvalidCredentials
	}

	// Thành công
	// (Ghi chú: Tại task EV-110 chỉ trả về Identity. Việc cấp JWT Token sẽ làm ở task EV-112).
	return identity, nil
}
