# Plan review: ops-warroom (v3 step 2b, High-risk)

Reviewer: plan reviewer, clean context. Inputs: `spec.md` (approved, D1-A / D2 apply-then-audit / D3 owner only /
D4 reason required), `contracts.md`, `plan.md`. Read-only checks against the worktree at 81d742c (base c5fb598).
Untracked test-designer files were present (`services/ai-worker/tests/test_ops_api.py`,
`services/gateway/internal/proxy/ops_routes_test.go`); I did not review them.

## Verdict: **FAIL** (3 major, 0 blocker)

The work-package split, the DAG and the route/role design are sound. Three issues must be fixed in the plan (one
needs a contracts change request) before implementation starts.

## What was checked and holds

| Area | Result | Evidence |
|---|---|---|
| AC coverage by WP | AC-1..13 → WP1, AC-14..17 → WP2, AC-18/19 → WP3, AC-20..30 → WP4, AC-31/32 → gates/WP5. No AC without an owner | plan.md:14-64 |
| Exclusive ownership / DAG | File sets are disjoint; no WP imports another WP's code; compose edit limited to the waitingroom block | plan.md:18-67 |
| Gateway route precedence | New patterns share no wildcard prefix with existing ones (`/v1/ops/...`, `/v1/organizer/...`), so ServeMux will not panic; other methods fall through to the `/` catch-all → 404 | routes.go:33-80, gateway.go:75-84 |
| Gateway role check | `Roles` checked before rate limit and before forwarding → 403 `/problems/forbidden`; identity headers stripped and re-injected | gateway.go:113-127, 140-143 |
| Service-side re-check | `authx.PrincipalFromHeaders` is available; pattern exists in ticketing (`requireOwner` 404-hide) | principal.go:75, order_handler.go:62-83 |
| CORS for PUT/DELETE | Allowed methods include PUT, DELETE | gateway.go:272 |
| Redis keys | `lobby` SET, `queue` ZSET, `admitted` ZSET scored by expire_at_ms, `meta.override/override_rate` (×1000), `leader`/`leader_epoch` match the code | join.lua:10,61; admit.lua:13-14,53; queue.go:174-194; cmd/controller/main.go:164-166 |
| "Override picked up within one tick" | True: `syncOverride` runs at the top of every tick (admit_controller.go:255) and the controller is wired with `WithStateStore(fq)` (cmd/controller/main.go:225); clamp to `[0, MaxRate]` (admit_controller.go:237). Worst case: a tick already past `syncOverride` admits one more batch, so "≤ 2 s" holds (see m1) | |
| DSN | Same pgbouncer URL as ticketing (transaction pooling); ticketing already runs pgx defaults through it, so the waitingroom pool will work the same way | docker-compose.yml:39,186 |
| request_id for audit | `httpx.RequestID` keeps the gateway's `X-Request-Id` | libs/go/httpx/middleware.go:48-58 |
| Web SSR | Ops pages become client-side; `RoleGuard` + `organizer-only.tsx` pattern exists; `Role` includes `ops` | RoleGuard.tsx, organizer-only.tsx, use-auth.ts:101 |
| Sample removal | Only `ops-service.ts` imports `lib/sample-data/events.ts` | real-api.contract.test.ts:45-48 |

## Findings

### MAJOR-1: `held` and `paid` count the same seats (needs a contracts change request)

- Evidence: holds are consumed only when tickets are issued (`services/ticketing/internal/adapter/postgres/ticket_repository.go:152-158`),
  not when the order becomes PAID (`services/payment/internal/adapter/postgres/repository.go:294-297` updates `orders`
  only). So a PAID-but-not-issued order still has an open `seat_holds` row. contracts.md:147-149 defines
  `held` = all open holds and `paid` = quantity of PAID orders, so the order counts in both.
- Impact: `held + paid + issued` goes above the real number. Near sold-out, `tierInventoryViolation` returns
  `OVER_QUOTA` (`apps/web/src/lib/ops-service.ts:109`) and the war room shows a false "Lệch tồn kho" alert at the
  moment Ops is watching. The funnel `holding` (AC-20) and the organizer `active_holds` (contracts.md:176) are too
  high for the same reason.
