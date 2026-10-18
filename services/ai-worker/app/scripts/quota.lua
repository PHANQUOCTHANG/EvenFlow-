-- quota.lua -- Han muc tro ly: toi da N cau / cua so truot (BR-A2: 10 cau / 15 phut).
--
-- Mot lenh nguyen tu: nhieu replica ai-worker cung nhan cau hoi cua mot khach ma khong
-- replica nao cho qua cau thu 11. Dem phia trinh duyet (localStorage) xoa duoc bang mot
-- lenh nen khong phai la han muc.
--
-- KEYS[1] = ai:quota:{<event_id>}:<identity>   ZSET  member -> thoi diem (ms)
-- ARGV[1] = now_ms
-- ARGV[2] = window_ms
-- ARGV[3] = limit
-- ARGV[4] = member (job_id, duy nhat)
--
-- Tra ve { allowed (1|0), used, reset_at_ms }
--   reset_at_ms: luc cau cu nhat trong cua so roi ra (luc khach hoi them duoc).

local now    = tonumber(ARGV[1])
local window = tonumber(ARGV[2])
local limit  = tonumber(ARGV[3])

redis.call('ZREMRANGEBYSCORE', KEYS[1], '-inf', now - window)
local used = redis.call('ZCARD', KEYS[1])

local reset = now + window
local oldest = redis.call('ZRANGE', KEYS[1], 0, 0, 'WITHSCORES')
if oldest[2] then
  reset = tonumber(oldest[2]) + window
end

if used >= limit then
  return { 0, used, reset }
end

redis.call('ZADD', KEYS[1], now, ARGV[4])
redis.call('PEXPIRE', KEYS[1], window)
return { 1, used + 1, reset }
