# Backend wire contracts (frozen input for frontend-real-api)

Source: read-only survey of develop @ 2970b7f. file:line refs are relative to the repo root.
This document is the contract the web client codes against. Changing a backend contract is out of scope
except for the two additions in §8 (gateway CORS, public event list).

## 0. Cross-cutting

- Errors: RFC 7807 `{type, title, status, detail}` (`libs/go/httpx/problem.go:25-41`). Branch on `type`, never on `detail` (Vietnamese text). Identity uses the same shape with Content-Type `application/json` (`services/identity/internal/adapter/http/handler.go:135-149`).
- Times: RFC 3339 strings. Exception: waitingroom `expires_at` is int64 epoch.
- Money: integer VND, no minor unit (`price_cents`, `amount`, `total_cents`).
- Gateway (`services/gateway/internal/proxy/gateway.go`):
  - Auth = `Authorization: Bearer <jwt>` only, no cookies (`:158-176`). HS256, access TTL 15 min (`libs/go/authx/access.go:16-27`). Claims: `sub, role, typ, ver, did, jti, iat, exp, iss, aud`. **No `email` claim.**
  - Strips client `X-Identity-*`, `X-Device-Id`, `X-User-Id`, `X-Role`, `X-Organizer-Id`; injects `X-Identity-Id`, `X-Identity-Role`, `X-Identity-Verified`, `X-Device-Id` from the token (`:112,136-138`).
  - Public routes attach identity when a valid token is present and ignore an invalid one.
  - 401 `/problems/token-expired` (+ `WWW-Authenticate`) when expired, else 401 `/problems/unauthorized`; 403 `/problems/forbidden`; 429 `/problems/rate-limited` + `Retry-After`; 502 `/problems/upstream-unavailable`; 404 `/problems/not-found`.
  - No path rewriting. Event + auth live under `/api/v1/...`; ticketing, payment, queue under `/v1/...`. Gateway listens on :8080.
  - CORS today (`:256-281`): Allow-Headers `Authorization, Content-Type, Idempotency-Key, X-Request-Id, Last-Event-ID`; Expose-Headers `X-Request-Id, Retry-After`; **no** `Access-Control-Allow-Credentials`.
- Idempotency (`libs/go/idem/middleware.go`): required on `POST /v1/events/{id}/holds`, `DELETE /v1/holds/{id}`, `POST /v1/events/{id}/payments`, `POST /v1/orders/{id}/issue-tickets`. Missing key → 400; in flight → 409 `urn:eventflow:problem:idempotency-in-progress` + `Retry-After: 1`; different body → 422 `urn:eventflow:problem:idempotency-key-reused`.

## 1. Identity (`services/identity/internal/adapter/http`), all via gateway `/api/v1/auth/` (Public)

`IdentityResponse = {id, email?, role, created_at, token?, access_token?, token_type?, expires_in?, refresh_token?, refresh_expires_in?}` (`dto.go:43-60`). `token` is a legacy alias of `access_token`. `role` is a single string `buyer|organizer|moderator|ops`. Refresh token travels in the JSON body only.

| Endpoint | Request | Success | Errors |
|---|---|---|---|
| POST /api/v1/auth/login | `{identifier, password, device_fingerprint?}` | 200 IdentityResponse with tokens | 400, 401, 429, 500 |
| POST /api/v1/auth/register/request-otp | `{identifier}` | 200 empty | 400, 409, 429, 500 |
| POST /api/v1/auth/register/verify | `{identifier, password, otp}` | **201 `{id, email?, role, created_at}` — no tokens** | 400, 409, 429, 500 |
| POST /api/v1/auth/google | `{credential}` | 200 IdentityResponse with tokens (no device bound) | 401, 500 |
| POST /api/v1/auth/refresh | `{refresh_token, device_fingerprint?}` | 200 IdentityResponse with a NEW pair; old refresh revoked | 400, 401 (reused/invalid), 500 |
| POST /api/v1/auth/logout | `{refresh_token}` | 204 | 400, 500 |
| GET /api/v1/auth/me | Bearer | 200 `{id, role}` | 401 |
| POST /api/v1/auth/devices | Bearer, `{fingerprint}` | 200 `{device_id}` | 400, 401, 429 |
| POST /api/v1/auth/forgot-password/request-otp, /reset | unchanged from current client | | |

## 2. Event service (`services/event/internal/adapter/http`)

Status enum: `DRAFT, PENDING_REVIEW, SCHEDULED, ON_SALE, SOLD_OUT, CLOSED, COMPLETED, REJECTED, CANCELLED`.
`EventResponse = {id, organizer_id, slug, title, description, venue, starts_at, sale_start_at, sale_end_at?, status, max_per_identity, max_per_order, hold_ttl_seconds, created_at, updated_at, tiers?}`; no image/banner/category field.
`TicketTier = {id, event_id, name, price_cents, currency:"VND", quota, bucket_count, created_at}`; no description. Tier `id` == ticketing `ticket_type_id`.

