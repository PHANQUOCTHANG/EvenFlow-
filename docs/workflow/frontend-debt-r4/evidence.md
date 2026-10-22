# Verification evidence: frontend-debt-r4 (v3 step 6, High-risk)

Independent verifier. Everything below was produced by the verifier on 2026-10-22, in a clean detached worktree
`EvenFlow-debt-verify` at `c5532e4`, with fresh `npm ci`. Raw logs are in `verify-logs/`.

## 1. Commit match

- `git diff --stat e91dff0 c5532e4` touches only `docs/workflow/frontend-debt-r4/**` (9 files: findings, gate report,
  gate logs). Code under test at c5532e4 is identical to e91dff0.
- `gate-report.json` `commit_sha` = `e91dff0416b8ef65bf95ae9e7c6b84f6932ea445` = `git rev-parse e91dff0`. **Match.**

## 2. Clean run results

| Step | Command | Exit | Raw tail |
|---|---|---|---|
| npm ci | `npm ci --no-audit --no-fund` | 0 | `added 548 packages in 30s` (`web-npm-ci.log`) |
| lint | `npm run lint` | 0 | warnings only (`no-explicit-any`), no errors (`web-lint.log`) |
| typecheck | `npm run typecheck` | 0 | `tsc --noEmit` clean (`web-typecheck.log`) |
| build | `env -u NEXT_PUBLIC_API_BASE npm run build` | 0 | route table printed (`web-build.log`) |
| unit + coverage | `npx vitest run --coverage` | 0 | `Test Files 167 passed (167)`, `Tests 2419 passed (2419)` (`web-test-coverage.log`) |
| Go gateway | `go vet ./... && go test ./... -count=1` | 0 | `ok .../internal/config`, `ok .../internal/proxy`, `ok .../internal/ratelimit` (`go-gateway.log`) |
| Go event | `go vet ./... && go vet -tags integration ./... && go test ./... -count=1` | 0 | `ok .../adapter/http`, `ok .../app`, `ok .../domain`, `ok .../worker` (`go-event.log`) |
| E2E | the e2e block of `full-gate.sh`, verbatim | 0 | `38 passed (17.2s)` (`e2e.log`) |

Coverage, clean run, nothing else running in parallel:

```
File               | % Stmts | % Branch | % Funcs | % Lines | Uncovered Line #s
All files          |   87.01 |    90.33 |    90.1 |   87.01 |
```

**87.01 % statements and lines.** The gate's 85.24 % came from a run that was probably polluted by a parallel process.

AC-7 tamper cross-check, `43375eb..e91dff0` (`tamper-check.log`): no test files deleted, no contract tests changed, no
pre-existing test file modified, no new `skip`, `only` or `todo`. Test count rose from 2342 (rollback tree = 43375eb)
to 2419.

## 3. Acceptance criteria

Every test below was seen passing in `ac-evidence-verbose.log` (16 files, 86 tests, all passing) and again in the
full run (2419 passing).

