package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/eventflow/eventflow/services/identity/internal/domain"
	"github.com/google/uuid"
)

type refreshTokenRepo struct {
	db *sql.DB
}

// NewRefreshTokenRepo luu refresh token (EV-112) vao bang refresh_tokens.
func NewRefreshTokenRepo(db *sql.DB) domain.RefreshTokenRepository {
	return &refreshTokenRepo{db: db}
}

func (r *refreshTokenRepo) Create(ctx context.Context, t *domain.RefreshToken) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO refresh_tokens (id, identity_id, family_id, token_hash, expires_at, created_at, device_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, t.ID, t.IdentityID, t.FamilyID, t.TokenHash, t.ExpiresAt, t.CreatedAt, nullUUID(t.DeviceID))
	return err
}

func nullUUID(id *uuid.UUID) uuid.NullUUID {
	if id == nil {
		return uuid.NullUUID{}
	}
	return uuid.NullUUID{UUID: *id, Valid: true}
}

func (r *refreshTokenRepo) SetDevice(ctx context.Context, tokenID, deviceID uuid.UUID) error {
	_, err := r.db.ExecContext(ctx, `UPDATE refresh_tokens SET device_id = $2 WHERE id = $1 AND revoked_at IS NULL`, tokenID, deviceID)
	return err
}

// Rotate chay trong mot transaction va khoa dong cu bang FOR UPDATE: hai request refresh cung
// mot token se xep hang tai day, request thu hai thay revoked_at da co -> nhanh tai su dung.
func (r *refreshTokenRepo) Rotate(ctx context.Context, oldHash string, next *domain.RefreshToken, now time.Time) (*domain.RefreshToken, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	var old domain.RefreshToken
	var dev uuid.NullUUID
	err = tx.QueryRowContext(ctx, `
		SELECT id, identity_id, family_id, token_hash, expires_at, created_at, revoked_at, device_id
		FROM refresh_tokens
		WHERE token_hash = $1
		FOR UPDATE
	`, oldHash).Scan(&old.ID, &old.IdentityID, &old.FamilyID, &old.TokenHash, &old.ExpiresAt, &old.CreatedAt, &old.RevokedAt, &dev)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrRefreshTokenInvalid
	}
	if err != nil {
		return nil, err
	}

	if dev.Valid {
		old.DeviceID = &dev.UUID
	}

	if old.RevokedAt != nil {
		// Token da xoay (hoac da logout) ma van duoc dua len: thu hoi ca ho va COMMIT, du
		// request nay that bai. Neu la logout thi ho da bi thu hoi san, cau lenh khong doi gi.
		if err := revokeFamily(ctx, tx, old.FamilyID, domain.RevokeReasonReuse, now); err != nil {
			return nil, err
		}
		if err := tx.Commit(); err != nil {
			return nil, err
		}
		return &old, domain.ErrRefreshTokenReused
	}
	if !now.Before(old.ExpiresAt) {
		return nil, domain.ErrRefreshTokenInvalid
	}

	// Token moi thuoc cung identity, cung ho va giu han cua ho: refresh khong keo dai phien.
	next.IdentityID = old.IdentityID
	next.FamilyID = old.FamilyID
	next.ExpiresAt = old.ExpiresAt
	next.DeviceID = old.DeviceID
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO refresh_tokens (id, identity_id, family_id, token_hash, expires_at, created_at, device_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, next.ID, next.IdentityID, next.FamilyID, next.TokenHash, next.ExpiresAt, next.CreatedAt, nullUUID(next.DeviceID)); err != nil {
		return nil, fmt.Errorf("chen refresh token moi: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE refresh_tokens
		SET revoked_at = $2, revoked_reason = $3, replaced_by = $4
		WHERE id = $1
	`, old.ID, now, domain.RevokeReasonRotated, next.ID); err != nil {
		return nil, fmt.Errorf("thu hoi refresh token cu: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &old, nil
}

func (r *refreshTokenRepo) RevokeFamilyByHash(ctx context.Context, hash, reason string, now time.Time) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE refresh_tokens
		SET revoked_at = $2, revoked_reason = $3
		WHERE revoked_at IS NULL
		  AND family_id = (SELECT family_id FROM refresh_tokens WHERE token_hash = $1)
	`, hash, now, reason)
	return err
}

func (r *refreshTokenRepo) RevokeAllForIdentity(ctx context.Context, identityID uuid.UUID, reason string, now time.Time) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE refresh_tokens
		SET revoked_at = $2, revoked_reason = $3
		WHERE identity_id = $1 AND revoked_at IS NULL
	`, identityID, now, reason)
	return err
}

func revokeFamily(ctx context.Context, tx *sql.Tx, familyID uuid.UUID, reason string, now time.Time) error {
	_, err := tx.ExecContext(ctx, `
		UPDATE refresh_tokens
		SET revoked_at = $2, revoked_reason = $3
		WHERE family_id = $1 AND revoked_at IS NULL
	`, familyID, now, reason)
	return err
}
