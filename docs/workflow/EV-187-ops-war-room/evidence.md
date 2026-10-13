# EV-187 — Bằng chứng xác minh (Verify)

| Mục | Giá trị |
|---|---|
| Người xác minh | Agent verify độc lập, chỉ đọc (không viết code/test của slice này) |
| Thời điểm | 2026-10-13 (giờ máy, khoảng 03:30 – 03:45) |
| Worktree | `D:/Documents/Nam4-1/Project1/EvenFlow-187` |
| Branch | `feature/EV-187-ops-war-room` |
| HEAD | `73a7bcab8400698b5a201893a485ff8da9e2a8fd` |
| Base xếp chồng | Nhánh xếp trên `origin/feature/EV-198-e2e-playwright` (PR #50, **chưa merge**), gốc `develop` @ `51c4729`. Commit của EV-187 = `git log origin/feature/EV-198-e2e-playwright..HEAD` (8 commit: `a5679a3`, `c5adf54`, `d4f8d51`, `5774d45`, `4c2c82a`, `3f623ed`, `e177eeb`, `73a7bca`). Khi PR #50 merge, cần rebase EV-187 lên `develop` và chạy lại các cổng dưới đây. |
| Trạng thái cây lúc bắt đầu | `git status` sạch |

## Kết luận: **PASS**

Mọi cổng xanh; 11/11 AC có bằng chứng chạy được; mọi finding của code review (M-1, m-1..m-10, t-1..t-3, t-5) và challenge
(C-1, C-3..C-6) đã sửa trong code, C-2 ghi nợ trong `traceability.md`; diff nằm trong danh sách file được phép.
Không có gap chặn merge. Lưu ý duy nhất: lỗi "window is not defined" thỉnh thoảng xuất hiện ở `chat-panel.test.tsx`
(EV-185), không liên quan EV-187 (xem §1.3).

## 1. Cổng `apps/web`

| Lệnh (trong `apps/web`) | Kết quả |
|---|---|
| `npm run lint` | exit 0. Chỉ có warning có sẵn ở file ngoài phạm vi (`components/auth/*.tsx`). |
| `npx eslint <các file EV-187> --max-warnings=0` (ops-service, format, hook, `components/ops`, `app/(ops)/ops`, `nav-config.ts`) | exit 0, **0 warning** |
| `npm run typecheck` (`tsc --noEmit`) | exit 0 |
| `npx vitest run` (lần 1) | **101 file / 1767 test pass**, 0 fail; exit 1 do **1 unhandled rejection** "ReferenceError: window is not defined" từ `src/components/assistant/chat-panel.test.tsx` (`hooks/use-assistant.ts:246`, sau khi môi trường test đã teardown) |
| `npx vitest run` (lần 2) | **101 file / 1767 test pass**, 0 error, exit 0 |
| `npx vitest run src/components/assistant/chat-panel.test.tsx` | 8/8 pass, exit 0 |
| `NEXT_PUBLIC_API_BASE=http://localhost:8080 npm run build` | exit 0, "Compiled successfully", 16/16 trang. `ƒ /ops/events` (165 B, 106 kB) và `ƒ /ops/events/[eventId]` (128 B, 114 kB) là **dynamic**. Cảnh báo có sẵn: không nạp được `@next/swc-win32-x64-msvc` (DLL), warning `errorMessage` ở `LoginForm.tsx` — đều ngoài phạm vi. |

### 1.1 Số test

- Base EV-198: 1564 test / 95 file. EV-187: **1767 / 101** → **+203 test, +6 file**.
- Khớp từng file với `docs/kiem-thu/README.md`: `format.test.ts` 66→94 (+28), `ops-service.test.ts` 53, `use-ops-war-room.test.tsx` 34,
  `ops-war-room.test.tsx` 74, `[eventId]/page.test.tsx` 7, `events/page.test.tsx` 5, `not-found.test.tsx` 2 → 28+53+34+74+7+5+2 = 203.

### 1.2 Test có sẵn

`nav-config.test.ts`, `app/(ops)/layout.test.tsx` không bị sửa (I-1) và vẫn xanh trong lần chạy toàn bộ.

### 1.3 Nợ không liên quan

`chat-panel.test.tsx` (EV-185): `setMessages` trong `use-assistant.ts:246` chạy sau khi jsdom teardown → unhandled rejection
"window is not defined", **ngắt quãng** (1/2 lần chạy toàn bộ; chạy riêng file thì sạch). EV-187 không đụng
`components/assistant/**` hay `hooks/use-assistant.ts` (`git diff origin/feature/EV-198-e2e-playwright..HEAD` rỗng cho các đường dẫn đó).
Ghi nhận là nợ của EV-185, không chặn EV-187.

## 2. E2E trên bản build standalone

Chuẩn bị:

```
cp -r apps/web/.next/static apps/web/.next/standalone/apps/web/.next/
cp -r apps/web/public       apps/web/.next/standalone/apps/web/
cd apps/web/.next/standalone/apps/web && HOSTNAME=127.0.0.1 PORT=3000 node server.js   # chạy nền
curl http://127.0.0.1:3000/api/healthz   # 200
cd tests/e2e && npm ci                    # dùng lockfile của EV-198
```

| Lệnh (trong `tests/e2e`) | Kết quả |
|---|---|
| `BASE_URL=http://localhost:3000 E2E_API_BASE=http://localhost:8080 npx playwright test --retries=0` | **7 passed** (7,9s): `smoke.spec.ts` (1), `buy-flow.spec.ts` (5), `ops-war-room.spec.ts` › "ops vào war room từ danh sách, thấy dữ liệu mẫu và làm mới được" (1,7s) |
| `… --repeat-each=20 --retries=0` | **140/140 passed** (50,0s), 0 flaky |

Dọn dẹp: chỉ kill PID 28936 (tiến trình LISTEN ở `127.0.0.1:3000` theo `netstat`), cổng trống sau đó; xoá `tests/e2e/test-results`
và `tests/e2e/report`. `git status` sạch (không sinh file lạ, `package-lock.json` không đổi).

Spec E2E bắt `console` error và `pageerror` (không có lỗi hydration), bấm "Làm mới" và chờ "Cập nhật lúc" đổi (M-5).
Hạn chế đã biết (code review t-4): poll 2s cũng làm đổi giờ nên E2E không cô lập tác dụng nút; unit test thì có cô lập.

CI: workflow `e2e-web.yml` (của EV-198) sẽ chạy spec này khi mở PR của EV-187; lần verify này là chạy cục bộ.

## 3. Hygiene E2E

Regex chép nguyên từ `.github/workflows/e2e-web.yml` dòng 53:

```
grep -rnE '\.[[:space:]]*waitForTimeout[[:space:]]*\(|\b(test|it|describe)([[:space:]]*\.[[:space:]]*(describe|serial|parallel))*[[:space:]]*\.[[:space:]]*(skip|only|todo|fixme)[[:space:]]*\(|new Promise[^;]*setTimeout' \
  tests/e2e --include='*.ts' --include='*.js' --include='*.mjs' --exclude-dir=node_modules
```

Kết quả: **không khớp dòng nào** (HYGIENE_OK).

## 4. Phạm vi diff

`git diff --stat origin/feature/EV-198-e2e-playwright..HEAD`: 26 file, +4622 / −12.

| File | Được phép theo |
|---|---|
| `apps/web/src/lib/ops-service.ts` (+test) | plan §3 |
| `apps/web/src/lib/format.ts` (+test) | plan §6 (dòng "File được phép thêm") |
| `apps/web/src/hooks/use-ops-war-room.ts` (+test) | plan §3 |
| `apps/web/src/components/ops/{index.ts, ops-panels.tsx, ops-war-room.tsx, ops-war-room.test.tsx}` | plan §3 `components/ops/**` |
| `apps/web/src/app/(ops)/ops/events/page.tsx` (+test), `[eventId]/page.tsx` (+test), `[eventId]/not-found.tsx` (+test) | plan §3 `app/(ops)/ops/**`, §6 |
| `apps/web/src/components/layout/nav-config.ts` | plan §3 — chỉ đổi cờ War room `ready: true` + 2 comment (m-2) |
| `tests/e2e/specs/ops-war-room.spec.ts` | plan §3 |
| `docs/workflow/EV-187-ops-war-room/**`, `docs/kiem-thu/README.md` | plan §3 |

Không có file ngoài danh sách. Không đụng `tests/e2e/playwright.config.ts`, `tests/e2e/fixtures/**`, `nav-config.test.ts`,
`app/(ops)/layout.test.tsx`.

## 5. AC ↔ bằng chứng

| AC | Bằng chứng (đều xanh trong lần chạy §1, §2) |
|---|---|
| AC-1 | `ops-war-room.test.tsx` › AC-1; `ops-service.test.ts` › bảng số mẫu M-4; `format.test.ts`; `[eventId]/page.test.tsx`; E2E thấy h1, phễu, bảng tồn kho |
| AC-2 | `use-ops-war-room.test.tsx` (interval 2s, `ageMs` null lúc render đầu, mốc 5s); `ops-war-room.test.tsx` › AC-2 (banner role `alert`, giữ số); `formatTimeVn` |
| AC-3 | `ops-war-room.test.tsx` › AC-3 (role `status` "Dữ liệu mẫu", unavailable/stale theo nguồn, không "bình thường" giả); E2E thấy nhãn "Dữ liệu mẫu" |
| AC-4 | `ops-service.test.ts` › `tierInventoryViolation` (gồm INVALID_DATA); `ops-war-room.test.tsx` › AC-4 |
| AC-5 | `ops-war-room.test.tsx` › AC-5 (4 nấc nguyên văn, "Ops đặt tay", mode lạ) |
| AC-6 | `ops-war-room.test.tsx` › AC-6 (khu riêng, nút disabled, `aria-describedby` EVF-60, không request); E2E kiểm nút disabled + câu EVF-60 |
| AC-7 | `use-ops-war-room.test.tsx` (403 dừng poll); `ops-war-room.test.tsx` › AC-7; `[eventId]/page.test.tsx` (`notFound()` ngoài `try`); `not-found.test.tsx` |
| AC-8 | `events/page.test.tsx` (link, `ready: true`); `nav-config.test.ts`, `(ops)/layout.test.tsx` xanh; E2E đi từ `/ops/events` vào war room |
| AC-9 | `use-ops-war-room.test.tsx` (không chồng request, dedupe theo `eventId`, timeout, dọn khi unmount, StrictMode đúng 1 fetch) |
| AC-10 | §1 + §2: lint/typecheck/test/build xanh; E2E 7/7 và 140/140 |
| AC-11 | `ops-war-room.test.tsx` › AC-11 (`<table>` + `<caption>` + header, không cướp focus, không `aria-live`), `aria-busy` giữ focus cho "Làm mới"/"Thử lại" |

`traceability.md`: cột "Kết quả" của 12 dòng (AC-1..AC-11 + hợp đồng phụ) đổi thành **Đạt**.

## 6. Finding ↔ chỗ sửa (đã đọc code tại HEAD)

| Finding | Trạng thái | Chỗ sửa |
|---|---|---|
| CR M-1 số thực | Sửa | `lib/format.ts:68` `formatDecimal`; dùng cho `ratePerSec`, `chatQueueLagSeconds` |
| CR m-1 ISO thiếu offset | Sửa | `lib/format.ts:84` regex `(Z|[+-]\d{2}:\d{2})$` |
| CR m-2 focus "Làm mới" | Sửa | `ops-war-room.tsx:158` `aria-busy`, không `loading`/`disabled` |
| CR m-3 `isRefreshing` khi dedupe | Sửa | `use-ops-war-room.ts:119` |
| CR m-4 dedupe theo `eventId` | Sửa | `use-ops-war-room.ts:115` (`current.eventId === eventId`), `:123` `isCurrent` |
| CR m-5 "Cập nhật lúc" cạnh nhãn mẫu | Sửa | `ops-war-room.tsx:167-178` cùng khối với Alert "Dữ liệu mẫu" |
| CR m-6 token CSS không tồn tại | Sửa | không còn `text-heading-*` / `text-body-xs` trong file EV-187 |
| CR m-7 lộ message thô | Sửa | `[eventId]/page.tsx:42` chỉ dùng message của `OpsServiceError` |
| CR m-8 timer tuổi sau 403 | Sửa | `use-ops-war-room.ts:185-189` (effect dừng khi `pollingStopped`) |
| CR m-9 `aria-describedby` | Sửa | `ops-panels.tsx:270,273` |
| CR m-10 lặp code | Sửa | `SALE_STATE_LABEL`, `saleStateLabel`, `opsErrorStatus` ở `lib/ops-service.ts:54-67`; `OpsNotFoundView` dùng chung cho `not-found.tsx` |
| CR t-1, t-2, t-3, t-5 | Sửa | test bổ sung, liệt kê ở `traceability.md` "Bổ sung sau code review" |
| CR t-4 E2E không cô lập nút | Ghi nhận | chấp nhận (mục tiêu M-5 là chứng minh hydrate); unit test có cô lập |
| C-1 request treo | Sửa | `use-ops-war-room.ts:48-65` `OPS_REQUEST_TIMEOUT_MS = 10_000`, `withTimeout` (có `clearTimeout`) |
| C-2 lệch đồng hồ | **Nợ** | ghi trong `traceability.md` (bảng "Bổ sung sau challenge", dòng C-2) |
| C-3 giá trị lạ | Sửa | `ops-panels.tsx:254` budget `"—"`; `ops-service.ts:63` "Không rõ trạng thái"; `ops-panels.tsx:215` AI lạ; `format.ts:58,69` `MAX_SAFE_INTEGER`; `ops-panels.tsx:179` "Chưa có hạng vé." |
| C-4 NaN trong BR-O1 | Sửa | `ops-service.ts:105-106` `INVALID_DATA`; `ops-panels.tsx:195` "Dữ liệu không hợp lệ" |
| C-5 nút "Thử lại" | Sửa | `ops-war-room.tsx:123` `aria-busy` |
| C-6 mutant sống | Sửa | test M46, M17, M47, X63, M24, M28 trong `traceability.md` "Bổ sung sau challenge" |
| C-7, C-8 | Info | không sửa (theo plan §8) |

## 7. Nợ còn lại (không chặn merge)

- C-2: tuổi dữ liệu dùng đồng hồ trình duyệt so với `generatedAt` của server — xử lý khi có Ops API thật (`useServerTimeSync`/offset).
- Ngoài phạm vi theo spec §4: Ops API thật, ghi đè admit rate (EVF-60), Grafana (EVF-125), biểu đồ xu hướng, `RoleGuard` cho `(ops)` (EV-114).
- `chat-panel.test.tsx` unhandled rejection ngắt quãng (EV-185, §1.3).
- Nhánh xếp chồng: phải rebase lên `develop` sau khi PR #50 merge; `docs/kiem-thu/README.md` có thể xung đột (plan §4).
