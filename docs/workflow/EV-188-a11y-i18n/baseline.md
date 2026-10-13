# EV-188 — Baseline (test viết trước, chạy trên code HIỆN TẠI)

Người chạy: test designer (agent riêng, không implement). Ngày 2026-10-13. Worktree `EvenFlow-188`, nhánh
`feature/EV-188-a11y-i18n`, HEAD `809e942` + test mới (chưa commit). Code `apps/web/src` **chưa** đổi (chỉ thêm/sửa
`*.test.ts(x)`).

## 1. Unit (Vitest, `apps/web`)

Lệnh: `cd apps/web && npx vitest run`

| | Kết quả |
|---|---|
| Test Files | **14 failed** \| 92 passed (106) |
| Tests | **37 failed** \| **1772 passed** (1809) |
| Test hiện có | **1767/1767 xanh** (1772 = 1767 cũ + 5 test mới đã đúng trên code cũ) |

**4 file mới lỗi import** (module chưa có → cả file đỏ, không đếm vào "Tests"; tổng ~95 test khi chạy được):

| File | Số test (sau khi `it.each` mở rộng) | Lý do đỏ |
|---|---|---|
| `src/lib/i18n/messages.test.ts` | 51 | chưa có `@/lib/i18n/messages` |
| `src/components/i18n/locale-provider.test.tsx` | 23 | chưa có `locale-provider` |
| `src/components/i18n/t.test.tsx` | 6 | chưa có `t` |
| `src/components/i18n/language-toggle.test.tsx` | 15 | chưa có `language-toggle` |

**37 test mới đỏ trong file có sẵn** (phần nối thêm, không sửa test cũ). Phần i18n dùng `import()` động với đường
dẫn biến (`/* @vite-ignore */`) để file cũ vẫn import được trên code cũ — test cũ cùng file vẫn chạy. Sau khi có
module, implementer có thể đổi sang import tĩnh (không bắt buộc).

| File | Đỏ | Xanh sẵn | Lý do đỏ |
|---|---|---|---|
| `components/layout/nav-config.test.ts` | 3 | 0 | `PUBLIC_NAV` chưa có `labelKey`; chưa có module messages |
| `components/layout/nav-link.test.tsx` | 4 | 1 (badge "Sắp có" giữ nguyên khi vi) | chưa có module i18n |
| `components/layout/skip-link.test.tsx` | 2 | 0 | chưa có module i18n |
| `components/layout/public-footer.test.tsx` | 1 | 1 (2 dòng vi nguyên văn) | chưa có module i18n |
| `components/layout/public-header.test.tsx` | 7 | 0 | chưa có nhóm ngôn ngữ / module i18n |
| `components/layout/public-shell.test.tsx` | 2 | 0 | `<main>` chưa có `lang="vi"` |
| `components/layout/workspace-shell.test.tsx` | 1 | 0 | `<main>` chưa có `lang="vi"` |
| `components/layout/focus-shell.test.tsx` (mới) | 2 | 2 (cấu trúc) | `<main>` chưa có `lang="vi"` |
| `components/theme/theme-toggle.test.tsx` | 2 | 1 (tên chứa chữ hiển thị, vi) | chưa có module i18n |
| `components/event/event-action-panel.test.tsx` | 13 | 0 | gốc panel chưa có `lang`; chưa có module i18n |

Ghi chú `tsc`: các file test mới tham chiếu kiểu từ module chưa có → `npm run typecheck` đỏ cho tới khi implement
xong (dự kiến).

## 2. Build production (để chạy E2E)

```
cd apps/web
NEXT_PUBLIC_API_BASE=http://localhost:8080 npm run build      # exit 0
cp -r .next/static .next/standalone/apps/web/.next/static
cp -r public       .next/standalone/apps/web/public
cd .next/standalone/apps/web && HOSTNAME=127.0.0.1 PORT=3000 node server.js   # nền; chờ /api/healthz = {"status":"ok"}
```

Dòng route liên quan AC-7 (trước khi đổi code — mốc so sánh):

```
├ ○ /events                                      210 B         111 kB          1m      1y
├ ● /events/[slug]                             2.78 kB         110 kB          1m      1y
├   ├ /events/vietnam-tech-summit-2026                                         1m      1y
```

Server dừng sau khi chạy (đúng PID lắng nghe `127.0.0.1:3000`), đã xoá `tests/e2e/test-results`.

## 3. E2E (Playwright, `tests/e2e`)

