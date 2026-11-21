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
#   LOAD_EVENT    event UUID; only 22222222-2222-4222-8222-0000000001xx (default ...000000000194). RESET wipes it,
#                 so any other id (e.g. the demo event 11111111-...) is refused.
#   USERS         load identities to create   (default 50; use >= what the profile needs, k6 prints it)
#   QUOTA         tickets per ticket type     (default DEFAULT_QUOTA of tests/load/lib/profiles.js; pass the same to k6)
#   HOLD_TTL      hold TTL in seconds         (default DEFAULT_HOLD_TTL_S of tests/load/lib/profiles.js)
#   RESET         1 = wipe the event first    (default 1)
#   ALLOW_REMOTE  1 = allow a non-local Docker engine (DOCKER_HOST / docker context); refused by default
#
# Identities 00000000-0000-4000-9000-{n as 12 digits}, n = 1..USERS: the 9000 range never collides with seed.sh
# (8000). password_hash '!' matches no password: they cannot log in, k6 mints their access tokens (spec D1).
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
COMPOSE=(docker compose -f "$ROOT/deploy/compose/docker-compose.yml")

# Single source of the defaults shared with k6 (R-3): read them from profiles.js.
profile_default() {
  sed -n "s/^export const $1 = \([0-9][0-9]*\);.*/\1/p" "$ROOT/tests/load/lib/profiles.js"
}

CMD="${1:-seed}"
LOAD_EVENT="${LOAD_EVENT:-22222222-2222-4222-8222-000000000194}"
USERS="${USERS:-50}"
QUOTA="${QUOTA:-$(profile_default DEFAULT_QUOTA)}"
HOLD_TTL="${HOLD_TTL:-$(profile_default DEFAULT_HOLD_TTL_S)}"
RESET="${RESET:-1}"

case "$CMD" in seed|verify) ;; *) echo "usage: $0 [seed|verify]" >&2; exit 2 ;; esac
[[ "$LOAD_EVENT" =~ ^22222222-2222-4222-8222-0000000001[0-9a-f]{2}$ ]] \
  || { echo "LOAD_EVENT must be a load event 22222222-2222-4222-8222-0000000001xx (RESET wipes it): $LOAD_EVENT" >&2; exit 2; }
for v in USERS QUOTA HOLD_TTL; do
  [[ "${!v}" =~ ^[1-9][0-9]*$ ]] || { echo "$v must be a positive integer: ${!v}" >&2; exit 2; }
done
(( USERS <= 999999999999 )) || { echo "USERS too large: $USERS" >&2; exit 2; }

# Refuse a remote Docker engine (R-2): `docker compose exec postgres` would seed and RESET that engine's database.
guard_docker() {
  if [[ "${ALLOW_REMOTE:-}" == "1" ]]; then
    echo "[seed-load] ALLOW_REMOTE=1: Docker engine not checked" >&2
    return 0
  fi
  case "${DOCKER_HOST:-}" in
    ""|unix://*|npipe://*) ;;
    *) echo "DOCKER_HOST=$DOCKER_HOST is not a local engine; set ALLOW_REMOTE=1 to seed it on purpose" >&2; exit 2 ;;
  esac
  local ctx
  ctx="$(docker context show 2>/dev/null || echo default)"
  ctx="${ctx//[$'\r\n ']/}"
  case "$ctx" in
    default|desktop-linux|desktop-windows|colima|orbstack|rancher-desktop) ;;
    *) echo "docker context '$ctx' may be remote; set ALLOW_REMOTE=1 to seed it on purpose" >&2; exit 2 ;;
  esac
}

psql_run() {
  "${COMPOSE[@]}" exec -T postgres psql -U eventflow -d eventflow -v ON_ERROR_STOP=1 -q "$@"
}

seed() {
  EVENT_ID="$LOAD_EVENT" QUOTA="$QUOTA" RESET="$RESET" HOLD_TTL_SECONDS="$HOLD_TTL" SALE_START_IN_SECONDS=0 \
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

guard_docker
case "$CMD" in
  seed) seed ;;
  verify) verify ;;
esac