| Method / path | Gateway | Role | Body | Success |
|---|---|---|---|---|
| GET /api/v1/events/{id} | Public | any | – | 200 EventResponse with tiers; 400 if id not UUID; 404 |
| GET /api/v1/events/slug/{slug} | Public | any | – | 200 EventResponse with tiers; 404 |
| POST /api/v1/events | Auth | organizer | `{slug?, title, description, venue, starts_at, sale_start_at, sale_end_at?}` | 201 EventResponse (DRAFT) |
| PUT /api/v1/events/{id} | Auth | organizer | full replace `{title, description, venue, starts_at, sale_start_at, sale_end_at?}`, no slug | 200 EventResponse. Only DRAFT/SCHEDULED/REJECTED. **Silently ignores the update if starts_at <= sale_start_at** — client must validate first |
| POST /api/v1/events/{id}/tiers | Auth | organizer | `{name, price_cents, quota}` | 201 TicketTier; 422 duplicate name / quota<=0 / price<0 |
| PUT /api/v1/events/{id}/tiers/{tierId} | Auth | organizer | `{new_quota}` only | 200 TicketTier; ON_SALE may only increase |
| POST /api/v1/events/{id}/submit | Auth | organizer | empty | 200 EventResponse → PENDING_REVIEW; 422 if no tier with quota>0 |
| POST /api/v1/events/{id}/cancel | Auth | organizer | empty | 200 EventResponse → CANCELLED (not from COMPLETED/CANCELLED) |
| GET /api/v1/events | Auth | organizer (own) / moderator, ops (need `?organizer_id=`) | – | 200 array of EventResponse **without tiers**, created_at DESC, no pagination |

Errors: 400 bad UUID/JSON, 401, 403 not owner, 404, 422 validation/state transition. Unpublished events (DRAFT, PENDING_REVIEW, REJECTED) return 404 to non-owners.
There is NO moderation-detail endpoint readable by organizers and NO sales endpoint.

## 3. Waitingroom (via gateway, Authenticated)

- POST /v1/events/{eventID}/queue/join, body `{signals?}` → 200 `{state, queue_token, rank, is_new, poll_after_ms}`; SOLD_OUT gives `{state:"SOLD_OUT", queue_token:"", rank:-1}`. 403 `/problems/otp-required` (JWT `ver` false), 403 `/problems/device-required` (no `did`), 401, 503.
- GET /v1/events/{eventID}/queue/status, header `X-Queue-Token` (required), optional `If-None-Match` → 200 `{state, rank, queue_depth, admit_rate, eta_seconds, poll_after_ms, expires_at?}` + `ETag`; 304 + `X-Poll-After-Ms`. 401 `/problems/queue-token-expired|invalid` (NOT a login problem), 403 `/problems/queue-token-mismatch`.
- GET .../queue/stream: SSE `event: position`.

## 4. Ticketing (via gateway)

- POST /v1/events/{eventID}/holds (Auth). Headers `Idempotency-Key`, `X-Queue-Token`. Body `{ticket_type_id, quantity}`. **200** `{order_id, hold_id, expires_at}`. 403 `/problems/queue-token-expired|invalid|mismatch` or `about:blank` (not admitted); 409 sold out; 422 `/problems/quantity-not-allowed` | `/problems/per-identity-limit`; 503.
- DELETE /v1/holds/{holdID} (Auth, Idempotency-Key) → 204; 404; 409 `/problems/hold-not-cancellable`.
- GET /v1/orders/{orderID} (Auth, owner) → 200 `{order_id, event_id, event_title, venue, tier_name, quantity, total_amount, total_cents, currency, status: PENDING|PAID|FAILED|EXPIRED|CANCELLED, order_status, expires_at, created_at, items:[{ticket_type_id, tier_name, quantity, unit_cents}], issued_tickets:[{ticketId, ticketCode, tierName, attendeeName, status}], hold_id?, paid_at?, payment_method?, failure_reason?}`. `issued_tickets` only when ISSUED and has **no QR payload**. 404 = not found or not owner.
- POST /v1/orders/{orderID}/issue-tickets (Auth, owner, Idempotency-Key). If the order already has tickets it returns them again with freshly signed `qr_payload` (idempotent, `services/ticketing/internal/app/issue_tickets.go:55-98`); otherwise requires PAID. Use it to obtain QR payloads for the result page.
- GET /v1/orders/{orderID}/tickets (Auth, owner) → `{order_id, count, tickets:[{id, order_id, ticket_type_id, jti, qr_sig, status, checked_in_at?, created_at}]}` (no payload).
- GET /v1/events/{eventID}/availability (**Public**) → `{event_id, tiers:[{ticket_type_id, name, availability: AVAILABLE|FEW_LEFT|SOLD_OUT}]}`; 404 when not on sale.
- GET /v1/events/{eventID}/purchase-limit (Auth) → `{max_per_order, max_per_identity, used, max_selectable, has_active_hold}`.

