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
	Status       HoldStatus
	Bucket       int
	ExpiresAt    time.Time
	ExistingHold string
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

// ===================== So lieu nghiep vu =====================

// Ket qua cua mot lan tao hold, dung lam nhan metric. Chi vai gia tri co dinh de
// khong lam no so luong chuoi.
const (
	OutcomeOK          = "ok"
	OutcomeSoldOut     = "sold_out"
	OutcomeNotAdmitted = "not_admitted"
	OutcomeLimit       = "limit"
	OutcomeError       = "error"
)

// HoldMetrics nhan so lieu nghiep vu cua use case giu ghe.
type HoldMetrics interface {
	// HoldAttempt ghi mot lan tao hold voi ket qua va thoi gian xu ly.
	HoldAttempt(outcome string, d time.Duration)
	// OversellGuardRejected ghi khi Postgres chan mot hold ma Redis da cho qua:
	// Redis dang lech so voi su that. Khong phai oversell (Postgres da chan), nhung
	// la dau hieu tang chan nhanh sai -- dung dieu alert nghiem trong can biet.
	OversellGuardRejected()
}

// ===================== Nap ton kho =====================

// InventoryRow la ton kho cua mot bucket, doc tu Postgres.
type InventoryRow struct {
	TicketTypeID string
	Bucket       int
	Available    int
}

// InventorySource doc ton kho cua mot su kien tu nguon su that (Postgres).
type InventorySource interface {
	ListInventory(ctx context.Context, eventID string) ([]InventoryRow, error)
}

// InventorySeeder nap ton kho vao Redis. Chi DIEN vao bucket chua co, khong
// bao gio ghi de so dang bi cac hold tru di.
type InventorySeeder interface {
	SeedInventory(ctx context.Context, eventID, ticketTypeID string, perBucket map[int]int) error
}

// ===================== Su kien ra ngoai =====================

type Publisher interface {
	Publish(ctx context.Context, routingKey string, payload any) error
}
