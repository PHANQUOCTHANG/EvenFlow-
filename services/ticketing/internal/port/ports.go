// Package port khai bao cac interface ma tang app can.
//
// Dao nguoc phu thuoc: app dinh nghia cai no CAN, adapter di hien thuc. Nho vay
// doi Redis sang Valkey hay doi pgx sang driver khac chi dung o tang adapter.
package port

import (
	"context"
	"time"
)

// ===================== Ton kho (Redis) =====================

type HoldStatus string

const (
	HoldOK             HoldStatus = "OK"
	HoldSoldOut        HoldStatus = "SOLD_OUT"
	HoldAlreadyHolding HoldStatus = "ALREADY_HOLDING"
	HoldBadQty         HoldStatus = "BAD_QTY"
)

type HoldRequest struct {
	EventID      string
	TicketTypeID string
	IdentityID   string
	HoldID       string
	Quantity     int
	TTL          time.Duration
	BucketCount  int
	StartBucket  int
}

type HoldResult struct {
	Status        HoldStatus
	Bucket        int
	ExpiresAt     time.Time
	ExistingHold  string
}

// InventoryGate la cong chan nhanh tren Redis.
type InventoryGate interface {
	Hold(ctx context.Context, req HoldRequest) (HoldResult, error)
	// Release phai IDEMPOTENT: goi nhieu lan chi tra kho dung mot lan (BR-O7).
	Release(ctx context.Context, eventID, holdID string) error
	// Consume dung khi da thanh toan: xoa hold nhung KHONG tra kho.
	Consume(ctx context.Context, eventID, holdID string) error
	IsAdmitted(ctx context.Context, eventID, queueToken string) (bool, error)
	// ExpiredHolds tra ve cac hold da qua han, cho sweeper xu ly.
	ExpiredHolds(ctx context.Context, eventID string, limit int) ([]string, error)
}

// ===================== Ben vung (Postgres) =====================

type SaleConfig struct {
	PriceCents     int64
	BucketCount    int
	HoldTTLSeconds int
	MaxPerOrder    int
	MaxPerIdentity int
}

type PersistHoldParams struct {
	EventID      string
	TicketTypeID string
	IdentityID   string
	HoldID       string
	Bucket       int
	Quantity     int
	UnitCents    int64
	ExpiresAt    time.Time
}

type HoldView struct {
	OrderID   string    `json:"order_id"`
	HoldID    string    `json:"hold_id"`
	Bucket    int       `json:"-"`
	ExpiresAt time.Time `json:"expires_at"`
}

type Repository interface {
	GetSaleConfig(ctx context.Context, eventID, ticketTypeID string) (SaleConfig, error)
	CountPurchased(ctx context.Context, eventID, identityID string) (int, error)
	GetActiveHold(ctx context.Context, eventID, identityID string) (HoldView, error)

	// PersistHold chot hold trong MOT transaction: tru ton kho, tao don, tao
	// hold, ghi outbox. Tra ve ErrInventoryExhausted khi CHECK constraint chan.
	PersistHold(ctx context.Context, p PersistHoldParams) (HoldView, error)

	// ReleaseHold tra kho, IDEMPOTENT theo dieu kien released_at IS NULL.
	// Tra ve released=false neu hold da duoc xu ly truoc do.
	ReleaseHold(ctx context.Context, holdID string) (released bool, err error)
}

// ===================== Su kien ra ngoai =====================

type Publisher interface {
	Publish(ctx context.Context, routingKey string, payload any) error
}
