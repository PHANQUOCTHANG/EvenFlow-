# Spec: remaining frontend debt (frontend-debt-r5)

Tier: **High-risk** (`tier.json`). Branch `feature/frontend-debt-r5` from develop @ c5fb598. Web only, no backend,
no new dependency, no migration. Sources, with their executed repros:
`docs/workflow/frontend-real-api/findings.json` (+ `findings-security.json`, `findings-correctness.json`,
`findings-challenge.json`, `findings-round3-review.json`) and `docs/workflow/frontend-debt-r4/findings.json`
(+ `findings-review.json`, `findings-challenge.json`, `findings-round2-challenge.json`, `findings-round3-review.json`).
Contracts: `docs/workflow/frontend-real-api/contracts.md` (unchanged).

## 1. Goal

Close the deferred minor findings that can be fixed in the web app with a small, testable change, and record a
reason for every one that stays deferred. The refresh design is **not** reopened: one POST per refresh token,
always inside the Web Lock, a user-id check at every point that returns a stored token, 45 s for joiners and 120 s
abort for the POST owner. The changes below only add guards around that design.

## 2. Acceptance criteria

### A. Refresh and session (lib/api-client.ts, lib/session.ts)

- **AC-1 (R-3) Refresh 200 whose body cannot be used.** Given a refresh POST answers 2xx, When reading the body
  fails (connection drop, not JSON) or the body is not a JSON object, Then the caller gets `AUTH_REQUIRED` and not
  `NETWORK_ERROR`. The session is cleared only if it is still the posted session (same epoch, same refresh token,
  same user id: the existing `isSamePostedSession` guard). The posted refresh token is never POSTed again (see AC-2).
  An abort that fires while the body is being read keeps today's abort outcome (AC-2 of r4).
