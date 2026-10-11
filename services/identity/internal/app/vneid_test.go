package app_test

import (
	"context"
	"testing"

	"github.com/eventflow/eventflow/services/identity/internal/app"
)

func TestVNeIDUseCase_HandleCallback_InvalidPayload(t *testing.T) {
	ctx := context.Background()
	uc := app.NewVNeIDUseCase(newFakeRepo(), nil, &fakeTokenGen{})

	err := uc.HandleCallback(ctx, app.VNeIDCallbackInput{
		SessionID:  "",
		CCCDNumber: "",
	})
	if err == nil {
		t.Error("expected error on empty session_id and cccd_number")
	}
}
