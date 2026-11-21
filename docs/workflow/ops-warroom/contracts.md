# Wire contracts (frozen) — ops-warroom

Source: read-only survey of develop @ c5fb598. `file:line` refs are relative to the repo root.
Once `spec.md` is approved these shapes are frozen; any change goes through a change request in this folder.

## 0. Cross-cutting (unchanged, restated)

- Gateway auth = `Authorization: Bearer <jwt>`; gateway strips client `X-Identity-*` and injects
  `X-Identity-Id`, `X-Identity-Role`, `X-Identity-Verified`, `X-Device-Id` (`services/gateway/internal/proxy/gateway.go`).
  Role-restricted routes return **403 `/problems/forbidden`** at the gateway (`gateway.go:124`); no token → 401
  `/problems/unauthorized`; expired → 401 `/problems/token-expired`.
- Services read identity with `authx.PrincipalFromHeaders` (`libs/go/authx/principal.go:75`) and re-check the role
  themselves (defense in depth, same pattern as check-in: `services/ticketing/internal/adapter/http/ticket_handler.go:139`).
- Errors: RFC 7807 `{type, title, status, detail}` via `httpx.WriteProblem` / `WriteProblemType`; clients branch on
  `status` and `type`, never on `detail`.
- Times: RFC 3339 UTC strings. Money: integer VND (`*_cents`, no minor unit). Rates: float people/second.
- All new responses carry `Cache-Control: no-store`.
- No path rewriting at the gateway: the service registers exactly the public path.

## 1. Existing state these endpoints expose (no behaviour change)

| State | Where | Notes |
|---|---|---|
| Lobby (before T0) | Redis SET `wr:{ev}:lobby` | `SCARD` |
| Queue | Redis ZSET `wr:{ev}:queue` | `ZCARD` (= `waitingroom_queue_depth`) |
| Admitted (valid window) | Redis ZSET `wr:{ev}:admitted`, score = expire_at_ms | `ZCOUNT key (now_ms +inf`; expired members are removed lazily by `status.lua` |
| Admit rate | HASH `wr:{ev}:meta` field `admit_rate` (rate × 1000, int) | written by the leader each tick (`set_rate.lua`) |
| Ops override | `meta.override = "1"`, `meta.override_rate` (×1000) | `Queue.SetRateOverride` / `ClearRateOverride` already exist (`services/waitingroom/internal/adapter/redis/queue.go:186,192`) but no caller. Controller syncs them every tick (`services/waitingroom/internal/app/admit_controller.go:225`) → effective ≤ 1 tick (1 s). Rate 0 = emergency pause (comment at `admit_controller.go:363-366`); controller clamps to `[0, MaxRate]` |
| Sold-out flags | `meta.sold_out`, `meta.stock_zero` (written by ticketing, EV-146) | |
| Sale start | `meta.sale_start_ms` (written by ticketing active-events discovery) | used here as "room configured" marker |
| Leader | STRING `wr:{ev}:leader` (lease), STRING `wr:{ev}:leader_epoch` | `services/waitingroom/cmd/controller/main.go:164-166` |
| Checkout health | HASH `loadsignal:ticketing` read via `loadsignal.NewReader(...).Read` → `Aggregate{Pods, P99, ErrorRate, PoolUsage}`; `ErrNoSignal` when no live pod | `libs/go/loadsignal/loadsignal.go:171-181` |
| Inventory | Postgres `ticket_types.quota`, `ticket_inventory.available` (per bucket), `seat_holds` (open = `released_at IS NULL AND consumed_at IS NULL`), `orders.status`, `order_items`, `tickets.status`, `payments` | `migrations/0001_core.sql:51-170` |
| AI mode | ai-worker `GET /admin/mode` (unauthenticated, blocked at gateway: `routes.go:79`) | `services/ai-worker/app/main.py:169-184` |
| Audit table | `audit_logs(actor_type, actor_id, action, subject, reason, metadata jsonb, created_at)` | `migrations/0001_core.sql:198-208`; insert style as `services/event/internal/adapter/postgres/audit_repo.go:31` |

## 2. New gateway routes (`services/gateway/internal/proxy/routes.go`)