| AC | Passing tests (file::name) | Result |
|---|---|---|
| AC-1 | `lib/debt/ac1-user-switch.test.ts`::"another tab signs out u-1 and signs in u-2: AUTH_REQUIRED, u-2 session untouched, 0 POST, no retry with u-2 token"; ::"u-2 holds the SAME refresh string but a different user id: still AUTH_REQUIRED"; ::"GUARD: the other tab rotated the pair for the SAME user: reused, request succeeds, no POST"; ::"GUARD: the other tab signed out and nobody signed in"; ::"u-1 -> u-2 -> u-1 within one wait"; ::"other tab signs in u-2 during u-1's POST, POST then succeeds: request NOT retried with u-2 token"; `fix1-f2-no-refresh-token.test.ts`::"u-1 request waits for the lock; another tab stores u-2 WITHOUT refresh token"; `fix1-f3-empty-user-id.test.ts`::"request sent by user id \"\"; another tab signs in u-2"; `fix1-f5-clear-guards.test.ts`::"POST answers 401 after the switch: u-2 with the SAME refresh string \"r1\" is not cleared" | **DONE** |
| AC-2 (clarified: the POST owner is bounded only by the 120 s abort) | `ac2-refresh-deadlines.test.ts`::"W-3: other tab waiting on the lock rejects NETWORK_ERROR at 45 s each time; 1 POST total"; ::"a later same-tab caller joins the in-flight refresh and rejects NETWORK_ERROR at 45 s; a call at 50 s joins too"; ::"GUARD (Z-1b): the owner whose POST answers at 60 s gets 200"; ::"after a joiner gave up, the POST that finally answers (60 s) still stores the pair and frees the lock"; ::"exports REFRESH_POST_TIMEOUT_MS = 120_000"; ::"owner pending at 119 s, signal aborted and AUTH_REQUIRED at 121 s, session cleared, r1 POSTed once"; ::"abort guard (other tab)"; ::"abort guard (same tab)"; ::"after the abort the lock is released"; ::"GUARD: a plain network failure of the POST keeps the session"; ::"GUARD (Z-3)"; ::"a u-2 caller in the same tab does not join u-1's hung refresh"; ::"calls at 0 s, 46 s and 100 s -> exactly 1 POST" (no locks, and SecurityError); ::timer hygiene ×4; `fix1-f5-clear-guards.test.ts`::"120 s abort: u-2 stored with the SAME refresh string \"r1\" is not cleared"; `lib/fix2/g1-lock-refresh.test.ts`::"Z-1b: holder POST takes 60 s: the waiter gives up at 45 s with NETWORK_ERROR (no POST, session kept); the holder finishes" (plus Z-1, Z-2, Z-3) | **DONE** |
| AC-3 | `ac3-locks-errors.test.ts`::"reject / throw InvalidStateError / NotSupportedError / TypeError -> ApiError NETWORK_ERROR, 0 POST, session kept, no pending timer" (6 tests); ::"reject AbortError -> ..."; ::"SecurityError (reject) -> in-tab single-flight: exactly 1 POST"; ::"SecurityError (throw) -> ..."; ::"W-2: the lock error also reaches hold-client as a classified (AMBIGUOUS) error"; ::"GUARD: a lock error does not clear buy-flow keys or the session" | **DONE** |
| AC-4 | `ac4-cross-tab-owner.test.tsx`::"getSession(): stored user id differs from this tab's ef:lastuid -> buy-flow keys cleared"; ::"W-1 setSession(): this tab then signs in as u-B -> keys of u-A are cleared"; ::"storage event for ef_user -> keys cleared"; ::"GUARD: same user ... keeps the keys"; ::"GUARD (G-4): other tab signs out"; ::"A -> B -> A"; ::"unrelated key"; ::"never touches the auth keys"; ::"explicit logout"; ::"ef:lastuid absent ... KEEPS the keys" | **DONE** |
| AC-5 | `edit/page.debt.test.tsx`::"W-4: retry matches the reloaded server tier: addTier called once in total, no quota PUT"; ::"matched tier with a different quota: only the quota is sent"; ::"trim + case-insensitive"; ::"any 422 from addTier triggers the same lookup"; ::"an UNRELATED existing tier with the same name but a different price is NOT matched"; ::"GUARD: the organizer renamed the lost tier"; ::"GUARD (F-3): normal save POSTs once" | **DONE** |
| AC-6 | `ac6-payment-forbidden.test.ts`::"initiatePayment / getOrderDetails / issueTickets -> FORBIDDEN" (3) + 7 GUARD tests (other 403s keep their mapping); `payment-handoff-view.debt.test.tsx`::"shows checkout.forbidden ... exactly one payment POST"; `order-result-view.debt.test.tsx`::"order poll 403 ... 1 GET over 120 s"; ::"issue-tickets 403 on a PAID order ... stops after 1 POST"; `order-result-view.fix1.test.tsx`::"initialOrder PENDING + GET 403"; `order-result-view.fix2.test.tsx`::"G-1 ... forbidden text appears exactly once" and G-2 ×3; `use-payment-session.debt.test.tsx` ×2; `use-payment-session.fix1.test.tsx` ×1 | **DONE** |
| AC-7 | §2: lint, typecheck, build (no API base), 2419/2419 Vitest, 38/38 E2E, Go gateway/event green; tamper cross-check clean | **DONE** |

**7 of 7 ACs DONE.**

## 4. Mutation test (threshold 9/9)

Run in the clean checkout, with `git checkout -- .` between mutants, against the full suite (`npx vitest run`).
Baseline: 2419/2419 passing (`mut-baseline.log`). Applier: `verify-logs/mutate.js`. Diffs: `mutant-N.diff`.

