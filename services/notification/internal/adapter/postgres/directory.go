// Package postgres doc du lieu tham chieu (identity, su kien) tu database dung chung.
package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/eventflow/eventflow/services/notification/internal/port"
)

// Directory la port.RecipientDirectory tren Postgres.
type Directory struct{ pool *pgxpool.Pool }

// NewDirectory tao Directory.
func NewDirectory(pool *pgxpool.Pool) *Directory { return &Directory{pool: pool} }

// Lookup doc email cua identity va ten su kien. Su kien khong ton tai thi EventTitle rong.
// identityID/eventID khong phai UUID hop le thi coi nhu khong tim thay (khong retry).
func (d *Directory) Lookup(ctx context.Context, identityID, eventID string) (port.Recipient, error) {
	id, err := uuid.Parse(identityID)
	if err != nil {
		return port.Recipient{}, nil
	}
	// event_id sai dinh dang chi lam mat ten su kien, van gui duoc thong bao.
	var ev *uuid.UUID
	if e, perr := uuid.Parse(eventID); perr == nil {
		ev = &e
	}
	const q = `
		SELECT COALESCE(i.email, ''),
		       COALESCE((SELECT e.title FROM events e WHERE e.id = $2::uuid), '')
		  FROM identities i
		 WHERE i.id = $1::uuid`
	var r port.Recipient
	err = d.pool.QueryRow(ctx, q, id, ev).Scan(&r.Email, &r.EventTitle)
	if errors.Is(err, pgx.ErrNoRows) {
		return port.Recipient{}, nil
	}
	if err != nil {
		return port.Recipient{}, fmt.Errorf("tra cuu nguoi nhan %s: %w", identityID, err)
	}
	return r, nil
}