| Pattern | Upstream | Access | Roles | Rate |
|---|---|---|---|---|
| `GET /v1/ops/events/{eventID}/queue` | Waitingroom | Authenticated | `ops` | RateDefault |
| `PUT /v1/ops/events/{eventID}/admit-override` | Waitingroom | Authenticated | `ops` | RateDefault |
| `DELETE /v1/ops/events/{eventID}/admit-override` | Waitingroom | Authenticated | `ops` | RateDefault |
| `GET /v1/ops/events/{eventID}/inventory` | Ticketing | Authenticated | `ops` | RateDefault |
| `GET /v1/ops/ai/mode` | Assistant (ai-worker) | Authenticated | `ops` | RateDefault |
| `GET /v1/organizer/events/{eventID}/sales` | Ticketing | Authenticated | `organizer` | RateDefault |

Any other method on these paths → gateway 404/405 per ServeMux (not forwarded). `/admin/` stays Blocked.
Existing routes are unchanged.

## 3. Waitingroom

### 3.1 `GET /v1/ops/events/{eventID}/queue`

200:
```json
{
  "event_id": "11111111-1111-4111-8111-111111111111",
  "generated_at": "2026-11-01T10:00:00.123Z",
  "lobby": 0,
  "queued": 4210,
  "admitted_active": 350,
  "admit_rate_per_sec": 50.0,
  "override": { "active": true, "rate_per_sec": 0.0 },
  "paused": true,
  "sold_out": false,
  "stock_zero": false,
  "controller": { "leader_present": true, "epoch": 7 },
  "checkout": { "signal": "ok", "pods": 3, "p99_ms": 420, "error_rate": 0.004, "db_pool_usage": 0.61 }
}
```
- `override` is `{ "active": false, "rate_per_sec": null }` when no override.
- `paused` = `override.active && override.rate_per_sec == 0`.
- `admit_rate_per_sec` = `meta.admit_rate / 1000`, `0` when absent.
- `controller.epoch` = `0` when the epoch key is absent.
- `checkout` = `{ "signal": "none", "pods": 0, "p99_ms": null, "error_rate": null, "db_pool_usage": null }` on `ErrNoSignal`;
  `{ "signal": "error", ... nulls }` on any other read error (the endpoint still returns 200).
- All Redis reads for `{ev}` in one pipeline (same hash slot).

Errors: 401 no principal; 403 `/problems/forbidden` role ≠ `ops`; 400 `eventID` not a UUID; 404 `/problems/not-found`
when `wr:{ev}:meta` has no `sale_start_ms` (room never configured); 503 Redis error.

### 3.2 `PUT /v1/ops/events/{eventID}/admit-override`

Request `{ "rate_per_sec": 120, "reason": "checkout p99 cao, giam tai" }`
- `rate_per_sec`: JSON **integer**, `0 ≤ rate ≤ 10000` (amended CR-1: a fractional value such as `0.3` is rejected with
  400, because rates below 0.5 give a batch of 0 and would stop admission while `paused=false`). Stored as `rate*1000`.
  `0` = emergency pause.
  Values above the controller's `ADMIT_MAX_RATE` are accepted and clamped by the controller (existing behaviour).
- `reason`: string, trimmed length 3..200.
- Unknown fields rejected (400). Body ≤ 4 KiB.

200:
```json
{ "event_id": "…", "override": { "active": true, "rate_per_sec": 120.0 }, "paused": false,
  "applied_at": "2026-11-01T10:00:01Z", "audit": "recorded" }
```
`audit` is `"recorded"` or `"failed"` (see 3.4). The `override` in the response is read back from the stored integer
(`override_rate/1000`), not echoed from the request.

Errors: 400 `/problems/invalid-override` (bad JSON, rate out of range/non-finite, reason length); 401; 403; 404 (as 3.1);
503 Redis error (nothing applied, nothing audited as applied); 503 `/problems/audit-unavailable` only when
`DATABASE_URL` is unset or cannot be parsed (fail-closed misconfiguration, nothing applied). An unreachable database is
**not** this case: the pool connects lazily, and connect or insert failures at request time follow 3.4
(`audit:"failed"`, change applied) (amended CR-1).

### 3.3 `DELETE /v1/ops/events/{eventID}/admit-override`

Request `{ "reason": "su co da xu ly" }` (same rules for `reason`). Idempotent: clearing when no override is active
returns 200 too.

