# Spec: frontend debt after frontend-real-api (frontend-debt-r4)

Tier: **High-risk** (`tier.json`). Branch `feature/frontend-debt-r4` from develop @ 43375eb. Web only, no backend,
no new dependency. Source findings and their executed repros:
`docs/workflow/frontend-real-api/findings-round4-review.json` and `findings-round4-challenge.json`.
Contracts: `docs/workflow/frontend-real-api/contracts.md` (unchanged).

## 1. Goal

Close the six minor findings deferred when PR #85 merged, without reopening the refresh design that review proved
correct (single POST per refresh token, always inside the Web Lock, no steal).

## 2. Acceptance criteria

### A. Session ownership and refresh

- **AC-1 (R4-4)** Given a request sent while user A was signed in, When a refresh is needed and, while it waits for
  the lock, another tab signs out A and signs in user B, Then the original request is NOT retried with B's token: it
  rejects with `AUTH_REQUIRED`, B's stored session is left untouched, and no `POST /api/v1/auth/refresh` is sent with
  any token. A rotation by another tab for the SAME user is still reused without a POST (unchanged).
- **AC-2 (R4-1 / W-3)** Given a refresh POST that does not answer:
  - each caller waiting on it (same tab or other tabs) rejects with `NETWORK_ERROR` after at most 45 s, the session
    is kept, and no second POST is sent while the first is in flight;
  - the refresh POST itself is aborted after 120 s; that outcome is `AUTH_REQUIRED` and the session is cleared,
    because the server may already have rotated the token, which must never be POSTed again;
  - once the POST settles, the lock is released and a later request in any tab can refresh normally.
- **AC-3 (R4-2 / W-2)** Given `navigator.locks.request` rejects or throws synchronously with any error that is not
  an `ApiError` (`InvalidStateError`, `SecurityError`, `NotSupportedError`, `AbortError`), Then `apiFetch` rejects
  with `ApiError` `NETWORK_ERROR`, the session is kept, no POST is sent, and no deadline timer is left pending. For
  `SecurityError`, the client may fall back to the in-tab single-flight refresh, since no other tab can share the
  lock in that context.
- **AC-4 (R4-3 / W-1)** Given two tabs sharing localStorage (persist = local) with separate sessionStorage, When tab B
  switches the signed-in user from A to B, Then tab T clears its own buy-flow keys (`ef:queue:token:*`,
  `evenflow_hold_*`, `evenflow_order_*`, `ef:issue:*`, `ef:pay:*`, `ef:hold:*`) the next time it observes the change.
  It observes through a `storage` event or through its next `getSession()` / `setSession()`, comparing against the
  tab-local `ef:lastuid`. The same user in both tabs keeps the keys.

### B. Organizer and payment

- **AC-5 (W-4)** Given `addTier` succeeded on the server but its response was lost, When the organizer retries the
  save, Then the unsaved tier is matched to the reloaded server tier with the same trimmed name. The client does not
  POST it again, does not end in 422 duplicate-name, and only sends a quota change if the quota differs. No duplicate
  tier is created.
- **AC-6 (W-5)** Given `initiatePayment`, `getOrderDetails` or `issueTickets` receive 403 `/problems/forbidden`, Then
  the client error type is `FORBIDDEN` (distinct from `UNKNOWN`), and the payment page and the result page show the
  existing `checkout.forbidden` message with no retry loop. Other 403s keep their current mapping.

### C. Cross-cutting

- **AC-7** Gates: lint, typecheck, build (`NEXT_PUBLIC_API_BASE` unset), all Vitest tests (baseline at 43375eb),
  Playwright E2E, Go gateway/event unchanged and green. No existing test is deleted, skipped or weakened. Contract
  tests (`*.contract.test.*`) are unchanged unless a test-change request is approved by the human.

## 3. Out of scope

Backend changes; payment `return_url` allow-list (backend debt); sample-data screens; httpOnly cookies.

## 4. Edge cases

Two tabs switching user and back within one lock wait; refresh POST settling exactly at 45 s / 120 s; a `storage`
event arriving while a refresh is in flight; a lost-response tier whose name was edited before retry (no name match:
POST as new, server answers 422, so show the server's detail); 403 `/problems/forbidden` during order polling.

## 5. Definition of Ready

Each AC is backed by an executed repro in the source findings. Open question for the human: approve this spec; the
120 s abort that logs the user out (AC-2) is a deliberate trade-off.
