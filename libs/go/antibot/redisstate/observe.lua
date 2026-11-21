-- observe.lua -- all anti-bot Redis state of one join in ONE call (contracts 4, AC-6).
--
-- Every key is built in Go (from Input.Now) and passed in KEYS, all under the hash tag {ev} (same slot as
-- wr:{ev}:), so the script routes correctly on Redis Cluster (plan review R3-1). The clock is ARGV[2]
-- (Input.Now in ms), never Redis TIME.
--
-- KEYS[1]      wr:{ev}:tok                 HASH identity -> token (HEXISTS only)
-- KEYS[2]      ab:{ev}:blk:{identity}      STRING block flag (EXISTS only; set by revoke.lua)
-- KEYS[3]      ab:{ev}:jt:{identity}       LIST join ms, newest first (only joins without a slot)
-- KEYS[4]      ab:{ev}:fp:{fphash}         ZSET identity -> ms
-- KEYS[5]      ab:{ev}:net:{prefix}        ZSET identity -> ms
-- KEYS[6]      ab:{ev}:sq:{sq}             ZSET identity -> ms
-- KEYS[7..11]  ab:{ev}:jc:{minute-4..minute}          STRING event join count per minute
-- KEYS[12..16] ab:{ev}:nj:{prefix}:{minute-4..minute} STRING prefix join count per minute
--
-- ARGV[1] identity   ARGV[2] now_ms   ARGV[3] has fp (1/0)   ARGV[4] has prefix   ARGV[5] has sq
-- ARGV[6] R4 window ms   ARGV[7] R7 window ms   ARGV[8] R4N share (fraction)   ARGV[9] R4N share min joins
-- ARGV[10] join-time window ms
--
-- Returns { hasSlot, sticky, fpCount, netCount, netSuppressed, seqCount, gap1, gap2, ... }

local id   = ARGV[1]
local now  = tonumber(ARGV[2])
local r4w  = tonumber(ARGV[6])
local r7w  = tonumber(ARGV[7])
local CAP  = 64

local hasSlot = redis.call('HEXISTS', KEYS[1], id)
if redis.call('EXISTS', KEYS[2]) == 1 then
  return { hasSlot, 1, 0, 0, 0, 0 }
end

local function distinct(key, window)
  redis.call('ZADD', key, now, id)
  redis.call('ZREMRANGEBYSCORE', key, '-inf', '(' .. (now - window))
  local n = redis.call('ZCARD', key)
  if n > CAP then
    redis.call('ZREMRANGEBYRANK', key, 0, n - CAP - 1)
    n = CAP
  end
  redis.call('PEXPIRE', key, 2 * window)
  return n
end

local out = { hasSlot, 0, 0, 0, 0, 0 }

-- R1-server: join times, only for joins made without a slot (D12).
if hasSlot == 0 then
  local jtw = tonumber(ARGV[10])
  local newest = redis.call('LINDEX', KEYS[3], 0)
  if newest and now - tonumber(newest) > jtw then
    redis.call('DEL', KEYS[3])
  end
  redis.call('LPUSH', KEYS[3], now)
  redis.call('LTRIM', KEYS[3], 0, 8)
  redis.call('PEXPIRE', KEYS[3], jtw)
  local l = redis.call('LRANGE', KEYS[3], 0, 8)
  for i = 1, #l - 1 do
    out[#out + 1] = tonumber(l[i]) - tonumber(l[i + 1])
  end
end

if ARGV[3] == '1' then
  out[3] = distinct(KEYS[4], r4w)
end

-- Event join count per minute (every join), prefix join count per minute (joins with a known IP).
redis.call('INCR', KEYS[11])
redis.call('EXPIRE', KEYS[11], 360)

if ARGV[4] == '1' then
  out[4] = distinct(KEYS[5], r4w)
  redis.call('INCR', KEYS[16])
  redis.call('EXPIRE', KEYS[16], 360)
  local ej, pj = 0, 0
  local jc = redis.call('MGET', KEYS[7], KEYS[8], KEYS[9], KEYS[10], KEYS[11])
  local nj = redis.call('MGET', KEYS[12], KEYS[13], KEYS[14], KEYS[15], KEYS[16])
  for i = 1, 5 do
    ej = ej + (tonumber(jc[i]) or 0)
    pj = pj + (tonumber(nj[i]) or 0)
  end
  -- share = prefix joins / event joins over the same 5 buckets (same unit, N-2)
  if ej >= tonumber(ARGV[9]) and pj > tonumber(ARGV[8]) * ej then
    out[5] = 1
  end
end

if ARGV[5] == '1' then
  out[6] = distinct(KEYS[6], r7w)
end

return out