Chung: `BASE_URL=http://localhost:3000 E2E_API_BASE=http://localhost:8080 npx playwright test <spec> --retries=0`.
`@axe-core/playwright` **4.10.2** (axe-core 4.10.3), cài bằng `npm install -D -E` trong `tests/e2e`; chỉ đổi
`tests/e2e/package.json` + `tests/e2e/package-lock.json` (lockfile gốc repo không đổi). Hygiene grep của
`e2e-web.yml` chạy trên `tests/e2e`: không khớp. `tsc -p tests/e2e`: sạch.

| Spec | Kết quả | Ghi chú |
|---|---|---|
| `smoke.spec.ts` + `buy-flow.spec.ts` | **6/6 xanh** | Chứng minh tách helper sang `fixtures/buy-flow-helpers.ts` giữ nguyên hành vi |
| `ops-war-room.spec.ts` | **1/1 xanh** | Không đụng |
| `a11y.spec.ts` | **6 đỏ / 6 xanh** (12 test, 26 lần quét, 15.1s) | 4 test đỏ do vi phạm axe thật (§4); 2 test `locale en` đỏ vì `<html lang>` không thành `en` (chưa có i18n — dự kiến) |
| `i18n.spec.ts` | **7 đỏ / 2 xanh** (13.7s) | Đỏ dự kiến: chưa có nhóm "Ngôn ngữ", `lang` không đổi. Xanh: 2 test ISR (`/events`, `/events/[slug]` trả `Cache-Control` có `s-maxage`, không `no-store`). **Không** có `console.error`/`pageerror` nào trên code hiện tại |

Test xanh của `a11y.spec.ts` trên code cũ: `phòng chờ khi join bị 401` (light, dark), `/auth/sign-in` (light, dark),
`M3 reflow 320px` (light, dark — **không tràn ngang** ở `/` khi menu đóng và mở; axe sạch).

## 4. Vi phạm axe THẬT trên code hiện tại (serious/critical — làm fail)

Tag `wcag2a, wcag2aa, wcag21a, wcag21aa`. Không có vi phạm `moderate`/`minor` nào. Selector là selector axe trả về.

| # | Rule | Impact | Trang / trạng thái | Theme | Node | Selector / phần tử (màu đo được) | Nguồn (gốc lỗi) |
|---|---|---|---|---|---|---|---|
| V1 | `color-contrast` | serious | `/events` (thẻ sự kiện **hết vé** — "Indie Acoustic Night") | light | 6 | badge "Hết vé" `.rounded-full.py-xs.text-label-sm` #838d9a / #e3e6e9 = **2.68**; địa điểm, ngày giờ, "(giờ địa điểm)", "Mọi hạng vé đã bán hết." `text-fg-muted` #838d9a / #fcfdfe = **3.3**; link "Xem chi tiết" `.text-primary` #8986ed / #fcfdfe = **3.07** | `components/event/event-card.tsx:141` — `copy.dimmed && "opacity-70"` trên cả thẻ làm nhạt chữ (token gốc đạt) |
| V1 | `color-contrast` | serious | như trên | dark | 6 | badge #758193 / #263041 = **3.36**; chữ muted #758193 / #10192a = **4.45**; "Xem chi tiết" #6564b5 / #10192a = **3.38** | như trên |
| V2 | `color-contrast` | serious | `/checkout` **sau khi giữ vé** (thẻ hạng vé khác bị khoá — "Standard Pass") | light | 4 | `h3` "Standard Pass" #707781 / #fbfcfd = **4.4**; badge "Còn vé" `.bg-success-soft` #70ab93 / #eef5f3 = **2.39**; Alert tiêu đề "Bạn đang giữ vé ở hạng khác" `.text-warning-fg` #be9964 / #f9f2e9 = **2.38**; nội dung Alert #707781 / #f9f2e9 = **4.07** | `components/checkout/ticket-tier-card.tsx:83` — `blocked && "opacity-60"` trên cả thẻ (kể cả Alert cảnh báo bên trong) |
| V2 | `color-contrast` | serious | như trên | dark | 2 | badge "Còn vé" #2e7f60 / #162f34 = **2.89**; tiêu đề Alert #947340 / #2d2c2e = **3.16** | như trên |
| V3 | `color-contrast` | serious | `/checkout/[eventId]/payment` | light | 1 | `span.font-bold.text-accent` "5.000.000 đ" #e6a23c / #ffffff = **2.18** | `components/checkout/payment-handoff-view.tsx:222` — dùng `text-accent` làm màu chữ; `styles/tokens.css:14-15,44` đã ghi rõ `--ef-accent` **không** dùng làm màu chữ (dùng `--ef-warning-fg` hoặc `text-fg`). Dark đạt |
| V4 | `aria-prohibited-attr` | serious | `/checkout/[eventId]/result` (đơn đã thanh toán) | light + dark | 2 | `div[aria-label="Mã QR vé E2E-TKT-0001"]`, `div[aria-label="Mã QR vé E2E-TKT-0002"]` — `aria-label` trên `div` không có role | `components/checkout/order-result-view.tsx:154` — thêm `role="img"` (khối QR giả là ảnh) hoặc đổi cấu trúc |

