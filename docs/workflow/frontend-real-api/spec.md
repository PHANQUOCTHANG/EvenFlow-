# Spec: frontend talks to the real backend (frontend-real-api)

Tier: **High-risk** (see `tier.json`). Branch `feature/frontend-real-api` from develop @ 2970b7f.
Contracts: `contracts.md` (frozen). Process: v3 (`docs/multi_agent_coding_workflow_v3_quy_trinh.pdf`).

## 1. Goal

Every buyer and organizer screen of `apps/web` works against the real services through the gateway
(`NEXT_PUBLIC_API_BASE`, :8080) instead of mock data, with a working login session (access + refresh token).
Screens whose backend does not exist yet keep sample data but say so on screen.

## 2. Scope

In: auth session + shared fetch wrapper; queue/hold/payment/order clients; public event pages; checkout and
waiting-room event lookup; organizer console CRUD; two small backend additions (gateway CORS, public event list);
E2E mock alignment.

Out: ops war room data (no API, EVF-60); organizer realtime sales and moderation detail (no API); payment
ownership check in payment service (reported only); token storage hardening (httpOnly cookies) — tokens stay in
`localStorage` as today; English countdown (EV-188 open decision).

## 3. Definitions

- `apiFetch` — one shared client wrapper (new module) used by every client module. Base URL is `apiBase()`.
- Session — `{accessToken, refreshToken, user: {id, role, email?}}` persisted in `localStorage`, or `sessionStorage` when
  "remember me" is off (same keys; read local first) (`ef_token` keeps the access token for backward compatibility, `ef_refresh` the refresh token, `ef_user` the user).
- Problem — RFC 7807 body; the client reads `type` and `status`.

## 4. Acceptance criteria

### A. Session and auth

- **AC-1** Given valid credentials, When the user logs in, Then the client calls `POST {base}/api/v1/auth/login`
  with `{identifier, password, device_fingerprint}` and stores access token, refresh token and `{id, role, email}`
  from the response. No request goes to `:8083` or uses `NEXT_PUBLIC_API_URL`.
- **AC-2** Given a successful `POST /api/v1/auth/register/verify` (201, no tokens), When registration completes,
  Then the client logs in with the same identifier/password and stores the session as in AC-1.
- **AC-3** Given a stored session, When any client module calls the API, Then the request carries
  `Authorization: Bearer <access>`; Given no session, Then no Authorization header is sent. No module sends
  `credentials: "include"` or any `X-Identity-*` header.
- **AC-4** Given a request that returns 401 with `type` `/problems/token-expired`, When the session has a refresh
  token, Then the client calls `POST /api/v1/auth/refresh` exactly once even if N requests fail concurrently,
  stores the new pair, and retries each original request once. Given refresh fails (401/400), Then the session is
  cleared and the call rejects with an `AUTH_REQUIRED` error. A retried request that returns 401 again is not
  refreshed a second time.
- **AC-5** Given a 401/403 whose `type` is `/problems/queue-token-expired`, `/problems/queue-token-invalid` or
  `/problems/queue-token-mismatch`, Then no refresh happens and the session is kept.
- **AC-6** Given a session, When the user logs out, Then the client calls `POST /api/v1/auth/logout` with the
  refresh token and clears the session even if that call fails.
- **AC-7** `useAuth` returns `{id, role, email?}` from the stored session; it treats an expired access token with
  a valid refresh token as logged in (refresh happens on the next call), and returns logged-out when there is no
  session. It no longer reads `email` from the JWT.
- **AC-8** Given a Google login (no device bound), When it succeeds, Then the client calls
  `POST /api/v1/auth/devices {fingerprint}` and then `POST /api/v1/auth/refresh {refresh_token, device_fingerprint}`
  and stores the refreshed pair. If the device call fails, the session from the Google login is still stored.

### B. Waiting room and hold

- **AC-9** `joinQueue`, `fetchQueueStatus` and the queue SSE stream go through the session (Bearer, AC-3).
  A 403 `/problems/otp-required` or `/problems/device-required` from join surfaces as a distinct error code that
  the waiting-room UI shows as an actionable message (verify OTP / sign in again), not a generic failure.
- **AC-10** `requestHold` sends `Idempotency-Key`, `X-Queue-Token` and Bearer, never `X-Identity-Id`; treats
  HTTP 200 (and 201) as success returning `{order_id, hold_id, expires_at}`; maps problem types to distinct codes:
  queue-token expired/invalid/mismatch → `QUEUE_TOKEN_INVALID` (UI sends the user back to the waiting room),
  `/problems/per-identity-limit` → `PER_IDENTITY_LIMIT`, `/problems/quantity-not-allowed` → `QUANTITY_NOT_ALLOWED`,
  409 → `SOLD_OUT`, other 403 → `NOT_ADMITTED`.
- **AC-11** A new `cancelHold(holdId)` calls `DELETE /v1/holds/{holdId}` with an Idempotency-Key; 204 and 404 both
  clear the locally stored hold.

### C. Payment and order

- **AC-12** `initiatePayment` posts the contract body through `apiFetch`. On any non-2xx or network error it
  rejects with a `PaymentClientError`; it never fabricates a `payment_id` or success in any environment.
