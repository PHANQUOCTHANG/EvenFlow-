# EV-188 — Review kế hoạch (Plan review)

Người review: agent review riêng, context sạch. Ngày 2026-10-13. Đầu vào: `spec.md`, `plan.md` (chưa commit) trên
worktree `EvenFlow-188`, HEAD `809e942`. Đã đối chiếu với code được trích dẫn (đường dẫn dưới đây tính từ `apps/web/src/`
nếu không ghi khác).

## Kết luận: **PASS WITH CHANGES**

Kế hoạch đúng hướng: phủ đủ AC-1..AC-10, hợp đồng i18n "render đầu luôn `vi`, áp locale sau mount" giữ được ISR
và tránh lỗi hydration, `T` là client component nhỏ nên SkipLink/footer vẫn là server component. Các khẳng định
về hiện trạng trong spec là đúng (`app/layout.tsx:24` có `lang="vi"` cứng; `events/page.tsx:6` và
`events/[slug]/page.tsx:14,20` là ISR; `lib/format.ts:1` không dùng ICU). Tuy vậy có 5 lỗ hổng mức major cần
sửa vào plan trước khi viết test: 2 lỗi nằm ngay trong thiết kế a11y của chính phần i18n (axe không bắt được),
1 lỗi chưa kiểm ở mobile, và 2 lỗi về cách đi tới trạng thái cần quét và độ ổn định khi quét.

## Bảng phủ AC

| AC | Plan phủ ở | Kiểm được? | Ghi chú |
|---|---|---|---|
| AC-1 | §1.4 `a11y.spec.ts` | Có, cần sửa | M4 (trạng thái QUEUED), M5 (`/auth/sign-in`), M3 (mobile) |
| AC-2 | §1.4 `fixtures/axe.ts` | Có | m1 (attachment chỉ thấy khi fail; thiếu `incomplete`) |
| AC-3 | §1.5 + traceability | Có | |
| AC-4 | `e2e-web.yml` sẵn có | Có | `workflow_dispatch.repeat_each` đã có (`e2e-web.yml:26-30`) |
| AC-5 | §1.1, §1.2 | Có | `en: Record<MessageKey,string>` bắt cả thiếu lẫn thừa khoá khi viết literal |
| AC-6 | §1.2 + `i18n.spec.ts` | Có | m3 (đường khôi phục khi mount phải đặt `lang`) |
| AC-7 | §1.2 + verify output build | Có | m2 (lỗi hydration ở production bị minify), m6 |
| AC-8 | §1.3 | Có | M2 (lang của phần chưa dịch), m5, m6 |
| AC-9 | §1.2 `LanguageToggle` | **Thiết kế hiện tại vi phạm WCAG** | M1 |
| AC-10 | Giá trị `vi` trùng nguyên văn, `useT` ngoài provider | Có | Đã rà test hiện có — xem mục "Test hiện có" |

## Phát hiện

### Major

**M1 — `LanguageToggle` vi phạm WCAG 2.5.3 Label in Name (mức A), và `lang` trên chữ bị vô hiệu.**
Plan §1.2 (plan.md:51-53): chữ hiển thị "English" (`lang="en"`), `aria-label` "Chuyển sang tiếng Anh". Tên truy cập
được không chứa chữ hiển thị "English" → người dùng điều khiển bằng giọng nói ("bấm English") không bấm được.
`aria-label` còn ghi đè nội dung nên `lang="en"` trên chữ không tới được screen reader. Rule axe tương ứng
(`label-content-name-mismatch`) là experimental, không bật theo tag → bài quét sẽ xanh giả. Thêm nữa, mẫu "hiện
ngôn ngữ đích" không nói rõ ngôn ngữ **hiện tại** như AC-9 yêu cầu; còn `aria-pressed` trên MỘT nút mà nhãn đổi
theo trạng thái là anti-pattern (đọc thành "English, đã bấm" — sai nghĩa).
Đối chiếu: `theme-toggle.tsx:14,23` đang làm đúng (nhãn "…sáng/tối" chứa chữ hiển thị "Sáng"/"Tối").
**Sửa plan** — chọn một:
- (a) Một nút, **không** `aria-label`; tên lấy từ nội dung:
  `<span className="sr-only">{t("language.switchTo")} </span><span lang="en">English</span>` → tên "Chuyển ngôn ngữ
  sang English" chứa chữ hiển thị; ngôn ngữ hiện tại suy ra từ `html[lang]`.
