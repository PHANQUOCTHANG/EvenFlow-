// Package redisadapter hien thuc kho luu hang cho tren Redis.
//
// Toan bo duong di nong cua phong cho nam trong package nay. Moi thao tac deu
// la MOT lenh Lua atomic -- khong co doc-roi-ghi, nen khong co race du hang tram
// nghin request dap vao cung mot thoi diem.
package redisadapter

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/eventflow/eventflow/libs/go/redisx"
	"github.com/eventflow/eventflow/services/waitingroom/internal/domain"
)

//go:embed script/*.lua
var scriptFS embed.FS

// Queue la kho luu hang cho.
type Queue struct {
	rdb     *redis.Client
	scripts *redisx.ScriptSet
	ttl     time.Duration
}

func NewQueue(ctx context.Context, rdb *redis.Client, sessionTTL time.Duration) (*Queue, error) {
	ss := redisx.NewScriptSet(rdb)
	if err := ss.LoadFS(scriptFS, "script"); err != nil {
		return nil, err
	}
	if err := ss.Preload(ctx); err != nil {
		return nil, err
	}
	return &Queue{rdb: rdb, scripts: ss, ttl: sessionTTL}, nil
}

// Hash tag {ev} giu moi key cua cung mot su kien tren CUNG MOT shard, nen script
// Lua chay duoc tren Redis Cluster va mot su kien lon khong lam phien su kien khac.
func k(eventID, suffix string) string { return fmt.Sprintf("wr:{%s}:%s", eventID, suffix) }

func (q *Queue) keys(eventID string) []string {
	return []string{
		k(eventID, "tok"), k(eventID, "lobby"), k(eventID, "queue"),
		k(eventID, "seq"), k(eventID, "meta"), k(eventID, "sess"),
	}
}

// JoinResult la ket qua ghi danh.
type JoinResult struct {
	State domain.State
	Token string
	Rank  int64
	IsNew bool
}

// Join ghi danh mot identity vao phong cho. Idempotent theo BR-Q3.
func (q *Queue) Join(ctx context.Context, eventID, identityID, newToken string) (JoinResult, error) {
	res, err := q.scripts.Run(ctx, "join", q.keys(eventID),
		identityID, newToken, time.Now().UnixMilli(), int(q.ttl.Seconds()),
	).Slice()
	if err != nil {
		return JoinResult{}, fmt.Errorf("join queue: %w", err)
	}
	if len(res) != 4 {
		return JoinResult{}, errors.New("join: script tra ve so truong khong dung")
	}
	return JoinResult{
		State: domain.State(toString(res[0])),
		Token: toString(res[1]),
		Rank:  toInt64(res[2]),
		IsNew: toInt64(res[3]) == 1,
	}, nil
}

// Status doc trang thai trong mot round-trip duy nhat.
func (q *Queue) Status(ctx context.Context, eventID, token string) (domain.Position, error) {
	keys := []string{
		k(eventID, "tok"), k(eventID, "queue"), k(eventID, "admitted"),
		k(eventID, "lobby"), k(eventID, "meta"),
	}
	res, err := q.scripts.Run(ctx, "status", keys, token, time.Now().UnixMilli()).Slice()
	if err != nil {
		return domain.Position{}, fmt.Errorf("queue status: %w", err)
	}
	if len(res) != 6 {
		return domain.Position{}, errors.New("status: script tra ve so truong khong dung")
	}
	return domain.Build(
		domain.State(toString(res[0])),
		toInt64(res[1]),
		toInt64(res[2]),
		float64(toInt64(res[3]))/1000.0, // admit_rate luu dang phan nghin de giu do chinh xac
		toInt64(res[4]),
	), nil
}

// Admit tha mot lo nguoi vao mua ve. Chi admit controller (da doat leader) goi.
func (q *Queue) Admit(ctx context.Context, eventID string, batch int, ttl time.Duration) ([]string, error) {
	keys := []string{k(eventID, "queue"), k(eventID, "admitted"), k(eventID, "meta")}
	res, err := q.scripts.Run(ctx, "admit", keys,
		batch, time.Now().UnixMilli(), ttl.Milliseconds()).StringSlice()
	if err != nil {
		return nil, fmt.Errorf("admit batch: %w", err)
	}
	return res, nil
}

// Shuffle chay lottery tai T0. An toan khi goi lai (lan sau la no-op).
func (q *Queue) Shuffle(ctx context.Context, eventID string, seed int64) (int64, error) {
	keys := []string{k(eventID, "lobby"), k(eventID, "queue"), k(eventID, "meta")}
	n, err := q.scripts.Run(ctx, "shuffle", keys, seed).Int64()
	if err != nil {
		return 0, fmt.Errorf("shuffle lobby: %w", err)
	}
	return n, nil
}

// SetAdmitRate ghi lai nhip admit hien tai de status.lua doc ra tinh ETA.
func (q *Queue) SetAdmitRate(ctx context.Context, eventID string, ratePerSec float64) error {
	return q.rdb.HSet(ctx, k(eventID, "meta"), "admit_rate", int64(ratePerSec*1000)).Err()
}

func (q *Queue) QueueDepth(ctx context.Context, eventID string) (int64, error) {
	return q.rdb.ZCard(ctx, k(eventID, "queue")).Result()
}

// MarkSoldOut dung viec tha them nguoi vao checkout (BR-Q6).
func (q *Queue) MarkSoldOut(ctx context.Context, eventID string) error {
	return q.rdb.HSet(ctx, k(eventID, "meta"), "sold_out", "1").Err()
}

func toString(v any) string {
	s, _ := v.(string)
	return s
}

func toInt64(v any) int64 {
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
