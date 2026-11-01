# Test change request — applied (ops-warroom, implementer)

Every entry below is a row of `test-change-request.md` (approved list). Nothing was skipped; no `it.skip`/`.only` was
added. Counts are `it(` / `it.each(` declarations before (HEAD f5c6d44) → after.

| File | Change | Cases |
|---|---|---|
| `apps/web/src/lib/ops-service.test.ts` | Sample-data suites removed as the TCR says (SAMPLE_EVENTS list, sample snapshot shape, `isSample = true`, deterministic sample table, `__setOpsSampleOverride` patch/error/no-op, sample 404). Covered by the frozen `lib/ops-service.contract.test.ts` (real API). Kept unchanged: constants (5 s / 2 s), `tierInventoryViolation` (BR-O1 + INVALID_DATA), `snapshotAgeMs`. Added `saleStateLabel` cases (the TCR says to keep them; the old file had none) | 44 → 17 |
| `apps/web/src/components/ops/ops-war-room.test.tsx` | Fixture gets the new `admission` fields + `checkout`, default `isSample: false`; tests that assert the sample label now pass `isSample: true` explicitly; `realOpsError` builds `new OpsServiceError(status)` (no more `__setOpsSampleOverride`). Replaced: "buttons disabled + EVF-60 note" → "buttons enabled, note gone"; m-9 "aria-describedby → EVF-60" → "disabled (waitingroom unavailable) buttons have aria-describedby → the visible lock reason". Polling / stale / 403 / 404 / inventory-violation cases unchanged | 62 → 62 |
| `apps/web/src/hooks/use-ops-war-room.test.tsx` | Fixture update only (new snapshot fields, `isSample: false`); `realOpsError` uses the `OpsServiceError` constructor instead of `__setOpsSampleOverride` | 34 → 34 |
| `apps/web/src/app/(ops)/ops/events/page.test.tsx` | Page is now a client list: `listOpsEvents` mocked; link / link-count / heading / force-dynamic / OPS_NAV cases kept on the client render | 5 → 5 |
| `apps/web/src/app/(ops)/ops/events/[eventId]/page.test.tsx` | Page is a thin shell (no server fetch): known event / 404 / 403 / other error now asserted on the client hook path (404 → `OpsNotFoundView`, `notFound()` never called); m-7 cases → "no initialError/initialSnapshot from the server" and "client error shows only the fixed OpsServiceError message" | 7 → 7 |
| `apps/web/src/app/(ops)/layout.test.tsx` | `useAuth` mocked as `ops` (as the TCR says) and `useRouter` added to the `next/navigation` mock, because `RoleGuard` calls it. Assertions unchanged | 15 → 15 |
| `apps/web/src/lib/organizer-service.test.ts` | getRealtimeSales case: sample snapshot → real endpoint (path, `isSample: false`, figures, AOV, breakdown list) | 22 → 22 |
| `apps/web/src/hooks/use-organizer-sales.test.tsx` | Fixture/type update only (`isSample: false`, ISO timestamp, `postSaleReport` with breakdown list) | 6 → 6 |
| `apps/web/src/components/organizer/sales-dashboard.test.tsx` | Sample label case → "no sample label/alert"; TPS → "Chưa có dữ liệu"; provider cards → list from data (vnpay/momo/card); toggle starts ON. `getRealtimeSales` mocked to a pending promise so the test does no network call | 5 → 5 |
| `apps/web/src/app/(organizer)/organizer/events/[eventId]/sales/page.test.tsx` | **Note:** the TCR row says "metadata text only", but the file has no metadata assertion; it asserts the dashboard sample labels (lines 81-82). Those two assertions became "no `Dữ liệu mẫu`" + "badge `Realtime (≤ 5s)`", and the fixture got `isSample: false` + `postSaleReport` (type). Flagged for reviewer confirmation | 2 → 2 |
| `apps/web/src/real-api.contract.test.ts` | "sample-data/events.ts exists and only ops-service imports it" → "file deleted and no production module imports sample-data" | 9 → 9 |
| `tests/e2e/specs/ops-war-room.spec.ts` | Rewritten per AC-31 (see WP4 Playwright commit) | see commit |
| `tests/e2e/specs/i18n.spec.ts` | Ops login mock added for `/ops/events` (see WP4 Playwright commit) | see commit |

## Requests NOT applied (pending, see `test-change-request.md` "New request 1..3")

1. `services/ai-worker/tests/test_ops_api.py:28` — ruff FURB162 (lint only).
2. `services/waitingroom/internal/adapter/http/ops_handler_test.go` `TestOpsPut_AC6_BoundariesAccepted` expects `0.5` accepted; contradicts approved CR-1 item 5. Fails one subtest.
3. `services/ticketing/internal/adapter/postgres/ops_repository_integration_test.go:24` — unused import, breaks compilation of the ticketing Postgres integration test package.
