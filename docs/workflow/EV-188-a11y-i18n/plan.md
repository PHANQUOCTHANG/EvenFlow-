# EV-188 — Kế hoạch thực thi (Plan)

Đầu vào: `spec.md` (AC-1..AC-10). Worktree `D:/Documents/Nam4-1/Project1/EvenFlow-188`, nhánh xếp trên EV-187
(`809e942`). Baseline unit: 1767 test / 101 file. E2E hiện có: smoke, buy-flow, ops-war-room.

Thứ tự công việc: **A11y trước** (bài quét có thể phát hiện lỗi trong chính chuỗi/nút sẽ thêm ở phần i18n, nên
bài quét chạy lại ở cuối trên toàn bộ thay đổi).

## 1. Hợp đồng chốt trước

### 1.1 Từ điển — `apps/web/src/lib/i18n/messages.ts` (mới)

```ts
export type Locale = "vi" | "en";
export const LOCALES: readonly Locale[] = ["vi", "en"];
export const DEFAULT_LOCALE: Locale = "vi";
export const LOCALE_STORAGE_KEY = "eventflow-locale";

/** Nguon: tieng Viet. Khoa phang dang "nhom.ten". */
export const vi = { "skipLink.label": "Bỏ qua điều hướng, tới nội dung chính", /* ... */ } as const;
export type MessageKey = keyof typeof vi;
export const en: Record<MessageKey, string> = { /* du khoa */ };
export const MESSAGES: Record<Locale, Record<MessageKey, string>>;

/** Thay {ten} bang params.ten; thieu khoa o en -> roi ve vi; locale la -> vi. */
export function translate(locale: Locale, key: MessageKey, params?: Record<string, string | number>): string;
export function isLocale(value: unknown): value is Locale;
```

Giá trị `vi` phải **trùng nguyên văn** chuỗi đang có trong component (test hiện có giữ nguyên).

### 1.2 Provider — `apps/web/src/components/i18n/` (mới)

```ts
// locale-provider.tsx ("use client")
export function LocaleProvider({ children }: { children: React.ReactNode }): JSX.Element;
export function useLocale(): { locale: Locale; setLocale: (next: Locale) => void };
export function useT(): (key: MessageKey, params?: Record<string, string | number>) => string;
// t.tsx ("use client") — de server component hien chuoi da dich ma khong thanh client component
export function T({ k, params }: { k: MessageKey; params?: Record<string, string | number> }): JSX.Element; // render text
// language-toggle.tsx ("use client")
export function LanguageToggle(): JSX.Element;
```

- State khởi tạo `DEFAULT_LOCALE` (server và render đầu của client giống nhau — AC-7). Sau mount: đọc
  `localStorage[LOCALE_STORAGE_KEY]` (try/catch, giá trị lạ bỏ qua) rồi áp dụng.
- `setLocale`: cập nhật state, ghi localStorage (nuốt lỗi), đặt `document.documentElement.lang`. Không điều
  hướng, không `router.refresh()`.
- Dùng ngoài provider: `useT()` trả hàm dịch theo `vi`, không ném lỗi (để unit test hiện có của component render
  không cần bọc provider).
