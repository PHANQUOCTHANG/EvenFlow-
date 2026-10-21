# Plan review: frontend-real-api (step 2b, High-risk)

Inputs: `spec.md`, `plan.md`, `contracts.md` + read-only checks of develop @ 2970b7f.

## Verdict: **FAIL** (1 blocker, 5 major)

Contract spot-checks that held: hold returns 200 (`services/ticketing/internal/adapter/http/handler.go:112`);
register/verify returns 201 (`services/identity/internal/adapter/http/handler.go:219`); IdentityResponse fields
(`identity/.../dto.go:43-60`); CORS lists (`services/gateway/internal/proxy/gateway.go:265,268`); queue-token problem
types (`waitingroom/.../handler.go:281-283`, `ticketing/.../handler.go:49-51`); event-service mux has no `/public`
conflict (Go 1.22 literal beats `{id}`, `services/event/internal/adapter/http/router.go:17`). AC-1..AC-29 each map to a WP.

## Findings

### B1 (blocker) — Server-side event fetches break `next build` in CI and the E2E buy flow (AC-16..18, AC-28, AC-29)
Evidence: WP3 turns `event-service.ts` into a server fetch used by RSC pages (`app/(marketing)/events/page.tsx:15`,
`events/[slug]/page.tsx:20-23` generateStaticParams, checkout `page|payment|result/page.tsx`, `waiting/[eventId]/page.tsx:12`).
- `ci.yml:393-398` runs `npm run build` with no backend and no `NEXT_PUBLIC_API_BASE`; `apiBase()` returns `""`
  (`lib/api-base.ts:10`), and a relative URL in Node `fetch` throws. `/events` has `revalidate = 60`, so Next prerenders it at
  build time and `generateStaticParams` runs then too, so the build fails.
- E2E (`.github/workflows/e2e-web.yml:70-73`) builds with `NEXT_PUBLIC_API_BASE=http://localhost:8080`, but
  `tests/e2e/fixtures/api-mock.ts` only mocks **browser** traffic through Playwright routes. The Next server's fetches go to a
  port where nothing listens, so `/events/[slug]`, `/checkout/[id]` and `/waiting/[id]` return 404 or errors.
  `buy-flow-helpers.ts:21` also uses `EVENT_ID = "evt-tech-summit-2026"` (not a UUID, so the backend contract returns 400).
  The plan says "E2E specs unchanged".
- cd-web (`cd-web.yml:100`) has the same problem: the build runner and the web container use one inlined base for server fetches.
Required fix: (a) Write down the server-fetch failure policy. On build or empty base, `generateStaticParams` returns `[]`. `/events`
must not bake an error page into the build (use `dynamic`/no prerender, or throw so ISR keeps the last good page). (b) Choose
and assign an E2E strategy: either a small stub HTTP server on `E2E_API_BASE` for server-side calls, or move the checkout and
waiting lookups to the client. WP6 owns it, together with `e2e-web.yml` and the test-change-request entries for the UUID
`EVENT_ID`/slug in `buy-flow-helpers.ts`, `a11y.spec.ts`, `i18n.spec.ts` and `ops-war-room.spec.ts`.

### M1 (major) — ops-service plan contradicts AC-19 and has no listed owner for its tests
Evidence: `lib/ops-service.ts:10,194,204` calls `getAllPublicEvents()`. It is production code used by `/ops` pages
(`app/(ops)/ops/events/page.tsx:5`, `[eventId]/page.tsx:9`, `components/ops/*`, `hooks/use-ops-war-room.ts:23`). Plan WP3
has it "import the moved fixture" from `src/test/fixtures`, which puts test fixtures in the prod bundle (AC-19). If it
instead keeps calling the now-real `getAllPublicEvents`, the war room silently shows real events labelled as sample.
`ops-service.test.ts:15,53-58` mocks `actual.MOCK_EVENTS`.
Required fix: Give WP3 explicit ownership of a production sample module (e.g. `lib/sample/ops-events.ts`, still `isSample`).
List `ops-service.test.ts`, the ops page tests and the `ops-war-room`/`i18n` E2E specs as affected.

