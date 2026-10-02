# EVF-1801 (phần 2) — Route groups + app shells — spec & plan

| | |
|---|---|
| Jira | EVF-1801, phần "Phạm vi công việc" bị cắt khỏi PR trước |
| Tier | **Standard** |
| Base | `feature/EVF-1801-design-system` (xếp tầng — merge sau PR đó) |
| Branch | `feature/EVF-1801-route-groups` |
| Nguồn | `docs/03-cau-truc-src.md` §6 (cấu trúc route), `docs/08-stitch-ui-ux-handoff.md` §4 (IA), §11.1 (shell), §5 (nguyên tắc UX) |

Sau PR này EVF-1801 còn thiếu **duy nhất** client API sinh từ OpenAPI, và phần đó **đang bị chặn bởi backend** (6/8 service Go vẫn là stub, chưa có spec OpenAPI).

---

## 1. Phạm vi

### Trong phạm vi

**A. 5 route group** theo `docs/03` §6, mỗi group có `layout.tsx` riêng:

| Group | Vai trò | Shell |
|---|---|---|
| `(marketing)` | Buyer, trang tĩnh/ISR phục vụ từ CDN | `PublicShell` — header + footer |
| `(queue)` | Buyer đang xếp hàng | Shell tối giản, **không** nav gây nhiễu |
| `(checkout)` | Buyer đang chọn vé / thanh toán | Shell tối giản (~~+ chỗ neo cho sticky summary~~ — **đã bỏ, xem §8.6**) |
| `(organizer)` | Organizer | `WorkspaceShell` — sidebar + topbar |
| `(ops)` | Ops | `WorkspaceShell` |

**B. Component layout dùng chung** trong `src/components/layout/`:

- `Container` — giới hạn chiều rộng. Public 1200px, workspace 1440px (handoff §6: "content width public 1120–1200px", DESIGN.md: "1200px luồng đặt vé, 1440px Ops Dashboard")
- `SkipLink` — bỏ qua nav, nhảy tới nội dung chính
- `PublicHeader` — logo, nav, `ThemeToggle`, menu mobile
- `PublicFooter`
- `PublicShell`
- `WorkspaceShell` — sidebar 240–264px + topbar (handoff §6), sidebar thu gọn trên mobile
- `NavLink` — đánh dấu route đang active
- `nav-config.ts` — danh sách nav theo vai trò

**C. Di chuyển trang chủ** vào `(marketing)/`

### Ngoài phạm vi

| Việc | Lý do |
|---|---|
| Client API từ OpenAPI | Chặn bởi backend |
| Page thật trong từng group (`events/[slug]`, `waiting/[eventId]`, `select/[eventId]`…) | Thuộc EVF-1802/1803/1804, cần API contract |
| `src/app/api/` (BFF) | Cần biết endpoint thật của gateway |
| Account menu có dữ liệu thật | Chưa có identity service (stub) |

## 2. Quyết định phải tuân thủ

**2.1 Không tạo link giả.** Handoff §5 nguyên tắc 3 "No fake certainty" và §7 "không tự suy diễn endpoints". Hiện **chưa có route nào** trong 5 group ngoài trang chủ. Vì vậy `nav-config.ts` gắn `ready: boolean` cho từng item:

- `ready: true` → render `<Link>` thật
- `ready: false` → render `<span aria-disabled="true">` kèm `Badge` "Sắp có", **không** phải `<Link>` dẫn tới 404

Đây là lý do kỹ thuật, không phải trang trí: `next build` sẽ không fail vì link chết, nhưng người dùng bấm vào sẽ ra 404 — tệ hơn là nói thẳng "chưa có".

**2.2 Nav theo vai trò là cấu hình, không phải quyền.** Handoff §11.1: "Do not invent permission links; annotate role-specific nav as configurable". `nav-config.ts` chỉ là **danh sách hiển thị**, không được hiểu là ma trận quyền. Phân quyền thật thuộc identity service. Phải ghi rõ trong comment để người sau không dùng nó làm authorization.

**2.3 `(queue)` và `(checkout)` cố tình KHÔNG có nav.** Handoff §6: "Queue/checkout: bình tĩnh, có trật tự, thông tin ưu tiên hơn trang trí"; §5 nguyên tắc 1 "Status first" và 2 "One primary action". Thêm nav vào 2 group này là mời người dùng rời luồng ở đúng lúc họ đang giữ vé có hạn. Shell của 2 group chỉ có logo + chỗ cho status, không có menu điều hướng.