- Also, "one statement per figure group" (plan.md:34) reads at different snapshots. An order moving HELD→PAID or
  PAID→ISSUED between statements is counted twice, even for a moment.
- Fix: file a change request against contracts §4.1/§4.2: `held` / `active_holds` = open holds whose order is in
  `('HELD','PAYMENT_PENDING')` (join `orders`). Compute all per-tier figures in one SQL statement (CTEs), or in one
  `REPEATABLE READ READ ONLY` transaction. Add test fixtures for a PAID-not-issued order that still has an open hold
  (AC-14/AC-15), plus a mutation: "drop the order-status filter on held".

### MAJOR-2: A Postgres blip at startup turns off emergency pause until restart

- Evidence: plan.md:23 says "failure to connect → log WARN, auditor nil, service still starts". With AC-12, a nil
  auditor makes both write endpoints return 503 `/problems/audit-unavailable`.
- Impact: if waitingroom starts while pgbouncer/Postgres is unreachable (compose start order, failover, a DB
  incident that caused a restart), PUT/DELETE stay at 503 for the life of the process. Ops cannot pause the sale.
  This contradicts D2 ("an emergency pause must not be blocked by an overloaded Postgres") and risk row 3
  (plan.md:105).
- Fix: the auditor is nil **only** when `DATABASE_URL` is unset or cannot be parsed (a real misconfiguration). Build
  the pool with `pgxpool.NewWithConfig`, which connects lazily. A failed startup `Ping` logs WARN but keeps the
  auditor. Runtime connect or insert failures go down the D2 path (`audit:"failed"` + ERROR log). Add a test:
  `DATABASE_URL` points to an unreachable host → PUT returns 200 with `audit:"failed"` and Redis is applied.

### MAJOR-3: New waitingroom integration tests would not run in CI

- Evidence: CI runs waitingroom integration tests only under `./test/...`, and counts only names matching
  `TestTichHop` (`.github/workflows/integration.yml:82-91`). The plan does not say where the new integration tests go.
  Its gate runs `./services/waitingroom/...` locally (plan.md:92). Neither the WPs nor the test designer own
  `integration.yml`.
- Impact: the tests that kill mutation 6 (ZCOUNT vs ZCARD on real Redis), cover AC-7 (real controller + Redis) and
  cover AC-10/11 (`audit_logs` row on real Postgres) pass once in this PR's local gate. They are never run by CI
  again, which leaves High-risk behaviour without a regression guard.
- Fix: in the plan / test-designer brief, require waitingroom integration tests to live under
  `services/waitingroom/test/<pkg>/` with `//go:build integration` and the `TestTichHop_` prefix. Postgres comes from
  `testx.Postgres(t, testx.WithMigrations(...))`. Alternatively, give `integration.yml` to WP1 with an explicit edit.

### Minor

- **m1 AC-7 / edge-case wording vs. controller behaviour.** After DELETE, the controller does not always "resume
  from MinRate":
  - leftover `cooldown` keeps the rate at 0 for up to `DecreaseCooldown` (5) ticks (admit_controller.go:324-327);
  - after that, a healthy tick gives `0 + StepUp` (10), not MinRate (admit_controller.go:331-343).

  Also, the tick already running when the PUT lands may admit one more batch. The AC-7 test should assert "no growth
  of `admitted` after the in-flight tick + 1" and "admission resumes with rate ≥ MinRate within `DecreaseCooldown + 2`
  ticks" (or set `DecreaseCooldown = 0` in the test config). Otherwise the frozen test is flaky or wrong. Ops should
  also be told that an AUTO rate of 0 can show for a few seconds after clearing.
- **m2 Audit context.** Run the audit insert on `context.WithoutCancel(r.Context())` + 2 s timeout, so a client
  disconnect after the Redis apply does not cancel the audit. Consider the same for the Redis apply.