### M2 (major) — Expired token on a Public route gives 404, not 401, so the organizer views of unpublished events never refresh
Evidence: the gateway only rejects missing or expired tokens on `Authenticated` routes. On Public routes it forwards anonymously
(`gateway.go:115-125`, `authenticate` at `:157-176`). `GET /api/v1/events/{id}` is Public (`routes.go:40`). Unpublished
events return 404 to non-owners (`contracts.md:135`). After 15 min idle, the edit and review-status pages (WP4) get 404 for a
DRAFT/PENDING_REVIEW/REJECTED event. AC-4 refresh never fires and the user sees "not found".
Required fix: In `apiFetch`, refresh proactively when the stored access token's `exp` hint has passed (with skew) and the call
is marked `auth: "required"`. Organizer reads use that option. Add a unit test and a note in contracts §0.

### M3 (major) — Single-flight is per tab; refresh-token reuse revokes the whole family
Evidence: `services/identity/internal/adapter/postgres/refresh_token_repo.go:74-80` (reuse leads to `revokeFamily`). The plan uses a
module-level promise (`plan.md:13`). Two open tabs that refresh concurrently log the user out everywhere. The spec's
"refresh token reused → logout" edge case turns this into normal behaviour.
Required fix: Before refreshing, re-read `ef_token`/`ef_refresh`. If they changed since the failed request, retry with the
stored pair and do not refresh. Serialize with `navigator.locks.request` when available, and listen to `storage` events.
Add a test with two clients sharing storage.

### M4 (major) — Files the change needs are missing from WP lists
- `hooks/use-queue-status.ts:114,165` builds the base URL and the SSE headers (`X-Queue-Token` only). AC-9 Bearer on the stream
  must pass through here, or `sse.ts` must read the session itself. Add it and its test to WP2.
- `useAuth` consumers: `components/auth/RoleGuard.tsx:5`, `app/unauthorized/page.tsx:8,54` (calls `logout`, which becomes
  async and does network I/O), and their tests. Add them to WP1.
- `LoginForm.tsx:117-119` writes `sessionStorage` for "not remember me", and `use-auth.ts:52,65,98` reads it. Spec §3 says
  localStorage only. Decide (drop remember-me, or session lives in sessionStorage) and state it in WP1.
- Organizer page tests and `use-organizer-sales.test.tsx`, plus the checkout/waiting `page.test.tsx` files that mock
  `getEventById`. List them in the test-change request.
Required fix: Add these to the owning WP's file list.

### M5 (major) — WP2 and WP3 share types and props but run in parallel without a frozen contract
Evidence: `components/checkout/checkout-view.tsx:15` (WP2) imports `EventDetailSnapshot` from `event-service` (WP3).
`ticket-tier-list.tsx:3` (WP3) exports `TierAvailability`, which AC-16 extends with "unknown". The checkout
`page|payment|result` routes (WP3) render `checkout-view`/`payment-handoff-view`/`order-result-view` (WP2). AC-13 (methods
fetch) and AC-15 (issue-tickets and polling) need someone to decide whether the page or the view fetches. Only
`session.ts`/`api-client.ts` are frozen (`plan.md:18`), so the integrator gets semantic conflicts, not mechanical ones.
Required fix: Freeze `EventDetailSnapshot`, `TicketTierSnapshot`, `TierAvailability` and the props of the three checkout
views in the WP1 checkpoint (types-only file). Alternatively, move the checkout pages to WP2 and make WP2 depend on WP3.

### Minor
- m1 `lib/i18n/messages.ts` is owned by WP2, and WP3 and WP4 must "request keys" mid-flight (`plan.md:25`). Pre-allocate keys
  in WP1, or let each WP use literal text (the file has 92 lines and few consumers).
