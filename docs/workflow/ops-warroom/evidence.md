# Verification evidence: ops-warroom (v3 step 6, High risk)

Verifier: independent, read-only on production code. Date 2026-11-21. Worktree `EvenFlow-ops-warroom`,
branch `feature/ops-warroom`, HEAD `506d5bd`.

## Verdict: FAIL (mutation threshold not met)

All suites are green and every AC that unit tests can reach has passing test evidence. Of 52 mutants, 42 were
killed and 10 survived. Four survivors are equivalent at system level (M02, M03, W11, W12), so 42 of 48
behaviour-changing mutants were killed. The other 6 are gaps. Three are against the contract: M12 and M17 (§3.4) and
T08 (§4.2). Two are hardening that no AC states: M21 and T04. One, T09, can only be seen with integration tests.
See the mutation section. Under v3 the
threshold is "all behaviour-changing mutants killed", so the verdict is FAIL until those gaps are closed by new tests
or the human accepts them as known gaps. No production bug was found: the code under test behaves as contracted.
Each survivor is a missing assertion.

## 1. Gate commit check

| Check | Result |
|---|---|
| `gate-report.json` `commit_sha` | `cfc630e7b5a3ab6ceaf729fba4d6ef2364cfff89` (base `c5fb598`) |
| Last production commit | `cfc630e` (`docs(workflow): record deferred round 2 nits`). Last code change is `12fca5c` |
| `git diff --name-only cfc630e 506d5bd` outside `docs/` | none (only 10 files under `docs/workflow/ops-warroom/gate/` and the report) |

PASS: HEAD differs from the gated commit only by docs.

## 2. Test runs at HEAD (logs in `verify-logs/`)

| Suite | Command | Result |
|---|---|---|
| waitingroom | `go test ./... -race -count=1` | all packages ok, exit 0 (`go-waitingroom.log`) |
| ticketing | `go test ./... -race -count=1` | all packages ok, exit 0 (`go-ticketing.log`) |
| gateway | `go test ./... -race -count=1` | all packages ok, exit 0 (`go-gateway.log`) |
| ai-worker | `uv run pytest -q` | 211 passed, 22 deselected, exit 0 (`py-ai-worker.log`) |
| web | `env -u VITEST_MAX_THREADS -u VITEST_MAX_FORKS npx vitest run` | 180 files, 2560 tests passed, exit 0 (`web-vitest.log`) |

Not run, as the brief said: `full-gate.sh` and Playwright E2E (ports 3000/8080 shared). For lint, typecheck, build and E2E,
AC-31 relies on `gate-report.json` (PASS at `cfc630e`).

## 3. AC table

"Unit" = the test files named in `traceability.yaml`, run green in section 2. "Mutants" = mutants from section 4 that
the AC's tests killed.

