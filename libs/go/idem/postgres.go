package idem

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// DB la phan cua *pgxpool.Pool ma PostgresStore can.
type DB interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// PostgresStore luu khoa trong bang idempotency_keys (migrations/0001_core.sql).
type PostgresStore struct{ db DB }

// NewPostgresStore tao store tren db.
func NewPostgresStore(db DB) *PostgresStore { return &PostgresStore{db: db} }

// Moi lenh la mot statement doc lap (autocommit) nen now() la gio luc chay lenh.
//
// ON CONFLICT ... WHERE: chi giao lai khoa da het han hoac dang xu ly qua lease. Hai request
// cung tiep quan mot khoa: request sau cho khoa dong, doc lai dong da doi created_at, khong con
// thoa WHERE nen khong nhan duoc khoa.
const beginSQL = `
INSERT INTO idempotency_keys (key, endpoint, identity_id, request_hash, expires_at)
VALUES ($1, $2, NULLIF($3::text, '')::uuid, $4, now() + $5::float8 * interval '1 second')
ON CONFLICT (key, endpoint) DO UPDATE SET
    identity_id  = EXCLUDED.identity_id,
    request_hash = EXCLUDED.request_hash,
    status_code  = NULL,
    response     = NULL,
    created_at   = now(),
    expires_at   = EXCLUDED.expires_at
WHERE idempotency_keys.expires_at <= now()
   OR (idempotency_keys.status_code IS NULL
       AND idempotency_keys.created_at <= now() - $6::float8 * interval '1 second')
RETURNING created_at::text`

const peekSQL = `
SELECT request_hash, status_code, response
FROM idempotency_keys
WHERE key = $1 AND endpoint = $2`

// created_at la quyen so huu: tiep quan doi created_at nen chu cu khong ghi de duoc.
const completeSQL = `
UPDATE idempotency_keys SET status_code = $3, response = $4
WHERE key = $1 AND endpoint = $2 AND created_at = $5::timestamptz AND status_code IS NULL`

const abandonSQL = `
DELETE FROM idempotency_keys
WHERE key = $1 AND endpoint = $2 AND created_at = $3::timestamptz`

// Giua INSERT (khong giao duoc) va SELECT, dong co the vua bi xoa (Abandon) hoac het han roi
// duoc giao lai; thu lai thay vi bao loi.
const maxBeginAttempts = 3

var uuidRE = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// stored la dang luu cua response. Body la []byte nen JSON luu base64: JSONB sap xep lai khoa
// va khoang trang, nhung phat lai phai dung TUNG BYTE.
type stored struct {
	ContentType string `json:"content_type"`
	Body        []byte `json:"body"`
}

func (s *PostgresStore) Begin(ctx context.Context, req Request, ttl, lease time.Duration) (Outcome, error) {
	// identity_id la UUID; gia tri khac dang (vd "guest-user") van nam trong hash, chi cot la NULL.
	identityID := ""
	if uuidRE.MatchString(req.Identity) {
		identityID = req.Identity
	}

	for range maxBeginAttempts {
		var token string
		err := s.db.QueryRow(ctx, beginSQL,
			req.Key, req.Endpoint, identityID, req.Hash, ttl.Seconds(), lease.Seconds()).Scan(&token)
		if err == nil {
			return Outcome{State: Acquired, Token: token}, nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return Outcome{}, fmt.Errorf("giu khoa: %w", err)
		}

		var (
			hash   string
			status *int
			raw    []byte
		)
		err = s.db.QueryRow(ctx, peekSQL, req.Key, req.Endpoint).Scan(&hash, &status, &raw)
		if errors.Is(err, pgx.ErrNoRows) {
			continue
		}
		if err != nil {
			return Outcome{}, fmt.Errorf("doc khoa: %w", err)
		}

		switch {
		case hash != req.Hash:
			return Outcome{State: Mismatch}, nil
		case status == nil:
			return Outcome{State: InProgress}, nil
		}
		var st stored
		if err := json.Unmarshal(raw, &st); err != nil {
			return Outcome{}, fmt.Errorf("giai ma response da luu: %w", err)
		}
		return Outcome{State: Replay, Response: Response{Status: *status, ContentType: st.ContentType, Body: st.Body}}, nil
	}
	return Outcome{}, errors.New("khong giu duoc khoa sau nhieu lan thu")
}

func (s *PostgresStore) Complete(ctx context.Context, req Request, token string, res Response) error {
	raw, err := json.Marshal(stored{ContentType: res.ContentType, Body: res.Body})
	if err != nil {
		return fmt.Errorf("ma hoa response: %w", err)
	}
	tag, err := s.db.Exec(ctx, completeSQL, req.Key, req.Endpoint, res.Status, raw, token)
	if err != nil {
		return fmt.Errorf("luu response: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrLeaseLost
	}
	return nil
}

func (s *PostgresStore) Abandon(ctx context.Context, req Request, token string) error {
	if _, err := s.db.Exec(ctx, abandonSQL, req.Key, req.Endpoint, token); err != nil {
		return fmt.Errorf("xoa khoa: %w", err)
	}
	return nil
}