**2.4 Trang chủ chuyển vào `(marketing)/` kèm cả file test.** `src/app/page.test.tsx` nằm trong nhóm "KHÔNG được sửa" của plan trước. Ở đây nó được **di chuyển cùng** `page.tsx` sang `(marketing)/`, **nội dung không đổi một ký tự** (`import Home from "./page"` vẫn giải đúng). Git sẽ hiển thị là rename. Assertion `<h1>EventFlow</h1>` vẫn phải pass.

## 3. AC

### AC-1 — Route group hoạt động, URL không đổi
- **Given** `npm run build`, **Then** build thành công và trang chủ vẫn ở `/` (route group không tạo thêm segment URL)
- **Then** `src/app/(marketing)/page.tsx` tồn tại, `src/app/page.tsx` không còn

### AC-2 — Mỗi group có layout đúng kiểu
- **Then** `(marketing)` render `PublicShell`: có `<header>`, `<main>`, `<footer>`
- **Then** `(organizer)` và `(ops)` render `WorkspaceShell`: có sidebar `<nav>` + topbar
- **Then** `(queue)` và `(checkout)` **không** render phần tử nào có `role="navigation"` hay `<nav>` chứa link điều hướng — chỉ logo + vùng nội dung
- **Then** mọi layout render đúng một `<main>`

### AC-3 — SkipLink
- **Given** load trang bất kỳ có shell, **When** Tab lần đầu, **Then** phần tử nhận focus là link "Bỏ qua điều hướng, tới nội dung chính" trỏ tới `#main`
- **Then** link bị ẩn khi chưa focus và hiện khi focus (không dùng `display: none`, vì như vậy bàn phím không tới được)
- **Then** phần tử `<main>` có `id="main"` và `tabIndex={-1}`

### AC-4 — NavLink đánh dấu route đang mở
- **Given** `usePathname()` trả `/organizer/events`, **Then** item `/organizer/events` có `aria-current="page"`, các item khác không có
- **Given** pathname là route con (`/organizer/events/123`), **Then** item cha `/organizer/events` vẫn `aria-current="page"`
- **Given** item `href="/"`, **When** pathname là `/organizer`, **Then** item `/` **không** active (khớp chính xác cho route gốc, tránh "/" luôn active)

### AC-5 — Không có link giả
- **Given** nav item có `ready: false`, **Then** **không** render thẻ `<a>`; render phần tử có `aria-disabled="true"` kèm nhãn text "Sắp có"
- **Given** nav item có `ready: true`, **Then** render `<a href>` đúng
- **Then** grep toàn bộ `src/components/layout`: mọi `href` xuất hiện đều phải nằm trong `nav-config.ts` hoặc trỏ tới `/` — không hard-code đường dẫn rải rác

### AC-6 — Menu mobile
- **Given** viewport nhỏ (nút mở menu hiển thị), **When** bấm nút, **Then** `aria-expanded` đổi `false` → `true` và danh sách nav xuất hiện
- **Then** nút có `aria-controls` trỏ tới `id` của panel nav
- **Given** menu đang mở, **When** bấm `Escape`, **Then** menu đóng và focus trở về nút mở
- **Given** menu đang mở, **When** điều hướng (pathname đổi), **Then** menu tự đóng

### AC-7 — Container
- **Then** `Container` mặc định max-width 1200px; `variant="workspace"` cho 1440px
- **Then** có padding ngang `gutter` trên mobile và `margin` từ tablet trở lên (DESIGN.md §Layout)

### AC-8 — Không hồi quy, không màu hard-code
- **Then** `src/app/(marketing)/page.test.tsx` pass **mà nội dung không đổi** so với `src/app/page.test.tsx` (kiểm bằng `git log --follow` / `git diff -M`)
- **Then** grep màu hard-code trong `src/components/layout` và `src/app`: rỗng
- **Then** toàn bộ test cũ vẫn pass, coverage vẫn đạt ngưỡng

### AC-9 — Gates
- **Then** `npm run lint`, `npm run typecheck`, `npm run build`, `npm run test:coverage` pass

## 4. File