## 5. Payment (via gateway)

- POST /v1/events/{eventID}/payments (Auth, Idempotency-Key). Body `{order_id, hold_id, amount, method: sandbox|vnpay|momo|stripe, buyer_name, buyer_email, buyer_phone, return_url}` → 200 `{payment_id, order_id, redirect_url, status, provider_txn_id?}`. 409 hold expired / order not HELD; 404; 422 amount mismatch; 400 bad method/input; 503.
- GET /v1/payments/{paymentID} (Auth) → `{payment_id, order_id, provider, provider_txn_id, amount_cents, currency, status: PENDING|SUCCEEDED|FAILED|REFUNDED, created_at}`.
- GET /v1/payments/methods (Public) → `{methods:[{code, name, description, enabled, icon}]}`.

## 6. Current frontend gaps (summary)

1. `lib/event-service.ts` and `lib/organizer-service.ts` are 100% mock; mock ids are not UUIDs.
2. No client sends `Authorization` except `getOrderDetails`; hold sends a fake `X-Identity-Id: guest-user`; queue/sse use `credentials:"include"`.
3. Auth client uses `NEXT_PUBLIC_API_URL` defaulting to identity :8083 (bypasses gateway), discards `refresh_token`, has no refresh/logout/me, and treats register/verify as if it returned tokens.
4. `payment-client.initiatePayment` fakes success on 401/network error in sandbox/dev; `getOrderDetails` serves sessionStorage cache on 404/network error and invents defaults (`quantity||1`, `tier_name||"Vé Tiêu Chuẩn"`).
5. Result page expects `issued_tickets[].qrPayload` which the backend never sends.
6. `useAuth` reads `email` from the JWT (claim does not exist).
7. Google login binds no device → waiting-room join returns 403 `/problems/device-required`.
8. E2E `tests/e2e/fixtures/api-mock.ts` CORS is more permissive than the gateway and returns 201 for holds.

## 7. Backend gaps left out of scope (UI shows an explicit "sample data / not yet available" state)

- Organizer realtime sales (no endpoint).
- Moderation detail (4 AI axes) for organizers.
- Ops war room API (EVF-60).
- Payment service has no order-ownership check (security gap, reported, not fixed here).

## 8. Backend additions IN scope

1. Gateway CORS (`services/gateway/internal/proxy/gateway.go:265,268`): Allow-Headers add `X-Queue-Token`, `If-None-Match`; Expose-Headers add `ETag`, `X-Poll-After-Ms`. Credentials stay disabled (web stops sending `credentials:"include"`).
2. Event service public catalogue: `GET /api/v1/events/public` → 200 `{events: EventResponse[]}` (tiers included) for status in `SCHEDULED, ON_SALE, SOLD_OUT`, ordered by `sale_start_at ASC`, max 100. Gateway route `GET /api/v1/events/public` Public, registered before `/api/v1/events/`.

## 9. Client-side decisions frozen after test design (2026-10-21)

- `POST /v1/orders/{id}/issue-tickets` → 200 `{order_id, tickets:[{id, ticket_type_id, status, qr_payload, ...}]}` (from `services/ticketing/internal/adapter/http/ticket_handler.go`).
- `getEventBySlug/getEventById` → `EventDetailSnapshot | null` (null on 404, and on 400 for a non-UUID id); other failures reject. `getAllPublicEvents` → array, rejects on failure. Availability is joined inside `getEventBySlug` for ON_SALE events; on failure tier availability = `UNKNOWN`. No separate exported `getAvailability` is required.
- Event repo port method: `ListPublic(ctx, limit int) ([]*domain.Event, error)`, handler limit 100. Fakes for it are defined in the new contract test files; do not redefine them in `handler_test.go` / `fakes_test.go`.
- Error objects expose `code` (string). Hold 403 with a queue-token problem type → `QUEUE_TOKEN_INVALID`. `QueueError.code` differs for otp-required, device-required and generic 403. Order 404 → `NOT_FOUND`.
- Web order model: `expiresAt`, `holdId`, `failureReason`, `items[{ticketTypeId, tierName, quantity, unitCents}]`.
- `apiFetch(..., {auth:false})` sends no Bearer. `setSession` defaults to localStorage. `connectSse` sends Bearer, no credentials.
- `updateOrganizerEvent(id, partial)` merges with the current event and always sends a full PUT. `updateTierQuota` reads the event and blocks decreases when ON_SALE. Client validation failures in `createOrganizerEvent` make no network call.
- Google login: when the device call fails the Google session is kept; `refreshSession` rejects on 401.
- 401 `token-expired` with no stored refresh token → clear session, reject `AUTH_REQUIRED` (same as refresh failure).
- `.env.example` documenting `NEXT_PUBLIC_API_BASE` lives in `apps/web/`.
