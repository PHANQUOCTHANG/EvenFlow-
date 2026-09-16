-- join.lua -- Ghi danh vao phong cho. O(1), mot round-trip, atomic.
--
-- Bat bien (BR-Q3): mot identity chi co DUNG MOT cho trong hang.
-- Goi lai tu tab/thiet bi khac phai tra ve DUNG token va rank cu.
--
-- Truoc T0 nguoi dung vao LOBBY (khong co so thu tu) de lottery tai T0 dam bao
-- cong bang (BR-Q1). Sau T0 nguoi dung duoc noi duoi FIFO (BR-Q2).
--
-- KEYS[1] = wr:{ev}:tok       HASH  identity_id -> token
-- KEYS[2] = wr:{ev}:lobby     SET   token  (truoc T0, khong thu tu)
-- KEYS[3] = wr:{ev}:queue     ZSET  token -> score (sau T0, FIFO)
-- KEYS[4] = wr:{ev}:seq       STR   bo dem tang dan
-- KEYS[5] = wr:{ev}:meta      HASH  sale_start_ms, shuffled
-- KEYS[6] = wr:{ev}:sess      HASH  token -> json(identity, state, joined_at)
--
-- ARGV[1] = identity_id
-- ARGV[2] = token moi (goi tao san, chi dung khi chua ton tai)
-- ARGV[3] = now_ms
-- ARGV[4] = session_ttl_seconds
--
-- Tra ve: { state, token, rank, is_new }
--   state: "LOBBY" | "QUEUED" | "ADMITTED"
--   rank : -1 neu con trong LOBBY (chua co thu tu)

local identity = ARGV[1]
local newtok   = ARGV[2]
local now      = tonumber(ARGV[3])
local ttl      = tonumber(ARGV[4])

-- 1. Idempotent: da co token thi tra lai nguyen trang thai cu.
local existing = redis.call('HGET', KEYS[1], identity)
if existing then
  local rank = redis.call('ZRANK', KEYS[3], existing)
  if rank then
    return { 'QUEUED', existing, rank + 1, 0 }
  end
  if redis.call('SISMEMBER', KEYS[2], existing) == 1 then
    return { 'LOBBY', existing, -1, 0 }
  end
  -- Khong o lobby cung khong o queue -> da duoc admit (hoac het han).
  return { 'ADMITTED', existing, 0, 0 }
end

-- 2. Chua co -> cap token moi.
local sale_start = tonumber(redis.call('HGET', KEYS[5], 'sale_start_ms') or '0')
local shuffled   = redis.call('HGET', KEYS[5], 'shuffled')

redis.call('HSET', KEYS[1], identity, newtok)
redis.call('EXPIRE', KEYS[1], ttl)

local state, rank

if now < sale_start and shuffled ~= '1' then
  -- Truoc T0: vao LOBBY, KHONG cap thu tu. Vao som khong co loi the (BR-Q1).
  redis.call('SADD', KEYS[2], newtok)
  redis.call('EXPIRE', KEYS[2], ttl)
  state, rank = 'LOBBY', -1
else
  -- Sau T0: noi duoi FIFO. Offset 1e12 dam bao luon dung sau nhom lottery.
  local seq = redis.call('INCR', KEYS[4])
  redis.call('ZADD', KEYS[3], 1000000000000 + seq, newtok)
  redis.call('EXPIRE', KEYS[3], ttl)
  state = 'QUEUED'
  rank = redis.call('ZRANK', KEYS[3], newtok) + 1
end

redis.call('HSET', KEYS[6], newtok,
  string.format('{"identity":"%s","state":"%s","joined_at":%d}', identity, state, now))
redis.call('EXPIRE', KEYS[6], ttl)

return { state, newtok, rank, 1 }
