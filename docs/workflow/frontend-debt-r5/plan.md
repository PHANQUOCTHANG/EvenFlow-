# Plan: frontend-debt-r5

Inputs: `spec.md` (AC-1..AC-12, waiting for human approval). Base develop @ c5fb598. Web only (`apps/web`).
Two work packages that touch disjoint files, done one after the other by one implementer (WP-A is the risky one
and goes first so review can start early). No parallel split: the test harness and the gate script are shared.

## 1. Changes per AC

### WP-A: refresh and session (`lib/api-client.ts`, `lib/session.ts`)

| AC | File(s) | Change |
|---|---|---|
| AC-2 | `lib/session.ts` | New exports: `markRefreshUsed(token)`, `isRefreshUsed(token)` over sessionStorage `ef:refresh:used` (JSON array of FNV-1a hashes, newest last, capped at 16; parse errors and storage errors -> treated as empty, never throw). New `clearSessionHoldingRefresh(token)`: for each of local/session store, `wipe()` only if that store's `ef_refresh === token`; if anything was wiped, `sessionEpoch += 1` and `notify()`. It never touches buy-flow keys or `ef:lastuid`. |
| AC-2 | `lib/api-client.ts` | `refreshCore`: call `markRefreshUsed(refreshToken)` as soon as a `Response` exists with status 2xx/400/401/403, and on the abort path. Not on fetch rejection, 5xx, 429. `doRefresh`: after the user check, the no-token branch and the "different token -> reuse" branch, and before `refreshCore`: if `isRefreshUsed(cur.refreshToken)` -> `clearSessionHoldingRefresh(cur.refreshToken)` and throw `AUTH_REQUIRED` (no POST). |
| AC-1 | `lib/api-client.ts` | In `refreshPost`, wrap `res.json()` and the object check (`body !== null && typeof body === "object" && !Array.isArray(body)`) in try/catch. If the abort signal fired, rethrow so `refreshCore` keeps the abort outcome. Otherwise go to the shared failure resolver (below). |
| AC-3 | `lib/api-client.ts` | One helper `resolveFailedRefresh(epoch, cur, problem?)` used by the 400/401/403 branch, the abort branch and the AC-1 branch: (1) `isSamePostedSession` -> `clearSession()` and throw `AUTH_REQUIRED`; (2) else re-read `peekSession()`: if `epoch === getSessionEpoch()`, `stored.user.id === cur.user.id`, `stored.refreshToken` is non-null and differs from `cur.refreshToken` -> return `stored.accessToken`; (3) else throw `AUTH_REQUIRED` without clearing. `refreshCore`'s abort branch returns the helper's value instead of always throwing. |
| AC-4 | `lib/api-client.ts` | In `apiFetch`, right after the proactive block re-reads `session`: `if (!session && startUserId !== null) throw new ApiError("AUTH_REQUIRED", 401, {}, ...)`. |
| AC-5 | `lib/api-client.ts` | In `withRefreshLock`, after `await Promise.race(...)`: `if (typeof token !== "string") throw networkError()`. `finally` unchanged (deadline timer cleared). |
| AC-6 | `lib/session.ts` | `const ROLES = new Set(["buyer","organizer","moderator","ops"])`; `readFrom` returns `null` when `!ROLES.has(user.role)`. `sessionFromIdentity`: role = `res.role` if valid, else `prev?.role` if valid, else `"buyer"`. |

### WP-B: checkout and payment

