-- status.lua -- Doc trang thai hang cho trong MOT round-trip.
--
-- Endpoint nay chiem ~90% luu luong trong suot 30 phut cho, nen moi round-trip
-- tiet kiem duoc deu nhan len hang tram nghin lan. Toan bo phep doc gom vao day.
--
-- KEYS[1] = wr:{ev}:tok        HASH
-- KEYS[2] = wr:{ev}:queue      ZSET
-- KEYS[3] = wr:{ev}:admitted   ZSET
-- KEYS[4] = wr:{ev}:lobby      SET
-- KEYS[5] = wr:{ev}:meta       HASH
-- ARGV[1] = token
-- ARGV[2] = now_ms
--
-- Tra ve: { state, rank, queue_depth, admit_rate, expire_at, lottery_size }
--   state: LOBBY | QUEUED | ADMITTED | EXPIRED | UNKNOWN

local token = ARGV[1]
local now   = tonumber(ARGV[2])

local depth      = redis.call('ZCARD', KEYS[2])
local admit_rate = tonumber(redis.call('HGET', KEYS[5], 'admit_rate') or '0')
local lottery    = tonumber(redis.call('HGET', KEYS[5], 'lottery_size') or '0')

-- Da duoc admit? Kiem tra truoc vi day la trang thai khach quan tam nhat.
local exp = redis.call('ZSCORE', KEYS[3], token)
if exp then
  if tonumber(exp) < now then
    redis.call('ZREM', KEYS[3], token)
    return { 'EXPIRED', -1, depth, admit_rate, 0, lottery }
  end
  return { 'ADMITTED', 0, depth, admit_rate, tonumber(exp), lottery }
end

local rank = redis.call('ZRANK', KEYS[2], token)
if rank then
  return { 'QUEUED', rank + 1, depth, admit_rate, 0, lottery }
end

if redis.call('SISMEMBER', KEYS[4], token) == 1 then
  return { 'LOBBY', -1, depth, admit_rate, 0, lottery }
end

return { 'UNKNOWN', -1, depth, admit_rate, 0, lottery }
