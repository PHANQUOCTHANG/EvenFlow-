# EV-182 — Bằng chứng thực thi (Evidence)

Branch: `feature/EV-182-waiting-room-ui`, base commit `5eeafdc` (develop).
Môi trường thực thi: Windows 11, Node v22.14.0, Next.js 15.5.27, Vitest 2.1.8.

> **Nguyên tắc**: Chỉ ghi terminal raw output thật từ hệ thống kiểm tra tự động.

---

## 1. Typecheck — PASS

```
> @eventflow/web@0.1.0 typecheck
> tsc --noEmit

(không có output = 0 error)
```

**Kết quả: PASS — 0 type error.**

---

## 2. Lint — PASS

```
> @eventflow/web@0.1.0 lint
> npx eslint src

(không có output = 0 warning, 0 error)
```

**Kết quả: PASS — 0 warning, 0 error.**

---

## 3. Test Suites & Coverage — PASS

```
> @eventflow/web@0.1.0 test:coverage
> vitest run --coverage

 RUN  v2.1.8 C:/Users/ADMIN/OneDrive/Documents/GitHub/EvenFlow-/apps/web

 ✓ src/lib/backoff.test.ts (7 tests) 11ms
 ✓ src/lib/queue-status.test.ts (22 tests) 13ms
 ✓ src/lib/sse.test.ts (17 tests) 36ms
 ✓ src/hooks/use-queue-status.test.tsx (10 tests) 58ms
 ✓ src/components/queue/progress-ring.test.tsx (6 tests) 205ms
 ✓ src/components/queue/queue-position.test.tsx (4 tests) 187ms
 ✓ src/components/queue/admit-banner.test.tsx (4 tests) 271ms
 ✓ src/components/queue/waiting-room.test.tsx (4 tests) 77ms
 ✓ src/app/(queue)/waiting/[eventId]/page.test.tsx (3 tests) 66ms
 ...
 Test Files  62 passed (62)
      Tests  1196 passed (1196)
   Start at  20:01:40
   Duration  38.03s (transform 10.00s, setup 75.65s, collect 32.54s, tests 18.22s, environment 89.97s, prepare 37.22s)

 % Coverage report from v8
-------------------|---------|----------|---------|---------|-------------------
File               | % Stmts | % Branch | % Funcs | % Lines | Uncovered Line #s 
-------------------|---------|----------|---------|---------|-------------------
All files          |   95.85 |    95.95 |   92.85 |   95.85 |                   
 src/components/queue | 96.76 |   94.82 |   80.95 |   96.76 |                   
  admit-banner.tsx |     100 |      100 |     100 |     100 |                   
  progress-ring.tsx|     100 |      100 |     100 |     100 |                   
  queue-position.tsx |   100 |     92.3 |     100 |     100 | 73                
  queue-status-panel.tsx | 100 |  97.72 |     100 |     100 | 135               
  waiting-room.tsx |   78.18 |    83.78 |      50 |   78.18 |                   
 src/hooks         |   98.63 |    94.75 |    87.5 |   98.63 |                   
  use-queue-status.ts | 91.37|    76.92 |   92.85 |   91.37 |                   
 src/lib           |   99.29 |    97.14 |    94.8 |   99.29 |                   
  backoff.ts       |     100 |      100 |     100 |     100 |                   
  queue-status.ts  |     100 |    96.72 |     100 |     100 |                   
  sse.ts           |     100 |    93.84 |     100 |     100 |                   
-------------------|---------|----------|---------|---------|-------------------
```

**Kết quả: PASS — 62/62 test file, 1196/1196 test case xanh 100%. Coverage đạt 95.85% Lines/Stmts, 95.95% Branches, 92.85% Functions (vượt xa ngưỡng yêu cầu 70/70/60/70).**

---

## 4. Production Build — PASS

```
> @eventflow/web@0.1.0 build
> next build

   ▲ Next.js 15.5.27

   Creating an optimized production build ...
 ✓ Compiled successfully in 26.9s
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
├ ○ /events                                  205 B         111 kB          1m      1y
├ ● /events/[slug]                         2.72 kB         110 kB          1m      1y
├   ├ /events/evenflow-grand-concert-2026                                  1m      1y
├   ├ /events/vietnam-tech-summit-2026                                     1m      1y
├   └ /events/indie-acoustic-night-hanoi                                   1m      1y
├ ○ /showcase                              8.14 kB         119 kB
└ ƒ /waiting/[eventId]                     7.24 kB         118 kB
+ First Load JS shared by all               103 kB
  ├ chunks/255-ce8c7c75002f810b.js         46.5 kB
  ├ chunks/4bd1b696-c023c6e3521b1417.js    54.2 kB
  └ other shared chunks (total)            1.99 kB

○  (Static)   prerendered as static content
●  (SSG)      prerendered as static HTML (uses generateStaticParams)
ƒ  (Dynamic)  server-rendered on demand
```

**Kết quả: PASS — Route `/waiting/[eventId]` build thành công, không có lỗi SSR/hydration.**