- (b) (khuyên dùng, khớp AC-9 "cho biết ngôn ngữ hiện tại") Nhóm 2 nút `role="group"` có `aria-label` từ từ điển,
  nút "Tiếng Việt" (`lang="vi"`) và "English" (`lang="en"`), mỗi nút `aria-pressed` = đang chọn.
Unit test giữ lại: tên truy cập chứa chữ hiển thị (cả `vi` lẫn `en`); focus vẫn ở nút sau khi bấm (không remount).

**M2 — Đặt `<html lang="en">` trong khi phần lớn trang vẫn tiếng Việt → hồi quy WCAG 3.1.1/3.1.2.**
AC-8 cố ý để lại phần lớn chuỗi là tiếng Việt (nợ). Khi bấm English, `setLocale` đổi `document.documentElement.lang`
(plan.md:47) → toàn bộ nội dung chưa dịch bị khai báo là tiếng Anh, screen reader đọc tiếng Việt bằng giọng Anh.
axe chỉ kiểm `html-has-lang`/`valid-lang`, không phát hiện được.
**Sửa plan:** đánh dấu phần chưa dịch là `lang="vi"`: `<main lang="vi">` trong `PublicShell` (`public-shell.tsx:154`),
`FocusShell` (`focus-shell.tsx:135`) và `WorkspaceShell` (dùng chung SkipLink); đảo mảnh đã dịch nằm trong `main`
(`EventActionPanel`) đặt `lang={locale}` ở phần tử gốc. `main lang="vi"` là thuộc tính tĩnh nên không ảnh hưởng
ISR. Thêm khẳng định E2E trong `i18n.spec.ts`: sau khi sang `en`, `main` có `lang="vi"`, panel có `lang="en"`.
Nếu không làm, phải ghi rõ thành nợ WCAG 3.1.2 trong traceability và được người dùng chấp nhận — không để ngầm.
Thêm các file shell vào danh sách file được phép.

**M3 — Header mobile chưa được kiểm, thêm nút thứ ba có nguy cơ tràn ngang (WCAG 1.4.10 Reflow).**
`public-header.tsx:57-75`: ở mobile header h-16 đã chứa logo + `ThemeToggle` (icon + chữ) + nút "Mở menu điều hướng"
(chữ dài, `md:hidden`). Ước lượng bề rộng hiện tại đã sát/qua 375px; thêm "Tiếng Việt" càng tệ. Mọi E2E chạy
`Desktop Chrome` (`tests/e2e/playwright.config.ts:25`) nên nút menu và trạng thái menu mở không bao giờ được quét;
axe không đo reflow.
**Sửa plan:** trong `a11y.spec.ts` thêm `test.describe` với `test.use({ viewport: { width: 320, height: 640 } })`
(không cần sửa `playwright.config.ts`): quét `/` khi menu đóng và mở, và khẳng định
`document.documentElement.scrollWidth <= window.innerWidth`. Nếu tràn: nhãn hiển thị gọn ("EN"/"VI") nhưng vẫn giữ
quy tắc của M1 (tên truy cập chứa chữ hiển thị).

**M4 — Không quét được trạng thái QUEUED với helper "hành vi không đổi"; chưa chốt cách đổi theme theo luồng.**
`queueUntilCheckout` (`tests/e2e/specs/buy-flow.spec.ts:108-127`) tự `admit.open()` ngay sau khi thấy rank, nên tách
nguyên văn sang `buy-flow-helpers.ts` thì `a11y.spec.ts` không có điểm dừng để quét `/waiting` khi QUEUED.
**Sửa plan:** tách thành `joinQueueFromEvent(page, api)` (tới QUEUED + rank, cổng còn đóng) và
`admitToCheckout(page, admit)`; `buy-flow.spec.ts` gọi nối tiếp hai hàm (hành vi giữ nguyên). Chốt: mỗi theme chạy
**một luồng riêng** với theme đặt bằng `addInitScript` trước khi tải trang (plan đã nêu) — **không** lật
`data-theme` giữa chừng vì `ThemeToggle`/nút có `transition-colors` (`theme-toggle.tsx:18`), quét lúc đang
chuyển màu cho kết quả contrast thất thường. Luồng: sự kiện → QUEUED (quét) → checkout trước giữ (quét) → sau giữ
(quét) → thanh toán (quét) → kết quả (quét); 401 là test riêng. Đặt `colorScheme` qua `test.use`/`emulateMedia`
**và** `localStorage["eventflow-theme"]` cho cả `light` lẫn `dark` (ThemeScript ưu tiên localStorage —
`theme-script.tsx:8-10`), để kết quả không phụ thuộc cài đặt máy runner.

