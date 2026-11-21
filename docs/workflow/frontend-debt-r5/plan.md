# Plan: frontend-debt-r5

Inputs: `spec.md` (AC-1..AC-12, human-approved 2026-11-01; AC-11 and AC-6 clarified after plan review round 1). Base develop @ c5fb598. Web only (`apps/web`).
Two work packages that touch disjoint files, done one after the other by one implementer (WP-A is the risky one
and goes first so review can start early). No parallel split: the test harness and the gate script are shared.

## 1. Changes per AC

### WP-A: refresh and session (`lib/api-client.ts`, `lib/session.ts`)

| AC | File(s) | Change |
|---|---|---|
| AC-2 | `lib/session.ts` | New exports: `markRefreshUsed(token)`, `isRefreshUsed(token)` over sessionStorage `ef:rt:used` (JSON array of FNV-1a hashes, newest last, capped at 16; parse errors and storage errors -> treated as empty, never throw). New `clearSessionHoldingRefresh(token)`: for each of local/session store, `wipe()` only if that store's `ef_refresh === token`; if anything was wiped, `sessionEpoch += 1` and `notify()`. It never touches buy-flow keys or `ef:lastuid`. |
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
| AC-11 | `hooks/use-hold-timer.ts`, `components/checkout/checkout-view.tsx`, new `components/checkout/cancel-hold-dialog.tsx`, `lib/i18n/messages.ts` | **Superseded in part by §7 (P-1, P-7): the trigger only opens the confirmation dialog.** Hook: `cancelActiveHold(): Promise<boolean>` + `cancelling` flag (ref guard against double click). Calls `cancelHold(hold.holdId)`; success -> `resetHold()` unless the hold already expired (then keep `expired`); failure -> keep hold, return the `HoldClientError`. View: secondary `Button` "Huỷ giữ vé" under `OrderSummary` only when `holdState === "active" && hold`; local `cancelError` state: `AUTH_REQUIRED` -> existing sign-in alert, else alert `checkout.cancelHoldFailed`. i18n: add `checkout.cancelHold`, `checkout.cancelHoldFailed` in vi and en. |
| AC-12 | `docs/workflow/frontend-debt-r5/full-gate.sh` | Copy of r4 `full-gate.sh` with base c5fb598 and the frozen-test commit updated. |

## 2. Tests (written first, red on c5fb598 unless marked GUARD)

- `src/lib/debt5/ac1-refresh-body.test.ts`: 200 + non-JSON body; 200 + stream error; 200 + `null` body -> `AUTH_REQUIRED`, session cleared, next 401 sends no POST; GUARD: body read fails after a sign-out (epoch changed) -> nothing cleared.
- `src/lib/debt5/ac2-used-refresh.test.ts`: R2-3 repro (mixed persist) -> posts `["r1"]`, sessionStorage auth keys gone, localStorage untouched; R2-S1 repro; GUARD: fetch rejection then retry -> 2 POSTs of `r1`; GUARD: 503 then retry -> 2 POSTs; corrupt `ef:rt:used` -> behaves as today; record never contains the raw token.
- `src/lib/debt5/ac3-same-user-reuse.test.ts`: X-4 repro (abort, owner + joiner get `acc-9`, 1 POST); R2-4 repro (401 -> `A:200`); AC-1 variant; GUARDS: other user stored -> `AUTH_REQUIRED`, stored session untouched, original request not retried; same-tab sign-out + same-user login (epoch changed) -> `AUTH_REQUIRED`.
- `src/lib/debt5/ac4-ac5-no-bearerless-send.test.ts`: X-S1 repro -> `AUTH_REQUIRED`, no request without `Authorization`; X-S2 repro -> `NETWORK_ERROR`, bearers `["Bearer old"]` only, timer count back to start.
- `src/lib/debt5/ac6-role-validation.test.ts`: X-14 repro (`admin` -> `null`), tampered role, `getStoredRefreshToken()` still returns the token, `logout()` POSTs it; `sessionFromIdentity` unknown role with/without valid prev; GUARD: all four known roles accepted.
- `src/components/checkout/order-result-view.debt5.test.tsx`: R3-1 repro (no order + 401 -> sign-in link, no "Thử lại", retry count stays 1 GET); R3-2 (no empty `p`); R3-3 repro (PAID + issue 401 -> sign-in link, no "Lấy vé lại"); exactly one sign-in link when both fail; GUARD: network error keeps "Thử lại".
- `src/lib/debt5/ac10-safe-redirect.test.ts` + `payment-handoff-view` case: `javascript:`, `data:`, `vbscript:`, unparsable -> default result URL, `window.location.href` never set to it; GUARD: `https://pay.test/x` and `/checkout/...` kept.
- `src/components/checkout/checkout-view.debt5.test.tsx` (**rewritten in §7 P-1**): button only in active state; confirm -> 1 DELETE with Idempotency-Key, double click on confirm still 1; success -> idle, `getActiveHold` null; 401 -> sign-in alert, hold kept; network -> `cancelHoldFailed`, hold kept; hold expires during DELETE -> expired state.

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
| AC-11 releases seats by an accidental click | Human-required confirmation dialog (§7 P-1); secondary styling, separate from the primary action |
| AC-10 blocks a legitimate provider redirect | Only non-http(s) schemes are rejected; all provider URLs are https |

