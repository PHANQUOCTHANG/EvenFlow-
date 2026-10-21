package redisadapter

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/eventflow/eventflow/libs/go/activeevents"
	"github.com/eventflow/eventflow/services/ticketing/internal/port"
)

const metaSaleStart = "sale_start_ms"

// PublishActiveEvents ghi tap su kien cho admit controller (libs/go/activeevents) va T0 cua tung su
// kien vao meta phong cho (sale_start_ms, doc boi join.lua va controller).
//
// T0 chi duoc ghi khi chua co, hoac khi T0 dang luu VAN CON o tuong lai (organizer doi lich truoc gio
// mo). T0 da qua thi khong bao gio sua: lottery da chay va thu tu trong hang da chot theo no.
func (g *Gate) PublishActiveEvents(ctx context.Context, events []port.SellableEvent, now time.Time) error {
	ids := make([]string, 0, len(events))
	var errs []error
	for _, e := range events {
		ids = append(ids, e.ID)
		if err := g.syncSaleStart(ctx, e, now); err != nil {
			errs = append(errs, err)
		}
	}
	if err := activeevents.Replace(ctx, g.rdb, ids); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

func (g *Gate) syncSaleStart(ctx context.Context, e port.SellableEvent, now time.Time) error {
	if e.SaleStartAt.IsZero() {
		return nil // su kien co dinh (ACTIVE_EVENT_IDS) khong co trong events: T0 do seed.sh ghi
	}
	key := waitingMetaKey(e.ID)
	want := e.SaleStartAt.UnixMilli()
	cur, err := g.rdb.HGet(ctx, key, metaSaleStart).Int64()
	switch {
	case errors.Is(err, redis.Nil):
		// chua co: ghi
	case err != nil:
		return fmt.Errorf("doc %s cua %s: %w", metaSaleStart, e.ID, err)
	case cur == want || cur <= now.UnixMilli():
		return nil
	}
	if err := g.rdb.HSet(ctx, key, metaSaleStart, want).Err(); err != nil {
		return fmt.Errorf("ghi %s cua %s: %w", metaSaleStart, e.ID, err)
	}
	return nil
}
