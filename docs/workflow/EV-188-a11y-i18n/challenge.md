# EV-188 — Challenge (phản biện đối kháng)

Người challenge: agent riêng, context sạch. Ngày 2026-10-13. Đầu vào: `spec.md`, `plan.md` (§6, §7 thay §1),
`code-review.md`, `traceability.md` và diff chưa commit trên worktree `EvenFlow-188` (nhánh
`feature/EV-188-a11y-i18n`, nền `809e942`). Đường dẫn tính từ `apps/web/src/` nếu không ghi khác.

Môi trường: `npm run build` với `NEXT_PUBLIC_API_BASE=http://localhost:8080`, standalone `node server.js` ở
`127.0.0.1:3000`, Playwright chromium với `--retries=0`. Probe là file tạm `tests/e2e/specs/zz-challenge.spec.ts`
(đã xoá). Mọi đột biến đã khôi phục nguyên byte (xem §5).

## Kết luận: **FAIL** (1 major, 2 minor, 2 ghi chú)

Phần lớn thay đổi đứng vững: cả 4 lỗi a11y đều bị bài quét axe bắt khi đảo ngược, các wrapper `lang` chính đều có
test giữ lại, ISR còn nguyên, không lỗi hydration, không flaky trong 330 lần chạy. **F1** phá AC-8 (một trạng thái
của `EventActionPanel` hiện chữ tiếng Việt khi chọn English) và tạo lại đúng loại lỗi WCAG 3.1.2 mà code review M1
đã xếp major, nên phải sửa trước khi merge.

## 1. Phát hiện

### Major

**F1 — `EventActionPanel` trạng thái SCHEDULED: chữ tiếng Việt nằm dưới `lang="en"` và lẫn vào giao diện English
(AC-8, WCAG 3.1.2).**
`components/event/event-action-panel.tsx` (nhánh `!isTimeForSale`) đặt `lang={locale}` ở gốc panel và dịch
"Sale starts" / "Sale not open yet", nhưng bên trong render `ServerExpiryCountdown`
(`components/ui/server-expiry-countdown.tsx:20-30`). Component này có nhãn cứng tiếng Việt ("Mở bán sau", câu
sr-only "Mở bán sau 49 giờ 43 phút 32 giây", vùng live "Đã tới giờ mở bán"). Hệ quả:
- Người dùng English thấy ba ngôn ngữ trộn lẫn: "Sale starts / ⏱ Mở bán sau 49:43:32 / Sale not open yet". AC-8 yêu
  cầu panel và **các trạng thái** của nó được dịch.
- Trình đọc màn hình đọc câu tiếng Việt bằng giọng Anh, vì tổ tiên `[lang]` gần nhất là gốc panel `lang="en"`.
  Trước EV-188 toàn trang là `lang="vi"` nên câu này đọc đúng, tức đây là hồi quy.
- Không test nào chạm vào: E2E i18n chỉ dùng `vietnam-tech-summit-2026` (ON_SALE). Unit test của panel không kiểm
  phần chữ của countdown.

Repro (probe P1): lưu `localStorage["eventflow-locale"]="en"` qua `addInitScript`, rồi `goto("/events/evenflow-grand-concert-2026")`
(sự kiện SCHEDULED, mở bán 2026-10-15). Chờ chữ "Sale starts", rồi duyệt text node có `closest("[lang]")` là `en`
và chứa dấu tiếng Việt.
Kết quả quan sát: `PANEL TEXT: "Sale starts\n⏱\nMở bán sau\n49:43:32\nMở bán sau 49 giờ 43 phút 32 giây\nSale not open yet"`;
`VIET TEXT UNDER lang=en: ["Mở bán sau","Mở bán sau 49 giờ 43 phút 32 giây", …]`. Lặp lại ở cả 2 lần build.
Hướng sửa: (a) bọc `ServerExpiryCountdown` trong panel bằng `<div lang="vi">` (rẻ nhất, đúng 3.1.2 nhưng chữ vẫn lẫn
ngôn ngữ); hoặc (b) cho `ServerExpiryCountdown` nhận nhãn qua từ điển cho biến thể `sale-start` (đúng AC-8, phải
dịch cả `spellDuration`, hiện là nợ). Thêm E2E en cho sự kiện SCHEDULED.

