# EV-188 — Ma trận truy vết AC ↔ Test

Viết bởi test designer từ `spec.md` + `plan.md` (§6 thay §1 khi khác) + `plan-review.md`, trước khi implement.
Cột "Kết quả" đã cập nhật ở bước verify (2026-10-13, `evidence.md`): unit 108 file / 1936, E2E 38/38, ×5 190/190.

Viết tắt đường dẫn (`apps/web/src/` lược bỏ):
`msg` = `lib/i18n/messages.test.ts`, `prov` = `components/i18n/locale-provider.test.tsx`, `t` = `components/i18n/t.test.tsx`,
`tog` = `components/i18n/language-toggle.test.tsx`, `cfg` = `components/layout/nav-config.test.ts`,
`link` = `components/layout/nav-link.test.tsx`, `skip` = `components/layout/skip-link.test.tsx`,
`hdr` = `components/layout/public-header.test.tsx`, `ftr` = `components/layout/public-footer.test.tsx`,
`pshell` = `components/layout/public-shell.test.tsx`, `fshell` = `components/layout/focus-shell.test.tsx` (mới),
`wshell` = `components/layout/workspace-shell.test.tsx`, `theme` = `components/theme/theme-toggle.test.tsx`,
`panel` = `components/event/event-action-panel.test.tsx`;
`axe` = `tests/e2e/specs/a11y.spec.ts`, `i18n` = `tests/e2e/specs/i18n.spec.ts`, `bf` = `tests/e2e/specs/buy-flow.spec.ts`,
`fx-axe` = `tests/e2e/fixtures/axe.ts`, `fx-bf` = `tests/e2e/fixtures/buy-flow-helpers.ts`.

## 1. AC → test

