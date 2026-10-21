package postgresadapter

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/eventflow/eventflow/services/ticketing/internal/domain"
	"github.com/eventflow/eventflow/services/ticketing/internal/port"
	"github.com/jackc/pgx/v5"
)

// OutboxTicketIssuedPayload la cau truc du lieu su kien outbox ticketing.ticket.issued
// phat ra cho Notification Service (EVF-75) gui email/SMS cho khach.
type OutboxTicketIssuedPayload struct {
	OrderID     string          `json:"order_id"`
	EventID     string          `json:"event_id"`
	IdentityID  string          `json:"identity_id"`
	TicketCount int             `json:"ticket_count"`
	Tickets     []domain.Ticket `json:"tickets"`
	IssuedAt    time.Time       `json:"issued_at"`
}

// GetOrderForIssuance doc thong tin don hang va cac order_items.
func (r *Repository) GetOrderForIssuance(ctx context.Context, orderID string) (port.OrderIssuanceInfo, error) {
	if !uuidRE.MatchString(orderID) {
		return port.OrderIssuanceInfo{}, domain.ErrOrderNotFound
	}

	const orderQuery = `
		SELECT id::text, event_id::text, identity_id::text, status, expires_at
		  FROM orders
		 WHERE id = $1::uuid;
	`

	var info port.OrderIssuanceInfo
	err := r.pool.QueryRow(ctx, orderQuery, orderID).Scan(
		&info.OrderID,
		&info.EventID,
		&info.IdentityID,
		&info.Status,
		&info.ExpiresAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return info, domain.ErrOrderNotFound
		}
		return info, fmt.Errorf("doc order that bai: %w", err)
	}

	const itemsQuery = `
		SELECT ticket_type_id::text, quantity, unit_cents
		  FROM order_items
		 WHERE order_id = $1::uuid;
	`
	rows, err := r.pool.Query(ctx, itemsQuery, orderID)
	if err != nil {
		return info, fmt.Errorf("doc order_items that bai: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var item port.OrderIssuanceItem
		if err := rows.Scan(&item.TicketTypeID, &item.Quantity, &item.UnitCents); err != nil {
			return info, fmt.Errorf("scan order_item that bai: %w", err)
		}
		info.Items = append(info.Items, item)
	}

	return info, rows.Err()
}

