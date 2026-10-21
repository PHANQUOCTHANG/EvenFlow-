# Plan: frontend-real-api (round 2, addresses plan-review.md round 1)

Inputs: `spec.md` (AC-1..AC-29, human-approved 2026-10-21), `contracts.md`. Base develop @ 2970b7f.
No migrations, no new npm/Go dependencies. Human also approved: both backend additions, tokens stay in web storage.

## 0. Changes since round 1

| Finding | Resolution |
|---|---|
| B1 server fetch at build / E2E | No backend fetch at build time: `generateStaticParams` returns `[]` (on-demand ISR), `/events` is `dynamic = "force-dynamic"` with fetch-level `revalidate: 60`. Server fetch failures render an error state (network/5xx) or `notFound()` (404), never throw. Buy-flow pages (checkout, payment, result, waiting) look the event up **client-side**, so Playwright `page.route` mocks see it. Server-side calls in E2E hit a dependency-free Node stub (`tests/e2e/fixtures/stub-api.mjs`) on :8080 started by Playwright `webServer`; E2E event ids become UUIDs (§WP6) |
| M1 ops-service imports mocks | Sample events move to `apps/web/src/lib/sample-data/events.ts` (production module, explicitly sample, used only by ops-service whose snapshot already carries `isSample: true`). AC-19 reads "no production *data path*": event-service and organizer-service never return sample data |
| M2 expired token on Public routes = anonymous | `apiFetch` refreshes **before** sending when the access token's `exp` is within 30 s (decoded, not trusted for auth), in addition to the 401 `token-expired` path |
| M3 cross-tab refresh revokes the family | Before refreshing, re-read storage: if the stored refresh token differs from the one that failed, reuse the stored access token instead. Serialize across tabs with `navigator.locks.request("ef-auth-refresh", …)` when available (fallback: in-tab single-flight) |
| M4 missing files | Added `hooks/use-queue-status.ts`, `components/auth/RoleGuard.tsx`, `app/unauthorized/page.tsx`. "Remember me" off → session in `sessionStorage`; `getSession` reads localStorage then sessionStorage (same keys) |
| M5 shared types across parallel WPs | WP1 freezes `apps/web/src/lib/types/event.ts` and implements `lib/event-service.ts` itself; checkout/waiting/payment/result pages move to WP2 (one owner for the buy flow) |
| m1 i18n | WP2 owns `lib/i18n/messages.ts`; WP3/WP4 must not edit it (use the existing keys or literal text with `lang` attribute) |
| m2 route wording | ServeMux picks the most specific pattern regardless of order; tests assert the resolved route for `/api/v1/events/public` vs `/api/v1/events/{id}` in both gateway and event service |
| m3 domain-gallery | Removed from scope |
| m4 stale availability | Detail page availability uses `revalidate: 10`; UI copy says availability is indicative; waiting room/hold are authoritative |
| m5 mutation/rollback | 7 mutations, exact commands in §5–§6 |
| m6 test-change-request | Expected files listed in §7 |

## 0b. Changes in round 3 (plan-review.md Round 2)

