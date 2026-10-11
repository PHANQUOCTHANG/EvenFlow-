package app_test

import (
	"context"
	"errors"
	"testing"

	"github.com/eventflow/eventflow/services/identity/internal/app"
)

func TestGoogleLoginUseCase_Execute_InvalidToken(t *testing.T) {
	ctx := context.Background()
	uc := app.NewGoogleLoginUseCase(newFakeRepo(), &fakePasswordHasher{}, &fakeTokenGen{}, "test-client-id")

	_, err := uc.Execute(ctx, app.GoogleLoginInput{
		IDToken: "invalid-token",
	})
	if !errors.Is(err, app.ErrInvalidGoogleToken) {
		t.Errorf("expected ErrInvalidGoogleToken, got %v", err)
	}
}
