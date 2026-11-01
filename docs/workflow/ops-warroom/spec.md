# Spec: ops war room and organizer sales on real data (ops-warroom)

Tier: **High-risk** (see `tier.json`). Branch `feature/ops-warroom` from develop @ c5fb598.
Jira: EV-187 / EVF-117 (war room), EV-147 / EVF-60 (Ops API: view/override admit_rate, emergency pause),
EV-186 / EVF-116 (organizer realtime sales). Owner: TranDinhVo.
Contracts: `contracts.md` (frozen after approval). Process: v3 (`docs/multi_agent_coding_workflow_v3_quy_trinh.pdf`).
Prior slices: `docs/workflow/EV-187-ops-war-room/spec.md` (screen + sample data), `docs/workflow/frontend-real-api/spec.md`
(AC-19/AC-24 left ops and sales on labelled sample data).

## 0. Triage

| Question | Answer |
|---|---|
| What changes | 6 new read/write endpoints (waitingroom 3, ticketing 2, ai-worker 1), 6 gateway routes, web war room + sales switch from sample to real data, war-room controls become live |
| Why high-risk | new role-restricted routes, a write API on the live admit rate (can stop a sale), new audit writer, waitingroom gains a Postgres connection, revenue figures |
| What does NOT change | admit controller algorithm, Lua scripts, Redis key layout, buy flow, existing routes, DB schema (no migration) |
| Size | one PR, 4 work packages (plan.md) |

## 1. Goal

An Ops user opens `/ops/events`, picks an event and sees the real funnel, admit rate, queue depth, controller/leader
state, checkout health, per-tier inventory and AI degradation state, refreshed every 2 s, and can override the admit
rate, emergency-pause and resume, every action audited with the actor. An organizer sees real sales of their own event
within ≤ 5 s. The "Dữ liệu mẫu" label disappears exactly where the numbers are real; nothing fabricated remains.

## 2. Scope

In:
- Waitingroom: `GET /v1/ops/events/{id}/queue`, `PUT|DELETE /v1/ops/events/{id}/admit-override` (EVF-60), audit to
  `audit_logs`.
- Ticketing: `GET /v1/ops/events/{id}/inventory`, `GET /v1/organizer/events/{id}/sales`.
- ai-worker: `GET /v1/ops/ai/mode` (role-checked read of the existing `/admin/mode` data).
- Gateway route table entries with roles.
- Web: `lib/ops-service.ts`, `lib/organizer-service.ts#getRealtimeSales`, war-room/sales pages and components, ops
  route guard, tests and E2E mocks.
- Compose: `DATABASE_URL` for waitingroom.

Out (reasons in §7): AI mode override from the UI, waitlist/heartbeat (EVF-58/59), Grafana (EVF-125), trend charts,
peak TPS, ops event list including CLOSED/COMPLETED events, `orders(event_id)` index, httpOnly-cookie tokens, SSE push.

## 3. Definitions

- **Ops** = JWT role `ops`. **Organizer owner** = JWT role `organizer` and `events.organizer_id == sub`.
- **Source** = one of `waitingroom` (queue endpoint), `ticketing` (inventory endpoint), `ai` (ai mode endpoint).
  `SourceHealth` = `ok | stale | unavailable` (existing type).
- **Override** = Redis `meta.override/override_rate` read by the controller every tick. **Pause** = override with rate 0.
- **Snapshot** = `OpsWarRoomSnapshot` composed client-side from the event detail + 3 sources.

## 4. Acceptance criteria

### A. Waitingroom read (EVF-60 "xem", EVF-117)

- **AC-1** Given an `ops` caller and a configured room, When `GET /v1/ops/events/{id}/queue`, Then 200 with the
  contracts §3.1 shape: `lobby = SCARD lobby`, `queued = ZCARD queue`, `admitted_active = ZCOUNT admitted (now,+inf)`,
  `admit_rate_per_sec = meta.admit_rate/1000`, override, `paused`, `sold_out`, `stock_zero`, leader presence + epoch,
  checkout health; `Cache-Control: no-store`.
- **AC-2** Given admitted members whose expiry is in the past, Then they are not counted in `admitted_active`.
- **AC-3** Given no live loadsignal pod, Then `checkout.signal = "none"` with null metrics and the endpoint still
  returns 200; Given a loadsignal read error, `signal = "error"`, still 200.