**M5 — `/auth/sign-in` phụ thuộc bên thứ ba và mạng.**
`components/auth/LoginForm.tsx:275-313` bọc `GoogleOAuthProvider` + `GoogleLogin` → tải script và iframe từ
`accounts.google.com`; `NEXT_PUBLIC_GOOGLE_CLIENT_ID` không được đặt khi build trong `e2e-web.yml` (chỉ có
`NEXT_PUBLIC_API_BASE`) nên client id rỗng. `app/auth/sign-in/page.tsx:24` tải `grainy-gradients.vercel.app/noise.svg`.
AxeBuilder mặc định quét cả iframe → kết quả phụ thuộc mạng/Google (flaky, có thể ra vi phạm của bên thứ ba).
Trang không gọi API khi tải (chỉ gọi `loginUser` khi submit) → **không cần mock** endpoint, nhưng vẫn dùng fixture
`api` để bắt request lạ. Lưu ý thêm: wrapper đặt cứng `data-theme="light"` (`sign-in/page.tsx:12`) nên lần quét
"dark" của trang này gần như trùng lần "light" — vẫn chạy (đúng AC-1) nhưng ghi chú trong traceability.
**Sửa plan:** trong `a11y.spec.ts` `context.route` abort `https://accounts.google.com/**` và
`https://grainy-gradients.vercel.app/**` (hoặc `.exclude()` iframe GSI) — ghi lý do vào traceability theo AC-2.
Bước baseline phải ghi rõ đã chọn cách nào.

### Minor

**m1 — Báo cáo moderate/minor chỉ xem được khi CI fail.** `e2e-web.yml:118-126` chỉ upload `report` khi
`failure()`. Hoặc chấp nhận (ghi rõ trong evidence: attachment xem ở report cục bộ), hoặc đổi thành `if: always()` —
khi đó phải thêm `.github/workflows/e2e-web.yml` vào danh sách file được phép (AC-4 chỉ cấm sửa `ci.yml`). Attachment
nên gồm cả `incomplete` (contrast trên nền gradient/ảnh — vd. trang sign-in — rơi vào đây chứ không vào `violations`).