| Finding | Resolution |
|---|---|
| R2-M1 WP1 checkpoint cannot pass its gate | Ownership is **sequential by phase**: WP1 also makes the *minimal compile/build edits* in files that consume the changed types/APIs, then hands them over. WP1 edit list adds: `components/event/ticket-tier-list.tsx` (`UNKNOWN` entry, drop `description`), `components/event/event-card.tsx` and `components/event/event-action-panel.tsx` (state type → `EventStatus`, unknown statuses render as "closed"), `lib/ops-service.ts` (`UNKNOWN` entry; import `SAMPLE_EVENTS` instead of `getAllPublicEvents`), `app/(marketing)/events/[slug]/page.tsx` (`generateStaticParams` → `[]`, drop `getAllEventSlugs`), `app/(marketing)/events/page.tsx` (see R2-M2). Owner after WP1: `event-action-panel.tsx` → WP3. **Every WP replaces, in the same WP, the existing tests its change breaks, exactly as listed in the approved `test-change-request.md`**; the WP1 gate therefore includes `event-service.test.ts` and `ops-service.test.ts` replacements. Expected-red at each checkpoint = only the `*.contract.test.*` files of WPs not yet run (listed per WP in the gate report) |
| R2-M2 `/events` must keep `s-maxage` (i18n.spec.ts:211-219) | No `force-dynamic`. `/events` keeps `export const revalidate = 60`. During `next build` (`process.env.NEXT_PHASE === "phase-production-build"`) the page skips the fetch and prerenders an "updating" shell; at runtime ISR regenerates it on the first request after ≤ 60 s (stale-while-revalidate). Documented as a known ≤ 60 s window after deploy |
| m: result type vs frozen tests | Follow the frozen contract tests: `getEventBySlug/getEventById` → `EventDetailSnapshot \| null` (null on 404), other failures reject with `ApiError`; pages catch and render the error state. §4 updated accordingly |
| m: availability revalidate vs AC-16 | Page `revalidate = 60` kept (AC-16); the availability fetch itself uses `revalidate: 10` — AC-16 governs the page, not every fetch |
| m: client-side notFound (AC-18) | Client components call `notFound()` from `next/navigation` when `getEventById` resolves `null` (supported in client components in Next 15) |
| m: clock skew | Proactive refresh only when the local clock says exp ≤ 30 s AND no refresh happened in the last 30 s; otherwise rely on the 401 path |
| m: duplicated tabs in sessionStorage mode | Accepted limitation (no shared storage between those tabs); documented |
| m: E2E stub details | Stub reads the same fixture JSON as `api-mock.ts`, listens on `127.0.0.1` and `::1`, plain `node:http`; `reuseExistingServer: !process.env.CI`; passes the e2e hygiene grep (no timeouts) |
| m: §7 incomplete | §7 adds `checkout/**/page.test.tsx`, `waiting/**/page.test.tsx`, `tests/e2e/specs/{i18n,a11y,buy-flow}.spec.ts` (slug/id constants) |

## 1. Work packages (exclusive ownership) and DAG

**WP1 → (WP2 ∥ WP3 ∥ WP4)**, **WP5 independent**, **WP6 integrator last**. WP2–WP4 branch from the WP1 checkpoint
commit, each in its own worktree. Max 4 implementers at once.

### WP1 — Foundation (AC-1..AC-8, AC-16 data part, AC-18 data part, AC-27)
- NEW `lib/session.ts`: `getSession/setSession(s, {persist: "local"|"session"})/clearSession`, keys `ef_token`, `ef_refresh`, `ef_user`; SSR-safe; dispatches `ef:session` event; listens to `storage` events for cross-tab updates.
- NEW `lib/api-client.ts`: `apiFetch`, `ApiError`, `readProblem`; proactive refresh (exp ≤ 30 s), 401 `/problems/token-expired` refresh + single retry, cross-tab lock + storage re-read, queue-token problems never refresh, never `credentials`, `cache: "no-store"` for authenticated calls.
- EDIT `lib/api/auth.ts` (gateway base, refresh/logout/me, register→login, google→devices+refresh).
- EDIT `hooks/use-auth.ts`, `components/auth/{LoginForm,RegisterForm,ForgotPasswordForm,RoleGuard}.tsx`, `app/unauthorized/page.tsx`.
- NEW `lib/types/event.ts`: `EventStatus` (9 values), `TierAvailability = "AVAILABLE"|"FEW_LEFT"|"SOLD_OUT"|"UNKNOWN"`, `TicketTierSnapshot {id, name, price, currency, quota?, availability}`, `EventDetailSnapshot {id, slug, title, description, venue, startsAt, saleStartAt, saleEndAt?, state, maxPerOrder, tiers}` (no `bannerUrl`; UI uses its existing placeholder). Re-exported from `lib/event-service.ts` and `components/event/event-card.tsx` keeps importing its state type from here.
- REWRITE `lib/event-service.ts`: `getEventBySlug`, `getEventById`, `getAllPublicEvents`, `getAvailability`; result type `{ok:true, data} | {ok:false, reason:"not_found"|"unavailable"}` for server use, plus thin client wrappers; public endpoints only, no session; `next: {revalidate}` options passed by caller.
- NEW `lib/sample-data/events.ts` (moved `MOCK_EVENTS`, renamed `SAMPLE_EVENTS`).
- EDIT `apps/web/.env.example` (`NEXT_PUBLIC_API_BASE`, drop `NEXT_PUBLIC_API_URL`).
- Checkpoint commit: `feat(web): add session store, shared api client and real event client`.

