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
-- KEYS[1] = wr:{ev}:lobby   SET
-- KEYS[2] = wr:{ev}:queue   ZSET
-- KEYS[3] = wr:{ev}:meta    HASH
-- ARGV[1] = random seed (lay tu server, KHONG dung math.randomseed mac dinh)
--
-- Tra ve: so token da duoc cap thu tu (0 neu da shuffle truoc do)

if redis.call('HGET', KEYS[3], 'shuffled') == '1' then
  return 0
end

local members = redis.call('SMEMBERS', KEYS[1])
local n = #members
if n == 0 then
  redis.call('HSET', KEYS[3], 'shuffled', '1')
  return 0
end

math.randomseed(tonumber(ARGV[1]))

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
redis.call('HSET', KEYS[3], 'shuffled', '1')
redis.call('HSET', KEYS[3], 'lottery_size', n)

return n