- **AC-4** Given a room whose `meta` lacks `sale_start_ms`, Then 404 `/problems/not-found`; non-UUID id → 400; Redis
  error → 503.
- **AC-5** Given a caller whose `X-Identity-Role` ≠ `ops` reaching the service directly, Then 403 `/problems/forbidden`;
  no principal → 401. (Second layer behind the gateway.)

### B. Waitingroom write (EVF-60)

- **AC-6** Given an `ops` caller, When `PUT …/admit-override {rate_per_sec, reason}` with `0 ≤ rate ≤ 10000` and reason
  3..200 chars (trimmed), Then Redis `meta.override="1"`, `meta.override_rate=int(rate*1000)` and 200 per contracts §3.2.
- **AC-7** Given an active override, When the controller ticks, Then the admitted batch follows the override within
  ≤ 2 s (EVF-60 AC-1). Verified by an integration test with the real controller + Redis: after PUT rate 0, no new
  member enters `admitted` after the next tick; after DELETE, admission resumes from `MinRate`.
- **AC-8** Given `DELETE …/admit-override {reason}`, Then both meta fields are removed and 200; repeating it returns 200.
- **AC-9** Given invalid input (missing/non-number/negative/>10000/NaN-like rate, reason missing/too short/too long,
  unknown field, body > 4 KiB), Then 400 `/problems/invalid-override` and Redis is unchanged.
- **AC-10** Given a successful apply, Then exactly one `audit_logs` row is written with `actor_type=human`,
  `actor_id=<caller id>`, `action ∈ {ADMIT_OVERRIDE_SET, ADMIT_PAUSED, ADMIT_OVERRIDE_CLEARED}`, `subject=event:{id}`,
  `reason`, metadata `{rate_per_sec, previous, request_id}` (EVF-60 AC-2). Response `audit:"recorded"`.
- **AC-11** Given the audit insert fails or times out (2 s) after the Redis apply, Then the override stays applied,
  the response is 200 with `audit:"failed"`, and one ERROR log line carries actor, action, subject, reason, metadata (D2).
- **AC-12** Given the service started without an audit store (`DATABASE_URL` unset), Then both write endpoints return
  503 `/problems/audit-unavailable` and Redis is unchanged; the read endpoint and the buyer endpoints keep working;
  readiness does not depend on Postgres.
- **AC-13** Given a non-`ops` caller (gateway 403, service 403 as AC-5), Then nothing is applied or audited
  (EVF-60 AC-3).

### C. Ticketing

- **AC-14** Given an `ops` caller, When `GET /v1/ops/events/{id}/inventory`, Then 200 per contracts §4.1 with per-tier
  `quota, available, held, paid, issued` computed exactly as defined there (consumed or released holds not in `held`,
  `VOID/REFUNDED` tickets not in `issued`, only `PAID` orders in `paid`).
- **AC-15** Given an `organizer` caller who owns the event, When `GET /v1/organizer/events/{id}/sales`, Then 200 per
  contracts §4.2; refunded/cancelled/expired/pending orders contribute nothing to sold, revenue or paid_orders.
- **AC-16** Given an organizer who does not own the event, or an unknown event, Then 404 (indistinguishable); role ≠
  `organizer` at the service → 403; ops endpoint with role ≠ `ops` → 403; non-UUID → 400; DB error → 503.
- **AC-17** Given N concurrent requests for the same event within 2 s, Then at most one set of DB queries runs
  (per-event TTL cache + single-flight); queries are bounded by a 2 s timeout.

### D. ai-worker

- **AC-18** Given `X-Identity-Role: ops`, When `GET /v1/ops/ai/mode`, Then 200 per contracts §5.1 with the same `mode`,
  `manual_override` and signals as `/admin/mode`; other role → 403, no identity → 401 (problem JSON). `/admin/*` is
  unchanged.

### E. Gateway

- **AC-19** The 6 routes in contracts §2 exist with the listed upstream, access and roles. A buyer/organizer/moderator
  token gets 403 on every `/v1/ops/...` route without the request reaching the upstream; buyer/ops get 403 on the sales
  route; no token → 401; `ops` reaches the correct upstream with the unchanged path. `/admin/mode` stays 404 for ops.
  Existing route-resolution tests stay green.

