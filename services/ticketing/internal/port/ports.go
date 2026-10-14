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
	// MaxPerIdentity la han muc ve/nguoi/su kien (BR-O4), kiem lai trong transaction.
	MaxPerIdentity int
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

// ===================== Han muc mua (EV-130) =====================

// PurchaseLimit la han muc mua cua mot khach tren mot su kien (BR-O4). UI dung MaxSelectable lam
// so luong toi da cho chon: so nay phu thuoc lich su mua ma UI khong biet, nen SERVER phai tinh.
type PurchaseLimit struct {
	MaxPerOrder    int `json:"max_per_order"`
	MaxPerIdentity int `json:"max_per_identity"`
	// Used la so ve dang chiem han muc (domain.QuotaOrderStatuses), ke ca hold dang giu.
	Used          int  `json:"used"`
	MaxSelectable int  `json:"max_selectable"`
	HasActiveHold bool `json:"has_active_hold"`
}

// PurchaseLimitSource doc han muc va so ve dang chiem han muc tu nguon su that. MaxSelectable do
// tang app tinh.
type PurchaseLimitSource interface {
	GetPurchaseLimit(ctx context.Context, eventID, identityID string) (PurchaseLimit, error)
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

// ===================== Doi soat ton kho (EV-132) =====================

// RedisHold la mot hold dang nam trong hash hold:{ev}:h cua Redis.
type RedisHold struct {
	TicketTypeID string
	Bucket       int
	Quantity     int
}

// RedisSnapshot la anh chup NGUYEN TU (mot MULTI) ton kho va cac hold cua mot su kien.
type RedisSnapshot struct {
	// Inventory: hang ve -> bucket -> so ve con. Bucket thieu nghia la 0.
	Inventory map[string]map[int]int
	// Holds: hold_id -> hold dang duoc Redis ghi nhan (chua release/consume).
	Holds map[string]RedisHold
}

// InventoryLedger la phan cua tang chan Redis ma job doi soat can.
type InventoryLedger interface {
	// Snapshot doc ton kho cac hang ve va hash hold cua su kien trong mot giao dich.
	Snapshot(ctx context.Context, eventID string, ticketTypeIDs []string) (RedisSnapshot, error)
	// AdjustInventory cong delta (am hoac duong) vao mot bucket, khong de ket qua < 0.
	// Dung delta thay vi SET de khong de len mot hold dang bay.
	AdjustInventory(ctx context.Context, eventID, ticketTypeID string, bucket, delta int) (int, error)
	// TryReconcileLock giu khoa doi soat cua su kien trong ttl; false neu instance khac dang giu.
	TryReconcileLock(ctx context.Context, eventID string, ttl time.Duration) (bool, error)
}

// HoldState la trang thai cua mot hold trong Postgres.
type HoldState struct {
	Released bool
	Consumed bool
}

// HoldStateSource tra trang thai hold trong Postgres. Hold khong co trong ket qua nghia la
// Postgres chua (hoac khong bao gio) ghi no.
type HoldStateSource interface {
	HoldStates(ctx context.Context, holdIDs []string) (map[string]HoldState, error)
}

// ReconcilerMetrics nhan so lieu cua job doi soat.
type ReconcilerMetrics interface {
	// InventoryDrift dat do lech chua giai thich duoc (tong gia tri tuyet doi theo bucket) cua
	// mot hang ve sau vong doi soat gan nhat. 0 nghia la hai tang khop nhau.
	InventoryDrift(ticketTypeID string, drift int)
	// DriftCorrected cong so ve da duoc sua trong Redis; direction la "refill" (Redis thieu)
	// hoac "drain" (Redis phong).
	DriftCorrected(direction string, tickets int)
}

// ===================== Su kien ra ngoai =====================

type Publisher interface {
	Publish(ctx context.Context, routingKey string, payload any) error
}
