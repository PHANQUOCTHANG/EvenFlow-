#!/usr/bin/env bash
# demo.sh -- chay tron luong EventFlow tu dau den cuoi, MOI request qua gateway (localhost:8080).
#
#   dang ky + OTP (Mailpit) -> organizer tao su kien -> AI kiem duyet -> tu mo ban dung gio
#   -> xep hang + bốc thăm -> giu ghe -> thanh toan -> ve tu phat + email -> kiem ve QR
#   -> huy giu ghe -> bao mat -> tro ly AI -> het ve
#
# Can: `make up && make migrate && make seed` (tai khoan demo). Chay lai nhieu lan duoc: moi lan
# tao su kien va nguoi mua moi. Mat ~2 phut (cho gio mo ban).
#
# Bien moi truong:
#   GATEWAY_URL  http://localhost:8080      MAILPIT_URL  http://localhost:8025
#   DEMO_LEAD    75   giay tu luc nop duyet toi gio mo ban (phai > MIN_SALE_LEAD_SECONDS cua event-svc, mac dinh compose 60)
#   DEMO_SOLDOUT 1    kiem them tin hieu het ve (ghi ton kho Postgres ve 0 cua su kien demo)
set -uo pipefail

GW="${GATEWAY_URL:-http://localhost:8080}"
MAILPIT="${MAILPIT_URL:-http://localhost:8025}"
LEAD="${DEMO_LEAD:-75}"
SOLDOUT="${DEMO_SOLDOUT:-1}"
PASSWORD='Demo@123456'
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
COMPOSE=(docker compose -f "$ROOT/deploy/compose/docker-compose.yml")

PY="$(command -v python3 || command -v python || true)"
[[ -n "$PY" ]] || { echo "can python3 de doc JSON" >&2; exit 2; }
export PYTHONIOENCODING=utf-8

# Gateway gioi han poll queue/status 0,5 lan/giay moi nguoi (BR-Q4): poll day hon se bi 429. 2,5s la du an toan.
POLL=2.5
PASS=0
START=$(date +%s)
step() { printf '\n[demo %3ss] %s\n' "$(( $(date +%s) - START ))" "$*"; }
ok()   { PASS=$((PASS + 1)); printf '  ok   %s\n' "$*"; }
info() { printf '  ..   %s\n' "$*"; }
die()  {
  printf '  FAIL %s\n' "$*" >&2
  [[ -n "${BODY:-}" ]] && printf '       phan hoi cuoi (%s): %s\n' "${CODE:-?}" "${BODY:0:300}" >&2
  printf '\n[demo] THAT BAI sau %d buoc dat (%ss).\n' "$PASS" "$(( $(date +%s) - START ))" >&2
  exit 1
}

