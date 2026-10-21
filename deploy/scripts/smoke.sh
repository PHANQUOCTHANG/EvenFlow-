#!/usr/bin/env bash
# smoke.sh -- chay tron mot luong mua ve tren moi truong local.
#
#   xep hang -> duoc admit -> giu ghe -> (het han) -> kho duoc tra
#
# Can `make up && make migrate` truoc. Tu tao su kien rieng (quota 100, giu ghe 5s)
# bang seed.sh nen khong dung vao su kien demo.
#
# Kiem ca hai tang ton kho: Postgres (nguon su that) va Redis (cong chan nhanh).
set -uo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
COMPOSE=(docker compose -f "$ROOT/deploy/compose/docker-compose.yml")

WAITINGROOM="${WAITINGROOM_URL:-http://localhost:8081}"
TICKETING="${TICKETING_URL:-http://localhost:8082}"
# Phai nam trong ACTIVE_EVENT_IDS cua worker + controller (mac dinh co san).
EVENT_ID="${SMOKE_EVENT_ID:-22222222-2222-4222-8222-222222222222}"
BUYER="00000000-0000-4000-8000-000000000001"
# Smoke goi THANG waitingroom/ticketing (khong qua gateway) nen tu dong vai gateway: gan cac header
# danh tinh ma gateway lay tu JWT. Thieu X-Identity-Verified thi join bi 403 (BR-Q3, EV-111); queue token
# gan voi thiet bi nen moi lan goi phai cung X-Device-Id (EV-137).
IDHDR=(-H "X-Identity-Id: $BUYER" -H "X-Identity-Role: buyer" -H "X-Identity-Verified: true" -H "X-Device-Id: 00000000-0000-4000-8000-0000000000d1")
QUOTA=100
TTL=5

PASS=0
step() { printf '\n[smoke] %s\n' "$*"; }
ok()   { PASS=$((PASS + 1)); printf '  ok   %s\n' "$*"; }
die()  {
  printf '  FAIL %s\n' "$*" >&2
  printf '\n[smoke] THAT BAI sau %d buoc dat.\n' "$PASS" >&2
  exit 1
}

# json_field <ten> -- doc truong tu JSON gon (Go encoder khong chen khoang trang).
json_field() {
  sed -n "s/.*\"$1\":\"\\{0,1\\}\\([^\",}]*\\)\"\\{0,1\\}.*/\\1/p" | head -n1
}

