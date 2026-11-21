package postgresadapter

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/eventflow/eventflow/services/ticketing/internal/domain"
	"github.com/eventflow/eventflow/services/ticketing/internal/port"
)

// opsQueryTimeout gioi han moi truy van war room / doanh so (AC-17): dang mo ban thi Postgres ban, man
// hinh Ops tra 503 con hon giu ket noi.
const opsQueryTimeout = 2 * time.Second

var _ port.OpsReader = (*Repository)(nil)

func (r *Repository) EventExists(ctx context.Context, eventID string) (bool, error) {
	if !uuidRE.MatchString(eventID) {
		return false, nil
	}
	ctx, cancel := context.WithTimeout(ctx, opsQueryTimeout)
	defer cancel()
	var ok bool
	err := r.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM events WHERE id = $1::uuid)`, eventID).Scan(&ok)
	return ok, err
}

func (r *Repository) EventOwner(ctx context.Context, eventID string) (string, error) {
	if !uuidRE.MatchString(eventID) {
		return "", domain.ErrEventNotFound
	}
	ctx, cancel := context.WithTimeout(ctx, opsQueryTimeout)
	defer cancel()
	var owner string
	err := r.pool.QueryRow(ctx, `SELECT organizer_id::text FROM events WHERE id = $1::uuid`, eventID).Scan(&owner)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", domain.ErrEventNotFound
	}
	if err != nil {
		return "", err
	}
	return owner, nil
}

// Cac CTE dung chung. Moi so lieu GROUP BY ticket_type_id TRUOC khi join, nen nhieu hold / nhieu dong
// order_items tren cung mot hang ve khong nhan ban nhau (fan-out).
//
// held (CR-1): hold mo (chua nha, chua tieu thu) cua don con HELD / PAYMENT_PENDING. Hold chi duoc tieu thu
// luc phat hanh ve, khong phai luc thanh toan, nen thieu loc trang thai don thi don PAID chua phat hanh bi
// dem ca o held lan paid.
const opsTierCTEs = `
	tt AS (
		SELECT id, name, price_cents, quota FROM ticket_types WHERE event_id = $1::uuid
	),
	held AS (
		SELECT h.ticket_type_id, SUM(h.quantity)::bigint AS n
		  FROM seat_holds h
		  JOIN tt ON tt.id = h.ticket_type_id
		  JOIN orders o ON o.id = h.order_id
		 WHERE h.released_at IS NULL AND h.consumed_at IS NULL
		   AND o.status IN ('HELD','PAYMENT_PENDING')
		 GROUP BY h.ticket_type_id
	)`

// TierInventory: MOT cau lenh (mot snapshot ke ca o READ COMMITTED), xuat phat tu events LEFT JOIN
// ticket_types de phan biet "su kien khong co hang ve" (1 dong, tier NULL) voi "khong co su kien" (0 dong).
func (r *Repository) TierInventory(ctx context.Context, eventID string) ([]port.TierInventoryRow, error) {
	if !uuidRE.MatchString(eventID) {
		return nil, domain.ErrEventNotFound
	}
	ctx, cancel := context.WithTimeout(ctx, opsQueryTimeout)
	defer cancel()
	q := `WITH ` + opsTierCTEs + `,
	inv AS (
		SELECT i.ticket_type_id, SUM(i.available)::bigint AS n
		  FROM ticket_inventory i JOIN tt ON tt.id = i.ticket_type_id
		 GROUP BY i.ticket_type_id
	),
	paid AS (
		SELECT oi.ticket_type_id, SUM(oi.quantity)::bigint AS n
		  FROM order_items oi JOIN orders o ON o.id = oi.order_id
		 WHERE o.event_id = $1::uuid AND o.status = 'PAID'
		 GROUP BY oi.ticket_type_id
	),
	issued AS (
		SELECT tk.ticket_type_id, COUNT(*)::bigint AS n
		  FROM tickets tk JOIN tt ON tt.id = tk.ticket_type_id
		 WHERE tk.status IN ('ISSUED','CHECKED_IN')
		 GROUP BY tk.ticket_type_id
	)
	SELECT tt.id::text, tt.name, tt.quota,
	       COALESCE(inv.n, 0), COALESCE(held.n, 0), COALESCE(paid.n, 0), COALESCE(issued.n, 0)
	  FROM events e
	  LEFT JOIN tt ON true
	  LEFT JOIN inv ON inv.ticket_type_id = tt.id
	  LEFT JOIN held ON held.ticket_type_id = tt.id
	  LEFT JOIN paid ON paid.ticket_type_id = tt.id
	  LEFT JOIN issued ON issued.ticket_type_id = tt.id
	 WHERE e.id = $1::uuid
	 ORDER BY tt.price_cents, tt.name`
	rows, err := r.pool.Query(ctx, q, eventID)
	if err != nil {
		return nil, fmt.Errorf("doc ton kho ops: %w", err)
	}
	defer rows.Close()
	found := false
	out := []port.TierInventoryRow{}
	for rows.Next() {
		found = true
		var (
			id, name                  *string
			quota                     *int
			avail, held, paid, issued int64
		)
		if err := rows.Scan(&id, &name, &quota, &avail, &held, &paid, &issued); err != nil {
			return nil, err
		}
		if id == nil {
			continue // su kien khong co hang ve
		}
		out = append(out, port.TierInventoryRow{
			TicketTypeID: *id, Name: *name, Quota: *quota,
			Available: int(avail), Held: int(held), Paid: int(paid), Issued: int(issued),
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if !found {
		return nil, domain.ErrEventNotFound
	}
	return out, nil
}

// EventSales: cac cau lenh trong MOT transaction REPEATABLE READ READ ONLY (mot snapshot; qua PgBouncer
// transaction mode transaction giu nguyen mot ket noi server toi COMMIT).
func (r *Repository) EventSales(ctx context.Context, eventID string) (port.SalesRows, error) {
	if !uuidRE.MatchString(eventID) {
		return port.SalesRows{}, domain.ErrEventNotFound
	}
	ctx, cancel := context.WithTimeout(ctx, opsQueryTimeout)
	defer cancel()
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return port.SalesRows{}, fmt.Errorf("mo transaction doanh so: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Don da ban = PAID / ISSUED (REFUNDING, REFUNDED, CANCELLED, EXPIRED, HELD, PAYMENT_PENDING bi loai).
	tierQ := `WITH ` + opsTierCTEs + `,
	sold AS (
		SELECT oi.ticket_type_id, SUM(oi.quantity)::bigint AS n, SUM(oi.quantity::bigint * oi.unit_cents)::bigint AS revenue
		  FROM order_items oi JOIN orders o ON o.id = oi.order_id
		 WHERE o.event_id = $1::uuid AND o.status IN ('PAID','ISSUED')
		 GROUP BY oi.ticket_type_id
	)
	SELECT e.organizer_id::text, tt.id::text, tt.name, tt.price_cents, tt.quota,
	       COALESCE(sold.n, 0), COALESCE(sold.revenue, 0), COALESCE(held.n, 0)
	  FROM events e
	  LEFT JOIN tt ON true
	  LEFT JOIN sold ON sold.ticket_type_id = tt.id
	  LEFT JOIN held ON held.ticket_type_id = tt.id
	 WHERE e.id = $1::uuid
	 ORDER BY tt.price_cents, tt.name`
	rows, err := tx.Query(ctx, tierQ, eventID)
	if err != nil {
		return port.SalesRows{}, fmt.Errorf("doc doanh so tung hang ve: %w", err)
	}
	out := port.SalesRows{Tiers: []port.TierSalesRow{}, Payments: []port.PaymentRow{}}
	found := false
	for rows.Next() {
		found = true
		var (
			owner              string
			id, name           *string
			price              *int64
			quota              *int
			sold, revenue, hld int64
		)
		if err := rows.Scan(&owner, &id, &name, &price, &quota, &sold, &revenue, &hld); err != nil {
			rows.Close()
			return port.SalesRows{}, err
		}
		out.OrganizerID = owner
		if id == nil {
			continue
		}
		out.Tiers = append(out.Tiers, port.TierSalesRow{
			TicketTypeID: *id, Name: *name, PriceCents: *price, Quota: *quota,
			Sold: int(sold), RevenueCents: revenue, ActiveHolds: int(hld),
		})
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return port.SalesRows{}, err
	}
	if !found {
		return port.SalesRows{}, domain.ErrEventNotFound
	}

	if err := tx.QueryRow(ctx,
		`SELECT COUNT(*) FROM orders WHERE event_id = $1::uuid AND status IN ('PAID','ISSUED')`,
		eventID).Scan(&out.PaidOrders); err != nil {
		return port.SalesRows{}, fmt.Errorf("dem don da ban: %w", err)
	}

	// Moi don da ban dem MOT lan, theo payment SUCCEEDED moi nhat (hoa tri theo id). So tien la doanh thu
	// cua don (tong order_items), de tong khop gross_revenue_cents. Don khong co payment SUCCEEDED khong o
	// day; app gom phan con lai vao "unknown".
	const payQ = `
	WITH sold AS (
		SELECT o.id,
		       (SELECT COALESCE(SUM(oi.quantity::bigint * oi.unit_cents), 0) FROM order_items oi WHERE oi.order_id = o.id) AS amount
		  FROM orders o
		 WHERE o.event_id = $1::uuid AND o.status IN ('PAID','ISSUED')
	),
	pay AS (
		SELECT DISTINCT ON (p.order_id) p.order_id, p.provider
		  FROM payments p JOIN sold ON sold.id = p.order_id
		 WHERE p.status = 'SUCCEEDED'
		 ORDER BY p.order_id, p.created_at DESC, p.id DESC
	)
	SELECT pay.provider, COUNT(*)::bigint, SUM(sold.amount)::bigint
	  FROM sold JOIN pay ON pay.order_id = sold.id
	 GROUP BY pay.provider
	 ORDER BY 3 DESC, 1`
	prow, err := tx.Query(ctx, payQ, eventID)
	if err != nil {
		return port.SalesRows{}, fmt.Errorf("doc phan bo thanh toan: %w", err)
	}
	defer prow.Close()
	for prow.Next() {
		var p port.PaymentRow
		var orders int64
		if err := prow.Scan(&p.Provider, &orders, &p.AmountCents); err != nil {
			return port.SalesRows{}, err
		}
		p.Orders = int(orders)
		out.Payments = append(out.Payments, p)
	}
	if err := prow.Err(); err != nil {
		return port.SalesRows{}, err
	}
	prow.Close()
	if err := tx.Commit(ctx); err != nil {
		return port.SalesRows{}, fmt.Errorf("ket thuc transaction doanh so: %w", err)
	}
	return out, nil
}