### Thêm
```
src/components/layout/container.tsx        + container.test.tsx
src/components/layout/skip-link.tsx        + skip-link.test.tsx
src/components/layout/nav-config.ts        + nav-config.test.ts
src/components/layout/nav-link.tsx         + nav-link.test.tsx
src/components/layout/public-header.tsx    + public-header.test.tsx
src/components/layout/public-footer.tsx    + public-footer.test.tsx
src/components/layout/public-shell.tsx     + public-shell.test.tsx
src/components/layout/workspace-shell.tsx  + workspace-shell.test.tsx
src/components/layout/index.ts
src/app/(marketing)/layout.tsx
src/app/(queue)/layout.tsx
src/app/(checkout)/layout.tsx
src/app/(organizer)/layout.tsx
src/app/(ops)/layout.tsx
```

### Di chuyển
```
src/app/page.tsx       -> src/app/(marketing)/page.tsx
src/app/page.test.tsx  -> src/app/(marketing)/page.test.tsx   (noi dung KHONG doi)
```

### KHÔNG sửa
```
src/app/layout.tsx              (root layout da xong o PR truoc)
src/styles/tokens.css           (khong them mau moi cho task nay)
src/components/ui/**            (dung lai, khong sua)
vitest.config.ts
src/lib/queue-client.*
```

## 5. Hợp đồng API

```ts
// nav-config.ts
export interface NavItem {
  href: string;
  label: string;
  /** false = route chua ton tai -> KHONG render <a>, render nhan "Sap co". */
  ready: boolean;
}
export const PUBLIC_NAV: NavItem[]
export const ORGANIZER_NAV: NavItem[]
export const OPS_NAV: NavItem[]
export function isActive(pathname: string, href: string): boolean
// href "/" khop CHINH XAC; href khac khop ca route con (pathname === href || pathname.startsWith(href + "/"))

// container.tsx
export type ContainerVariant = "public" | "workspace"   // 1200px | 1440px
export interface ContainerProps extends React.HTMLAttributes<HTMLDivElement> {
  variant?: ContainerVariant   // default "public"
  as?: "div" | "main" | "section"
}

// skip-link.tsx
export function SkipLink(props: { targetId?: string }): JSX.Element   // default "main"
// <a href="#main"> "Bỏ qua điều hướng, tới nội dung chính"

// nav-link.tsx
export interface NavLinkProps { item: NavItem; className?: string }
export function NavLink(props: NavLinkProps): JSX.Element
// ready -> <Link aria-current={active ? "page" : undefined}>
// !ready -> <span aria-disabled="true"> + <Badge variant="neutral">Sắp có</Badge>

// public-header.tsx / public-footer.tsx / public-shell.tsx
export function PublicHeader(props: { items?: NavItem[] }): JSX.Element
export function PublicFooter(): JSX.Element
export function PublicShell(props: { children: React.ReactNode }): JSX.Element

// workspace-shell.tsx
export interface WorkspaceShellProps {
  children: React.ReactNode;
  title: string;          // hien o topbar
  items: NavItem[];       // nav cua vai tro — CAU HINH hien thi, KHONG phai quyen
}
export function WorkspaceShell(props: WorkspaceShellProps): JSX.Element
```

## 6. Rủi ro

| Rủi ro | Giảm thiểu |
|---|---|
| Di chuyển `page.test.tsx` bị coi là "sửa file bị cấm" | Nội dung không đổi một ký tự; verify bằng `git diff -M --find-renames` cho thấy similarity 100% |
| Route group rỗng (chỉ có layout, không page) | Next cho phép; layout không có page thì không sinh route. Nhưng layout sẽ không được build-time render → coverage phải đến từ test trực tiếp, không từ build |
| `usePathname` cần client component | `NavLink`, `PublicHeader`, `WorkspaceShell` là `"use client"`. Layout của group vẫn là server component và chỉ bọc shell |
| Thêm nhiều file layout ít logic → kéo coverage | Test từng layout trực tiếp (render + assert cấu trúc), không dựa vào build |

## 7. Điểm dừng báo người dùng

- Nếu `next build` báo lỗi vì route group không có page nào → dừng, báo, **không** tự tạo page giả để build xanh
- Nếu phải sửa nội dung `page.test.tsx` (không phải chỉ di chuyển) → dừng, báo

---

## 8. Lệch khỏi plan trong lúc làm (ghi lại để review kiểm)

### 8.1 Thêm `focus-shell.tsx` — file không có trong §4

