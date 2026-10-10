package app

import (
	"context"
	"errors"
	"time"

	"github.com/eventflow/eventflow/services/identity/internal/domain"
	"github.com/google/uuid"
	"google.golang.org/api/idtoken"
)

var (
	ErrInvalidGoogleToken = errors.New("invalid google token")
)

type GoogleLoginInput struct {
	IDToken string
}

type GoogleLoginOutput struct {
	Identity *domain.Identity
	Token    string
}

type GoogleLoginUseCase interface {
	Execute(ctx context.Context, input GoogleLoginInput) (*GoogleLoginOutput, error)
}

type googleLoginUseCase struct {
	repo     domain.IdentityRepository
	hasher   domain.PasswordHasher
	jwtSvc   domain.TokenGenerator
	clientID string
}

func NewGoogleLoginUseCase(repo domain.IdentityRepository, hasher domain.PasswordHasher, jwtSvc domain.TokenGenerator, clientID string) GoogleLoginUseCase {
	return &googleLoginUseCase{
		repo:     repo,
		hasher:   hasher,
		jwtSvc:   jwtSvc,
		clientID: clientID,
	}
}

func (uc *googleLoginUseCase) Execute(ctx context.Context, input GoogleLoginInput) (*GoogleLoginOutput, error) {
	// 1. Verify the Google ID Token
	payload, err := idtoken.Validate(ctx, input.IDToken, uc.clientID)
	if err != nil {
		return nil, ErrInvalidGoogleToken
	}

	email, ok := payload.Claims["email"].(string)
	if !ok || email == "" {
		return nil, errors.New("email not provided by Google")
	}

	// 2. Check if identity already exists
	identity, err := uc.repo.FindByEmailOrPhone(ctx, email)
	if err != nil {
		if errors.Is(err, domain.ErrIdentityNotFound) {
			// 3. Create new identity
			hashed, _ := uc.hasher.Hash(uuid.New().String()) // Random unguessable password
			
			now := time.Now()
			identity = &domain.Identity{
				ID:           uuid.New(),
				Email:        &email,
				PasswordHash: hashed,
				Role:         domain.RoleBuyer,
				CreatedAt:    now,
			}
			err = uc.repo.Save(ctx, identity)
			if err != nil {
				return nil, err
			}
		} else {
			return nil, err
		}
	}

	// 4. Generate JWT
	token, err := uc.jwtSvc.GenerateToken(identity)
	if err != nil {
		return nil, err
	}

	return &GoogleLoginOutput{
		Identity: identity,
		Token:    token,
	}, nil
}