200: `{ "event_id": "…", "override": { "active": false, "rate_per_sec": null }, "paused": false, "applied_at": "…", "audit": "recorded" }`

Errors: as 3.2.

### 3.4 Audit record (both writes)

Order: validate → apply in Redis → insert audit. Both the Redis apply and the insert run on
`context.WithoutCancel(r.Context())` with their own timeouts (Redis 1 s, insert 2 s), so a client disconnect after the
apply cannot cancel the audit. One row per successful apply:

| column | value |
|---|---|
| actor_type | `human` |
| actor_id | principal id (UUID string) |
| action | `ADMIT_OVERRIDE_SET` / `ADMIT_OVERRIDE_CLEARED` (`ADMIT_PAUSED` when rate = 0) |
| subject | `event:{eventID}` |
| reason | request `reason` |
| metadata | `{"rate_per_sec": 120, "previous": {"active": false, "rate_per_sec": null}, "request_id": "<X-Request-Id>"}` |

If the insert fails after the Redis apply, the change stays applied (an emergency pause must not depend on Postgres
being healthy), the response says `"audit": "failed"`, and the service logs one ERROR line with the same fields
(`actor_id, action, subject, reason, metadata`). See decision D2 in `spec.md`.

Idempotency-Key is **not** required: PUT-to-value and DELETE are idempotent by definition; a retried request
produces a second identical audit row, which is acceptable.

## 4. Ticketing

### 4.1 `GET /v1/ops/events/{eventID}/inventory`

200:
```json
{
  "event_id": "…",
  "generated_at": "2026-11-01T10:00:00Z",
  "tiers": [
    { "ticket_type_id": "…", "name": "Standard", "quota": 1000, "available": 410,
      "held": 40, "paid": 300, "issued": 250 }
  ]
}
```
- `available` = `SUM(ticket_inventory.available)` for the tier (Postgres, source of truth).
- `held` = `SUM(seat_holds.quantity)` with `released_at IS NULL AND consumed_at IS NULL` **and the hold's order in
  `('HELD','PAYMENT_PENDING')`** (amended CR-1). Holds are consumed only at ticket issuance
  (`ticket_repository.go:152-158`), not when the order becomes PAID, so without the order-status filter a
  PAID-not-issued order would be counted in both `held` and `paid`. Expired-but-unswept holds of HELD orders still count.
- `paid` = `SUM(order_items.quantity)` of orders with `status = 'PAID'` (paid, tickets not issued yet).
- `issued` = `COUNT(tickets)` with `status IN ('ISSUED','CHECKED_IN')`.
- Each seat is therefore counted in at most one of `held`, `paid`, `issued`.
- **Single snapshot** (amended CR-1): all per-tier figures come from **one SQL statement** (CTEs per figure, joined on
  `ticket_type_id`), so an order moving HELD→PAID or PAID→ISSUED between reads cannot be counted twice.
- Tiers ordered by `price_cents, name`; empty array when the event has no tiers.
- Served from a per-event cache with TTL 2 s and single-flight. The loader runs on a detached context with a 2 s
  timeout, so one caller's cancel cannot fail the waiters. `generated_at` = the time the cache entry was filled, not the
  response time, so the age shown in the UI is honest.

Errors: 401; 403 role ≠ `ops`; 400 non-UUID; 404 event row does not exist; 503 DB error/timeout.

### 4.2 `GET /v1/organizer/events/{eventID}/sales`

200:
```json
{
  "event_id": "…",
  "generated_at": "2026-11-01T10:00:00Z",
  "currency": "VND",
  "totals": { "gross_revenue_cents": 123000000, "tickets_sold": 820, "quota": 1200,
              "paid_orders": 455, "active_holds": 48 },
  "tiers": [
    { "ticket_type_id": "…", "name": "Standard", "price_cents": 150000, "quota": 1000,
      "sold": 700, "active_holds": 40, "revenue_cents": 105000000 }
  ],
  "payment_breakdown": [ { "provider": "vnpay", "orders": 300, "amount_cents": 80000000 } ]
}
```
- Sold orders = `orders.status IN ('PAID','ISSUED')` (REFUNDING/REFUNDED/CANCELLED/EXPIRED/HELD/PAYMENT_PENDING excluded).
- `sold` / `tickets_sold` = `SUM(order_items.quantity)` of sold orders; `revenue_cents` = `SUM(quantity*unit_cents)` of sold orders.
- `active_holds` = open hold quantity exactly as in 4.1 `held` (HELD/PAYMENT_PENDING orders only, amended CR-1).
- `paid_orders` = count of sold orders.
- `payment_breakdown` = sold orders grouped by the provider of **one** payment per order: the latest `SUCCEEDED`
  payment (`DISTINCT ON (order_id) … ORDER BY order_id, created_at DESC`), so an order with two SUCCEEDED rows is not
  counted twice. Sold orders without any SUCCEEDED payment are grouped under provider `"unknown"`. Then
  `SUM(orders)` = `paid_orders` and `SUM(amount_cents)` = `gross_revenue_cents` (amount is the order revenue, not the
  payment amount). Sorted by `amount_cents` desc; provider is free text (`vnpay`, `momo`, `sandbox`, …).