| AC | File(s) | Change |
|---|---|---|
| AC-7, AC-8 | `components/checkout/order-result-view.tsx` | `authExpired = error?.type === "AUTH_REQUIRED"` (drop `order !== null`). Render the body `<p>` only when its text is not null. |
| AC-9 | `hooks/use-order-status.ts`, `order-result-view.tsx` | `useIssuedTickets`: new state `authRequired` set on `AUTH_REQUIRED` in the stop branch, reset in `retry()`; returned as `paid && !hasTickets && authRequired`. `failed` formula unchanged. View: if `issued.authRequired`, show `Alert` `checkout.authRequired` + sign-in `Link` instead of the ticketsFailed alert; skip it when the top error card already shows the AUTH prompt (one prompt only). |
| AC-10 | `lib/payment-client.ts` | New `safeRedirect(raw, fallback)`: `new URL(raw, base)` where base = `window.location.origin` or `http://localhost` without `window`; keep `raw` only if protocol is `http:` or `https:`; parse error -> fallback. Used where `redirectUrl` is built (~:294). |
| AC-11 | `hooks/use-hold-timer.ts`, `components/checkout/checkout-view.tsx`, `lib/i18n/messages.ts` | Hook: `cancelActiveHold(): Promise<boolean>` + `cancelling` flag (ref guard against double click). Calls `cancelHold(hold.holdId)`; success -> `resetHold()` unless the hold already expired (then keep `expired`); failure -> keep hold, return the `HoldClientError`. View: secondary `Button` "Huỷ giữ vé" under `OrderSummary` only when `holdState === "active" && hold`; local `cancelError` state: `AUTH_REQUIRED` -> existing sign-in alert, else alert `checkout.cancelHoldFailed`. i18n: add `checkout.cancelHold`, `checkout.cancelHoldFailed` in vi and en. |
| AC-12 | `docs/workflow/frontend-debt-r5/full-gate.sh` | Copy of r4 `full-gate.sh` with base c5fb598 and the frozen-test commit updated. |

## 2. Tests (written first, red on c5fb598 unless marked GUARD)

- `src/lib/debt5/ac1-refresh-body.test.ts`: 200 + non-JSON body; 200 + stream error; 200 + `null` body -> `AUTH_REQUIRED`, session cleared, next 401 sends no POST; GUARD: body read fails after a sign-out (epoch changed) -> nothing cleared.
- `src/lib/debt5/ac2-used-refresh.test.ts`: R2-3 repro (mixed persist) -> posts `["r1"]`, sessionStorage auth keys gone, localStorage untouched; R2-S1 repro; GUARD: fetch rejection then retry -> 2 POSTs of `r1`; GUARD: 503 then retry -> 2 POSTs; corrupt `ef:refresh:used` -> behaves as today; record never contains the raw token.
- `src/lib/debt5/ac3-same-user-reuse.test.ts`: X-4 repro (abort, owner + joiner get `acc-9`, 1 POST); R2-4 repro (401 -> `A:200`); AC-1 variant; GUARDS: other user stored -> `AUTH_REQUIRED`, stored session untouched, original request not retried; same-tab sign-out + same-user login (epoch changed) -> `AUTH_REQUIRED`.
- `src/lib/debt5/ac4-ac5-no-bearerless-send.test.ts`: X-S1 repro -> `AUTH_REQUIRED`, no request without `Authorization`; X-S2 repro -> `NETWORK_ERROR`, bearers `["Bearer old"]` only, timer count back to start.
- `src/lib/debt5/ac6-role-validation.test.ts`: X-14 repro (`admin` -> `null`), tampered role, `getStoredRefreshToken()` still returns the token, `logout()` POSTs it; `sessionFromIdentity` unknown role with/without valid prev; GUARD: all four known roles accepted.
- `src/components/checkout/order-result-view.debt5.test.tsx`: R3-1 repro (no order + 401 -> sign-in link, no "Thử lại", retry count stays 1 GET); R3-2 (no empty `p`); R3-3 repro (PAID + issue 401 -> sign-in link, no "Lấy vé lại"); exactly one sign-in link when both fail; GUARD: network error keeps "Thử lại".
- `src/lib/debt5/ac10-safe-redirect.test.ts` + `payment-handoff-view` case: `javascript:`, `data:`, `vbscript:`, unparsable -> default result URL, `window.location.href` never set to it; GUARD: `https://pay.test/x` and `/checkout/...` kept.
- `src/components/checkout/checkout-view.debt5.test.tsx`: button only in active state; click -> 1 DELETE with Idempotency-Key, double click still 1; success -> idle, `getActiveHold` null; 401 -> sign-in alert, hold kept; network -> `cancelHoldFailed`, hold kept; hold expires during DELETE -> expired state.

