# EV-188 — Bằng chứng verify (Accessibility WCAG AA + i18n vi/en)

| Mục | Giá trị |
|---|---|
| Người verify | agent verify độc lập, chỉ đọc (không viết code/test nào của EV-188) |
| Thời điểm | 2026-10-13 08:45–09:06 (+07:00) |
| Worktree | `D:/Documents/Nam4-1/Project1/EvenFlow-188` |
| Nhánh | `feature/EV-188-a11y-i18n` — thay đổi **chưa commit** (40 file sửa + 23 file mới) |
| HEAD | `809e942` (nhánh EV-187 `docs(workflow): add EV-187 verification evidence`) |
| Nền xếp chồng | EV-188 → EV-187 (`809e942`) → EV-198 (PR #50) → `develop` @ `51c4729` |
| Môi trường | Windows 11, Node v22.21.0, Playwright 1.49.1 (chromium), `@axe-core/playwright` 4.10.2 (axe-core ~4.10.3) |

## Kết luận: **PASS** (kèm nợ đã ghi, xem §7)

Mọi gate xanh: unit 108 file / **1936** test, lint 0 lỗi, typecheck sạch, build OK với `/events` (○, 1m) và
`/events/[slug]` (●, 1m). E2E toàn bộ **38/38**, lặp `--repeat-each=5 --retries=0` **190/190**, 0 vi phạm axe
serious/critical, 0 moderate/minor. Hygiene grep sạch, lockfile gốc không đổi, diff nằm trong phạm vi cho phép.

## 1. Unit / lint / typecheck (`apps/web`)

| Lệnh | Kết quả |
|---|---|
| `npm run lint` | exit 0 — **0 lỗi**, 21 cảnh báo, tất cả ở `components/auth/{ForgotPasswordForm,LoginForm,OTPVerification,RegisterForm}.tsx` (`no-unused-vars`, `no-explicit-any`, `no-img-element`) — có từ trước, không nằm trên dòng EV-188 sửa (chỉ đổi path SVG) |
| `npm run typecheck` (`tsc --noEmit`) | exit 0, sạch |
| `npx vitest run` | **108 file / 1936 test xanh** (26.6s). Baseline trước EV-188: 101 file / 1767 → +7 file, +169 test |

Test hiện có không bị sửa: `git diff -U0` trên mọi `*.test.ts(x)` — dòng bị xoá **chỉ** là dòng import (thêm
`afterEach`/`waitFor`/`vi`/hằng `TICKET_TIERS_HEADING_ID`) (AC-10).

## 2. Build production (AC-7)

```
cd apps/web && NEXT_PUBLIC_API_BASE=http://localhost:8080 npm run build     # exit 0, Compiled successfully, 16/16 static pages
```

Dòng route (chép nguyên văn):

```
┌ ○ /                                            210 B         111 kB
├ ○ /events                                      210 B         111 kB          1m      1y
├ ● /events/[slug]                             3.88 kB         111 kB          1m      1y
├   ├ /events/evenflow-grand-concert-2026                                      1m      1y
├   ├ /events/vietnam-tech-summit-2026                                         1m      1y
├   └ /events/indie-acoustic-night-hanoi                                       1m      1y
```

So với baseline (`baseline.md` §2): `/events` giữ `○` 1m; `/events/[slug]` giữ `●` 1m (2.78 kB → 3.88 kB do
`EventActionPanel` dùng i18n). Không route công khai nào thành `ƒ`.
`grep -rnE "cookies\(|headers\("` trên `app/layout.tsx`, `components/i18n`, `lib/i18n`, `app/(marketing)/events`: chỉ
khớp một dòng **comment** trong `app/layout.tsx:32` — không có lời gọi.

## 3. E2E trên bản standalone

```
cp -r .next/static .next/standalone/apps/web/.next/static ; cp -r public .next/standalone/apps/web/public
cd .next/standalone/apps/web && HOSTNAME=127.0.0.1 PORT=3000 node server.js     # nền, PID 35452; /api/healthz = {"status":"ok"}
cd tests/e2e && npm ci
BASE_URL=http://localhost:3000 E2E_API_BASE=http://localhost:8080 npx playwright test --retries=0
BASE_URL=http://localhost:3000 E2E_API_BASE=http://localhost:8080 npx playwright test --repeat-each=5 --retries=0
```

| Spec | 1 lần (`--retries=0`) | `--repeat-each=5 --retries=0` |
|---|---|---|
| `a11y.spec.ts` | 12/12 | 60/60 |
| `i18n.spec.ts` | 19/19 | 95/95 |
| `buy-flow.spec.ts` | 5/5 | 25/25 |
| `smoke.spec.ts` | 1/1 | 5/5 |
| `ops-war-room.spec.ts` | 1/1 | 5/5 |
| **Tổng** | **38 passed (14.9s)** | **190 passed (49.7s)**, 0 failed, 0 flaky |

Sau khi chạy: dừng đúng PID 35452 (cổng 3000 không còn LISTEN), xoá `tests/e2e/test-results` và `tests/e2e/report`.

### 3.1 Axe (AC-1, AC-2)

- 12 test axe phủ: `/`, `/events`, `/events/vietnam-tech-summit-2026`; luồng mua vé `/waiting` QUEUED →
  `/checkout` trước giữ → sau giữ → thanh toán → kết quả; `/waiting` khi join 401; `/auth/sign-in`; locale `en` trên
  `/` và trang sự kiện; `/` ở 320px menu đóng/mở — mỗi cái ở **light và dark**. Tag `wcag2a, wcag2aa, wcag21a,
  wcag21aa` (`fixtures/axe.ts`).
- **serious/critical: 0** ở mọi lần quét (cả 190 lần chạy).
- **moderate/minor: 0** (không dòng nào được in).
- `incomplete` (không chặn, được in ra log + attachment): chỉ rule `color-contrast` (serious) — 13 lần/lượt chạy:
  `/ | dark`, `/ en | dark`, `/ 320px menu đóng/mở | dark`, `/events | dark`, `/events/vietnam-tech-summit-2026 (vi, en) |
  dark` (1 node), `/waiting QUEUED | light, dark` (2 node — vòng tiến trình, đã ghi ở `baseline.md` §4.1),
  `/checkout trước giữ | light, dark`, `/checkout sau giữ | light, dark` (1 node). Là phần tử trên nền ảnh/gradient axe
  không tự đo — không phải vi phạm.
- Không `disableRules`/`exclude` (grep chỉ khớp comment trong `fixtures/axe.ts:14`). Chặn bên thứ ba trên
  `/auth/sign-in` có lý do ở `traceability.md` §4.

## 4. Hygiene, lockfile, phiên bản (AC-4)

| Kiểm | Kết quả |
|---|---|
| Regex hygiene nguyên văn từ `.github/workflows/e2e-web.yml` chạy trên `tests/e2e` (`--include=*.ts,*.js,*.mjs`, bỏ `node_modules`) | **không khớp** (grep exit 1) |
| `git diff --stat -- package-lock.json` (gốc repo) | rỗng |
| `.github/**`, `tests/e2e/playwright.config.ts` | không đổi |
| `tests/e2e/package.json` | `"@axe-core/playwright": "4.10.2"` (pin chính xác); `package-lock.json` của `tests/e2e` resolved 4.10.2 |
| `npm ci` trong `tests/e2e` | không làm đổi file nào trong git |

Spec mới nằm trong `tests/e2e/specs/` nên `e2e-web.yml` (chạy toàn bộ spec) tự chạy; `ci.yml` không đổi.

## 5. Phạm vi diff

Mọi file đổi nằm trong danh sách cho phép:

- plan §3: `lib/i18n/**`, `components/i18n/**`, `app/layout.tsx`; file §1.3: `components/layout/{skip-link,public-header,
  public-footer,focus-shell,nav-link,nav-config}.tsx|ts`, `components/theme/theme-toggle.tsx`,
  `components/event/event-action-panel.tsx` + test cạnh chúng; `tests/e2e/**` (package.json, lockfile, `buy-flow.spec.ts`,
  `fixtures/{axe,buy-flow-helpers}.ts`, `specs/{a11y,i18n}.spec.ts`); `docs/workflow/EV-188-a11y-i18n/**`.
- plan §6: `components/layout/{public-shell,workspace-shell}.tsx` + test, `focus-shell.test.tsx` (mới).
- File sửa lỗi axe (traceability §3): `components/event/event-card.tsx`, `components/checkout/{ticket-tier-card,
  payment-handoff-view,order-result-view}.tsx`, `app/(marketing)/events/[slug]/page.tsx`, `components/event/ticket-tier-list.tsx`
  + test.
- plan §7: `app/auth/layout.tsx` (mới) + test, `app/unauthorized/page.tsx` + test (mới), `components/ui/toast.tsx` + test.
- Sửa path SVG (lỗi console "Expected arc flag"): `components/auth/{LoginForm,ForgotPasswordForm,RegisterForm}.tsx` — mỗi
  file đúng 1 dòng `d=` của `EyeIcon`.

Không có file ngoài phạm vi. `docs/kiem-thu/README.md` không đổi (được phép nhưng không bắt buộc).

## 6. AC → bằng chứng

| AC | Bằng chứng | Kết quả |
|---|---|---|
| AC-1 | `a11y.spec.ts` 12 test × light/dark, mọi trạng thái spec yêu cầu (§3.1) | ĐẠT |
| AC-2 | 0 serious/critical; thông báo fail gồm nhãn trang + rule + selector + help URL (`fixtures/axe.ts`, đã thấy đỏ đúng ở `baseline.md` §4 và challenge A1–A4); moderate/minor/incomplete in log + attachment; không `disableRules`/`exclude` | ĐẠT |
| AC-3 | V1–V4 + I1 sửa ở component/token; unit giữ lại (đột biến đỏ, `traceability.md` §3); V3 chứng minh bằng axe; tất cả xanh trong 1936 unit + axe | ĐẠT |
| AC-4 | Spec trong `tests/e2e/specs/`, `e2e-web.yml`/`ci.yml` không đổi; hygiene sạch; ×5 190/190 không flaky | ĐẠT |
| AC-5 | `messages.ts` (`en: Record<MessageKey,string>`) + `messages.test.ts` so khoá/tham số; `LocaleProvider` ở `app/layout.tsx`; typecheck sạch | ĐẠT |
| AC-6 | `i18n.spec.ts` › AC-6 (4 test): chữ đổi, `html lang=en`, biến `window` còn, URL không đổi, 0 `framenavigated`, focus giữ, nhớ qua reload | ĐẠT |
| AC-7 | Build: `/events` ○ 1m, `/events/[slug]` ● 1m; không `cookies()`/`headers()`; `i18n.spec.ts` › AC-7 (5 test): HTML server là `vi`, sau mount `en`, 0 `console.error`/`pageerror`, `Cache-Control` có `s-maxage` | ĐẠT |
| AC-8 | Khung chung + `EventActionPanel` dịch (unit `msg/cfg/link/skip/hdr/ftr/fshell/theme/panel` + E2E AC-6, F1); nợ theo file ở `traceability.md` §5 | ĐẠT (kèm nợ) |
| AC-9 | `role="group"` "Ngôn ngữ"/"Language", 2 `<button aria-pressed>` có `lang`, tên chứa chữ hiển thị ở 320px và 1280px (`i18n.spec.ts` › m1), unit `language-toggle.test.tsx`; quét axe `en` + 320px sạch | ĐẠT |
| AC-10 | lint/typecheck/test/build xanh; test cũ chỉ đổi dòng import; smoke 1/1, buy-flow 5/5, ops-war-room 1/1 (×5 cũng xanh) | ĐẠT |

## 7. Phát hiện review → xử lý

| Nguồn | Finding | Xử lý | Kiểm |
|---|---|---|---|
| plan-review | M1 nút ngôn ngữ (2.5.3) | Nhóm 2 nút `aria-pressed`, không `aria-label` | `tog`, `i18n` › m1 — xanh |
| plan-review | M2 `lang` phần chưa dịch | `<main lang="vi">` ×3 shell, panel `lang={locale}` | `pshell/fshell/wshell/panel`, `i18n` › AC-6 — xanh |
| plan-review | M3 reflow | header `flex-wrap`, nhãn ngắn VI/EN `<md` | `a11y` › M3 320px, `i18n` › m6 700px — xanh |
| plan-review | M4 helper | `joinQueueFromEvent` + `admitToCheckout` | `buy-flow` 5/5, `a11y` luồng mua — xanh |
| plan-review | M5 sign-in bên thứ ba | `page.route(...).abort()` + lý do traceability §4 | `a11y` › sign-in — xanh |
| plan-review | m1 | Không sửa workflow; moderate/minor/incomplete in log + attachment | thấy trong log §3.1 |
| plan-review | m2–m11 | Theo plan §6 | unit + E2E xanh; m7 lockfile §4; m8 hygiene §4 |
| code-review | M1 vùng chưa dịch ngoài `<main>` | `app/auth/layout.tsx`, `unauthorized`, `WorkspaceShell`, toast `lang="vi"` | unit (đột biến đỏ) + `i18n` › M1 ×2 — xanh |
| code-review | m1 nhãn ngắn | bỏ `aria-hidden`, tên "Tiếng Việt VI"/"English EN" | `tog`, `i18n` › m1 320px — xanh |
| code-review | m2 quét en chờ chữ dịch | chờ nhóm "Language" + nút "Join the waiting room" | `a11y` › locale en — xanh |
| code-review | m3 id cứng | prop `headingId` + hằng `TICKET_TIERS_HEADING_ID` | `ticket-tier-list.test.tsx` — xanh. **Còn lại:** điều kiện "có hạng vé" vẫn lặp ở `[slug]/page.tsx:79` và `ticket-tier-list.tsx:37` → ghi nợ nhỏ |
| code-review | m4 bản dịch | "Skip to main content"; "Sale closed/starts/not open yet" thống nhất | `msg` — xanh |
| code-review | m5 `in` → `Object.hasOwn` | `messages.ts:90` | `msg` (đột biến đỏ) — xanh |
| code-review | m6 header 640–767px | nhãn ngắn tới `md` | `i18n` › m6 — xanh |
| code-review | m7 traceability | Làm ở bước verify này (§5 nợ theo file, cột kết quả, dòng build) | `traceability.md` |
| challenge | F1 countdown tiếng Việt dưới `lang="en"` | Hướng (a): bọc `ServerExpiryCountdown` bằng `<div lang="vi">` (3.1.2 đúng). Chữ "Mở bán sau …" **vẫn tiếng Việt** trong giao diện English → nợ (cần dịch `ServerExpiryCountdown` + `spellDuration`, vốn ngoài phạm vi §3) | `panel` (đột biến đỏ), `i18n` › challenge F1 — xanh |
| challenge | F2 badge "Coming soon" trong `nav lang="vi"` | `Badge lang={locale}` | `link` (đột biến đỏ) — xanh |
| challenge | F3 path SVG không có test | E2E `/auth/sign-up`, `/auth/forgot-password` × vi/en fail khi có `console.error` | `i18n` › challenge F3 ×4 — xanh |
| challenge | N1 không đồng bộ giữa tab | Không làm — **nợ** | — |
| challenge | N2 `document-title` khi quét sau client navigation | `prepareForScan` chờ `toHaveTitle(/\S/)` | `a11y` 60/60 ×5 — xanh |

### Nợ còn lại (đã ghi trong `traceability.md` §5)

1. Chuỗi chưa dịch theo file (AC-8) — danh sách đầy đủ ở `traceability.md` §5.
2. Formatter theo locale: `lib/format.ts` (`formatVnd`, `spellDuration`, ngày giờ) chưa theo locale.
3. Metadata trang (`metadata`/`generateMetadata`, `<title>`) chỉ tiếng Việt.
4. Bản dịch trợ lý AI, auth, luồng mua vé/checkout/queue.
5. `ServerExpiryCountdown` trong panel SCHEDULED (challenge F1, phần còn lại) và nhãn mặc định `Spinner` ("Đang tải") trong
   trạng thái jitter — đã khai báo `lang="vi"` nhưng chưa dịch.
6. N1: không đồng bộ ngôn ngữ giữa các tab (không nghe sự kiện `storage`).
7. Người dùng `en` thấy chữ `vi` nháy sang `en` sau khi mount (đánh đổi chấp nhận ở plan §6 m2, để giữ ISR).
8. m3 (nhỏ): điều kiện "có hạng vé" lặp ở trang và component.

## 8. Trạng thái cây làm việc sau verify

Verifier không sửa file nguồn/test/workflow, không commit/push. File verifier tạo/sửa: `evidence.md` (mới),
`traceability.md` (cột kết quả, §5, dòng build AC-7) — đều chưa commit.
