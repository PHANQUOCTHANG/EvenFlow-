# EVF-1806 — Bằng chứng thực thi (Evidence)

| | |
|---|---|
| Chụp lúc | 2026-10-12 14:53:24 → 14:58 +07:00 |
| Nhánh | `feature/EVF-1806-assistant-panel` |
| HEAD | `5f2ec729190f116d790af7acc7668249aeba7792` (merge `origin/develop` `ad09bf3` vào nhánh) |
| Working tree | sạch trừ `docs/workflow/EVF-1806-assistant-panel/HANDOFF.md` (untracked) và 3 file tài liệu verify này |
| Môi trường | Windows 11 Pro, Node v22.21.0, npm 10.9.4, Next.js 15.5.27, Vitest 2.1.8 (jsdom), `@vitest/coverage-v8` |
| Người chạy | agent verify độc lập, chỉ đọc; không sửa source/test, không commit, không push |

Tất cả lệnh chạy trong `apps/web`, tuần tự, lưu raw output ở scratchpad của phiên verify.

## 1. Tóm tắt gate

| Gate | Lệnh | Exit | Kết quả |
|---|---|---|---|
| Lint | `npm run lint` | 0 | 0 error, 21 warning — **tất cả** trong `src/components/auth/*` (code của develop, nhánh không đụng) |
| Typecheck | `npm run typecheck` (`tsc --noEmit`) | 0 | sạch |
| Test tập trung | `npx vitest run src/components/assistant/assistant-panel.test.tsx src/components/showcase` | 0 | 2 file, **187/187** (179 + 8), 4.86s, 0 dòng stderr |
| Test toàn bộ | `npm test` (`vitest run`) | 0 | **93 file, 1532/1532**, 25.85s |
| Build | `npm run build` (`next build`) | 0 | Compiled successfully, 16/16 trang tĩnh, `/showcase` 10.5 kB / 128 kB First Load |
| Coverage (tuỳ chọn) | xem §4 | 0 | `assistant-panel.tsx`: 100% stmt / **99% branch** / 100% func / **100% line** |

So với baseline (`baseline.md`, `ad09bf3` đo thật): **91 → 93 file (+2), 1345 → 1532 test (+187)**, 0 fail, 0 hồi quy.

## 2. Output rút gọn

### 2.1 Lint

```text
> @eventflow/web@0.1.0 lint
> next lint
 ⚠ Warning: Next.js inferred your workspace root ... Detected additional lockfiles: ... apps\web\package-lock.json

./src/components/auth/ForgotPasswordForm.tsx   (4 warning: no-unused-vars, no-explicit-any x2, no-img-element)
./src/components/auth/LoginForm.tsx            (6 warning: no-unused-vars x4, no-explicit-any x2)
./src/components/auth/OTPVerification.tsx      (3 warning: no-unused-vars, no-explicit-any x2)
./src/components/auth/RegisterForm.tsx         (8 warning: no-unused-vars, no-explicit-any x6, no-img-element)
EXIT=0
```

Không có warning nào trong `components/assistant/` hay `components/showcase/`.

### 2.2 Typecheck

```text
> @eventflow/web@0.1.0 typecheck
> tsc --noEmit
EXIT=0
```

`tsconfig.json` include `**/*.tsx` nên file test cũng được typecheck — đây là nơi chốt type-level AC-1
(`@ts-expect-error` khi truyền `rank`, `NO_LEAKED_KEYS`). Đã thử ngược: thêm `rank?: number` vào
`AssistantPanelProps` trong worktree tạm → `tsc` exit 2:

```text
assistant-panel.test.tsx(192,7): error TS2322: Type 'boolean' is not assignable to type 'never'.
assistant-panel.test.tsx(196,3): error TS2578: Unused '@ts-expect-error' directive.
```

### 2.3 Test tập trung

```text
 ✓ src/components/showcase/domain-gallery.test.tsx (8 tests) 1239ms
 ✓ src/components/assistant/assistant-panel.test.tsx (179 tests) 1493ms
 Test Files  2 passed (2)
      Tests  187 passed (187)
   Duration  4.86s
EXIT=0
```

### 2.4 Test toàn bộ

```text
 RUN  v2.1.8 D:/Documents/Nam4-1/Project1/EvenFlow-/apps/web
 ...
 Test Files  93 passed (93)
      Tests  1532 passed (1532)
   Start at  14:53:56
   Duration  25.85s
EXIT=0
```

### 2.5 Build

```text
   ▲ Next.js 15.5.27
 ✓ Compiled successfully in 5.4s
   Linting and checking validity of types ...
 ✓ Generating static pages (16/16)
Route (app)                                       Size  First Load JS
├ ○ /showcase                                  10.5 kB         128 kB
└ ƒ /waiting/[eventId]                          6.6 kB         125 kB
+ First Load JS shared by all                   103 kB
EXIT=0
```