| # | Mutation (diff) | Full suite | Kill kind | Killed |
|---|---|---|---|---|
| 1 | api-client: removed `if (cur && sentUserId !== null && cur.user.id !== sentUserId) throw userChanged();` (doRefresh), `if (after.user.id !== cur.user.id) throw userChanged();` (post-POST) and `if (session && session.user.id !== startUserId) throw userChanged();` (proactive) | 5 failed | assertion (`expected 'A:200' to be 'A:AUTH_REQUIRED'`) | yes |
| 2 | `if (existing) return joinInflight(existing);` → `return existing;` | 4 failed | 5 s test timeouts only. Bounded probe P-2: `expected null to be 'J:NETWORK_ERROR'` | yes (bounded) |
| 3 | `setTimeout(() => controller.abort(), REFRESH_POST_TIMEOUT_MS)` → `undefined` (no abort) | 8 failed | assertion (`expected 'pending' to be 'AUTH_REQUIRED'`) | yes |
| 4 | onLockError: `throw networkError();` → `throw err;` | 7 failed | assertion (`expected DOMException{...} to be an instance of ApiError`) | yes |
| 5 | session `reconcileBuyFlowOwner`: stored user id (local/session) compared before `ef:lastuid` | 1 failed | assertion (`expected [ 'ef:queue:token:e1', …(5) ] to deeply equal []`) | yes |
| 6 | `joinInflight` timeout also deletes `inflight[key]` | 2 failed | 5 s test timeouts only. Bounded probe P-6: `expected [ {…}, {…} ] to have a length of 1 but got 2` (second POST) | yes (bounded) |
| 7 | abort path: `if (isSamePostedSession(epoch, cur)) clearSession();` → `clearSession();` | 3 failed | assertion (`expected null to be 'acc-u2'`) | yes |
| 8 | AC-5 match `(s) => !referenced.has(s.id) && s.price === tier.price && normName(...)` → name only | 1 failed | Testing Library query error (`Unable to find an element with the text: /tên hạng vé trùng/`) | yes |
| 9 | only the post-POST `if (after.user.id !== cur.user.id) throw userChanged();` removed | 1 failed | assertion (`expected 'A:200' to be 'A:AUTH_REQUIRED'`) | yes |

No worker crashed in any run. For mutants 2 and 6, the repo suite fails only because tests `await` a promise the mutant
leaves pending, which hits the 5 s test timeout. Following the rule, the verifier wrote a bounded scratch probe
(`verify-logs/zz-verify-probe.test.ts`). It reads promise state through flags and never awaits a pending promise.
The probe passed 2/2 on clean code (`mutant-clean-bounded.log`) and failed at the assertion level on mutant 2 and on
mutant 6 (`mutant-2-bounded.log`, `mutant-6-bounded.log`). It was deleted from the checkout afterwards.

**Mutation score: 9/9.** Observation (minor, test quality, not blocking): the repo's AC-2 tests detect mutants 2 and 6
only through a hang.

## 5. Rollback

Scratch branch `verify/rollback-scratch-r4` from `43375eb`: `git merge --no-ff --no-edit e91dff0`, then
`git revert -m 1 --no-edit HEAD`. `git diff 43375eb --stat` printed nothing, so the tree is identical to 43375eb
(`rollback-git.log`).

On the reverted tree: npm ci exit 0, lint 0, typecheck 0, `npm test` 0 (`Test Files 152 passed`,
`Tests 2342 passed`), build 0 with `NEXT_PUBLIC_API_BASE` unset. The scratch branch and its worktree were deleted.
**Rollback PASS.**

## 6. Open findings

From `findings.json`, cross-checked against `findings-round3-review.json` (blocker 0, major 0):

- Round 1: F-1 (major) and F-2..F-5 (minor) are `fixed`. D-R-3, D-X-4, D-X-5, D-X-S1 and D-X-S2 are `deferred`, all
  minor.
- Round 2: blocker 0, major 0. G-1 and G-2 are `fixed`. D-R2-3, D-R2-4 and D-R2-S1 are `deferred`, all minor.
- Round 3 (on e91dff0): blocker 0, major 0. D-R3-1, D-R3-2 and D-R3-3 are `deferred`, all minor.

No open blocker or major finding. Every to-fix finding is fixed and every deferred finding is minor.

## 7. Overall verdict
PASS

## 8. Addendum after merging origin/develop (2026-10-22)

- `origin/develop` (+31 commits from teammates) was merged into the branch at `ac5cda6`. `git diff e91dff0 55cbf1e -- apps/web` is empty, so the verified web code is unchanged.
- Full gate re-run on the merged head `55cbf1e`: PASS (Vitest 2419/2419, coverage 87.01%, E2E 38/38, Go, audit, tamper `approved TCR-1 only`). One earlier run exited 139 because Node segfaulted after `next build` had finished; the re-run was clean.
