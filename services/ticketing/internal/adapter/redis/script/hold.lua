-- hold.lua -- Cong chan ton kho nhanh (BR-O1, lop 1/2).
--
-- VAI TRO: chan tu 95% request thua NGAY TAI REDIS, khong de chung cham xuong
-- Postgres. Day KHONG phai nguon su that -- Postgres moi la nguon su that.
--
-- He qua an toan quan trong: neu Redis lech so (failover, mat du lieu), he thong
-- KHONG THE oversell, vi Postgres con constraint CHECK (available >= 0) chan lai.
-- Redis lech chi gay "bao het ve som", va job doi soat se dong bo lai trong 10s.
--
-- KEYS[1] = inv:{ev}:tt:<ticket_type_id>   HASH  bucket -> available
-- KEYS[2] = hold:{ev}:z                    ZSET  hold_id -> expire_at_ms
-- KEYS[3] = hold:{ev}:h                    HASH  hold_id -> json
-- KEYS[4] = uhold:{ev}                     HASH  identity -> hold_id  (BR-O3)
--
-- ARGV[1] = ticket_type_id
-- ARGV[2] = quantity
-- ARGV[3] = hold_id
-- ARGV[4] = identity_id
-- ARGV[5] = now_ms
-- ARGV[6] = ttl_ms                (BR-O2: 600000 = 10 phut)
-- ARGV[7] = bucket_count
-- ARGV[8] = start_bucket          (client bam ngau nhien de trai contention)
--
-- Tra ve: { "OK", hold_id, bucket, expire_at }  hoac
--         { "SOLD_OUT" } | { "ALREADY_HOLDING", hold_id } | { "BAD_QTY" }

local tt        = ARGV[1]
local qty       = tonumber(ARGV[2])
local hold_id   = ARGV[3]
local identity  = ARGV[4]
local now       = tonumber(ARGV[5])
local ttl_ms    = tonumber(ARGV[6])
local nbuckets  = tonumber(ARGV[7])
local start_b   = tonumber(ARGV[8])

if qty == nil or qty <= 0 then
  return { 'BAD_QTY' }
end

-- BR-O3: mot khach chi co mot hold dang hoat dong tren moi su kien.
local prev = redis.call('HGET', KEYS[4], identity)
if prev then
  local exp = redis.call('ZSCORE', KEYS[2], prev)
  if exp and tonumber(exp) > now then
    return { 'ALREADY_HOLDING', prev }
  end
  -- Hold cu da het han; sweeper se don. O day chi go anh xa.
  redis.call('HDEL', KEYS[4], identity)
end

-- Quet vong tron toi da 3 bucket: giam contention ma van khong bao het ve som.
local chosen = nil
for i = 0, math.min(2, nbuckets - 1) do
  local b = (start_b + i) % nbuckets
  local avail = tonumber(redis.call('HGET', KEYS[1], b) or '0')
  if avail >= qty then
    chosen = b
    break
  end
end

if chosen == nil then
  return { 'SOLD_OUT' }
end

-- Giam ton va tao hold -- atomic, khong the bi chen ngang.
redis.call('HINCRBY', KEYS[1], chosen, -qty)

local expire_at = now + ttl_ms
redis.call('ZADD', KEYS[2], expire_at, hold_id)
redis.call('HSET', KEYS[3], hold_id, string.format(
  '{"tt":"%s","bucket":%d,"qty":%d,"identity":"%s","expire_at":%d}',
  tt, chosen, qty, identity, expire_at))
redis.call('HSET', KEYS[4], identity, hold_id)

return { 'OK', hold_id, chosen, expire_at }
