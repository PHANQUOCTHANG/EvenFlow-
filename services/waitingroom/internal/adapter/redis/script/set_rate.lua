-- set_rate.lua -- Ghi nhip admit hien tai (EV-142).
--
-- status.lua doc admit_rate de tinh ETA, va leader moi doc lai de TIEP TUC tu nhip cu thay vi bat
-- dau lai tu san. Vi vay chi leader hien tai duoc ghi: leader cu bi tu choi bang fencing, neu
-- khong no se de len nhip cua leader moi.
--
-- KEYS[1] = wr:{ev}:meta          HASH
-- KEYS[2] = wr:{ev}:leader_epoch  STRING
-- ARGV[1] = epoch cua leader goi
-- ARGV[2] = admit_rate * 1000 (so nguyen, de giu do chinh xac)
--
-- Tra ve: 1; loi 'FENCED' neu epoch da cu.

if tonumber(redis.call('GET', KEYS[2]) or '0') ~= tonumber(ARGV[1]) then
  return redis.error_reply('FENCED')
end

redis.call('HSET', KEYS[1], 'admit_rate', ARGV[2])
return 1
