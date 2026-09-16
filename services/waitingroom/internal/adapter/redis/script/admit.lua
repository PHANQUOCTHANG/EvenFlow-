-- admit.lua -- Cho mot lo nguoi vao mua ve (BR-Q5, BR-Q7).
--
-- Day la van dieu tiet tai: so nguoi duoc tha vao moi giay do admit controller
-- quyet dinh dua tren thong luong checkout THUC DO. Nho vay tai xuong Postgres
-- bi ghim o muc hang so, bat ke co 30 nghin hay 3 trieu nguoi dang xep hang.
--
-- ZPOPMIN dam bao atomic: hai instance controller chay song song (vd luc doi
-- leader) cung khong the admit trung mot token.
--
-- KEYS[1] = wr:{ev}:queue      ZSET
-- KEYS[2] = wr:{ev}:admitted   ZSET  token -> expire_at_ms
-- KEYS[3] = wr:{ev}:meta       HASH
-- ARGV[1] = batch size
-- ARGV[2] = now_ms
-- ARGV[3] = admit_ttl_ms (mac dinh 15 phut)
--
-- Tra ve: danh sach token vua duoc admit

local batch   = tonumber(ARGV[1])
local now     = tonumber(ARGV[2])
local ttl_ms  = tonumber(ARGV[3])

if batch <= 0 then return {} end

-- Khong tha them ai khi da het ve (BR-Q6).
if redis.call('HGET', KEYS[3], 'sold_out') == '1' then
  return {}
end

local popped = redis.call('ZPOPMIN', KEYS[1], batch)
if #popped == 0 then return {} end

local tokens = {}
local zargs  = {}
local expire_at = now + ttl_ms

-- ZPOPMIN tra ve phang: { member, score, member, score, ... }
for i = 1, #popped, 2 do
  local tok = popped[i]
  tokens[#tokens + 1] = tok
  zargs[#zargs + 1] = expire_at
  zargs[#zargs + 1] = tok
end

redis.call('ZADD', KEYS[2], unpack(zargs))
redis.call('HINCRBY', KEYS[3], 'admitted_total', #tokens)
redis.call('HSET', KEYS[3], 'last_admit_ms', now)

return tokens
