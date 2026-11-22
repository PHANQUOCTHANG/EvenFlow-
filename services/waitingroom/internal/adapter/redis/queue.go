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

	"github.com/eventflow/eventflow/libs/go/leader"
	"github.com/eventflow/eventflow/libs/go/redisx"
	"github.com/eventflow/eventflow/services/waitingroom/internal/domain"
)

//go:embed script/*.lua
var scriptFS embed.FS

// Queue la kho luu hang cho. Cac thao tac ghi cua admit controller di qua Fenced(epoch).
type Queue struct {
	rdb     *redis.Client
	scripts *redisx.ScriptSet
	ttl     time.Duration
	penalty int64 // ANTIBOT_SLOW_LANE_PENALTY cho shuffle.lua; 0 = mac dinh cua script (100000)
}

// WithSlowLanePenalty dat P ma shuffle.lua dung khi xep slow lobby (controller doc ANTIBOT_SLOW_LANE_PENALTY).
func (q *Queue) WithSlowLanePenalty(p int64) *Queue {
	q.penalty = p
	return q
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

// blockKey la co chan 24 h cua anti-bot (D6); cung hash tag {ev} nen cung shard voi wr:{ev}:.
func blockKey(eventID, identityID string) string {
	return fmt.Sprintf("ab:{%s}:blk:%s", eventID, identityID)
}

func (q *Queue) keys(eventID, identityID string) []string {
	return []string{
		k(eventID, "tok"), k(eventID, "lobby"), k(eventID, "queue"),
		k(eventID, "seq"), k(eventID, "meta"), k(eventID, "sess"),
		k(eventID, "lobby_slow"), blockKey(eventID, identityID),
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
// Duong develop: lane binh thuong, khong kiem tra co chan cua anti-bot.
func (q *Queue) Join(ctx context.Context, eventID, identityID, newToken string) (JoinResult, error) {
	return q.join(ctx, eventID, identityID,
		identityID, newToken, time.Now().UnixMilli(), int(q.ttl.Seconds()))
}

// JoinLane ghi danh vao mot lane cua anti-bot (contracts 2, 4). Cho da co duoc tra lai nguyen trang (BR-Q3, D12);
// chua co cho ma co chan 24 h ton tai thi tra State BLOCKED, khong cap token (D6). Slow lane: truoc T0 vao
// lobby_slow (LOBBY), sau T0 score 1e12 + seq + penalty + m.
func (q *Queue) JoinLane(ctx context.Context, eventID, identityID, newToken string, lane domain.Lane, penalty int64) (JoinResult, error) {
	if lane != domain.LaneSlow {
		lane = domain.LaneNormal
	}
	if penalty <= 0 {
		penalty = 100000
	}
	return q.join(ctx, eventID, identityID,
		identityID, newToken, time.Now().UnixMilli(), int(q.ttl.Seconds()), string(lane), penalty)
}

func (q *Queue) join(ctx context.Context, eventID, identityID string, args ...any) (JoinResult, error) {
	res, err := q.scripts.Run(ctx, "join", q.keys(eventID, identityID), args...).Slice()
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

// Revoke go bo cho cua identity khoi lobby, lobby_slow, queue, admitted, tok va sess, va dat co chan 24 h, trong
// mot script (D6). true neu da go mot cho; co van duoc dat ca khi identity chua co cho.
func (q *Queue) Revoke(ctx context.Context, eventID, identityID string) (bool, error) {
	keys := []string{
		k(eventID, "tok"), k(eventID, "lobby"), k(eventID, "lobby_slow"), k(eventID, "queue"),
		k(eventID, "admitted"), k(eventID, "sess"), blockKey(eventID, identityID),
	}
	n, err := q.scripts.Run(ctx, "revoke", keys, identityID, 86400).Int64()
	if err != nil {
		return false, fmt.Errorf("revoke slot: %w", err)
	}
	return n == 1, nil
}

// Status doc trang thai trong mot round-trip duy nhat.
func (q *Queue) Status(ctx context.Context, eventID, token string) (domain.Position, error) {
	keys := []string{
		k(eventID, "tok"), k(eventID, "queue"), k(eventID, "admitted"),
		k(eventID, "lobby"), k(eventID, "meta"), k(eventID, "lobby_slow"),
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

// Fenced tra ve cac thao tac GHI cua admit controller, gan voi epoch cua nhiem ky leader hien tai.
// Moi lenh ghi mang epoch vao script va bi script tu choi (domain.ErrFenced) neu epoch da cu --
// nen leader cu "dung hinh" roi tinh lai khong the tha them nguoi hay de len nhip cua leader moi.
func (q *Queue) Fenced(epoch int64) *FencedQueue { return &FencedQueue{q: q, epoch: epoch} }

// FencedQueue la Queue nhin tu mot nhiem ky leader. Hien thuc app.Admitter.
type FencedQueue struct {
	q     *Queue
	epoch int64
}

func epochKey(eventID string) string { return k(eventID, "leader_epoch") }

// fenceErr doi loi FENCED cua script thanh domain.ErrFenced de tang app khong phu thuoc Redis.
func fenceErr(op string, err error) error {
	if err == nil {
		return nil
	}
	if leader.IsFenced(err) {
		return fmt.Errorf("%s: %w", op, domain.ErrFenced)
	}
	return fmt.Errorf("%s: %w", op, err)
}

// Admit tha mot lo nguoi vao mua ve. Chi leader (epoch hien tai) thanh cong.
func (f *FencedQueue) Admit(ctx context.Context, eventID string, batch int, ttl time.Duration) ([]string, error) {
	keys := []string{k(eventID, "queue"), k(eventID, "admitted"), k(eventID, "meta"), epochKey(eventID)}
	res, err := f.q.scripts.Run(ctx, "admit", keys,
		batch, time.Now().UnixMilli(), ttl.Milliseconds(), f.epoch).StringSlice()
	if err != nil {
		return nil, fenceErr("admit batch", err)
	}
	return res, nil
}

// Shuffle chay lottery tai T0. An toan khi goi lai (lan sau la no-op) va khi doi leader.
func (f *FencedQueue) Shuffle(ctx context.Context, eventID string, seed int64) (int64, error) {
	keys := []string{k(eventID, "lobby"), k(eventID, "queue"), k(eventID, "meta"), epochKey(eventID), k(eventID, "lobby_slow")}
	args := []any{seed, f.epoch}
	if f.q.penalty > 0 {
		args = append(args, f.q.penalty)
	}
	n, err := f.q.scripts.Run(ctx, "shuffle", keys, args...).Int64()
	if err != nil {
		return 0, fenceErr("shuffle lobby", err)
	}
	return n, nil
}

// SetAdmitRate ghi nhip admit hien tai de status.lua tinh ETA va de leader ke tiep tiep tuc tu do.
func (f *FencedQueue) SetAdmitRate(ctx context.Context, eventID string, ratePerSec float64) error {
	keys := []string{k(eventID, "meta"), epochKey(eventID)}
	err := f.q.scripts.Run(ctx, "set_rate", keys, f.epoch, int64(ratePerSec*1000)).Err()
	return fenceErr("set admit rate", err)
}

func (f *FencedQueue) QueueDepth(ctx context.Context, eventID string) (int64, error) {
	return f.q.QueueDepth(ctx, eventID)
}

// AdmitRate doc nhip admit leader truoc da ghi, de leader moi TIEP TUC thay vi bat dau lai tu san
// (khong thi doi leader giua cao diem keo nhip tu hang tram ve 5 va mat ~50 giay de leo lai).
func (f *FencedQueue) AdmitRate(ctx context.Context, eventID string) (rate float64, ok bool, err error) {
	v, err := f.q.rdb.HGet(ctx, k(eventID, "meta"), "admit_rate").Int64()
	if errors.Is(err, redis.Nil) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("doc admit_rate: %w", err)
	}
	return float64(v) / 1000.0, true, nil
}

// RateOverride doc lenh ghi de cua Ops. Nam trong Redis (khong phai trong RAM controller) de
// "tam dung khan cap" song sot qua doi leader va restart: leader moi tu mo van lai la loi an toan.
func (f *FencedQueue) RateOverride(ctx context.Context, eventID string) (rate float64, active bool, err error) {
	vals, err := f.q.rdb.HMGet(ctx, k(eventID, "meta"), "override", "override_rate").Result()
	if err != nil {
		return 0, false, fmt.Errorf("doc override: %w", err)
	}
	if toString(vals[0]) != "1" {
		return 0, false, nil
	}
	return float64(toInt64(vals[1])) / 1000.0, true, nil
}

// SetRateOverride dat lenh ghi de cua Ops (EV-147 se goi tu API). Khong fencing: Ops khong phai leader.
func (q *Queue) SetRateOverride(ctx context.Context, eventID string, ratePerSec float64) error {
	return q.rdb.HSet(ctx, k(eventID, "meta"),
		"override", "1", "override_rate", int64(ratePerSec*1000)).Err()
}

// ClearRateOverride bo ghi de, tra quyen lai cho AIMD.
func (q *Queue) ClearRateOverride(ctx context.Context, eventID string) error {
	return q.rdb.HDel(ctx, k(eventID, "meta"), "override", "override_rate").Err()
}

func (q *Queue) QueueDepth(ctx context.Context, eventID string) (int64, error) {
	return q.rdb.ZCard(ctx, k(eventID, "queue")).Result()
}

// SaleStart doc thoi diem mo ban (T0) cua su kien tu meta. ok=false khi chua co ai
// ghi -- caller phai cho chu KHONG duoc coi la "da qua T0": join.lua doc thieu
// sale_start_ms la 0 va se bo qua LOBBY.
func (q *Queue) SaleStart(ctx context.Context, eventID string) (t time.Time, ok bool, err error) {
	v, err := q.rdb.HGet(ctx, k(eventID, "meta"), "sale_start_ms").Int64()
	if errors.Is(err, redis.Nil) {
		return time.Time{}, false, nil
	}
	if err != nil {
		return time.Time{}, false, fmt.Errorf("doc sale_start_ms: %w", err)
	}
	return time.UnixMilli(v), true, nil
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