- **AC-13** The payment page lists methods from `GET /v1/payments/methods`, showing only `enabled` ones;
  `stripe` is representable. If the call fails, the page shows an error with retry instead of a hard-coded list.
- **AC-14** `getOrderDetails` maps the real `GET /v1/orders/{id}` fields (incl. `items`, `expires_at`, `hold_id`,
  `failure_reason`, statuses `PENDING|PAID|FAILED|EXPIRED|CANCELLED`). 404 rejects with `NOT_FOUND`; it never
  serves the sessionStorage cache as live data and never invents `quantity` or `tier_name`.
- **AC-15** Given an order with status `PAID`, When the result page renders, Then it calls
  `POST /v1/orders/{id}/issue-tickets` with an Idempotency-Key that is stable per order (same key on reload) and
  renders one QR per ticket from `qr_payload`. While tickets are not issued yet it shows a pending state and
  polls `GET /v1/orders/{id}` (bounded backoff); no demo QR is ever rendered.

### D. Public event pages

- **AC-16** `/events/[slug]` loads `GET /api/v1/events/slug/{slug}` and, for ON_SALE events,
  `GET /v1/events/{id}/availability`; tier price = `price_cents`; tier availability comes from the availability
  call (joined on `tier.id == ticket_type_id`), defaulting to unknown when the call fails. 404 → Next `notFound()`.
  ISR `revalidate = 60` is kept; `generateStaticParams` no longer reads mock data.
- **AC-17** `/events` lists `GET /api/v1/events/public`; shows an empty state when the list is empty and an error
  state (not mock data) when the call fails.
- **AC-18** Checkout, payment, result and waiting-room pages resolve the event by UUID via
  `GET /api/v1/events/{id}`; unknown id → `notFound()`.
- **AC-19** `event-service` and `organizer-service` never return mock/sample data. Remaining sample data lives in
  `lib/sample-data/` and is only used by screens that label it as sample (`isSample`), i.e. ops war room and
  organizer sales.

### E. Organizer console

- **AC-20** `/organizer/events` lists `GET /api/v1/events` (own events) and renders all 9 statuses, including
  `COMPLETED`; the list does not require tiers.
- **AC-21** Creating an event calls `POST /api/v1/events`, then `POST /api/v1/events/{id}/tiers` for each tier,
  then `POST /submit` when "submit for review" is chosen. If a later step fails, the UI reports which step failed and
  links to the created draft so nothing is silently lost. Client validation rejects `starts_at <= sale_start_at`,
  empty tiers, quota <= 0 and price < 0 before any call.
- **AC-22** Editing calls `PUT /api/v1/events/{id}` with the full field set (never a partial body), only offered
  for DRAFT/SCHEDULED/REJECTED; new tiers use `POST .../tiers`; existing tiers only allow quota changes via
  `PUT .../tiers/{tierId} {new_quota}` (name/price read-only); on ON_SALE a quota decrease is blocked client-side.
- **AC-23** Submit (DRAFT/REJECTED → PENDING_REVIEW) and cancel (any but COMPLETED/CANCELLED, with confirm) call
  their endpoints and refresh the view; 422 shows the problem `detail`.
- **AC-24** The review-status page shows the real status; the moderation-detail block and the sales dashboard are
  labelled as sample/unavailable data (same `isSample` pattern as the ops war room). No fabricated number is shown
  without that label.

### F. Backend additions

- **AC-25** Gateway CORS preflight for an allowed origin returns Allow-Headers including `X-Queue-Token` and
  `If-None-Match`, Expose-Headers including `ETag` and `X-Poll-After-Ms`; still no Allow-Credentials; disallowed
  origins unchanged. Covered by Go unit tests.
- **AC-26** `GET /api/v1/events/public` (event service) returns `{events:[...]}` with tiers, only statuses
  SCHEDULED/ON_SALE/SOLD_OUT, sorted by `sale_start_at` ascending, at most 100; reachable through the gateway without
  a token; `GET /api/v1/events/{id}` and the organizer list behave as before. Covered by Go unit tests (handler,
  gateway route table) and a repository integration test.

### G. Cross-cutting

- **AC-27** One env var `NEXT_PUBLIC_API_BASE` for every call; `.env.example` documents it; `NEXT_PUBLIC_API_URL`
  is removed.
- **AC-28** `tests/e2e/fixtures/api-mock.ts` mirrors the gateway CORS headers and real status codes (hold 200) and
  mocks the new calls (auth, events, availability, methods, issue-tickets); the E2E buy flow passes.
- **AC-29** Gates: lint, typecheck, build, all Vitest tests (baseline 1946 pass), Go tests for touched services pass;
  no existing test deleted, skipped or weakened (tests that asserted mock behaviour are replaced, listed in a test
  change request).

## 5. Edge cases

Concurrent 401s (single refresh); refresh token reused → logout; clock skew (do not decide expiry client-side
beyond a hint); availability 404 when event not on sale; SOLD_OUT join; idempotent retry after 409 in-flight
(`Retry-After`); SSR fetch without a session (public endpoints only); `apiBase()` empty in tests.

## 6. Definition of Ready

- Contracts frozen in `contracts.md` (backend surveyed with file:line).
- Open question for the human (needs approval): the two backend additions in §F and keeping tokens in
  `localStorage`.
