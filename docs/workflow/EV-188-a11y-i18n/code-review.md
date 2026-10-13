# EV-188 — Code review

Người review: agent review riêng, context sạch. Ngày 2026-10-13. Đầu vào: `spec.md`, `plan.md` (§6 thay §1),
`plan-review.md`, `baseline.md`, `traceability.md` và diff chưa commit trên worktree `EvenFlow-188`
(nhánh `feature/EV-188-a11y-i18n`, nền `809e942`). Đường dẫn tính từ `apps/web/src/` nếu không ghi khác.

## Kết luận: **FAIL** (1 major, 7 minor)

Phần lớn thay đổi đúng hợp đồng và chất lượng tốt. Phải sửa M1 trước khi merge; các minor có thể sửa cùng đợt
hoặc ghi nợ.

## Gate đã chạy lại

| Lệnh | Kết quả |
|---|---|
| `cd apps/web && npx vitest run` | **106 file / 1914 test xanh** |
| `cd apps/web && npx tsc --noEmit` | sạch |
| `npx eslint` trên `components/{i18n,layout,theme,event,checkout}`, `lib/i18n`, `app/layout.tsx`, `app/(marketing)/events/[slug]` | 0 lỗi, 0 cảnh báo |
| `tsc -p tests/e2e` | sạch |
| `git status` lockfile gốc `package-lock.json` | **không đổi**; chỉ `tests/e2e/package.json` + `tests/e2e/package-lock.json` đổi (`@axe-core/playwright` pin `4.10.2`) |
| Test hiện có bị sửa? | `git diff` của các file `*.test.*`: dòng xoá **chỉ** là dòng import (thêm `afterEach`/`waitFor`/`vi`); không khẳng định cũ nào bị đổi |

(Không chạy build/E2E theo phạm vi review.)

## Đã kiểm và đạt

- **Từ điển** (`lib/i18n/messages.ts`): 23 khoá; mọi giá trị `vi` trùng nguyên văn chuỗi cũ trong diff (skip link,
  menu, nav label, footer ×2, nav ×3, theme ×3, panel ×9). `en: Record<MessageKey, string>` chặn thiếu/thừa khoá khi
  viết literal; test so khoá và so tập tham số `{…}`. Interpolation `{seconds}` dùng đúng ở panel jitter;
  thiếu khoá `en` → `vi`; locale lạ → `vi`.
- **Provider** (`components/i18n/locale-provider.tsx`): state khởi tạo `vi` (server và render đầu client giống nhau);
  khôi phục trong `useEffect` có try/catch, bỏ giá trị lạ, đặt `documentElement.lang` và **không** ghi lại storage
  (:36-45); `setLocale` ổn định (`useCallback`), ghi storage nuốt lỗi; `value` và hàm `useT` memo theo locale; context
  mặc định cho ngoài provider dịch `vi` + `setLocale` no-op. Không `useRouter`/`useSearchParams`/`cookies()`/`headers()`.
- **ISR / server component**: `app/layout.tsx` chỉ bọc `LocaleProvider` (client) — không đọc request; `skip-link.tsx`,
  `public-footer.tsx`, `public-shell.tsx` không có `"use client"`, dùng `<T>` (client, render chữ trần).
  `<html lang="vi" suppressHydrationWarning>` giữ nguyên. Không thấy nguồn lệch hydration.
- **LanguageToggle**: `role="group"` có tên từ từ điển ("Ngôn ngữ"/"Language"), hai `<button type="button">` có
  `aria-pressed`, không `aria-label`, `lang` đúng trên từng nút và không đổi theo locale; bấm nút đang chọn là no-op;
  focus giữ (cùng phần tử DOM, key ổn định). Ở ≥sm tên truy cập = chữ hiển thị.
- **`lang`**: `<main lang="vi">` ở `PublicShell`, `FocusShell`, `WorkspaceShell`; gốc mọi nhánh của `EventActionPanel`
  có `lang={locale}` (6/6 nhánh return).
- **ThemeToggle**: nhãn `en` "Toggle light/dark theme" chứa chữ hiển thị "Light"/"Dark" (2.5.3), `vi` giữ nguyên.
- **Sửa a11y ở gốc**: V1 (`event-card.tsx:142,151`) bỏ `opacity-70` cả thẻ → viền nét đứt + chỉ ảnh mờ/grayscale;
  trạng thái hết vé vẫn rõ bằng badge "Hết vé" + ghi chú chữ + nút panel. V2 (`ticket-tier-card.tsx:83-90`) bỏ
  `opacity-60` → viền nét đứt + tên `text-fg-muted`; Alert cảnh báo trong thẻ khoá giờ đọc đủ tương phản (cải thiện
  quan trọng). V3 (`payment-handoff-view.tsx:222-223`) `text-accent` → `text-warning-fg`, đúng quy ước `tokens.css`.
  V4 (`order-result-view.tsx:154-156`) `role="img"` cho khối QR. I1: `aria-labelledby` chỉ đặt khi có hạng vé
  (`events/[slug]/page.tsx:124-126`) + `id` trên `h2` (`ticket-tier-list.tsx:45`). Các sửa đều theo token dùng chung,
  áp dụng cả hai theme; thay đổi thị giác chấp nhận được.
