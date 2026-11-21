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
-- KEYS[7] = wr:{ev}:lobby_slow SET  token  (anti-bot slow lane truoc T0; tuy chon)
-- KEYS[8] = ab:{ev}:blk:{id}  STR   co chan 24 h cua anti-bot (tuy chon; chi doc tren duong lane)
--
-- ARGV[1] = identity_id
-- ARGV[2] = token moi (goi tao san, chi dung khi chua ton tai)
-- ARGV[3] = now_ms
-- ARGV[4] = session_ttl_seconds
-- ARGV[5] = lane 'n' | 's' (tuy chon). Thieu = duong develop: lane n, KHONG kiem tra co chan.
-- ARGV[6] = slow lane penalty P (tuy chon, mac dinh 100000)
--
-- Tra ve: { state, token, rank, is_new }
--   state: "LOBBY" | "QUEUED" | "ADMITTED" | "SOLD_OUT" (chi khi khach moi vao luc het ve han)
--          | "BLOCKED" (chi duong lane: co chan ton tai va chua co cho; khong cap token)
--   rank : -1 neu con trong LOBBY (chua co thu tu)

local identity = ARGV[1]
local newtok   = ARGV[2]
local now      = tonumber(ARGV[3])
local ttl      = tonumber(ARGV[4])
local lane     = ARGV[5]
local penalty  = tonumber(ARGV[6] or '100000')
local slowKey  = KEYS[7]

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
  if slowKey and redis.call('SISMEMBER', slowKey, existing) == 1 then
    return { 'LOBBY', existing, -1, 0 }
  end
  -- Khong o lobby cung khong o queue -> da duoc admit (hoac het han).
  return { 'ADMITTED', existing, 0, 0 }
end

-- 1a. Anti-bot (D6): tren duong lane, co chan 24 h (revoke.lua dat) thi khong cap cho moi.
-- Cho da co o tren van duoc tra truoc (BR-Q3, D12).
if lane and KEYS[8] and redis.call('EXISTS', KEYS[8]) == 1 then
  return { 'BLOCKED', '', -1, 0 }
end

-- 1b. Het ve han (EV-146): khong ghi danh them ai vao hang vo vong. Nguoi da co cho o tren van duoc tra
-- lai trang thai cu; nguoi moi nhan SOLD_OUT, khong co token.
if redis.call('HGET', KEYS[5], 'sold_out') == '1' then
  return { 'SOLD_OUT', '', -1, 0 }
end

-- 2. Chua co -> cap token moi.
local sale_start = tonumber(redis.call('HGET', KEYS[5], 'sale_start_ms') or '0')
local shuffled   = redis.call('HGET', KEYS[5], 'shuffled')

redis.call('HSET', KEYS[1], identity, newtok)
redis.call('EXPIRE', KEYS[1], ttl)

local state, rank

if now < sale_start and shuffled ~= '1' then
  -- Truoc T0: vao LOBBY, KHONG cap thu tu. Vao som khong co loi the (BR-Q1).
  -- Slow lane vao lobby rieng; lottery xep ca lobby nay sau nhom chinh (shuffle.lua).
  local lobby = KEYS[2]
  if lane == 's' and slowKey then
    lobby = slowKey
  end
  redis.call('SADD', lobby, newtok)
  redis.call('EXPIRE', lobby, ttl)
  state, rank = 'LOBBY', -1
else
  -- Sau T0: noi duoi FIFO. Offset 1e12 dam bao luon dung sau nhom lottery.
  local seq = redis.call('INCR', KEYS[4])
  local score = 1000000000000 + seq
  if lane == 's' then
    -- Slow lane sau T0: 1e12 + seq + P + m, m = so nguoi slow lobby da xep o shuffle (0 truoc shuffle),
    -- de moi nguoi slow lobby truoc T0 luon dung truoc nguoi slow vao sau T0 (P-13).
    score = score + penalty + tonumber(redis.call('HGET', KEYS[5], 'slow_lottery_size') or '0')
  end
  redis.call('ZADD', KEYS[3], score, newtok)
  redis.call('EXPIRE', KEYS[3], ttl)
  state = 'QUEUED'
  rank = redis.call('ZRANK', KEYS[3], newtok) + 1
end

redis.call('HSET', KEYS[6], newtok,
  string.format('{"identity":"%s","state":"%s","joined_at":%d}', identity, state, now))
redis.call('EXPIRE', KEYS[6], ttl)

return { state, newtok, rank, 1 }
