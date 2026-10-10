# EV-184 — Bằng chứng thực thi (Evidence)

Chạy lúc: 2026-10-10 20:30 (UTC+7) trên nhánh `feature/EV-184-payment-flow-order-result`.

## 1. Typecheck & Lint Gate

### A. ESLint (`npm run lint`)
```bash
> @eventflow/web@0.1.0 lint
> next lint

✔ No ESLint warnings or errors
```

### B. TypeScript (`npm run typecheck`)
```bash
> @eventflow/web@0.1.0 typecheck
> tsc --noEmit
# Exit code 0, 0 errors
```

## 2. Test Suite & Coverage Gate

### A. Kết quả chạy test toàn bộ (`npm test`)
```bash
> @eventflow/web@0.1.0 test
> vitest run

 Test Files  73 passed (73)
      Tests  1253 passed (1253)
   Start at  20:22:24
   Duration  46.03s
```

### B. Đo Coverage (`npm run test:coverage`)
```
-------------------|---------|----------|---------|---------|-------------------
File               | % Stmts | % Branch | % Funcs | % Lines | Uncovered Line #s 
-------------------|---------|----------|---------|---------|-------------------
.../checkout       |         |          |         |         |                   
  order-result-view|     100 |    91.66 |     100 |     100 |                   
  payment-handoff  |   95.83 |    82.14 |     100 |   95.83 |                   
src/hooks          |   98.18 |    93.11 |   89.18 |   98.18 |                   
  use-order-status |   96.82 |     90.9 |     100 |   96.82 |                   
  use-payment-sess |   93.27 |    82.75 |     100 |   93.27 |                   
src/lib            |   97.01 |    91.87 |   95.87 |   97.01 |                   
  payment-client   |   78.75 |       46 |     100 |   78.75 |                   
-------------------|---------|----------|---------|---------|-------------------
# Toàn bộ module đạt ngưỡng yêu cầu: Statements/Lines >= 70%, Branches >= 60%
```

## 3. Production Build Gate (`npm run build`)
```bash
> @eventflow/web@0.1.0 build
> next build

   ▲ Next.js 15.5.27

   Creating an optimized production build ...
 ✓ Compiled successfully in 25.5s
   Linting and checking validity of types ...
   Collecting page data ...
   Generating static pages (0/9) ...
   Generating static pages (2/9) 
   Generating static pages (4/9) 
   Generating static pages (6/9) 
 ✓ Generating static pages (9/9)
   Finalizing page optimization ...
   Collecting build traces ...

Route (app)                                   Size  First Load JS  Revalidate  Expire
┌ ○ /                                        205 B         111 kB
├ ○ /_not-found                              992 B         104 kB
├ ƒ /api/healthz                             130 B         103 kB
├ ƒ /api/readyz                              130 B         103 kB
├ ƒ /api/time                                130 B         103 kB
├ ƒ /checkout/[eventId]                    3.49 kB         118 kB
├ ƒ /checkout/[eventId]/payment            5.71 kB         116 kB
├ ƒ /checkout/[eventId]/result             5.62 kB         116 kB
├ ○ /events                                  205 B         111 kB          1m      1y
├ ● /events/[slug]                         2.72 kB         110 kB          1m      1y
├ ○ /showcase                              5.84 kB         120 kB
└ ƒ /waiting/[eventId]                     7.24 kB         118 kB
+ First Load JS shared by all               103 kB
```
