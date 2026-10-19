package app_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/eventflow/eventflow/services/identity/internal/app"
	"github.com/eventflow/eventflow/services/identity/internal/domain"
	"github.com/google/uuid"
)

func TestLoginUseCase_Execute(t *testing.T) {
	ctx := context.Background()

	t.Run("invalid input", func(t *testing.T) {
		uc := app.NewLoginUseCase(newFakeRepo(), &fakePasswordHasher{}, &fakeTokenGen{})
		_, err := uc.Execute(ctx, app.LoginInput{})
		if !errors.Is(err, app.ErrInvalidInput) {
			t.Errorf("expected ErrInvalidInput, got %v", err)
		}
	})

	t.Run("user not found returns invalid credentials", func(t *testing.T) {
		repo := newFakeRepo()
		uc := app.NewLoginUseCase(repo, &fakePasswordHasher{}, &fakeTokenGen{})
		_, err := uc.Execute(ctx, app.LoginInput{
			Identifier: "nonexistent@example.com",
			Password:   "password123",
		})
		if !errors.Is(err, app.ErrInvalidCredentials) {
			t.Errorf("expected ErrInvalidCredentials, got %v", err)
		}
	})

	t.Run("db error", func(t *testing.T) {
		repo := newFakeRepo()
		repo.findErr = errors.New("db connection failed")
		uc := app.NewLoginUseCase(repo, &fakePasswordHasher{}, &fakeTokenGen{})
		_, err := uc.Execute(ctx, app.LoginInput{
			Identifier: "user@example.com",
			Password:   "password123",
		})
		if err == nil || err.Error() != "db connection failed" {
			t.Errorf("expected db connection failed, got %v", err)
		}
	})

	t.Run("wrong password returns invalid credentials", func(t *testing.T) {
		repo := newFakeRepo()
		email := "user@example.com"
		repo.identities[email] = &domain.Identity{
			ID:           uuid.New(),
			Email:        &email,
			PasswordHash: "hashed:password123",
			Role:         domain.RoleBuyer,
		}
		hasher := &fakePasswordHasher{match: false}
		uc := app.NewLoginUseCase(repo, hasher, &fakeTokenGen{})
		_, err := uc.Execute(ctx, app.LoginInput{
			Identifier: email,
			Password:   "wrongpassword",
		})
		if !errors.Is(err, app.ErrInvalidCredentials) {
			t.Errorf("expected ErrInvalidCredentials, got %v", err)
		}
	})

	t.Run("hasher error returns err", func(t *testing.T) {
		repo := newFakeRepo()
		email := "user@example.com"
		repo.identities[email] = &domain.Identity{
			ID:           uuid.New(),
			Email:        &email,
			PasswordHash: "hashed:password123",
		}
		hasher := &fakePasswordHasher{verErr: errors.New("hasher corrupted")}
		uc := app.NewLoginUseCase(repo, hasher, &fakeTokenGen{})
		_, err := uc.Execute(ctx, app.LoginInput{
			Identifier: email,
			Password:   "password123",
		})
		if err == nil || err.Error() != "hasher corrupted" {
			t.Errorf("expected hasher error, got %v", err)
		}
	})

	t.Run("token generation error", func(t *testing.T) {
		repo := newFakeRepo()
		email := "user@example.com"
		repo.identities[email] = &domain.Identity{
			ID:           uuid.New(),
			Email:        &email,
			PasswordHash: "hashed:password123",
		}
		hasher := &fakePasswordHasher{match: true}
		tokenGen := &fakeTokenGen{err: errors.New("jwt sign error")}
		uc := app.NewLoginUseCase(repo, hasher, tokenGen)
		_, err := uc.Execute(ctx, app.LoginInput{
			Identifier: email,
			Password:   "password123",
		})
		if err == nil || err.Error() != "jwt sign error" {
			t.Errorf("expected jwt sign error, got %v", err)
		}
	})

	t.Run("successful login", func(t *testing.T) {
		repo := newFakeRepo()
		email := "buyer@example.com"
		expectedIdentity := &domain.Identity{
			ID:        uuid.New(),
			Email:     &email,
			Role:      domain.RoleBuyer,
			CreatedAt: time.Now(),
		}
		repo.identities[email] = expectedIdentity
		hasher := &fakePasswordHasher{match: true}
		tokenGen := &fakeTokenGen{token: "mock-jwt-token"}

		uc := app.NewLoginUseCase(repo, hasher, tokenGen)
		output, err := uc.Execute(ctx, app.LoginInput{
			Identifier: email,
			Password:   "password123",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if output.Tokens.AccessToken != "mock-jwt-token" || output.Tokens.RefreshToken == "" {
			t.Errorf("expected access mock-jwt-token and a refresh token, got %+v", output.Tokens)
		}
		if output.Identity.ID != expectedIdentity.ID {
			t.Errorf("expected identity ID %v, got %v", expectedIdentity.ID, output.Identity.ID)
		}
	})
}