- m2 `/api/v1/events/public` already matches the Public `GET /api/v1/events/{id}` in the gateway (`routes.go:40`). ServeMux
  ignores registration order, so "registered before" (`plan.md:39`, `contracts.md:181`) is wrong. The test should assert the
  resolved handler via the mux, not the slice order.
- m3 `components/showcase/domain-gallery.tsx:3-16` imports no mock data. Remove it from WP3.
- m4 ISR: `/events/[slug]` availability is cached 60 s (stale SOLD_OUT/FEW_LEFT). Document that the waiting room and the hold
  are authoritative.
- m5 Mutation set (`plan.md:76-80`) skips session clear on refresh failure and the hold problem-type mapping (AC-10).
  Add 2 mutations (7/7 threshold). Rollback check: name the exact commands (`git revert -m 1`, `npm test`, `go test
  ./services/gateway/... ./services/event/...`, E2E) and the branch name.
- m6 The plan defers the test-change-request. Enumerate expected files now (`event-service.test.ts`, `ops-service.test.ts`,
  `events/[slug]/page.test.tsx`, `use-auth.test.ts`, `payment-client.test.ts`, `hold-client.test.ts`, organizer component tests)
  so reviewers can check that nothing was weakened.

## Round 2

Inputs: `plan.md` round 2 (§0 resolution table), `spec.md`, `contracts.md`, plus read-only checks of the worktree
(`feature/frontend-real-api`, untracked `*.contract.test.*` from the test designer), Next `15.5.27` in `apps/web/node_modules`.

### Verdict: **FAIL** (0 blocker, 2 major)

### Round-1 resolutions checked
| R1 | Status | Evidence |
|---|---|---|
| B1 | Build part resolved, E2E strategy sound, **but `/events` force-dynamic breaks an existing E2E** (R2-M2) | `generateStaticParams` → `[]` plus on-demand ISR is fine. Next 15.5 still writes the Data Cache for an explicit `next.revalidate` under `force-dynamic` (`node_modules/next/dist/server/lib/patch-fetch.js:354-361`), so the semantic claim holds. Checkout and waiting lookups move client-side, so `page.route` sees them. |
| M1 | Resolved | `real-api.contract.test.ts` enforces "no `MOCK_EVENTS`" and "sample-data imported only by ops-service" |
| M2 | Resolved | Proactive refresh at exp ≤ 30 s, opaque token gives no refresh (`api-client.contract.test.ts:276-330`) |
| M3 | Resolved for localStorage (see m-R2-4 for sessionStorage) | `api-client.contract.test.ts:334-405`. jsdom 25 has no `navigator.locks`; the test stubs it and also covers the fallback |
| M4 | Resolved (files added). `lib/sse.ts:156` (`credentials: "include"`) is owned by WP2 | §7 is still incomplete (m-R2-2) |
| M5 | Types frozen and buy-flow pages moved to WP2, but the WP1 checkpoint cannot go green (R2-M1) | |
| m1–m6 | m1, m2, m3, m5 resolved. `GET /api/v1/events/public` vs `GET /api/v1/events/{id}`: same method, literal wins, no ServeMux conflict (`services/gateway/internal/proxy/routes.go:40`, `gateway.go:73-80`). m4 and m6 partial (m-R2-3, m-R2-2) | |

### Findings

