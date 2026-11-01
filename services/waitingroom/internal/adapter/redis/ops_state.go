package redisadapter

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/eventflow/eventflow/libs/go/loadsignal"
	"github.com/eventflow/eventflow/services/waitingroom/internal/app"
)

// RoomConfigured: phong cho da duoc ticketing cau hinh (meta.sale_start_ms co mat).
// Cung voi SetRateOverride / ClearRateOverride (queue.go) va RateOverride, Queue hien thuc app.OverrideStore.
func (q *Queue) RoomConfigured(ctx context.Context, eventID string) (bool, error) {
	n, err := q.rdb.HExists(ctx, k(eventID, "meta"), "sale_start_ms").Result()
	if err != nil {
		return false, fmt.Errorf("doc sale_start_ms: %w", err)
	}
	return n, nil
}

// RateOverride doc lenh ghi de hien tai (khong fencing, danh cho Ops API).
func (q *Queue) RateOverride(ctx context.Context, eventID string) (rate float64, active bool, err error) {
	vals, err := q.rdb.HMGet(ctx, k(eventID, "meta"), "override", "override_rate").Result()
	if err != nil {
		return 0, false, fmt.Errorf("doc override: %w", err)
	}
	if toString(vals[0]) != "1" {
		return 0, false, nil
	}
	return float64(toInt64(vals[1])) / 1000.0, true, nil
}

// OpsReader doc anh chup phong cho cho war room Ops trong MOT pipeline (cac key cung hash slot {ev}).
// Hien thuc app.OpsStateReader.
type OpsReader struct {
	rdb      redis.Cmdable
	checkout app.CheckoutReader
}

// NewOpsReader. checkout nil -> Signal "none".
func NewOpsReader(rdb redis.Cmdable, checkout app.CheckoutReader) *OpsReader {
	return &OpsReader{rdb: rdb, checkout: checkout}
}

func (o *OpsReader) QueueOpsState(ctx context.Context, eventID string, now time.Time) (app.OpsState, error) {
	pipe := o.rdb.Pipeline()
	lobby := pipe.SCard(ctx, k(eventID, "lobby"))
	queued := pipe.ZCard(ctx, k(eventID, "queue"))
	// Chi dem nguoi con trong cua so mua: score = expire_at_ms; nguoi het han duoc status.lua don tre.
	admitted := pipe.ZCount(ctx, k(eventID, "admitted"), "("+strconv.FormatInt(now.UnixMilli(), 10), "+inf")
	meta := pipe.HMGet(ctx, k(eventID, "meta"),
		"admit_rate", "override", "override_rate", "sold_out", "stock_zero", "sale_start_ms")
	leaderKey := pipe.Exists(ctx, k(eventID, "leader"))
	epoch := pipe.Get(ctx, epochKey(eventID))
	if _, err := pipe.Exec(ctx); err != nil && !errors.Is(err, redis.Nil) {
		return app.OpsState{}, fmt.Errorf("doc trang thai ops: %w", err)
	}

	m, err := meta.Result()
	if err != nil {
		return app.OpsState{}, fmt.Errorf("doc meta: %w", err)
	}
	if m[5] == nil {
		return app.OpsState{}, app.ErrRoomNotConfigured
	}
	var ep int64
	if v, err := epoch.Int64(); err == nil {
		ep = v
	} else if !errors.Is(err, redis.Nil) {
		return app.OpsState{}, fmt.Errorf("doc leader_epoch: %w", err)
	}

	st := app.OpsState{
		Lobby:           lobby.Val(),
		Queued:          queued.Val(),
		AdmittedActive:  admitted.Val(),
		AdmitRatePerSec: float64(toInt64(m[0])) / 1000.0,
		SoldOut:         toString(m[3]) == "1",
		StockZero:       toString(m[4]) == "1",
		Controller:      app.ControllerState{LeaderPresent: leaderKey.Val() > 0, Epoch: ep},
	}
	if toString(m[1]) == "1" {
		r := float64(toInt64(m[2])) / 1000.0
		st.Override = app.OverrideState{Active: true, RatePerSec: &r}
	}
	for _, c := range []interface{ Err() error }{lobby, queued, admitted, leaderKey} {
		if err := c.Err(); err != nil {
			return app.OpsState{}, fmt.Errorf("doc trang thai ops: %w", err)
		}
	}
	if o.checkout != nil {
		st.Checkout = o.checkout.Checkout(ctx)
	} else {
		st.Checkout = app.CheckoutState{Signal: app.CheckoutSignalNone}
	}
	return st, nil
}

// LoadSignalCheckout doi tin hieu loadsignal cua ticketing thanh app.CheckoutState.
type LoadSignalCheckout struct {
	r *loadsignal.Reader
}

func NewLoadSignalCheckout(r *loadsignal.Reader) *LoadSignalCheckout { return &LoadSignalCheckout{r: r} }

func (c *LoadSignalCheckout) Checkout(ctx context.Context) app.CheckoutState {
	agg, err := c.r.Read(ctx)
	if errors.Is(err, loadsignal.ErrNoSignal) {
		return app.CheckoutState{Signal: app.CheckoutSignalNone}
	}
	if err != nil {
		return app.CheckoutState{Signal: app.CheckoutSignalError}
	}
	p99 := float64(agg.P99) / float64(time.Millisecond)
	er, pool := agg.ErrorRate, agg.PoolUsage
	return app.CheckoutState{Signal: app.CheckoutSignalOK, Pods: agg.Pods, P99Ms: &p99, ErrorRate: &er, DBPoolUsage: &pool}
}