### F. Web — war room

- **AC-20** `getOpsWarRoom(eventId)` calls `GET /api/v1/events/{id}`, `GET /v1/ops/events/{id}/queue`,
  `GET /v1/ops/events/{id}/inventory`, `GET /v1/ops/ai/mode` in parallel through `apiFetch` and returns a snapshot with
  `isSample: false`. Mapping: title/saleState from the event (`ON_SALE|SOLD_OUT|SCHEDULED`, else `CLOSED`); funnel
  `lobby/queued/admitted` from queue, `holding/paid/issued` = sums over inventory tiers; `admission.mode = MANUAL` iff
  override active; inventory rows 1:1; AI fields from ai mode (`manualOverride = manual_override != null`);
  `generatedAt` = the oldest `generated_at` among sources that succeeded.
- **AC-21** Source health: a source whose call fails (network, timeout, 5xx, 404 for the queue) is `unavailable` and its
  numbers render "Không có dữ liệu" (existing rule); `waitingroom` is `stale` when `controller.leader_present` is false
  while the sale is `ON_SALE`. No "healthy" indicator appears when any source is not `ok`.
- **AC-22** Errors: event 404 → `OpsServiceError(404)` (page shows "Không tìm thấy sự kiện"); any source 403 →
  `OpsServiceError(403)` ("Không có quyền", polling stops — existing hook behaviour); `AUTH_REQUIRED` → `OpsServiceError(401)`
  and the route guard sends the user to login; event detail fails otherwise, or all three sources fail →
  `OpsServiceError(503)` (hook keeps the last snapshot and shows stale warning).
- **AC-23** The war room shows the "Dữ liệu mẫu" label only when `isSample` is true (never on the real path). The
  admission panel additionally shows: override rate when MANUAL, a prominent "ĐANG TẠM DỪNG" status (text, not colour
  only) when `paused`, "Hết vé" when `soldOut`, and "Không có leader điều phối" when `!leaderPresent`. A checkout-health
  block shows p99 (ms), error rate (%), DB pool (%), pods, or "Không có tín hiệu" when `checkout` is null.
- **AC-24** Controls (EVF-60 UI): the "Điều khiển" panel has (a) rate input (integer 0..10000) + reason (3..200) +
  "Ghi đè admit rate"; (b) "Tạm dừng khẩn cấp" which asks for confirmation and a reason, then sends rate 0;
  (c) "Bỏ ghi đè / Tiếp tục tự động" (only when MANUAL) with reason. Buttons are disabled while a write is in flight or
  when `sources.waitingroom` is `unavailable`, with a visible reason. Client validation mirrors AC-9 before any call.
- **AC-25** Write results: success → status message (role `status`) naming the new state, then an immediate refresh;
  `audit:"failed"` → warning "Đã áp dụng nhưng chưa ghi được nhật ký kiểm toán"; 400 → field error (client-side text, never the
  server `detail`); 403 → "Không có quyền"; 404 → "Sự kiện chưa có phòng chờ"; 503 / network → error with
  retry; never an optimistic state change before the server confirms.
