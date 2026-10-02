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
| `(checkout)` | Buyer đang chọn vé / thanh toán | Shell tối giản + chỗ neo cho sticky summary |
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

Chốt: mỗi shell render **đúng một** `<nav>`, **luôn** nằm trong accessibility tree (không `hidden`,
không `aria-hidden`); ẩn trên mobile là việc của CSS; trạng thái đọc qua `aria-expanded` của nút và
`data-state="open"|"closed"` trên `<nav>`. `aria-controls` vì vậy không bao giờ trỏ vào phần tử không
tồn tại. Đã gửi hợp đồng này cho agent test ngay khi phát hiện.

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

AC-8, phần di chuyển không đổi nội dung:

```
$ git diff --cached -M --stat -- src/app
 apps/web/src/app/{ => (marketing)}/page.test.tsx | 0
 apps/web/src/app/{ => (marketing)}/page.tsx      | 0
 2 files changed, 0 insertions(+), 0 deletions(-)
```

Git nhận là rename với **0 insertion, 0 deletion** → `page.test.tsx` không đổi một ký tự.