#### R2-M1 (major) — The WP1 checkpoint cannot pass its own fast gate (lint, typecheck, build, affected tests): the type freeze breaks files owned by WP2, WP3 and WP4
WP1 changes `TierAvailability` (adds `UNKNOWN`), drops `TicketTierSnapshot.description`, widens `state` to the 9-value `EventStatus`,
removes `MOCK_EVENTS`/`getAllEventSlugs`, and makes `getAllPublicEvents` real. At the checkpoint, before WP2–WP4 run:
- `components/event/ticket-tier-list.tsx:14-15` `Record<TierAvailability,…>` is missing `UNKNOWN`, and `:69-71` reads `tier.description`. This is WP2's file, so typecheck fails.
- `lib/ops-service.ts:148` `Record<TierAvailability,…>`, and `:10,194,204` call `getAllPublicEvents`, which is now real. This is WP4's file, so typecheck fails and ops shows real events.
- `app/(marketing)/events/[slug]/page.tsx:9,21` imports `getAllEventSlugs` (WP3), so typecheck and build fail.
- `app/(marketing)/events/page.tsx:6,15` still has `revalidate = 60` plus a real `getAllPublicEvents`, so `npm run build` with the base
  unset prerenders the page and the fetch rejects (`event-service.contract.test.ts:198-204`). The build fails, and §3 makes exactly this build part of the fast gate.
- `components/event/event-card.tsx:13,97` and `components/event/event-action-panel.tsx:6,19` (**not owned by any WP**) type `state`
  as the 6-value `EventPublicState`, so passing `EventStatus` fails typecheck.
- `lib/event-service.test.ts:4,12` and `lib/ops-service.test.ts:59` assert or mock the removed exports.

Fix (choose one and write it in §1/§2): (a) WP1 also makes the minimal compile and build edits in those files: the `UNKNOWN` record entries,
`generateStaticParams` → `[]`, the `/events` render-mode change (see R2-M2), ops-service → `lib/sample-data/events.ts`, and a
`toPublicState()` narrowing helper in `lib/types/event.ts`. The owning WPs then build on those edits. Or (b) the WP1 checkpoint
keeps a compatibility surface (`getAllEventSlugs` returning `[]`, optional `description`) and the gate is scoped. In both cases, assign
`event-action-panel.tsx` to an owner (WP3), and list which contract tests are expected red at each checkpoint, since they turn green only after WP6.

#### R2-M2 (major) — `/events` with `dynamic = "force-dynamic"` breaks an existing E2E assertion (AC-28/AC-29)
`tests/e2e/specs/i18n.spec.ts:211-219` asserts that `/events` and `/events/[slug]` respond with `Cache-Control` containing `s-maxage` and
**not** `no-store` ("van la ISR"). `force-dynamic` makes Next send `private, no-cache, no-store…`. Fetch-level `revalidate`
does not change the response header. The spec must not be weakened without a human-approved test change (AC-29), and the plan does not
list it.
Fix: keep `/events` as ISR (`revalidate = 60`). During the build (`process.env.NEXT_PHASE === "phase-production-build"` or `apiBase()`
empty), render the "temporarily unavailable" state without fetching. At runtime, on `unavailable`, throw from the page so ISR keeps
serving the last good page (Next keeps the stale entry when regeneration fails), and add an `error.tsx` for the first-render case.
Document that for up to 60 s after deploy the build-time shell may be served. If the human prefers `force-dynamic`, put
`i18n.spec.ts:211-219` in the test-change request explicitly with the reason. `/events/[slug]` with `generateStaticParams → []` +
`revalidate` keeps `s-maxage` and is fine.

#### Minor
- **m-R2-1** The plan's event-service API contradicts the frozen contract tests. WP1 and §4 say the server helpers return `{ok:false, reason}` and "never
  throw", but `event-service.contract.test.ts:129-150,162-170,198-204` require `getEventBySlug`/`getEventById` to return
  `null` on 404/400 and **reject** on 5xx/network, and `getAllPublicEvents` to reject. Name the API exactly: the exported getters keep
  null/reject semantics and pages wrap them in `try/catch` → error state (or `throw` per R2-M2). Map 400 (non-UUID) → `null` as the test requires.
