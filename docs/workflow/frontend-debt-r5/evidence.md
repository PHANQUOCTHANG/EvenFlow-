# Evidence: frontend-debt-r5 (v3 step 6, independent verify, High-risk)

Verifier: independent agent, 2026-11-20. Clean detached worktree `../EvenFlow-r5-verify` at `0b8f49c`. It has been removed since.
`git diff --name-only c6dcff1 0b8f49c` lists only files under `docs/workflow/` (9 files), so the code under test is `c6dcff1`.
Raw logs are in `verify-logs/`.

## 1. Gate report identity

- `gate-report.json` `commit_sha` = `c6dcff1c27e1ed333bda85ece3a19726755410b3`. Matches.

## 2. Clean run (apps/web, NEXT_PUBLIC_API_BASE unset)

| Step | Result | Log |
|---|---|---|
| npm ci | exit 0 | verify-logs/npm-ci.log |
| lint | exit 0 (warnings only, all in files this change does not touch) | verify-logs/lint.log |
| typecheck | exit 0 | verify-logs/typecheck.log |
| build | exit 0, no segfault | verify-logs/build.log |
| vitest --coverage | 182 files / 2579 tests passed. All files: Stmts 87.46, Branch 90.74, Funcs 90.64, Lines 87.46 | verify-logs/test-coverage.log |

The numbers are the same as the committed gate (`gate/web-test-coverage.log`: 182/2579, 87.46 lines).

The first vitest attempt in this shell failed before collecting any tests: `RangeError: options.minThreads and options.maxThreads must not conflict`. The cause is the host environment. It sets `VITEST_MAX_FORKS=8` and `VITEST_MAX_THREADS=8`, while the default minimum on 32 CPUs is 31. The code is not at fault. All runs above and below use `VITEST_MIN_FORKS=1 VITEST_MIN_THREADS=1`, and lint, typecheck and build were rerun in series with that setting. The failed attempt is kept in `verify-logs/attempt1-env-error/`.

E2E was NOT run, because another gate may be using ports 3000/8080. This report cites the committed `gate/e2e.log`, which shows `38 passed (14.9s)`.

## 3. Acceptance criteria

Command: `npx vitest run src/lib/debt5 src/lib/debt5-fix <checkout + hooks debt5 files> --reporter=json`. Result: 160/160 passed (`verify-logs/ac-tests.json`).

| AC | Evidence (file :: test, all seen passing) | Status |
|---|---|---|
| AC-1 | ac1-refresh-body.test.ts :: "200 + not JSON -> AUTH_REQUIRED, session cleared, 1 POST" (+ stream error/null/string/number; "GUARD: abort at 120 s while the body is being read keeps the abort outcome") 8/8 | DONE |
| AC-2 | ac2-used-refresh.test.ts :: "R2-3: mixed persist, abort at 120 s, ... r1 POSTed once", "R2-S1 ...", "2xx with unusable body marks the token used (Google X-15)", "survives a reload of the tab" 21/21; debt5-fix/r2-used-record-eviction.test.ts :: "marking t0..t16 (17 marks) evicts t0 only" 3/3 | DONE |
| AC-3 | ac3-same-user-reuse.test.ts :: "owner and joiner both get acc-9 written by another tab at 100 s; 1 POST", "401/400/403 while same user signed in again (r9) -> A:200 with acc-9" 11/11 | DONE |
| AC-4 | ac4-ac5-no-bearerless-send.test.ts :: "X-S1: lock never granted, another tab signs out at 2 s -> AUTH_REQUIRED at 45 s" + 2 guards | DONE |
| AC-5 | ac4-ac5-no-bearerless-send.test.ts :: "X-S2: request() resolves undefined -> NETWORK_ERROR, session kept, 0 POST"; debt5-fix/x2-empty-lock-token.test.ts :: "request() resolves '' -> NETWORK_ERROR" | DONE (gap: a truthy non-string result is not tested, see M-8) |
| AC-6 | ac6-role-validation.test.ts :: "X-14 repro: ef_user {id:'u', role:'admin'} is no longer returned", "invalid role in localStorage does NOT fall through to a valid sessionStorage session", "no Authorization header is sent for a tampered role" 33/33 | DONE (gap: the prev-role user-id check is not tested, see M-14) |
| AC-7 | order-result-view.debt5.test.tsx :: "R3-1: shows the auth title and one sign-in link, no 'Thử lại'" + guards | DONE |
| AC-8 | order-result-view.debt5.test.tsx :: "no order: the alert has no empty <p>", "PENDING order then 401: the alert has no empty <p>" | DONE |
| AC-9 | order-result-view.debt5.test.tsx :: "R3-3: PAID + issue-tickets 401 -> one prompt with sign-in link", "the auth prompt appears exactly once"; use-order-status.debt5.test.tsx 4/4 | DONE |
| AC-10 | ac10-safe-redirect.test.ts :: "rejected \"javascript:alert(document.domain)\" -> default result URL" (+ data:/vbscript:/file:/blob:, guards) 21/21 | DONE |
| AC-11 | checkout-view.debt5.test.tsx :: "clicking 'Huỷ giữ vé' opens an accessible dialog and sends NO request yet", "confirm -> exactly 1 DELETE ... Idempotency-Key", "Escape closes it: 0 DELETE", "hold expires while the dialog is open -> dialog closes", "hold expires while the DELETE is in flight -> ends expired" 21/21; cancel-hold-dialog(.fallback).test.tsx 7/7 + 7/7; checkout-view.cancel-focus.test.tsx 2/2 | DONE |
| AC-12 | §2 above (lint/tsc/build/vitest green, 2579 tests, which is at least the 2419 baseline); E2E: committed gate log only (38 passed); Go: committed gate logs only, not rerun | DONE (E2E/Go taken from committed gate, not re-executed) |