Tổng: **4 nhóm lỗi gốc**, 6 lần quét fail (V1×2 theme, V2×2, V3×1, V4×2 → test "trang công khai" light/dark và
"luồng mua vé" light/dark đỏ). Các trang sạch serious/critical trên cả hai theme: `/`, `/events/vietnam-tech-summit-2026`,
`/waiting` (QUEUED và 401), `/checkout` trước giữ vé, `/auth/sign-in`, `/` ở 320px (menu đóng/mở). Payment sạch ở dark.

### 4.1 `incomplete` (axe không tự kết luận — không fail, nhưng nên xem)

| Rule | Impact | Trang | Theme | Phần tử | Nhận định |
|---|---|---|---|---|---|
| `aria-valid-attr-value` | critical | `/events/vietnam-tech-summit-2026` | cả hai | `<section aria-labelledby="ticket-tiers-heading">` — **không có phần tử id `ticket-tiers-heading` trên trang** | **Lỗi thật** (tham chiếu id hỏng → section không có tên). Nguồn `app/(marketing)/events/[slug]/page.tsx:124`. Khuyên sửa cùng đợt |
| `aria-prohibited-attr` | serious | như trên | cả hai | cùng `section` | Hệ quả của dòng trên |
| `color-contrast` | serious | `/waiting` QUEUED | cả hai | "0%" `.text-headline-sm`, "tiến trình" `.text-body-xs` | Nền chứa ảnh/SVG (vòng tiến trình) — axe không đo được; kiểm bằng mắt |
| `color-contrast` | serious | `/checkout` trước/sau giữ | cả hai | `<span>−</span>` trong nút giảm số lượng | Chỉ ký tự không phải chữ — bỏ qua được |
| `color-contrast` | serious | `/`, `/events`, `/events/[slug]`, `/` 320px | dark | `<span aria-hidden="true">☾</span>` trong ThemeToggle | Biểu tượng `aria-hidden`, không phải chữ — bỏ qua được |

### 4.2 Quyết định trong bài quét

- Sign-in (M5): `page.route(...).abort()` cho `https://accounts.google.com/**` và `https://grainy-gradients.vercel.app/**`
  (bên thứ ba, phụ thuộc mạng, client id rỗng khi build E2E). Không dùng `exclude`/`disableRules`. Trang ép
  `data-theme="light"` trên wrapper → lần quét dark cho kết quả như light (đã thấy: cả hai 0 vi phạm, 25 pass).
- Theme đặt trước khi tải trang: `emulateMedia({ colorScheme, reducedMotion: "reduce" })` + `addInitScript` ghi
  `localStorage["eventflow-theme"]`. Mỗi theme một luồng mua vé đầy đủ; không lật theme giữa trang.
- Trước mỗi lần quét (m9): chờ `html[data-theme]` đúng theme, chờ hydrate (`#main` hoặc phần tử chỉ định), trang có
  ThemeToggle chờ `aria-pressed` khớp theme, `mouse.move(0,0)`, chờ `document.fonts.ready`.
- `expect.soft` trong `fixtures/axe.ts`: một test quét nhiều trang vẫn chạy hết và báo đủ mọi trang lỗi.
- Bảng `moderate/minor + incomplete` in ra log (`console.log`), toàn bộ kết quả (gồm `incomplete`) đính kèm JSON
  (`axe-<nhãn>.json`) trong report của Playwright.

## 5. Gợi ý cho implementer (không ràng buộc)

- V1/V2: bỏ `opacity-*` trên cả thẻ; làm "mờ" bằng cách khác không giảm tương phản chữ (vd. viền/nền/ảnh mờ, hoặc
  chỉ đổi badge). Lưu ý V2 làm mờ cả `Alert` cảnh báo — phần quan trọng nhất của thẻ bị khoá.
- V3: `text-accent` → `text-fg` (hoặc `text-warning-fg`) theo quy ước trong `tokens.css`.
- V4: `role="img"` + `aria-label` cho khối QR. Có thể giữ lại bằng unit test RTL (`getByRole("img", { name: "Mã QR vé …" })`).
- (incomplete) id `ticket-tiers-heading`: đặt id cho heading của danh sách hạng vé.
- Sau khi thêm nút ngôn ngữ, chạy lại `a11y.spec.ts` (cả `locale en` và 320px — header thêm hai nút có thể gây tràn).