- **AC-26** `/ops/events` lists events from `GET /api/v1/events/public` client-side (loading, empty "Chưa có sự kiện
  nào đang hoặc sắp mở bán", error with retry). Ops pages fetch client-side only (token lives in browser storage), no
  server-side fetch, `force-dynamic` kept or the page is a thin shell.
- **AC-27** The `(ops)` layout wraps content in `RoleGuard allowedRoles={["ops"]}` (same pattern as
  `organizer-only.tsx`); non-ops users are redirected to `/unauthorized`, anonymous users to login.
- **AC-28** `lib/sample-data/events.ts` and `__setOpsSampleOverride` are removed; no production module imports sample
  data any more (the `real-api.contract.test.ts` assertion is replaced via the test change request).

### G. Web — organizer sales

- **AC-29** `getRealtimeSales(eventId)` calls `GET /v1/organizer/events/{id}/sales` and maps contracts §4.2 to
  `RealtimeSalesSnapshot` with `isSample: false`, `fillRatePercentage = round(sold/quota*100)` (0 when quota 0),
  `averageOrderValue = round(gross/paid_orders)` (0 when none), `peakSalesVelocityTps: null`, payment breakdown per
  provider. 404 → error "Không tìm thấy sự kiện hoặc bạn không sở hữu sự kiện này"; 403/401 as elsewhere.
- **AC-30** The sales dashboard drops the sample badge and the sample alert, enables auto-refresh by default (4 s,
  ≤ 5 s, EVF-116 AC-2), shows "Chưa có dữ liệu" for peak TPS, renders the payment breakdown from the list (any provider
  name), and shows an empty state when nothing is sold yet. Error keeps the last data with an error alert.

### H. Cross-cutting

- **AC-31** Gates: Go `gofmt`, `go vet`, `go test -race` for gateway, waitingroom, ticketing (+ integration tags for new
  Redis/Postgres code), golangci-lint 0 issues; ai-worker `pytest` + ruff; web lint, typecheck, Vitest (baseline from
  develop c5fb598), build; Playwright suite incl. a rewritten `ops-war-room.spec.ts` (mocked API: real-data war room,
  override → pause → resume) and a sales check. No test deleted/skipped/weakened except via the approved test change
  request.
- **AC-32** No DB migration; no new npm/Python dependency; only `pgx/v5 v5.9.2` added to `services/waitingroom/go.mod`.

## 5. Edge cases

Override set while no leader is running (applies when a leader appears — shown as MANUAL + no leader); override rate
above `ADMIT_MAX_RATE` (controller clamps; UI shows the effective `admit_rate_per_sec` on the next poll); two Ops
writing at once (last write wins, both audited); clearing after pause (controller resumes from MinRate, not 0); sold-out
while paused (both flags shown); event in public list but room not yet configured (queue 404 → waitingroom unavailable);
organizer event in DRAFT (sales all zero, 200); ai-worker down (AI unavailable, rest live); Postgres overloaded during a
sale (inventory/sales 503 or slow → ticketing unavailable; pause still works — D2); clock skew between services
(generatedAt uses server times; age computed in browser, existing behaviour).

## 6. Decisions for the human (must be answered at spec approval)

- **D1 — Where EVF-60 writes and audit live.** Recommended **A**: waitingroom hosts the override endpoints (it owns the
  keys; `Queue.SetRateOverride` was written for "EV-147 will call it from the API") and writes `audit_logs` through a
  small pgx pool (adds `pgx/v5` to waitingroom go.mod + `DATABASE_URL` in compose). Alternative **B**: ticketing hosts
  the writes (already has Postgres and already writes `wr:{ev}:meta` for sold-out flags), zero new dependency, but a
  second service writes waitingroom control keys.
- **D2 — Audit failure policy.** Recommended: apply first, audit after; on audit failure keep the change, return
  `audit:"failed"` and log ERROR (an emergency pause must not be blocked by an overloaded Postgres). Alternative: fail
  closed (no audit → no change).
- **D3 — Who may read sales.** Recommended: only the owning organizer. Alternative: also `ops` (would add `ops` to the
  route roles and skip the ownership check for ops).
- **D4 — Mandatory reason.** Recommended: `reason` required (3..200) on every write. Alternative: optional.

## 7. Out of scope (with reason)

- AI degradation override from the UI: `POST /admin/mode/{mode}` exists but ai-worker has no audit store; exposing it
  unaudited contradicts EVF-60's audit rule. Read-only in this PR.
- Grafana/alerts (EVF-125), trend charts (no time-series source in the app), peak TPS (no source).
- Waitlist, heartbeat, `queue_sessions` audit (EVF-58/59 phase-5 items).
- Ops event list with CLOSED/COMPLETED events (public list covers SCHEDULED/ON_SALE/SOLD_OUT); a dedicated ops list
  endpoint would be new work.
- `orders(event_id)` index (migration) — mitigated by the 2 s cache; recorded as debt.
- Push (SSE/WebSocket) instead of 2 s / 4 s polling; httpOnly-cookie tokens.

## 8. Definition of Ready

- Contracts frozen in `contracts.md` with file:line references (done).
- D1–D4 answered by the human; spec approved.
- Test designer writes failing tests per AC (red check) before implementation; test change request lists every existing
  test that asserted sample behaviour (plan §7).
