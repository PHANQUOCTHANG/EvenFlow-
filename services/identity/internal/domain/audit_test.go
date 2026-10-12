package domain_test

import (
	"testing"
	"time"

	"github.com/eventflow/eventflow/services/identity/internal/domain"
	"github.com/google/uuid"
)

func TestAuditLog_Model(t *testing.T) {
	log := domain.AuditLog{
		ID:        1,
		ActorType: domain.ActorTypeHuman,
		ActorID:   uuid.New().String(),
		Action:    domain.ActionAccessDenied,
		Subject:   "event:123",
		Reason:    "insufficient permission",
		Metadata:  map[string]any{"ip": "127.0.0.1"},
		CreatedAt: time.Now(),
	}

	if log.ActorType != domain.ActorTypeHuman {
		t.Errorf("expected ActorTypeHuman, got %s", log.ActorType)
	}
	if log.Action != domain.ActionAccessDenied {
		t.Errorf("expected ActionAccessDenied, got %s", log.Action)
	}
}

func TestIdentity_ErrorsAndRoles(t *testing.T) {
	if domain.RoleBuyer != "buyer" || domain.RoleOrganizer != "organizer" || domain.RoleModerator != "moderator" || domain.RoleOps != "ops" {
		t.Error("role constants mismatch")
	}

	if domain.ErrIdentityNotFound == nil || domain.ErrInvalidPassword == nil || domain.ErrIdentityExists == nil {
		t.Error("expected non-nil domain errors")
	}
}
