# EV-183 — Bằng chứng thực thi (Evidence)

Chạy lúc: 2026-10-10, branch `feature/EV-183-ticket-selection-hold-timer`.

## 1. Typecheck & Lint Gate

### A. Typecheck (`npm run typecheck`)
```text
> @eventflow/web@0.1.0 typecheck
> tsc --noEmit
```
Kết quả: **Exit code 0, không có lỗi kiểu dữ liệu.**

### B. Lint (`npm run lint`)
```text
> @eventflow/web@0.1.0 lint
> next lint

✔ No ESLint warnings or errors
```
Kết quả: **Exit code 0, 0 warning, 0 error.**

---

## 2. Test Suites & Coverage Gate

### A. Full Test Suite (`npm test`)
```text
 RUN  v2.1.8 C:/Users/ADMIN/OneDrive/Documents/GitHub/EvenFlow-/apps/web

 ✓ src/lib/server-time.test.ts (39 tests) 30ms
 ✓ src/lib/money.test.ts (24 tests) 17ms
 ✓ src/lib/queue-client.test.ts (11 tests) 115ms
 ✓ src/lib/queue-status.test.ts (22 tests) 25ms
 ✓ src/components/ui/button.test.tsx (32 tests) 487ms
 ✓ src/lib/readiness.test.ts (19 tests) 741ms
 ✓ src/components/ui/radio.test.tsx (18 tests) 421ms
 ✓ src/hooks/use-theme.test.tsx (13 tests) 81ms
 ✓ src/components/theme/theme-script.test.tsx (13 tests) 125ms
 ✓ src/components/ui/checkbox.test.tsx (20 tests) 397ms
 ✓ src/lib/hold-client.test.ts (9 tests) 18ms
 ✓ src/components/ui/field.test.tsx (16 tests) 189ms
 ✓ src/hooks/use-hold-timer.test.tsx (8 tests) 63ms
 ✓ src/components/layout/container.test.tsx (13 tests) 198ms
 ✓ src/components/ui/spinner.test.tsx (17 tests) 402ms
 ✓ src/components/checkout/checkout-view.test.tsx (6 tests) 398ms
 ✓ src/components/ui/select.test.tsx (14 tests) 404ms
 ✓ src/app/(ops)/layout.test.tsx (15 tests) 348ms
 ✓ src/app/(organizer)/layout.test.tsx (14 tests) 379ms
 ✓ src/components/ui/card.test.tsx (11 tests) 180ms
 ✓ src/components/layout/public-shell.test.tsx (12 tests) 285ms
 ✓ src/components/ui/alert.test.tsx (14 tests) 284ms
 ✓ src/app/(marketing)/layout.test.tsx (13 tests) 351ms
 ✓ src/components/layout/skip-link.test.tsx (11 tests) 257ms
 ✓ src/components/queue/waiting-room.test.tsx (4 tests) 166ms
 ✓ src/app/(queue)/layout.test.tsx (11 tests) 209ms
 ✓ src/app/(checkout)/layout.test.tsx (11 tests) 222ms
 ✓ src/components/theme/theme-toggle.test.tsx (7 tests) 415ms
 ✓ src/lib/theme.test.ts (11 tests) 13ms
 ✓ src/lib/jitter.test.ts (7 tests) 15ms
 ✓ src/components/ui/badge.test.tsx (15 tests) 88ms
 ✓ src/hooks/use-server-time-sync.test.tsx (3 tests) 232ms
 ✓ src/components/queue/progress-ring.test.tsx (6 tests) 176ms
 ✓ src/components/event/event-action-panel.test.tsx (6 tests) 257ms
 ✓ src/app/(checkout)/checkout/[eventId]/page.test.tsx (4 tests) 52ms
 ✓ src/components/queue/queue-position.test.tsx (4 tests) 218ms
 ✓ src/components/layout/public-footer.test.tsx (7 tests) 228ms
 ✓ src/lib/event-service.test.ts (5 tests) 8ms
 ✓ src/app/(marketing)/events/[slug]/page.test.tsx (5 tests) 213ms
 ✓ src/lib/backoff.test.ts (7 tests) 9ms
 ✓ src/app/(queue)/waiting/[eventId]/page.test.tsx (3 tests) 49ms
 ✓ src/components/event/ticket-tier-list.test.tsx (3 tests) 81ms
 ✓ src/lib/cn.test.ts (7 tests) 7ms
 ✓ src/app/api/time/route.test.ts (1 test) 6ms
 ✓ src/components/queue/admit-banner.test.tsx (4 tests) 127ms
 ✓ src/components/event/lobby-notice.test.tsx (1 test) 64ms
 ✓ src/app/(marketing)/events/page.test.tsx (1 test) 86ms
 ✓ src/app/api/healthz/route.test.ts (13 tests) 339ms
 ✓ src/app/api/readyz/route.test.ts (19 tests) 339ms
 ✓ src/instrumentation.test.ts (19 tests) 1058ms
 ✓ src/app/(marketing)/page.test.tsx (1 test) 59ms

 Test Files  66 passed (66)
      Tests  1223 passed (1223)
```
Kết quả: **100% tests PASS (1223/1223 tests, 66 test files).**

### B. Coverage (`npm run test:coverage`)
```text
File                 | % Stmts | % Branch | % Funcs | % Lines | Uncovered Line #s 
---------------------|---------|----------|---------|---------|-------------------
 src/hooks           |   98.13 |    93.23 |   88.23 |   98.13 | 
  use-hold-timer.ts  |   90.65 |    69.23 |     100 |   90.65 |
 src/lib             |   98.16 |    95.49 |   95.34 |   98.16 |
  hold-client.ts     |   78.94 |     62.5 |     100 |   78.94 |
 src/components/ui   |   99.87 |    97.28 |   93.33 |   99.87 |
```
Kết quả: **Đạt ngưỡng Lines/Statements > 70%, Branches > 60%.**

---

## 3. Build Gate (`npm run build`)
```text
> @eventflow/web@0.1.0 build
> next build

   ▲ Next.js 15.5.27

   Creating an optimized production build ...
 ✓ Compiled successfully in 41s
   Linting and checking validity of types ...
   Collecting page data ...
   Generating static pages (0/9) ...
   Generating static pages (9/9)
   Finalizing page optimization ...

Route (app)                                   Size  First Load JS
├ ƒ /checkout/[eventId]                    3.39 kB         117 kB
```
Kết quả: **Build Next.js thành công 100%, route `/checkout/[eventId]` được biên dịch chuẩn xác.**
