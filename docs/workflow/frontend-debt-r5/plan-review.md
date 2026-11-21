# Plan review: frontend-debt-r5 (v3 step 2b, High-risk)

Reviewer: plan reviewer, clean context. Inputs: `spec.md` (human-approved 2026-11-01), `plan.md`, `tier.json`,
read-only checks against `apps/web/src` at c5fb598 and the source findings in `docs/workflow/frontend-real-api` and
`docs/workflow/frontend-debt-r4`.

## Verdict: FAIL

One major finding (P-1): the plan does not implement the confirmation dialog that the human approval made
mandatory for AC-11, and its AC-11 tests assert the opposite behaviour (click sends DELETE). Everything else is
minor and can be folded in during implementation. Once P-1 is fixed the plan can PASS without another full round,
provided the reviewer of the revised plan checks only P-1 (and, if accepted, P-2/P-3).

The refresh invariants were checked and the plan keeps them:

- One POST per refresh token: `markRefreshUsed` / `isRefreshUsed` are only added on top; no new POST site.
- POST only inside the Web Lock: the `isRefreshUsed` check and the marking both run in `doRefresh` / `refreshPost`,
  which only run inside the lock callback or the existing `locksUnusable` single-flight path
  (`api-client.ts:204`, `:237-241`).
- User-id check at every stored-token return point: the new return in `resolveFailedRefresh` step (2) checks
  `stored.user.id === cur.user.id` and the epoch (plan.md:16), the same rule as `api-client.ts:183-187`.
- 45 s joiners / 120 s owner abort with the posted-session guard: unchanged (`joinInflight`, `REFRESH_POST_TIMEOUT_MS`);
  step (1) of the resolver is the existing `isSamePostedSession`.
- No session resurrection after logout: the resolver never calls `setSession`; same-tab logout bumps the epoch and
  blocks step (2); other-tab logout leaves no stored session so step (2) cannot match.
- Backend rotates refresh tokens (`services/identity/internal/adapter/postgres/refresh_token_repo.go:45` `Rotate`),
  so marking the posted token as used on 2xx cannot lock out a server that returns the same refresh token.

## Findings

### P-1 (major) AC-11 has no confirmation dialog; tests encode the unapproved behaviour

