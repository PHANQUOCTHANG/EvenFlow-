package app

import (
	"context"
	"errors"
	"regexp"
	"time"

	"github.com/eventflow/eventflow/services/identity/internal/domain"
	"github.com/google/uuid"
)

var (
	ErrInvalidEmail     = errors.New("invalid email format")
	ErrPasswordTooShort = errors.New("password must be at least 8 characters")
)

type RegisterInput struct {
	Email    string
	Password string
}

type RegisterUseCase interface {
	Execute(ctx context.Context, input RegisterInput) (*domain.Identity, error)
}

type registerUseCase struct {
	repo   domain.IdentityRepository
	hasher domain.PasswordHasher
}

func NewRegisterUseCase(repo domain.IdentityRepository, hasher domain.PasswordHasher) RegisterUseCase {
	return &registerUseCase{
		repo:   repo,
		hasher: hasher,
	}
}

func (uc *registerUseCase) Execute(ctx context.Context, input RegisterInput) (*domain.Identity, error) {
	if !isValidEmail(input.Email) {
		return nil, ErrInvalidEmail
	}
	if len(input.Password) < 8 {
		return nil, ErrPasswordTooShort
	}

	// Băm mật khẩu (Argon2id)
	hashed, err := uc.hasher.Hash(input.Password)
	if err != nil {
		return nil, err
	}

	// Tạo thông tin người dùng mới (vai trò mặc định là Buyer)
	now := time.Now()
	email := input.Email
	identity := &domain.Identity{
		ID:           uuid.New(),
		Email:        &email,
		PasswordHash: hashed,
		Role:         domain.RoleBuyer,
		CreatedAt:    now,
	}

	// Lưu xuống CSDL qua interface Repository
	err = uc.repo.Save(ctx, identity)
	if err != nil {
		return nil, err
	}

	return identity, nil
}

func isValidEmail(email string) bool {
	// Kiểm tra định dạng email cơ bản
	re := regexp.MustCompile(`^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,4}$`)
	return re.MatchString(email)
}
