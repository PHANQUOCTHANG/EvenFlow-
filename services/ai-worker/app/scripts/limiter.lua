-- limiter.lua -- Token bucket TOAN CUC cho Gemini API (RPM + TPM).
--
-- Day la cau tra loi cho bai toan rate-limit: thay vi de Gemini tra 429 roi moi
-- lui, he thong TU DIEU TIET de KHONG BAO GIO cham tran. Moi replica cua
-- ai-worker dung chung mot bo dem tren Redis, nen scale worker khong lam vo han
-- muc -- dieu ma limiter cuc bo trong tung process khong the dam bao.
--
-- Kiem CA HAI chieu, vi Gemini gioi han ca hai:
--   * RPM -- so request moi phut
--   * TPM -- so token moi phut  (thuong la rang buoc that su bi cham truoc)
--
-- Worker DAT CHO token TRUOC khi goi (uoc luong prompt + max_output_tokens),
-- roi hoan lai phan chenh sau khi biet usage that (dung op = 'refund').
--
-- KEYS[1] = ai:rl:req      HASH { tokens, ts }
-- KEYS[2] = ai:rl:tok      HASH { tokens, ts }
-- ARGV[1] = op             'acquire' | 'refund'
-- ARGV[2] = now_ms
-- ARGV[3] = rpm_capacity
-- ARGV[4] = tpm_capacity
-- ARGV[5] = est_tokens     (acquire: dat cho; refund: so token tra lai)
--
-- Tra ve acquire: { 1, 0 }        -> duoc phep goi
--                 { 0, wait_ms }  -> chua du, worker nack + requeue sau wait_ms
-- Tra ve refund : { 1, 0 }

local op        = ARGV[1]
local now       = tonumber(ARGV[2])
local rpm_cap   = tonumber(ARGV[3])
local tpm_cap   = tonumber(ARGV[4])
local est       = tonumber(ARGV[5])

local function refill(key, cap)
  local t  = tonumber(redis.call('HGET', key, 'tokens') or cap)
  local ts = tonumber(redis.call('HGET', key, 'ts') or now)
  local elapsed = math.max(0, now - ts)
  -- Bo dem nap lai lien tuc theo thoi gian troi (cap / 60000 token moi ms),
  -- khong dung cua so co dinh -- tranh dinh nhon dau moi phut.
  local refilled = math.min(cap, t + elapsed * cap / 60000.0)
  return refilled, elapsed
end

if op == 'refund' then
  local t = tonumber(redis.call('HGET', KEYS[2], 'tokens') or tpm_cap)
  redis.call('HSET', KEYS[2], 'tokens', math.min(tpm_cap, t + est))
  return { 1, 0 }
end

local req_tokens = refill(KEYS[1], rpm_cap)
local tok_tokens = refill(KEYS[2], tpm_cap)

-- Thieu o bat ky chieu nao deu phai doi. Tra ve thoi gian doi CHINH XAC de
-- worker requeue dung luc, thay vi poll mu.
if req_tokens < 1 then
  local wait = math.ceil((1 - req_tokens) * 60000.0 / rpm_cap)
  return { 0, wait }
end
if tok_tokens < est then
  local wait = math.ceil((est - tok_tokens) * 60000.0 / tpm_cap)
  return { 0, wait }
end

redis.call('HSET', KEYS[1], 'tokens', req_tokens - 1,   'ts', now)
redis.call('HSET', KEYS[2], 'tokens', tok_tokens - est, 'ts', now)
redis.call('EXPIRE', KEYS[1], 300)
redis.call('EXPIRE', KEYS[2], 300)

return { 1, 0 }