| AC | Yêu cầu | File | Test | Kết quả |
|---|---|---|---|---|
| AC-1 | Quét axe mọi màn hình Buyer, tag wcag2a/2aa/21a/21aa, cả light và dark | `axe`, `fx-axe`, `fx-bf` | Mỗi theme (`light`, `dark`): "trang công khai: /, /events, /events/vietnam-tech-summit-2026"; "luồng mua vé: QUEUED -> checkout trước/sau giữ -> thanh toán -> kết quả" (một luồng đầy đủ/theme, M4); "phòng chờ khi join bị 401"; "/auth/sign-in (chặn Google + grainy-gradients)" (M5); "locale en (m10): /, /events/vietnam-tech-summit-2026"; "M3 reflow 320px: / menu đóng và mở, không tràn ngang" | **đạt** — verify 2026-10-13: 12/12 (×5: 60/60), 0 serious/critical, 0 moderate/minor (`evidence.md` §3.1) |
| AC-2 | serious/critical làm fail kèm rule id + selector + trang; moderate/minor vào báo cáo | `fx-axe` | `expectNoSeriousA11yViolations(page, label)`: `expect.soft` với thông báo `[nhãn] rule (impact) — N node` + selector từng node + help URL; attachment JSON gồm `violations` + `incomplete`; `console.log` bảng moderate/minor + incomplete (m1) | **đạt** — 0 serious/critical; incomplete `color-contrast` in log + attachment |
| AC-3 | Sửa ở gốc; mỗi loại lỗi có unit test hoặc chứng minh bằng axe | `axe`, `event-card.test.tsx`, `ticket-tier-card.test.tsx`, `order-result-view.test.tsx`, `ticket-tier-list.test.tsx`, `[slug]/page.test.tsx` | Danh sách lỗi gốc + test giữ lại ở §3. Tương phản (V1–V3) chứng minh bằng `axe`; V1/V2 thêm unit giữ gốc lỗi (không opacity trên chữ); V4, I1 có unit RTL; cả 10 test đã kiểm đột biến | **đạt** — unit 1936/1936; axe 0 serious/critical (verify 2026-10-13) |
| AC-4 | Chạy trong CI bằng `e2e-web.yml`, `--repeat-each=5 --retries=0` không flaky | `axe`, `i18n` | Spec nằm trong `tests/e2e/specs/` (workflow chạy toàn bộ); không `waitForTimeout`/sleep/`.skip`/`.only`/`.fixme` (hygiene grep sạch); chờ bằng `expect` + cổng `createGate` | **đạt** — 38/38; `--repeat-each=5 --retries=0` 190/190; hygiene grep không khớp |
| AC-5 | Từ điển có kiểu; `en` đủ khoá; `useT`/`useLocale` qua `LocaleProvider` | `msg`, `prov`, `t` | `msg` › hằng số (3), "en có ĐÚNG tập khoá của vi", "không có chuỗi rỗng" (vi, en), "cùng tập tham số {tên}", "khoá dạng nhóm.tên", translate (6: dịch đúng, interpolation chuỗi + số, thay mọi lần xuất hiện, param thừa, thiếu khoá en → vi, locale lạ → vi), isLocale (13); `prov` › toàn bộ; `t` › toàn bộ | **đạt** — vitest 1936/1936, typecheck sạch |
| AC-6 | Đổi ngôn ngữ không reload; `html lang` đổi; nhớ qua reload | `prov`, `tog`, `hdr`, `i18n` | `prov` › "setLocale: đổi chữ, ghi localStorage, đặt html lang", "không điều hướng: URL và history giữ nguyên", "không remount con", "khôi phục locale đã lưu…" (+ không ghi lại storage), "giá trị lưu lạ bị bỏ qua" (×5), localStorage bị chặn (2); `tog` › hành vi (6); `hdr` › "bấm English trong header: chữ đổi ngay"; `i18n` › "trang chủ: bấm English -> chữ đổi ngay, html lang = en, không điều hướng (biến window còn, URL không đổi, 0 framenavigated), focus giữ trên nút", "bấm Tiếng Việt -> trở về vi", "lựa chọn được nhớ qua reload", "trang sự kiện: bấm English -> panel dịch, gốc panel lang = en, main vẫn lang = vi" | **đạt** — unit xanh; `i18n` › AC-6 4/4 (×5) |
| AC-7 | Không `cookies()`/`headers()`; ISR giữ; render đầu `vi`; không lỗi hydration | `prov`, `t`, `i18n` + output build | `prov` › "render trên server ra vi dù localStorage là en", "render server KHÔNG đọc localStorage", "hydrate HTML server với en đã lưu: không lỗi hydration", "lần render ĐẦU TIÊN của client là vi"; `t` › "render server ra vi"; `i18n` › "/ và /events/vietnam-tech-summit-2026: HTML server là vi; sau mount thành en" (en lưu sẵn qua addInitScript), "reload với en đã lưu không sinh lỗi hydration", "/events và /events/[slug]: Cache-Control của ISR (s-maxage), không no-store"; mọi test `i18n` fail khi có BẤT KỲ `console.error`/`pageerror` (listener gắn trong `beforeEach` trước `goto`). Verify còn phải chép dòng `/events` (○, 1m) và `/events/[slug]` (●, 1m) từ `next build` (mốc trong `baseline.md` §2) | **đạt** — `i18n` › AC-7 5/5 (×5); build: `├ ○ /events  210 B  111 kB  1m  1y`, `├ ● /events/[slug]  3.88 kB  111 kB  1m  1y` |
| AC-8 | Dịch khung chung + `EventActionPanel`; phần còn lại là nợ | `msg`, `cfg`, `link`, `skip`, `hdr`, `ftr`, `fshell`, `pshell`, `theme`, `panel` | `msg` › "có khoá vi = …" (×19 chuỗi hiện có, nguyên văn), "chuỗi đã chuyển vào từ điển thực sự được dịch sang en", "panel jitter có chuỗi đếm ngược có tham số"; `cfg` › PUBLIC_NAV labelKey (3); `link` › i18n (5, gồm badge "Sắp có"); `skip` › i18n (2); `hdr` › i18n (3); `ftr` › i18n (2); `fshell` › "locale en: skip link dịch sang en"; `theme` › i18n (3); `panel` › i18n (13: mọi trạng thái, jitter, đổi locale khi đang mở) | **đạt kèm nợ** — unit xanh; nợ theo file ở §5 |
| AC-9 | Nút ngôn ngữ truy cập được ở cả hai ngôn ngữ, cho biết ngôn ngữ hiện tại | `tog`, `hdr`, `i18n`, `axe` | `tog` › "nhóm có tên 'Ngôn ngữ' chứa đúng hai nút", `type="button"`, "chữ hiển thị có lang đúng", "lang không đổi khi đổi locale", "KHÔNG có aria-label/aria-labelledby trên nút", "tên truy cập chứa chữ hiển thị (WCAG 2.5.3)", aria-pressed mặc định / khi en, nhóm "Language", "focus ở lại đúng nút vừa bấm (không remount)", "bấm nút đang chọn: không làm gì", bàn phím Tab/Enter/Space; `hdr` › nhóm ngôn ngữ cạnh ThemeToggle (4); `axe` › quét `locale en` + 320px | **đạt** — `tog` xanh; `i18n` › m1 320px/1280px xanh; axe en + 320px sạch |
| AC-10 | Không hồi quy; E2E hiện có xanh | toàn bộ `apps/web`, `bf`, smoke, ops | Test cũ không bị sửa (chỉ nối thêm `describe` mới + thêm `waitFor`/`afterEach` vào dòng import). `bf` dùng helper tách ra `fx-bf`, hành vi giữ nguyên (baseline 6/6 xanh). `theme` › "tên truy cập chứa chữ hiển thị ở vi" | **đạt** — lint 0 lỗi, typecheck, 1936 unit, build; smoke 1/1, buy-flow 5/5, ops-war-room 1/1 |
| M2 (plan §6) | `<main lang="vi">` ở 3 shell; gốc `EventActionPanel` `lang={locale}` | `pshell`, `fshell`, `wshell`, `panel`, `i18n` | `pshell` › "<main> có lang='vi'", "locale en: html lang = en nhưng <main> vẫn lang='vi'"; `fshell` › 2; `wshell` › 1; `panel` › "trạng thái %s, không provider: gốc lang='vi'" (×5), các test en kiểm gốc `lang="en"`; `i18n` › `main#main` lang `vi` sau khi đổi | **đạt** — unit + E2E xanh |
| m3 (plan §6) | Hook ngoài provider; memo theo locale; không `useRouter` | `prov` | "useT dịch theo vi, không ném lỗi", "useT ngoài provider vẫn thay tham số", "setLocale ngoài provider là no-op", "cùng một locale: hàm t giữ nguyên…; đổi locale: hàm mới"; file test KHÔNG mock `next/navigation` (provider dùng `useRouter` thì render ném lỗi) | **đạt** — unit xanh |