- **AC-2 (R2-3, R2-S1) A refresh token is POSTed at most once per tab.** Given this tab has already POSTed refresh
  token `r` and got a response (2xx, 400/401/403, 2xx with a bad body) or aborted the POST at 120 s, When any later
  refresh in this tab finds `r` as the stored refresh token (for example the tab-local sessionStorage copy that
  survived because `peekSession()` reads localStorage first, or a same-tab `setSession` that wrote the same pair
  back), Then no POST is sent, the stores whose `ef_refresh` equals `r` are wiped (auth keys only: buy-flow keys
  and `ef:lastuid` are kept, as G-4 requires), a store holding another token is left alone, and the caller gets
  `AUTH_REQUIRED`. A POST that failed before any response (fetch rejected, `NETWORK_ERROR`) or answered 5xx/429
  does **not** mark the token as used: retry after going offline or after a server error works as today.
  - The record of used tokens is tab-scoped: sessionStorage key `ef:rt:used`, holding a short hash (not the
    token) of at most the 16 most recent used tokens. It survives a reload of the tab, is not a buy-flow key, and
    an unreadable or corrupt value counts as empty (fail open to today's behaviour, never throws).
  - Repro to turn green: R2-3 in `frontend-debt-r4/findings-round2-challenge.json` ends with posts `["r1"]`, not
    `["r1","r1"]`; R2-S1 likewise.
- **AC-3 (X-4, R2-4) Same user signed in again elsewhere while the POST was in flight.** Given the POST of `r1`
  ends in 400/401/403, in the 120 s abort, or in an AC-1 body failure, and the guard decided not to clear, When the
  stored session now belongs to the **same user id** as the posted session, holds a **different** refresh token, and
  the tab's session epoch has not changed (the same rule as the success path, `api-client.ts:183-187`), Then the
  owner and every joiner get that stored access token (and `apiFetch` retries the original request once with it)
  instead of `AUTH_REQUIRED`. If the stored session belongs to another user, is absent, or the epoch changed, the
  outcome stays `AUTH_REQUIRED` and the stored session is left untouched.
- **AC-4 (X-S1) No unauthenticated send after a swallowed proactive refresh.** Given a request started with a
  session (user A) and the proactive refresh failed with a transient error, When the session is gone afterwards
  (another tab signed out), Then `apiFetch` rejects with `AUTH_REQUIRED` and sends nothing: no request without an
  `Authorization` header, no raw 401 `Response` returned to the caller.
- **AC-5 (X-S2) Lock result that is not a token.** Given `navigator.locks.request` resolves without running the
  callback (no string token), Then `refreshAccessToken` rejects with `NETWORK_ERROR`, the session is kept, no POST is
  sent and `apiFetch` does not retry the original request without a bearer.
- **AC-6 (X-14) Only known roles.** Given `ef_user` holds a role that is not `buyer`, `organizer`, `moderator` or
  `ops` (for example `"admin"` or a tampered value), Then `getSession()` and `peekSession()` return `null` (signed
  out for the UI, no `Authorization` header), and `ef_refresh` stays readable by `getStoredRefreshToken()` so
  `logout()` can still revoke it (F-15). Given an identity response (login, refresh, Google) with an unknown or
  missing role, Then `sessionFromIdentity` uses the previous session's role when it is valid and belongs to the
  same user id, otherwise `buyer` (least privilege; the server enforces roles). A known role is stored unchanged.
  (Clarified after plan review P-6: the store that holds `ef_token` + `ef_user` decides; an invalid session in
  localStorage does not fall through to a session in sessionStorage.)

### B. Checkout and payment

- **AC-7 (R3-1) 401 on the first order GET.** Given the result page has no order yet, When `getOrderDetails`
  fails with `AUTH_REQUIRED`, Then the error card shows the `checkout.authRequired` title and a sign-in link
  (`/auth/sign-in`, text `checkout.signInAction`), and no "Thử lại" button. This is the same screen as the
  "PENDING order on screen" case. Network and other errors keep the generic load error with "Thử lại".
- **AC-8 (R3-2) No empty paragraph.** Given the error card shows the `AUTH_REQUIRED` prompt (with or without an
  order), Then the alert contains no empty `<p>`. Other error kinds keep their body text.
- **AC-9 (R3-3) Ticket issuing stops on an expired session.** Given the order is PAID and `issueTickets` fails with
  `AUTH_REQUIRED`, Then the ticket section shows the `checkout.authRequired` prompt with a sign-in link, and does not
  show the "Lấy vé lại" retry. `useIssuedTickets` exposes `authRequired: true` (reset by `retry()`); `failed` keeps
  its current value so the F-13 test stays valid. Exactly one sign-in prompt is on the page.
- **AC-10 (S-4, client part) Safe payment redirect.** Given `initiatePayment` gets `redirect_url`, When it is not a
  relative URL or an absolute `http:`/`https:` URL (for example `javascript:...`, `data:...`, or a value that does
  not parse), Then `redirectUrl` is the default result URL (`/checkout/{eventId}/result?orderId={orderId}`, or the
  caller's `returnUrl`) and the browser never navigates to the rejected value. `https://...` provider URLs and
  relative URLs are kept unchanged, so `payment-client.contract.test.ts` passes untouched.
- **AC-11 (C-6) Release held tickets, after confirmation.** (Clarified 2026-11-01 to match the human approval:
  a confirmation dialog is required.) Given the checkout page shows an active hold, Then a secondary button
  "Huỷ giữ vé" (i18n key `checkout.cancelHold`, English "Release tickets") is shown. When the buyer clicks it, Then
  no request is sent; an accessible confirmation dialog opens (`role="alertdialog"`, `aria-modal="true"`,
  labelled by its title and described by its body, focus moves into the dialog and returns to the trigger when it
  closes; keys `checkout.cancelHoldConfirmTitle`, `checkout.cancelHoldConfirmBody`, `checkout.cancelHoldConfirm`,
  `checkout.cancelHoldKeep`, vi and en). "Giữ lại" (keep) or Esc closes it and sends nothing. When the buyer
  confirms, Then `DELETE /v1/holds/{holdId}` is sent exactly once (confirm disabled with a spinner while it runs,
  double clicks ignored; the primary action and tier selection are disabled meanwhile). If the hold leaves the
  active state while the dialog is open (expiry), the dialog closes and nothing is sent. On success (204/404/2xx)
  the stored hold is removed, the countdown disappears and the page returns to the idle "Giữ vé và thanh toán"
  state. On `AUTH_REQUIRED` the existing sign-in alert is shown (one prompt) and the hold stays on screen. On any
  other failure (network `AMBIGUOUS`, other HTTP status) the hold stays on screen and an error alert says the
  release could not be confirmed (key `checkout.cancelHoldFailed`). The button is not shown in the idle, creating,
  expired, sold-out or ambiguous states.

### C. Cross-cutting

- **AC-12** Gates: lint, typecheck, build (`NEXT_PUBLIC_API_BASE` unset), all Vitest tests (baseline at c5fb598),
  Playwright E2E, Go gateway/event unchanged and green. No existing test is deleted, skipped or weakened. Contract
  tests (`*.contract.test.*`) are unchanged unless the human approves a test-change request.

## 3. Kept deferred

| ID | Source | Reason it stays deferred |
|---|---|---|
| S-2 | real-api security | Human accepted tokens in web storage; the fix (HttpOnly refresh cookie) needs a gateway change. |
| S-4 (server part) | real-api security | Payment-service `return_url` allow-list and a provider-host allow-list are backend/config work; AC-10 only blocks non-http(s) schemes. |
| S-5 | real-api security | Backend DTO (public event list exposes `organizer_id`, `bucket_count`); same data already public via GET by id. |
| S-6 | real-api security | Browsers without Web Locks; fails safe (forced logout, no leak); a localStorage lease would reopen the refresh design. |
| S-8 | real-api security | Supply chain reviewed, no new advisory; nothing to change. |
| X-15 | real-api challenge | The client cannot learn the pair rotated by a lost `/refresh` response; any client choice ends the same way (re-login). Needs an idempotent refresh on the server. AC-2 now prevents a second POST of the Google refresh token in this tab. |
| F-16 gap | real-api round 2 | RegisterForm Google button has no remember-me control; the documented default (local) is a product decision. |
| F-17 gap | real-api round 2 | sessionStorage is per tab by design; other tabs lose access on their next call through `AUTH_REQUIRED`. |
| R3-5 (real-api) | real-api round 3 | Payment initiate response has no `provider` field, so the client cannot tell the method changed; needs a backend field. |
| X-5 | r4 challenge | Same end state as the human-approved 422 adoption path; telling "my lost tier" from "a colleague's tier" needs an idempotency key on `addTier` (backend). |

Already closed by frontend-debt-r4 (listed so nothing is lost): R4-1/W-3, R4-2/W-2, R4-3/W-1, R4-4, W-4, W-5.

## 4. Out of scope

Backend changes; HttpOnly cookies; refresh design changes (lock, deadlines, single POST); sample-data screens;
a release button on the payment page; a shared dialog component for other pages (the AC-11 dialog stays local).

## 5. Edge cases

- AC-1: the body read fails after another tab rotated the pair for the same user -> AC-3 applies (reuse, no clear).
  The body read fails after a sign-out in this tab (epoch changed) -> `AUTH_REQUIRED`, nothing cleared.
- AC-2: the used-token record is per tab (sessionStorage); cross-tab reuse stays covered by the Web Lock and the
  re-read inside it. Storage blocked (private mode) -> no record, behaviour as today. A used token found during the lock re-read must not wipe a store whose token
  differs. Google login (X-15): after an AC-1 failure the restored Google session keeps working until its access
  token expires, then ends with `AUTH_REQUIRED` and no second POST.
- AC-3: the abort at 120 s finds a same-user pair written by another tab at 100 s -> owner and joiner both get it;
  a call that starts after the abort is not affected.
- AC-6: a stored session with a valid role but an unknown extra field is still accepted.
- AC-7/AC-9: `AUTH_REQUIRED` on the order GET and on issue-tickets at the same time -> one prompt only.
- AC-2 (plan review P-4): when the token is marked used and the guard did not clear, any other store still
  holding that token is wiped at once, so no stale copy outlives the 16-entry record.
- AC-11: the hold expires while the confirmation dialog is open -> dialog closes, nothing sent.
- AC-11: the hold expires while the DELETE is in flight -> the page ends in the expired state, not idle; the
  DELETE answers 404 -> treated as released.

## 6. Definition of Ready

Each fixed AC is backed by an executed repro in the source findings (C-6 and AC-11 have no repro, it is a missing
feature). Questions for the human:

1. Approve this spec, including the AC-2 rule that a refresh token which got a response is never POSTed again from
   this tab (it wipes the store that still holds it).
2. AC-6: unknown role in storage -> treated as signed out; unknown role in an identity response -> previous role or
   `buyer`. Confirm, or prefer failing the login instead.
3. AC-11: add the "Huỷ giữ vé" button without a confirmation dialog (as proposed), or require one.

## Human approval

Approved by TranDinhVo on 2026-11-01 in the Claude Code session, with all recommended decisions. AC-11: the "Huỷ giữ vé" button REQUIRES a confirmation dialog before sending DELETE.