Traceability goes in `traceability.yaml`; baseline (file/test counts at c5fb598) in `baseline.json`.

## 3. Order

WP-A: AC-6 -> AC-2 -> AC-1 -> AC-3 (AC-1 and AC-3 share the resolver, AC-2 must exist first so AC-1's failure
marks the token) -> AC-4 -> AC-5. WP-B: AC-10 -> AC-7/AC-8 -> AC-9 -> AC-11. Commit and **push** after each AC
(user rule: push as you go).

## 4. Risks

| Risk | Mitigation |
|---|---|
| Existing tests reuse the same refresh string (`"r1"`) across `it` blocks in one module instance and expect a POST each time (e.g. `api-client.contract.test.ts`, protected) | The record lives in sessionStorage, which `setupEnv()` clears before every test, not in module memory. Run the full suite after AC-2; any other failure caused only by token reuse across tests goes to a test-change request, never a code workaround that weakens AC-2 |
| jsdom "tabs" in `lib/debt/harness.ts` share one sessionStorage, real tabs do not | The record only blocks a POST of a token that this or a sibling tab already sent, which is reuse anyway; r4 tests (`lib/debt/*`, `fix2/g1-lock-refresh`) must stay green unchanged |
| AC-3 hands a joiner the token of a session it did not start with | Reuse only for the same user id and unchanged epoch, the same rule as the success path; GUARD tests for other user and epoch change; mutation (5) |
| AC-2 wipes a store too eagerly | `clearSessionHoldingRefresh` wipes only stores whose `ef_refresh` equals the used token; test that the other store survives; mutation (2) |
| AC-6 signs out a real user if the backend adds a role | Backend roles are exactly the four in `services/identity/internal/domain/identity.go:20-23`; a new role must update `Role` anyway |
| AC-1 turns a flaky proxy (200 with truncated body) into a logout | Deliberate: the server rotated the token on 2xx, so the old one must not be POSTed again; same trade-off the human accepted for the 120 s abort |
| AC-11 releases seats by an accidental click | Secondary styling, separate from the primary action; question 3 in spec asks the human about a confirm dialog |
| AC-10 blocks a legitimate provider redirect | Only non-http(s) schemes are rejected; all provider URLs are https |

## 5. Rollback

One PR with a merge commit; `git revert -m 1 <merge>`. No data, no backend, no migration. The only new
persisted state is sessionStorage `ef:refresh:used`, which older code ignores. Check after the revert: `npm test`,
`npx tsc --noEmit`, lint and `npm run build` pass.

## 6. Mutation test (threshold 10/10)

Each mutation must make at least one new test fail:

1. Remove the `isRefreshUsed` check in `doRefresh` (R2-3 / R2-S1 repro posts `r1` twice).
2. `clearSessionHoldingRefresh` wipes both stores unconditionally (other-store survival test).
3. Mark the token used on fetch rejection too (offline-retry GUARD).
4. Drop the try/catch around `res.json()` in `refreshPost` (AC-1 returns `NETWORK_ERROR`).
5. In `resolveFailedRefresh`, drop the `stored.user.id === cur.user.id` check (other-user GUARD).
6. In `resolveFailedRefresh`, drop the epoch check (same-tab re-login GUARD).
7. Remove the AC-4 throw (`!session && startUserId`) in `apiFetch`.
8. Remove the `typeof token !== "string"` check in `withRefreshLock`.
9. `readFrom` accepts any string role again.
10. `safeRedirect` returns `raw` for every scheme.
11. (WP-B, counted separately, 3/3) restore `order !== null` in `authExpired`; drop the `issued.authRequired` branch in the view; call `resetHold()` before `cancelHold` resolves.