- **m3 Cache semantics (AC-17).** "At most one set of DB queries" conflicts with "ownership checked before the
  cache" (one owner lookup per request). State that AC-17 counts the figure queries only, or cache `organizer_id` per
  event inside the same TTL entry (still checked before serving). Also:
  - `generated_at` = cache fill time, not response time, so the age shown is honest;
  - the single-flight loader should use a detached context with the 2 s timeout, so one caller's cancel does not fail
    the waiters (`availCache` uses the first caller's ctx, order_handler.go:255-276).
- **m4 `payment_breakdown` duplicates.** Nothing stops an order from having two `SUCCEEDED` payments (only
  `uq_provider_txn`, migrations/0001_core.sql:143), so a plain join double-counts. Use one payment per order
  (`DISTINCT ON (order_id)` latest). Say how sold orders without a SUCCEEDED payment are shown (excluded, or provider
  `"unknown"`), so the totals reconcile.
- **m5 Rate precision.** The API accepts any float. `rate < 0.001` is stored as 0 (queue.go:188). `rate < 0.5` gives
  `batch = 0` (admit_controller.go:279), which admits no one while `paused=false`. Build the response from the stored
  integer (`override_rate/1000`) and use `rate == 0` for `ADMIT_PAUSED`. Accepting integers only (as the UI sends)
  would also be fine.
- **m6 Risk-table claim.** "Services only reachable via loopback in compose" (plan.md:104) is false for ai-worker:
  `ports: ["8090:8090"]` (docker-compose.yml:245). Anyone on the host network can spoof `X-Identity-Role: ops` and
  read `/v1/ops/ai/mode` (read-only, low impact; `/admin/*` is already exposed the same way). Correct the claim, or
  bind to 127.0.0.1 (a compose change outside the current WP1 scope).
- **m7 Gate commands.** CI runs `go vet`/`go test`/golangci-lint per module (ci.yml:94-128, 189-190) and waitingroom
  integration only under `./test/...`. Run the plan §3 gates the same way. The root-level multi-module
  `golangci-lint run ./services/...` may not behave like CI.
- **m8 Test-change list.**
  - `hooks/use-ops-war-room.test.tsx` *does* import `__setOpsSampleOverride` (lines 20, 65, 71). It must be in the
    TCR, not "only if".
  - Name the file and owner for the organizer "sales check" in Playwright (AC-31): a new spec under WP4, or an
    extension of an existing one.
  - Say that the war room calls `GET /api/v1/events/{id}` through `apiFetch` directly, not through `getEventById`,
    which adds an availability call for ON_SALE events (event-service.ts:116-122).
- **m9 Rollback step 0.** Clearing every override without looking also lifts an emergency pause that is protecting
  an ongoing incident. First list active overrides (`--scan` + `HMGET … override override_rate`) and get on-call
  confirmation. Give the exact invocation (`docker compose exec redis redis-cli …`). Record the manual clear (e.g. an
  `audit_logs` row with `actor_type=system`).
- **m10 Mutation list gaps (optional additions).**
  - nil-auditor check moved after the apply (AC-12 fail-closed order);
  - audit failure turned into a 5xx / rollback of the apply (D2);
  - `Roles: ["organizer"]` removed from the sales route;
  - web client-side validation of rate/reason bypassed (AC-24);
  - the MAJOR-1 held filter.
- **m11** "Placed before the generic `/v1/...` entries" (plan.md:46): ServeMux picks by specificity, not order. The
  comment is harmless but should not suggest that order matters.
- **m12 Client 404.** `LiveWarRoom` already renders `OpsNotFoundView` when there is no snapshot and the error is 404
  (ops-war-room.tsx:104). Pick that path and drop "`notFound()` from client" (plan.md:62). `notFound()` thrown outside
  render does nothing, and `not-found.test.tsx` stays valid.

## Required before re-review

1. Contracts change request for MAJOR-1 (held/active_holds definition + single snapshot), approved by the human,
   with the plan/test list updated.
2. plan.md WP1 `main.go` row rewritten per MAJOR-2, plus the new test.
3. Location rule (or `integration.yml` ownership) for waitingroom integration tests per MAJOR-3.

Minor items can be folded into the same revision or recorded as accepted.
