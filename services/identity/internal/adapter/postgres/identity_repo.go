package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/eventflow/eventflow/libs/go/pii"
	"github.com/eventflow/eventflow/services/identity/internal/domain"
	"github.com/google/uuid"
	"github.com/lib/pq"
)

type identityRepo struct {
	db  *sql.DB
	pii *pii.Protector
}

// NewIdentityRepo tao repository identities. protector bam va ma hoa so dien thoai (EV-115):
// phone_hash = HMAC (tra cuu, UNIQUE), phone_enc = AES-GCM (doc lai khi can gui SMS).
func NewIdentityRepo(db *sql.DB, protector *pii.Protector) *IdentityRepo {
	return &IdentityRepo{identityRepo{db: db, pii: protector}}
}

// IdentityRepo la domain.IdentityRepository kem thao tac bao tri du lieu PII.
type IdentityRepo struct{ identityRepo }

var _ domain.IdentityRepository = (*IdentityRepo)(nil)

// phoneAAD gan ban ma so dien thoai voi dung dong identity.
func phoneAAD(id uuid.UUID) string { return "identities.phone_enc:" + id.String() }

func (r *identityRepo) Save(ctx context.Context, identity *domain.Identity) error {
	var phoneHash *string
	var phoneEnc []byte
	if identity.Phone != nil {
		h := r.pii.Hash(*identity.Phone)
		enc, err := r.pii.Encrypt(*identity.Phone, phoneAAD(identity.ID))
		if err != nil {
			return fmt.Errorf("ma hoa so dien thoai: %w", err)
		}
		phoneHash, phoneEnc = &h, enc
		identity.PhoneHash = phoneHash
	}

	query := `
		INSERT INTO identities (id, email, phone_hash, phone_enc, password_hash, role, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`
	_, err := r.db.ExecContext(ctx, query,
		identity.ID,
		identity.Email,
		phoneHash,
		phoneEnc,
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

// FindByEmailOrPhone tra theo email, hoac theo HMAC cua so dien thoai da chuan hoa: "0912..." va
// "84912..." la cung mot tai khoan.
func (r *identityRepo) FindByEmailOrPhone(ctx context.Context, identifier string) (*domain.Identity, error) {
	column, value := "email", identifier
	if phone, ok := pii.NormalizePhoneVN(identifier); ok && !strings.Contains(identifier, "@") {
		column, value = "phone_hash", r.pii.Hash(phone)
	}
	row := r.db.QueryRowContext(ctx, `
		SELECT id, email, phone_hash, password_hash, role, otp_verified_at, created_at
		FROM identities
		WHERE `+column+` = $1
		LIMIT 1
	`, value)
	return scanIdentity(row)
}

// FindByID tra identity theo khoa chinh (dung khi refresh: role co the da doi tu luc dang nhap).
func (r *identityRepo) FindByID(ctx context.Context, id uuid.UUID) (*domain.Identity, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, email, phone_hash, password_hash, role, otp_verified_at, created_at
		FROM identities
		WHERE id = $1
	`, id)
	return scanIdentity(row)
}

func scanIdentity(row *sql.Row) (*domain.Identity, error) {
	var i domain.Identity
	var role string
	if err := row.Scan(&i.ID, &i.Email, &i.PhoneHash, &i.PasswordHash, &role, &i.OtpVerifiedAt, &i.CreatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.ErrIdentityNotFound
		}
		return nil, err
	}
	i.Role = domain.Role(role)
	return &i, nil
}

func (r *identityRepo) UpdatePassword(ctx context.Context, id uuid.UUID, newHash string) error {
	query := `UPDATE identities SET password_hash = $1 WHERE id = $2`
	_, err := r.db.ExecContext(ctx, query, newHash, id)
	return err
}

// ProtectLegacyPhones chuyen cac dong cu (truoc EV-115) dang luu SO DIEN THOAI THO trong cot
// phone_hash sang HMAC + phone_enc. HMAC luon dai 64 ky tu hex nen dong cu nhan ra duoc bang
// do dai. Idempotent: chay lai khong doi gi. Tra so dong da chuyen va so dong bo qua (hai dong cu
// cung mot so o hai dang "09..." / "849..." -> dong sau dung UNIQUE, de nguyen cho nguoi xu ly).
func (r *IdentityRepo) ProtectLegacyPhones(ctx context.Context) (done, skipped int, err error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, phone_hash FROM identities
		WHERE phone_hash IS NOT NULL AND length(phone_hash) <> 64
	`)
	if err != nil {
		return 0, 0, err
	}
	type legacy struct {
		id    uuid.UUID
		phone string
	}
	var todo []legacy
	for rows.Next() {
		var l legacy
		if err := rows.Scan(&l.id, &l.phone); err != nil {
			_ = rows.Close()
			return 0, 0, err
		}
		todo = append(todo, l)
	}
	if err := rows.Close(); err != nil {
		return 0, 0, err
	}
	if err := rows.Err(); err != nil {
		return 0, 0, err
	}

	for _, l := range todo {
		phone, ok := pii.NormalizePhoneVN(l.phone)
		if !ok {
			phone = l.phone // giu nguyen gia tri de khong mat du lieu; van bam va ma hoa
		}
		enc, encErr := r.pii.Encrypt(phone, phoneAAD(l.id))
		if encErr != nil {
			return done, skipped, encErr
		}
		_, execErr := r.db.ExecContext(ctx, `
			UPDATE identities SET phone_hash = $2, phone_enc = $3
			WHERE id = $1 AND phone_hash = $4
		`, l.id, r.pii.Hash(phone), enc, l.phone)
		var pqErr *pq.Error
		switch {
		case errors.As(execErr, &pqErr) && pqErr.Code == "23505":
			skipped++
		case execErr != nil:
			return done, skipped, fmt.Errorf("bao ve so dien thoai identity %s: %w", l.id, execErr)
		default:
			done++
		}
	}
	return done, skipped, nil
}
