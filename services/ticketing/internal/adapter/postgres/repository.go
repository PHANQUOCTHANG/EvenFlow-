// Package postgresadapter hien thuc port.Repository tren PostgreSQL.
//
// Day la NGUON SU THAT cua ton kho. Moi bat bien chong oversell cuoi cung deu
// duoc thuc thi o day, trong transaction, duoi su bao ve cua constraint
// CHECK (available >= 0).
package postgresadapter

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/eventflow/eventflow/services/ticketing/internal/domain"
	"github.com/eventflow/eventflow/services/ticketing/internal/port"
)

type Repository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

func (r *Repository) GetSaleConfig(ctx context.Context, eventID, ticketTypeID string) (port.SaleConfig, error) {
	const q = `
		SELECT tt.price_cents, tt.bucket_count,
		       e.hold_ttl_seconds, e.max_per_order, e.max_per_identity
		  FROM ticket_types tt
		  JOIN events e ON e.id = tt.event_id
		 WHERE tt.id = $1 AND e.id = $2 AND e.status = 'ON_SALE'`

	var c port.SaleConfig
	err := r.pool.QueryRow(ctx, q, ticketTypeID, eventID).Scan(
		&c.PriceCents, &c.BucketCount, &c.HoldTTLSeconds, &c.MaxPerOrder, &c.MaxPerIdentity)
	if errors.Is(err, pgx.ErrNoRows) {
		return c, fmt.Errorf("su kien khong o trang thai dang ban: %w", domain.ErrSoldOut)
	}
	return c, err
}

// CountPurchased dem so ve da mua thanh cong cua mot nguoi (BR-O4).
// Chi tinh cac don da thanh toan hoac dang giu -- don da huy/het han khong tinh.
func (r *Repository) CountPurchased(ctx context.Context, eventID, identityID string) (int, error) {
	const q = `
		SELECT COALESCE(SUM(oi.quantity), 0)
		  FROM orders o
		  JOIN order_items oi ON oi.order_id = o.id
		 WHERE o.event_id = $1 AND o.identity_id = $2
		   AND o.status IN ('HELD','PAYMENT_PENDING','PAID','ISSUED')`

	var n int
	err := r.pool.QueryRow(ctx, q, eventID, identityID).Scan(&n)
	return n, err
}

func (r *Repository) GetActiveHold(ctx context.Context, eventID, identityID string) (port.HoldView, error) {
	const q = `
		SELECT o.id::text, h.id::text, h.bucket, h.expires_at
		  FROM orders o
		  JOIN seat_holds h ON h.order_id = o.id
		 WHERE o.event_id = $1 AND o.identity_id = $2
		   AND o.status IN ('HELD','PAYMENT_PENDING')
		   AND h.released_at IS NULL AND h.consumed_at IS NULL
		 LIMIT 1`

	var v port.HoldView
	err := r.pool.QueryRow(ctx, q, eventID, identityID).Scan(&v.OrderID, &v.HoldID, &v.Bucket, &v.ExpiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return v, domain.ErrOrderNotFound
	}
	return v, err
}