| AC | Proving tests | Run | Mutants killed | Status |
|---|---|---|---|---|
| AC-1 queue snapshot | waitingroom `ops_handler_test.go` TestOpsQueue_AC1_*; integration `test/opsapi` TestTichHop_OpsQueue_AC1_* | unit green; integration NOT RUN | - | VERIFIED (unit); Redis pipeline NOT VERIFIED |
| AC-2 expired admitted not counted | integration `test/opsapi` TestTichHop_OpsQueue_AC1_AC2_SnapshotCountsOnlyActiveAdmitted only | NOT RUN (Docker) | - | NOT VERIFIED |
| AC-3 no loadsignal | TestOpsQueue_AC3_* | green | - | VERIFIED |
| AC-4 404/400/503 | TestOpsQueue_AC4_Errors, TestOpsWrite_AC4_* | green | - | VERIFIED |
| AC-5 service role 403 | TestOpsQueue_AC5_RoleAndPrincipal | green | M18 | VERIFIED |
| AC-6 PUT applies | TestOpsPut_AC6_* | green | M01, M04, M22 | VERIFIED |
| AC-7 controller follows override within 2 s | integration `test/opsapi` TestTichHop_OpsOverride_AC7_RealControllerFollowsPauseAndResume only | NOT RUN (Docker) | - | NOT VERIFIED |
| AC-8 DELETE idempotent | TestOpsDelete_AC8_AC10_* | green | M13 | VERIFIED |
| AC-9 invalid input 400 | TestOpsWrite_AC9_*, `ops_reason_fix_test.go`, `ops_round2_fix_test.go`, `ops_rate_literal_fix_test.go` | green | M04-M09, M19, M20, M22 | VERIFIED (trailing-data gap M21) |
| AC-10 one audit row, commanded rate | TestOpsPut_AC10_*; integration `test/opsaudit` TestTichHop_OpsAudit_AC10_OneRowPerApply | unit green; integration NOT RUN | M10, M11, M13 | VERIFIED (unit); gap M12 (audit row on failed apply) |
| AC-11 audit failure keeps override | TestOpsWrite_AC11_*, `ops_audit_panic_fix_test.go` | green | M14, M15 | VERIFIED; gap M17 (detached context) |
| AC-12 no audit store fails closed | TestOpsWrite_AC12_*; integration `test/opsaudit` AC12 | unit green | M16 | VERIFIED (unit) |
| AC-13 non-ops nothing applied | TestOpsWrite_AC13_* | green | M18 | VERIFIED |
| AC-14 ops inventory | ticketing `ops_handler_test.go` TestOpsInventory_AC14_*; `ops_repository_integration_test.go` | unit green; SQL integration NOT RUN | T05 | VERIFIED (handler); SQL definitions NOT VERIFIED |
| AC-15 organizer sales | TestOpsSales_AC15_*, `ops_queries_test.go`; repo integration AC15 | unit green; SQL NOT RUN | T07 | VERIFIED (handler/app); gap T08; SQL NOT VERIFIED |
| AC-16 owner-only 404 | TestOpsSales_AC16_*, TestOpsCache_AC16_*, `ops_owner_case_fix_test.go` | green | T01, T02 | VERIFIED |
| AC-17 2 s cache, single-flight, one snapshot | TestOpsCache_AC17_*, `ops_generated_at_fix_test.go` | green | T03, T06 | VERIFIED (cache); snapshot isolation T09 integration-only, NOT VERIFIED |
| AC-18 ai mode endpoint | ai-worker `test_ops_api.py`, `test_ops_identity_fix.py` | green | P01, P02, P03 | VERIFIED |
| AC-19 gateway routes and roles | gateway `ops_routes_test.go` | green | G01-G04 | VERIFIED |
| AC-20 getOpsWarRoom calls/mapping | `ops-service.contract.test.ts` | green | W10 | VERIFIED |
| AC-21 source health | `ops-service.contract.test.ts`, `ops-war-room.real.test.tsx` | green | W04 | VERIFIED |
| AC-22 error mapping (401 core-only, 404, 403) | `ops-service.contract.test.ts`, `ops-service.auth.contract.test.ts`, `ops-auth.fix.test.tsx` | green | W01, W02, W03 | VERIFIED |
| AC-23 sample label only when isSample | `ops-war-room.real.test.tsx` | green | W10 | VERIFIED |
| AC-24 controls + client validation | `ops-war-room.real.test.tsx`, `ops-service.reason.fix.test.ts`, `ops-service.round2.fix.test.ts` | green | W05-W08 | VERIFIED |
| AC-25 write results | `ops-war-room.real.test.tsx`, `ops-service.contract.test.ts` | green | W09 | VERIFIED |
| AC-26 /ops/events list | `ops-pages.real.test.tsx` | green | - | VERIFIED (tests only) |
| AC-27 RoleGuard ops | `ops-pages.real.test.tsx`, `(ops)/layout.test.tsx` | green | - | VERIFIED (tests only) |
| AC-28 sample data removed | `ops-warroom.no-sample.contract.test.ts` | green | - | VERIFIED |
| AC-29 getRealtimeSales | `organizer-service.sales.contract.test.ts`, `.fix.test.ts` | green | - | VERIFIED |
| AC-30 sales dashboard auto-refresh 4 s, cap 5 s | `sales-dashboard.real.test.tsx`, `use-organizer-sales.test.tsx` | green | W13, W14 (W11, W12 equivalent) | VERIFIED |
| AC-31 gates | `gate-report.json` PASS at `cfc630e`; unit suites re-run here | partly re-run | - | VERIFIED via gate report; E2E not re-run |
| AC-32 no migration, only pgx added | `git diff c5fb598 cfc630e`: no `*.sql`/migration file; waitingroom `go.mod` adds direct `pgx/v5 v5.9.2` (+ indirect transitive) | checked | - | VERIFIED |

