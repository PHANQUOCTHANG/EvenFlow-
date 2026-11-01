# Plan: ops-warroom (round 1)

Inputs: `spec.md` (AC-1..AC-32, decisions D1–D4 pending human approval), `contracts.md` (frozen). Base develop @ c5fb598.
This plan assumes the recommended answers (D1-A, D2 apply-then-audit, D3 owner only, D4 reason required). If the human
picks D1-B, WP1's write part moves to WP2 (files listed in §8).

No migrations. Dependencies: only `github.com/jackc/pgx/v5 v5.9.2` added to `services/waitingroom/go.mod`.

## 1. Work packages (exclusive file ownership) and DAG

**WP1 ∥ WP2 ∥ WP3 ∥ WP4** (all code against the frozen contracts; no WP imports another WP's code), then **WP5
integrator**. Each WP in its own worktree from the branch head after the test-designer commit. Max 4 implementers.

### WP1 — Waitingroom ops API + audit (AC-1..AC-13), Go

| File | Change |
|---|---|
| NEW `services/waitingroom/internal/app/ops_override.go` | `OpsOverride` use case: `Set(ctx, actor, eventID, rate, reason, requestID)`, `Clear(...)`. Validates (AC-9), checks room configured (404), reads previous override, applies through `OverrideStore`, then `Auditor.Record` with 2 s timeout; returns `Result{Active, Rate, Paused, AppliedAt, Audit: "recorded"|"failed"}`. Nil auditor → `ErrAuditUnavailable` before any apply (AC-12). Action name `ADMIT_PAUSED` when rate == 0 |
| NEW `services/waitingroom/internal/app/ops_state.go` | `OpsState` DTO + `OpsStateReader` port (`QueueOpsState(ctx, eventID, now)`), `CheckoutReader` port (wraps loadsignal) |
| NEW `services/waitingroom/internal/adapter/redis/ops_state.go` | one pipeline on `wr:{ev}:*`: `SCARD lobby`, `ZCARD queue`, `ZCOUNT admitted (now +inf`, `HMGET meta admit_rate override override_rate sold_out stock_zero sale_start_ms`, `EXISTS leader`, `GET leader_epoch`; `RoomConfigured` = `sale_start_ms` present. Reuses existing `Queue.SetRateOverride/ClearRateOverride` (no edit to `queue.go`) |
| NEW `services/waitingroom/internal/adapter/postgres/audit.go` | `Auditor` on `pgxpool.Pool`: single `INSERT INTO audit_logs (actor_type, actor_id, action, subject, reason, metadata)` |
| NEW `services/waitingroom/internal/adapter/http/ops_handler.go` | `OpsHandler.Routes(mux)`: `GET /v1/ops/events/{eventID}/queue`, `PUT|DELETE /v1/ops/events/{eventID}/admit-override`; principal + role `ops` check (401/403), UUID check (400), `http.MaxBytesReader` 4 KiB, `DisallowUnknownFields`, `rate_per_sec` as `*float64` (missing → 400), problem types per contracts §3, `Cache-Control: no-store`, ERROR log on `audit:"failed"` |
| EDIT `services/waitingroom/cmd/server/main.go` | optional pgx pool when `DATABASE_URL` set (MaxConns 2, 2 s connect timeout; failure to connect → log WARN, auditor nil, service still starts); `loadsignal.NewReader(rdb, loadsignal.DefaultKey, 3s)`; register `OpsHandler`. Readiness stays Redis-only |
| EDIT `services/waitingroom/go.mod`, `go.sum` | add `github.com/jackc/pgx/v5 v5.9.2` (`go mod tidy`) |
| EDIT `deploy/compose/docker-compose.yml` | waitingroom: `DATABASE_URL` (same pgbouncer URL as ticketing), `depends_on: pgbouncer` (service_started) — **only the waitingroom block** |

Not touched: `handler.go`, `queue.go`, Lua scripts, controller, `admit_controller.go`.

### WP2 — Ticketing ops inventory + organizer sales (AC-14..AC-17), Go

| File | Change |
|---|---|
| NEW `services/ticketing/internal/port/ops.go` | `OpsReader` port: `EventExists`, `EventOwner(ctx, eventID) (organizerID string, err)`, `TierInventory(ctx, eventID) ([]TierInventoryRow, error)`, `EventSales(ctx, eventID) (SalesRows, error)` |
| NEW `services/ticketing/internal/adapter/postgres/ops_repository.go` | the queries of contracts §4 (one statement per figure group, `context.WithTimeout` 2 s), UUID guard like `ListTiers` |
| NEW `services/ticketing/internal/app/ops_queries.go` | `GetOpsInventory`, `GetOrganizerSales` (ownership: not owner / not found → `domain.ErrEventNotFound`), totals computed in Go from per-tier rows |
| NEW `services/ticketing/internal/adapter/http/ops_handler.go` | routes `GET /v1/ops/events/{eventID}/inventory` (role `ops`), `GET /v1/organizer/events/{eventID}/sales` (role `organizer`, ownership checked **before** the cache); generic per-event `ttlCache[T]` (TTL 2 s, single-flight, size cap like `availCache`), `Cache-Control: no-store` |
| EDIT `services/ticketing/internal/adapter/http/handler.go` | `WithOps(...)` option + `registerOpsRoutes(mux)` call inside `Routes` (routes only registered when configured) |
| EDIT `services/ticketing/cmd/server/main.go` | wire `WithOps(app.NewGetOpsInventory(repo), app.NewGetOrganizerSales(repo))` |

### WP3 — ai-worker read + gateway routes (AC-18, AC-19)

| File | Change |
|---|---|
| EDIT `services/ai-worker/app/main.py` | `@app.get("/v1/ops/ai/mode")` next to `/admin/mode`: reads `x-identity-id` / `x-identity-role` (401/403 problem JSON), returns contracts §5.1 (shared helper with `get_mode` so values cannot drift) |
| NEW `services/ai-worker/tests/test_ops_api.py` | (test designer) |
| EDIT `services/gateway/internal/proxy/routes.go` | 6 routes of contracts §2, placed before the generic `/v1/...` entries with a comment; nothing else changes |
| NEW `services/gateway/internal/proxy/ops_routes_test.go` | (test designer) role matrix + resolved upstream/path |

### WP4 — Web (AC-20..AC-30)

| File | Change |
|---|---|
| REWRITE `apps/web/src/lib/ops-service.ts` | keep exported helpers (`SALE_STATE_LABEL`, `saleStateLabel`, `opsErrorStatus`, `OpsServiceError`, `STALE_AFTER_MS`, `OPS_POLL_INTERVAL_MS`, `tierInventoryViolation`, `snapshotAgeMs`); real `getOpsWarRoom` (`Promise.allSettled`, mapping + error rules AC-20..22), `listOpsEvents` via `getAllPublicEvents` from `event-service`, `setAdmitOverride`, `clearAdmitOverride`, `OpsWriteError` with codes `INVALID|FORBIDDEN|NOT_FOUND|UNAVAILABLE`; remove sample builder and `__setOpsSampleOverride` |
| DELETE `apps/web/src/lib/sample-data/events.ts` | AC-28 |
| EDIT `apps/web/src/lib/organizer-service.ts` | only `RealtimeSalesSnapshot` type and `getRealtimeSales` (AC-29); everything else untouched |
| EDIT `apps/web/src/hooks/use-organizer-sales.ts` | none expected; only if the type change requires it |
| EDIT `apps/web/src/components/ops/ops-panels.tsx` | admission panel extra states, checkout-health block (AC-23); `OpsControlsPanel` moves out |
| NEW `apps/web/src/components/ops/ops-controls.tsx` | controls of AC-24/25 (confirm step for pause, in-flight lock, validation, status/alert messages, calls `refresh` after success) |
| EDIT `apps/web/src/components/ops/ops-war-room.tsx`, `components/ops/index.ts` | wire controls with `snapshot` + `refresh`; sample label stays conditional on `isSample` |
| NEW `apps/web/src/app/(ops)/_lib/ops-only.tsx`; EDIT `app/(ops)/layout.tsx` | RoleGuard `["ops"]` (AC-27) |
| EDIT `app/(ops)/ops/events/page.tsx` + NEW `app/(ops)/ops/events/events-client.tsx` | client list with loading/empty/error (AC-26) |
| EDIT `app/(ops)/ops/events/[eventId]/page.tsx` | thin shell, no server fetch, `OpsWarRoom` with `initialSnapshot={null}`; 404 handled client-side (`notFound()` from client or not-found panel) |
| EDIT `apps/web/src/components/organizer/sales-dashboard.tsx`, `app/(organizer)/organizer/events/[eventId]/sales/page.tsx` (metadata text only) | AC-30 |
| EDIT `tests/e2e/fixtures/api-mock.ts`, `tests/e2e/specs/ops-war-room.spec.ts` | mocks for the 6 endpoints + public list + ops/organizer login; spec rewritten per TCR (AC-31) |

Does not touch: `lib/api-client.ts`, `lib/session.ts`, `lib/event-service.ts`, `hooks/use-ops-war-room.ts` (unless a
test proves a gap; then it is listed in a change request), `lib/i18n/messages.ts`.

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

```
gofmt -l services/waitingroom services/ticketing services/gateway   # empty
go vet ./services/waitingroom/... ./services/ticketing/... ./services/gateway/...
go test -race ./services/waitingroom/... ./services/ticketing/... ./services/gateway/...
go test -race -tags integration ./services/waitingroom/... ./services/ticketing/internal/adapter/postgres/...
golangci-lint run ./services/waitingroom/... ./services/ticketing/... ./services/gateway/...
cd services/ai-worker && uv run ruff check . && uv run pytest -q
cd apps/web && npm run lint && npm run typecheck && npm test && npm run build
cd tests/e2e && npx playwright test
```

## 4. Risks

| Risk | Mitigation |
|---|---|
| A buggy write stops a live sale (pause stuck) | DELETE always allowed and idempotent; controller logic unchanged and already tested (EV-147 tests); UI shows paused state in text; rollback step clears overrides (§5) |
| Privilege escalation to the control plane | gateway roles + service-side role re-check (both tested, both in mutation list); services only reachable via loopback in compose |
| Override applied but not audited | D2 policy: response `audit:"failed"` + ERROR log; misconfiguration fails closed (AC-12); compose sets `DATABASE_URL` |
| Waitingroom startup/readiness coupled to Postgres | pool optional, connect failure is WARN, readiness stays Redis-only (AC-12 test) |
| Ops/sales queries load Postgres during a hot sale | 2 s per-event cache + single-flight + 2 s statement timeout; ≤ 0.5 query rounds/s/event; index left as debt |
| Revenue mis-stated (refunds, pending) | AC-15 test fixtures with every order status; mutation (7) |
| Cross-tenant sales leak | ownership before cache; 404 for non-owner; mutation (6) |
| Sample label removed where data is fake | only real path sets `isSample:false`; peak TPS shows "Chưa có dữ liệu"; test asserts no fabricated number |
| Tokens only in browser → SSR cannot fetch | ops pages become client-side (AC-26); `npm run build` with `NEXT_PUBLIC_API_BASE` unset in the gate |
| Teammates touching the same files | `git merge origin/develop` before PR + full gate rerun |
| ServeMux pattern conflict panics gateway at start | `/v1/ops/` and `/v1/organizer/` share no wildcard prefix with existing routes; gateway `New` test constructs the full table |

## 5. Rollback (checked during verification)

```
# 0. Clear every active override first: after a revert the (unchanged) controller would keep honouring it
#    and no API would remain to clear it.
redis-cli --scan --pattern 'wr:{*}:meta' | xargs -r -I{} redis-cli HDEL {} override override_rate
# 1. Revert
git switch -c rollback-check develop && git revert -m 1 --no-edit <merge-commit>
go test ./services/waitingroom/... ./services/ticketing/... ./services/gateway/...
cd services/ai-worker && uv run pytest -q
cd apps/web && npm test && npm run build && cd ../../tests/e2e && npx playwright test
```
Data: `audit_logs` rows written by this feature remain (append-only, harmless). Compose `DATABASE_URL` for waitingroom is
ignored by the old binary. Verification checks that the reverted tree equals develop @ base for all touched paths.

## 6. Mutation test (threshold 12/12 killed)

Each mutation must make at least one frozen test fail:

1. Waitingroom ops handler: role check removed (`p.Role != "ops"` -> always pass): buyer gets 200.
2. Waitingroom validation: upper bound removed (10001 accepted).
3. Waitingroom validation: lower bound relaxed (`rate < 0` -> `rate < -1`).
4. `OpsOverride`: audit call skipped on success (no `audit_logs` row / fake auditor not called).
5. `OpsOverride`: action not switched to `ADMIT_PAUSED` for rate 0.
6. Redis ops state: `ZCOUNT admitted (now +inf` replaced by `ZCARD admitted` (expired members counted).
7. Ticketing sales: ownership check removed (other organizer gets 200).
8. Ticketing sales: `REFUNDED`/`REFUNDING` orders counted as sold.
9. Ticketing inventory: `consumed_at IS NULL` dropped from `held`.
10. Gateway: `Roles: []string{"ops"}` removed from `GET /v1/ops/events/{eventID}/queue`.
11. Web `ops-service`: a 403 from one source mapped to `unavailable` instead of `OpsServiceError(403)`.
12. Web `ops-service`: `isSample` left `true` on the real path (sample label shown over real data).

Reported but outside the threshold: ai-worker role check removed.

## 7. Expected test-change-request files

Existing tests asserting sample behaviour or server-side fetching, to be replaced (never silently deleted):
`apps/web/src/lib/ops-service.test.ts`, `components/ops/ops-war-room.test.tsx` (sample label, disabled EVF-60 buttons,
m-9 aria-describedby), `hooks/use-ops-war-room.test.tsx` (only if it imports sample helpers),
`app/(ops)/ops/events/page.test.tsx`, `app/(ops)/ops/events/[eventId]/page.test.tsx`, `app/(ops)/layout.test.tsx`
(RoleGuard), `lib/organizer-service.test.ts` (getRealtimeSales part), `hooks/use-organizer-sales.test.tsx`,
`components/organizer/sales-dashboard.test.tsx`, `app/(organizer)/organizer/events/[eventId]/sales/page.test.tsx`,
`src/real-api.contract.test.ts` (sample-data assertions → "no production module imports sample data"),
`tests/e2e/specs/ops-war-room.spec.ts`, `tests/e2e/specs/i18n.spec.ts` (`/ops/events` now needs an ops session mock).
Final list comes from the test designer's request.

## 8. If the human picks D1-B (writes in ticketing)

WP1 keeps only the read endpoint (`ops_state.go`, read part of `ops_handler.go`, `main.go` loadsignal wiring) and drops
pgx, `audit.go`, compose change. WP2 adds `internal/adapter/redis/admit_override.go` (HSET/HDEL on `wr:{ev}:meta`, key
format documented as shared contract like `sale_state.go`), `internal/app/ops_override.go`, audit insert in
`ops_repository.go`, and the two write routes; gateway PUT/DELETE upstream becomes Ticketing and ticketing's write routes
do **not** take the Idempotency middleware (idempotent by method; recorded as an explicit BR-O5 exception).
