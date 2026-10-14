-- reconcile.lua -- Cong delta vao mot bucket ton kho, khong de am (EV-132).
--
-- Co y dung DELTA chu khong phai SET: trong luc job doi soat tinh toan, mot hold co the vua
-- tru kho. SET se de len phep tru do (kho phong len); HINCRBY thi giao hoan voi no.
--
-- KEYS[1] = inv:{ev}:tt:<ticket_type_id>   HASH  bucket -> available
-- ARGV[1] = bucket
-- ARGV[2] = delta (am hoac duong)
--
-- Tra ve: ton kho moi cua bucket.

local bucket = ARGV[1]
local delta  = tonumber(ARGV[2])
local cur    = tonumber(redis.call('HGET', KEYS[1], bucket) or '0')

local nxt = cur + delta
if nxt < 0 then
  nxt = 0
end

redis.call('HSET', KEYS[1], bucket, nxt)
return nxt