## 4. Mutation testing

Script: `verify-logs/mutation/run_mutants.py` (52 mutants, one exact-text replacement each). For each mutant it runs the
owning suite and then reverts with `git checkout -- services apps`. Results are in `verify-logs/mutation/results.json`,
with per-mutant failure excerpts in `verify-logs/mutation/<ID>.log` and the run order in `run.out`. Suites:
waitingroom/ticketing `go test ./internal/... [./cmd/...] -count=1`, gateway `go test ./...`, ai-worker
`uv run pytest -q -x`, web `npx vitest run` over `src/lib src/hooks src/components/ops src/components/organizer
src/app/(ops) src/app/(organizer)`.

**Result: 42 killed / 10 survived of 52 (4 survivors equivalent, so 42/48 behaviour-changing killed). Threshold NOT met.**
W13 and W14 were added after W11 and W12 proved equivalent.

| ID | File | Mutant | Suite | Result |
|---|---|---|---|---|
| G01 | `routes.go` | PUT override also allowed for organizer | gw | KILLED |
| G02 | `routes.go` | sales route also allowed for buyer | gw | KILLED |
| G03 | `routes.go` | ai mode routed to wrong upstream | gw | KILLED |
| G04 | `routes.go` | inventory route forwards any method | gw | KILLED |
| M01 | `ops_override.go` | ValidRate upper bound 10000 -> 10001 | wr | KILLED |
| M02 | `ops_override.go` | ValidRate lower bound 0 -> -1 | wr | SURVIVED (equivalent) |
| M03 | `ops_override.go` | ValidRate accepts fractional rates | wr | SURVIVED (equivalent) |
| M04 | `ops_handler.go` | parseRate literal digit check disabled (-0 accepted) | wr | KILLED |
| M05 | `ops_override.go` | reason visible-char minimum 3 -> 2 | wr | KILLED |
| M06 | `ops_override.go` | Cf chars counted as visible | wr | KILLED |
| M07 | `ops_override.go` | bidi controls no longer rejected | wr | KILLED |
| M08 | `ops_override.go` | reason max 200 -> 201 | wr | KILLED |
| M09 | `ops_override.go` | control chars no longer rejected | wr | KILLED |
| M10 | `ops_override.go` | audit records read-back rate instead of commanded rate | wr | KILLED |
| M11 | `ops_override.go` | rate 0 audited as SET instead of PAUSED | wr | KILLED |
| M12 | `ops_override.go` | Set audits even when the Redis apply failed (audit before successful apply) | wr | SURVIVED |
| M13 | `ops_override.go` | Clear audits before apply (order swapped) | wr | KILLED |
| M14 | `ops_override.go` | auditor panic reported as recorded | wr | KILLED |
| M15 | `ops_override.go` | audit DB error reported as recorded | wr | KILLED |
| M16 | `ops_override.go` | no audit store no longer fails closed (503) | wr | KILLED |
| M17 | `ops_override.go` | Set apply/audit bound to request context | wr | SURVIVED |
| M18 | `ops_handler.go` | waitingroom handler role check removed | wr | KILLED |
| M19 | `ops_handler.go` | decodeStrict accepts duplicate keys | wr | KILLED |
| M20 | `ops_handler.go` | decodeStrict exact-key check removed (case mismatch + dup) | wr | KILLED |
| M21 | `ops_handler.go` | trailing data after JSON accepted | wr | SURVIVED |
| M22 | `ops_handler.go` | parseRate accepts quoted string "12" | wr | KILLED |
| P01 | `main.py` | ai mode allows organizer | py | KILLED |
| P02 | `main.py` | ai mode identity (401) check removed | py | KILLED |
| P03 | `main.py` | ops ai mode leaks policy field | py | KILLED |
| T01 | `ops_handler.go` | sales owner check removed | tk | KILLED |
| T02 | `ops_handler.go` | owner compare case-sensitive | tk | KILLED |
| T03 | `ops_handler.go` | 2s cache disabled | tk | KILLED |
| T04 | `ops_handler.go` | not-found no longer cached | tk | SURVIVED |
| T05 | `ops_handler.go` | inventory role ops -> organizer | tk | KILLED |
| T06 | `ops_handler.go` | cache TTL 2s -> 20s (staleness bound) | tk | KILLED |
| T07 | `ops_queries.go` | totals not derived from same-snapshot tier rows | tk | KILLED |
| T08 | `ops_queries.go` | unknown-provider reconciliation condition | tk | SURVIVED |
| T09 | `ops_repository.go` | sales snapshot tx REPEATABLE READ removed (integration-only) | tk | SURVIVED |
| W01 | `ops-service.ts` | 401 from any source (not only core) -> sign-in | web | KILLED |
| W02 | `ops-service.ts` | event 404 not mapped to not-found | web | KILLED |
| W03 | `ops-service.ts` | source 403 not surfaced as forbidden | web | KILLED |
| W04 | `ops-service.ts` | all three sources down not 503 | web | KILLED |
| W05 | `ops-service.ts` | client rate max 10000 -> 10001 | web | KILLED |
| W06 | `ops-service.ts` | client accepts fractional rate | web | KILLED |
| W07 | `ops-service.ts` | client bidi reject removed | web | KILLED |
| W08 | `ops-service.ts` | client visible count includes invisible chars | web | KILLED |
| W09 | `ops-service.ts` | write 401 not treated as session lost | web | KILLED |
| W10 | `ops-service.ts` | war room snapshot labelled sample | web | KILLED |
| W11 | `use-organizer-sales.ts` | sales auto-refresh off by default | web | SURVIVED (equivalent) |
| W12 | `use-organizer-sales.ts` | sales polling cap 5s removed | web | SURVIVED (equivalent) |
| W13 | `sales-dashboard.tsx` | dashboard auto-refresh off by default | web | KILLED |
| W14 | `use-organizer-sales.ts` | default polling 4s -> 4.9s | web | KILLED |

