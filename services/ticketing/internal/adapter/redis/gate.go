// Package redisadapter hien thuc cong chan ton kho tren Redis.
//
// Day la lop 1 trong hai lop chong oversell. Vai tro cua no la NHANH va chan
// bot: 95% request thua bi tu choi ngay tai day, khong bao gio cham toi Postgres.
// Tinh dung dan cuoi cung van thuoc ve Postgres (xem adapter/postgres).
package redisadapter

import (
	"context"
	"embed"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/eventflow/eventflow/libs/go/redisx"
	"github.com/eventflow/eventflow/services/ticketing/internal/port"
)

//go:embed script/*.lua
var scriptFS embed.FS

type Gate struct {
	rdb     *redis.Client
	scripts *redisx.ScriptSet
}

func NewGate(ctx context.Context, rdb *redis.Client) (*Gate, error) {
	ss := redisx.NewScriptSet(rdb)
	if err := ss.LoadFS(scriptFS, "script"); err != nil {
		return nil, err
	}
	if err := ss.Preload(ctx); err != nil {
		return nil, err
	}
	return &Gate{rdb: rdb, scripts: ss}, nil
}

// Hash tag {ev} giu moi key cua mot su kien tren cung shard, de script Lua chay
// duoc tren Redis Cluster.
func invKey(eventID, ticketTypeID string) string {
	return fmt.Sprintf("inv:{%s}:tt:%s", eventID, ticketTypeID)
}
func invPrefix(eventID string) string  { return fmt.Sprintf("inv:{%s}:tt:", eventID) }
func holdZKey(eventID string) string   { return fmt.Sprintf("hold:{%s}:z", eventID) }
func holdHKey(eventID string) string   { return fmt.Sprintf("hold:{%s}:h", eventID) }
func userHoldKey(eventID string) string { return fmt.Sprintf("uhold:{%s}", eventID) }
func admittedKey(eventID string) string { return fmt.Sprintf("wr:{%s}:admitted", eventID) }

func (g *Gate) Hold(ctx context.Context, req port.HoldRequest) (port.HoldResult, error) {
	keys := []string{
		invKey(req.EventID, req.TicketTypeID),
		holdZKey(req.EventID),
		holdHKey(req.EventID),
		userHoldKey(req.EventID),
	}
	res, err := g.scripts.Run(ctx, "hold", keys,
		req.TicketTypeID, req.Quantity, req.HoldID, req.IdentityID,
		time.Now().UnixMilli(), req.TTL.Milliseconds(),
		req.BucketCount, req.StartBucket,
	).Slice()
	if err != nil {
		return port.HoldResult{}, fmt.Errorf("chay script hold: %w", err)
	}
	if len(res) == 0 {
		return port.HoldResult{}, fmt.Errorf("script hold tra ve rong")
	}

	status := port.HoldStatus(asString(res[0]))
	switch status {
	case port.HoldOK:
		if len(res) < 4 {
			return port.HoldResult{}, fmt.Errorf("script hold: thieu truong khi OK")
		}
		return port.HoldResult{
			Status:    port.HoldOK,
			Bucket:    int(asInt64(res[2])),
			ExpiresAt: time.UnixMilli(asInt64(res[3])),
		}, nil
	case port.HoldAlreadyHolding:
		var existing string
		if len(res) > 1 {
			existing = asString(res[1])
		}
		return port.HoldResult{Status: status, ExistingHold: existing}, nil
	default:
		return port.HoldResult{Status: status}, nil
	}
}

// Release tra ve vao kho. IDEMPOTENT -- script xoa hold TRUOC roi moi cong kho,
// nen lan goi thu hai khong lam gi (BR-O7).
func (g *Gate) Release(ctx context.Context, eventID, holdID string) error {
	keys := []string{
		holdHKey(eventID), holdZKey(eventID), userHoldKey(eventID), invPrefix(eventID),
	}
	return g.scripts.Run(ctx, "release", keys, holdID).Err()
}

// Consume dung khi thanh toan thanh cong: xoa hold nhung KHONG tra kho.
//
// Day la khac biet then chot voi Release. Dung nham Release o day se lam kho
// phong len dung bang so ve vua ban -- tuc la oversell.
func (g *Gate) Consume(ctx context.Context, eventID, holdID string) error {
	pipe := g.rdb.TxPipeline()
	pipe.HDel(ctx, holdHKey(eventID), holdID)
	pipe.ZRem(ctx, holdZKey(eventID), holdID)
	_, err := pipe.Exec(ctx)
	return err
}

// IsAdmitted kiem tra khach da duoc phong cho tha vao chua (BR-Q7).
//
// Day la cho tai xuong Postgres bi ghim lai: khong co token admit con han thi
// khong co request nao di tiep duoc.
func (g *Gate) IsAdmitted(ctx context.Context, eventID, queueToken string) (bool, error) {
	score, err := g.rdb.ZScore(ctx, admittedKey(eventID), queueToken).Result()
	if err == redis.Nil {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return int64(score) > time.Now().UnixMilli(), nil
}

// ExpiredHolds tra ve cac hold da qua han cho sweeper xu ly.
func (g *Gate) ExpiredHolds(ctx context.Context, eventID string, limit int) ([]string, error) {
	return g.rdb.ZRangeByScore(ctx, holdZKey(eventID), &redis.ZRangeBy{
		Min:   "-inf",
		Max:   fmt.Sprintf("%d", time.Now().UnixMilli()),
		Count: int64(limit),
	}).Result()
}

// SeedInventory nap ton kho tu Postgres vao Redis (luc khoi dong va khi doi soat).
func (g *Gate) SeedInventory(ctx context.Context, eventID, ticketTypeID string, perBucket map[int]int) error {
	vals := make(map[string]any, len(perBucket))
	for bucket, avail := range perBucket {
		vals[fmt.Sprintf("%d", bucket)] = avail
	}
	return g.rdb.HSet(ctx, invKey(eventID, ticketTypeID), vals).Err()
}

func asString(v any) string {
	s, _ := v.(string)
	return s
}

func asInt64(v any) int64 {
	switch n := v.(type) {
	case int64:
		return n
	case string:
		var out int64
		_, _ = fmt.Sscanf(n, "%d", &out)
		return out
	default:
		return 0
	}
}

var _ port.InventoryGate = (*Gate)(nil)