- **Header mobile** (`public-header.tsx:45-53`): `h-16` → `min-h-16 py-sm flex-wrap`. Desktop: 8+44+8 = 60px < 64px
  nên chiều cao vẫn 64px, một hàng như cũ. Không test hiện có nào khẳng định `h-16`.
- **`ticket-tier-list.tsx` ngoài danh sách §1.3**: hợp lệ theo plan §3 ("file component/token cần sửa cho lỗi axe,
  liệt kê trong traceability") — đã liệt kê ở traceability §3 dòng I1; thay đổi tối thiểu (một `id`). Xem m3 về thiết kế.
- **Fixture axe** (`tests/e2e/fixtures/axe.ts`): đúng 4 tag; lọc `serious`/`critical`; thông báo gồm nhãn trang, rule
  id, impact, selector từng node, help URL; không `disableRules`/`exclude`; attachment JSON gồm `violations` +
  `incomplete`; `expect.soft` hợp lý (một luồng nhiều trang báo đủ, test vẫn fail cuối).
- **Spec**: không `waitForTimeout`/sleep/`.skip`/`.only`/`.fixme`; chờ bằng `expect`, cổng `admit`, `fonts.ready`;
  `reducedMotion: "reduce"`; theme đặt trước khi tải trang. Chặn Google/grainy-gradients chỉ trên `/auth/sign-in`, có
  lý do ở traceability §4 — chấp nhận (bên thứ ba, phụ thuộc mạng, không phải code dự án). `i18n.spec.ts` fail với
  mọi `console.error`/`pageerror`, kiểm marker `window` + URL + 0 `framenavigated`, ISR qua `Cache-Control`.
- **Tách helper** `fixtures/buy-flow-helpers.ts`: `buy-flow.spec.ts` gọi `joinQueueFromEvent` + `admitToCheckout`;
  baseline 6/6 xanh sau khi tách.

## Phát hiện

### Major

**M1 — `<html lang="en">` gắn sai cho nội dung tiếng Việt ngoài ba shell (hồi quy WCAG 3.1.1 / 3.1.2).**
Bằng chứng: `locale-provider.tsx:43,49` đặt `documentElement.lang` cho **mọi** route; chỉ `<main>` của 3 shell được
đánh dấu `lang="vi"`. Các vùng sau toàn tiếng Việt nhưng nằm dưới `html[lang=en]` khi người dùng đã chọn English
(lựa chọn được nhớ và áp dụng ở mọi trang):
- `app/auth/sign-in/page.tsx:10`, `app/auth/sign-up/page.tsx`, `app/auth/forgot-password/page.tsx`,
  `app/unauthorized/page.tsx:10` — không dùng shell nào, **cả trang** tiếng Việt bị khai báo là tiếng Anh;
- `components/layout/workspace-shell.tsx:59-89` — tiêu đề "Vận hành"/"Nhà tổ chức", `nav aria-label="Điều hướng
  workspace"`, link sidebar, nút menu (ngoài `<main>`);
- `components/ui/toast.tsx:85-89` — vùng toast (`aria-live`) ở cấp root, nội dung toast của form auth tiếng Việt.
Trước thay đổi `lang="vi"` luôn đúng; giờ trình đọc màn hình đọc tiếng Việt bằng giọng Anh trên trang đăng nhập —
ngược mục tiêu ticket. axe không bắt được (không có rule so ngôn ngữ thực), bài quét sign-in cũng không chạy với `en`.
Sửa: đánh dấu `lang="vi"` cho mọi vùng chưa dịch, ví dụ thêm `app/auth/layout.tsx` (hoặc wrapper của từng trang auth)
và wrapper `/unauthorized` có `lang="vi"`; trong `WorkspaceShell` đặt `lang="vi"` trên header-title/nav/nút menu (giữ
SkipLink và ThemeToggle theo locale); vùng toast `lang="vi"` (nội dung toast hiện đều tiếng Việt). Mở rộng danh sách
file được phép trong plan/traceability. Thêm test giữ lại: E2E `i18n.spec.ts` với `en` đã lưu → `/auth/sign-in` có
`h1` nằm trong tổ tiên `[lang="vi"]`; unit `workspace-shell.test.tsx` cho tiêu đề/nav.

### Minor

**m1 — Chữ ngắn "VI"/"EN" trên mobile chỉ khớp tên truy cập "tình cờ" theo chuỗi con (2.5.3).**
`components/i18n/language-toggle.tsx:13-14,42-45`: ở <sm chữ hiển thị là "VI"/"EN" (`aria-hidden`), tên truy cập là
"Tiếng Việt"/"English" (`sr-only`). "en" ⊂ "English" ở đầu từ nên ổn; "vi" chỉ là mảnh của "Việt" — Voice Control/
Dragon khớp theo từ, người dùng nói "bấm VI" sẽ không khớp. `lang` trên nút đúng; với chữ ngắn thì `lang` không có tác
dụng (đã `aria-hidden`, và "VI"/"EN" là mã ngôn ngữ chứ không phải từ của ngôn ngữ đó) — vô hại. Test
`language-toggle.test.tsx:95-105` chỉ kiểm nhãn đầy đủ (jsdom không áp CSS) nên không chứng minh gì cho chữ ngắn.
Sửa: bỏ `aria-hidden` ở span ngắn để tên ở mobile là "Tiếng Việt VI"/"English EN" (ở ≥sm span ngắn `display:none` nên
tên vẫn đúng chữ hiển thị); cập nhật truy vấn test sang `name: /^Tiếng Việt/` hoặc tương đương. Hoặc ghi rõ lựa chọn
hiện tại là chấp nhận trong traceability.

**m2 — Quét `locale en` chỉ chờ `html[lang=en]`, chưa chờ chữ đã dịch.**
`tests/e2e/specs/a11y.spec.ts:195,200` + `prepareForScan` (`:72-74`): `lang` được đặt ngay trong effect, còn re-render
chữ `en` commit sau; axe có thể quét lúc chữ vẫn `vi`, khiến lần quét "en" không thật sự quét giao diện `en`.
Sửa: khi `locale === "en"` chờ thêm `getByRole("group", { name: "Language" })` visible (và trên trang sự kiện chờ
nút panel không còn "Vào phòng chờ mua vé").

**m3 — `id="ticket-tiers-heading"` cứng trong component tái sử dụng, điều kiện nhân đôi ở page.**
`components/event/ticket-tier-list.tsx:45` + `app/(marketing)/events/[slug]/page.tsx:126`: page phải biết khi nào
component render `h2` (`tiers.length > 0` lặp lại logic `ticket-tier-list.tsx:34`); dùng component hai lần trên một
trang sẽ trùng id. Sửa (không bắt buộc ngay): để `TicketTierList` nhận `headingId` qua prop hoặc tự render
`<section aria-labelledby>` của mình; giữ hai test I1.

**m4 — Chất lượng/nhất quán bản dịch `en`.**
`lib/i18n/messages.ts:45` "Skip navigation, go to main content" → tự nhiên hơn "Skip to main content";
`:65,69,70` lẫn "Sales"/"Sale": "Sales closed" / "Sale starts" / "Sale not open yet" → thống nhất "Sales start" (hoặc
"Sales open"), "Sales not open yet", "Sales closed". Các chuỗi còn lại tự nhiên, thuật ngữ "waiting room" nhất quán.

**m5 — `translate` dùng toán tử `in` (dính prototype).**
`lib/i18n/messages.ts:90`: `{toString}` trong template sẽ ra mã nguồn hàm `Object.prototype.toString`. Hiện không có
khoá nào như vậy; đổi sang `Object.hasOwn(params, name)`.

**m6 — Header ở bề rộng 640–~660px xuống hai hàng.**
`public-header.tsx:47-53`: ở sm..md vẫn hiện nút menu ("Mở menu điều hướng") cùng nhãn ngôn ngữ đầy đủ; tổng ước
~625px > 608px khả dụng → bọc dòng (không tràn ngang, đúng 1.4.10). Chỉ là thị giác; cụm nút bên phải `flex-wrap` có thể
tách lẻ nút xuống dòng. Khuyên chụp màn hình ở 640px ở bước verify; nếu xấu, dùng nhãn ngắn tới `md` thay vì `sm`.

**m7 — Traceability chưa chốt.**
`traceability.md` §5 (dòng 76) vẫn là danh sách nợ theo glob, chưa "theo từng file" như AC-8 yêu cầu; cột "Kết quả"
AC-1..AC-9 còn "chưa chạy trên code mới"; bằng chứng dòng route `/events` (○) và `/events/[slug]` (●) từ `next build`
(AC-7) chưa có. Phải hoàn tất ở bước verify.

### Ghi chú (không cần sửa)

- V3 dùng `text-warning-fg` cho tổng tiền: đúng quy ước token (thay thế màu accent), dù ngữ nghĩa "warning" hơi lệch;
  `text-fg` cũng hợp lệ.
- `ThemeToggle` dùng nhãn cố định "Toggle light/dark theme" + `aria-pressed` — ngữ nghĩa "pressed" mơ hồ nhưng có từ
  trước EV-188, ngoài phạm vi.
- Người dùng `en` thấy chữ `vi` nháy trước khi mount — đánh đổi đã chấp nhận ở plan §6 m2.