### Survivors

| ID | Mutant | Classification | Why it matters |
|---|---|---|---|
| M02 | `ValidRate` lower bound 0 -> -1 | equivalent at system level | `parseRate` rejects any non-digit byte before `ValidRate`, so a negative value cannot reach it. No app-level test of `OpsOverride.Set` with a negative rate exists. |
| M03 | `ValidRate` accepts fractions | equivalent at system level | Same reason: `parseRate` only produces integers. Defence-in-depth check, not covered by a direct app test. |
| M12 | `Set` writes an audit row when the Redis apply failed | **test gap (contract §3.4)** | The contract says "one row per successful apply". `fakeOverrideStore.err` fails every store call, including the precheck, so no test makes only `SetRateOverride` fail and then asserts that no audit row was written. |
| M17 | `Set` apply/audit on the request context instead of `context.WithoutCancel` | **test gap (contract §3.4)** | The contract requires the detached context so a client disconnect cannot cancel the audit. No test cancels the request context after the apply. |
| M21 | trailing bytes after the JSON body accepted | test gap (hardening, not in AC-9 text) | `decodeStrict` documents the rejection, but no test sends `{...}garbage`. |
| T04 | ticketing cache no longer caches not-found | test gap (hardening, code comment only) | Protects against random-UUID loops. Neither the contract nor AC-17 states it, and no test asserts it. |
| T08 | `unknown` provider added only when both remaining orders **and** amount are > 0 | **test gap (contract §4.2 invariant)** | The invariant `SUM(orders)=paid_orders`, `SUM(amount)=gross` breaks when unpaid-payment orders have amount 0 (free tier) or amounts differ while counts match. No test covers that mix. |
| T09 | sales transaction without REPEATABLE READ READ ONLY | needs integration (Docker) | Snapshot consistency can only be observed against real Postgres. The unit fakes cannot see the isolation level, and the repo integration suite has no concurrent-write case. NOT VERIFIED. |
| W11 | hook default `initialAutoRefresh` true -> false | equivalent | The only caller (`sales-dashboard.tsx`) passes `initialAutoRefresh: true`. The stronger mutant W13 on the dashboard was killed. |
| W12 | hook 5 s polling cap removed | equivalent | No caller passes an interval, and the default 4000 ms is below the cap. The stronger mutant W14 (default 4000 -> 4900) was killed. |