## 5. Rollback

One PR with a merge commit; `git revert -m 1 <merge>`. No data, no backend, no migration. The only new
persisted state is sessionStorage `ef:rt:used`, which older code ignores. Check after the revert: `npm test`,
`npx tsc --noEmit`, lint and `npm run build` pass.

## 6. Mutation test (threshold 10/10; extended in §7 P-8)

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

## 7. Round 2 (plan-review.md round 1: FAIL, P-1 major + P-2..P-8 minor)

| Finding | Resolution |
|---|---|
| P-1 AC-11 has no confirmation dialog | spec AC-11 and §4 amended to match the approval line (a clarification, not a new decision). New local `components/checkout/cancel-hold-dialog.tsx` built on native `<dialog>` opened with `showModal()` (no dependency): `role="alertdialog"`, `aria-modal="true"`, `aria-labelledby` (title), `aria-describedby` (body), initial focus on "Giữ lại" (the safe choice), focus returned to the trigger on close. Esc (the dialog `cancel` event) and "Giữ lại" close it without DELETE. The confirm button is the ONLY caller of `cancelActiveHold`. jsdom has no `showModal`: the component falls back to the `open` attribute when `showModal` is not a function (unit tests use that path, Playwright the real one). The view closes the dialog in an effect when `holdState !== "active"`. i18n vi+en: `checkout.cancelHold` ("Huỷ giữ vé" / "Release tickets"), `checkout.cancelHoldConfirmTitle` ("Huỷ giữ vé?" / "Release these tickets?"), `checkout.cancelHoldConfirmBody`, `checkout.cancelHoldConfirm` ("Huỷ giữ vé" / "Release"), `checkout.cancelHoldKeep` ("Giữ lại" / "Keep tickets"), `checkout.cancelHoldFailed`. Tests: click the trigger -> an `alertdialog` is shown, 0 DELETE; "Giữ lại" -> closed, 0 DELETE, hold kept; Esc -> closed, 0 DELETE; confirm -> exactly 1 DELETE with Idempotency-Key; double click on confirm -> still 1; hold expires while the dialog is open -> dialog gone, 0 DELETE, expired state; focus is inside the dialog when open and back on the trigger after "Giữ lại". The success / 401 / network / expiry-during-DELETE cases all start from confirm. Mutation W-1: trigger calls `cancelActiveHold` directly (confirm bypassed). Mutation W-2: dialog not closed on expiry. The stale "question 3" note (§4) and "waiting for human approval" note (header) are removed |
| P-2 used check placed after the reuse branch | `doRefresh` order becomes: user check -> no-token branch -> `isRefreshUsed(cur.refreshToken)` (wipe + `AUTH_REQUIRED`, no POST) -> reuse branch -> `refreshCore`. `resolveFailedRefresh` step (2) also requires `!isRefreshUsed(stored.refreshToken)`. The success hand-off is unaffected (the new r2 is not marked). Test: request sent with r2, another tab clears localStorage, the used r1 copy surfaces from sessionStorage -> `AUTH_REQUIRED`, 0 POST, r1 copy wiped, no raw 401 returned. Mutation M-11: move the check back after the reuse branch |
| P-3 epoch bumped when only a shadowed store is wiped | `clearSessionHoldingRefresh(token)` notes which store `peekSession()` would return BEFORE wiping, and bumps `sessionEpoch` only if that active store was wiped; it always calls `notify()` when anything was wiped. GUARD test: live u-2 session in localStorage with its refresh in flight, used u-1/r1 copy in sessionStorage wiped meanwhile -> u-2's 2xx result is stored, not discarded. Mutation M-12: bump the epoch unconditionally |
| P-4 16-entry cap can evict a stale copy | Option (a): on the abort / 400-401-403 / AC-1 paths, when `isSamePostedSession` decided NOT to clear, call `clearSessionHoldingRefresh(postedToken)` right after marking, so any store still holding the posted token is wiped at that moment (P-3 rule applies; in that branch the active store holds the posted token only after a same-tab rewrite, R2-S1, where wiping it is the intended outcome). The lazy wipe in `doRefresh` stays as a second line. Record rules: re-marking an existing hash moves it to the newest slot (no duplicates); eviction drops the oldest. Tests: in the R2-3 repro the sessionStorage copy is gone right after the 120 s abort; 17 marks -> the first is evicted and 16 are kept; a re-mark moves to newest. Spec §5 records the eager wipe |
| P-5 which responses mark, hash width, timing | Mark exactly on 2xx, 400, 401, 403, the 120 s abort and the AC-1 body failure. Never on fetch rejection, 404, 409, 413, 415, 422 or any other 4xx, 429, 5xx (gateway 502/504 after a rotation stays the X-15 residual). Hash: 32-bit FNV-1a, lowercase hex (same shape as `payment-client.ts:150`, local copy in `session.ts`). `markRefreshUsed` runs synchronously right after `fetch` resolves with a marking status, before any `await` on `res.json()` / `readProblem`, so an abort during the body read finds the token already marked. GUARD tests: 404 and 429 do not mark (a later 401 still POSTs once); abort during `res.json()` -> token marked, abort outcome kept |
| P-6 invalid role falls through; previous-role fallback | New internal `readStore(s)` returns absent / invalid / ok(session); "present" means both `ef_token` and `ef_user` exist. `peekSession`, `getSessionPersist` and the fallback read in `reconcileBuyFlowOwner` stop at the first present store: invalid localStorage session -> `null`, no fall-through to sessionStorage. `getStoredRefreshToken` unchanged (logout still revokes). `sessionFromIdentity`: role = valid `res.role`, else `prev.role` only if valid AND `prev.id === resolved id`, else `"buyer"`. Tests: tampered local + valid session store -> `getSession()` null and the request has no `Authorization`; prev role from a different user id -> `buyer`. Mutations M-13: fall through to sessionStorage on invalid; M-14: drop the prev id check |
| P-7 AC-11 hook details | `cancelActiveHold` reads hold and state through refs (`holdRef`, `holdStateRef`, updated every render), never the stale closure. After DELETE success: `setHoldState(s => s === "expired" ? s : "idle")`, and `setHold(null)` / `clearActiveHold` only when not expired. A `mountedRef` drops results after unmount. `cancelling` (state + `cancellingRef` guard) disables the primary OrderSummary action (`checkout-view.tsx:109`, through a new optional `primaryDisabled` prop on `OrderSummary`) and the tier cards. The view routes `cancelError?.type === "AUTH_REQUIRED"` into the existing `isAuthRequired` alert (`checkout-view.tsx:125,156`): exactly one sign-in prompt. `cancelHold` itself is unchanged. Tests: primary action disabled while cancelling; unmount during DELETE -> no update after unmount; 401 -> exactly one sign-in link |
| P-8 test and mutation gaps | Added tests: Google X-15 edge in `src/lib/debt5/ac2-used-refresh.test.ts` (google 200, devices 200, refresh 200 with a bad body -> the restored Google session works until a 401 token-expired, then `AUTH_REQUIRED` with exactly 1 refresh POST in total); the record survives `vi.resetModules()` + re-import (same sessionStorage); lock re-read with a used token in one store and a different token in the other wipes only the first; a joiner of a used-token refresh gets `AUTH_REQUIRED` with 0 POST. Added mutations: M-15 no marking on 2xx; M-16 no marking on abort; M-17 resolver step (2) without the "different refresh token" check; M-18 AC-1 without the abort rethrow (abort during the body read must keep the abort outcome); W-1 and W-2 from P-1. Final set: WP-A M-1..M-18 (all must fail, 18/18); WP-B §6 item 11 (3) + W-1 + W-2 (5/5) |
