# Plan: frontend-debt-r4

Inputs: `spec.md` (AC-1..AC-7, human-approved 2026-10-21). Base develop @ 43375eb. Web only.
Single work package (all changes are small and touch overlapping auth files, so no parallel split).

## 1. Changes per AC

| AC | File(s) | Change |
|---|---|---|
| AC-1 | `lib/api-client.ts` | Capture `sentUserId` with `usedRefresh` when the request is sent. In `doRefresh` / `refreshCore`, reuse `cur.accessToken` only if `cur.user.id === sentUserId`; else throw `AUTH_REQUIRED` WITHOUT `clearSession` (the new session belongs to someone else) and without POST. `apiFetch` must not retry the original request in that case. |
| AC-2 | `lib/api-client.ts` | Split the caller's wait from the POST: the single in-flight refresh promise stays shared, and each caller races it against its own 45 s timer that rejects `NETWORK_ERROR` with the session kept. The refresh fetch gets `AbortSignal.timeout(REFRESH_POST_TIMEOUT_MS = 120_000)`. An abort maps to `AUTH_REQUIRED` + `clearSession()`, and the old refresh token is never re-POSTed. The lock is released when the POST settles. |
| AC-3 | `lib/api-client.ts` | Build the lock request inside `try` (wrap `navigator.locks.request` in `new Promise` / try). Any non-`ApiError` becomes `networkError()`, including a synchronous throw. `finally` clears the deadline timer. A `SecurityError` falls back to in-tab single-flight. |
| AC-4 | `lib/session.ts`, `hooks/use-auth.ts` | `reconcileBuyFlowOwner` prefers the tab-local `ef:lastuid`. Reconciliation also runs in `getSession()` (stored user id ≠ `ef:lastuid` → `clearBuyFlowStorage()` + update lastuid) and in a once-per-tab `storage` listener for `ef_user`. |
| AC-5 | `edit-event-client.tsx` (+ `event-wizard.tsx` if needed) | After `current = await reload()`, match still-unsaved tiers by trimmed name to `current.tiers`. A match counts as created (map both ids), so the tier is not POSTed and only a differing quota is sent. A 422 duplicate-name from `addTier` triggers the same lookup. |
| AC-6 | `lib/payment-client.ts`, payment page client, `order-result-view.tsx` / result page client | Add `FORBIDDEN` to `PaymentClientErrorType`. Map 403 `/problems/forbidden` in `initiatePayment`, `getOrderDetails`, `issueTickets`. The UI shows `checkout.forbidden` and the polling hooks stop on FORBIDDEN. |
| AC-7 | — | Gates via `docs/workflow/frontend-real-api/full-gate.sh` adapted as `docs/workflow/frontend-debt-r4/full-gate.sh`, with the base and frozen-test commit updated. |

## 2. Order

AC-3 → AC-2 → AC-1 (same function, each builds on the previous) → AC-4 → AC-6 → AC-5. Commit and **push** after each AC
(user rule: push as you go).

## 3. Risks

| Risk | Mitigation |
|---|---|
| The 120 s abort logs out a user whose identity call is merely slow | Human accepted the trade-off; 120 s is far above normal latency; test at the 119/121 s boundaries |
| A caller timeout at 45 s while the POST later succeeds | The successful POST still stores the rotated pair (shared promise), and the next request uses it; test it |
| The `storage` listener firing during an in-flight refresh | Reconciliation only touches buy-flow keys, never auth keys; test it |
| Name matching for tiers | Names are unique per event (backend 422 rule); trim before comparing |

## 4. Rollback

One PR with a merge commit; `git revert -m 1 <merge>`. No data, no backend. Check: after the revert, `npm test` and
`npm run build` pass.

## 5. Mutation test (threshold 5/5)

(1) Drop the `sentUserId` check. (2) Remove the per-caller 45 s race. (3) Remove the 120 s abort, or map the abort to
NETWORK_ERROR without clearing. (4) Rethrow the raw locks error. (5) Compare against `getSession()` before
`ef:lastuid`. Each mutation must fail at least one test.

## 6. Round 2 (plan-review.md round 1: FAIL, B-1 + M-1..M-4 + m-1..m-7)

| Finding | Resolution |
|---|---|
| B-1 Z-1b expects the holder's own caller to get 200 after 60 s | spec AC-2 clarified: the 45 s limit applies to callers that did NOT start the POST (later same-tab joiners and other tabs waiting on the lock); the POST owner is bounded by the 120 s abort. Z-1b unchanged; no test-change request |
| M-1 race placement / `inflight` lifetime | The 45 s race wraps only the *joining* caller's `await` (outside the shared promise). `inflight` is cleared only in the POST's own `finally`, never on a caller timeout, on every path (lock, no-locks, SecurityError fallback). The lock-wait deadline is disarmed once the lock is granted. Test: caller times out at 45 s, a new call at 50 s while the POST is still pending joins the same `inflight` (still 1 POST). Mutation (6): clear `inflight` on caller timeout |
| M-2 unguarded clear on 120 s abort | On abort, `clearSession()` only if `sessionEpoch` is unchanged AND the stored refresh token still equals the one that was POSTed (same guard as `api-client.ts:136`); otherwise reject `AUTH_REQUIRED` and leave the newer session. Mutation (7): unguarded clear |
| M-3 `AbortSignal.timeout` vs fake timers | Use `new AbortController()` + `setTimeout(abort, REFRESH_POST_TIMEOUT_MS)`, kept armed through `res.json()`, cleared in `finally`; classify the outcome by `controller.signal.aborted`, not by error name. Test stubs honour `init.signal` |
| M-4 name match can hit an unrelated tier | Match only server tiers not already referenced by `existingTierIds` or the created map, with the same `price_cents`, comparing `trim().toLowerCase()` (backend uses EqualFold). If there is no unique match, show the server's 422 detail |
| m-1 inflight shared across users | Key the in-flight refresh by `sentUserId`; a caller with a different user does not join another user's promise |
| m-2 recursion | Reconciliation in the read path uses the low-level reader (`readFrom`), writes `ef:lastuid` only when it differs, and never calls `notify()` or `setSession` |
| m-3 per-error expectations | SecurityError → in-tab single-flight (1 POST). InvalidStateError / NotSupportedError / AbortError / sync TypeError → `NETWORK_ERROR`, 0 POST, `vi.getTimerCount()` unchanged |
| m-4 AC-6 files | `hooks/use-order-status.ts` (order polling and ticket issuing stop on FORBIDDEN), `components/checkout/order-result-view.tsx` (no retry button for FORBIDDEN), `components/checkout/payment-handoff-view.tsx`, `lib/payment-client.ts` |
| m-5 AC-5 trigger | Any 422 from `addTier` → reload + the M-4 lookup; no match → show the server detail |
| m-6 mutations | Threshold becomes 8/8: the original 5 plus (6) clear `inflight` on caller timeout, (7) unguarded clear on abort, (8) name match without the referenced/price filter |
| m-7 test files and rollback | Tests: `src/lib/debt/ac1-user-switch.test.ts`, `ac2-refresh-deadlines.test.ts`, `ac3-locks-errors.test.ts`, `ac4-cross-tab-owner.test.ts`, `ac6-payment-forbidden.test.ts`, organizer `edit/page.debt.test.tsx`, checkout `*.debt.test.tsx`. Rollback check adds `npx tsc --noEmit` and lint |
