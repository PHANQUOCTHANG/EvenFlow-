package port

import (
	"context"
	"time"
)

// ===================== API don hang (EV-NEW-2) =====================

// OrderItemView la mot dong hang trong don.
type OrderItemView struct {
	TicketTypeID string
	TierName     string
	Quantity     int
	UnitCents    int64
}

// OrderView la don hang nhin tu phia khach. Status la trang thai THO cua bang orders.
type OrderView struct {
	OrderID    string
	EventID    string
	EventTitle string
	Venue      string
	Status     string
	TotalCents int64
	Currency   string
	ExpiresAt  time.Time
	CreatedAt  time.Time
	// HoldID la hold dang song (chua tra / chua dung); rong khi don khong con giu ghe.
	HoldID string
	Items  []OrderItemView
	// Thanh toan gan nhat: PaidAt co khi co payment SUCCEEDED; PaymentFailed khi payment gan nhat that bai.
	PaidAt          *time.Time
	PaymentProvider string
	PaymentFailed   bool
}

// OrderReader doc don hang CUA MOT KHACH. Don khong ton tai va don cua nguoi khac cho CUNG ket qua
// (ErrOrderNotFound): khong de do ra don nao ton tai.
type OrderReader interface {
	GetOrder(ctx context.Context, orderID, identityID string) (OrderView, error)
	// OrderOwnedBy cho biet don co thuoc khach khong (kiem quyen cho ve va phat hanh ve).
	OrderOwnedBy(ctx context.Context, orderID, identityID string) (bool, error)
}

// CancelResult la ket qua khach tu huy hold.
type CancelResult struct {
	EventID string
	// Cancelled=true khi lan goi nay tra kho; false khi hold da duoc huy / het han truoc do (idempotent).
	Cancelled bool
}

// HoldCanceller huy hold CUA MOT KHACH trong Postgres: tra kho va dat don CANCELLED trong mot transaction.
// Hold khong ton tai hoac cua nguoi khac -> ErrOrderNotFound. Don da thanh toan / dang thanh toan ->
// ErrHoldNotCancellable. Goi lai khi da huy / het han -> thanh cong, Cancelled=false.
type HoldCanceller interface {
	CancelHold(ctx context.Context, holdID, identityID string) (CancelResult, error)
}

// TierInfo la hang ve cua mot su kien dang ban.
type TierInfo struct {
	TicketTypeID string
	Name         string
	Capacity     int
}

// AvailabilitySource doc hang ve (Postgres) va so ve con hien tai (Redis, duong nong).
type AvailabilitySource interface {
	ListTiers(ctx context.Context, eventID string) ([]TierInfo, error)
	// Remaining tra tong so ve con theo hang ve doc tu Redis.
	Remaining(ctx context.Context, eventID string, ticketTypeIDs []string) (map[string]int, error)
}
