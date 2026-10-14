//go:build integration

package postgresadapter_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/eventflow/eventflow/libs/go/testx"
	postgresadapter "github.com/eventflow/eventflow/services/ticketing/internal/adapter/postgres"
)

func setupPurchaseLimitTest(t *testing.T) (*postgresadapter.Repository, *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	dsn := testx.Postgres(t)

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect postgres: %v", err)
	}
	t.Cleanup(pool.Close)

	return postgresadapter.NewRepository(pool), pool
}

func TestCountPurchased_ChiDemDonHoatDong(t *testing.T) {
	repo, pool := setupPurchaseLimitTest(t)
	ctx := context.Background()

	orgID := uuid.New().String()
	eventID := uuid.New().String()
	otherEventID := uuid.New().String()
	buyerA := uuid.New().String()
	buyerB := uuid.New().String()
	ticketTypeID := uuid.New().String()
	otherTicketTypeID := uuid.New().String()

	// Seed identities
	const insID = `INSERT INTO identities (id, email, password_hash, role) VALUES ($1::uuid, $2, 'pass', $3)`
	_, _ = pool.Exec(ctx, insID, orgID, "org-"+orgID+"@test.com", "organizer")
	_, _ = pool.Exec(ctx, insID, buyerA, "buyerA-"+buyerA+"@test.com", "buyer")
	_, _ = pool.Exec(ctx, insID, buyerB, "buyerB-"+buyerB+"@test.com", "buyer")

	// Seed events
	const insEv = `INSERT INTO events (id, organizer_id, slug, title, starts_at, sale_start_at, status, max_per_identity, max_per_order)
		VALUES ($1::uuid, $2::uuid, $3, 'Ev', now() + interval '10 days', now(), 'ON_SALE', 4, 4)`
	_, _ = pool.Exec(ctx, insEv, eventID, orgID, "ev-"+eventID)
	_, _ = pool.Exec(ctx, insEv, otherEventID, orgID, "ev-"+otherEventID)

	// Seed ticket types
	const insTT = `INSERT INTO ticket_types (id, event_id, name, price_cents, quota, bucket_count)
		VALUES ($1::uuid, $2::uuid, 'TT', 100000, 1000, 32)`
	_, _ = pool.Exec(ctx, insTT, ticketTypeID, eventID)
	_, _ = pool.Exec(ctx, insTT, otherTicketTypeID, otherEventID)

	// Helper to insert an order with an order_item
	insertOrder := func(evID, idID, ttID, status string, qty int) {
		orderID := uuid.New().String()
		const oQ = `INSERT INTO orders (id, event_id, identity_id, status, total_cents, expires_at)
			VALUES ($1::uuid, $2::uuid, $3::uuid, $4, $5, $6)`
		if _, err := pool.Exec(ctx, oQ, orderID, evID, idID, status, int64(100000*qty), time.Now().Add(time.Hour)); err != nil {
			t.Fatalf("insert order (%s): %v", status, err)
		}
		const oiQ = `INSERT INTO order_items (order_id, ticket_type_id, bucket, quantity, unit_cents)
			VALUES ($1::uuid, $2::uuid, 0, $3, 100000)`
		if _, err := pool.Exec(ctx, oiQ, orderID, ttID, qty); err != nil {
			t.Fatalf("insert order item (%s): %v", status, err)
		}
	}

	// 1. Buyer A trên eventID:
	// Các trạng thái ĐƯỢC TÍNH: HELD (2), PAID (3), ISSUED (1) -> Tổng = 6
	// (Lưu ý: Không thể cùng có HELD và PAYMENT_PENDING trên cùng 1 buyer vì ràng buộc BR-O3:
	// uq_active_order_per_identity_event chỉ cho phép tối đa 1 đơn HELD hoặc PAYMENT_PENDING).
	insertOrder(eventID, buyerA, ticketTypeID, "HELD", 2)
	insertOrder(eventID, buyerA, ticketTypeID, "PAID", 3)
	insertOrder(eventID, buyerA, ticketTypeID, "ISSUED", 1)

	// Các trạng thái KHÔNG ĐƯỢC TÍNH: EXPIRED (2), CANCELLED (4), REFUNDED (5)
	insertOrder(eventID, buyerA, ticketTypeID, "EXPIRED", 2)
	insertOrder(eventID, buyerA, ticketTypeID, "CANCELLED", 4)
	insertOrder(eventID, buyerA, ticketTypeID, "REFUNDED", 5)

	// 2. Buyer B trên eventID: kiểm tra PAYMENT_PENDING (1) + PAID (9) -> Tổng = 10
	insertOrder(eventID, buyerB, ticketTypeID, "PAYMENT_PENDING", 1)
	insertOrder(eventID, buyerB, ticketTypeID, "PAID", 9)

	// 3. Buyer A trên otherEventID (không được cộng vào eventID)
	insertOrder(otherEventID, buyerA, otherTicketTypeID, "PAID", 6)

	// Kiểm tra CountPurchased cho Buyer A: 2 (HELD) + 3 (PAID) + 1 (ISSUED) = 6
	countA, err := repo.CountPurchased(ctx, eventID, buyerA)
	if err != nil {
		t.Fatalf("CountPurchased failed: %v", err)
	}
	if countA != 6 {
		t.Fatalf("CountPurchased for Buyer A = %d, expected 6 (2 HELD + 3 PAID + 1 ISSUED)", countA)
	}

	// Kiểm tra CountPurchased cho Buyer B: 1 (PAYMENT_PENDING) + 9 (PAID) = 10
	countB, err := repo.CountPurchased(ctx, eventID, buyerB)
	if err != nil {
		t.Fatalf("CountPurchased for Buyer B failed: %v", err)
	}
	if countB != 10 {
		t.Fatalf("CountPurchased for Buyer B = %d, expected 10 (1 PAYMENT_PENDING + 9 PAID)", countB)
	}

	// Kiểm tra Buyer mới chưa có đơn nào
	buyerC := uuid.New().String()
	_, _ = pool.Exec(ctx, insID, buyerC, "buyerC-"+buyerC+"@test.com", "buyer")
	countC, err := repo.CountPurchased(ctx, eventID, buyerC)
	if err != nil {
		t.Fatalf("CountPurchased for Buyer C failed: %v", err)
	}
	if countC != 0 {
		t.Fatalf("CountPurchased for Buyer C = %d, expected 0", countC)
	}
}