## 2. Hợp đồng test đặt thêm (implementer cần biết)

- Khoá không bị chốt tên, trừ `skipLink.label` (plan §1.1) và `language.group` = "Ngôn ngữ"/"Language" (plan §6 M1).
  Test tìm khoá theo **giá trị** tiếng Việt; một chuỗi vi trùng ở nhiều khoá phải có cùng bản dịch en.
- Chuỗi jitter của panel vào từ điển có tham số, dạng vi `Còn {x}s (giảm tải đột biến)` (tên tham số tuỳ ý).
- `<T>` render chữ trần, không bọc thêm phần tử.
- Nút ngôn ngữ: thứ tự DOM "Tiếng Việt" rồi "English"; `lang` đặt trên nút hoặc phần tử chữ bên trong.
- ThemeToggle en: nhãn `aria-label` tiếng Anh phải chứa (không phân biệt hoa thường) chữ hiển thị tiếng Anh
  ("Light"/"Dark" hoặc tương đương) — WCAG 2.5.3.
- Unit test i18n trong file có sẵn: lúc baseline dùng `import()` động (đường dẫn biến, xem `baseline.md` §1); sau khi
  implement đã đổi sang import tĩnh (`@/lib/i18n/messages`, `@/components/i18n/locale-provider`), khẳng định giữ nguyên
  (2026-10-13: vitest 1904/1904, `typecheck` sạch, `lint` 0 lỗi).