`(queue)` và `(checkout)` dùng cùng một shell tối giản. Plan §4 chỉ liệt `public-shell` và
`workspace-shell`, nên hai layout đó lẽ ra phải tự inline cấu trúc giống nhau — nhân đôi ~12 dòng.
Đã tách thành `src/components/layout/focus-shell.tsx`. Hệ quả: `index.ts` export thêm `FocusShell`;
test của `(queue)`/`(checkout)` layout phủ luôn file này.

### 8.2 Hợp đồng menu mobile phải chốt lại giữa lúc làm

Spec §3 AC-2 đòi `(organizer)`/`(ops)` **có** landmark `navigation`, còn AC-6 đòi "danh sách nav
xuất hiện" khi bấm toggle. Hai cái xung đột trong jsdom:

- nếu ẩn nav bằng thuộc tính `hidden` thì jsdom coi nó inaccessible → AC-2 đỏ;
- nếu ẩn bằng CSS thì jsdom không đánh giá CSS → nav luôn "xuất hiện" → test AC-6 theo role luôn pass, vô nghĩa.

Chốt: mỗi shell render **đúng một** `<nav>`, không nhân đôi danh sách link; ẩn trên mobile bằng CSS;
trạng thái đọc qua `aria-expanded` của nút và `data-state="open"|"closed"` trên `<nav>`. Đã gửi hợp
đồng này cho agent test ngay khi phát hiện.

> **Sửa phát biểu sai (phát hiện bởi code review độc lập — B2).** Bản trước của mục này viết rằng
> `<nav>` **"luôn nằm trong accessibility tree (không `hidden`, không `aria-hidden`)"**. **Câu đó sai.**
> Class `hidden` của Tailwind biên dịch ra `.hidden{display:none}` (đã verify trong CSS build), và
> `display:none` loại phần tử khỏi accessibility tree **y hệt** thuộc tính `hidden`. Hai cách ẩn không
> khác nhau một chút nào với assistive tech; chúng chỉ khác ở chỗ **jsdom đánh giá được cái nào**.
>
> Hành vi thật, và nó là hành vi **đúng**: ở mobile khi menu đóng, `<nav>` có `display:none` nên
> **không** nằm trong a11y tree — một menu đang đóng thì không nên đọc được. Lỗi nằm ở phát biểu, không
> ở code.
>
> Hệ quả phải nói rõ: **AC-2 chỉ được verify ở desktop.** Dưới 768px với menu đóng, screen reader đếm
> được **0 landmark `navigation`** trên `(organizer)`/`(ops)`. Và các test dùng
> `getByRole("navigation")`, `toHaveLength(1)`, `not.toHaveAttribute("hidden")` **chỉ chứng minh hành
> vi desktop** — chúng pass vì jsdom không chạy Tailwind nên không có `display:none` nào được áp. Riêng
> `not.toHaveAttribute("hidden")` là assertion gần như vô nghĩa: code không bao giờ đặt thuộc tính đó.
>
> Toàn bộ hành vi mobile (ẩn/hiện theo breakpoint) **chưa có test nào kiểm** và không kiểm được bằng
> vitest. Muốn kiểm thật thì cần Playwright với viewport thật — thuộc task sau.

### 8.3 Sửa `(marketing)/page.tsx` ngoài việc di chuyển

`PublicShell` giờ sở hữu `<main>` và `ThemeToggle`. Nếu để nguyên `page.tsx` thì trang chủ có **2
`<main>`** và **2 nút đổi theme**. Đã đổi `<main>` của page thành `<div>` và bỏ `ThemeToggle` khỏi page.
`page.test.tsx` chỉ assert `<h1>EventFlow</h1>` nên vẫn pass và **không bị sửa**.

### 8.4 Phát hiện: `docs/03` §6 thiếu route group cho Moderator

`docs/03-cau-truc-src.md` §6 liệt 5 group `(marketing) (queue) (checkout) (organizer) (ops)`, nhưng
`docs/08-stitch-ui-ux-handoff.md` §4 định nghĩa cả nhánh `/moderator/reviews`,
`/moderator/reviews/[eventId]`, `/moderator/appeals`, và §2 coi Moderator là một trong 5 vai trò có
"bề mặt thao tác riêng".

Jira EVF-1801 ghi "cấu trúc route group theo docs/03 mục 6", nên PR này làm **đúng 5 group của docs/03**
và **không tự thêm group thứ 6**. Hệ quả: Moderator hiện **không có chỗ** trong cấu trúc route.
Cần Product/Design quyết: thêm `(moderator)` hay gộp Moderator vào `(ops)`. Chưa quyết thì
`nav-config.ts` cũng chưa có `MODERATOR_NAV`.

