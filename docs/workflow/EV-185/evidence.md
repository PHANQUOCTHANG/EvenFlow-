# EV-185 — Bằng chứng thực thi (Evidence)

Chụp lúc: 2026-10-10 22:38:53 +07:00 trên nhánh `feature/EV-185-assistant-chat-panel`.

## 1. Typecheck & Lint Gate

### 1.1 Typecheck (`tsc --noEmit`)
```text
> @eventflow/web@0.1.0 typecheck
> tsc --noEmit

(exited with code 0)
```

### 1.2 Lint (`npm run lint`)
```text
> @eventflow/web@0.1.0 lint
> next lint

`next lint` is deprecated and will be removed in Next.js 16.
For new projects, use create-next-app to choose your preferred linter.
For existing projects, migrate to the ESLint CLI:
npx @next/codemod@canary next-lint-to-eslint-cli .

✔ No ESLint warnings or errors
```

---

## 2. Test Suites & Coverage

### 2.1 Test Suites (`vitest run`)
```text
 RUN  v2.1.8 C:/Users/ADMIN/OneDrive/Documents/GitHub/EvenFlow-/apps/web

 ✓ src/hooks/use-assistant.test.tsx (7 tests) 47ms
 ✓ src/components/assistant/streaming-message.test.tsx (4 tests) 102ms
 ✓ src/components/assistant/faq-chips.test.tsx (4 tests) 136ms
 ✓ src/lib/assistant-client.test.ts (13 tests) 877ms
 ✓ src/components/assistant/chat-panel.test.tsx (8 tests) 185ms
 ✓ src/components/queue/waiting-room.test.tsx (5 tests) 153ms
 ... [toàn bộ 78 files test]

 Test Files  78 passed (78)
      Tests  1290 passed (1290)
   Start at  22:35:05
   Duration  45.22s
```
*So với Baseline:*
- Test files: **73 → 78 (+5 test files mới)**
- Tests passed: **1253 → 1290 (+37 tests)**
- 0 failed, 0 hồi quy.

### 2.2 Coverage Summary
```text
-------------------|---------|----------|---------|---------|-------------------
File               | % Stmts | % Branch | % Funcs | % Lines | Uncovered Line #s 
-------------------|---------|----------|---------|---------|-------------------
src/components/assistant
  faq-chips.tsx    |     100 |      100 |     100 |     100 |
  streaming-message|     100 |      100 |     100 |     100 |
  chat-panel.tsx   |   98.41 |    92.85 |     100 |   98.41 |
src/hooks
  use-assistant.ts |   83.25 |    87.87 |     100 |   83.25 |
src/lib
  assistant-client |   85.98 |    79.36 |     100 |   85.98 |
-------------------|---------|----------|---------|---------|-------------------
Toàn bộ vượt ngưỡng yêu cầu: Lines >= 70%, Functions >= 70%, Branches >= 60%.
```

---

## 3. Build Gate (`npm run build`)

```text
> @eventflow/web@0.1.0 build
> next build

   ▲ Next.js 15.5.27

   Creating an optimized production build ...
 ✓ Compiled successfully in 32.3s
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
├ ƒ /checkout/[eventId]/payment            5.73 kB         116 kB
├ ƒ /checkout/[eventId]/result             5.65 kB         116 kB
├ ○ /events                                  205 B         111 kB          1m      1y
├ ● /events/[slug]                         2.77 kB         110 kB          1m      1y
├   ├ /events/evenflow-grand-concert-2026                                  1m      1y
├   ├ /events/vietnam-tech-summit-2026                                     1m      1y
├   └ /events/indie-acoustic-night-hanoi                                   1m      1y
├ ○ /showcase                              5.84 kB         120 kB
└ ƒ /waiting/[eventId]                     12.7 kB         123 kB
+ First Load JS shared by all               103 kB

○  (Static)   prerendered as static content
●  (SSG)      prerendered as static HTML (uses generateStaticParams)
ƒ  (Dynamic)  server-rendered on demand
```