# jti_of <queue_token> -- jti nam trong payload base64url cua token (q1.<payload>.<chu ky>).
# Token duoc ky lai moi lan join (EV-137) nen chuoi khac nhau; jti la dinh danh cho trong hang.
jti_of() {
  local p; p="$(printf '%s' "$1" | cut -d. -f2 | tr '_-' '/+')"
  while (( ${#p} % 4 )); do p="${p}="; done
  printf '%s' "$p" | base64 -d 2>/dev/null | json_field jti
}

# http <method> <url> [curl args...] -> in "body\nstatus"
http() {
  local method="$1" url="$2"; shift 2
  curl -sS -m 10 -X "$method" -w '\n%{http_code}' "$@" "$url"
}
body_of()   { sed '$d'; }
status_of() { tail -n1; }

pg() { "${COMPOSE[@]}" exec -T postgres psql -U eventflow -d eventflow -t -A -q -v ON_ERROR_STOP=1 -c "$1" | tr -d '\r'; }
rd() { "${COMPOSE[@]}" exec -T redis redis-cli "$@" | tr -d '\r'; }

pg_available() { pg "SELECT COALESCE(sum(available),0) FROM ticket_inventory WHERE ticket_type_id = '$TT'"; }
redis_available() {
  rd HVALS "inv:{${EVENT_ID}}:tt:${TT}" | awk '{s+=$1} END {print s+0}'
}

pg_is()    { [[ "$(pg_available)" == "$1" ]]; }
redis_is() { [[ "$(redis_available)" == "$1" ]]; }

wait_for() { # <mo ta> <so lan> <lenh...> -- thu lai moi giay, lenh duoc danh gia MOI LAN
  local what="$1" tries="$2"; shift 2
  for _ in $(seq 1 "$tries"); do
    if "$@" >/dev/null 2>&1; then return 0; fi
    sleep 1
  done
  die "qua thoi gian cho: $what"
}

# ---------------------------------------------------------------------------
step "0. Chuan bi su kien smoke (quota=$QUOTA, giu ghe ${TTL}s)"
EVENT_ID="$EVENT_ID" QUOTA="$QUOTA" HOLD_TTL_SECONDS="$TTL" RESET=1 \
  bash "$ROOT/deploy/scripts/seed.sh" >/dev/null || die "seed.sh that bai"
TT="$(pg "SELECT md5('${EVENT_ID}:std')::uuid")"
[[ -n "$TT" ]] || die "khong tinh duoc ticket_type_id"
ok "su kien $EVENT_ID, hang ve $TT"

step "1. Readiness"
for svc in "$WAITINGROOM" "$TICKETING"; do
  wait_for "$svc/readyz" 30 curl -sf -m 3 "$svc/readyz" || true
  code="$(curl -s -o /dev/null -m 3 -w '%{http_code}' "$svc/readyz")"
  [[ "$code" == "200" ]] || die "$svc/readyz tra $code"
  ok "$svc/readyz = 200"
done

step "2. Ton kho duoc nap vao Redis (ticketing-worker/seeder)"
wait_for "ton kho Redis = $QUOTA" 30 redis_is "$QUOTA"
ok "Postgres=$(pg_available)  Redis=$(redis_available)"

step "3. Xep hang"
resp="$(http POST "$WAITINGROOM/v1/events/$EVENT_ID/queue/join" \
  -H 'Content-Type: application/json' "${IDHDR[@]}" -d '{"signals":{}}')"
[[ "$(echo "$resp" | status_of)" == "200" ]] || die "join tra $(echo "$resp" | status_of): $(echo "$resp" | body_of)"
TOKEN="$(echo "$resp" | body_of | json_field queue_token)"
[[ -n "$TOKEN" ]] || die "join khong tra queue_token"
ok "join -> token ${TOKEN:0:8}...  state=$(echo "$resp" | body_of | json_field state)"

# Goi lai phai ra dung token cu (BR-Q3)
resp2="$(http POST "$WAITINGROOM/v1/events/$EVENT_ID/queue/join" \
  -H 'Content-Type: application/json' "${IDHDR[@]}" -d '{}')"
TOKEN2="$(echo "$resp2" | body_of | json_field queue_token)"
[[ -n "$(jti_of "$TOKEN")" && "$(jti_of "$TOKEN2")" == "$(jti_of "$TOKEN")" ]] \
  || die "join lan 2 phai giu nguyen cho trong hang (cung jti, BR-Q3)"
TOKEN="$TOKEN2"
ok "join lai -> cung jti, giu cho trong hang (BR-Q3)"

step "4. Chua duoc admit thi khong giu ghe duoc"
code="$(curl -s -o /dev/null -m 10 -w '%{http_code}' -X POST \
  "$TICKETING/v1/events/$EVENT_ID/holds" \
  -H 'Content-Type: application/json' -H "Idempotency-Key: smoke-bogus-$$" \
  "${IDHDR[@]}" -H 'X-Queue-Token: khong-ton-tai' \
  -d "{\"ticket_type_id\":\"$TT\",\"quantity\":1}")"
[[ "$code" == "403" ]] || die "token gia phai bi 403, nhan $code"
ok "token gia -> 403"

step "5. Cho admit controller tha vao"
admitted=0
for _ in $(seq 1 40); do
  st="$(http GET "$WAITINGROOM/v1/events/$EVENT_ID/queue/status" "${IDHDR[@]}" -H "X-Queue-Token: $TOKEN" | body_of | json_field state)"
  if [[ "$st" == "ADMITTED" ]]; then admitted=1; break; fi
  sleep 1
done
(( admitted )) || die "khong duoc admit sau 40s (state=$st). waitingroom-controller co chay khong?"
ok "state=ADMITTED"

step "6. Giu ghe"
hold() { # <idempotency-key> <quantity>
  http POST "$TICKETING/v1/events/$EVENT_ID/holds" \
    -H 'Content-Type: application/json' -H "Idempotency-Key: $1" \
    "${IDHDR[@]}" -H "X-Queue-Token: $TOKEN" \
    -d "{\"ticket_type_id\":\"$TT\",\"quantity\":$2}"
}

code="$(curl -s -o /dev/null -m 10 -w '%{http_code}' -X POST "$TICKETING/v1/events/$EVENT_ID/holds" \
  -H 'Content-Type: application/json' "${IDHDR[@]}" -H "X-Queue-Token: $TOKEN" \
  -d "{\"ticket_type_id\":\"$TT\",\"quantity\":1}")"
[[ "$code" == "400" ]] || die "thieu Idempotency-Key phai bi 400, nhan $code"
ok "thieu Idempotency-Key -> 400 (BR-O5)"

r="$(hold "smoke-qty-$$" 5)"
[[ "$(echo "$r" | status_of)" == "422" ]] || die "5 ve (> max_per_order) phai bi 422, nhan $(echo "$r" | status_of)"
ok "5 ve -> 422 (BR-O4)"

r="$(hold "smoke-hold-$$" 2)"
[[ "$(echo "$r" | status_of)" == "200" ]] || die "giu 2 ve tra $(echo "$r" | status_of): $(echo "$r" | body_of)"
HOLD_ID="$(echo "$r" | body_of | json_field hold_id)"
EXPIRES="$(echo "$r" | body_of | json_field expires_at)"
[[ -n "$HOLD_ID" && -n "$EXPIRES" ]] || die "phan hoi thieu hold_id/expires_at: $(echo "$r" | body_of)"
ok "giu 2 ve -> hold $HOLD_ID, het han $EXPIRES"
FIRST_BODY="$(echo "$r" | body_of)"

# BR-O5: cung key + cung noi dung -> DUNG response cu (khong phai chi "cung hold" nho BR-O3).
r="$(hold "smoke-hold-$$" 2)"
[[ "$(echo "$r" | status_of)" == "200" && "$(echo "$r" | body_of)" == "$FIRST_BODY" ]]   || die "goi lai cung Idempotency-Key phai tra dung response cu (BR-O5), nhan $(echo "$r" | status_of): $(echo "$r" | body_of)"
ok "goi lai cung key -> dung response cu (BR-O5)"

r="$(hold "smoke-hold-$$" 3)"
[[ "$(echo "$r" | status_of)" == "422" ]]   || die "cung key khac noi dung phai bi 422 (BR-O5), nhan $(echo "$r" | status_of)"
ok "cung key khac noi dung -> 422 (BR-O5)"

kst="$(pg "SELECT status_code FROM idempotency_keys WHERE key = 'smoke-hold-$$'")"
[[ "$kst" == "200" ]] || die "idempotency_keys phai luu status 200, nhan '$kst'"
ok "idempotency_keys da luu response"

r="$(hold "smoke-hold-again-$$" 2)"
[[ "$(echo "$r" | body_of | json_field hold_id)" == "$HOLD_ID" ]] \
  || die "giu lan 2 phai tra dung hold cu (BR-O3), nhan: $(echo "$r" | body_of)"
ok "giu lan 2 -> cung hold, khong giu them (BR-O3)"

step "7. Hai tang ton kho khop nhau"
expect=$((QUOTA - 2))
pgv="$(pg_available)"; rdv="$(redis_available)"
[[ "$pgv" == "$expect" ]] || die "Postgres con $pgv, muon $expect"
[[ "$rdv" == "$expect" ]] || die "Redis con $rdv, muon $expect"
ok "Postgres=$pgv  Redis=$rdv"

step "8. Het han -> sweeper tra kho (BR-O2, BR-O7)"
wait_for "Postgres tra ve $QUOTA" 30 pg_is "$QUOTA"
wait_for "Redis tra ve $QUOTA" 30 redis_is "$QUOTA"
ok "Postgres=$(pg_available)  Redis=$(redis_available)"
st="$(pg "SELECT status FROM orders WHERE event_id = '$EVENT_ID' AND identity_id = '$BUYER' ORDER BY created_at DESC LIMIT 1")"
[[ "$st" == "EXPIRED" ]] || die "don phai o trang thai EXPIRED, nhan '$st'"
ok "don -> EXPIRED"

step "9. Lam lech Redis -> job doi soat tu sua (EV-132)"
# Tru 2 ve khoi bucket 0 o Redis ma khong dong vao Postgres: "mat mot lan tra kho".
rd HINCRBY "inv:{${EVENT_ID}}:tt:${TT}" 0 -2 >/dev/null
[[ "$(redis_available)" == "$((QUOTA - 2))" ]] || die "khong lam lech duoc Redis, con $(redis_available)"
ok "Redis lech: Postgres=$(pg_available)  Redis=$(redis_available)"
# Chu ky 10 giay, can thay do lech o hai chu ky lien tiep nen toi da ~20 giay (+ du phong).
wait_for "job doi soat sua Redis ve $QUOTA" 40 redis_is "$QUOTA"
ok "da tu sua: Postgres=$(pg_available)  Redis=$(redis_available)"

printf '\n[smoke] DAT: %d buoc.\n' "$PASS"
