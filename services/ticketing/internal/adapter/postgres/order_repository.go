package postgresadapter

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/eventflow/eventflow/libs/go/otelx"
	"github.com/eventflow/eventflow/services/ticketing/internal/domain"
	"github.com/eventflow/eventflow/services/ticketing/internal/port"
)

// GetOrder doc don cua mot khach. Id sai dinh dang, don khong ton tai va don cua nguoi khac deu
// tra ErrOrderNotFound.
func (r *Repository) GetOrder(ctx context.Context, orderID, identityID string) (port.OrderView, error) {
	var v port.OrderView
	if !uuidRE.MatchString(orderID) || !uuidRE.MatchString(identityID) {
		return v, domain.ErrOrderNotFound
	}

	const head = `
		SELECT o.id::text, o.event_id::text, e.title, COALESCE(e.venue, ''), o.status,
		       o.total_cents, o.currency, o.expires_at, o.created_at,
		       COALESCE((SELECT h.id::text FROM seat_holds h
		                  WHERE h.order_id = o.id AND h.released_at IS NULL AND h.consumed_at IS NULL
		                  LIMIT 1), ''),
		       (SELECT p.created_at FROM payments p
		         WHERE p.order_id = o.id AND p.status = 'SUCCEEDED'
		         ORDER BY p.created_at DESC LIMIT 1),
		       COALESCE((SELECT p.provider FROM payments p
		                  WHERE p.order_id = o.id ORDER BY p.created_at DESC LIMIT 1), ''),
		       COALESCE((SELECT p.status = 'FAILED' FROM payments p
		                  WHERE p.order_id = o.id ORDER BY p.created_at DESC LIMIT 1), false)
		  FROM orders o
		  JOIN events e ON e.id = o.event_id
		 WHERE o.id = $1::uuid AND o.identity_id = $2::uuid`
	err := r.pool.QueryRow(ctx, head, orderID, identityID).Scan(
		&v.OrderID, &v.EventID, &v.EventTitle, &v.Venue, &v.Status,
		&v.TotalCents, &v.Currency, &v.ExpiresAt, &v.CreatedAt,
		&v.HoldID, &v.PaidAt, &v.PaymentProvider, &v.PaymentFailed)
	if errors.Is(err, pgx.ErrNoRows) {
		return v, domain.ErrOrderNotFound
	}
	if err != nil {
		return v, err
	}

	const items = `
		SELECT oi.ticket_type_id::text, tt.name, oi.quantity, oi.unit_cents
		  FROM order_items oi
		  JOIN ticket_types tt ON tt.id = oi.ticket_type_id
		 WHERE oi.order_id = $1::uuid
		 ORDER BY oi.id`
	rows, err := r.pool.Query(ctx, items, orderID)
	if err != nil {
		return v, err
	}
	defer rows.Close()
	for rows.Next() {
		var it port.OrderItemView
		if err := rows.Scan(&it.TicketTypeID, &it.TierName, &it.Quantity, &it.UnitCents); err != nil {
			return v, err
		}
		v.Items = append(v.Items, it)
	}
	return v, rows.Err()
}

