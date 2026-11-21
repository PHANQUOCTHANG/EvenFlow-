-- revoke.lua -- Anti-bot BLOCK (D6): go bo cho cua mot identity khoi MOI cau truc va dat co chan 24 h,
-- trong CUNG mot script (atomic, P-12). join.lua (duong lane) tu choi cap cho moi khi co ton tai.
--
-- KEYS[1] = wr:{ev}:tok         HASH identity -> token
-- KEYS[2] = wr:{ev}:lobby       SET
-- KEYS[3] = wr:{ev}:lobby_slow  SET
-- KEYS[4] = wr:{ev}:queue       ZSET
-- KEYS[5] = wr:{ev}:admitted    ZSET
-- KEYS[6] = wr:{ev}:sess        HASH token -> json
-- KEYS[7] = ab:{ev}:blk:{id}    STRING co chan
-- ARGV[1] = identity_id
-- ARGV[2] = TTL co chan (giay), mac dinh 86400
--
-- Tra ve: 1 neu da go mot cho, 0 neu identity chua co cho (co van duoc dat).

local identity = ARGV[1]
local ttl = tonumber(ARGV[2] or '86400')

local removed = 0
local token = redis.call('HGET', KEYS[1], identity)
if token then
  redis.call('SREM', KEYS[2], token)
  redis.call('SREM', KEYS[3], token)
  redis.call('ZREM', KEYS[4], token)
  redis.call('ZREM', KEYS[5], token)
  redis.call('HDEL', KEYS[1], identity)
  redis.call('HDEL', KEYS[6], token)
  removed = 1
end

-- NX (R-2): the 24 h window starts at the first BLOCK; a STICKY retry never extends it.
redis.call('SET', KEYS[7], '1', 'EX', ttl, 'NX')
return removed
