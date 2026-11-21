package port

import "context"

// OpsReader doc so lieu cho war room Ops va doanh so organizer (contracts 4).
// Moi ham tra domain.ErrEventNotFound (khong boc) khi su kien khong ton tai.
type OpsReader interface {
	EventExists(ctx context.Context, eventID string) (bool, error)
	EventOwner(ctx context.Context, eventID string) (organizerID string, err error)
	// TierInventory: mot cau SQL duy nhat (mot snapshot). Su kien khong co hang ve -> slice rong, khong loi.
	TierInventory(ctx context.Context, eventID string) ([]TierInventoryRow, error)
	// EventSales: mot snapshot (REPEATABLE READ READ ONLY).
	EventSales(ctx context.Context, eventID string) (SalesRows, error)
}

// TierInventoryRow: moi ghe nam o toi da mot trong Held / Paid / Issued (contracts 4.1, CR-1).
type TierInventoryRow struct {
	TicketTypeID string
	Name         string
	Quota        int
	Available    int
	Held         int
	Paid         int
	Issued       int
}

// SalesRows la so lieu doanh so mot su kien trong cung mot snapshot.
type SalesRows struct {
	Tiers []TierSalesRow
	// Payments: don da ban gom theo nha cung cap cua MOT payment SUCCEEDED moi nhat cua don. Don da ban
	// khong co payment SUCCEEDED nao KHONG nam o day; app gom phan con lai vao "unknown".
	Payments   []PaymentRow
	PaidOrders int
	// OrganizerID doc trong cung snapshot. Rong neu adapter khong cung cap (app se goi EventOwner).
	OrganizerID string
}

type TierSalesRow struct {
	TicketTypeID string
	Name         string
	PriceCents   int64
	Quota        int
	Sold         int
	ActiveHolds  int
	RevenueCents int64
}

// PaymentRow: AmountCents la doanh thu cua don (SUM quantity*unit_cents), khong phai payments.amount_cents,
// de tong khop gross_revenue_cents.
type PaymentRow struct {
	Provider    string
	Orders      int
	AmountCents int64
}