// OrderOwnedBy kiem don co thuoc khach khong.
func (r *Repository) OrderOwnedBy(ctx context.Context, orderID, identityID string) (bool, error) {
	if !uuidRE.MatchString(orderID) || !uuidRE.MatchString(identityID) {
		return false, nil
	}
	var ok bool
	err := r.pool.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM orders WHERE id = $1::uuid AND identity_id = $2::uuid)`,
		orderID, identityID).Scan(&ok)
	return ok, err
}

// CancelHold huy hold cua khach: tra kho + dat don CANCELLED + ghi outbox, trong MOT transaction.
//
// Khoa dong hold va don (FOR UPDATE) nen huy dong thoi voi sweeper (het han) hay voi thanh toan
// khong tra kho hai lan: cung nhu ReleaseHold, an toan nam o dieu kien released_at IS NULL.
func (r *Repository) CancelHold(ctx context.Context, holdID, identityID string) (port.CancelResult, error) {
	ctx, end := otelx.StartSpan(ctx, "postgres.cancel_hold")
	res, err := r.cancelHold(ctx, holdID, identityID)
	end(err)
	return res, err
}

func (r *Repository) cancelHold(ctx context.Context, holdID, identityID string) (port.CancelResult, error) {
	var out port.CancelResult
	holdUUID := toUUID(holdID)
	if !uuidRE.MatchString(holdUUID) || !uuidRE.MatchString(identityID) {
		return out, domain.ErrOrderNotFound
	}

	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return out, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	const lock = `
		SELECT o.id::text, o.event_id::text, o.status, h.ticket_type_id::text, h.bucket, h.quantity,
		       h.released_at IS NOT NULL, h.consumed_at IS NOT NULL
		  FROM seat_holds h
		  JOIN orders o ON o.id = h.order_id
		 WHERE h.id = $1::uuid AND o.identity_id = $2::uuid
		   FOR UPDATE OF h, o`
	var (
		orderID, status, ticketTypeID string
		bucket, qty                   int
		released, consumed            bool
	)
	err = tx.QueryRow(ctx, lock, holdUUID, identityID).Scan(
		&orderID, &out.EventID, &status, &ticketTypeID, &bucket, &qty, &released, &consumed)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, domain.ErrOrderNotFound
	}
	if err != nil {
		return out, err
	}

	switch {
	case released:
		// Da huy hoac het han: muc tieu (hold khong con) da dat, khong tra kho lan nua.
		return out, nil
	case consumed || status != "HELD":
		return out, fmt.Errorf("don %s o trang thai %s: %w", orderID, status, domain.ErrHoldNotCancellable)
	}

	if _, err := tx.Exec(ctx, `UPDATE seat_holds SET released_at = now() WHERE id = $1::uuid`, holdUUID); err != nil {
		return out, err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE ticket_inventory SET available = available + $3
		 WHERE ticket_type_id = $1::uuid AND bucket = $2`, ticketTypeID, bucket, qty); err != nil {
		return out, wrapConstraint(err)
	}
	if _, err := tx.Exec(ctx, `UPDATE orders SET status = 'CANCELLED', updated_at = now() WHERE id = $1::uuid`, orderID); err != nil {
		return out, err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO outbox_events (aggregate_type, aggregate_id, routing_key, payload)
		VALUES ('order', $1::uuid, 'ticketing.hold.cancelled',
		        jsonb_build_object('order_id', $1::text, 'event_id', $2::text, 'hold_id', $3::text, 'quantity', $4::int))`,
		orderID, out.EventID, holdUUID, qty); err != nil {
		return out, err
	}
	if err := tx.Commit(ctx); err != nil {
		return out, err
	}
	out.Cancelled = true
	return out, nil
}

// ListTiers liet ke hang ve cua su kien dang ban (hoac sap ban) cung suc chua.
func (r *Repository) ListTiers(ctx context.Context, eventID string) ([]port.TierInfo, error) {
	if !uuidRE.MatchString(eventID) {
		return nil, domain.ErrEventNotFound
	}
	const q = `
		SELECT tt.id::text, tt.name, tt.quota
		  FROM ticket_types tt
		  JOIN events e ON e.id = tt.event_id
		 WHERE e.id = $1::uuid AND e.status IN ('SCHEDULED','ON_SALE','SOLD_OUT')
		 ORDER BY tt.price_cents, tt.name`
	rows, err := r.pool.Query(ctx, q, eventID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []port.TierInfo
	for rows.Next() {
		var t port.TierInfo
		if err := rows.Scan(&t.TicketTypeID, &t.Name, &t.Capacity); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return nil, domain.ErrEventNotFound
	}
	return out, nil
}