To reach the threshold, the smallest fix is new tests (no production change):
1. waitingroom: a store fake that fails only `SetRateOverride` / `ClearRateOverride`, then assert that the auditor has 0 rows and the response is 503 (kills M12).
2. waitingroom: cancel the request context inside `SetRateOverride`, then assert `audit:"recorded"` and 1 row (kills M17).
3. waitingroom: PUT `{"rate_per_sec":1,"reason":"abc"} x`, then assert 400 (kills M21).
4. ticketing: two calls on an unknown event within 2 s, then assert one loader call (kills T04); a reconcile case with 0-amount orders lacking a SUCCEEDED payment (kills T08).
5. M02/M03: either accept them as equivalent or add an `OpsOverride.Set` table test with -1 and 0.5.

## 5. Rollback check

Temporary worktree at `origin/develop` `c5fb598` (`git worktree add --detach D:/tmp/evf-rollback-c5fb598 c5fb598`,
removed afterwards). Log: `verify-logs/rollback.log`.

| Item | Command | Result |
|---|---|---|
| waitingroom | `go build ./... && go vet ./... && go test ./... -count=1` | exit 0 |
| ticketing | same | exit 0 |
| gateway | same | exit 0 |
| ai-worker | `uv sync --all-extras && uv run pytest -q` (fresh venv needed the extras) | 203 passed, 22 deselected |
| Schema | `git diff --name-only c5fb598 cfc630e \| grep -iE 'migrat\|\.sql'` | none. The feature adds no tables and writes to the existing `audit_logs` table (`migrations/0001_core.sql` at `c5fb598`). |
| Redis | the override uses `meta` hash fields of the room (contracts §3), and the base controller ignores them | additive |
| Deploy | `docker-compose.yml`: adds `DATABASE_URL` and a `pgbouncer` depends_on to waitingroom | additive. Reverting the merge commit is safe. |

Rollback: PASS. Reverting the feature needs no data migration. Audit rows already written stay in `audit_logs`, which is harmless.

## 6. R2-5 integration suites

`docker info` gives: `error during connect: ... open //./pipe/dockerDesktopLinuxEngine: The system cannot find the file
specified.` The Docker engine is not running.

**NOT VERIFIED.** Commands to run once Docker is up:

```
cd services/waitingroom && go test -tags integration ./... -count=1   # test/opsapi (AC-1, AC-2, AC-4, AC-6, AC-7, AC-8), test/opsaudit (AC-10, AC-11, AC-12)
cd services/ticketing   && go test -tags integration ./... -count=1   # internal/adapter/postgres ops_repository_integration_test.go (AC-14, AC-15, AC-16)
```

Partial check that was possible without Docker: `go vet -tags integration ./...` in waitingroom and ticketing gives
exit 0 for both. The suites compile, but they were not executed.

## 7. Open human items (sign-off)