### 8.5 `.next/types` cũ gây lỗi typecheck giả sau khi di chuyển file

Sau khi `git mv src/app/page.tsx`, `npm run typecheck` báo
`.next/types/app/page.ts: Cannot find module '../../../src/app/page.js'` — do `tsconfig.json`
include `.next/types/**/*.ts` và artifact cũ còn trỏ đường dẫn cũ. `rm -rf .next/types` + build lại
là hết. **Không ảnh hưởng CI**: job `lint-web` chạy typecheck trên checkout sạch nên `.next/types`
không tồn tại. Ghi lại để người sau gặp thì không tưởng là lỗi code.

## 9. Evidence

```
$ npm run build
✓ Compiled successfully, Generating static pages (4/4)

Route (app)                    Size     First Load JS
┌ ○ /                          194 B           103 kB
└ ○ /_not-found                896 B           101 kB
```

Trang chủ **vẫn ở `/`** sau khi vào `(marketing)` → route group không tạo segment URL (AC-1).

```
$ npm run typecheck   -> 0 error
$ npm run lint        -> ✔ No ESLint warnings or errors
```

AC-8, phần di chuyển:

```
$ git diff 6bf82ff -M --numstat -- apps/web/src/app
0   0   apps/web/src/app/{ => (marketing)}/page.test.tsx
5   7   apps/web/src/app/{ => (marketing)}/page.tsx
```

- `page.test.tsx`: **0 insertion, 0 deletion** → rename thuần, không đổi một ký tự. Đây là điều AC-8 đòi, và nó đạt.
- `page.tsx`: **5 insertion, 7 deletion** (similarity 80%) → **có** đổi nội dung, đúng như §8.3 đã khai (bỏ `<main>` và `ThemeToggle`). AC-8 không cấm việc này.

> **Sửa lỗi bằng chứng (phát hiện bởi code review độc lập — B1).** Bản trước của khối này dán output
> cho thấy **cả hai** file đều `| 0` rồi kết luận chung là "không đổi một ký tự". Output đó được chạy
> ngay sau `git mv`, **trước khi** `page.tsx` bị sửa ở §8.3, và không được chạy lại sau đó. Nó vì vậy
> phản lại chính §8.3 của tài liệu này, và người đọc tin nó sẽ kết luận sai rằng không file nào trong
> `src/app` bị sửa nội dung. Bài học: evidence phải chạy lại **sau** lần sửa cuối, và phải tách từng
> file thay vì gộp một câu kết luận cho nhiều file.

Lệnh kiểm chặt, đúng điều cần chứng minh:

```
$ git diff 6bf82ff -M --numstat -- 'apps/web/src/app/(marketing)/page.test.tsx'
0   0   apps/web/src/app/{ => (marketing)}/page.test.tsx
```

### 8.6 BỎ hạng mục "chỗ neo cho sticky summary" của `(checkout)`

§1 bảng A ban đầu ghi `(checkout)` là "Shell tối giản **+ chỗ neo cho sticky summary**". Hạng mục này
**đã bị bỏ**, và bản trước của §8 **không khai** — code review độc lập bắt được (M3).

Lý do bỏ: §5 không chốt `id`, `role`, prop hay tên nào cho vùng này. Tự phát minh một contract lúc này
là suy diễn, và sticky summary (handoff §11.6) cần biết cấu trúc dữ liệu đơn hàng + `expires_at` của
server mới thiết kế đúng — những thứ thuộc EVF-1804.

Hệ quả phải chấp nhận: `(checkout)` và `(queue)` hiện dùng **cùng một** `FocusShell` không tham số.
Khi EVF-1804 làm sticky summary, nó sẽ phải hoặc thêm prop `aside?: ReactNode` vào `FocusShell`, hoặc
tự dựng vùng sticky trong page. Cần chốt contract ở task đó, không để mỗi checkout page tự lo.

### 8.7 Năm file `(group)/layout.test.tsx` không nằm trong danh sách §4

§4 liệt 5 file `layout.tsx` nhưng không liệt 5 file test tương ứng, dù §6 có nói "Test từng layout trực
tiếp". Vô hại và cần thiết, nhưng là lệch khỏi §4 nên ghi lại cho đủ (code review nêu ở m9).

