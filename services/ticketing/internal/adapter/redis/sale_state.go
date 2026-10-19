package redisadapter

import (
	"context"
	"fmt"
	"strconv"

	"github.com/redis/go-redis/v9"

	"github.com/eventflow/eventflow/services/ticketing/internal/port"
)

// waitingMetaKey la hash meta cua PHONG CHO cho mot su kien. Ticketing ghi hai co o day (hop dong giua hai
// service, xem script admit.lua / join.lua / status.lua cua waitingroom):
//
//	sold_out   = "1"  het ve han: phong cho tra SOLD_OUT, khong nhan nguoi moi, ngung admit
//	stock_zero = "1"  kho dang 0 nhung con hold: chi ngung admit
func waitingMetaKey(eventID string) string { return fmt.Sprintf("wr:{%s}:meta", eventID) }

const (
	metaSoldOut   = "sold_out"
	metaStockZero = "stock_zero"
)

// Stock doc tong ton kho, so hold song va co "da nap" trong MOT giao dich (MULTI): cung mot thoi diem,
// nen khong co hold nao chen giua hai so lieu.
func (g *Gate) Stock(ctx context.Context, eventID string, ticketTypeIDs []string) (port.StockInfo, error) {
	pipe := g.rdb.TxPipeline()
	cmds := make(map[string]*redis.MapStringStringCmd, len(ticketTypeIDs))
	for _, tt := range ticketTypeIDs {
		cmds[tt] = pipe.HGetAll(ctx, invKey(eventID, tt))
	}
	holds := pipe.HLen(ctx, holdHKey(eventID))
	if _, err := pipe.Exec(ctx); err != nil {
		return port.StockInfo{}, fmt.Errorf("doc ton kho redis: %w", err)
	}

	info := port.StockInfo{Seeded: len(ticketTypeIDs) > 0, ActiveHolds: int(holds.Val())}
	for tt, cmd := range cmds {
		if len(cmd.Val()) == 0 {
			info.Seeded = false
			continue
		}
		for bucket, val := range cmd.Val() {
			n, err := strconv.Atoi(val)
			if err != nil {
				return port.StockInfo{}, fmt.Errorf("ton kho %q cua %s/%s khong phai so: %w", val, tt, bucket, err)
			}
			info.Total += max(n, 0)
		}
	}
	return info, nil
}

// SetSaleState dat hoac go hai co het ve trong meta cua phong cho, nguyen tu (MULTI).
func (g *Gate) SetSaleState(ctx context.Context, eventID string, s port.SaleState) error {
	key := waitingMetaKey(eventID)
	pipe := g.rdb.TxPipeline()
	for field, on := range map[string]bool{metaSoldOut: s.SoldOut, metaStockZero: s.StockZero} {
		if on {
			pipe.HSet(ctx, key, field, "1")
		} else {
			pipe.HDel(ctx, key, field)
		}
	}
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("ghi co het ve: %w", err)
	}
	return nil
}

var (
	_ port.StockReader   = (*Gate)(nil)
	_ port.SaleStateSink = (*Gate)(nil)
)