## 3. Lỗi a11y gốc (từ baseline) → file sửa → test giữ lại

Kết quả axe/E2E theo báo cáo của implementer (axe 0 serious/critical, E2E xanh); test designer chỉ chạy lại unit — xác
nhận E2E độc lập ở bước verify. Unit test giữ lại (AC-3) thêm 2026-10-13: 10 test, mỗi test đã được **kiểm đột biến**
(trả bản sửa về như cũ → test đỏ → khôi phục nguyên byte, `cmp` khớp). Sau đó: vitest 1914/1914, `typecheck` sạch,
`lint` 0 lỗi.

| # | Rule | Trang | File (gốc lỗi) | File implementer sửa | Test giữ lại | Kết quả |
|---|---|---|---|---|---|---|
| V1 | `color-contrast` (serious) | `/events` thẻ hết vé, light + dark | `components/event/event-card.tsx:141` (`opacity-70` cả thẻ) | `components/event/event-card.tsx`: bỏ `opacity-70` trên thẻ → `border-dashed`; chỉ ảnh `opacity-60 grayscale` | `axe` › "trang công khai … | light/dark" (tương phản thật). Unit giữ gốc lỗi — `components/event/event-card.test.tsx` › "EventCard — thẻ bị chặn không hạ opacity phần chữ (EV-188 V1)": "SOLD_OUT / CLOSED / COMPLETED / CANCELLED: gốc thẻ và phần tử chứa chữ KHÔNG có lớp opacity-*" (chỉ `img` trang trí được làm mờ), "SOLD_OUT vẫn nói bằng chữ". Đột biến `border-dashed`→`opacity-70`: 1 test đỏ | đã sửa; unit có (đột biến đỏ); **E2E đạt** (verify) |
| V2 | `color-contrast` (serious) | `/checkout` sau giữ vé, thẻ hạng khác bị khoá, light + dark | `components/checkout/ticket-tier-card.tsx:83` (`opacity-60` cả thẻ) | `components/checkout/ticket-tier-card.tsx`: `opacity-60` → `border-dashed`, tên hạng `text-fg-muted` khi bị khoá | `axe` › "luồng mua vé … | light/dark" (tương phản thật). Unit giữ gốc lỗi — `components/checkout/ticket-tier-card.test.tsx` › "TicketTierCard — thẻ bị khoá không hạ opacity (EV-188 V2)": "sold_out: không có lớp opacity-*; vẫn nói hết vé bằng chữ", "disabled + đang giữ vé ở hạng khác: không có lớp opacity-*; Alert cảnh báo hiện đủ chữ". Đột biến `border-dashed`→`opacity-60`: 2 test đỏ | đã sửa; unit có (đột biến đỏ); **E2E đạt** (verify) |
| V3 | `color-contrast` (serious) | `/checkout/…/payment`, light | `components/checkout/payment-handoff-view.tsx:222` (`text-accent` làm màu chữ) | `components/checkout/payment-handoff-view.tsx`: `text-accent` → `text-warning-fg` | **Chỉ chứng minh bằng bài quét axe** `axe` › "luồng mua vé … | light" — tương phản màu không đo được trong jsdom (không có unit test tương đương; AC-3 cho phép) | đã sửa; **E2E đạt** (verify) |
| V4 | `aria-prohibited-attr` (serious) | `/checkout/…/result`, light + dark | `components/checkout/order-result-view.tsx:154` (`aria-label` trên `div` không role) | `components/checkout/order-result-view.tsx`: thêm `role="img"` cho khối QR | `axe` › "luồng mua vé"; `components/checkout/order-result-view.test.tsx` › "OrderResultView — khối mã QR truy cập được (EV-188 V4)": "mỗi vé có một khối QR role img, tên Mã QR vé <mã vé>", "không còn aria-label trên div/span không có role". Đột biến bỏ `role="img"`: 2 test đỏ | đã sửa; unit có (đột biến đỏ); **E2E đạt** (verify) |
| I1 | `aria-valid-attr-value` (incomplete, critical) | `/events/vietnam-tech-summit-2026` | `app/(marketing)/events/[slug]/page.tsx:124` (`aria-labelledby="ticket-tiers-heading"` trỏ id không tồn tại) | `app/(marketing)/events/[slug]/page.tsx`: chỉ đặt `aria-labelledby` khi có hạng vé; `components/event/ticket-tier-list.tsx`: `<h2 id="ticket-tiers-heading">` | `components/event/ticket-tier-list.test.tsx` › "TicketTierList — heading làm tên cho section (EV-188 I1)" (2: heading cấp 2 có `id="ticket-tiers-heading"`; không có hạng vé thì không có id); `app/(marketing)/events/[slug]/page.test.tsx` › "EventDetailPage — section hạng vé có tên (EV-188 I1)" (2: có hạng vé → region tên "Các hạng vé"; không hạng vé → không `aria-labelledby`; cả hai không còn `aria-labelledby` trỏ id không tồn tại). Đột biến bỏ id ở heading: 1 test đỏ; đột biến trả `aria-labelledby` cố định ở trang: 1 test đỏ | đã sửa; unit có (đột biến đỏ); verify: unit xanh |
| M3 | Reflow 1.4.10 (không phải rule axe) | `/` ở 320px, menu đóng/mở | header thêm nhóm ngôn ngữ (nguy cơ tràn ngang) | `components/layout/public-header.tsx`: `Container` `flex-wrap min-h-16 gap-y-sm py-sm`, cụm nút `flex-wrap`; `components/i18n/language-toggle.tsx`: chữ ngắn "VI"/"EN" ở màn hẹp (<sm), tên đầy đủ "Tiếng Việt"/"English" là `sr-only sm:not-sr-only` (tên truy cập vẫn chứa chữ hiển thị, không phân biệt hoa thường — 2.5.3) | `axe` › "M3 reflow 320px: / menu đóng và mở, không tràn ngang" (`scrollWidth <= innerWidth`); `tog` › "tên truy cập chứa chữ hiển thị" | đã sửa; **E2E đạt** (verify) |

