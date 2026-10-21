package postgresadapter

import (
	"context"
	"fmt"
	"time"

	"github.com/eventflow/eventflow/services/ticketing/internal/port"
)

// ListSellableEvents tra su kien dang ban / het ve tam thoi (SOLD_OUT co the co lai ve khi hold het
// han) / da len lich (SCHEDULED) va mo ban trong `horizon` toi. Su kien SCHEDULED duoc dua vao truoc T0
// de phong cho co sale_start_ms va mo LOBBY + lottery dung gio.
func (r *Repository) ListSellableEvents(ctx context.Context, horizon time.Duration) ([]port.SellableEvent, error) {
	const q = `
		SELECT id::text, sale_start_at
		  FROM events
		 WHERE status IN ('ON_SALE', 'SOLD_OUT')
		    OR (status = 'SCHEDULED' AND sale_start_at <= now() + $1::interval)
		 ORDER BY sale_start_at, id`
	rows, err := r.pool.Query(ctx, q, fmt.Sprintf("%d seconds", int64(horizon.Seconds())))
	if err != nil {
		return nil, fmt.Errorf("liet ke su kien dang ban: %w", err)
	}
	defer rows.Close()
	var out []port.SellableEvent
	for rows.Next() {
		var e port.SellableEvent
		if err := rows.Scan(&e.ID, &e.SaleStartAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
