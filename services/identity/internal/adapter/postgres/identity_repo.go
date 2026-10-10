package postgres

import (
	"context"
	"database/sql"
	"errors"

	"github.com/eventflow/eventflow/services/identity/internal/domain"
	"github.com/lib/pq"
)

type identityRepo struct {
	db *sql.DB
}

func NewIdentityRepo(db *sql.DB) domain.IdentityRepository {
	return &identityRepo{db: db}
}

func (r *identityRepo) Save(ctx context.Context, identity *domain.Identity) error {
	query := `
		INSERT INTO identities (id, email, phone_hash, password_hash, role, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)
	`
	_, err := r.db.ExecContext(ctx, query,
		identity.ID,
		identity.Email,
		identity.PhoneHash,
		identity.PasswordHash,
		string(identity.Role),
		identity.CreatedAt,
	)

	if err != nil {
		// Check for PostgreSQL unique constraint violation (code 23505)
		var pqErr *pq.Error
		if errors.As(err, &pqErr) && pqErr.Code == "23505" {
			return domain.ErrIdentityExists
		}
		return err
	}

	return nil
}

func (r *identityRepo) FindByEmailOrPhone(ctx context.Context, identifier string) (*domain.Identity, error) {
	query := `
		SELECT id, email, phone_hash, password_hash, role, otp_verified_at, created_at
		FROM identities
		WHERE email = $1 OR phone_hash = $1
		LIMIT 1
	`
	
	row := r.db.QueryRowContext(ctx, query, identifier)

	var i domain.Identity
	var role string

	err := row.Scan(
		&i.ID,
		&i.Email,
		&i.PhoneHash,
		&i.PasswordHash,
		&role,
		&i.OtpVerifiedAt,
		&i.CreatedAt,
	)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.ErrIdentityNotFound
		}
		return nil, err
	}

	i.Role = domain.Role(role)
	return &i, nil
}
