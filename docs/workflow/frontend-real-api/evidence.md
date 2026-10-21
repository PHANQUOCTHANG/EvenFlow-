# Verification evidence: frontend-real-api (v3 step 6, High-risk)

Independent verifier, 2026-10-21. Everything below was produced in a clean, detached worktree
(`git worktree add --detach ../EvenFlow-verify 5077c7e`), which was removed afterwards. Nothing was committed, and
branch `feature/frontend-real-api` was not changed. Raw logs are in `verify-logs/` (43 files).

## 1. Commit match

| Check | Result |
|---|---|
| `gate-report.json` `commit_sha` | `06fd7c446a9af78de5a33d4c573e150b50d55890` = `06fd7c4` (MATCH) |
| `git diff --stat 06fd7c4 5077c7e` | 11 files, all under `docs/workflow/frontend-real-api/` (findings-round4-*.json, findings.json, gate-report.json, gate/*.log). No code changed, so the code verified at 5077c7e is the gated code. |

## 2. Clean-checkout results

| Step | Command | Exit | Result | Log |
|---|---|---|---|---|
| web install | `npm ci` | 0 | added 548 packages | `web-npm-ci.log` |
| web lint | `npm run lint` | 0 | 0 errors (warnings only, in auth forms: unused vars / `any` / `<img>`) | `web-lint.log` |
| web typecheck | `npm run typecheck` | 0 | `tsc --noEmit` clean | `web-typecheck.log` |
| web build | `env -u NEXT_PUBLIC_API_BASE npm run build` | 0 | builds; `/events` is ISR (Revalidate 1m) | `web-build.log` |
| web unit | `npx vitest run` | 0 | **152 files, 2342 tests passed, 0 failed** (baseline 1946) | `web-vitest.log` |
| gateway | `go vet ./...`, `go vet -tags integration ./...`, `go test ./... -count=1` | 0/0/0 | config, proxy, ratelimit ok | `go-gateway.log` |
| event | same three | 0/0/0 | http, app, domain, worker ok | `go-event.log` |
| event integration (AC-26) | `go test -tags integration ./internal/adapter/postgres/ -run ListPublic` (Docker Desktop started for this) | 0 | 2/2 PASS | `go-event-integration-listpublic.log` |
| e2e | like `full-gate.sh`: build with `NEXT_PUBLIC_API_BASE=http://localhost:8080`, standalone `server.js` on 127.0.0.1:3000, `tests/e2e npm ci`, `CI=1 npx playwright test` | 0 | **38 passed** | `e2e-build.log`, `e2e-playwright.log` |
| tamper | deleted tests vs 2970b7f; new skip/only; contract tests changed since freeze d7740fa | — | no deleted test files; no new skip/only/todo/t.Skip; only change is the approved G-9 line in `hold-client.contract.test.ts` | `tamper-check.log` |

Raw tails:

```
# web-vitest.log
 Test Files  152 passed (152)
      Tests  2342 passed (2342)
exit=0

# e2e-playwright.log
  ok 7 [chromium] › buy-flow.spec.ts:44:7 › EV-198 luong mua ve (backend gia lap) › AC-1..AC-4: xep hang -> duoc goi -> giu ve -> thanh toan -> nhan ve (11.7s)
  38 passed (19.0s)
exit=0

# go-gateway.log / go-event.log (go test part)
ok  github.com/eventflow/eventflow/services/gateway/internal/config     0.683s
ok  github.com/eventflow/eventflow/services/gateway/internal/proxy      1.519s
ok  github.com/eventflow/eventflow/services/gateway/internal/ratelimit  0.350s
ok  github.com/eventflow/eventflow/services/event/internal/adapter/http 1.145s
ok  github.com/eventflow/eventflow/services/event/internal/app          0.334s
ok  github.com/eventflow/eventflow/services/event/internal/domain       0.300s
ok  github.com/eventflow/eventflow/services/event/internal/worker       3.209s

# go-event-integration-listpublic.log
--- PASS: TestTichHop_ListPublic_LocTrangThaiSapXepVaTiers (4.56s)
--- PASS: TestTichHop_ListPublic_CatTheoLimit (0.04s)
ok  github.com/eventflow/eventflow/services/event/internal/adapter/postgres 7.109s
```

## 3. Acceptance criteria (AC-1..AC-29)

Every test named here was seen passing in `web-vitest.log`, `go-*.log` or `e2e-playwright.log` from this run.
Web paths are relative to `apps/web/`. "(n)" is the number of passing `it()` cases in that describe block.

| AC | Status | Evidence (tests that passed here + runtime) |
|---|---|---|
| AC-1 login | DONE | `src/lib/api/auth.contract.test.ts::loginUser (AC-1)` (5); `src/lib/session.contract.test.ts::session (AC-1, AC-7)` (6); `src/real-api.contract.test.ts::mot bien moi truong NEXT_PUBLIC_API_BASE (AC-1, AC-27)` (2: no 8083 / NEXT_PUBLIC_API_URL) |
| AC-2 register→login | DONE | `auth.contract.test.ts::registration (AC-2)` (3); `src/lib/fix1/f6-register-autologin.test.tsx::F-6` (3) |
| AC-3 Bearer, no credentials/X-Identity | DONE | `api-client.contract.test.ts::apiFetch co ban (AC-3)` (7); `hold-client.contract.test.ts::requestHold: request (AC-3, AC-10)` (3); `payment-client.contract.test.ts::initiatePayment: request (AC-3, AC-12)` (4); `queue-client.contract.test.ts::joinQueue (AC-3, AC-9)` (5), `::fetchQueueStatus (AC-3, AC-5, AC-9)` (6); `organizer-service.contract.test.ts` (28 total); `real-api.contract.test.ts::khong credentials include, khong X-Identity-* (AC-3)` (2) |
| AC-4 refresh single-flight + single retry | DONE | `api-client.contract.test.ts::refresh don le (AC-4)` (5), `::refresh dong thoi (AC-4, concurrent 401)` (2), `::refresh that bai (AC-4)` (2), `::refresh chu dong khi exp <= 30s` (5), `::hai tab: doc lai storage truoc khi refresh` (5); `auth.contract.test.ts::refreshSession` (2); `hold-client.contract.test.ts::requestHold + refresh (AC-4)` (2); `session.contract.test.ts::clearSession (AC-4, AC-6)` (2), `::persist: local / session` (6); `fix2/g1-lock-refresh.test.ts::G-1` (2) + `::H-1` (7); `fix1/f4-proactive-refresh.test.ts` (6); `fix1/f14-lock-timeout.test.ts` (1). Mutants 1-4 killed (section 4) |
| AC-5 queue-token problems do not refresh | DONE | `api-client.contract.test.ts::401/403 cua queue token KHONG refresh (AC-5)` (5); `queue-client.contract.test.ts::fetchQueueStatus (AC-3, AC-5, AC-9)` (6); `hold-client.contract.test.ts::requestHold + refresh` (queue-token 401 case). Mutant 1 killed by these |
| AC-6 logout | DONE | `auth.contract.test.ts::logout (AC-6)` (4); `session.contract.test.ts::clearSession (AC-4, AC-6)` (2); `fix1/f12-logout-inflight.test.ts` (1), `f15-logout-session-write.test.ts` (3), `f17-logout-clears-buyflow.test.ts` (1) |
| AC-7 useAuth | DONE | `src/hooks/use-auth.contract.test.tsx::useAuth (AC-7)` (6); `session.contract.test.ts::session (AC-1, AC-7)` (6). The manual "sign in → reload" check in traceability was not run by hand; the contract test covers expired-access + valid-refresh = logged in, and no email read from the JWT |
| AC-8 Google → devices → refresh | DONE | `auth.contract.test.ts::loginWithGoogle (AC-8)` (4); `fix1/f16-google-remember.test.tsx` (3) |
| AC-9 queue via session; otp/device-required actionable | DONE | `queue-client.contract.test.ts::joinQueue` (5), `::fetchQueueStatus` (6), `::connectSse (AC-9)` (1); `components/queue/waiting-room.wp2.test.tsx::WaitingRoom — loi join co the hanh dong (AC-9)` (3); `fix1/f11-waiting-auth.test.tsx` (3); E2E `buy-flow.spec.ts:118 AC-5: join tra 401 -> phong cho bao can dang nhap/OTP` ok |
| AC-10 requestHold (amended G-9) | DONE | `hold-client.contract.test.ts::requestHold: thanh cong (AC-10)` (2: 200 and 201), `::requestHold: anh xa loi (AC-10)` (11, incl. `403 /problems/forbidden -> FORBIDDEN`, `403 about:blank -> NOT_ADMITTED`, MUTATION per-identity vs quantity), `::requestHold: request` (3); `components/checkout/checkout-view.fix3.test.tsx::CheckoutView - G-9` (1). Tamper: the only contract-test edit is the approved G-9 row. Mutant 7 killed |
| AC-11 cancelHold | DONE | `hold-client.contract.test.ts::cancelHold (AC-11)` (5) |
| AC-12 no fake payment success | DONE | `payment-client.contract.test.ts::initiatePayment: khong bao gio gia thanh cong (AC-12)` (9), `::initiatePayment: request` (4); `real-api.contract.test.ts::payment-client khong con nhanh gia thanh cong / gia gia tri mac dinh`. Mutant 5 killed |
| AC-13 payment methods | DONE | `payment-client.contract.test.ts::listPaymentMethods (AC-13)` (3); `components/checkout/payment-handoff-view.test.tsx` "chỉ liệt kê cổng đang bật, hiển thị cả stripe", "lỗi tải cổng thanh toán: hiện lỗi + thử lại, không có danh sách cứng", "không có cổng nào đang bật"; E2E buy flow (methods mocked) ok |
| AC-14 getOrderDetails real fields, 404, no cache | DONE | `payment-client.contract.test.ts::getOrderDetails (AC-14)` (7, incl. "404 -> reject NOT_FOUND, KHONG phuc vu cache sessionStorage"); `order-result-view.test.tsx::OrderResultView — phat hanh ve va loi (AC-14, AC-15)` (4). Mutant 6 killed |
| AC-15 issue-tickets stable key, QR, polling | DONE | `payment-client.contract.test.ts::issueTickets (AC-15)` (6); `components/checkout/ticket-qr.test.tsx::TicketQr (AC-15)` (4); `order-result-view.test.tsx` (AC-14, AC-15) (4); `fix1/f13-issue-tickets.test.tsx` (8); E2E `buy-flow.spec.ts:44 ... thanh toan -> nhan ve` ok |
| AC-16 /events/[slug] | DONE (see note) | `event-service.contract.test.ts::getEventBySlug (AC-16)` (10); `app/(marketing)/events/[slug]/page.test.tsx` (11 + 2); `fix1/f18-availability-url.test.ts` (1). Runtime (`runtime-ac16-17-curl.txt`, `runtime-ac16-17-html.txt`, standalone server + `stub-api.mjs`): `/events/vietnam-tech-summit-2026` 200, title "Vietnam Tech Summit 2026 — Kỷ Nguyên AI & Cloud Scale", tier prices 2.500.000 đ / 900.000 đ from the stub; `/events/evenflow-grand-concert-2026` and `/events/indie-acoustic-night-hanoi` 200 with stub titles; `/events/khong-ton-tai-xyz` → **404**; no mock strings in HTML. `export const revalidate = 60` is present. **Note:** the served header is `Cache-Control: s-maxage=10`, not 60, because the availability fetch uses `revalidate: 10` and Next takes the lowest value. plan.md §0b accepted this ("AC-16 governs the page, not every fetch"). It is still ISR (not no-store); E2E `i18n.spec.ts:213` checks this and passes |
| AC-17 /events list | DONE | `event-service.contract.test.ts::getAllPublicEvents (AC-17)` (4); `app/(marketing)/events/page.test.tsx::EventsListPage (EVF-111, AC-17)` (5); `components/event/event-list-view.test.tsx`, `events-browser-list.test.tsx`. Runtime: first `/events` request returns the build-time shell ("Đang tải danh sách sự kiện...", `x-nextjs-cache: STALE`), the second returns all 3 stub events (`HIT`), `Cache-Control: s-maxage=60, stale-while-revalidate`. E2E `i18n.spec.ts:213 /events: Cache-Control cua ISR (s-maxage)` ok. The error state is covered by the page test only; I did not exercise it at runtime |
| AC-18 resolve event by UUID | DONE | `event-service.contract.test.ts::getEventById (AC-18)` (4); page tests `checkout/[eventId]/page.test.tsx` (2), `payment/page.test.tsx` (3), `result/page.test.tsx` (3), `waiting/[eventId]/page.test.tsx` (2); `payment-page-client.test.tsx` (4), `result-page-client.test.tsx` (2), `event-loader.test.tsx` (3) |
| AC-19 no mock in event/organizer service | DONE | `real-api.contract.test.ts::khong con mock trong ma san xuat (AC-19)` (5); `event-service.contract.test.ts` (no sample data on 404/5xx); `organizer-service.contract.test.ts` |
| AC-20 organizer list, 9 statuses | DONE | `organizer-service.contract.test.ts::getOrganizerEvents / getOrganizerEventById (AC-20)` (6); `components/organizer/event-list.status.test.tsx` (6); `organizer/events/page.test.tsx (AC-20)` (3) |
| AC-21 create → tiers → submit, partial failure | DONE | `organizer-service.contract.test.ts::createOrganizerEvent (AC-21)` (11); `event-wizard.partial.test.tsx::lỗi một phần khi tạo (AC-21)` (4); `event-wizard.fixround1.test.tsx` (9); `organizer/events/new/page.test.tsx (AC-21)` (7) |
| AC-22 edit full PUT, tier quota only | DONE | `organizer-service.contract.test.ts::updateOrganizerEvent (AC-22)` (4), `::tiers (AC-22)` (3); `organizer-service.test.ts` (22, incl. validateTierQuotaUpdate); `edit/page.test.tsx (AC-22)` (22); `event-wizard.partial.test.tsx::chỉnh sửa (AC-22)` (4) |
| AC-23 submit / cancel | DONE | `organizer-service.contract.test.ts::submit / cancel (AC-23)` (4); `moderation-status-card.actions.test.tsx::nộp duyệt và huỷ (AC-23)` (10); `review-status/page.test.tsx (AC-23, AC-24)` (7) |
| AC-24 real status, sample labelled | DONE | `moderation-status-card.actions.test.tsx::trạng thái thật và nhãn mẫu (AC-24)` (11); `sales-dashboard.test.tsx` "gắn nhãn Dữ liệu mẫu rõ ràng trước các khối số liệu (AC-24)"; `sales/page.test.tsx (AC-24)` (2) |
| AC-25 gateway CORS | DONE | `go-gateway-ac25-26-verbose.log`: PASS `TestGateway_CORS_PreflightChoPhepHeaderCuaPhongCho`, `TestGateway_CORS_ExposeHeaders`, `TestGateway_CORS_KhongCoAllowCredentials`, `TestGateway_CORS_OriginLaKhongDoi`, `TestGateway_CORS` |
| AC-26 public events endpoint | DONE | gateway PASS `TestGateway_EventsPublic_CongKhaiToiEventService`, `_TokenHongBiBoQua`, `_KhongLamHongRouteCu`; event PASS `TestPublicEvents_*` ×7 (`go-event-ac26-verbose.log`); `internal/app` list_public contract test (in `go test ./...` ok); integration PASS `TestTichHop_ListPublic_LocTrangThaiSapXepVaTiers`, `TestTichHop_ListPublic_CatTheoLimit` (real postgres:16-alpine via testcontainers) |
| AC-27 one env var | DONE | `real-api.contract.test.ts::mot bien moi truong NEXT_PUBLIC_API_BASE (AC-1, AC-27)` (2: no `NEXT_PUBLIC_API_URL`/8083 in code; `.env.example` documents it) |
| AC-28 E2E mock alignment, buy flow | DONE | `e2e-playwright.log`: 38/38 passed (retries 0), incl. `buy-flow.spec.ts:44 AC-1..AC-4 ... nhan ve`, `buy-flow.spec.ts:118`, `buy-flow.spec.ts:157/182` (mock reports unhandled requests), a11y buy flow in light and dark |
| AC-29 gates | DONE | lint 0 errors, typecheck 0, build 0, vitest 2342/2342 (≥ 1946), go vet (+integration tag) and go test for gateway and event 0; tamper: 0 deleted test files, 0 new skip/only, the only contract-test change is the approved G-9 one |

**Count: 29 DONE / 0 NOT DONE.**

## 4. Mutation test (plan.md §6, threshold 7/7)

Each mutant was applied alone in the clean checkout by `verify-logs/mutate.js` (exact-anchor replace).
Command: `npx vitest run src/lib --testTimeout=15000` (49 files, 702 tests, including the contract tests and the fix1/fix2/fix3 regression tests). Between mutants: `git checkout -- .`. The diff for each mutant is in `verify-logs/mutation-<n>.log` and the vitest output in `mutation-<n>.vitest.log`.

| # | Mutation (file) | Diff (essence) | Result | Killing tests (examples) |
|---|---|---|---|---|
| 1 | refresh on any 401 type (api-client.ts) | `- if ((await readProblem(res)).type !== TOKEN_EXPIRED) return res;` | **KILLED**: 6 failed / 3 files | api-client.contract `401 /problems/queue-token-expired -> khong goi refresh`, `401 /problems/unauthorized ... cung khong refresh`; hold-client.contract `queue-token het han (401) KHONG refresh`; queue-client.contract ×2 |
| 2 | remove single-retry guard (api-client.ts) | `- return fetch(url, withBearer(base, fresh));` `+ return apiFetch(path, init);` | **KILLED**: recursion crashed 2 workers ("Worker exited unexpectedly", 2 files and 40 tests did not pass). Bounded variant 2b (one extra refresh + retry): 1 failed | 2b: api-client.contract `refresh don le (AC-4) > request 401 lan hai (sau khi da refresh) van het han: KHONG refresh lan 2` |
| 3 | drop single-flight reuse (api-client.ts) | `- if (!inflight) {` `+ if (true) {` | **KILLED**: 3 failed | api-client.contract `3 request cung 401 -> dung 1 lan refresh`, `refresh that bai khi dong thoi ... chi 1 lan refresh`; g1-lock-refresh `no navigator.locks: in-tab single-flight` |
| 4 | skip clearSession on refresh 400/401/403 (api-client.ts refreshCore) | `- if (epoch === ... ) clearSession();` | **KILLED**: 6 failed / 4 files | api-client.contract `refresh tra 400/401 -> xoa phien`; g1-lock-refresh `Z-3`; g4-involuntary-expiry; h2-user-switch ×2 |
| 5 | fake payment success on error (payment-client.ts initiatePayment) | `+ if (!response.ok) { return { paymentId: \`mock_pay_${Date.now()}\`, ... status: "PENDING" }; }` | **KILLED**: 8 failed / 4 files | payment-client.contract `HTTP 500/502/503 -> reject PaymentClientError`, `409 -> HOLD_EXPIRED, 422 -> INVALID_INPUT`; f7, g5-g6, payment-client.test |
| 6 | serve cached order on 404 (payment-client.ts getOrderDetails) | `+ const cached = getRecentOrder(orderId); if (cached) return cached;` | **KILLED**: 1 failed | payment-client.contract `404 -> reject NOT_FOUND, KHONG phuc vu cache sessionStorage nhu du lieu song` |
| 7 | per-identity-limit → QUANTITY_NOT_ALLOWED (hold-client.ts) | `- "PER_IDENTITY_LIMIT", 422);` `+ "QUANTITY_NOT_ALLOWED", 422);` | **KILLED**: 2 failed | hold-client.contract `422 /problems/per-identity-limit -> PER_IDENTITY_LIMIT`, `MUTATION: per-identity-limit KHONG duoc map ...` |

**Mutation score: 7/7 killed (threshold met).** Note on #2: the literal mutant is killed only by a crash, not by an assertion. The bounded variant 2b shows that a specific assertion also catches the missing guard.

## 5. Rollback (plan.md §5)

Scratch branch in the clean checkout: `git switch -c rollback-check 2970b7f && git merge --no-ff --no-edit 06fd7c4 && git revert -m 1 --no-edit HEAD` (merge 0b24098, revert 3738ba3).

| Check | Result |
|---|---|
| `git diff 2970b7f --stat` | empty (exit 0); `HEAD^{tree}` = `2970b7f^{tree}` = `e46a99bb10b0…` (identical tree, code and docs) |
| apps/web `npm ci` | exit 0 (521 packages) |
| apps/web `npm test` | **109 files, 1946 tests passed** (= baseline), exit 0 |
| apps/web `npm run build` (API base unset) | exit 0 |
| services/gateway `go test ./... -count=1` | exit 0 |
| services/event `go test ./... -count=1` | exit 0 |
| E2E against the reverted build (plan.md §5 last line) | not run; outside the requested step-5 command list |

The scratch branch `rollback-check` was deleted and the worktree removed. **Rollback: PASS.** Logs: `rollback-*.log`.

## 6. Open findings

- `findings.json` round 4 (`review_commit` 06fd7c4): `blocker: 0, major: 0`. All six items are `minor/deferred` (D-R4-1..4, D-W-4, D-W-5).
- `findings-round4-review.json`: verdicts H-1, H-2, H-3 and G-9 are `fixed`. New findings R4-1..R4-4 are all minor.
- `findings-round4-challenge.json`: W-1..W-5 are all minor (they map to the deferred D-* items).
- Bookkeeping caveat: the round-1 `items` in `findings.json` still say `status: "to_fix"` (9 major, 11 minor), and round-2 G-* / round-3 H-* also stay `to_fix`. They were never updated in place. The later review files close them: round 2 marked F-2..F-20 fixed and F-1 partial (became G-2); round 3 marked G-1..G-8 fixed; round 4 marked H-1..H-3 and G-9 fixed. **No blocker or major is open.** I recommend updating the status fields so the file does not read as open work.
- Follow-ups worth tracking (minor, deferred): W-1 cross-tab user switch keeps the previous user's buy-flow keys; W-3 a hung `/refresh` blocks all tabs until the holder closes; W-5 payment-client maps 403 forbidden to UNKNOWN.

## 7. Overall verdict

**PASS.**
- Gated commit matches (06fd7c4; 5077c7e is docs-only on top).
- All gates are green in a clean checkout: lint, typecheck, build, vitest 2342/2342, Go vet/test (+integration), E2E 38/38.
- Acceptance criteria: 29/29 DONE.
- Mutation testing: 7/7 killed.
- Rollback restores the exact 2970b7f tree, and its gates are green (1946 tests).
- Open findings: 0 blocker/major.

Observations, none blocking:
1. AC-16: the effective `s-maxage` on `/events/[slug]` is 10 s because of the availability fetch. plan.md accepted this.
2. The literal mutant #2 is killed only by a crash.
3. The round-1/2/3 statuses in `findings.json` were never updated.
4. Docker Desktop was started on this machine to run the AC-26 integration test. It is still running.