All 12 ACs have test evidence: 12/12.

## 4. Mutation test

Each mutant was applied with `verify-logs/mutation/apply.js` and followed by `git checkout -- src`. The test set was src/lib/debt5, debt5-fix, debt, fix2, components/checkout and hooks: 55 files / 566 tests, all green unmutated (`BASELINE.log`). Every survivor was then rerun against the FULL suite (2579 tests, `full-suite-survivors.txt`). No mutant only hung.

| ID | Mutation | Result | Failing tests |
|---|---|---|---|
| M-1 | drop `isRefreshUsed` check in doRefresh | KILLED | 10 |
| M-2 | clearSessionHoldingRefresh wipes both stores | KILLED | 1 |
| M-3 | mark used on fetch rejection | KILLED | 1 |
| M-4 | drop try/catch around `res.json()` | KILLED | 5 |
| M-5 | resolver: drop same-user check | KILLED | 4 |
| M-6 | resolver: drop epoch check | KILLED | 2 |
| M-7 | drop AC-4 throw (`!session && startUserId`) | KILLED | 1 |
| M-8 | lock guard: `typeof token !== "string" \|\| !token` -> `!token` | **SURVIVED** (full suite too) | 0 |
| M-8b | (bounded variant) remove the whole lock guard | KILLED | 4 |
| M-9 | readStore accepts any string role | KILLED | 9 |
| M-10 | safeRedirect returns raw for every scheme | KILLED | 9 |
| M-11 | used check moved after the reuse branch (P-2) | **SURVIVED** (full suite too) | 0 |
| M-12 | epoch bumped whenever anything is wiped (P-3) | **SURVIVED** (full suite too) | 0 |
| M-13 | fall through to sessionStorage on invalid local | KILLED | 2 |
| M-14 | sessionFromIdentity: drop `prev.id === id` (P-6) | **SURVIVED** (full suite too) | 0 |
| M-15 | no marking on 2xx | KILLED | 2 |
| M-16 | no marking on abort | KILLED | 2 |
| M-17 | resolver step (2) without "different refresh token" | SURVIVED (full suite), judged EQUIVALENT | 0 |
| M-18 | AC-1 catch without abort rethrow | SURVIVED (full suite), judged EQUIVALENT | 0 |
| E-1 | eviction `shift` -> `pop` (R-2) | KILLED | 2 |
| E-2 | no duplicate filter on re-mark (R-2) | KILLED | 1 |
| W-1 | trigger calls `handleConfirmCancel` directly (confirm dialog bypassed) | KILLED | 16 |
| W-2 | effect no longer closes dialog on expiry | SURVIVED (full suite), judged EQUIVALENT | 0 |
| W-2b | (bounded variant) W-2 + drop `&& holdActive` render guard | KILLED | 1 |
| W-3 | restore `&& order !== null` in authExpired | KILLED | 2 |
| W-4 | drop issued.authRequired prompt (ticketsAuthRequired = false) | KILLED | 2 |
| W-5 | `resetHold()` before `cancelHold` resolves | KILLED | 7 |