**AC-3:** mọi lỗi kiểm được trong jsdom (V4, I1) có unit test RTL; V1/V2 có unit test giữ **gốc lỗi** (không hạ opacity
trên phần tử chứa chữ) còn tương phản thật do axe chứng minh; V3 chỉ do axe chứng minh.

## 3b. Test theo code review vòng 1 (plan §7)

Unit chạy 2026-10-13: vitest 1932/1932 (108 file), `typecheck` sạch, `lint` 0 lỗi. Mỗi test mới đã kiểm đột biến (bỏ
bản sửa → đỏ → khôi phục nguyên byte). E2E mới **chưa chạy** lúc viết — verify 2026-10-13: **đạt** (E2E 38/38, ×5 190/190).

| Finding | File test | Test | Đột biến | Kết quả |
|---|---|---|---|---|
| M1 /auth | `app/auth/layout.test.tsx` (mới) | "html lang = en: nội dung con có ngôn ngữ hiệu lực là vi", "không đổi giao diện" | bỏ `lang` ở `app/auth/layout.tsx`: 1 đỏ | unit đạt |
| M1 /unauthorized | `app/unauthorized/page.test.tsx` (mới) | "phần tử gốc có lang=vi", "tiêu đề 403 có ngôn ngữ hiệu lực là vi" | bỏ `lang` gốc: 2 đỏ | unit đạt |
| M1 WorkspaceShell | `components/layout/workspace-shell.test.tsx` | "tiêu đề, <nav> và nút menu đều có ngôn ngữ hiệu lực là vi", "các link trong nav…" | bỏ `lang` ở tiêu đề: 1 đỏ | unit đạt |
| M1 toast | `components/ui/toast.test.tsx` | "vùng aria-live và toast bên trong có ngôn ngữ hiệu lực là vi" | bỏ `lang` vùng toast: 1 đỏ | unit đạt |
| M1 E2E | `tests/e2e/specs/i18n.spec.ts` | "M1 … /auth/sign-in", "M1 … /ops/events" (en lưu sẵn; `closest([lang])` không tính `<html>`) | — | **đạt** (verify E2E 2026-10-13) |
| m1 | `components/i18n/language-toggle.test.tsx`; `components/layout/public-header.test.tsx`; `i18n.spec.ts` | Truy vấn nút nới thành `/Tiếng Việt/`, `/English/` (giữ ý: tìm đúng nút theo tên ngôn ngữ); mới: "nhãn ngắn VI/EN không bị aria-hidden" (×2), "tên truy cập chứa cả tên đầy đủ và nhãn ngắn" (×2); E2E "m1 … 320px" và "… 1280px": tên truy cập chứa chữ **thực sự hiển thị** (bỏ text sr-only) | thêm `aria-hidden` cho nhãn ngắn: 4 đỏ | unit đạt; **E2E đạt** (verify) |
| m2 | `tests/e2e/specs/a11y.spec.ts` | quét `locale en`: chờ nhóm "Language"; trang sự kiện chờ "Join the waiting room" hiện và "Vào phòng chờ mua vé" biến mất | — | **đạt** (verify E2E 2026-10-13) |
| m3 | `components/event/ticket-tier-list.test.tsx` | "TICKET_TIERS_HEADING_ID là giá trị mặc định", "headingId tuỳ chỉnh…", "không có hạng vé: không render id tuỳ chỉnh" | heading dùng hằng thay vì prop: 1 đỏ | unit đạt |
| m4 | `lib/i18n/messages.test.ts` | "skipLink.label en = Skip to main content" (không test nào ghim bản cũ) | — | unit đạt |
| m5 | `lib/i18n/messages.test.ts` | "thuộc tính kế thừa từ prototype KHÔNG được thay", "Object.prototype không phải tham số", "thiếu tham số giữ {tên}" | `Object.hasOwn` → `in`: 1 đỏ | unit đạt |
| m6 | `tests/e2e/specs/i18n.spec.ts` | "m6 header công khai ở 700px vẫn một hàng": tâm dọc logo / nhóm ngôn ngữ / ThemeToggle / nút menu lệch ≤ 4px; chiều cao hàng trên ở 700px ≤ ở 1280px + 1px | — | **đạt** (verify E2E 2026-10-13) |
| — | `i18n.spec.ts` | Lọc console: lỗi có nguồn từ host bên thứ ba bị chặn chủ đích (`accounts.google.com`, `grainy-gradients.vercel.app`) không tính; mọi `console.error` khác vẫn fail | — | — |

