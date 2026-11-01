# Test change request (ops-warroom)

Existing tests that assert sample-data or server-side-fetch behaviour and will legitimately break (or must be replaced)
when the feature lands. Nothing here was edited by the test designer; the implementer replaces them only per this list.
New behaviour is covered by the new frozen tests (see `traceability.yaml`).

| Existing test file | Why it breaks | Replaced by |
|---|---|---|
| `apps/web/src/lib/ops-service.test.ts` | imports `SAMPLE_EVENTS`, `__setOpsSampleOverride`; asserts `isSample === true`, deterministic sample numbers | `lib/ops-service.contract.test.ts` (keep the pure-helper tests: `tierInventoryViolation`, `snapshotAgeMs`, constants, `saleStateLabel`) |
| `apps/web/src/components/ops/ops-war-room.test.tsx` | sample label expected on all snapshots, disabled EVF-60 buttons + `aria-describedby` note (m-9), `__setOpsSampleOverride` for errors, snapshots lack the new `admission`/`checkout` fields | `components/ops/ops-war-room.real.test.tsx` (keep polling/stale/403/404/inventory-violation cases, updated for the new snapshot type) |
| `apps/web/src/hooks/use-ops-war-room.test.tsx` | only if it imports sample helpers or the old snapshot shape | same file, fixture update only |
| `apps/web/src/app/(ops)/ops/events/page.test.tsx` | calls the server page and expects links from the sample list; `dynamic = "force-dynamic"` may go | `app/(ops)/ops-pages.real.test.tsx` |
| `apps/web/src/app/(ops)/ops/events/[eventId]/page.test.tsx` | expects the server page to fetch the snapshot / call `notFound()` | `app/(ops)/ops-pages.real.test.tsx` (client-side fetch, no SSR) |
| `apps/web/src/app/(ops)/layout.test.tsx` | renders `OpsLayout` without a session; with `RoleGuard` the children are hidden until `useAuth` reports `ops` | same file: mock `useAuth` as `ops` (role cases live in `ops-pages.real.test.tsx`) |
| `apps/web/src/lib/organizer-service.test.ts` (getRealtimeSales part) | asserts the deterministic sample snapshot | `lib/organizer-service.sales.contract.test.ts` |
| `apps/web/src/hooks/use-organizer-sales.test.tsx` | fixtures typed `isSample: true`, `paymentBreakdown` as an object | fixture/type update only; 4 s polling unchanged |
| `apps/web/src/components/organizer/sales-dashboard.test.tsx` | asserts the "Dữ liệu mẫu" alert/badge, numeric peak TPS, vnpay/momo/card cards, default autoRefresh off | `components/organizer/sales-dashboard.real.test.tsx` |
| `apps/web/src/app/(organizer)/organizer/events/[eventId]/sales/page.test.tsx` | metadata text "Doanh số mẫu ..." | same file, metadata text only |
| `apps/web/src/real-api.contract.test.ts` | test "lib/sample-data/events.ts ton tai va chi duoc import boi ops-service" | `ops-warroom.no-sample.contract.test.ts` (file deleted, nothing imports sample data) |
| `tests/e2e/specs/ops-war-room.spec.ts` | built on sample data and disabled controls | rewritten per AC-31 (mocked API, override -> pause -> resume) |
| `tests/e2e/specs/i18n.spec.ts` | `/ops/events` now needs an ops session mock | add the ops login mock |

Go / Python: no existing test is expected to change. The existing tests in
`services/waitingroom/internal/adapter/http` and `services/ticketing/internal/adapter/http` do not compile until the new
symbols exist (the new `*_test.go` files in those directories reference them). That is the red state, not a change to
those tests.

## New request 1 (implementer, 2026-11-01) — APPROVED by the human 2026-11-01, applied

| Existing test file | Why it must change | Proposed change |
|---|---|---|
| `services/ai-worker/tests/test_ops_api.py` (frozen, line 28) | `uv run ruff check .` (ruff 0.16.10, the version pinned in `pyproject.toml` and used by CI `ci.yml:170`) fails with `FURB162 Unnecessary timezone replacement with zero offset` on `datetime.fromisoformat(body["generated_at"].replace("Z", "+00:00"))`. The test itself passes; only the lint gate fails | lint-only: `datetime.fromisoformat(body["generated_at"])` (Python 3.12 parses a trailing `Z`; the endpoint returns `+00:00` anyway). Assertion strength unchanged. Not applied by the implementer: needs approval |

## New request 2 (implementer, 2026-11-01) — APPROVED by the human 2026-11-01, applied

| Existing test file | Why it must change | Proposed change |
|---|---|---|
| `services/waitingroom/internal/adapter/http/ops_handler_test.go` (frozen), `TestOpsPut_AC6_BoundariesAccepted` | the test expects `rate_per_sec: 0.5` to be accepted (200). Contracts §3.2 as amended by the **approved** CR-1 (item 5), spec AC-6/AC-9 ("integer only", "non-integer → 400") and the frozen web tests (`ops-service.contract.test.ts:309` rejects `1.5`; `ops-war-room.real.test.tsx:145`) all require a fractional rate to be rejected with 400. The implementation follows the approved contract, so this one subtest (`rate 0.5: 400`) fails. `1e2` (= 100, integral) is accepted | move `"0.5"` out of the accepted list (keep `"0"`, `"10000"`, `"1e2"`) and add `"fraction": {"rate_per_sec":0.5,...}` to `TestOpsWrite_AC9_InvalidInputIs400AndNothingApplied`. Strengthens the test; needs approval. Alternative if the human prefers fractions: a new contracts change request reverting CR-1 item 5 (then web tests change too) |

## New request 3 (implementer, 2026-11-01) — APPROVED by the human 2026-11-01, applied, blocks CI integration for ticketing

| Existing test file | Why it must change | Proposed change |
|---|---|---|
| `services/ticketing/internal/adapter/postgres/ops_repository_integration_test.go` (frozen, line 24) | `go vet -tags integration ./...` (and therefore CI `go test -tags=integration ./...`, `integration.yml:35-37`) fails to compile the whole `postgresadapter_test` package: `"…/internal/adapter/postgres" imported as postgresadapter and not used` (the file uses `setupPostgresTest` from `repository_integration_test.go` and never names the package). Every existing ticketing Postgres integration test in that package is blocked with it. Verified with a `go vet -overlay` copy that drops only that import line: no other error (`OVERLAY_VET_OK`) | delete the unused import line `postgresadapter "github.com/eventflow/eventflow/services/ticketing/internal/adapter/postgres"`. No assertion changes. Not applied by the implementer: needs approval |