# call <METHOD> <url> [curl args...] -> CODE, BODY
call() {
  local method="$1" url="$2" out; shift 2
  out="$(curl -sS -m 20 -X "$method" -w '\n%{http_code}' "$@" "$url" 2>&1)" || { CODE=000; BODY="$out"; return; }
  CODE="${out##*$'\n'}"; BODY="${out%$'\n'*}"
}
# jget <bieu thuc python tren d> -- doc BODY, in gia tri; rong neu loi/khong co
jget() { printf '%s' "$BODY" | "$PY" -c 'import sys,json
try:
    d=json.load(sys.stdin); v=eval(sys.argv[1]); print("" if v is None else v)
except Exception:
    pass' "$1"; }
expect() { [[ "$CODE" == "$1" ]] || die "$2: mong $1, nhan $CODE"; }
idem() { echo "demo-$1-$$-$RANDOM$RANDOM"; }
auth() { echo "Authorization: Bearer $1"; }
future() { "$PY" -c "import datetime as d;print((d.datetime.now(d.timezone.utc)+d.timedelta(seconds=$1)).strftime('%Y-%m-%dT%H:%M:%SZ'))"; }

# mailbox <email> <mau> -- cho toi 30s toi khi co email gui toi <email> co chu de/noi dung khop <mau>; in email dau tien khop
mailbox() {
  local email="$1" pat="$2" i
  for i in $(seq 1 60); do
    local m; m="$(curl -sS -m 5 "$MAILPIT/api/v1/messages?limit=100" 2>/dev/null | "$PY" -c '
import sys,json,re
d=json.load(sys.stdin); to,pat=sys.argv[1],sys.argv[2]
for m in d["messages"]:
    if any(a["Address"]==to for a in m["To"]) and re.search(pat, m["Subject"]+" "+m["Snippet"]):
        print(m["Subject"]+"\t"+m["Snippet"]); break' "$email" "$pat" 2>/dev/null)"
    [[ -n "$m" ]] && { printf '%s' "$m"; return 0; }
    sleep 0.5
  done
  return 1
}

register_and_login() { # <email> -> in access token (BODY cua login giu nguyen)
  local email="$1" mail otp
  call POST "$GW/api/v1/auth/register/request-otp" -H 'Content-Type: application/json' -d "{\"identifier\":\"$email\"}"
  expect 200 "xin OTP"
  mail="$(mailbox "$email" 'OTP')" || die "khong thay email OTP cua $email trong Mailpit ($MAILPIT)"
  otp="$(printf '%s' "$mail" | grep -oE '[0-9]{6}' | head -1)"
  call POST "$GW/api/v1/auth/register/verify" -H 'Content-Type: application/json' \
    -d "{\"identifier\":\"$email\",\"password\":\"$PASSWORD\",\"otp\":\"$otp\"}"
  expect 201 "xac minh OTP"
  call POST "$GW/api/v1/auth/login" -H 'Content-Type: application/json' \
    -d "{\"identifier\":\"$email\",\"password\":\"$PASSWORD\",\"device_fingerprint\":\"demo-$email\"}"
  expect 200 "dang nhap"
  jget "d['access_token']"
}

login_seed() { # <ten> -> token
  call POST "$GW/api/v1/auth/login" -H 'Content-Type: application/json' \
    -d "{\"identifier\":\"$1@eventflow.local\",\"password\":\"$PASSWORD\",\"device_fingerprint\":\"demo-$1\"}"
  [[ "$CODE" == 200 ]] || die "dang nhap $1@eventflow.local that bai ($CODE): chay 'make seed' truoc"
  jget "d['access_token']"
}

# ---------------------------------------------------------------------------
step "0. Chuan bi"
call GET "$GW/healthz"; expect 200 "gateway $GW"
curl -sS -m 5 -o /dev/null "$MAILPIT/api/v1/messages?limit=1" || die "khong toi duoc Mailpit $MAILPIT"
ORG="$(login_seed organizer)"; MOD="$(login_seed moderator)"; OPS="$(login_seed ops)"
ok "gateway, Mailpit, 3 tai khoan seed (organizer, moderator, ops) dang nhap duoc"

# ---------------------------------------------------------------------------
step "1. Dang ky khach hang: email OTP (Mailpit) -> xac minh -> dang nhap"
STAMP="$(date +%s)$RANDOM"
EMAIL1="khach1.$STAMP@example.com"; EMAIL2="khach2.$STAMP@example.com"
T1="$(register_and_login "$EMAIL1")"; [[ -n "$T1" ]] || die "khong co access_token"
ok "$EMAIL1 dang ky + OTP + dang nhap (JWT)"
call GET "$GW/api/v1/auth/me" -H "$(auth "$T1")"; expect 200 "auth/me"
ok "GET /auth/me -> vai tro $(jget "d['role']")"

# ---------------------------------------------------------------------------
step "2. Organizer tao su kien -> AI kiem duyet -> (Moderator) -> len lich mo ban"
T0="$(future "$LEAD")"; SLUG="demo-$STAMP"
call POST "$GW/api/v1/events" -H "$(auth "$ORG")" -H 'Content-Type: application/json' -H "Idempotency-Key: $(idem ev)" \
  -d "{\"slug\":\"$SLUG\",\"title\":\"Dem nhac EventFlow Live $STAMP\",\"description\":\"Su kien tao truc tiep luc demo\",\"venue\":\"San van dong My Dinh\",\"starts_at\":\"$(future 604800)\",\"sale_start_at\":\"$T0\"}"
expect 201 "tao su kien"; EV="$(jget "d['id']")"; [[ -n "$EV" ]] || die "khong co id su kien"
ok "tao su kien $SLUG (gio mo ban $T0)"
call POST "$GW/api/v1/events/$EV/tiers" -H "$(auth "$ORG")" -H 'Content-Type: application/json' -H "Idempotency-Key: $(idem tier)" \
  -d '{"name":"Standard","price_cents":50000000,"quota":200}'
expect 201 "them hang ve"; ok "hang ve Standard, 200 ve, 500.000 VND"
call POST "$GW/api/v1/events/$EV/submit" -H "$(auth "$ORG")" -H 'Content-Type: application/json' -H "Idempotency-Key: $(idem submit)" -d '{}'
expect 200 "nop duyet"
STATUS=""
for _ in $(seq 1 30); do
  call GET "$GW/api/v1/events/$EV" -H "$(auth "$ORG")"; STATUS="$(jget "d['status']")"
  [[ "$STATUS" != PENDING_REVIEW ]] && break; sleep 1
done
if [[ "$STATUS" == PENDING_REVIEW ]]; then
  info "AI chuyen cho Moderator xem (hang doi kiem duyet)"
  call POST "$GW/api/v1/events/$EV/approve" -H "$(auth "$MOD")" -H 'Content-Type: application/json' -H "Idempotency-Key: $(idem approve)" -d '{"reason":"Noi dung hop le"}'
  expect 200 "moderator duyet"; STATUS="$(jget "d['status']")"
fi
[[ "$STATUS" == SCHEDULED ]] || die "su kien phai SCHEDULED, dang $STATUS"
ok "kiem duyet xong -> SCHEDULED"
call GET "$GW/api/v1/events/public"; expect 200 "danh sach cong khai"
[[ "$(printf '%s' "$BODY" | "$PY" -c "import sys,json;print(any(e['id']=='$EV' for e in json.load(sys.stdin)))")" == True ]] \
  || die "su kien chua co trong danh sach cong khai"
ok "khach an danh thay su kien trong GET /events/public"

# ---------------------------------------------------------------------------
step "3. Phong cho: join truoc gio mo ban (LOBBY) -> boc tham -> ADMITTED"
# Kho Redis duoc nap sau vai giay (ticketing-worker quet Postgres moi 5s). Truoc do availability bao
# SOLD_OUT: co y, "chua nap = tin phia an toan" de khong ai giu duoc ve tu kho chua dong bo.
AV=""
for _ in $(seq 1 30); do
  call GET "$GW/v1/events/$EV/availability"; expect 200 "tinh trang ve"
  AV="$(jget "d['tiers'][0]['availability']")"; [[ "$AV" == AVAILABLE ]] && break; sleep 1
done
[[ "$AV" == AVAILABLE ]] || die "kho ve chua duoc nap sau 30s (availability=$AV): ticketing-worker co chay khong?"
TT="$(jget "d['tiers'][0]['ticket_type_id']")"
ok "ticketing tu nap kho cho su kien moi, availability cong khai: AVAILABLE"
call POST "$GW/v1/events/$EV/queue/join" -H "$(auth "$T1")" -H 'Content-Type: application/json' -d '{"signals":{}}'
expect 200 "join"; Q1="$(jget "d['queue_token']")"; ok "join -> $(jget "d['state']") (chua toi gio mo ban)"
ADMITTED=""; LOOP0=$SECONDS; DEADLINE=$((SECONDS + LEAD + 90)); NEXT=$((SECONDS + 15))
while (( SECONDS < DEADLINE )); do
  call GET "$GW/v1/events/$EV/queue/status" -H "$(auth "$T1")" -H "X-Queue-Token: $Q1"
  [[ "$(jget "d['state']")" == ADMITTED ]] && { ADMITTED=1; break; }
  (( SECONDS >= NEXT )) && { info "cho gio mo ban... $(jget "d['state']") ($(( SECONDS - LOOP0 ))s)"; NEXT=$((SECONDS + 15)); }
  sleep "$POLL"
done
[[ -n "$ADMITTED" ]] || die "khong duoc admit sau $((LEAD + 90))s"
ok "toi gio mo ban: boc tham xong, ADMITTED"
call GET "$GW/api/v1/events/$EV"; [[ "$(jget "d['status']")" == ON_SALE ]] || die "su kien phai tu sang ON_SALE, dang $(jget "d['status']")"
ok "su kien tu chuyen ON_SALE dung gio (khong can Ops bam tay)"

# ---------------------------------------------------------------------------
step "4. Giu ghe -> thanh toan sandbox -> ve tu phat -> email"
call POST "$GW/v1/events/$EV/holds" -H "$(auth "$T1")" -H "X-Queue-Token: $Q1" -H 'Content-Type: application/json' \
  -H "Idempotency-Key: $(idem hold)" -d "{\"ticket_type_id\":\"$TT\",\"quantity\":1}"
expect 200 "giu ghe"; HOLD="$(jget "d['hold_id']")"; ORDER="$(jget "d['order_id']")"
ok "giu 1 ve -> hold $HOLD, het han $(jget "d['expires_at']")"
call GET "$GW/v1/orders/$ORDER" -H "$(auth "$T1")"; expect 200 "xem don"
AMOUNT="$(jget "d['items'][0]['unit_cents']*d['items'][0]['quantity']")"
ok "don $ORDER: $(jget "d['order_status']"), $AMOUNT cent"
call POST "$GW/v1/events/$EV/payments" -H "$(auth "$T1")" -H 'Content-Type: application/json' -H "Idempotency-Key: $(idem pay)" \
  -d "{\"order_id\":\"$ORDER\",\"hold_id\":\"$HOLD\",\"amount\":$AMOUNT,\"method\":\"sandbox\",\"buyer_name\":\"Khach Demo\",\"buyer_email\":\"$EMAIL1\",\"return_url\":\"http://localhost:3000/orders\"}"
expect 200 "tao thanh toan"; PID="$(jget "d['payment_id']")"; TXN="$(jget "d['provider_txn_id']")"
ok "thanh toan sandbox $PID"
call POST "$GW/v1/payments/webhook/sandbox?provider_txn_id=$TXN&order_id=$ORDER&payment_id=$PID&status=SUCCESS&amount=$AMOUNT"
expect 200 "webhook"; ok "webhook cong thanh toan -> don PAID"
ISSUED=""
for _ in $(seq 1 20); do
  call GET "$GW/v1/orders/$ORDER" -H "$(auth "$T1")"
  [[ "$(jget "d['order_status']")" == ISSUED ]] && { ISSUED=1; break; }; sleep 1
done
[[ -n "$ISSUED" ]] || die "don khong sang ISSUED trong 20s (relay payment / consumer ticketing co chay khong?)"
ok "ve TU PHAT (outbox -> RabbitMQ -> ticketing): don ISSUED"
mailbox "$EMAIL1" 'Xác nhận thanh toán' >/dev/null || die "khong co email xac nhan thanh toan"
mailbox "$EMAIL1" 'Vé điện tử' >/dev/null || die "khong co email ve dien tu"
ok "Mailpit co email xac nhan thanh toan va email ve dien tu"

# ---------------------------------------------------------------------------
step "5. Cong soat ve: kiem ve QR"
call POST "$GW/v1/orders/$ORDER/issue-tickets" -H "$(auth "$T1")" -H "Idempotency-Key: $(idem issue)"
expect 200 "lay ve"; QR="$(jget "d['tickets'][0]['qr_payload']")"
[[ "$(jget "d['already_issued']")" == True ]] || die "goi lai issue-tickets phai idempotent (already_issued)"
ok "lay ve (idempotent, khong tao ve thu hai)"
call POST "$GW/v1/tickets/verify" -H "$(auth "$OPS")" -H 'Content-Type: application/json' -d "{\"qr_payload\":\"$QR\"}"
expect 200 "kiem ve hop le"; ok "QR hop le: $(jget "d['message']")"
call POST "$GW/v1/tickets/verify" -H "$(auth "$OPS")" -H 'Content-Type: application/json' -d "{\"qr_payload\":\"${QR%?}x\"}"
expect 400 "kiem ve gia"; ok "QR bi sua 1 ky tu bi tu choi (400 tampered-qr)"

# ---------------------------------------------------------------------------
step "6. Khach thu hai: huy giu ghe tra kho"
T2="$(register_and_login "$EMAIL2")"; [[ -n "$T2" ]] || die "khong co access_token khach 2"
ok "$EMAIL2 dang ky + dang nhap"
call POST "$GW/v1/events/$EV/queue/join" -H "$(auth "$T2")" -H 'Content-Type: application/json' -d '{"signals":{}}'
expect 200 "join khach 2"; Q2="$(jget "d['queue_token']")"
DEADLINE=$((SECONDS + 60))
while (( SECONDS < DEADLINE )); do
  call GET "$GW/v1/events/$EV/queue/status" -H "$(auth "$T2")" -H "X-Queue-Token: $Q2"
  [[ "$(jget "d['state']")" == ADMITTED ]] && break; sleep "$POLL"
done
[[ "$(jget "d['state']")" == ADMITTED ]] || die "khach 2 khong duoc admit"
call POST "$GW/v1/events/$EV/holds" -H "$(auth "$T2")" -H "X-Queue-Token: $Q2" -H 'Content-Type: application/json' \
  -H "Idempotency-Key: $(idem hold2)" -d "{\"ticket_type_id\":\"$TT\",\"quantity\":2}"
expect 200 "khach 2 giu ghe"; HOLD2="$(jget "d['hold_id']")"; ORDER2="$(jget "d['order_id']")"
ok "khach 2 giu 2 ve"
KEY="$(idem cancel)"
call DELETE "$GW/v1/holds/$HOLD2" -H "$(auth "$T2")" -H "Idempotency-Key: $KEY"
[[ "$CODE" == 200 || "$CODE" == 204 ]] || die "huy giu ghe: mong 200/204, nhan $CODE"
call GET "$GW/v1/orders/$ORDER2" -H "$(auth "$T2")"
[[ "$(jget "d['order_status']")" == CANCELLED ]] || die "don phai CANCELLED, dang $(jget "d['order_status']")"
ok "huy giu ghe -> don CANCELLED, kho duoc tra"
call DELETE "$GW/v1/holds/$HOLD2" -H "$(auth "$T2")" -H "Idempotency-Key: $KEY"
[[ "$CODE" == 200 || "$CODE" == 204 ]] || die "huy lai cung Idempotency-Key phai thanh cong, nhan $CODE"
ok "huy lai cung Idempotency-Key -> thanh cong (idempotent)"

# ---------------------------------------------------------------------------
step "7. Bao mat"
call POST "$GW/internal/reconcile"; [[ "$CODE" == 404 || "$CODE" == 403 ]] || die "/internal/* phai bi chan tu ngoai, nhan $CODE"
ok "/internal/* bi chan tu ngoai ($CODE)"
call POST "$GW/v1/events/$EV/queue/join" -H 'X-Identity-Id: 00000000-0000-4000-8000-000000000001' \
  -H 'X-Identity-Verified: true' -H 'Content-Type: application/json' -d '{}'
expect 401 "gia header danh tinh khi khong co JWT"; ok "gia X-Identity-Id khong co JWT -> 401"
call GET "$GW/v1/orders/$ORDER" -H "$(auth "$T2")"
expect 404 "xem don cua nguoi khac"; ok "khach 2 xem don cua khach 1 -> 404 (khong lo don ton tai)"
call POST "$GW/v1/events/$EV/holds" -H "$(auth "$T2")" -H "X-Queue-Token: $Q1" -H 'Content-Type: application/json' \
  -H "Idempotency-Key: $(idem steal)" -d "{\"ticket_type_id\":\"$TT\",\"quantity\":1}"
[[ "$CODE" == 401 || "$CODE" == 403 ]] || die "dung queue token cua nguoi khac phai bi tu choi, nhan $CODE"
ok "dung queue token cua khach 1 bang tai khoan khach 2 -> $CODE"
call GET "$GW/api/v1/events" -H "$(auth "$T1")"
expect 403 "buyer xem danh sach quan tri"; ok "buyer khong vao duoc danh sach su kien cua organizer (403)"

# ---------------------------------------------------------------------------
step "8. Tro ly AI trong luc cho"
call POST "$GW/v1/events/$EV/assistant/messages" -H "$(auth "$T1")" -H 'Content-Type: application/json' -d '{"question":"Khi nao mo ban ve?"}'
expect 202 "gui cau hoi"; JOB="$(jget "d['job_id']")"
ANS=""
for _ in $(seq 1 30); do
  call GET "$GW/v1/assistant/messages/$JOB" -H "$(auth "$T1")"
  [[ "$(jget "d['status']")" == done ]] && { ANS="$(jget "d['answer']")"; break; }; sleep 1
done
[[ -n "$ANS" ]] || die "tro ly khong tra loi trong 30s"
ok "tro ly tra loi (${#ANS} ky tu, nguon $(jget "d['source']")): ${ANS:0:70}"

# ---------------------------------------------------------------------------
if [[ "$SOLDOUT" == 1 ]]; then
  step "9. Het ve: ton kho ve 0 -> khach moi nhan SOLD_OUT"
  if "${COMPOSE[@]}" exec -T postgres psql -U eventflow -d eventflow -q -c \
       "UPDATE ticket_inventory SET available = 0 WHERE ticket_type_id = '$TT'" >/dev/null 2>&1; then
    SO=""
    for _ in $(seq 1 30); do
      call GET "$GW/v1/events/$EV/availability"
      [[ "$(jget "d['tiers'][0]['availability']")" == SOLD_OUT ]] && { SO=1; break; }; sleep 1
    done
    [[ -n "$SO" ]] || die "availability khong sang SOLD_OUT sau khi kho ve 0"
    ok "GET availability -> SOLD_OUT"
    # Co "het ve han" can 3 lan quan sat lien tiep (EV-146) de khong bao nham khi con hold co the het han.
    EMAIL3="khach3.$STAMP@example.com"; T3="$(register_and_login "$EMAIL3")"
    call POST "$GW/v1/events/$EV/queue/join" -H "$(auth "$T3")" -H 'Content-Type: application/json' -d '{"signals":{}}'
    expect 200 "join khach 3"; Q3="$(jget "d['queue_token']")"; ST="$(jget "d['state']")"
    if [[ "$ST" == SOLD_OUT ]]; then
      # Co het ve da bat truoc luc join: nguoi moi bi tu choi ngay, khong cap token, khong vao hang.
      ok "khach moi join khi da het ve -> SOLD_OUT ngay (khong cap token)"
    else
      DEADLINE=$((SECONDS + 30))
      while (( SECONDS < DEADLINE )); do
        call GET "$GW/v1/events/$EV/queue/status" -H "$(auth "$T3")" -H "X-Queue-Token: $Q3"
        ST="$(jget "d['state']")"; [[ "$ST" == SOLD_OUT ]] && break; sleep "$POLL"
      done
      [[ "$ST" == SOLD_OUT ]] || die "khach dang cho phai nhan SOLD_OUT trong 30s, dang '$ST'"
      ok "khach dang cho duoc bao SOLD_OUT"
    fi
    call POST "$GW/v1/events/$EV/queue/join" -H "$(auth "$T1")" -H 'Content-Type: application/json' -d '{"signals":{}}'
    ok "khach da co cho khong bi day them vao hang vo vong ($(jget "d['state']"))"
  else
    info "bo qua: khong chay duoc docker compose exec postgres (DEMO_SOLDOUT=0 de tat buoc nay)"
  fi
fi

# ---------------------------------------------------------------------------
step "Quan sat (thong tin, khong tinh buoc)"
JS="$(curl -sS -m 5 http://localhost:16686/api/services 2>/dev/null | "$PY" -c 'import sys,json;print(", ".join(json.load(sys.stdin)["data"]))' 2>/dev/null)"
info "Jaeger http://localhost:16686 co trace cua: ${JS:-khong doc duoc}"
info "Mailpit $MAILPIT | RabbitMQ http://localhost:15672 (eventflow/eventflow) | Grafana http://localhost:3001"

printf '\n[demo] DAT: %d buoc trong %ss.\n' "$PASS" "$(( $(date +%s) - START ))"
