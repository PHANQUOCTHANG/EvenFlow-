-- shuffle.lua -- Lottery tai T0 (BR-Q1).
--
-- Day la co che CONG BANG cot loi cua he thong: toan bo nguoi da vao LOBBY
-- truoc gio mo ban duoc xao tron NGAU NHIEN roi moi cap so thu tu.
--
-- Hai he qua quan trong:
--   1. Vao som 2 tieng hay 2 giay deu co co hoi nhu nhau -> triet tieu dong co
--      bot bam F5 va loi the duong truyen.
--   2. Khach co the hoan viec bam "vao hang" mot khoang ngau nhien 0..5s ma
--      khong thiet gi -> dinh tai tai giay T0 duoc trai phang (Tang 2, docs/02).
--
-- Chay DUNG MOT LAN moi su kien, boi admit controller da duoc bau lam leader.
-- Co ma 'shuffled' de chay lai la no-op an toan.
--
-- KEYS[1] = wr:{ev}:lobby         SET
-- KEYS[2] = wr:{ev}:queue         ZSET
-- KEYS[3] = wr:{ev}:meta          HASH
-- KEYS[4] = wr:{ev}:leader_epoch  STRING
-- KEYS[5] = wr:{ev}:lobby_slow    SET (anti-bot slow lane truoc T0; tuy chon)
-- ARGV[1] = random seed (lay tu server, KHONG dung math.randomseed mac dinh)
-- ARGV[2] = epoch cua leader goi
-- ARGV[3] = slow lane penalty P (tuy chon, mac dinh 100000)
--
-- Slow lobby (anti-bot, D5 / P-5): xao tron rieng (Fisher-Yates) roi xep score 1e12 + P + i (i = 1..m), tuc
-- sau moi nguoi trung lottery va sau P nguoi vao sau T0. Chay ca khi lobby chinh rong, de slow lobby khong bi
-- bo roi. lottery_size chi dem lan chinh; slow_lottery_size = m (join.lua dung cho slow vao sau T0).
--
-- Tra ve: so token da duoc cap thu tu (0 neu da shuffle truoc do); loi 'FENCED' neu epoch da cu.
-- Co ma 'shuffled' nen doi leader giua chung khong bao gio lam lottery hai lan (BR-Q1).

if tonumber(redis.call('GET', KEYS[4]) or '0') ~= tonumber(ARGV[2]) then
  return redis.error_reply('FENCED')
end

if redis.call('HGET', KEYS[3], 'shuffled') == '1' then
  return 0
end

local members = redis.call('SMEMBERS', KEYS[1])
local n = #members

math.randomseed(tonumber(ARGV[1]))

-- Lobby cham (anti-bot) duoc xep trong CA HAI nhanh, ke ca khi lobby chinh rong (P-5).
local function place_slow_lobby()
  if not KEYS[5] then
    return
  end
  local penalty = tonumber(ARGV[3] or '100000')
  local slow = redis.call('SMEMBERS', KEYS[5])
  local m = #slow
  for i = m, 2, -1 do
    local j = math.random(i)
    slow[i], slow[j] = slow[j], slow[i]
  end
  local CHUNK_SLOW = 2000
  local batch = {}
  for i = 1, m do
    batch[#batch + 1] = 1000000000000 + penalty + i
    batch[#batch + 1] = slow[i]
    if #batch >= CHUNK_SLOW then
      redis.call('ZADD', KEYS[2], unpack(batch))
      batch = {}
    end
  end
  if #batch > 0 then
    redis.call('ZADD', KEYS[2], unpack(batch))
  end
  redis.call('DEL', KEYS[5])
  redis.call('HSET', KEYS[3], 'slow_lottery_size', m)
end

if n == 0 then
  place_slow_lobby()
  redis.call('HSET', KEYS[3], 'shuffled', '1')
  return 0
end

-- Fisher-Yates: moi hoan vi co xac suat bang nhau.
for i = n, 2, -1 do
  local j = math.random(i)
  members[i], members[j] = members[j], members[i]
end

-- Cap score 1..n. Nguoi vao sau T0 dung offset 1e12 (join.lua) nen luon
-- xep sau toan bo nhom lottery (BR-Q2).
local args = {}
for i = 1, n do
  args[#args + 1] = i
  args[#args + 1] = members[i]
end

-- Nap theo lo 1000 cap de tranh mot lenh qua lon.
local CHUNK = 2000
local i = 1
while i <= #args do
  local batch = {}
  for k = i, math.min(i + CHUNK - 1, #args) do
    batch[#batch + 1] = args[k]
  end
  redis.call('ZADD', KEYS[2], unpack(batch))
  i = i + CHUNK
end

redis.call('DEL', KEYS[1])
place_slow_lobby()
redis.call('HSET', KEYS[3], 'shuffled', '1')
redis.call('HSET', KEYS[3], 'lottery_size', n)

return n