---

## 10. Code review độc lập (bước 7) và cách xử lý

Reviewer: agent riêng, context sạch (chỉ nhận spec-plan + diff + tài liệu design). Nó tự chạy lại cả 4
gate và xác nhận số liệu khớp. **14 phát hiện: 2 blocker, 4 major, 8 minor.** Mọi finding đã được tự
verify lại bằng lệnh trước khi sửa.

### Blocker — đã sửa

| | Nội dung | Xử lý |
|---|---|---|
| **B1** | Khối evidence §9 nói sai: dán output cho thấy `page.tsx` đổi `0` dòng, trong khi thực tế là 5 insertion / 7 deletion. Đã tự verify lại bằng `git diff 6bf82ff -M --numstat`. Nguyên nhân: output được chạy ngay sau `git mv`, **trước** lần sửa ở §8.3, và không chạy lại. | Đã thay bằng output thật, tách từng file, kèm ghi chú vì sao sai. Điều AC-8 thực sự đòi (`page.test.tsx` = 0/0) thì vẫn đạt. |
| **B2** | §8.2 khẳng định `nav` "luôn nằm trong accessibility tree". Sai: `.hidden{display:none}` (đã verify trong CSS build) loại phần tử khỏi a11y tree y hệt thuộc tính `hidden`. Hai cách ẩn chỉ khác ở chỗ jsdom thấy được cái nào. | Đã sửa phát biểu, nói rõ AC-2 **chỉ verify được ở desktop**, và các test role-based chỉ chứng minh hành vi desktop. Hành vi code là đúng; chỉ phát biểu sai. |

### Major

| | Nội dung | Xử lý |
|---|---|---|
| **M1** | `WorkspaceShell` đặt `nav` **trước** nút toggle trong DOM. Ở mobile (`md:grid` nên dưới 768px là block flow): Tab từ nút đi tới `main` chứ không tới link vừa hiện (phải Shift+Tab); và menu mở ra **phía trên** topbar, đẩy nút vừa bấm xuống đúng bằng chiều cao menu, trong khi ngón tay người dùng đang ở đó. Class `border-b` cho thấy ý định là menu nằm **dưới** nút. | **Đã sửa.** Thứ tự DOM giờ là topbar, nav, main. Desktop dùng `md:col-start` / `md:row-start` / `md:row-span-2` để đặt nav sang cột 1 kéo hết 2 hàng, không cần đổi DOM. |
| **M2** | `WorkspaceShell` render `title` thành `h1`, mà `title` hardcode ở **layout** nên mọi route trong group có cùng một `h1` — heading duy nhất, giống nhau, nằm ngoài `main`. Và ngay khi EVF-1802 thêm page tự đặt `h1` theo pattern của repo, trang sẽ có **hai `h1`**. Đúng loại lỗi §8.3 đã bắt cho `main` / ThemeToggle nhưng bỏ sót cho heading. | **Đã sửa:** `h1` thành `p`. Topbar là chrome, không phải tiêu đề nội dung; `h1` thuộc về page. |
| **M3** | Hạng mục `(checkout)` sticky anchor bị bỏ im lặng. | **Đã ghi lại** ở §8.6 và gỡ khỏi §1. Không tự phát minh contract. |
| **M4** | `ORGANIZER_NAV` có `/organizer/events` và `/organizer/events/new` lồng tiền tố. Ở pathname `/organizer/events/new`, `isActive` trả true cho **cả hai** nên hai item cùng `aria-current="page"`. Latent hôm nay vì cả hai `ready: false` nên `NavLink` return sớm, nhưng EVF-1802 bật `ready: true` là lộ ngay. | **Đã sửa:** thêm `activeHref(items, pathname)` chọn theo khớp **dài nhất**, tính ở cấp danh sách; `NavLink` nhận `active?: boolean`, bỏ trống thì tự tính như cũ. |

### Minor