## 3. Nhiễu đã biết (không chặn)

| Nhiễu | Nguồn | Có liên quan EVF-1806? |
|---|---|---|
| 21 lint warning (`no-explicit-any`, `no-unused-vars`, `no-img-element`) | `src/components/auth/*` — code của develop | Không. Nhánh không sửa file nào trong `auth/` |
| `Error: Not implemented: navigation (except hash changes)` (stderr jsdom) | `layout/workspace-shell.test.tsx`, `layout/public-header.test.tsx` — develop | Không |
| `Warning: An update to … was not wrapped in act(...)` | `hooks/use-hold-timer.test.tsx`, `organizer/event-wizard.test.tsx` — develop | Không |
| `console.warn` của `toEpochMs` (chuỗi không có múi giờ) | `server-time`, `use-server-countdown`, `server-expiry-countdown`, `event-card`, `queue-status-panel` tests | Không. Test của AP đã spy + assert `console.warn` nên **AP/DG không in dòng stderr nào** (grep stderr trên 2 file = 0) |
| `next lint` deprecated / workspace root inferred (nhiều lockfile) | cấu hình repo | Không |

## 4. Coverage

```bash
npx vitest run --coverage \
  --coverage.include=src/components/assistant/assistant-panel.tsx \
  --coverage.include=src/components/showcase/domain-gallery.tsx \
  --coverage.thresholds.{lines,functions,branches,statements}=0 --coverage.reporter=text \
  src/components/assistant/assistant-panel.test.tsx src/components/showcase
```

```text
-------------------|---------|----------|---------|---------|-------------------
File               | % Stmts | % Branch | % Funcs | % Lines | Uncovered Line #s
-------------------|---------|----------|---------|---------|-------------------
  ...ant-panel.tsx |     100 |       99 |     100 |     100 | 389
  ...n-gallery.tsx |     100 |      100 |      75 |     100 |
-------------------|---------|----------|---------|---------|-------------------
```

- Nhánh chưa phủ duy nhất `:389` là `KIND_LABEL[kind ?? "ai"]` — vế `?? "ai"` không thể chạm vì `kind` của tin
  trợ lý đã được chuẩn hoá ở `:362-366`. Không phải thiếu test.
- `domain-gallery.tsx` 75% function: `noop` handler của demo không bao giờ được gọi trong test (đúng ý đồ).
- Vượt ngưỡng repo (lines/functions/statements ≥ 70%, branches ≥ 60%).

## 5. Kiểm "test có bắt lỗi thật không" — mutation probe

Chạy trong worktree tạm của HEAD (`git worktree add --detach … HEAD`, junction `node_modules`), mỗi mutant
thay một đoạn trong `assistant-panel.tsx`, chạy `npx vitest run src/components/assistant/assistant-panel.test.tsx`,
rồi khôi phục. Worktree đã gỡ; working tree chính không đổi.

| Mutant | Hành vi bị phá | Kết quả |
|---|---|---|
| M1 | `retryAllowed = true` | KILLED (18 đỏ) |
| M1b | chỉ bỏ `disabled` của nút Gửi lại (F-1/C-2) | KILLED (4) |
| M2 | Enter bỏ qua `isComposing` (F-2) | KILLED (2) |
| M3 | Enter bỏ qua `keyCode 229` | KILLED (1) |
| M4 | bỏ chặn Date ngoài miền (C-1) | KILLED (3) |
| M5 | không lọc `supportHref` (C-7) | KILLED (4) |
| **M6** | **Escape bỏ qua `defaultPrevented` (AC-9b)** | **SURVIVED (179/179 xanh)** |
| M7 | không lọc ký tự không bề rộng (C-11) | KILLED (1) |
| M8 | `quotaRemaining === 0` thay `<= 0` (C-5) | KILLED (1) |
| M9 / M9b | bỏ `quotaLimit > 0` / bỏ `remaining ≤ limit` (C-6) | KILLED (1) / KILLED (1) |
| M10 | bỏ qua tin pending (AC-4) | KILLED (8) |
| M11 | chặn FAQ ngoài NORMAL (AC-6) | KILLED (4) |
| M12 | không chuẩn hoá `kind` lạ (C-10) | KILLED (1) |
| M13 | luôn hiện gợi ý "Enter để gửi" (F-4) | KILLED (2) |
| M14 | chèn "Gemini 429" vào copy FAQ_ONLY (§2.5, AC gốc Jira) | KILLED (2) |
| M15 | gỡ `onKeyDown` khỏi gốc panel | KILLED (2) |
| M16 | bỏ `trim()` khi gửi | KILLED (4) |
| M17 | render `text` bằng `dangerouslySetInnerHTML` (BR-A7) | KILLED (2) |
| M18 | cho câu hỏi mở ở SAVING | KILLED (4) |