## 3c. Test theo challenge (`challenge.md`)

Unit chạy 2026-10-13: vitest 1936/1936 (108 file), `typecheck` sạch, `lint` 0 lỗi. Test unit mới đã kiểm đột biến
(bỏ bản sửa → đỏ → khôi phục nguyên byte). E2E mới **chưa chạy** lúc viết — verify 2026-10-13: **đạt** (E2E 38/38, ×5 190/190).

| Finding | File test | Test | Đột biến | Kết quả |
|---|---|---|---|---|
| F1 (major) | `components/event/event-action-panel.test.tsx` | "SCHEDULED + en: gốc panel lang='en', đồng hồ vẫn hiện, và mọi đoạn chữ tiếng Việt có lang hiệu lực 'vi'" (kể cả câu sr-only); "mặc định (vi): không đoạn chữ tiếng Việt nào bị khai báo ngôn ngữ khác" | bỏ `<div lang="vi">` quanh `ServerExpiryCountdown`: 1 đỏ | unit đạt |
| F1 E2E | `tests/e2e/specs/i18n.spec.ts` | "challenge F1 … /events/evenflow-grand-concert-2026": en lưu sẵn; `/api/time` bị ghim = mốc mở bán − 2 ngày để trạng thái "chưa tới giờ" **không phụ thuộc ngày chạy** (mốc thật 2026-10-15); mọi text node có dấu tiếng Việt trong gốc panel `lang="en"` phải có `[lang]` gần nhất = `vi` | — | **đạt** (verify E2E 2026-10-13) |
| F2 (minor) | `components/layout/nav-link.test.tsx` | "locale en: chữ badge là bản dịch en và ngôn ngữ hiệu lực của badge là 'en'" (trong `<nav lang="vi">`, nhãn mục vẫn `vi`); "locale vi: badge có ngôn ngữ hiệu lực 'vi'" | bỏ `lang={locale}` trên Badge: 1 đỏ | unit đạt |
| F3 (minor) | `tests/e2e/specs/i18n.spec.ts` | "challenge F3 … /auth/sign-up \| vi/en", "… /auth/forgot-password \| vi/en": mở trang, chờ hydrate nút submit; mọi `console.error`/`pageerror` (vd. "Expected arc flag") làm fail ở `afterEach`; chặn Google + grainy-gradients như M5 và bỏ qua lỗi tải tài nguyên của chính các host đó | — | **đạt** (verify E2E 2026-10-13) |
| N2 | `tests/e2e/specs/a11y.spec.ts` | `prepareForScan`: chờ `toHaveTitle(/\S/)` (title khác rỗng) trước mọi lần quét | — | **đạt** (verify E2E 2026-10-13) |

