#!/usr/bin/env bash
# seed.sh -- tao du lieu mau cho moi truong local: 1 su kien ON_SALE co ton kho.
#
# Chay duoc lai nhieu lan (idempotent). Chi can docker; khong can psql/redis-cli
# tren may host.
#
# Bien moi truong:
#   EVENT_ID              UUID su kien            (mac dinh: su kien demo)
#   QUOTA                 tong so ve moi hang     (mac dinh: 10000 -> 2 hang = 20.000)
#   HOLD_TTL_SECONDS      thoi gian giu ghe       (mac dinh: 600 = 10 phut, BR-O2)
#   SALE_START_IN_SECONDS T0 sau bao nhieu giay   (mac dinh: 0 = da mo ban)
#   RESET=1               xoa du lieu cu cua su kien nay (ca Postgres lan Redis)
#
# Hang ve duoc tao voi id xac dinh: md5('<event>:<ten>')::uuid, nen script khac
# (smoke.sh) tinh lai duoc ma khong can doc output.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
COMPOSE=(docker compose -f "$ROOT/deploy/compose/docker-compose.yml")

EVENT_ID="${EVENT_ID:-11111111-1111-4111-8111-111111111111}"
QUOTA="${QUOTA:-10000}"
HOLD_TTL_SECONDS="${HOLD_TTL_SECONDS:-600}"
SALE_START_IN_SECONDS="${SALE_START_IN_SECONDS:-0}"
RESET="${RESET:-0}"

[[ "$EVENT_ID" =~ ^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$ ]] \
  || { echo "EVENT_ID khong phai UUID: $EVENT_ID" >&2; exit 2; }
for v in QUOTA HOLD_TTL_SECONDS SALE_START_IN_SECONDS; do
  [[ "${!v}" =~ ^[0-9]+$ ]] || { echo "$v phai la so nguyen khong am: ${!v}" >&2; exit 2; }
done
(( QUOTA >= 64 )) || { echo "QUOTA phai >= 64 de moi bucket co it nhat 1 ve" >&2; exit 2; }

psql_run() {
  "${COMPOSE[@]}" exec -T postgres psql -U eventflow -d eventflow -v ON_ERROR_STOP=1 -q "$@"
}
redis_run() {
  "${COMPOSE[@]}" exec -T redis redis-cli "$@"
}

if [[ "$RESET" == "1" ]]; then
  echo "[seed] RESET su kien $EVENT_ID"
  psql_run -v event_id="$EVENT_ID" <<'SQL'
DELETE FROM seat_holds WHERE order_id IN (SELECT id FROM orders WHERE event_id = :'event_id'::uuid);
DELETE FROM order_items WHERE order_id IN (SELECT id FROM orders WHERE event_id = :'event_id'::uuid);
DELETE FROM orders WHERE event_id = :'event_id'::uuid;
DELETE FROM queue_sessions WHERE event_id = :'event_id'::uuid;
DELETE FROM events WHERE id = :'event_id'::uuid;   -- cascade: ticket_types, ticket_inventory
SQL
  # Moi key cua su kien deu mang hash tag {<event>} (xem docs/03 muc 7).
  "${COMPOSE[@]}" exec -T redis sh -c \
    "redis-cli --scan --pattern '*{${EVENT_ID}}*' | xargs -r redis-cli DEL >/dev/null"
fi

echo "[seed] su kien $EVENT_ID  quota=$QUOTA/hang  ttl=${HOLD_TTL_SECONDS}s"
psql_run -v event_id="$EVENT_ID" -v quota="$QUOTA" -v ttl="$HOLD_TTL_SECONDS" \
         -v sale_in="$SALE_START_IN_SECONDS" <<'SQL'
-- Nguoi dung: 1 organizer + 10 buyer, UUID co dinh de script/k6 tham chieu duoc.
-- password_hash = '!' khong khop bat ky mat khau nao: tai khoan khong dang nhap duoc.
INSERT INTO identities (id, email, password_hash, role, otp_verified_at)
VALUES ('00000000-0000-4000-8000-0000000000a0', 'organizer@eventflow.local', '!', 'organizer', now())
ON CONFLICT DO NOTHING;

INSERT INTO identities (id, email, password_hash, role, otp_verified_at)
SELECT ('00000000-0000-4000-8000-' || lpad(n::text, 12, '0'))::uuid,
       'buyer' || n || '@eventflow.local', '!', 'buyer', now()
  FROM generate_series(1, 10) AS n
ON CONFLICT DO NOTHING;

INSERT INTO events (id, organizer_id, slug, title, description, venue,
                    starts_at, sale_start_at, status, hold_ttl_seconds)
VALUES (:'event_id'::uuid,
        '00000000-0000-4000-8000-0000000000a0',
        'demo-' || substr(:'event_id', 1, 8),
        'EventFlow Demo Concert',
        'Su kien mau cho moi truong local.',
        'San van dong Quoc gia My Dinh',
        now() + interval '30 days',
        now() + make_interval(secs => :sale_in),
        'ON_SALE',
        :ttl)
ON CONFLICT (id) DO UPDATE
   SET status = 'ON_SALE',
       hold_ttl_seconds = EXCLUDED.hold_ttl_seconds,
       sale_start_at = EXCLUDED.sale_start_at,
       updated_at = now();

INSERT INTO ticket_types (id, event_id, name, price_cents, quota, bucket_count)
VALUES (md5(:'event_id' || ':std')::uuid, :'event_id'::uuid, 'Standard', 50000000, :quota, 32),
       (md5(:'event_id' || ':vip')::uuid, :'event_id'::uuid, 'VIP',     150000000, :quota, 32)
ON CONFLICT (id) DO NOTHING;

-- Chia quota deu ra 32 bucket; phan du don vao cac bucket dau.
-- ON CONFLICT DO NOTHING: chay lai KHONG dat lai ton kho dang bi hold tru di.
INSERT INTO ticket_inventory (ticket_type_id, bucket, capacity, available)
SELECT tt.id, b,
       tt.quota / 32 + CASE WHEN b < tt.quota % 32 THEN 1 ELSE 0 END,
       tt.quota / 32 + CASE WHEN b < tt.quota % 32 THEN 1 ELSE 0 END
  FROM ticket_types tt
 CROSS JOIN generate_series(0, 31) AS b
 WHERE tt.event_id = :'event_id'::uuid AND tt.bucket_count = 32
ON CONFLICT (ticket_type_id, bucket) DO NOTHING;
SQL

# T0 cho phong cho. event-svc (EVF-20) se ghi thay buoc nay.
#
# `psql -c` khong noi suy bien (:'x'), nen dung truc tiep EVENT_ID -- da duoc kiem
# la UUID o dau script nen khong co nguy co chen SQL.
T0_MS="$(psql_run -t -A \
  -c "SELECT (extract(epoch FROM sale_start_at) * 1000)::bigint FROM events WHERE id = '${EVENT_ID}'")"
redis_run HSET "wr:{${EVENT_ID}}:meta" sale_start_ms "${T0_MS//[$'\r\n ']/}" >/dev/null

echo "[seed] xong. Su kien $EVENT_ID  T0=${T0_MS//[$'\r\n ']/}ms"
echo "[seed] ton kho se duoc worker nap vao Redis trong vai giay (ticketing-worker)."
