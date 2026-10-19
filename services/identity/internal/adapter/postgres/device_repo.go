package postgres

import (
	"context"
	"database/sql"
	"errors"
	"net"

	"github.com/eventflow/eventflow/libs/go/pii"
	"github.com/eventflow/eventflow/services/identity/internal/domain"
	"github.com/google/uuid"
)

type deviceRepo struct {
	db  *sql.DB
	pii *pii.Protector
}

// NewDeviceRepo luu thiet bi vao bang devices. fingerprint chi duoc luu dang HMAC: lo bang nay khong
// lo fingerprint that, nhung idx_devices_fingerprint van cho antibot dem so identity tren mot thiet bi (R4).
func NewDeviceRepo(db *sql.DB, protector *pii.Protector) domain.DeviceRepository {
	return &deviceRepo{db: db, pii: protector}
}

func (r *deviceRepo) Register(ctx context.Context, identityID uuid.UUID, fingerprint, ip string, max int) (uuid.UUID, error) {
	fp := r.pii.Hash(fingerprint)
	var ipArg any
	if parsed := net.ParseIP(ip); parsed != nil {
		ipArg = parsed.String()
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return uuid.Nil, err
	}
	defer func() { _ = tx.Rollback() }()

	// Khoa theo identity de hai dang nhap dong thoi tren hai thiet bi moi khong cung vuot gioi han.
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, identityID.String()); err != nil {
		return uuid.Nil, err
	}

	var id uuid.UUID
	err = tx.QueryRowContext(ctx, `
		UPDATE devices SET last_seen = now(), last_ip = COALESCE($3::inet, last_ip)
		WHERE identity_id = $1 AND fingerprint = $2
		RETURNING id
	`, identityID, fp, ipArg).Scan(&id)
	switch {
	case err == nil:
		return id, tx.Commit()
	case !errors.Is(err, sql.ErrNoRows):
		return uuid.Nil, err
	}

	var count int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM devices WHERE identity_id = $1`, identityID).Scan(&count); err != nil {
		return uuid.Nil, err
	}
	if count >= max {
		return uuid.Nil, domain.ErrTooManyDevices
	}
	if err := tx.QueryRowContext(ctx, `
		INSERT INTO devices (identity_id, fingerprint, last_ip) VALUES ($1, $2, $3::inet) RETURNING id
	`, identityID, fp, ipArg).Scan(&id); err != nil {
		return uuid.Nil, err
	}
	return id, tx.Commit()
}