**20/21 KILLED.** Lỗ hổng duy nhất: nhánh `defaultPrevented` của AC-9b không có test (xem `traceability.md`).

## 6. Phạm vi diff (so với `origin/develop`)

`git diff --stat origin/develop HEAD`:

```text
apps/web/src/components/assistant/assistant-panel.test.tsx  | 1697 +
apps/web/src/components/assistant/assistant-panel.tsx       |  421 +
apps/web/src/components/assistant/index.ts                  |    9 +
apps/web/src/components/showcase/domain-gallery.test.tsx    |   78 +
apps/web/src/components/showcase/domain-gallery.tsx         |  113 +
apps/web/vitest.config.ts                                   |    4 +-
docs/workflow/EVF-1806-assistant-panel/{challenge,code-review,spec-plan}.md
```

- **Ngoài danh sách của spec §5 bước 3:** `apps/web/vitest.config.ts` (thêm `src/**/*.{test,spec}.{ts,tsx}` vào
  `coverage.exclude`). Đến từ commit `a8fb8c9` ("chore(web): exclude test files from coverage measurement"),
  vốn là develop cục bộ cũ và **không có trong `origin/develop`**, nên đi kèm PR này. Thay đổi vô hại (chỉ ảnh hưởng
  cách đo coverage, làm số coverage trung thực hơn), nhưng người review PR cần biết; hoặc tách thành PR riêng.
- `domain-gallery.tsx`: chỉ thêm import, hằng demo, một `<Section title="Trợ lý AI">`; không sửa section cũ.

## 7. Cùng tồn tại với `ChatPanel` (EV-185, PR #38)

- `src/components/assistant/` hiện có **hai** component trợ lý: `ChatPanel` (+ `FaqChips`, `StreamingMessage`) của
  EV-185 và `AssistantPanel` của EVF-1806. Ngoài phạm vi verify này, nhưng ghi nhận:
  - `index.ts` re-export cả hai (`export *` cho 3 file EV-185 + export có tên cho `AssistantPanel` và các type).
    Không trùng tên export (EV-185 export `ChatPanel`, `ChatPanelProps`, `FaqChips`, `FaqChipsProps`,
    `StreamingMessage`, `StreamingMessageProps`); typecheck + build xanh.
  - `waiting-room.tsx` dùng `ChatPanel` (import trực tiếp `@/components/assistant/chat-panel`); `AssistantPanel`
    hiện chỉ được dùng ở `/showcase`.
  - Test EV-185 (`chat-panel` 8, `faq-chips` 4, `streaming-message` 4) vẫn xanh trong lần chạy 1532/1532.
- Hai component trùng vai trò và khác hợp đồng (EV-185 có "streaming", `AssistantPanel` cố ý không giả streaming,
  §1 spec). Cần quyết định ở cấp story/PO component nào là chuẩn cho phòng chờ — **chưa phải việc của EVF-1806**.

## 8. Kết luận verify

**PASS** — mọi gate xanh (lint 0 error, typecheck, 1532/1532, build), AC-1..AC-11 đều có test thực chất, các
finding major (F-1/C-2, F-2, C-1) đều có test hồi quy bị mutant giết. Một lỗ hổng nhỏ không chặn: nhánh
`defaultPrevented` của AC-9b chưa có test.

## 7. Bổ sung sau verify (2026-10-12)

Verifier chỉ ra nhánh `!event.defaultPrevented` (`assistant-panel.tsx:204`, AC-9b) không có test (mutant M6 SURVIVED).
Đã thêm test `Escape da bi preventDefault o phan tu con -> KHONG dong panel (AC-9b)`: gắn listener native
`keydown` trên textarea gọi `preventDefault()` rồi phát Escape, assert `onClose` không được gọi.

| Lần chạy | Lệnh | Kết quả |
|---|---|---|
| Code thật | `npx vitest run src/components/assistant/assistant-panel.test.tsx` | **180/180** pass |
| Mutant M6 (xoá `&& !event.defaultPrevented`) | cùng lệnh | **1 failed / 179 passed** — đúng test mới đỏ; code được khôi phục bằng `git checkout` |
| Toàn bộ | `npx vitest run --reporter=json` | **93 file, 1533/1533**, 0 fail |

Số ở các mục trên (179, 187, 1532) là của lần verify; sau bổ sung: AP 180, tập trung 188, toàn bộ 1533 (baseline 1345 → +188).
Mutation: **21/21 KILLED**.