Ghi chú: F3 trên `/auth/forgot-password` chỉ bắt lỗi ở bước đầu của form (challenge: icon mắt chưa render ở bước
đầu); `/auth/sign-up` là nơi lỗi path SVG lộ ra khi tải trang.

## 4. Loại trừ / chặn trong bài quét (AC-2)

| Mục | Cách | Lý do |
|---|---|---|
| `https://accounts.google.com/**` trên `/auth/sign-in` | `page.route(…).abort()` | Script + iframe Google Identity của bên thứ ba, phụ thuộc mạng; `NEXT_PUBLIC_GOOGLE_CLIENT_ID` rỗng khi build E2E → kết quả không ổn định và không thuộc code của dự án (plan §6 M5) |
| `https://grainy-gradients.vercel.app/**` trên `/auth/sign-in` | `page.route(…).abort()` | Ảnh nền nhiễu của bên thứ ba (`sign-in/page.tsx:24`), phụ thuộc mạng |
| Quét dark của `/auth/sign-in` | Vẫn chạy (không skip) | Trang ép `data-theme="light"` trên wrapper → kết quả gần như light |
| `disableRules` / `exclude` | **Không dùng** | — |

## 5. Nợ i18n theo file (AC-8) — chốt ở bước verify (2026-10-13)

Đã dịch vi/en: `lib/i18n/messages.ts` (23 khoá), `components/layout/{skip-link,public-header,public-footer,nav-link}`,
`PUBLIC_NAV` trong `nav-config.ts`, `components/theme/theme-toggle.tsx`, `components/i18n/language-toggle.tsx`,
`components/event/event-action-panel.tsx` (chữ của chính panel). `FocusShell` chỉ có tên thương hiệu.

Còn tiếng Việt (chưa dịch). Mọi file dưới đây hiển thị trong vùng đã khai báo `lang="vi"` (`<main>` của 3 shell,
`app/auth/layout.tsx`, gốc `/unauthorized`, khung `WorkspaceShell`, vùng toast, `<div lang="vi">` quanh countdown, `Spinner lang="vi"`).
Số trong ngoặc là số dòng code có chữ tiếng Việt (đếm bằng grep, bỏ dòng comment; xấp xỉ).

