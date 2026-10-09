package domain

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

var (
	ErrIdentityNotFound = errors.New("identity not found")
	ErrInvalidPassword  = errors.New("invalid password")
	ErrIdentityExists   = errors.New("identity already exists")
)

type Role string

const (
	RoleBuyer     Role = "buyer"
	RoleOrganizer Role = "organizer"
	RoleModerator Role = "moderator"
	RoleOps       Role = "ops"
)

// Identity represents a user in the system.
type Identity struct {
	ID            uuid.UUID
	Email         *string // Pointer because it can be null if registered via phone
	PhoneHash     *string // Pointer because it can be null if registered via email
	PasswordHash  string
	Role          Role
	OtpVerifiedAt *time.Time
	CreatedAt     time.Time
}

// IdentityRepository defines the data access methods for Identity.
type IdentityRepository interface {
	Save(ctx context.Context, identity *Identity) error
	FindByEmailOrPhone(ctx context.Context, identifier string) (*Identity, error)
}

// PasswordHasher defines how passwords should be hashed and verified.
type PasswordHasher interface {
	Hash(password string) (string, error)
	Verify(password, encodedHash string) (bool, error)
}