- **m-R2-2** §7 still omits files that round-1 M4/m6 named: `app/(checkout)/checkout/[eventId]/{,payment/,result/}page.test.tsx`,
  `app/(queue)/waiting/[eventId]/page.test.tsx`, `hooks/use-organizer-sales.test.tsx`, `components/event/ticket-tier-list.test.tsx`,
  `components/event/event-card.test.tsx` (state type), the `RoleGuard`/`unauthorized` tests, the `use-queue-status` tests and the ops page tests. In E2E,
  `i18n.spec.ts:363` hard-codes `SCHEDULED_SLUG = "evenflow-grand-concert-2026"`, `ops-war-room.spec.ts:24` hard-codes
  `evt-tech-summit-2026` (fine only if the `SAMPLE_EVENTS` ids are kept), and `i18n.spec.ts:211-219` is covered by R2-M2. WP6 lists only `fixtures/*`, so either
  the stub serves those slugs or the specs go into the request.
- **m-R2-3** The availability fetch uses `revalidate: 10`, so the route's effective revalidate becomes 10 (Next takes the minimum) and `s-maxage=10`. The spec's
  AC-16 still says "ISR `revalidate = 60` is kept". Align the AC wording or the value.
- **m-R2-4** M3 and sessionStorage: with "remember me" off, a duplicated tab copies sessionStorage, and `storage` events do not cross tabs
  for sessionStorage. The storage re-read cannot see the other tab's rotation, so the two tabs reuse the refresh token and the family gets revoked. Either
  accept and document this as a known limitation, or broadcast the rotated pair over `BroadcastChannel("ef-auth")`.
- **m-R2-5** AC-18 after the client-side move: the plan does not say how "unknown id → `notFound()`" happens in `hooks/use-event.ts`
  consumers (call `notFound()` during render when the result is `null`; the HTTP status is 200, which should be noted) or what the loading/error states look like.
  The generic `generateMetadata` titles for the buy-flow pages should also be noted.
- **m-R2-6** E2E stub details:
  - Share one fixture JSON between `stub-api.mjs` and `api-mock.ts` so the server and browser views agree on ids.
  - Let `listen(8080)` bind dual-stack, because the web server fetches `http://localhost:8080`.
  - Do not use `new Promise(…setTimeout…)`: the hygiene grep scans `*.mjs` (`.github/workflows/e2e-web.yml:51`).
  - Use `reuseExistingServer: !process.env.CI`, so a local real gateway on :8080 is not mixed silently with `page.route` mocks.
- **m-R2-7** Proactive refresh and clock skew: if the client clock runs more than 15 min ahead, every call refreshes (one rotation per call). Estimate
  the skew from `iat` when the session is stored, or cap proactive refreshes at one per N seconds.

## Round 3

Inputs: `plan.md` §0b (round-2 resolutions), `spec.md`, `contracts.md`, `test-change-request.md`, plus read-only checks of the worktree
(Next `15.5.27`, `apps/web/next.config.mjs`, `.github/workflows/e2e-web.yml`, `tests/e2e/**`, a grep of every importer of the changed APIs).

### Verdict: **PASS** (0 blocker, 0 major, 6 minor)

