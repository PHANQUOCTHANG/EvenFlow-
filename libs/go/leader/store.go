package leader

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

// Moi thao tac la mot script nguyen tu: doc-roi-ghi tren hai key (khoa va epoch) khong the tach.

// KEYS[1] = lock, KEYS[2] = epoch; ARGV[1] = id, ARGV[2] = ttl_ms.
// Tra ve epoch moi, hoac 0 neu khoa dang do nguoi khac giu.
//
// Epoch = max(epoch cu + 1, thoi gian Redis tinh bang ms): luon tang, KE CA KHI key epoch bi xoa
// (flush, xoa tay, script don dep). Dem tron 1, 2, 3... se quay ve 1 khi mat key, va mot leader
// cu mang epoch 1 lai duoc chap nhan -- fencing mat tac dung dung luc can nhat.
var acquireScript = redis.NewScript(`
local cur = redis.call('GET', KEYS[1])
if cur and cur ~= ARGV[1] then return 0 end

local prev = tonumber(redis.call('GET', KEYS[2]) or '0')
if cur == ARGV[1] and prev > 0 then
  -- Da la cua minh (vd lan thu lai sau loi mang): gia han, giu nguyen epoch.
  redis.call('PEXPIRE', KEYS[1], ARGV[2])
  return prev
end

local t = redis.call('TIME')
local now_ms = t[1] * 1000 + math.floor(t[2] / 1000)
local nxt = prev + 1
if now_ms > nxt then nxt = now_ms end

redis.call('SET', KEYS[1], ARGV[1], 'PX', ARGV[2])
redis.call('SET', KEYS[2], nxt)
return nxt
`)

// KEYS[1] = lock; ARGV[1] = id, ARGV[2] = ttl_ms. Chi chu khoa moi gia han duoc.
var renewScript = redis.NewScript(`
if redis.call('GET', KEYS[1]) == ARGV[1] then
  redis.call('PEXPIRE', KEYS[1], ARGV[2])
  return 1
end
return 0
`)

// KEYS[1] = lock; ARGV[1] = id. Chi chu khoa moi nha duoc: khong xoa nham khoa cua nguoi moi len.
var releaseScript = redis.NewScript(`
if redis.call('GET', KEYS[1]) == ARGV[1] then
  return redis.call('DEL', KEYS[1])
end
return 0
`)

type redisStore struct {
	rdb   redis.Scripter
	lock  string
	epoch string
}

func (s *redisStore) acquire(ctx context.Context, id string, ttl time.Duration) (int64, error) {
	return acquireScript.Run(ctx, s.rdb, []string{s.lock, s.epoch}, id, ttl.Milliseconds()).Int64()
}

func (s *redisStore) renew(ctx context.Context, id string, ttl time.Duration) (bool, error) {
	n, err := renewScript.Run(ctx, s.rdb, []string{s.lock}, id, ttl.Milliseconds()).Int64()
	return n == 1, err
}

func (s *redisStore) release(ctx context.Context, id string) error {
	return releaseScript.Run(ctx, s.rdb, []string{s.lock}, id).Err()
}
