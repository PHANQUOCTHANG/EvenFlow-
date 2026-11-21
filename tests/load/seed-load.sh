#!/usr/bin/env bash
# seed-load.sh -- data for the k6 load suite (tests/load/main.js), spec D2 / AC-4 / AC-10.
#
#   tests/load/seed-load.sh [seed]   dedicated event (deploy/scripts/seed.sh) + USERS load identities
#   tests/load/seed-load.sh verify   authoritative oversell check in Postgres: issued + open holds <= quota
#
# Idempotent: identities use ON CONFLICT DO NOTHING; the event is re-created by seed.sh (RESET=1 by default, so
# every run starts with the full quota). Needs only docker (compose up + migrated), like seed.sh.
#
# Env:
#   LOAD_EVENT  event UUID                       (default 22222222-2222-4222-8222-000000000194)
#   USERS       load identities to create        (default 50; use >= what the profile needs, k6 prints it)
#   QUOTA       tickets per ticket type          (default 200)
#   RESET       1 = wipe the event first         (default 1)
#
# Identities 00000000-0000-4000-9000-{n as 12 digits}, n = 1..USERS: the 9000 range never collides with seed.sh
# (8000). password_hash '!' matches no password: they cannot log in, k6 mints their access tokens (spec D1).
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
COMPOSE=(docker compose -f "$ROOT/deploy/compose/docker-compose.yml")

CMD="${1:-seed}"
LOAD_EVENT="${LOAD_EVENT:-22222222-2222-4222-8222-000000000194}"
USERS="${USERS:-50}"
QUOTA="${QUOTA:-200}"
RESET="${RESET:-1}"

[[ "$LOAD_EVENT" =~ ^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$ ]] \
  || { echo "LOAD_EVENT is not a UUID: $LOAD_EVENT" >&2; exit 2; }
for v in USERS QUOTA; do
  [[ "${!v}" =~ ^[1-9][0-9]*$ ]] || { echo "$v must be a positive integer: ${!v}" >&2; exit 2; }
done
(( USERS <= 999999999999 )) || { echo "USERS too large: $USERS" >&2; exit 2; }

psql_run() {
  "${COMPOSE[@]}" exec -T postgres psql -U eventflow -d eventflow -v ON_ERROR_STOP=1 -q "$@"
}

seed() {
  EVENT_ID="$LOAD_EVENT" QUOTA="$QUOTA" RESET="$RESET" SALE_START_IN_SECONDS=0 \
    bash "$ROOT/deploy/scripts/seed.sh"
  echo "[seed-load] identities 1..$USERS (range 00000000-0000-4000-9000-*)"
  psql_run -v users="$USERS" <<'SQL'
INSERT INTO identities (id, email, password_hash, role, otp_verified_at)
SELECT ('00000000-0000-4000-9000-' || lpad(n::text, 12, '0'))::uuid,
       'load' || n || '@eventflow.local', '!', 'buyer', now()
  FROM generate_series(1, :users) AS n
ON CONFLICT DO NOTHING;
SQL
  local have
  have="$(psql_run -t -A -v users="$USERS" <<'SQL'
SELECT count(*) FROM identities
 WHERE id BETWEEN '00000000-0000-4000-9000-000000000001'::uuid
              AND ('00000000-0000-4000-9000-' || lpad(:'users', 12, '0'))::uuid;
SQL
)"
  have="${have//[$'\r\n ']/}"
  [[ "$have" == "$USERS" ]] || { echo "[seed-load] expected $USERS identities, found $have" >&2; exit 1; }
  local tt
  tt="$(psql_run -t -A -c "SELECT md5('${LOAD_EVENT}' || ':std')::uuid")"
  echo "EVENT=$LOAD_EVENT"
  echo "TICKET_TYPE=${tt//[$'\r\n ']/}"
  echo "QUOTA=$QUOTA"
  echo "USERS=$USERS"
}

verify() {
  echo "[seed-load] verify event $LOAD_EVENT: issued + open holds <= quota per ticket type"
  local out
  out="$(psql_run -t -A -F ' ' -v event_id="$LOAD_EVENT" <<'SQL'
SELECT tt.name, tt.quota,
       (SELECT count(*) FROM tickets t
         WHERE t.ticket_type_id = tt.id AND t.status IN ('ISSUED', 'CHECKED_IN')) AS issued,
       (SELECT coalesce(sum(h.quantity), 0) FROM seat_holds h
         WHERE h.ticket_type_id = tt.id AND h.released_at IS NULL AND h.consumed_at IS NULL) AS open_holds
  FROM ticket_types tt
 WHERE tt.event_id = :'event_id'::uuid
 ORDER BY tt.name;
SQL
)"
  [[ -n "${out//[$'\r\n ']/}" ]] || { echo "[seed-load] no ticket types for $LOAD_EVENT (seed first)" >&2; exit 1; }
  local fail=0 name quota issued holds
  while read -r name quota issued holds; do
    [[ -n "$name" ]] || continue
    holds="${holds//$'\r'/}"
    if (( issued + holds <= quota )); then
      echo "  PASS $name: issued=$issued open_holds=$holds quota=$quota"
    else
      echo "  FAIL $name: issued=$issued open_holds=$holds > quota=$quota (OVERSELL)"
      fail=1
    fi
  done <<< "$out"
  return "$fail"
}

case "$CMD" in
  seed) seed ;;
  verify) verify ;;
  *) echo "usage: $0 [seed|verify]" >&2; exit 2 ;;
esac