- Evidence: `spec.md:142` "AC-11: the 'Huỷ giữ vé' button REQUIRES a confirmation dialog before sending DELETE."
  `plan.md:28` (button click calls `cancelActiveHold` directly), `plan.md:40` ("click -> 1 DELETE ..., double click
  still 1"), `plan.md:60` (risk mitigation still says "question 3 in spec asks the human about a confirm dialog").
  The spec body still lists the dialog as out of scope (`spec.md:112`) and AC-11 text (`spec.md:78-79`) says
  "When the buyer clicks it, Then DELETE ... is sent", which contradicts the approval line. There is no dialog
  component in `apps/web/src/components/ui` (alert, badge, button, card, checkbox, field, input, radio, select,
  spinner, toast only) and no new dependency is allowed (`tier.json` `no_new_dependencies`).
- Fix:
  1. Plan: add a confirmation step. Clicking "Huỷ giữ vé" opens an accessible dialog (`role="alertdialog"`,
     `aria-modal="true"`, labelled title + body, focus moved into the dialog and returned to the trigger, Esc and
     "Giữ lại" cancel, the confirm button is the only path to `cancelActiveHold`). Build it from native
     `<dialog>` or a small local component; no dependency. Add i18n keys (e.g. `checkout.cancelHoldConfirmTitle`,
     `checkout.cancelHoldConfirmBody`, `checkout.cancelHoldConfirm`, `checkout.cancelHoldKeep`) in vi and en.
     The dialog closes (without DELETE) if the hold leaves the `active` state while it is open (expiry).
  2. Tests (`checkout-view.debt5.test.tsx`): click -> dialog shown, 0 DELETE; cancel / Esc -> 0 DELETE, hold kept;
     confirm -> exactly 1 DELETE with Idempotency-Key; double-click on confirm -> still 1; hold expires while the
     dialog is open -> dialog closes, 0 DELETE, expired state. Keep the existing success / 401 / network / expiry
     during DELETE cases, all starting from "confirm".
  3. Mutation (WP-B): "confirm step bypassed (trigger calls `cancelActiveHold` directly)" must fail a test.
  4. Spec hygiene: amend `spec.md` AC-11 and §4 so the text matches the approval line (record it as a
     spec clarification, not a new decision), and drop the stale "question 3" note at `plan.md:60` and the
     "waiting for human approval" note at `plan.md:3`.

### P-2 (minor) AC-2 check is placed after the "different token -> reuse" branch

- Evidence: `plan.md:14` puts `isRefreshUsed(cur.refreshToken)` after the reuse branch at `api-client.ts:130`
  (`if (usedRefresh && cur.refreshToken !== usedRefresh) return cur.accessToken;`). Case: request sent with pair r2
  (localStorage), another tab clears localStorage, the tab-local sessionStorage copy holding the already-used r1
  surfaces; `doRefresh(r2)` sees `cur.refreshToken = r1 !== r2` and returns the stale access token, `apiFetch`
  retries once and hands the caller a raw 401 `Response`. No second POST (invariant holds), but AC-2 says "any
  later refresh in this tab finds r as the stored refresh token ... no POST, the stores ... wiped, AUTH_REQUIRED",
  and the stale used copy stays in storage.
- Fix: run the used check right after the no-token branch and before the reuse branch:
  `if (isRefreshUsed(cur.refreshToken)) { clearSessionHoldingRefresh(cur.refreshToken); throw AUTH_REQUIRED }`.
  This does not break the success hand-off: after a 2xx the posted r1 is marked but the stored token is the new r2,
  which is not marked, so joiners/later callers still take the reuse branch. Add one test for this ordering.
  Apply the same rule in `resolveFailedRefresh` step (2): do not hand out a stored pair whose refresh token
  `isRefreshUsed`.

### P-3 (minor) `clearSessionHoldingRefresh` always bumps the epoch, even when it only wipes a shadowed store

- Evidence: `plan.md:13` ("if anything was wiped, `sessionEpoch += 1` and `notify()`"). The epoch is the F-12 signal
  that makes every in-flight refresh in the tab discard its 2xx (`api-client.ts:183`). If the wiped store is not the
  one `peekSession()` returns (e.g. the stale sessionStorage copy under a live localStorage session), bumping the
  epoch discards a concurrent, successful refresh of the live session: the server already rotated its token, the
  new pair is dropped, the old token is now marked used, and the next refresh wipes the live session (forced logout
  of a session that did nothing wrong). With the plan's lazy wipe in `doRefresh` the wiped store is normally the
  active one, so this is an edge, but the helper is generic.
- Fix: bump the epoch only when the store that `peekSession()` returned before the wipe was wiped (or when the
  active user changes as a result); always `notify()`. Add a GUARD test: live u-2 session in localStorage refreshing,
  used u-1/r1 copy in sessionStorage wiped -> u-2's refresh result is kept.

### P-4 (minor) 16-entry cap lets a long-lived stale copy be POSTed again

- Evidence: `plan.md:13` cap 16, newest last. R2-3 scenario (`frontend-debt-r4/findings-round2-challenge.json`
  R2-3): stale u-1/r1 in sessionStorage under a live u-2 localStorage session in the same tab. Every u-2 refresh in
  this tab is marked too; with 15-minute access tokens, 16 refreshes (about 4 h) evict r1's hash, and an u-2 logout
  in another tab then makes this tab POST r1 again. Consequence is the same as R2-3 (server reuse detection revokes
  the family, self-healing), so not a blocker.
- Fix (pick one, record it): (a) wipe the store(s) still holding the posted token at the moment of marking on the
  abort / 4xx / AC-1 paths when the guard did not clear (R2-3's own suggested fix; must respect P-3), so no stale
  copy outlives its record; or (b) keep lazy wipe and document the eviction residual in the spec's §5. Also specify:
  re-marking an existing hash moves it to the newest slot (no duplicates), eviction drops the oldest; add a test
  with 17 entries.

### P-5 (minor) AC-2 "response" set and hash details should be pinned in the plan

- Evidence: `spec.md` AC-2 lists 2xx, 400/401/403, AC-1 body failure and the 120 s abort as "used"; fetch rejection,
  5xx and 429 as not used. Other 4xx (404, 409, 413, 415, 422) are unspecified; `plan.md:14` marks only the listed
  ones, which is the safer reading (gateway 404 / misroute did not consume the token). Gateway 502/504 after the
  identity service already rotated is the X-15 lost-response case and stays a known residual. Hash: FNV-1a over
  a high-entropy token in the same storage area that already holds raw tokens has no privacy cost; a 32-bit hash
  collision across 16 entries is about 4e-9 per check and fails closed (logout), acceptable.
- Fix: state in the plan that only 2xx/400/401/403/abort/AC-1 mark (other 4xx do not), name the hash width
  (32-bit FNV-1a, hex), and add a GUARD test that 404 and 429 do not mark. Make sure marking happens before any
  `await` on the body (`res.json()` / `readProblem`) so an abort during the body read still finds the token marked.

### P-6 (minor) AC-6: an invalid role in localStorage falls through to sessionStorage

- Evidence: `session.ts:88-89` `peekSession = readFrom(local) ?? readFrom(session)`. Plan (`plan.md:19`) makes
  `readFrom` return `null` for an unknown role, so with a tampered `ef_user` in localStorage and any session in
  sessionStorage (possibly another user, or the used stale copy) `peekSession()` returns the sessionStorage session
  instead of `null` as AC-6 requires. `getSessionPersist` (`session.ts:93-96`) and `reconcileBuyFlowOwner`
  (`session.ts:123`) inherit the same fall-through.
  RoleGuard/use-auth are otherwise fine: `useAuth.loadAuth` uses `getSession()` (`use-auth.ts:51`), so an unknown
  role yields `isAuthenticated: false` and `RoleGuard` redirects to sign-in (`RoleGuard.tsx:26-28`), never to a
  privileged page; `logout()` reads `getStoredRefreshToken()` (`auth.ts:129`), which the plan leaves untouched.
- Fix: have the reader distinguish "absent" from "present but invalid" and stop at the first store that holds
  `ef_token`+`ef_user` (invalid -> `null`, no fall-through). Test: tampered local + valid session store ->
  `getSession()` null, no `Authorization` header.
- Also: `sessionFromIdentity` uses `res.id ?? prev?.id` (`session.ts:259`); take the fallback role from `prev` only
  when `prev.id` equals the resolved id, otherwise `buyer`. Login passes no `prev` (`auth.ts:56`), so this only
  matters for refresh bodies.

### P-7 (minor) AC-11 hook details

- Evidence: `plan.md:28` "success -> resetHold() unless the hold already expired". `createHold`/`resetHold` are
  `useCallback`s over state (`use-hold-timer.ts:79-142`); reading `holdState` inside the async callback after the
  DELETE resolves sees the stale closure value.
- Fix: decide "expired vs idle" from a ref or a functional update (`setHoldState(s => s === "expired" ? s : "idle")`,
  and only clear the hold if not expired), ignore results after unmount, and disable the primary
  "Thanh toán" action (`checkout-view.tsx:109`) and tier selection while `cancelling`, so the buyer cannot move to
  payment with a hold that is being released. `cancelHold` already clears storage by holdId on 204/404/2xx
  (`hold-client.ts:275-297`) and maps a non-expired 401 to `AUTH_REQUIRED`; the view must route `cancelError`
  `AUTH_REQUIRED` into the existing `isAuthRequired` alert (`checkout-view.tsx:125`, `:156`) so there is one prompt.

### P-8 (minor) Test / mutation gaps

- Spec §5 Google edge (X-15): after an AC-1 failure inside `loginWithGoogle` the restored Google session
  (`auth.ts:113-115`) keeps working, then ends with `AUTH_REQUIRED` and no second POST. No test in `plan.md:33-40`.
  Add one.
- AC-2: add "record survives a module reload (same sessionStorage)", "lock re-read with a used token in one store and
  a different token in the other wipes only the first" (already listed as other-store survival, keep it explicit),
  and "joiner of a used-token refresh gets AUTH_REQUIRED with 0 POST".
- Mutations (`plan.md:69-83`) do not cover: marking on 2xx removed (R-3/AC-1 path), marking on abort removed,
  resolver step (2) without the "different refresh token" check, AC-1 without the abort rethrow (abort during body
  read must keep the abort outcome), and the AC-11 confirm bypass (P-1). Add them; keep the 10/10 threshold.

### P-9 (info) AC-10 and AC-7..AC-9 check out

- AC-10: `new URL(raw, base)` uses the same WHATWG parser the browser uses for `location.href`, so leading
  whitespace, embedded tab/newline (`java\nscript:`) and case variants all resolve to `javascript:` and are
  rejected. `https://pay.test/redirect` and `https://x` (contract test `payment-client.contract.test.ts:54,79,112`)
  are kept as `raw`, so the contract test passes untouched. Protocol-relative `//host/x` resolves to https and is
  kept, which AC-10 allows (host allow-list is the deferred server part of S-4).
- AC-7/AC-8: dropping `order !== null` (`order-result-view.tsx:59`) and skipping the empty `<p>` (`:91-99`) matches
  the spec; the retry button condition (`:109`) then hides "Thử lại" for AUTH_REQUIRED without an order as required.
- AC-9: `failed` formula untouched (`use-order-status.ts:220`), new `authRequired` reset in `retry()` (`:205-212`);
  the error card only renders for `order === null` or PENDING (`order-result-view.tsx:60`) while the ticket section is
  PAID-only, so the "one prompt" rule holds by construction; keep the explicit test.
- AC-4 / AC-5 changes are minimal and placed correctly (`api-client.ts:329-331`, `:249-253`).

## Round 2

Reviewer: plan reviewer, clean context. Inputs: `spec.md` (AC-6 and AC-11 clarified), `plan.md` §7, round 1 above,
read-only checks against `apps/web/src` at 5273691 (code unchanged since c5fb598), jsdom 25.0.1 in `apps/web/node_modules`.

### Verdict: PASS

All round 1 findings are resolved in the plan. No new blocker or major. The new minors below concern the AC-11
dialog mechanics and one missed call site for P-6. Fold them in during implementation; the step 4 reviewers should
check R2-1..R2-5 explicitly.

### Round 1 resolutions

| Finding | Status | Check |
|---|---|---|
| P-1 major | Resolved | spec AC-11 (spec.md:79-93) and §4 now require the dialog; plan §7 makes confirm the only caller of `cancelActiveHold`, adds the keys, the expiry-close effect, 0-DELETE tests for open / keep / Esc / expiry, and mutations W-1/W-2. Stale notes removed (plan.md header and §4 risk row). Mechanics gaps: R2-1..R2-4. |
| P-2 | Resolved | New order: user check -> no-token -> used check -> reuse (`api-client.ts:125-131`). Success hand-off unaffected: after a 2xx the stored r2 is not marked, so later in-tab callers with `usedRefresh=r1` still take the reuse branch. Resolver step (2) also rejects a used stored token. M-11 covers it. |
| P-3 | Resolved | Bump only if the active store was wiped; always `notify()`. Note: under the Web Lock no wipe can interleave with a POST (lazy and eager wipes both run inside the lock callback), so the GUARD test has to call `clearSessionHoldingRefresh` directly or use the `locksUnusable` path; fine, the helper is exported. Define "active store" with P-6's `readStore` (first present store), not "what `peekSession()` returns", so a present-but-invalid localStorage session counts as active (info, no behaviour risk). |
| P-4 | Resolved (option a) | Eager wipe after marking when the guard did not clear. Traced: R2-3 (u-2 live in local, u-1/r1 in session) -> session store wiped, no bump, resolver sees u-2 != u-1 -> `AUTH_REQUIRED`, posts stay `["r1"]`. R2-S1 (same pair rewritten after a same-tab clear) -> local holds r1 and is active -> wiped, bump. X-4/R2-4 (same user r9 in local) -> at most the shadowed r1 copy is wiped, no bump, step (2) returns acc-9. Move-to-newest and 17-entry tests added. |
| P-5 | Resolved | Marking set pinned (2xx/400/401/403/abort/AC-1); 404/429 GUARD; marking is synchronous after `fetch` resolves, before `res.json()` / `readProblem`; abort during the body read (M-18) keeps the abort outcome and finds the token marked. |
| P-6 | Resolved, one call site missed | `readStore` + stop at first present store for `peekSession`, `getSessionPersist`, `reconcileBuyFlowOwner`; prev role only for the same id (M-13, M-14). Missed: `observeStorageEvents` (R2-5). RoleGuard/useAuth: `useAuth.loadAuth` uses `getSession()` (`use-auth.ts:51`), so an invalid role means signed out and RoleGuard sends the user to sign-in; signing in again writes the target store and wipes the other (`session.ts:180`), replacing the tampered copy; `logout()` still revokes via `getStoredRefreshToken()` (`auth.ts:129`). No contract or existing test seeds a non-standard role (`test-utils/fetch-mock.ts:59-63`, `session.contract.test.ts`, `lib/debt/harness.ts:100`); refresh mocks without `role`/`id` (e.g. the "WRONG" body in `api-client.contract.test.ts`) fall back to the prev role of the same id, so they keep passing. |
| P-7 | Resolved, see R2-4 | Refs for hold/state, functional state update, `mountedRef`, `cancelling` guard, one sign-in prompt via `isAuthRequired`. `primaryDisabled` is a new optional prop; the only other owner of `OrderSummary` is `components/showcase/domain-gallery.tsx:300`, unaffected. Tier cards: in the active state only the held tier is enabled and `onQuantityChange` already returns early (`checkout-view.tsx:235-238`), so disabling them while cancelling is belt and braces. |
| P-8 | Resolved | Google X-15 edge traced through `auth.ts:108-116`: AC-1 failure -> guard clears -> catch restores the Google pair (already marked) -> next token-expired 401 hits the used check -> `AUTH_REQUIRED`, 1 POST total. Module-reload, lock re-read, joiner tests and M-15..M-18 added. |

### Invariants (re-checked against §7)

- One POST per refresh token / POST only inside the Web Lock: no new POST site; `isRefreshUsed`, marking and both
  wipes run in `doRefresh` / `refreshPost` / the resolver, i.e. inside the lock callback or the existing
  `locksUnusable` path (`api-client.ts:204,229,241`). AC-2 only removes POSTs.
- User-id check at every stored-token return: resolver step (2) checks same user id, unchanged epoch, different
  and unused refresh token; the success path (`api-client.ts:182-187`) is unchanged.
- 45 s / 120 s with the posted-session guard: unchanged; resolver step (1) is `isSamePostedSession`.
- No resurrection after logout: the resolver and both wipes never call `setSession`; logout bumps the epoch and
  empties both stores, so step (2) cannot match. The eager wipe touches only stores whose raw `ef_refresh` equals a
  token this tab already got a final answer for.
- Wiping localStorage vs another tab's live session: localStorage is wiped only when it holds the posted token. If it
  holds it for the same user at the same epoch, the existing guard already clears both stores (`api-client.ts:175`),
  so the new wipe reaches localStorage only after a same-tab epoch change (R2-S1). That token is rejected, rotated
  or abort-burned, and wiping it stops another tab from POSTing it a second time. No live session is lost that the
  current design would keep.

### Findings

#### R2-1 (minor) The native `showModal()` path has no automated test

- Evidence: plan §7 P-1 says "unit tests use that path [open attribute fallback], Playwright the real one", but §2
  and §7 add no Playwright case; `tests/e2e/specs/buy-flow.spec.ts` has no hold-release step and
  `tests/e2e/fixtures/api-mock.ts` has no `DELETE /v1/holds/{id}` handler. jsdom 25 has `HTMLDialogElement` with no
  `showModal`/`close` (`node_modules/jsdom/lib/jsdom/living/nodes/HTMLDialogElement-impl.js:5`). So the branch every
  real browser takes (top layer, native Esc -> `cancel`, focus) never runs in the gates. A bug there (for example
  `showModal()` throwing `InvalidStateError` on an element that already has `open`) ships with green tests.
- Fix: in `checkout-view.debt5.test.tsx`, stub `HTMLDialogElement.prototype.showModal`/`close` (set/remove `open`,
  dispatch `close`; the Esc test dispatches `cancel` on the dialog) and run the open / keep / Esc / confirm cases on
  that branch too, keeping one case on the fallback branch. Optionally add one Playwright case (mock DELETE 204,
  open dialog, press Escape -> 0 DELETE, confirm -> 1 DELETE). Add a WP-B mutation "native branch never calls
  `showModal`" that must fail.

#### R2-2 (minor) Fallback path: Esc and close need their own handling

- Evidence: plan §7 P-1 maps Esc to "the dialog `cancel` event". Browsers fire `cancel` only for a modal dialog; on
  the `open`-attribute fallback (jsdom) pressing Escape fires nothing, and `close()` is not a function there either.
  The planned "Esc -> closed, 0 DELETE" unit test therefore either fails or gets faked by dispatching `cancel` by
  hand. In the native path Esc closes the element itself, so React state must follow the `close` event.
- Fix: one close routine for every path (`el.close` if it is a function, else `removeAttribute("open")`); on the
  fallback add a `keydown` Escape handler on the dialog; listen for `cancel` (preventDefault, then close through state)
  and `close` (sync state); open imperatively with `if (!el.open) el.showModal()` and never also render the `open`
  prop from React on the native branch. The fallback Esc test must use `userEvent.keyboard("{Escape}")`.

#### R2-3 (minor) Focus return when the trigger no longer exists

- Evidence: spec AC-11 "focus ... returns to the trigger when it closes". The trigger renders only while
  `holdState === "active" && hold` (plan §1 WP-B). After a successful release (-> idle) or an expiry while open
  (-> expired) the dialog closes because that condition went false, so the trigger is unmounted and focus falls to
  `<body>`.
- Fix: return focus to the trigger if it is still connected, else to the OrderSummary primary button (present in idle
  and expired). Test: after a confirmed release `document.activeElement` is the primary button, not `body`.

#### R2-4 (minor) Dialog and primary action while the DELETE is in flight

- Evidence: plan §7 P-7 disables confirm and the primary action while `cancelling`, but says nothing about "Giữ lại"
  and Esc during the DELETE. Closing with "keep" while the release is still running tells the buyer the tickets are
  kept, then the DELETE releases them. Separately, `primaryDisabled = cancelling` keeps "Chọn lại" disabled after an
  expiry during a hung DELETE; `cancelHold` has no timeout (`hold-client.ts:275-283`).
- Fix: while `cancelling`, disable "Giữ lại", preventDefault `cancel` and ignore Escape; the dialog closes on the
  result (on failure either close and show the alert, or stay open with the alert: pick one and test it). Pass
  `primaryDisabled={cancelling && holdState === "active"}`. On success also clear the hook `error` (as `resetHold`
  does, `use-hold-timer.ts:137-142`) and `cancelError`; clear `cancelError` on a new attempt and when the hold
  leaves `active`.

#### R2-5 (minor) P-6 misses `observeStorageEvents`

- Evidence: `session.ts:138` `const s = readFrom(store("local")) ?? readFrom(store("session"))` is not in the P-6
  list (`peekSession`, `getSessionPersist`, `reconcileBuyFlowOwner`). With a tampered role in localStorage and another
  user's copy in sessionStorage, a `storage` event makes `observeUser` see that other user and clear this tab's
  buy-flow keys, against the "invalid local -> null, no fall-through" rule.
- Fix: use `peekSession()` there (or `readStore` with the same stop rule) and add it to M-13's scope.

#### R2-6 (info) FNV-1a 32-bit collision impact

- A collision makes `isRefreshUsed` true for an unrelated live token. In `doRefresh` the lazy wipe removes that live
  session (raw-token compare, so only that session) and the caller gets `AUTH_REQUIRED` (forced re-login); in resolver
  step (2) it refuses a valid same-user pair (`AUTH_REQUIRED`, nothing wiped). It never causes a POST, a token
  hand-out to another user, or a leak. Rate about 16 / 2^32 = 3.7e-9 per refresh, fail-closed. Accept; record it
  next to the hash choice in the plan.