### WP2 — Buy flow (AC-9..AC-15, AC-18 pages)
- `lib/queue-client.ts`, `lib/queue-status.ts`, `lib/sse.ts`, `hooks/use-queue-status.ts`, `components/queue/{waiting-room,queue-position}.tsx`.
- `lib/hold-client.ts` (+`cancelHold`), `hooks/use-hold-timer.ts`.
- `lib/payment-client.ts` (+`listPaymentMethods`, `issueTickets`), `hooks/use-payment-session.ts`, `hooks/use-order-status.ts`.
- `components/checkout/{checkout-view,payment-handoff-view,order-result-view}.tsx`, `components/event/ticket-tier-list.tsx`.
- Pages: `app/(checkout)/checkout/[eventId]/{page,payment/page,result/page}.tsx`, `app/(queue)/waiting/[eventId]/page.tsx` → thin server shells rendering client components that call `getEventById` client-side (new `hooks/use-event.ts`).
- Owns `lib/i18n/messages.ts`.

### WP3 — Public event pages (AC-16, AC-17, AC-19)
- `app/(marketing)/events/page.tsx`, `app/(marketing)/events/[slug]/page.tsx` (+ their existing tests' replacements), `components/event/event-card.tsx` (status mapping incl. UNKNOWN availability). No other files.

### WP4 — Organizer + ops sample (AC-20..AC-24, AC-19 ops part)
- `lib/organizer-service.ts` (real calls; validators kept; sales stays sample with `isSample`), `hooks/use-organizer-sales.ts`.
- `components/organizer/{event-list,event-wizard,moderation-status-card,sales-dashboard}.tsx`.
- Pages under `app/(organizer)/organizer/**` → client fetching with loading/error/empty states.
- `lib/ops-service.ts` switches to `lib/sample-data/events.ts`.

### WP5 — Backend (AC-25, AC-26), Go
- `services/gateway/internal/proxy/{gateway.go,routes.go}` + tests (resolved-route test for `/public`).
- `services/event/internal/port/repository.go` (`ListPublic(ctx, limit int) ([]*domain.Event, error)`), `adapter/postgres/event_repo.go` (+ tiers), `app/get_event.go`, `adapter/http/{router,handler,dto}.go`, test fakes (additive), integration test under `//go:build integration`.

### WP6 — Integrator (AC-28, AC-29)
- Merge order WP1 → WP2 → WP3 → WP4 → WP5; full gate after each merge.
- `tests/e2e/fixtures/{api-mock.ts,buy-flow-helpers.ts}`: gateway-accurate CORS (no credentials, exact allow/expose lists), hold 200, mocks for auth/login, events by id, availability, payment methods, issue-tickets; `EVENT_ID` becomes a UUID.
- NEW `tests/e2e/fixtures/stub-api.mjs` (node:http, no deps) serving `/api/v1/events/public`, `/slug/{slug}`, `/{id}`, `/v1/events/{id}/availability` from fixture JSON; `tests/e2e/playwright.config.ts` gains `webServer` for it on :8080 (`reuseExistingServer: true`). CI workflow files are NOT changed.
- Apply the approved test-change-request.

## 2. Order and checkpoints

1. WP1 → fast gate (lint, typecheck, build, affected tests, tamper check) → checkpoint commit.
2. WP2/WP3/WP4 (+WP5 anytime) in parallel → fast gate each → commits per WP.
3. WP6 merge + full gate (step 4) → parallel review (step 5) → fix rounds ≤ 3 → verification (step 6) → human sign-off → PR base develop.

## 3. Risks

| Risk | Mitigation |
|---|---|
| Refresh storms / cross-tab revocation | single-flight + Web Locks + storage re-read; unit tests incl. two simulated tabs |
| Build-time fetch | none by design; `npm run build` with `NEXT_PUBLIC_API_BASE` unset is part of the fast gate |
| Per-user data cached by Next | authenticated calls `cache: "no-store"` and run client-side only |
| Mock-asserting tests break | replaced per test-change-request, never deleted silently |
| Teammates touching the same files | `git merge origin/develop` before PR + full gate rerun |
| Local demo without fake payment | backend `sandbox` provider |
| XSS reads stored tokens | pre-existing, accepted by the human for this task, recorded as security debt |

## 4. Server fetch policy

`getEventBySlug/getEventById` resolve `null` on 404 → `notFound()`; network/5xx/timeout (5 s AbortSignal) or empty
`apiBase()` reject with `ApiError` → the page catches and renders an error state with a retry link (never throws out
of render). No fetch at all during `next build`.

## 5. Rollback (checked during verification)

```
git switch -c rollback-check feature/frontend-real-api && git revert -m 1 --no-edit <merge-commit-on-develop>
cd apps/web && npm test && npm run build
go test ./services/gateway/... ./services/event/...
cd tests/e2e && npx playwright test   # against the reverted build
```
Backend additions are additive; old web never calls `/public`. No data step.

## 6. Mutation test (threshold 7/7 killed)

On `api-client.ts`, `session.ts`, `payment-client.ts`, `hold-client.ts`: (1) refresh on any 401 type, (2) remove retry
flag, (3) drop single-flight reuse, (4) skip clearSession on refresh failure, (5) restore fake payment success,
(6) serve cache on 404, (7) map `/problems/per-identity-limit` to `QUANTITY_NOT_ALLOWED`. Each must fail ≥ 1 test.

## 7. Expected test-change-request files

`lib/event-service.test.ts`, `lib/ops-service.test.ts`, `lib/organizer-service.test.ts`, `lib/payment-client.test.ts`,
`lib/hold-client.test.ts`, `lib/queue-client.test.ts`, `lib/queue-status.test.ts`, `hooks/use-auth.test.ts`,
`app/(marketing)/events/page.test.tsx`, `app/(marketing)/events/[slug]/page.test.tsx`,
`app/(organizer)/organizer/**/*.test.tsx`, `components/organizer/*.test.tsx`, `components/checkout/*.test.tsx`,
`components/auth/*.test.tsx`, `tests/e2e/fixtures/*`. Final list comes from the test designer's request.

## 8. Round-3 minors (plan-review.md Round 3, verdict PASS)

- m-R3-1: WP1 keeps `EventPublicState` exported from `components/event/event-card.tsx` (alias of the public subset of `EventStatus`) for `components/showcase/domain-gallery.tsx`.
- m-R3-2: where §1 or `test-change-request.md` contradicts §0b/§9 of contracts, §0b and contracts §9 win (`null`/reject shape, `reuseExistingServer: !process.env.CI`, WP1 minimal-edit list).
- m-R3-3: the build-time `/events` shell is not "≤ 60 s": it is served to the first visitor after each deploy/pod start. Fix: the shell renders a client component that loads `GET /api/v1/events/public` in the browser, so that first visitor still sees the list.
- m-R3-4: accepted as-is (error page for ≤ 60 s if the backend fails during regeneration).
- m-R3-5: verification notes that `/events/[slug]` carries `s-maxage=10` because of the availability fetch; not a regression of AC-16.
- m-R3-6: WP1 also replaces the `unauthorized` page test and `RoleGuard` tests if async `logout` breaks them (added to the TCR).

## 9. Amendment after implementation

- `qrcode@1.5.4` + `@types/qrcode@1.5.6` were added (exact versions) after the human chose "add qrcode" on 2026-10-21 to meet AC-15 (scannable QR). This supersedes "no new npm dependencies" for that package only.