// IssueTicketsTx thuc thi trong CUNG MOT TRANSACTION an toan (Quy tac bat bien 1):
// 1. Khoa don bang SELECT ... FOR UPDATE chong tranh chap song song.
// 2. Kiem tra tinh hop le cua don hang (phai la PAID hoac ISSUED).
// 3. Neu da ISSUED va co ve, tra ve alreadyIssued = true.
// 4. Luu cac ve vao bang tickets.
// 5. Consume toan bo seat_holds cua don hang (BR-O7).
// 6. Cap nhat orders.status = 'ISSUED'.
// 7. Ghi outbox_events ticketing.ticket.issued.
func (r *Repository) IssueTicketsTx(
	ctx context.Context,
	order port.OrderIssuanceInfo,
	tickets []domain.Ticket,
	traceID string,
) (alreadyIssued bool, err error) {
	if !uuidRE.MatchString(order.OrderID) {
		return false, domain.ErrOrderNotFound
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return false, fmt.Errorf("khoi tao transaction that bai: %w", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	// 1. Khoa dong don hang FOR UPDATE
	const lockOrderQuery = `
		SELECT status
		  FROM orders
		 WHERE id = $1::uuid
		   FOR UPDATE;
	`
	var currentStatus string
	if err := tx.QueryRow(ctx, lockOrderQuery, order.OrderID).Scan(&currentStatus); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, domain.ErrOrderNotFound
		}
		return false, fmt.Errorf("khoa order FOR UPDATE that bai: %w", err)
	}

	// 2. Kiem tra da cap ve truoc do chua (Idempotent AC-3)
	if currentStatus == string(domain.TicketStatusIssued) {
		var existingCount int
		const countTickets = `SELECT count(*)::int FROM tickets WHERE order_id = $1::uuid;`
		if err := tx.QueryRow(ctx, countTickets, order.OrderID).Scan(&existingCount); err == nil && existingCount > 0 {
			if err := tx.Commit(ctx); err != nil {
				return false, fmt.Errorf("commit transaction idempotent that bai: %w", err)
			}
			return true, nil
		}
	}

	if currentStatus != "PAID" && currentStatus != string(domain.TicketStatusIssued) {
		return false, domain.ErrOrderNotPaid
	}

	// 3. Chen cac ban ghi ve vao bang tickets
	const insertTicketQuery = `
		INSERT INTO tickets (id, order_id, ticket_type_id, jti, qr_sig, status, created_at)
		VALUES ($1::uuid, $2::uuid, $3::uuid, $4, $5, $6, $7);
	`
	for _, t := range tickets {
		_, err := tx.Exec(ctx, insertTicketQuery,
			t.ID,
			t.OrderID,
			t.TicketTypeID,
			t.JTI,
			t.QRSig,
			string(t.Status),
			t.CreatedAt,
		)
		if err != nil {
			return false, fmt.Errorf("chen ban ghi ticket that bai (jti=%s): %w", t.JTI, err)
		}
	}

	// 4. Consume seat_holds cua don hang (BR-O7): danh dau consumed_at de hold sweeper khong thu hoi
	const consumeHoldsQuery = `
		UPDATE seat_holds
		   SET consumed_at = now()
		 WHERE order_id = $1::uuid
		   AND consumed_at IS NULL
		   AND released_at IS NULL;
	`
	if _, err := tx.Exec(ctx, consumeHoldsQuery, order.OrderID); err != nil {
		return false, fmt.Errorf("consume seat_holds that bai: %w", err)
	}

	// 5. Cap nhat orders sang trang thai ISSUED
	const updateOrderQuery = `
		UPDATE orders
		   SET status = 'ISSUED',
		       updated_at = now()
		 WHERE id = $1::uuid;
	`
	if _, err := tx.Exec(ctx, updateOrderQuery, order.OrderID); err != nil {
		return false, fmt.Errorf("cap nhat orders sang ISSUED that bai: %w", err)
	}

	// 6. Ghi su kien outbox_events ticketing.ticket.issued (Quy tac bat bien 1)
	outboxPayload := OutboxTicketIssuedPayload{
		OrderID:     order.OrderID,
		EventID:     order.EventID,
		IdentityID:  order.IdentityID,
		TicketCount: len(tickets),
		Tickets:     tickets,
		IssuedAt:    time.Now().UTC(),
	}
	outboxBytes, err := json.Marshal(outboxPayload)
	if err != nil {
		return false, fmt.Errorf("marshal outbox payload that bai: %w", err)
	}

	const insertOutboxQuery = `
		INSERT INTO outbox_events (aggregate_type, aggregate_id, routing_key, payload, trace_id, created_at)
		VALUES ('order', $1, 'ticketing.ticket.issued', $2, $3, now());
	`
	if _, err := tx.Exec(ctx, insertOutboxQuery, order.OrderID, outboxBytes, traceID); err != nil {
		return false, fmt.Errorf("chen outbox_events that bai: %w", err)
	}

	// 7. Commit Transaction
	if err := tx.Commit(ctx); err != nil {
		return false, fmt.Errorf("commit transaction phat ve that bai: %w", err)
	}

	return false, nil
}