// PersistHold chot lenh giu ghe trong MOT transaction.
//
// Bon viec phai cung song cung chet:
//  1. Tru ton kho (co dieu kien available >= qty)
//  2. Tao don hang
//  3. Tao ban ghi hold
//  4. Ghi outbox de bao cho cac service khac
//
// Neu bat ky buoc nao hong, ca bon bi rollback. Khong co trang thai nua voi.
func (r *Repository) PersistHold(ctx context.Context, p port.PersistHoldParams) (port.HoldView, error) {
	var out port.HoldView

	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return out, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// (1) Tru ton kho. Mot cau UPDATE duy nhat tren mot dong -> khoa dong toi
	// thieu. Dieu kien available >= $3 lam cho thao tac nay an toan truoc moi
	// muc do dong thoi: Postgres serialize cac UPDATE tren cung mot dong.
	const decrement = `
		UPDATE ticket_inventory
		   SET available = available - $3
		 WHERE ticket_type_id = $1 AND bucket = $2 AND available >= $3`

	tag, err := tx.Exec(ctx, decrement, p.TicketTypeID, p.Bucket, p.Quantity)
	if err != nil {
		return out, wrapConstraint(err)
	}
	if tag.RowsAffected() == 0 {
		// Khong co dong nao bi anh huong nghia la ton kho khong du. Day la
		// truong hop Redis da bao "con ve" trong khi su that la het.
		return out, domain.ErrInventoryExhausted
	}

	// (2) Tao don. Unique index uq_active_order_per_identity_event thuc thi BR-O3.
	const insertOrder = `
		INSERT INTO orders (event_id, identity_id, status, total_cents, expires_at)
		VALUES ($1, $2, 'HELD', $3, $4)
		RETURNING id::text`

	total := p.UnitCents * int64(p.Quantity)
	if err := tx.QueryRow(ctx, insertOrder,
		p.EventID, p.IdentityID, total, p.ExpiresAt).Scan(&out.OrderID); err != nil {
		return out, wrapConstraint(err)
	}

	const insertItem = `
		INSERT INTO order_items (order_id, ticket_type_id, bucket, quantity, unit_cents)
		VALUES ($1, $2, $3, $4, $5)`
	if _, err := tx.Exec(ctx, insertItem,
		out.OrderID, p.TicketTypeID, p.Bucket, p.Quantity, p.UnitCents); err != nil {
		return out, err
	}

	// (3) Ban ghi hold. Dung chinh HoldID cua Redis de hai tang doi chieu duoc.
	const insertHold = `
		INSERT INTO seat_holds (id, order_id, ticket_type_id, bucket, quantity, expires_at)
		VALUES ($1::uuid, $2, $3, $4, $5, $6)
		RETURNING id::text`
	if err := tx.QueryRow(ctx, insertHold, toUUID(p.HoldID),
		out.OrderID, p.TicketTypeID, p.Bucket, p.Quantity, p.ExpiresAt).Scan(&out.HoldID); err != nil {
		return out, err
	}

	// (4) Outbox: ghi CUNG transaction voi nghiep vu. Day la ly do khong bao gio
	// xay ra chuyen "don da tao nhung khong ai duoc bao" hay nguoc lai.
	payload, _ := json.Marshal(map[string]any{
		"order_id":   out.OrderID,
		"event_id":   p.EventID,
		"expires_at": p.ExpiresAt,
		"quantity":   p.Quantity,
	})
	const insertOutbox = `
		INSERT INTO outbox_events (aggregate_type, aggregate_id, routing_key, payload)
		VALUES ('order', $1::uuid, 'ticketing.hold.created', $2)`
	if _, err := tx.Exec(ctx, insertOutbox, out.OrderID, payload); err != nil {
		return out, err
	}

	if err := tx.Commit(ctx); err != nil {
		return out, err
	}

	out.Bucket = p.Bucket
	out.ExpiresAt = p.ExpiresAt
	return out, nil
}

// ReleaseHold tra ve vao kho. IDEMPOTENT (BR-O7).
//
// Toan bo an toan nam o dieu kien released_at IS NULL: lan goi thu hai khong
// cap nhat dong nao, nen kho khong bi cong hai lan. Neu mat dieu kien nay, mot
// hold bi quet hai lan se lam kho phong len va he thong se oversell.
func (r *Repository) ReleaseHold(ctx context.Context, holdID string) (bool, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	const claim = `
		UPDATE seat_holds
		   SET released_at = now()
		 WHERE id = $1::uuid AND released_at IS NULL AND consumed_at IS NULL
		RETURNING order_id::text, ticket_type_id::text, bucket, quantity`

	var orderID, ticketTypeID string
	var bucket, qty int
	err = tx.QueryRow(ctx, claim, toUUID(holdID)).Scan(&orderID, &ticketTypeID, &bucket, &qty)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil // da xu ly truoc do -- khong phai loi
	}
	if err != nil {
		return false, err
	}

	const restore = `
		UPDATE ticket_inventory
		   SET available = available + $3
		 WHERE ticket_type_id = $1 AND bucket = $2`
	if _, err := tx.Exec(ctx, restore, ticketTypeID, bucket, qty); err != nil {
		return false, wrapConstraint(err)
	}

	const expire = `
		UPDATE orders SET status = 'EXPIRED', updated_at = now()
		 WHERE id = $1::uuid AND status = 'HELD'`
	if _, err := tx.Exec(ctx, expire, orderID); err != nil {
		return false, err
	}

	return true, tx.Commit(ctx)
}

// wrapConstraint dich loi constraint cua Postgres sang loi nghiep vu.
//
// Vi pham inv_available_le_capacity nghia la co cho nao do da tra kho hai lan --
// mot bug nghiem trong, phai nhin thay ngay chu khong nuot vao loi chung chung.
func wrapConstraint(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.ConstraintName {
		case "ticket_inventory_available_check":
			return domain.ErrInventoryExhausted
		case "inv_available_le_capacity":
			return fmt.Errorf("PHAT HIEN TRA KHO HAI LAN tren ton kho: %w", err)
		case "uq_active_order_per_identity_event":
			return domain.ErrPerIdentityLimit
		}
	}
	return err
}

func toUUID(s string) string {
	if len(s) == 32 {
		return fmt.Sprintf("%s-%s-%s-%s-%s", s[0:8], s[8:12], s[12:16], s[16:20], s[20:32])
	}
	return s
}