Why three survivors are judged equivalent:
- M-17: step (2) of `failedRefresh` runs only after `isSamePostedSession` is false. A stored token equal to the posted one, with the same user and epoch, already makes that guard true. The same token is also marked used, so `!isRefreshUsed` blocks it. The removed clause is therefore redundant.
- M-18: on abort during the body read, the mutant calls `failedRefresh(epoch, cur)` directly. The original rethrows to `refreshCore`, which calls `markRefreshUsed` and then the same `failedRefresh(epoch, cur)`. The token was already marked on the 2xx, so the outcome is identical.
- W-2: the render condition `confirmOpen && holdActive` already hides the dialog, so the effect is redundant for every tested path. The faithful variant W-2b is killed.

Why the other four survivors are real test gaps. For each one, plan.md promised a test that does not exist:
- M-8 (AC-5): a lock result that is a truthy non-string, such as `{}`, is not tested. Under the mutant it would become `Bearer [object Object]` instead of NETWORK_ERROR. Round 2 checked `{}` and `[]` only in removed `__challenge__` repros.
- M-11 (plan P-2): the promised test is missing. It is "request sent with r2, another tab clears localStorage, used r1 copy surfaces from sessionStorage -> AUTH_REQUIRED, 0 POST, r1 copy wiped, no raw 401". Under the mutant the stale r1 access token is returned instead.
- M-12 (plan P-3): the promised GUARD test is missing. It is "live u-2 in localStorage refreshing, used u-1/r1 copy in sessionStorage wiped meanwhile -> u-2's 2xx result stored, not discarded". Mutation M-12 is named in the plan.
- M-14 (plan P-6): the promised test "prev role from a different user id -> buyer" is missing. Every prev-role test in ac6-role-validation.test.ts uses the same user id.

Score:
- WP-A M-1..M-18: 12/18 killed, 12/16 excluding the 2 equivalents. The plan threshold is 18/18.
- WP-B W-1..W-5: 4/5 killed, 4/4 excluding the equivalent W-2, and W-2b is killed. The plan threshold is 5/5.
- Extra mutants E-1, E-2 and M-8b: 3/3 killed.

Note: the per-mutant logs for M-8, M-11, M-12, M-14, M-17, M-18 and W-2 in `verify-logs/mutation/` were overwritten by the full-suite rerun. `summary.txt` holds the subset results.

## 5. Rollback

- Scratch branch `scratch/r5-rollback-verify` from `c5fb598`, then `merge --no-ff c6dcff1` (825d661), then `revert -m 1`.
- `git diff c5fb598 --stat`: empty.
- npm ci exit 0. npm test: 167 files / 2419 tests passed, which equals the c5fb598 baseline. Build exit 0.
- Scratch worktree and branch deleted. Logs: verify-logs/rollback.log, rollback-npm-test.log, rollback-build.log.

Result: rollback PASS.

## 6. Findings

- findings-review.json: R-1, R-2 and R-3 are all minor. R-1 (lazy wipe instead of eager) was accepted by the human on 2026-11-01 (spec "Human decision after review"). R-2 was fixed in round 2, and E-1/E-2 above confirm it. R-3 is deferred in the spec.
- findings-challenge.json: X-1..X-5 are minor. X-1 was accepted with R-1. X-2, X-4 and X-5 were fixed (round-2 verdicts). X-3 is deferred in the spec (R-3/X-3). X-6 and X-7 are "suspicion":
  - X-6 (no mutation record, Google X-15 test missing): the Google test now exists (ac2 :: "2xx with unusable body marks the token used (Google X-15)"). This report is the mutation record, and it shows survivors.
  - X-7 (close-watcher) is not listed in the spec's deferred list.
- findings-round2.json: `blockers` [] and `majors` []. N2-1, N2-2 and N2-3 (minor or suspicion) are all listed in spec "Deferred after review round 2".
- No open blocker or major finding.

## 7. Overall verdict
FAIL

What passes:
- AC evidence 12/12.
- Clean gates green: lint, typecheck, build, and vitest 2579/2579 at 87.46% lines.
- Rollback.
- No open blocker or major finding.

What fails is the High-risk mutation threshold in plan.md §6/§7 P-8. Four non-equivalent mutants survive the full suite: M-8, M-11, M-12 and M-14. Three of them correspond to tests the approved plan explicitly promised (P-2, P-3, P-6) that do not exist.

To fix, add four tests and rerun the mutants:
1. Lock resolves `{}` -> NETWORK_ERROR.
2. The P-2 stale-r1-surfaces case.
3. The P-3 shadowed-store wipe GUARD.
4. Prev role from a different user id -> buyer.

No production code change is expected to be needed. By code inspection, not by executed tests, each of these behaviours is correct in the current code; it is just unguarded.
