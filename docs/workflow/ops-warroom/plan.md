# Plan: ops-warroom (round 2, addresses plan-review.md round 1)

Inputs: `spec.md` (AC-1..AC-32, approved 2026-11-01 with D1-A, D2 apply-then-audit, D3 owner only, D4 reason required;
round-2 amendments marked inline), `contracts.md` (frozen; amended by `contracts-change-request-1.md`, approved by the
human on 2026-11-01). Base develop @ c5fb598. §8 (D1-B) is kept only for reference; the human chose D1-A.

No migrations. Dependencies: only `github.com/jackc/pgx/v5 v5.9.2` added to `services/waitingroom/go.mod`.

## 1. Work packages (exclusive file ownership) and DAG

**WP1 ∥ WP2 ∥ WP3 ∥ WP4** (all code against the frozen contracts; no WP imports another WP's code), then **WP5
integrator**. Each WP in its own worktree from the branch head after the test-designer commit. Max 4 implementers.

### WP1 — Waitingroom ops API + audit (AC-1..AC-13), Go

| File | Change |
|---|---|
| NEW `services/waitingroom/internal/app/ops_override.go` | `OpsOverride` use case: `Set(ctx, actor, eventID, rate, reason, requestID)`, `Clear(...)`. Validates (AC-9), checks room configured (404), reads previous override, applies through `OverrideStore`, then `Auditor.Record`. Apply and audit run on `context.WithoutCancel(ctx)` with their own timeouts (Redis 1 s, insert 2 s). Returns `Result{Active, Rate, Paused, AppliedAt, Audit: "recorded"|"failed"}`. Nil auditor → `ErrAuditUnavailable` checked **before** any apply (AC-12). Audit error → `Audit:"failed"`, never an error return (D2). Action `ADMIT_PAUSED` when the stored rate == 0; result built from the value read back from Redis |
| NEW `services/waitingroom/internal/app/ops_state.go` | `OpsState` DTO + `OpsStateReader` port (`QueueOpsState(ctx, eventID, now)`), `CheckoutReader` port (wraps loadsignal) |
| NEW `services/waitingroom/internal/adapter/redis/ops_state.go` | one pipeline on `wr:{ev}:*`: `SCARD lobby`, `ZCARD queue`, `ZCOUNT admitted (now +inf`, `HMGET meta admit_rate override override_rate sold_out stock_zero sale_start_ms`, `EXISTS leader`, `GET leader_epoch`; `RoomConfigured` = `sale_start_ms` present. Reuses existing `Queue.SetRateOverride/ClearRateOverride` (no edit to `queue.go`) |
| NEW `services/waitingroom/internal/adapter/postgres/audit.go` | `Auditor` on `pgxpool.Pool`: single `INSERT INTO audit_logs (actor_type, actor_id, action, subject, reason, metadata)` |
| NEW `services/waitingroom/internal/adapter/http/ops_handler.go` | `OpsHandler.Routes(mux)`: `GET /v1/ops/events/{eventID}/queue`, `PUT|DELETE /v1/ops/events/{eventID}/admit-override`; principal + role `ops` check (401/403), UUID check (400), `http.MaxBytesReader` 4 KiB, `DisallowUnknownFields`, `rate_per_sec` decoded as `json.Number`, must be an integer 0..10000 (missing / fractional → 400, CR-1), problem types per contracts §3, `Cache-Control: no-store`, ERROR log on `audit:"failed"` |
| EDIT `services/waitingroom/cmd/server/main.go` | `DATABASE_URL` unset → auditor nil (WARN). Set: `pgxpool.ParseConfig` (parse error → auditor nil, ERROR log: real misconfiguration), MaxConns 2, `ConnectTimeout` 2 s, `pgxpool.NewWithConfig` (connects lazily). Then one startup `Ping` with 2 s timeout: failure logs WARN and **keeps** the auditor, so later requests go down the D2 path (`audit:"failed"`) and recover by themselves when the DB comes back (MAJOR-2). `loadsignal.NewReader(rdb, loadsignal.DefaultKey, 3s)`; register `OpsHandler`. Readiness stays Redis-only. This logic lives in `postgresadapter.NewAuditorFromEnv(ctx, getenv, log) (app.Auditor, func())` in `internal/adapter/postgres/audit.go` (not in package `main`), so the integration test under `test/` can import it; `main.go` only calls it |
| EDIT `services/waitingroom/go.mod`, `go.sum` | add `github.com/jackc/pgx/v5 v5.9.2` (`go mod tidy`) |
| EDIT `deploy/compose/docker-compose.yml` | waitingroom: `DATABASE_URL` (same pgbouncer URL as ticketing), `depends_on: pgbouncer` (service_started) — **only the waitingroom block** |

Not touched: `handler.go`, `queue.go`, Lua scripts, controller, `admit_controller.go`, `.github/workflows/integration.yml`.

**Integration-test location rule (MAJOR-3, binding for the test designer and WP1).** Every new waitingroom test that
needs real Redis or Postgres goes under `services/waitingroom/test/<pkg>/` (new packages `test/opsapi/` and
`test/opsaudit/`), with `//go:build integration` and test names starting `TestTichHop_`, because CI runs only
`go test -tags=integration ./test/...` filtered on `TestTichHop` (`.github/workflows/integration.yml:85-91`). Redis comes
from `testx.Redis(t)`, Postgres from `testx.Postgres(t)` (repo `migrations/`, which contains `audit_logs`). These
tests use only exported APIs (`redisadapter`, `postgresadapter`, `httpadapter`, `app`), so they need no build-tag
exception. Required cases:
- AC-1/AC-2 + mutation 6: expired admitted members are not counted (real ZSET);
- AC-7: real controller + Redis, with the m1 slack (in-flight tick + 1 tick for the pause; `DecreaseCooldown + 2` ticks to resume);
- AC-10: one `audit_logs` row with the expected columns;
- AC-11/AC-12 (MAJOR-2): `DATABASE_URL=postgres://u:p@127.0.0.1:1/x?connect_timeout=1`. `NewAuditorFromEnv` returns a
  non-nil auditor; PUT returns 200 with `audit:"failed"` and Redis holds the override;
- AC-12: `DATABASE_URL` unset and `DATABASE_URL=::not a url` → 503 `audit-unavailable` and Redis untouched.

Plain unit tests with fakes stay next to the code (`internal/...`).
Ticketing integration tests may stay in `internal/adapter/postgres/` because CI runs `./...` there
(`integration.yml:35-37`); they must also carry `//go:build integration`.

### WP2 — Ticketing ops inventory + organizer sales (AC-14..AC-17), Go

| File | Change |
|---|---|
| NEW `services/ticketing/internal/port/ops.go` | `OpsReader` port: `EventExists`, `EventOwner(ctx, eventID) (organizerID string, err)`, `TierInventory(ctx, eventID) ([]TierInventoryRow, error)`, `EventSales(ctx, eventID) (SalesRows, error)` |
| NEW `services/ticketing/internal/adapter/postgres/ops_repository.go` | contracts §4 as amended by CR-1. Inventory: **one statement** (CTEs `inv`, `held`, `paid`, `issued`, `owner`) returning per-tier rows plus `organizer_id`. `held` joins `orders` and filters `o.status IN ('HELD','PAYMENT_PENDING')`. Sales: `BEGIN ISOLATION LEVEL REPEATABLE READ READ ONLY` around the tier statement and the breakdown statement (`DISTINCT ON (order_id)` latest SUCCEEDED payment; no payment → `'unknown'`). `context.WithTimeout` 2 s; UUID guard like `ListTiers` |
| NEW `services/ticketing/internal/app/ops_queries.go` | `GetOpsInventory`, `GetOrganizerSales` (ownership: not owner / not found → `domain.ErrEventNotFound`), totals computed in Go from the per-tier rows of the same snapshot |
| NEW `services/ticketing/internal/adapter/http/ops_handler.go` | routes `GET /v1/ops/events/{eventID}/inventory` (role `ops`), `GET /v1/organizer/events/{eventID}/sales` (role `organizer`). Generic per-event `ttlCache[T]`: TTL 2 s, single-flight, size cap like `availCache`. The loader runs on `context.WithoutCancel` + 2 s timeout, unlike `availCache`, which uses the first caller's ctx (`order_handler.go:255-276`). The entry stores `generatedAt` = fill time and `organizerID`; the ownership check runs on every request against the cached `organizerID` before serving (m3). `Cache-Control: no-store` |
| EDIT `services/ticketing/internal/adapter/http/handler.go` | `WithOps(...)` option + `registerOpsRoutes(mux)` call inside `Routes` (routes only registered when configured) |
| EDIT `services/ticketing/cmd/server/main.go` | wire `WithOps(app.NewGetOpsInventory(repo), app.NewGetOrganizerSales(repo))` |

### WP3 — ai-worker read + gateway routes (AC-18, AC-19)

| File | Change |
|---|---|
| EDIT `services/ai-worker/app/main.py` | `@app.get("/v1/ops/ai/mode")` next to `/admin/mode`: reads `x-identity-id` / `x-identity-role` (401/403 problem JSON), returns contracts §5.1 (shared helper with `get_mode` so values cannot drift) |
| NEW `services/ai-worker/tests/test_ops_api.py` | (test designer) |
| EDIT `services/gateway/internal/proxy/routes.go` | 6 routes of contracts §2 with a comment saying ServeMux picks by specificity, so placement does not matter (m11); nothing else changes |
| NEW `services/gateway/internal/proxy/ops_routes_test.go` | (test designer) role matrix + resolved upstream/path |

### WP4 — Web (AC-20..AC-30)

| File | Change |
|---|---|
| REWRITE `apps/web/src/lib/ops-service.ts` | keep exported helpers (`SALE_STATE_LABEL`, `saleStateLabel`, `opsErrorStatus`, `OpsServiceError`, `STALE_AFTER_MS`, `OPS_POLL_INTERVAL_MS`, `tierInventoryViolation`, `snapshotAgeMs`); real `getOpsWarRoom` (`Promise.allSettled`, mapping + error rules AC-20..22). The event detail is read with `apiFetch("/api/v1/events/{id}")` directly, **not** `getEventById`, which also calls availability for ON_SALE events (`event-service.ts:116-122`) (m8), `listOpsEvents` via `getAllPublicEvents` from `event-service`, `setAdmitOverride`, `clearAdmitOverride`, `OpsWriteError` with codes `INVALID|FORBIDDEN|NOT_FOUND|UNAVAILABLE`; remove sample builder and `__setOpsSampleOverride` |
| DELETE `apps/web/src/lib/sample-data/events.ts` | AC-28 |
| EDIT `apps/web/src/lib/organizer-service.ts` | only `RealtimeSalesSnapshot` type and `getRealtimeSales` (AC-29); everything else untouched |
| EDIT `apps/web/src/hooks/use-organizer-sales.ts` | none expected; only if the type change requires it |
| EDIT `apps/web/src/components/ops/ops-panels.tsx` | admission panel extra states, checkout-health block (AC-23); `OpsControlsPanel` moves out |
| NEW `apps/web/src/components/ops/ops-controls.tsx` | controls of AC-24/25: confirm step for pause, in-flight lock, integer validation, status/alert messages, calls `refresh` after success. After resume it shows the note "Tự động có thể hiện 0/s vài giây trước khi tăng lại" (AC-7 m1) |
| EDIT `apps/web/src/components/ops/ops-war-room.tsx`, `components/ops/index.ts` | wire controls with `snapshot` + `refresh`; sample label stays conditional on `isSample` |
| NEW `apps/web/src/app/(ops)/_lib/ops-only.tsx`; EDIT `app/(ops)/layout.tsx` | RoleGuard `["ops"]` (AC-27) |
| EDIT `app/(ops)/ops/events/page.tsx` + NEW `app/(ops)/ops/events/events-client.tsx` | client list with loading/empty/error (AC-26) |
| EDIT `app/(ops)/ops/events/[eventId]/page.tsx` | thin shell, no server fetch, `OpsWarRoom` with `initialSnapshot={null}`. A 404 renders the existing `OpsNotFoundView` path in `LiveWarRoom` (`ops-war-room.tsx:104`), never `notFound()` from client code. `[eventId]/not-found.tsx` and its test stay unchanged (m12) |
| EDIT `apps/web/src/components/organizer/sales-dashboard.tsx`, `app/(organizer)/organizer/events/[eventId]/sales/page.tsx` (metadata text only) | AC-30 |
| EDIT `tests/e2e/fixtures/api-mock.ts`, `tests/e2e/specs/ops-war-room.spec.ts`; NEW `tests/e2e/specs/organizer-sales.spec.ts` | mocks for the 6 endpoints + public list + ops/organizer sessions; ops spec rewritten per TCR. The new organizer-sales spec (owner WP4) covers real figures, no sample label, and the peak-TPS "Chưa có dữ liệu" text (AC-30/31, m8) |

Does not touch: `lib/api-client.ts`, `lib/session.ts`, `lib/event-service.ts`, `hooks/use-ops-war-room.ts` (its test
does change, see §7), `lib/i18n/messages.ts`.

### WP5 — Integrator

Merge WP1 → WP2 → WP3 → WP4; full gate after each; resolves only mechanical conflicts (none expected: disjoint files).
Runs compose smoke: `docker compose up`, log in as ops (seed user), PUT pause on a seeded event, observe
`waitingroom_admit_adjust_total{action="override"}` increasing and `admitted` not growing, DELETE, observe resume;
`SELECT * FROM audit_logs WHERE subject LIKE 'event:%' ORDER BY id DESC LIMIT 3`.

## 2. Order and checkpoints

1. Spec + D1–D4 approved by human → test designer writes failing tests per AC (Go unit + integration-tagged,
   pytest, Vitest, Playwright) + test change request → red check (new tests fail for the right reason, untouched suites
   green) → freeze commit.
2. Plan review (this file) → PASS.
3. WP1–WP4 in parallel → fast gate per WP (lint/vet/typecheck + the WP's tests + tamper check of frozen tests).
4. WP5 merge + full gate → 3 parallel reviewers (correctness, security, challenge on a different model) → fix rounds ≤ 3
   → verification (mutation §6, rollback §5, compose smoke) → human sign-off → PR to develop (merge commit).

## 3. Gates (commands)

Per module, the same way CI does it (`ci.yml:94-128,189-190`, `integration.yml:35-37,82-91`) (m7):
```
gofmt -l services/waitingroom services/ticketing services/gateway   # empty
for m in waitingroom ticketing gateway; do (cd services/$m && go vet ./... && go test -race ./... && golangci-lint run ./...); done
(cd services/waitingroom && go vet -tags=integration ./... && REQUIRE_PASS=1 bash ../../.github/scripts/run-and-report-tests.sh TestTichHop go test -v -tags=integration ./test/... -count=1)   # R2-m5: exit code of the run, not of grep
(cd services/ticketing && go test -tags=integration ./... -count=1)
cd services/ai-worker && uv run ruff check . && uv run pytest -q
cd apps/web && npm run lint && npm run typecheck && npm test && npm run build
cd tests/e2e && npx playwright test
```

## 4. Risks

| Risk | Mitigation |
|---|---|
| A buggy write stops a live sale (pause stuck) | DELETE always allowed and idempotent; controller logic unchanged and already tested (EV-147 tests); UI shows paused state in text; rollback inspects overrides with on-call before clearing (§5) |
| Privilege escalation to the control plane | gateway roles + service-side role re-check (both tested, both in mutation list). In compose, waitingroom and ticketing bind to 127.0.0.1 only. **ai-worker publishes `8090:8090` on all interfaces** (`docker-compose.yml:245`), so a host-network client can spoof `X-Identity-Role: ops` and read `/v1/ops/ai/mode`. That endpoint is read-only and low impact, and `/admin/*` is already exposed the same way today. Accepted for this PR and recorded as security debt; binding ai-worker to 127.0.0.1 is a separate compose change (m6) |
| Override applied but not audited | D2 policy: response `audit:"failed"` + ERROR log. Only an unset/unparsable `DATABASE_URL` fails closed (AC-12); compose sets `DATABASE_URL` |
| Waitingroom startup/readiness coupled to Postgres | lazy pool; a startup ping failure keeps the auditor (D2 path, self-healing); readiness stays Redis-only; test with an unreachable DB URL (MAJOR-2) |
| Ops/sales queries load Postgres during a hot sale | 2 s per-event cache + single-flight + 2 s statement timeout; ≤ 0.5 snapshot queries/s/event; index left as debt |
| False "Lệch tồn kho" alert from double counting | `held` limited to HELD/PAYMENT_PENDING orders + single snapshot (CR-1); fixture with a PAID-not-issued order that still has an open hold; mutation 13 (MAJOR-1) |
| Revenue mis-stated (refunds, pending) | AC-15 test fixtures with every order status; mutation 8 |
| Cross-tenant sales leak | ownership before cache; 404 for non-owner; mutations 7, 16 |
| Sample label removed where data is fake | only real path sets `isSample:false`; peak TPS shows "Chưa có dữ liệu"; test asserts no fabricated number |
| Tokens only in browser → SSR cannot fetch | ops pages become client-side (AC-26); `npm run build` with `NEXT_PUBLIC_API_BASE` unset in the gate |
| Teammates touching the same files | `git merge origin/develop` before PR + full gate rerun |
| ServeMux pattern conflict panics gateway at start | `/v1/ops/` and `/v1/organizer/` share no wildcard prefix with existing routes; gateway `New` test constructs the full table |

## 5. Rollback (checked during verification)

After a revert, the unchanged controller keeps honouring any override still in Redis, and no API is left to change
it. So step 0 inspects first and clears nothing blindly (m9):
```
# 0a. List active overrides (read-only)
docker compose -f deploy/compose/docker-compose.yml exec -T redis sh -c \
  'redis-cli --scan --pattern "wr:{*}:meta" | while read k; do v=$(redis-cli HMGET "$k" override override_rate | tr "\n" " "); case "$v" in 1*) echo "$k $v";; esac; done'
# 0b. For each listed event, the on-call Ops decides: keep (an emergency pause protecting a live incident stays,
#     and the rollback waits) or clear. Clear one event at a time, only after confirmation:
docker compose -f deploy/compose/docker-compose.yml exec -T redis redis-cli HDEL 'wr:{<eventID>}:meta' override override_rate
# 0c. Record each manual clear:
docker compose -f deploy/compose/docker-compose.yml exec -T postgres psql -U eventflow -d eventflow -c \
  "INSERT INTO audit_logs (actor_type, actor_id, action, subject, reason) VALUES ('system', '<on-call id>', 'ADMIT_OVERRIDE_CLEARED', 'event:<eventID>', 'rollback of ops-warroom')"
# 1. Revert
git switch -c rollback-check develop && git revert -m 1 --no-edit <merge-commit>
go test ./services/waitingroom/... ./services/ticketing/... ./services/gateway/...
cd services/ai-worker && uv run pytest -q
cd apps/web && npm test && npm run build && cd ../../tests/e2e && npx playwright test
```
Data: `audit_logs` rows written by this feature remain (append-only, harmless). Compose `DATABASE_URL` for waitingroom is
ignored by the old binary. Verification checks that the reverted tree equals develop @ base for all touched paths.

## 6. Mutation test (threshold 17/17 killed)

Each mutation must make at least one frozen test fail:

1. Waitingroom ops handler: role check removed (`p.Role != "ops"` -> always pass): buyer gets 200.
2. Waitingroom validation: upper bound removed (10001 accepted).
3. Waitingroom validation: lower bound relaxed (`rate < 0` -> `rate < -1`).
4. `OpsOverride`: audit call skipped on success (no `audit_logs` row / fake auditor not called).
5. `OpsOverride`: action not switched to `ADMIT_PAUSED` for rate 0.
6. Redis ops state: `ZCOUNT admitted (now +inf` replaced by `ZCARD admitted` (expired members counted).
7. Ticketing sales: ownership check removed (other organizer gets 200).
8. Ticketing sales: `REFUNDED`/`REFUNDING` orders counted as sold.
9. Ticketing inventory: `released_at IS NULL` dropped from `held` (R2-m1: the `consumed_at` variant is equivalent under
   the CR-1 order-status filter; this one is killed by the released-hold fixture).
10. Gateway: `Roles: []string{"ops"}` removed from `GET /v1/ops/events/{eventID}/queue`.
11. Web `ops-service`: a 403 from one source mapped to `unavailable` instead of `OpsServiceError(403)`.
12. Web `ops-service`: `isSample` left `true` on the real path (sample label shown over real data).
13. Ticketing inventory/sales: order-status filter dropped from `held` / `active_holds` (MAJOR-1; killed by the
    PAID-not-issued-with-open-hold fixture).
14. `OpsOverride`: nil-auditor check moved after the Redis apply (AC-12 fail-closed order).
15. `OpsOverride`: audit failure turned into an error return / 5xx (D2).
16. Gateway: `Roles: []string{"organizer"}` removed from the sales route.
17. Web `ops-controls`: client-side validation of rate/reason bypassed (request sent with rate 10001 or empty reason).

Reported but outside the threshold: ai-worker role check removed.

## 7. Expected test-change-request files

Existing tests asserting sample behaviour or server-side fetching, to be replaced (never silently deleted):
`apps/web/src/lib/ops-service.test.ts`, `components/ops/ops-war-room.test.tsx` (sample label, disabled EVF-60 buttons,
m-9 aria-describedby), `hooks/use-ops-war-room.test.tsx` (it imports `__setOpsSampleOverride` at lines 20, 65, 71: must be replaced),
`app/(ops)/ops/events/page.test.tsx`, `app/(ops)/ops/events/[eventId]/page.test.tsx`, `app/(ops)/layout.test.tsx`
(RoleGuard), `lib/organizer-service.test.ts` (getRealtimeSales part), `hooks/use-organizer-sales.test.tsx`,
`components/organizer/sales-dashboard.test.tsx`, `app/(organizer)/organizer/events/[eventId]/sales/page.test.tsx`,
`src/real-api.contract.test.ts` (sample-data assertions → "no production module imports sample data"),
`tests/e2e/specs/ops-war-room.spec.ts`, `tests/e2e/specs/i18n.spec.ts` (`/ops/events` now needs an ops session mock).
New (not a change request): `tests/e2e/specs/organizer-sales.spec.ts`. Final list comes from the test designer's request.

## 8. If the human picks D1-B (writes in ticketing)

WP1 keeps only the read endpoint (`ops_state.go`, read part of `ops_handler.go`, `main.go` loadsignal wiring) and drops
pgx, `audit.go`, compose change. WP2 adds `internal/adapter/redis/admit_override.go` (HSET/HDEL on `wr:{ev}:meta`, key
format documented as shared contract like `sale_state.go`), `internal/app/ops_override.go`, audit insert in
`ops_repository.go`, and the two write routes; gateway PUT/DELETE upstream becomes Ticketing and ticketing's write routes
do **not** take the Idempotency middleware (idempotent by method; recorded as an explicit BR-O5 exception).

## Round 2 — resolution of plan-review.md round 1

| Finding | Resolution |
|---|---|
| MAJOR-1 `held` and `paid` double counting | `contracts-change-request-1.md` (approved 2026-11-01) amends contracts §4.1/§4.2: `held`/`active_holds` = open holds whose order is `HELD`/`PAYMENT_PENDING`. Inventory is one SQL statement (CTEs); sales is one `REPEATABLE READ READ ONLY` tx. Spec AC-14/AC-15 amended; fixture "PAID-not-issued order with open hold" required; mutation 13 added; risk row added; WP2 rows rewritten |
| MAJOR-2 startup DB blip disables pause | WP1 `main.go` row: auditor nil only when `DATABASE_URL` is unset or unparsable; `pgxpool.NewWithConfig` connects lazily; a startup ping failure is WARN and keeps the auditor; runtime failures → apply + `audit:"failed"`. Spec AC-12 and contracts §3.2 amended. Integration test with `127.0.0.1:1` (unreachable) → 200 `audit:"failed"`, Redis applied; mutations 14/15 |
| MAJOR-3 integration tests not run by CI | Binding location rule in WP1: `services/waitingroom/test/<pkg>/`, `//go:build integration`, `TestTichHop_` prefix, `testx.Redis`/`testx.Postgres`. `integration.yml` not touched. Ticketing integration tests stay under `internal/` (CI runs `./...`) |
| m1 AC-7 wording vs controller | AC-7 amended: pause asserted after in-flight tick + 1; resume within `DecreaseCooldown + 2` ticks with rate > 0 (first healthy tick = StepUp). Edge-case line fixed; UI note after resume (WP4) |
| m2 audit context | Apply and insert on `context.WithoutCancel` with their own timeouts (contracts §3.4, WP1 use case row) |
| m3 cache semantics | AC-17 counts figure-snapshot queries; `organizer_id` cached in the entry and checked before serving; detached loader ctx; `generated_at` = fill time (contracts §4, WP2 row) |
| m4 `payment_breakdown` duplicates | `DISTINCT ON (order_id)` latest SUCCEEDED payment; no payment → provider `"unknown"`; breakdown totals reconcile with `paid_orders`/`gross_revenue_cents` (contracts §4.2, AC-15) |
| m5 rate precision | `rate_per_sec` must be an integer 0..10000 (contracts §3.2, AC-6/AC-9). Response read back from the stored integer; `ADMIT_PAUSED` on stored 0 |
| m6 ai-worker port claim | Risk row corrected: ai-worker publishes 8090 on all interfaces. Accepted as recorded security debt (read-only endpoint); bind-to-loopback is out of scope |
| m7 gate commands | §3 runs vet/test/lint per module and waitingroom integration via `./test/...` with the `TestTichHop_` count, like CI |
| m8 test-change list | `hooks/use-ops-war-room.test.tsx` is a definite TCR entry; organizer Playwright check = NEW `tests/e2e/specs/organizer-sales.spec.ts` owned by WP4; war room reads `/api/v1/events/{id}` via `apiFetch`, not `getEventById` |
| m9 rollback step 0 | Inspect active overrides first, on-call decides per event, exact `docker compose exec` commands, manual clear recorded in `audit_logs` (`actor_type=system`) |
| m10 mutation gaps | Added mutations 13–17 (held filter, nil-auditor order, audit failure → 5xx, organizer role on sales route, client validation bypass); threshold 17/17 |
| m11 route order wording | WP3 row: comment states ServeMux picks by specificity; placement irrelevant |
| m12 client 404 | War room 404 uses existing `OpsNotFoundView` in `LiveWarRoom`; no client `notFound()`; `not-found.test.tsx` unchanged |

## Implementer notes — plan-review round 2 minors

| Minor | Where it landed |
|---|---|
| R2-m1 mutation 9 | §6 item 9 now drops `released_at IS NULL` (fixture: released hold of a HELD order, `ops_repository_integration_test.go`) |
| R2-m2 inventory statement | `ops_repository.go` `TierInventory`: `FROM events e LEFT JOIN tt`, figure CTEs `GROUP BY ticket_type_id` before the join; event without tiers → 1 row with NULL tier → `[]`; no row → `ErrEventNotFound` |
| R2-m3 breakdown amount / tie-break | `EventSales`: amount = per-order `SUM(quantity*unit_cents)`; `DISTINCT ON (order_id) … ORDER BY order_id, created_at DESC, id DESC`. Orders without a SUCCEEDED payment are not returned by the repo (the frozen repo test expects 2 rows); `app.GetOrganizerSales` adds the `"unknown"` remainder so totals reconcile (contracts §4.2) |
| R2-m4 typed nil / empty URL | `postgresadapter.NewAuditorFromEnv` returns a plain `nil` interface; `DATABASE_URL` empty or blank = unset. Unit test `internal/adapter/postgres/audit_test.go` (no Docker) + integration `test/opsaudit` |
| R2-m5 gate pipe | §3 waitingroom integration line uses `REQUIRE_PASS=1 .github/scripts/run-and-report-tests.sh` |
| R2-m6 TestMain | `test/opsapi/main_test.go`, `test/opsaudit/main_test.go` call `testx.Shutdown()` |
| R2-m7 not-found caching | ticketing `ttlCache` caches `ErrEventNotFound` for the same 2 s TTL; other errors are not cached (`TestOpsCache_AC17_ErrorsAreNotCached`) |
| R2-m8 stale status text | plan.md header, Round 2 table and CR-1 status line now say approved |