### Minor

**F2 — Workspace nav: badge "Coming soon" (tiếng Anh) nằm trong `<nav lang="vi">`.**
`components/layout/nav-link.tsx` dịch badge bằng `t("nav.comingSoon")` cho **mọi** nav, kể cả `OPS_NAV` (mục "Hạ tầng
AI", `ready: false`). Nav đó bị đánh dấu `lang="vi"` ở `components/layout/workspace-shell.tsx`, nên khi chọn English
thì chữ tiếng Anh bị khai báo là tiếng Việt. Ngoài ra dòng nav còn trộn "Hạ tầng AI COMING SOON".
Repro (probe P2): lưu en, `goto("/ops/events")`, rồi `getByText("Coming soon").evaluate(el => el.closest("[lang]").lang)`.
Kết quả quan sát: `Coming soon badge effective lang: vi`; `nav text: "War room\nHạ tầng AI\nCOMING SOON"`.
Hướng sửa: badge dùng `t()` chỉ khi item có `labelKey` (tức PUBLIC_NAV), còn lại giữ "Sắp có"; hoặc đặt `lang={locale}`
trên `<Badge>`.

**F3 — Sửa path SVG trong `RegisterForm` / `ForgotPasswordForm` không có test giữ lại.**
Bản sửa đúng: lệnh arc giờ có đủ cờ `0 1`, và lỗi console biến mất (probe P3 xanh trên `/auth/sign-up`,
`/auth/forgot-password`). Tuy vậy chỉ `LoginForm` được bảo vệ, và chỉ gián tiếp qua listener console của
`i18n.spec.ts` › "M1 … /auth/sign-in". Không spec nào truy cập `/auth/sign-up` hay `/auth/forgot-password`, và
`components/auth/` không có unit test.
Repro (đột biến vòng 2): trả path cũ `…19.876 0 1 1 0 .696 10.75…` ở `RegisterForm.tsx` và `ForgotPasswordForm.tsx`,
build lại, chạy bộ E2E đã commit.
Kết quả quan sát: bộ đã commit **xanh hoàn toàn**. Chỉ probe P3 đỏ:
`/auth/sign-up :: Error: <path> attribute d: Expected arc flag ('0' or '1'), "… 19.876 0 1 1 0 .696 10.75 10.75…"` (×4).
`/auth/forgot-password` không lộ lỗi vì icon mắt chưa render ở bước đầu.
Hướng sửa: thêm E2E (hoặc mở rộng ca M1) mở `/auth/sign-up` với listener console. Có thể trích `EyeIcon` dùng chung
cho 3 form để chỉ còn một chỗ.

### Ghi chú (không chặn)

- **N1 — Không đồng bộ giữa tab.** Đổi sang English ở tab A thì tab B đang mở vẫn `lang="vi"` cho tới khi reload
  (probe P10: `tab2 lang after tab1 switch: vi`). Spec không yêu cầu; có thể nghe sự kiện `storage` nếu muốn.
- **N2 — Một lần `document-title` (serious) trên `/waiting QUEUED | light`.** Xuất hiện 1 lần trong lượt chạy có đột
  biến; đột biến không liên quan tới `<title>`. Bài quét chạy ngay sau client navigation `router.push` sang
  `/waiting/[eventId]` (`generateMetadata`, Next 15 stream metadata), nên `<title>` có thể chưa được gắn. Không tái
  hiện trên code gốc: 330 lần (full ×10) + 80 lần (luồng mua vé ×40, 8 worker) + 50 lần (×25, 12 worker), tổng 0 lần.
  Nếu muốn chắc chắn: trong `prepareForScan` chờ `expect(page).toHaveTitle(/EvenFlow/)` trước khi quét.

## 2. Tấn công không phá được

| # | Tấn công | Cách làm | Kết quả |
|---|---|---|---|
| A1 | Đảo V1 (`event-card` `border-dashed` → `opacity-70`) | Đột biến + build | axe đỏ: `[/events \| light/dark] color-contrast (serious) — 6 node` (`.opacity-70 > …`) |
| A2 | Đảo V2 (`ticket-tier-card` `opacity-60`, `h3 text-fg`) | Đột biến + build | axe đỏ: `[/checkout sau giu ve \| light] color-contrast x4`, `dark x2` |
| A3 | Đảo V3 (`text-warning-fg` → `text-accent`) | Đột biến + build | axe đỏ: `[/checkout/payment \| light] color-contrast x1 (.font-bold)`. Dark không đỏ, khớp baseline (V3 chỉ light) |
| A4 | Đảo V4 (bỏ `role="img"`) | Đột biến + build | axe đỏ: `[/checkout/result \| light/dark] aria-prohibited-attr x2` |
| A5 | Đảo I1 (`aria-labelledby` cứng + bỏ `id` của h2) | Đột biến + build | E2E không đỏ (axe chỉ xếp `incomplete`, đúng như baseline ghi). Unit `ticket-tier-list.test.tsx` / `[slug]/page.test.tsx` là test giữ lại, chấp nhận được |
| A6 | Bỏ `lang="vi"` trên `<main>` của PublicShell | Đột biến + build | `i18n.spec.ts:111` đỏ (`main#main` lang = "") |
| A7 | Bỏ `lang={locale}` ở nhánh nút vào phòng chờ | Đột biến + build | `i18n.spec.ts:170` đỏ (không thấy panel `lang=en`) |
| A8 | Bỏ `documentElement.lang = stored` khi khôi phục | Đột biến + build | 9 test i18n/a11y-en đỏ (`html lang` mãi là "vi") |
| A9 | Bỏ `lang="vi"` ở `app/auth/layout.tsx` | Đột biến + build (vòng 2) | `i18n.spec.ts:259` đỏ (`ownLang(h1)` = null) |
| A10 | Bỏ `lang="vi"` ở tiêu đề WorkspaceShell | Đột biến + build (vòng 2) | `i18n.spec.ts:272` đỏ |
| A11 | Bỏ `lang="vi"` ở vùng toast và trang `/unauthorized` | Đột biến, `vitest` | E2E không phủ, nhưng unit đỏ: toast 1, unauthorized 2 |
| A12 | Đảo path SVG ở `LoginForm` | Đột biến + build | `i18n.spec.ts` › M1 sign-in đỏ qua listener console (`Expected arc flag`) |
| A13 | Hard reload deep page khi đã lưu en: `/auth/sign-up`, `/auth/forgot-password`, `/auth/sign-in`, `/unauthorized`, `/events`, `/ops/events`, 2 trang sự kiện, `/checkout/…/payment`, `/checkout/…/result` | Probe P3, P11 (listener `console.error` + `pageerror`) | 0 lỗi console, không lỗi hydration; `html lang=en`, `main lang=vi`, skip link "Skip to main content" |
| A14 | Client navigation khi đang en: `/` → Events → chi tiết → Back → đổi vi → Forward | Probe P4 | `lang` và chữ luôn khớp locale; biến `window` còn nguyên (không reload); nút "Join the waiting room" / "Vào phòng chờ mua vé" đúng locale |
| A15 | localStorage ném lỗi trong trình duyệt thật (`Storage.prototype.getItem/setItem` throw; getter `window.localStorage` throw) | Probe P5, P5b | Đổi ngôn ngữ vẫn chạy, `html lang=en`, 0 lỗi console |
| A16 | Chỉ dùng bàn phím | Probe P7 | Thứ tự Tab: skip link → logo → "Tiếng Việt" → "English" → theme → nav. Enter/Space đổi ngôn ngữ, focus ở lại nút vừa bấm, Shift+Tab đúng; focus ring là box-shadow 3px |
| A17 | Tên truy cập (aria snapshot) ở 320/700/1280, vi/en | Probe P8 | ≥md: `group "Ngôn ngữ"/"Language"` › `button "Tiếng Việt" [pressed]`, `button "English"`. <md: "Tiếng Việt VI"/"English EN" (chứa chữ hiển thị). Theme: "Toggle light/dark theme": Light. Menu: "Open navigation menu" |
| A18 | Tràn ngang ở 320/700/1280 × light/dark × vi/en trên 8 trang (`/`, `/events`, 2 trang sự kiện, sign-in, sign-up, `/ops/events`, `/unauthorized`) + menu mobile mở | Probe P6 (`scrollWidth <= innerWidth`) | 0 tràn ở cả 4 tổ hợp. Header `/` cao 149px (320), 65px (700, một hàng), 113px (1280, có hàng nav) |
| A19 | Còn chuỗi tiếng Việt chưa dịch trên khung đã dịch (header, footer, skip link; text + `aria-label`/`title`/`alt`) khi en | Probe P9 trên 5 trang công khai | Không còn chuỗi nào (ngoài F1, nằm trong panel) |
| A20 | ISR | `next build` (4 lần) | `├ ○ /events  210 B  111 kB  1m  1y` và `├ ● /events/[slug]  3.87 kB  111 kB  1m  1y` (3 slug được prerender) |
| A21 | Flaky | Full E2E `--repeat-each=10 --retries=0` | **330 passed (1.8m)**. Thêm luồng mua vé ×40 (8 worker): 80 passed |
| A22 | Lỗi môi trường khi tải nặng | ×25, 12 worker | 344 passed, 6 fail đều do `net::ERR_NETWORK_CHANGED` của Windows (1 ca hệ quả: JS không tải nên `lang` không đổi). Lỗi môi trường, không phải lỗi code |

## 3. Kết quả E2E

| Lượt | Code | Kết quả |
|---|---|---|
| Full, 1 lần | gốc | 33 passed (17.5s) |
| Full `--repeat-each=10 --retries=0` | gốc | **330 passed** (1.8m), 0 flaky |
| Full + probe, đột biến vòng 1 (V1–V4, I1, main/panel lang, provider, SVG ×3) | đột biến | 28 failed / 27 passed (đúng mong đợi, xem §2) |
| Full + probe, đột biến vòng 2 (auth layout, workspace title, toast, unauthorized, SVG sign-up/forgot) | đột biến | 5 failed: 2 test đã commit (A9, A10) + 3 probe (F1, F2, F3) |
| Full, sau khôi phục và build lại | gốc | 33 passed (12.6s) |

## 4. Đề nghị

1. Sửa F1 (bắt buộc), thêm E2E en cho sự kiện SCHEDULED (`evenflow-grand-concert-2026`).
2. Sửa F2 và thêm F3 vào test (có thể làm cùng đợt).
3. N1, N2: tuỳ chọn.

## 5. Khôi phục cây làm việc

- Mọi file bị đột biến được chép lại từ bản sao lưu; `md5sum -c` khớp 16/16 file.
- `git diff` cộng md5 của các file untracked (trừ file này) cho cùng một md5 với lúc bắt đầu
  (`5823ef8e…`).
- Đã xoá probe `tests/e2e/specs/zz-challenge.spec.ts` và `tests/e2e/test-results/`.
- Đã dừng server (chỉ PID của mình). Không commit, không push.
- File mới duy nhất: `docs/workflow/EV-188-a11y-i18n/challenge.md`.
