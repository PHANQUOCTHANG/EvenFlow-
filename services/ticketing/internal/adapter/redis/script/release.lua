-- release.lua -- Nha hold va tra ve vao kho (BR-O7).
--
-- IDEMPOTENT la yeu cau song con: neu mot hold bi release HAI lan, kho se phong
-- len va he thong se oversell. Toan bo an toan nam o mot dieu kien duy nhat --
-- hold phai con ton tai trong HASH. Xoa truoc, cong kho sau.
--
-- Dung cho ca 3 tinh huong: het han (sweeper), khach huy, thanh toan that bai.
-- Khi don thanh toan THANH CONG thi dung consume.lua (khong tra kho).
--
-- KEYS[1] = hold:{ev}:h         HASH
-- KEYS[2] = hold:{ev}:z         ZSET
-- KEYS[3] = uhold:{ev}          HASH
-- KEYS[4] = inv:{ev}:tt:        prefix -- script tu noi ticket_type_id vao
-- ARGV[1] = hold_id
--
-- Tra ve: { "RELEASED", tt, bucket, qty } | { "NOOP" }

local hold_id = ARGV[1]

local raw = redis.call('HGET', KEYS[1], hold_id)
if not raw then
  -- Da duoc release (hoac da consume) truoc do. Khong lam gi them.
  return { 'NOOP' }
end

-- Xoa TRUOC: bien viec release thanh thao tac chi thanh cong dung mot lan.
redis.call('HDEL', KEYS[1], hold_id)
redis.call('ZREM', KEYS[2], hold_id)

local tt       = string.match(raw, '"tt":"([^"]+)"')
local bucket   = tonumber(string.match(raw, '"bucket":(%d+)'))
local qty      = tonumber(string.match(raw, '"qty":(%d+)'))
local identity = string.match(raw, '"identity":"([^"]+)"')

if identity then
  -- Chi go anh xa neu no van tro toi chinh hold nay.
  if redis.call('HGET', KEYS[3], identity) == hold_id then
    redis.call('HDEL', KEYS[3], identity)
  end
end

if tt and bucket and qty then
  redis.call('HINCRBY', KEYS[4] .. tt, bucket, qty)
  return { 'RELEASED', tt, bucket, qty }
end

return { 'NOOP' }
