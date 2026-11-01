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