| | Xử lý |
|---|---|
| **m1** `/ops/events` là route index không có trong handoff §4 (Ops chỉ có `/ops/events/[eventId]` và `/ops/ai`) | Đã thêm comment nói rõ đây là route **đề xuất**, chưa có trong §4, giữ `ready: false` tới khi Product chốt. |
| **m4** Bấm link trùng route hiện tại thì menu mobile không đóng (pathname không đổi nên effect không chạy) | **Đã sửa:** `onClick` trên chính `nav` đóng menu, ở cả 2 shell. |
| **m6** `open` sống sót qua lần đổi breakpoint; desktop dính `border-b` lạ | **Đã sửa:** thêm `md:border-b-0`. |
| **m3** `aria-disabled` trên `span` không tương tác là vô nghĩa với AT — thứ thực sự truyền tin là text của Badge "Sắp có" | **Chưa sửa.** AC-5 đang encode attribute này. Giữ để thoả AC, mở ticket riêng: bỏ `aria-disabled` hoặc bọc nav trong `ul` cho cấu trúc rõ hơn. |
| **m7** SkipLink trong `(queue)` / `(checkout)` đọc "Bỏ qua điều hướng" nhưng ở đó không có điều hướng nào | **Chưa sửa.** Cần thêm prop `label` cho SkipLink và sửa test đang assert đúng chuỗi đó. Ticket riêng. |
| **m5** Listener Escape gắn trên `document` không `stopPropagation`, nên khi có dialog thì một lần Escape đóng cả dialog và menu | **Chưa sửa.** Repo chưa có dialog. Ticket riêng, xử lý khi làm ConfirmationDialog (handoff §11.10). |
| **m8** Coverage 99.51% bị thổi lên vì `vitest.config.ts` tính cả file test | Ngoài phạm vi (§4 cấm sửa file đó). Đã có ticket từ PR trước. |
| **m9** Năm file layout test không có trong §4 | Đã ghi ở §8.7. |

### Reviewer xác nhận sạch

- **AC-1**: build exit 0, bảng route chỉ có `/` và `/_not-found` nên route group không sinh segment. Build **không warning** về 4 group chỉ có `layout.tsx` — Next 15.0.3 chấp nhận, chỉ không sinh route. Không có `html` / `body` trùng.
- **AC-3**: kiểm tới **CSS build thật** vì lo `focus:not-sr-only` (đặt `position:static`, `margin:0`, `padding:0`) ghi đè `focus:absolute` / `focus:px-md`. Offset trong CSS: `.focus\:not-sr-only:focus` ở 14412, `.focus\:absolute:focus` ở 14547 nên utility position/padding thắng. SkipLink **là** focusable đầu tiên trong app thật (ToastProvider render children trước vùng toast).
- **AC-4** `isActive` với mọi input khó: `/organizer/eventsarchive` so với `/organizer/events` cho false; pathname rỗng cho false và không throw; trailing slash cho true; `/organizer` so với `/` cho false.
- **AC-5** grep độc lập: chỉ 3 dạng `href` được phép.
- **AC-7** kiểm bằng CSS build chứ không bằng tên class: `.max-w-[1200px]`, `.max-w-[1440px]`, `.px-gutter{padding-inline:1rem}`, `.md\:px-margin` trong `@media (min-width:48rem)`, `.md\:grid-cols-[264px_1fr]` đều được sinh. 264px khớp handoff §6 ("240 đến 264px").
- **AC-8** grep màu hard-code cho kết quả rỗng.
- **Hook/React sạch**: `NavLink` gọi `usePathname()` **trước** early return nên không vi phạm thứ tự hook; không stale closure; `if (!open) return` nghĩa là shell có menu đóng **không gắn listener nào** trên `document`, nên không có chuyện Escape của shell này bắn cho shell kia, và ở runtime chỉ một shell mount mỗi page; cleanup đủ; `toggleRef.current` không thể null khi `.focus()` chạy.
- **App Router**: `"use client"` đúng ở 3 component dùng hook, không có ở 5 component server. Năm layout group là server component bọc shell.
- **Kỷ luật phạm vi**: diff **không** đụng `src/app/layout.tsx`, `src/styles/tokens.css`, `src/components/ui/`, `vitest.config.ts`, `src/lib/queue-client`.
- **§2.2** cảnh báo "cấu hình hiển thị, không phải ma trận quyền" có ở `nav-config.ts`, nhắc lại ở `workspace-shell.tsx` và đầu file test.

### Reviewer đồng ý với quyết định `(queue)` / `(checkout)` không có nav

Trên cơ sở nội dung, không coi là sót: handoff §6, §5 nguyên tắc 2 và nguyên tắc 8 đều chống lại việc
đặt menu điều hướng vào một luồng có TTL. Việc logo là `span` thay vì link về trang chủ cũng đúng cùng
logic — một logo-link là đường thoát một-click khỏi hold 10 phút.