- `LanguageToggle`: một `<button type="button">`, chữ hiển thị là ngôn ngữ **sẽ chuyển sang** ("English" với
  `lang="en"` khi đang vi; "Tiếng Việt" với `lang="vi"` khi đang en), `aria-label` từ từ điển ("Chuyển sang tiếng
  Anh" / "Switch to Vietnamese").
- `app/layout.tsx`: bọc `LocaleProvider` quanh `ToastProvider`. Giữ `<html lang="vi" suppressHydrationWarning>`.
  Không `cookies()`.

### 1.3 Chuỗi được chuyển vào từ điển (AC-8)

`components/layout/skip-link.tsx`, `public-header.tsx` (nút menu, `aria-label` nav, thêm `LanguageToggle` cạnh
`ThemeToggle`), `public-footer.tsx`, `focus-shell.tsx` (nếu có chuỗi), `nav-link.tsx` + `nav-config.ts`
(`NavItem` thêm `labelKey?: MessageKey`, chỉ đặt cho `PUBLIC_NAV`; `NavLink` hiện `t(labelKey)` nếu có, ngược lại
`label`), `components/theme/theme-toggle.tsx`, `components/event/event-action-panel.tsx`.

### 1.4 Bài quét axe — `tests/e2e`

- `tests/e2e/package.json`: thêm `@axe-core/playwright` (pin bản chính xác), cập nhật `package-lock.json`.
- `tests/e2e/fixtures/buy-flow-helpers.ts` (mới): chuyển các helper giả lập/điều hướng của `buy-flow.spec.ts`
  (`mockQueue`, `mockHoldCreated`, `queueUntilCheckout`, `waitForHydration`, chọn hạng vé...) ra dùng chung;
  `buy-flow.spec.ts` import lại, **hành vi không đổi**.
- `tests/e2e/fixtures/axe.ts` (mới): `expectNoSeriousA11yViolations(page, label)` — chạy AxeBuilder với tag ở
  spec AC-1, đính kèm toàn bộ kết quả làm attachment, fail với danh sách `impact` serious/critical (rule id,
  selector, help URL).
- `tests/e2e/specs/a11y.spec.ts` (mới): các trạng thái ở AC-1, mỗi trạng thái ở `light` và `dark`
  (`page.emulateMedia({ colorScheme })` + đặt `localStorage["eventflow-theme"]` qua `addInitScript`).
- `tests/e2e/specs/i18n.spec.ts` (mới): AC-6, AC-7 (đổi ngôn ngữ, kiểm `lang`, chữ đổi, cờ `window` còn, URL
  không đổi; reload → vẫn `en`; không lỗi console).

### 1.5 Sửa lỗi a11y

Danh sách lỗi chỉ biết sau khi chạy bài quét trên code hiện tại (bước baseline). Mỗi lỗi: sửa ở component/token
dùng chung nếu có thể, ghi vào `traceability.md` (rule id → file sửa → test giữ lại).

## 2. Thứ tự

1. Test design (agent riêng): unit test cho `messages.ts`, provider/hook/`T`/`LanguageToggle`, NavLink `labelKey`;
   `a11y.spec.ts`, `i18n.spec.ts`, `fixtures/axe.ts`, tách `buy-flow-helpers.ts`; chạy bài quét trên code hiện tại →
   `baseline.md` gồm **danh sách vi phạm axe thật** theo trang/theme.
2. Implementation: i18n theo §1.1–1.3; sửa lỗi a11y theo danh sách baseline.
3. Gates, code review, challenge, verify như EV-187.

## 3. File được phép đổi

`apps/web/src/lib/i18n/**`, `apps/web/src/components/i18n/**`, `apps/web/src/app/layout.tsx`, các file ở §1.3 và test
cạnh chúng, file component/token cần sửa cho lỗi axe (liệt kê trong traceability), `tests/e2e/**` (trừ
`playwright.config.ts` nếu không cần), `docs/workflow/EV-188-a11y-i18n/**`, `docs/kiem-thu/README.md`.

## 4. Rủi ro

| Rủi ro | Giảm thiểu |
|---|---|
| Đổi chuỗi làm vỡ test hiện có | Giá trị `vi` trùng nguyên văn; `useT` ngoài provider vẫn chạy |
| Lỗi hydration do locale | Render đầu luôn `vi`, áp locale sau mount; E2E bắt lỗi console |
| Làm trang ISR thành dynamic | Không `cookies()`; verify kiểm ký hiệu route trong output build |
| Sửa a11y đụng component của bạn khác | Sửa tối thiểu, ưu tiên token/thành phần dùng chung; ghi rõ file |
| Bài quét flaky do animation/trạng thái chưa ổn định | Quét sau khi trạng thái đích hiện ra (`expect` visible), `reducedMotion: "reduce"` |
| Lượng lỗi axe quá lớn | Nếu > 3 vòng sửa, dừng và báo người dùng kèm danh sách |

## 5. Rollback

Revert merge commit. Không có migration; khoá localStorage mới có thể bỏ qua an toàn.

## 6. Sửa sau plan review vòng 1 (`plan-review.md`: PASS WITH CHANGES)

| Finding | Quyết định (thay §1 khi khác) |
|---|---|
| M1 nút ngôn ngữ | **Nhóm hai nút**: `<div role="group" aria-label={t("language.group")}>` ("Ngôn ngữ" / "Language") chứa hai `<button type="button" aria-pressed>`: "Tiếng Việt" (`lang="vi"`) và "English" (`lang="en"`). **Không** `aria-label` trên nút — tên lấy từ chữ hiển thị (WCAG 2.5.3). Bấm nút đang chọn: không làm gì. Focus ở lại nút vừa bấm. |
| M2 `lang` | `<main>` trong `PublicShell`, `FocusShell`, `WorkspaceShell` có `lang="vi"` cố định (nội dung chưa dịch là tiếng Việt). Gốc `EventActionPanel` có `lang={locale}`. `<html lang>` = locale (đổi qua `setLocale`). E2E kiểm cả hai. |
| M3 reflow | `a11y.spec.ts` thêm khối viewport 320×640: quét `/` khi menu đóng và mở; assert `document.documentElement.scrollWidth <= window.innerWidth`. Nếu tràn: sửa header (vd. rút gọn chữ nút ngôn ngữ trên mobile thành "VI"/"EN" nhưng vẫn giữ tên truy cập chứa chữ hiển thị). |
| M4 helper | Tách `joinQueueFromEvent(page, api)` (tới QUEUED, cổng admit chưa mở) và `admitToCheckout(page, gate)`; `buy-flow.spec.ts` gọi cả hai, hành vi không đổi. Mỗi theme chạy **một** luồng đầy đủ: theme đặt bằng `addInitScript` ghi `localStorage["eventflow-theme"]` + `emulateMedia({ colorScheme })`; **không** đổi theme giữa trang. |
| M5 sign-in | Trong bài quét `/auth/sign-in`: `page.route` **abort** `https://accounts.google.com/**` và `https://grainy-gradients.vercel.app/**` (bên thứ ba, phụ thuộc mạng). Ghi lý do trong traceability. Trang ép `data-theme="light"` → quét dark vẫn chạy, ghi chú kết quả gần như light. |
| m1 | Giữ upload report khi fail (không sửa `e2e-web.yml`). Vi phạm moderate/minor và `incomplete` được **in ra log** test (`console.log` dạng bảng ngắn) và đính kèm attachment. |
| m2 | `i18n.spec.ts` fail khi có **bất kỳ** `console.error` / `pageerror`; listener gắn trước `goto` đầu tiên; có ca chạy với `en` đã lưu sẵn. Ghi nhận chấp nhận: người dùng `en` thấy chữ `vi` chớp trước khi mount. |
| m3 | Provider: khôi phục locale lúc mount đặt `documentElement.lang`, **không** ghi lại localStorage; hàm từ `useT` memo theo locale; ngoài provider `setLocale` là no-op (test `LanguageToggle` phải bọc provider). Provider và toggle **không** dùng `useRouter` / `useSearchParams`. |
| m4 | Unit test mới xoá `eventflow-locale` và đặt lại `<html lang>` trong `afterEach`; ca localStorage ném lỗi stub `Storage.prototype` như `theme-script.test.tsx:41-48`. |
| m5 | Test: mọi mục `PUBLIC_NAV` có `translate("vi", labelKey) === label`. Badge "Sắp có" (`nav-link.tsx`) đưa vào từ điển. |
| m6 | `FocusShell` chỉ có tên thương hiệu → chỉ thêm `lang="vi"` cho `<main>`. `skip-link`, `public-footer`, `public-shell` vẫn là server component (dùng `<T>`). Evidence AC-7 chép dòng `/events`, `/events/[slug]` từ output `next build`. |
| m7 | `@axe-core/playwright` pin **4.10.2**; chỉ đổi `tests/e2e/package.json` + `tests/e2e/package-lock.json`. **Cấm** đổi `package-lock.json` ở gốc repo. |
| m8 | Không dùng `test.skip`/`fixme` cho ca nào (gate hygiene). |
| m9 | Trước mỗi lần quét: chờ hydrate; trang có `ThemeToggle` chờ `aria-pressed` khớp theme; `page.mouse.move(0, 0)`. |
| m10 | Thêm quét với locale `en` trên `/` và `/events/vietnam-tech-summit-2026`. |
| m11 | Base cập nhật `809e942`. |
| File được phép thêm | `components/layout/{public-shell,focus-shell,workspace-shell}.tsx` + test, `components/layout/index.ts` nếu thêm export. |

## 7. Sửa sau code review vòng 1 (`code-review.md`: FAIL, 1 major)

| Finding | Quyết định |
|---|---|
| M1 | Mọi vùng chưa dịch nằm **ngoài** `<main>` của ba shell phải có `lang="vi"`: thêm `app/auth/layout.tsx` (bọc nội dung `/auth/**` trong phần tử `lang="vi"`, không đổi giao diện), trang `app/unauthorized/page.tsx`, phần khung của `WorkspaceShell` (tiêu đề, nav, menu — toàn bộ gốc shell trừ phần đã dịch nếu có), vùng toast (`components/ui/toast.tsx`). Test: unit cho từng chỗ; E2E với `en` đã lưu: nội dung `/auth/sign-in` và `/ops/events` nằm trong `[lang="vi"]`. |
| m1 | Nhãn ngắn "VI"/"EN" **không** `aria-hidden`; tên truy cập trên mobile là "Tiếng Việt VI" / "English EN" (vẫn chứa chữ hiển thị). Bỏ `lang` thừa nếu vô tác dụng. Test kiểm tên chứa chữ hiển thị ở cả hai kích thước. |
| m2 | Quét axe locale `en`: chờ nhóm "Language" hiện và (trang sự kiện) nút "Join the waiting room" hiện, nút tiếng Việt biến mất, rồi mới quét. |
| m3 | `TicketTierList` nhận prop `headingId?: string` (mặc định `"ticket-tiers-heading"`), trang `[slug]` dùng chung một hằng; giữ điều kiện "có hạng vé" ở một chỗ. |
| m4 | en: `"Skip to main content"`; thống nhất "Sale"/"Sales" theo một cách (danh từ đợt bán: "sale"; trang doanh số: "Sales"). |
| m5 | `translate` dùng `Object.hasOwn(params, name)`. |
| m6 | Header: nhãn ngắn VI/EN dùng tới breakpoint `md` (thay `sm`) để không xuống hàng ở 640–767px; E2E thêm kiểm tra chiều cao header không quá một hàng ở 700px. |
| m7 | Làm ở bước verify: traceability §5 thành danh sách nợ theo file, cột kết quả, dòng route `next build` cho AC-7. |
| File được phép thêm | `app/auth/layout.tsx` (mới), `app/unauthorized/page.tsx`, `components/ui/toast.tsx`, `components/event/ticket-tier-list.tsx` + test của chúng. |