// GetTicketsByOrderID tra ve danh sach cac ve da phat hanh cua mot don hang.
func (r *Repository) GetTicketsByOrderID(ctx context.Context, orderID string) ([]domain.Ticket, error) {
	if !uuidRE.MatchString(orderID) {
		return nil, nil
	}

	const q = `
		SELECT id::text, order_id::text, ticket_type_id::text, jti, qr_sig, status, checked_in_at, created_at
		  FROM tickets
		 WHERE order_id = $1::uuid
		 ORDER BY created_at ASC;
	`
	rows, err := r.pool.Query(ctx, q, orderID)
	if err != nil {
		return nil, fmt.Errorf("doc tickets theo order_id that bai: %w", err)
	}
	defer rows.Close()

	var tickets []domain.Ticket
	for rows.Next() {
		var t domain.Ticket
		var statusStr string
		if err := rows.Scan(
			&t.ID,
			&t.OrderID,
			&t.TicketTypeID,
			&t.JTI,
			&t.QRSig,
			&statusStr,
			&t.CheckedInAt,
			&t.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan ticket that bai: %w", err)
		}
		t.Status = domain.TicketStatus(statusStr)
		tickets = append(tickets, t)
	}

	return tickets, rows.Err()
}

// GetTicketByJTI tra ve thong tin ve dua vao JTI (AC-4, AC-5).
func (r *Repository) GetTicketByJTI(ctx context.Context, jti string) (domain.Ticket, error) {
	const q = `
		SELECT id::text, order_id::text, ticket_type_id::text, jti, qr_sig, status, checked_in_at, created_at
		  FROM tickets
		 WHERE jti = $1;
	`
	var t domain.Ticket
	var statusStr string
	err := r.pool.QueryRow(ctx, q, jti).Scan(
		&t.ID,
		&t.OrderID,
		&t.TicketTypeID,
		&t.JTI,
		&t.QRSig,
		&statusStr,
		&t.CheckedInAt,
		&t.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return t, domain.ErrTicketNotFound
		}
		return t, fmt.Errorf("doc ticket theo jti that bai: %w", err)
	}
	t.Status = domain.TicketStatus(statusStr)
	return t, nil
}

// UpdateTicketStatus cap nhat trang thai cua mot ve (vi du chuyen sang CHECKED_IN).
func (r *Repository) UpdateTicketStatus(
	ctx context.Context,
	ticketID string,
	currentStatus, newStatus domain.TicketStatus,
	checkedInAt *time.Time,
) error {
	if !uuidRE.MatchString(ticketID) {
		return domain.ErrTicketNotFound
	}

	const q = `
		UPDATE tickets
		   SET status = $1,
		       checked_in_at = $2
		 WHERE id = $3::uuid
		   AND status = $4;
	`
	tag, err := r.pool.Exec(ctx, q, string(newStatus), checkedInAt, ticketID, string(currentStatus))
	if err != nil {
		return fmt.Errorf("cap nhat trang thai ve that bai: %w", err)
	}
	if tag.RowsAffected() == 0 {
		// Kiem tra xem ve co ton tai khong
		var exists bool
		const checkQ = `SELECT EXISTS(SELECT 1 FROM tickets WHERE id = $1::uuid);`
		_ = r.pool.QueryRow(ctx, checkQ, ticketID).Scan(&exists)
		if !exists {
			return domain.ErrTicketNotFound
		}
		return domain.ErrTicketAlreadyUsed
	}
	return nil
}

var _ port.TicketRepository = (*Repository)(nil)

// ListOrderHoldIDs tra id moi hold (seat_holds) cua don, ke ca hold da consumed: dung de don hold con
// sot trong Redis sau khi phat ve. Id khong phai UUID hop le thi coi nhu don khong co hold.
func (r *Repository) ListOrderHoldIDs(ctx context.Context, orderID string) ([]string, error) {
	if !uuidRE.MatchString(orderID) {
		return nil, nil
	}
	rows, err := r.pool.Query(ctx, `SELECT id::text FROM seat_holds WHERE order_id = $1::uuid`, orderID)
	if err != nil {
		return nil, fmt.Errorf("liet ke hold cua don: %w", err)
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