- Per-tier figures, totals and the breakdown come from one snapshot: a single statement, or a
  `BEGIN ISOLATION LEVEL REPEATABLE READ READ ONLY` transaction around the two statements (tiers, breakdown).
- Same 2 s cache / timeout / detached loader / `generated_at` rule as 4.1. The cache entry also stores the event's
  `organizer_id`, and the ownership check runs on every request **before** a cached value is served (on a miss, the
  owner is read in the same snapshot). AC-17 counts these snapshot queries: at most one per event per 2 s.

Errors: 401; 403 role ≠ `organizer`; 400 non-UUID; **404 when the event does not exist or `events.organizer_id` ≠ caller**
(same "hide existence" rule as `requireOwner`, `order_handler.go:62-83`); 503.

## 5. ai-worker

### 5.1 `GET /v1/ops/ai/mode`

Requires header `X-Identity-Role: ops` (injected by the gateway); otherwise 403 JSON
`{"type":"/problems/forbidden","title":"Forbidden","status":403,"detail":"..."}`; missing `X-Identity-Id` → 401 same shape.

200:
```json
{ "mode": "NORMAL", "manual_override": null,
  "signals": { "chat_queue_lag_seconds": 2.0, "budget_used_ratio": 0.35, "gemini_circuit_open": false },
  "generated_at": "2026-11-01T10:00:00Z" }
```
Same values as `GET /admin/mode` minus `policy`. `/admin/mode` and `POST /admin/mode/{mode}` are unchanged and stay
blocked at the gateway.

## 6. Reused endpoint (no change)

- `GET /api/v1/events/public` (event service, Public) — ops event list (SCHEDULED/ON_SALE/SOLD_OUT, ≤ 100).
- `GET /api/v1/events/{id}` (Public) — war-room title/status and the 404 decision.

## 7. Web types (`apps/web/src/lib/ops-service.ts`, `lib/organizer-service.ts`) — additive

```ts
// ops-service.ts (existing fields kept)
export interface OpsWarRoomSnapshot {
  /* … existing … */
  admission: {
    ratePerSec: number; mode: "AUTO" | "MANUAL"; queueDepth: number;
    overrideRatePerSec: number | null; paused: boolean; soldOut: boolean; stockZero: boolean;
    leaderPresent: boolean;
  };
  checkout: { p99Ms: number; errorRate: number; dbPoolUsage: number; pods: number } | null;
  isSample: boolean; // always false from the real path
}
export async function setAdmitOverride(eventId: string, ratePerSec: number, reason: string): Promise<AdmitOverrideResult>;
export async function clearAdmitOverride(eventId: string, reason: string): Promise<AdmitOverrideResult>;
export interface AdmitOverrideResult { active: boolean; ratePerSec: number | null; paused: boolean; audit: "recorded" | "failed" }

// organizer-service.ts
export interface RealtimeSalesSnapshot {
  isSample: false;
  eventId: string; timestamp: string /* ISO generated_at */;
  grossRevenue: number; totalTicketsSold: number; totalQuota: number; overallFillRatePercentage: number;
  paidOrdersCount: number; activeHoldsCount: number; tierSales: TierSalesDetail[];
  postSaleReport: {
    averageOrderValue: number;                 // gross / paidOrders, 0 when no orders
    peakSalesVelocityTps: null;                // no source: UI shows "Chưa có dữ liệu"
    paymentBreakdown: { provider: string; orders: number; amount: number }[];
  };
}
```