**m2 — Bắt lỗi hydration ở build production.** Production React chỉ log lỗi dạng minified (#418/#423…), không có chữ
"hydration". `i18n.spec.ts` phải fail với **mọi** `console.error`/`pageerror` (như `ops-war-room.spec.ts:14-19`),
gắn listener trước `goto` đầu tiên và giữ qua `reload`, chạy với locale đã lưu `en`. Ghi vào traceability rằng người
dùng `en` thấy chữ `vi` nháy sang `en` sau mount (đánh đổi đã chấp nhận ở AC-7).

**m3 — Hợp đồng provider cần nói rõ.** (a) Đường khôi phục khi mount phải đặt `document.documentElement.lang` (plan.md:44
chỉ nói "áp dụng") và không ghi lại localStorage. (b) Hàm trả về từ `useT()` memo theo `locale`. (c) Ngoài provider
`setLocale` là no-op — test của `LanguageToggle` phải bọc provider; test header hiện có không bọc vẫn chạy.

**m4 — Dọn trạng thái trong unit test.** `vitest.setup.ts` chỉ gọi `cleanup()`. Test mới phải xoá
`eventflow-locale` và trả `document.documentElement.lang` về `vi` ở `afterEach`. localStorage của jsdom dùng được
(test theme đang dùng); ca localStorage ném lỗi thì stub `Storage.prototype` như `theme-script.test.tsx:41-48`.

**m5 — `labelKey` trùng `label`.** `(marketing)/layout.test.tsx:77` và test header khẳng định `item.label`. Thêm unit
test: với mọi item của `PUBLIC_NAV`, `translate("vi", item.labelKey) === item.label`. Badge "Sắp có"
(`nav-link.tsx:190`) chưa có trong §1.3 — dịch luôn hoặc ghi nợ. `NavItem.labelKey?` là tuỳ chọn nên
`ORGANIZER_NAV`/`OPS_NAV`/`WorkspaceShell` không ảnh hưởng.

**m6 — `FocusShell` không có chuỗi nào ngoài thương hiệu** (`focus-shell.tsx:126-139`) — chỉ dịch qua `SkipLink`;
plan nên ghi rõ thay cho "nếu có chuỗi". Giữ `skip-link.tsx`, `public-footer.tsx`, `public-shell.tsx` **không**
`"use client"` (chỉ `T` là client). Evidence AC-7: chép dòng của `/events` và `/events/[slug]` trong output
`next build` (phải là `○`/`●` kèm revalidate, không phải `ƒ`).

**m7 — Phiên bản và lockfile.** Đã kiểm registry: `@axe-core/playwright` 4.10.2 (axe-core `~4.10.3`) và 4.13.0
(latest, axe-core `~4.13.0`) đều có peer `playwright-core >= 1.0.0` → tương thích `@playwright/test` 1.49.1
(`playwright-core` 1.49.1 có sẵn trong `tests/e2e/package-lock.json`). Pin chính xác bằng `npm install -D -E` trong
`tests/e2e`, chỉ đổi `tests/e2e/package.json` + `tests/e2e/package-lock.json`; **không** đụng `package-lock.json` ở gốc
(file lạ, nhưng nằm trong `paths` của `e2e-web.yml:13`).

**m8 — Hygiene grep của `e2e-web.yml:51`.** Helper/spec mới không được dùng `waitForTimeout(`, `new Promise(... setTimeout`,
hay `test.skip(`/`test.fixme(` (vd. không `skip` lần quét dark của sign-in — dùng vòng lặp/điều kiện thường).

**m9 — Độ ổn định khi quét.** Trước mỗi lần quét: chờ hydrate (dùng `waitForHydration`), với trang có `ThemeToggle` chờ
`aria-pressed` khớp theme (`use-theme.ts:16-21` khởi tạo `"light"` rồi mới đọc lại sau mount), đưa chuột ra góc
(`page.mouse.move(0, 0)`) để không quét trạng thái hover của nút vừa bấm. `reducedMotion: "reduce"` chỉ tắt
`ef-spin`/`ef-pulse` (`app/globals.css:50-55`), không tắt transition — ổn với cách đặt theme ở M4.

**m10 — Quét thêm locale `en`.** AC-1 quét bằng `vi`; chuỗi `en` mới (và nút ngôn ngữ ở trạng thái `en`) chưa được
quét. Thêm một lần quét `/` và `/events/vietnam-tech-summit-2026` với `localStorage["eventflow-locale"] = "en"`.

**m11 — Thông tin cũ / chi phí.** plan.md:4 ghi nền `73a7bca`; HEAD hiện là `809e942` (thêm evidence EV-187, chỉ docs).
Chi phí: ~8 trạng thái × 2 theme ≈ 16 lần quét + 2 luồng mua + mobile + `en` — ước tính dưới 2 phút mỗi lần chạy,
chấp nhận được trong `timeout-minutes: 20`; ghi thời gian thật vào evidence khi chạy `--repeat-each=5`.

## Test hiện có có thể vỡ

Đã rà `skip-link`, `public-header`, `public-footer`, `public-shell`, `nav-link`, `nav-config`, `theme-toggle`,
`event-action-panel` và test layout của các route group. Chúng khẳng định chuỗi tiếng Việt theo tên truy cập, số
lượng `nav`/`main`, thứ tự DOM toggle trước `nav` (`public-header.test.tsx:318-331`), Tab đầu tiên vào SkipLink
(`public-shell.test.tsx:161-171`). Không test nào đếm số nút trong header, nên thêm `LanguageToggle` không làm vỡ —
**miễn là** đặt nó trong cụm `div` cạnh `ThemeToggle` (trước `nav`, sau SkipLink). Các test mock `next/navigation`
chỉ với `usePathname` (`public-header.test.tsx:22-24`) hoặc `useRouter` (`event-action-panel.test.tsx:8-12`) → provider
và `LanguageToggle` **không được** gọi `useRouter`/`useSearchParams` (plan đã cấm `router.refresh()` — đúng). E2E
`buy-flow` chọn phần tử bằng tên tiếng Việt; mỗi test là context mới không có locale lưu → vẫn `vi`. Nếu làm M2,
không test nào khẳng định thuộc tính của `<main>` ngoài `id`/`tabindex`.

## Danh sách file được phép — cần bổ sung

- `apps/web/src/components/layout/public-shell.tsx`, `focus-shell.tsx`, `workspace-shell.tsx` và test cạnh chúng (M2).
- `apps/web/src/components/layout/index.ts` nếu export thêm.
- `.github/workflows/e2e-web.yml` — chỉ khi chọn upload report luôn (m1).
- Ghi rõ: `tests/e2e/package-lock.json` có, `package-lock.json` gốc **không** (m7).