| Nhóm | File (chuỗi chưa dịch) |
|---|---|
| Trang công khai / sự kiện | `app/(marketing)/page.tsx` (10), `app/(marketing)/events/page.tsx` (4), `app/(marketing)/events/[slug]/page.tsx` (10), `components/event/event-card.tsx` (14), `components/event/ticket-tier-list.tsx` (6), `components/event/lobby-notice.tsx` (4), `lib/event-service.ts` (19, dữ liệu mẫu) |
| Panel sự kiện (phần con) | `components/ui/server-expiry-countdown.tsx` (8 — "Mở bán sau …", câu sr-only, live "Đã tới giờ mở bán"; challenge F1), `components/ui/spinner.tsx` (1 — nhãn mặc định "Đang tải") |
| Phòng chờ | `app/(queue)/waiting/[eventId]/page.tsx` (2), `components/queue/waiting-room.tsx` (23), `components/queue/queue-status-panel.tsx` (37), `components/queue/queue-position.tsx` (7), `components/queue/admit-banner.tsx` (4), `components/queue/progress-ring.tsx` (3) |
| Checkout / thanh toán / kết quả | `app/(checkout)/checkout/[eventId]/page.tsx` (2), `.../payment/page.tsx` (4), `.../result/page.tsx` (2), `components/checkout/checkout-view.tsx` (6), `components/checkout/ticket-tier-card.tsx` (12), `components/checkout/order-summary.tsx` (24), `components/checkout/payment-handoff-view.tsx` (26), `components/checkout/order-result-view.tsx` (37), `hooks/use-hold-timer.ts` (2), `hooks/use-payment-session.ts` (8), `lib/hold-client.ts` (7), `lib/payment-client.ts` (12) |
| Auth | `app/auth/{sign-in,sign-up,forgot-password}/page.tsx` (2 mỗi file), `components/auth/AuthLayout.tsx` (4), `LoginForm.tsx` (26), `RegisterForm.tsx` (56), `ForgotPasswordForm.tsx` (40), `OTPVerification.tsx` (13), `app/unauthorized/page.tsx` (6) |
| Trợ lý AI | `components/assistant/assistant-panel.tsx` (35), `chat-panel.tsx` (21), `faq-chips.tsx` (3), `streaming-message.tsx` (5), `hooks/use-assistant.ts` (5), `lib/assistant-client.ts` (46) |
| UI dùng chung | `components/ui/toast.tsx` (1), `components/ui/input.tsx` (2), `components/ui/alert.tsx` (1) |
| Formatter | `lib/format.ts` (5 — `formatVnd`, `spellDuration`, ngày giờ: chưa theo locale) |
| Metadata | `app/layout.tsx` (`description`), `metadata`/`generateMetadata` của các trang — chỉ tiếng Việt |
| Ngoài Buyer (không thuộc phạm vi WCAG Buyer, nêu để đủ) | `components/layout/workspace-shell.tsx` (2), `nav-config.ts` (`ORGANIZER_NAV`/`OPS_NAV`), `app/(ops)/**`, `app/(organizer)/**`, `components/ops/**`, `components/organizer/**`, `lib/ops-service.ts`, `lib/organizer-service.ts`, `hooks/use-ops-war-room.ts`, `app/(marketing)/showcase/page.tsx`, `components/showcase/**` |

Nợ khác (không phải chuỗi):
- **N1** (challenge): không đồng bộ ngôn ngữ giữa các tab đang mở (không nghe sự kiện `storage`).
- Người dùng `en` thấy chữ `vi` nháy sang `en` sau mount — đánh đổi đã chấp nhận (plan §6 m2) để giữ ISR/không hydration lỗi.
- code-review m3 (nhỏ): điều kiện "có hạng vé" vẫn lặp ở `app/(marketing)/events/[slug]/page.tsx:79` và
  `components/event/ticket-tier-list.tsx:37` (id đã qua prop `headingId` + hằng chung).
