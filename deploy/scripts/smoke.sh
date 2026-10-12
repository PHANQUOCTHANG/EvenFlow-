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
  -H 'Content-Type: application/json' -H "X-Identity-Id: $BUYER" -d '{"signals":{}}')"
[[ "$(echo "$resp" | status_of)" == "200" ]] || die "join tra $(echo "$resp" | status_of): $(echo "$resp" | body_of)"
TOKEN="$(echo "$resp" | body_of | json_field queue_token)"
[[ -n "$TOKEN" ]] || die "join khong tra queue_token"
ok "join -> token ${TOKEN:0:8}...  state=$(echo "$resp" | body_of | json_field state)"

# Goi lai phai ra dung token cu (BR-Q3)
resp2="$(http POST "$WAITINGROOM/v1/events/$EVENT_ID/queue/join" \
  -H 'Content-Type: application/json' -H "X-Identity-Id: $BUYER" -d '{}')"
[[ "$(echo "$resp2" | body_of | json_field queue_token)" == "$TOKEN" ]] \
  || die "join lan 2 phai tra lai dung token (BR-Q3)"
ok "join lai -> cung token (BR-Q3)"

step "4. Chua duoc admit thi khong giu ghe duoc"
code="$(curl -s -o /dev/null -m 10 -w '%{http_code}' -X POST \
  "$TICKETING/v1/events/$EVENT_ID/holds" \
  -H 'Content-Type: application/json' -H "Idempotency-Key: smoke-bogus-$$" \
  -H "X-Identity-Id: $BUYER" -H 'X-Queue-Token: khong-ton-tai' \
  -d "{\"ticket_type_id\":\"$TT\",\"quantity\":1}")"
[[ "$code" == "403" ]] || die "token gia phai bi 403, nhan $code"
ok "token gia -> 403"

step "5. Cho admit controller tha vao"
admitted=0
for _ in $(seq 1 40); do
  st="$(http GET "$WAITINGROOM/v1/events/$EVENT_ID/queue/status" -H "X-Queue-Token: $TOKEN" | body_of | json_field state)"
  if [[ "$st" == "ADMITTED" ]]; then admitted=1; break; fi
  sleep 1
done
(( admitted )) || die "khong duoc admit sau 40s (state=$st). waitingroom-controller co chay khong?"
ok "state=ADMITTED"

step "6. Giu ghe"
hold() { # <idempotency-key> <quantity>
  http POST "$TICKETING/v1/events/$EVENT_ID/holds" \
    -H 'Content-Type: application/json' -H "Idempotency-Key: $1" \
    -H "X-Identity-Id: $BUYER" -H "X-Queue-Token: $TOKEN" \
    -d "{\"ticket_type_id\":\"$TT\",\"quantity\":$2}"
}

code="$(curl -s -o /dev/null -m 10 -w '%{http_code}' -X POST "$TICKETING/v1/events/$EVENT_ID/holds" \
  -H 'Content-Type: application/json' -H "X-Identity-Id: $BUYER" -H "X-Queue-Token: $TOKEN" \
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

printf '\n[smoke] DAT: %d buoc.\n' "$PASS"
