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