| ID | Item | Verifier note |
|---|---|---|
| R-8 | The applied change to `sales/page.test.tsx` goes beyond the approved TCR row ("metadata text only") | Confirm or reject at sign-off. No code change needed. |
| R3-1 | Edge-control whitespace: the web check is stricter than the server | The web rejects some inputs the server would trim and accept. This is safe (fail-early) but not exact parity. Accept or ask for alignment. |
| R2-6 | Tamper-check threshold for frozen tests raised from 4 to 5 changed lines | A process/CI change. Needs human approval. |
| event-detail 401 -> 503 | A 401 on the public event detail is shown as a source error (503, the previous snapshot is kept) rather than sign-in (X2-3) | Confirm the choice. It is pinned by `ops-service.round2.fix.test.ts` ("public event detail 401 is not session loss"), and mutant W01 (401 from any source) was killed. |
| R2-5 | Integration suites are not executed in the gate | Still open. Not executed here either (section 6). |
| Mutation survivors | M12, M17, T08 (contract), M21, T04 (hardening), T09 (integration-only); M02, M03, W11, W12 (equivalent) | Decide: add tests (recommended, about 5 small cases) or accept as known gaps. |

## 8. Re-verification after 7a0f27f (2026-11-21)

Trigger: commit `7a0f27f` (`test(ops): cover surviving ops war room mutants`) targets M12, M17, M21, T04 and T08.

**Test-only check: PASS.** `git show --name-only 7a0f27f` lists 3 new test files and nothing else:
- `services/waitingroom/internal/adapter/http/ops_mutation_gap_test.go`
- `services/ticketing/internal/adapter/http/ops_mutation_gap_test.go`
- `services/ticketing/internal/app/ops_sales_invariant_test.go`

No production file changed after the gated commit `cfc630e`.

**Suites at 7a0f27f** (logs in `verify-logs/reverify/`):
- waitingroom `go test ./... -race -count=1`: exit 0
- ticketing `go test ./... -race -count=1`: exit 0

Gateway, ai-worker and web are unchanged since section 2, so they were not re-run.

**Full mutation re-run:** all 52 mutants, same `run_mutants.py`. The round-1 results moved to `verify-logs/mutation/round1/`. The new `results.json`, `run.out` and `<ID>.log` files are in `verify-logs/mutation/`.

**Result: 47 killed / 5 survived of 52.**
- Equivalent survivors, unchanged: M02, M03, W11, W12 (reasons in section 4).
- Behaviour-changing mutants: **47 / 48 killed**.
- The remaining survivor is **T09** (sales transaction without REPEATABLE READ READ ONLY). Only the integration suite against real Postgres can see it, and Docker is still not running. **NOT VERIFIED.**

| Former survivor | Now | Killing test |
|---|---|---|
| M12 audit row on failed apply | KILLED | waitingroom `TestOpsPut_ApplyFailureIsNotAudited` |
| M17 request context instead of detached | KILLED | waitingroom `TestOpsPut_RequestCancelledMidApplyStillAppliesAndAudits` |
| M21 trailing bytes accepted | KILLED | waitingroom `TestOpsPut_TrailingBytesAfterJSONIs400` |
| T04 not-found not cached | KILLED | ticketing `TestOpsCache_NotFoundIsCachedInsideTTL` |
| T08 unknown-provider reconcile | KILLED | ticketing `TestGetOrganizerSales_Contract42_BreakdownSumsToTotals/remaining_orders_with_zero_amount_…` |

### Re-verification verdict: PASS (conditional)

Every behaviour-changing mutant that unit tests can reach is killed. These items are still NOT VERIFIED because the Docker engine is not running:
- T09 (snapshot isolation)
- R2-5: the integration suites (`go test -tags integration ./... -count=1` in waitingroom and ticketing). Under this suite, AC-2, AC-7, the Redis side of AC-1, the real-Postgres side of AC-10/11/12 and the SQL of AC-14/15/16 are covered only here.

E2E and `full-gate.sh` were not re-run, as instructed. The human accepts these at sign-off or runs the integration commands with Docker up.

Open human items, unchanged from section 7:
- R-8: `sales/page.test.tsx` change beyond its TCR row
- R3-1: edge-control whitespace check stricter on web than on the server
- R2-6: tamper threshold 4 -> 5
- Event-detail 401 shown as 503 (X2-3)
- R2-5: integration suites not run in the gate