### Round-2 resolutions checked
| R2 | Status | Evidence |
|---|---|---|
| R2-M1 checkpoint gate | Resolved | §0b gives WP1 the minimal compile and build edits in every file that consumes the changed types or APIs: `ticket-tier-list.tsx`, `event-card.tsx`, `event-action-panel.tsx`, `ops-service.ts` and both `events` pages. Ownership then passes on: ticket-tier-list → WP2, event-card/event-action-panel/pages → WP3, ops-service → WP4. A grep of importers of `event-service`, `EventPublicState`, `TierAvailability`, `TicketTierSnapshot`, `EventDetailSnapshot`, `MOCK_EVENTS`, `getAllEventSlugs`, `use-auth` and `lib/api/auth` finds every production importer owned by some WP, except `components/showcase/domain-gallery.tsx` (m-R3-1). `components/checkout/ticket-tier-card.tsx` declares its own lowercase `TierAvailability`, and `checkout-view.tsx:42` maps from `string`, so it is unaffected. The rule "each WP replaces in the same WP the tests its change breaks" covers the cross-owner tests that break at the WP1 checkpoint (`ticket-tier-list.test.tsx:13` `description`, and the `MOCK_EVENTS` users in the checkout, waiting, `[slug]` and `checkout-view` tests). All of these are listed in `test-change-request.md` |
| R2-M2 `/events` ISR | Resolved | Next sets `process.env.NEXT_PHASE = "phase-production-build"` before it creates the static worker (`node_modules/next/dist/build/index.js:1010-1011`). Workers inherit the env, and `define-env.js` does not inline `NEXT_PHASE`, so the standalone `server.js` sees the variable unset at runtime. `export const revalidate = 60` with no dynamic API (`app/layout.tsx:32` avoids `cookies()`/`headers()`) prerenders, so `/events` keeps `s-maxage=60, stale-while-revalidate…`, which satisfies `i18n.spec.ts:211-219`. `/events/[slug]` with `generateStaticParams → []` renders on demand as ISR. The availability fetch at `revalidate: 10` lowers the route to `s-maxage=10`, which is still `s-maxage` and not `no-store`. Passing a fetch `signal` does not disable the Data Cache (`patch-fetch.js:596-600`). `a11y.spec.ts:117-120` needs only an `h1` on `/events`, so the shell must keep the header |
| m-R2-1 API shape | Resolved in §0b/§4 (null on 404/400, reject otherwise). Stale text remains (m-R3-2) | |
| m-R2-2..7 | Resolved or accepted (sessionStorage dup-tab limitation documented; clock-skew throttle; stub on 127.0.0.1 + ::1 matches the server fetch to `localhost:8080` from `e2e-web.yml:70-73`) | |

### Findings (minor only)
- **m-R3-1** `components/showcase/domain-gallery.tsx:12,29` (and `showcase/page.tsx`, `domain-gallery.test.tsx`) import `type EventPublicState`
  from `event-card.tsx`, and no WP owns them. When WP1 changes event-card's state type to `EventStatus`, it must keep exporting
  `EventPublicState` (the 6-value subset), or typecheck breaks in an unowned file. State this in the WP1 edit list.
- **m-R3-2** Stale text contradicts §0b and the frozen tests:
  - §1 WP1 still describes `event-service` as returning `{ok:true…}|{ok:false…}`.
  - The first row of `test-change-request.md` says "tra Result `{ok,...}`".
  - §1 WP6 says `reuseExistingServer: true`, while §0b says `!process.env.CI`.
  - §1 WP1 does not list the extra compile edits from §0b.
  - §1 WP3 says "No other files" but now owns `event-action-panel.tsx`.

  Fix all of these in §1 so that implementers read a single source.
- **m-R3-3** The claim "known ≤ 60 s window after deploy" is inaccurate. With stale-while-revalidate, the build-time "updating" shell is served
  to every request until the first request that arrives **after** 60 s triggers regeneration, and that request still gets the shell. On a
  low-traffic deployment (the demo), the first real visitor after each deploy or pod start sees no events. Either reword the limitation as
  "first visit after deploy per instance", or have the shell mount a small client component that fetches `/api/v1/events/public` from the
  browser (CORS already allows it). The shell must keep the `h1` (`a11y.spec.ts:118`).
- **m-R3-4** §4 "never throws out of render" applies to `/events` regeneration too. A transient backend failure during regeneration therefore
  replaces a good cached list with the error state for 60 s. This is consistent with AC-17. Throwing during regeneration only, when a previous
  good entry exists, would keep the stale page; ISR keeps the old entry on error. Optional.
- **m-R3-5** AC-16 says "ISR `revalidate = 60` is kept", but the effective route revalidate of `/events/[slug]` is 10 s, because of the availability
  fetch. Add a sentence to the verification report so the reviewer does not flag `s-maxage=10` as a regression.
- **m-R3-6** `test-change-request.md` does not list `app/unauthorized/page.test.tsx` or the `RoleGuard` tests, though WP1 changes `logout`
  to async. Add them under "Co the vo".
