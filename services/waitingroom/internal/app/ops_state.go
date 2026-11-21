package app

import (
	"context"
	"errors"
	"time"
)

// ErrRoomNotConfigured: wr:{ev}:meta chua co sale_start_ms, tuc phong cho cua su kien chua tung duoc
// ticketing cau hinh. Ops API tra 404 thay vi bao so lieu 0 gia.
var ErrRoomNotConfigured = errors.New("phong cho chua duoc cau hinh")

// OverrideState la lenh ghi de admit rate cua Ops. RatePerSec nil khi khong co ghi de.
type OverrideState struct {
	Active     bool
	RatePerSec *float64
}

// Paused = ghi de dang bat voi nhip 0 (tam dung khan cap).
func (o OverrideState) Paused() bool {
	return o.Active && o.RatePerSec != nil && *o.RatePerSec == 0
}

// ControllerState: co leader admit controller dang giu lease khong, va epoch nhiem ky (0 khi chua co).
type ControllerState struct {
	LeaderPresent bool
	Epoch         int64
}

// Gia tri CheckoutState.Signal.
const (
	CheckoutSignalOK    = "ok"
	CheckoutSignalNone  = "none"  // khong pod ticketing nao con bao cao
	CheckoutSignalError = "error" // doc tin hieu loi
)

// CheckoutState la suc khoe checkout (ticketing) gop tu loadsignal. Cac so lieu nil khi Signal != ok.
type CheckoutState struct {
	Signal      string
	Pods        int
	P99Ms       *float64
	ErrorRate   *float64
	DBPoolUsage *float64
}

// OpsState la anh chup phong cho cho war room Ops (contracts 3.1).
type OpsState struct {
	Lobby           int64
	Queued          int64
	AdmittedActive  int64
	AdmitRatePerSec float64
	Override        OverrideState
	SoldOut         bool
	StockZero       bool
	Controller      ControllerState
	Checkout        CheckoutState
}

// OpsStateReader doc anh chup phong cho. Tra ErrRoomNotConfigured khi phong chua cau hinh.
type OpsStateReader interface {
	QueueOpsState(ctx context.Context, eventID string, now time.Time) (OpsState, error)
}

// CheckoutReader doc suc khoe checkout. Khong tra loi: loi duoc the hien bang Signal.
type CheckoutReader interface {
	Checkout(ctx context.Context) CheckoutState
}
