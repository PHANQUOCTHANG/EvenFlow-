# EVF-1801 — Code review độc lập (bước 7) + xử lý

Reviewer: agent riêng, context sạch (chỉ nhận `spec.md` + `plan.md` + diff, không tham gia viết code).
Thời điểm review: trạng thái 248/248 test pass, lint/typecheck/build xanh, coverage 99.66%.

Mọi finding dưới đây **người implement đã tự verify lại** trước khi sửa, không nhận ngay.

---

## BLOCKER

### B-1 — `docker build` chắc chắn fail: `apps/web/public` không tồn tại

`deploy/docker/web.Dockerfile:22` có `COPY --from=build /app/public ./public`, nhưng thư mục
`apps/web/public` **không có trong repo**.

Tự verify:

```
$ ls -d apps/web/public
ls: cannot access 'apps/web/public': No such file or directory

$ ls -d apps/web/.next/standalone/public
ls: cannot access ... : No such file or directory
```

BuildKit lỗi ngay khi `COPY` một path không tồn tại. Dòng 22 là code cũ không nằm trong diff, nhưng
nó thuộc đúng file mà task này sửa và đúng AC mà task này cam kết. Nghiêm trọng hơn: `trivy.yml`
(matrix `web`) build Dockerfile này trên **mọi PR chạm `apps/web/**`**, nên job `trivy (web)` của PR
này sẽ đỏ; và `cd-web.yml` mới sẽ fail ở mọi lần push `main` → CD không bao giờ publish được image.

**Đã sửa:** thêm `apps/web/public/.gitkeep`. Đây cũng là layout chuẩn của Next.

---

## MAJOR

### M-1 — `workflow_dispatch` dịch được tag `:latest` sang nhánh bất kỳ, bỏ qua cổng CI

`cd-web.yml`: `workflow_dispatch: {}` không giới hạn nhánh, và `if` cũ
(`github.event_name == 'workflow_dispatch' || ...`) pass ngay ở nhánh đầu **không kiểm tra CI gì cả**.
Bất kỳ ai có quyền write mở Actions → Run workflow → chọn nhánh tuỳ ý là `:latest` của
`ghcr.io/<owner>/eventflow-web` trỏ vào code chưa qua lint/test/build. Trái trực tiếp phần giảm thiểu
ở `spec.md` §5 ("chỉ trigger trên main").

**Đã sửa:** `if` của job giờ bắt buộc `github.ref == 'refs/heads/main'` cho nhánh dispatch.

### M-2 — PR từ fork vẫn kích hoạt được CD; AC-13 yêu cầu ngược lại

`workflow_run.branches: [main]` lọc theo `workflow_run.head_branch`, và với PR từ **fork** thì
`head_branch` là tên nhánh **trong fork**. Một fork PR mở từ nhánh tên `main` (rất thường gặp) sẽ sinh
`workflow_run` có `head_branch == "main"` → lọt filter → `if` pass (CI của fork PR vẫn `success`) →
`actions/checkout` với `ref: head_sha` fetch được commit của fork → build code chưa review và push lên
GHCR bằng `GITHUB_TOKEN` có `packages: write`. Lỗ supply-chain, và AC-13 ghi rõ "không chạy trên PR từ fork".

**Đã sửa:** thêm vào `if`: `github.event.workflow_run.event == 'push'` (loại `pull_request`) và
`github.event.workflow_run.head_repository.full_name == github.repository`.

### M-3 — Accessible name của Button bị spinner làm bẩn khi `loading`

`Spinner` có `role="status"` + `aria-label="Đang xử lý"` và nằm **bên trong** `<button>`, nên
`aria-label` đó nhập vào name-from-content của button. Reviewer tính thật bằng `dom-accessibility-api`
(chính thư viện mà testing-library dùng) trên đúng cây DOM component sinh ra:

```
NAME = "Dang xu ly Mua ve"
```

Screen reader đọc "Đang xử lý Mua vé, nút, bận". Và mọi consumer viết
`getByRole("button", { name: "Mua vé" })` sẽ **trượt ngay khi nút vào loading** — đúng lúc cần chờ nút.
Test cũ không bắt được vì nó query `getByRole("status")`, tức là *dựa vào* chính cái role gây lỗi.

**Đã sửa:** `SpinnerProps` thêm `decorative?: boolean`. Khi `decorative` thì bỏ `role` và `aria-label`,
thêm `aria-hidden="true"`. `Button` khi `loading` render `<Spinner size="sm" decorative />`; trạng thái
bận vẫn được truyền bằng `aria-busy="true"` trên chính `<button>`. Test đã được giao lại cho agent test
sửa sang assert `getByRole("button", { name: "Mua vé" })` + `aria-busy` — tức là chốt luôn điều lẽ ra
phải bắt được bug này từ đầu.

### M-4 — Fallback của `color-mix()` biến nền soft thành màu đặc → chữ tương phản 1:1

Nghiêm trọng nhất về mặt người dùng. Lightning CSS (Tailwind v4) sinh fallback **thật sự đang ship**:

```css
:root{--ef-info-soft:var(--ef-info)}
@supports (color:color-mix(in lab,red,red)){:root{--ef-info-soft:color-mix(in oklab,var(--ef-info) 12%,var(--ef-surface))}}
```

Tự verify trên CSS build:

```
$ grep -o 'ef-info-soft:[^;}]*' <css>
ef-info-soft:var(--ef-info)
ef-info-soft:color-mix(in oklab,var(--ef-info) 12%,var(--ef-surface))
```

Browserslist mà Next resolve cho project có `chrome 109`, `and_uc 15.5`, `and_qq 14.9`, `op_mini all`,
`kaios 2.5` — đều **không có `color-mix`** (Chrome có từ 111). Trên các browser đó nền soft = màu
semantic đặc, cùng màu với chữ đặt trên nó:

| Badge | nền thực tế | chữ | tương phản |
|---|---|---|---|
| `queued` | `#5b55e7` | `#5b55e7` | **1:1 — chữ biến mất** |
| `failed` | `#b42318` | `#b42318` | **1:1** |
| `admitted` / `paid` | `#18794e` | `#18794e` | **1:1** |
| `lobby` / `expired` / `soldout` / `neutral` | `#526071` | `#526071` | **1:1** |
| `holding` / `pending` | `#e6a23c` | `#9a5b00` | ~2.5:1 |

Vi phạm AC-9 trên target thật mà build đang hỗ trợ. Chrome 109 là bản cuối cho Windows 7/8.1 — không
phải nhóm bỏ qua được với một site bán vé ở VN. Badge mất chữ còn phá luôn AC-7 ("không truyền tin chỉ
bằng màu").

Reviewer **đúng** khi xác nhận phần cascade: token soft dẫn xuất bằng `color-mix` **có** re-derive trong
dark (cùng một element `<html>`, custom property chỉ có một computed value), nên việc khối dark không
khai lại là đúng về cascade. Vấn đề nằm hoàn toàn ở fallback.

**Đã sửa:** bỏ `color-mix`, khai 5 token `--ef-*-soft` bằng **hex tường minh** trong cả `:root` và **cả
hai** khối dark. Giá trị chọn bằng cách tính toán, không ước lượng — xem `evidence.md` §14.

---

## MINOR

| # | Nội dung | Xử lý |
|---|---|---|
| m-1 | `.ef-focus-ring` nằm ở `@layer components` nên `border-color` bị utility `.border-border-strong` (`@layer utilities`) đè — cascade layer thắng specificity. Ring vẫn thấy nhờ `box-shadow` nên AC-9 vẫn đạt, nhưng đó là CSS chết. | **Đã sửa:** chuyển sang `@layer utilities`. Verify trên CSS build: `.ef-focus-ring` nằm trong `@layer utilities{` và ở offset 14093, sau `.border-border-strong` ở 9686 → thắng cả theo layer, specificity và source order. |
| m-2 | Logic hợp nhất ref của Checkbox **đúng** nhưng không test nào chạm (coverage `checkbox.tsx` dòng 37, 39-40 chưa cover; `grep createRef` trong file test không có kết quả). Nếu ai bỏ forwardRef thì không test nào đỏ. | **Đã giao** agent test thêm test object ref + function ref + `indeterminate` khi có ref ngoài. |
| m-3 | Guard chặn click của Button (dòng 52-54) không bao giờ thực thi trong test vì `disabled` khiến jsdom không dispatch click → 2 test "không gọi onClick" pass hoàn toàn nhờ `disabled`. Ngoài ra `loading → disabled` làm nút rời tab order và **mất focus ngay lúc submit**, pattern ARIA khuyến nghị `aria-disabled` + guard JS. | **Không sửa trong PR này.** `spec.md` AC-3 và `plan.md` §3 đã chốt `loading -> disabled`; đổi sang `aria-disabled` là đổi hợp đồng, vượt scope. Guard giữ lại làm belt-and-braces. **Mở ticket riêng** cho vấn đề mất focus. |
| m-4 | `caughtErrors: "none"` nới lỏng rule cho **toàn repo, vĩnh viễn** vì đúng **một** chỗ (`queue-client.ts:127`). Chỗ `for await (const _ of ...)` là *variable*, đã được `varsIgnorePattern` phủ. | **Đã sửa:** giữ `caughtErrors` mặc định `"all"` cho toàn repo, khoanh ngoại lệ bằng block config `files: ["src/lib/queue-client.ts"]`. Code mới vẫn bị check đầy đủ. |
| m-5 | 5 file ngoài danh sách cho phép của `plan.md` §2. Reviewer đánh giá **lý do của cả 5 là đủ**, và xác nhận: không file nào trong nhóm "KHÔNG được sửa" bị chạm (`git diff` rỗng); `.dockerignore` **không phá** 3 Dockerfile còn lại (đã đọc từng cái); `next-env.d.ts` vắng mặt trong checkout sạch **không** làm `tsc` lỗi (reviewer chạy thử với tsconfig bỏ file đó khỏi `include`). Góp ý: nên loại thêm `docs/` và `apps/web/doc` (37 ảnh mockup ~14 MB) khỏi build context. | **Đã sửa** phần `.dockerignore`. Phần `eslint.config.mjs` đổi hành vi CI của cả team → **sẽ nêu rõ trong PR description**. |

---

## Phán quyết về mục mở `field.test.tsx`

Reviewer độc lập kết luận: **TEST sai, IMPLEMENTATION đúng** — trùng với kết luận đã có. Căn cứ nó đưa
ra mạnh hơn phần mình tự lập luận:

1. `input.test.tsx:199-202` và `select.test.tsx:135-138` render field **có `required`** rồi
   `getByLabelText("Email")` / `getByLabelText("Loại vé")`. `getByLabelText` mặc định so khớp **chính
   xác**. Nếu `*` nằm trong `<label>` thì accessible name thành `"Email *"` và 2 test đó trượt. Chúng độc
   lập với `field.test.tsx` và chốt đúng hành vi mà hiện thực đang làm.
2. Tính bắt buộc phải truyền bằng `required` native → `aria-required`, và `input.tsx:41` đã làm đúng
   (`input.test.tsx:202` `toBeRequired()` chứng minh). Dấu `*` là trang trí, đã `aria-hidden`. Nhét nó
   vào accessible name là bơm nội dung trang trí vào tên control, và screen reader đọc "Email dấu hoa
   thị" hay "Email" là không xác định.
3. Assertion cũ đo **sai phần tử**: nó đo `<label>`, trong khi thứ phải dài ra là container.

---

## Reviewer đã kiểm và xác nhận sạch

- **AC-1**: grep mở rộng (`#hex|rgba?\(|hsla?\(|oklch\(|color-mix\(`) trên `src/components src/app src/hooks src/lib` → 0 kết quả.
- **AC-9**: grep trên **CSS đã build**, `--ef-accent` chỉ xuất hiện ở `border-color` và trong background. `accent-primary` ở Checkbox/Radio là `accent-color` (tint của form control), không phải `color`.
- **Cascade dark mode, cả 5 tổ hợp** (system light/dark × chưa chọn/chọn light/chọn dark), bao gồm trường hợp **JS bị tắt** (media query vẫn cho dark). Hai selector cùng specificity 0-2-0, khối manual đặt sau nên thắng source order — đúng như comment trong tokens.css.
- **Chống FOUC**: inline script nằm ở offset 929 trong `.next/server/app/index.html`, trước `</head>` (1251) và `<body` (1258) → script đồng bộ, chạy trước paint.
- **Toast**: không tìm thấy lỗi nào — `dismiss` identity-stable, `toast` phụ thuộc đúng `[dismiss]`, timer map là `useRef` và được clear + delete, `useEffect([])` cleanup clear hết timer khi unmount nên **không có setState sau unmount**, `counter.current` chỉ mutate trong event handler chứ không trong render, toast mới append không ghi đè.
- **`use-theme.ts`**: không có hook điều kiện, `useEffect([])` ưu tiên attribute mà ThemeScript đã đặt (đúng, tránh flash).
- **Field layer / aria**: `useFieldIds` luôn gọi `useId`; `describedBy` chỉ trả id của phần tử **thực sự được render** (`FieldShell` render có điều kiện với **cùng** điều kiện) → không có `aria-describedby` trỏ phần tử không tồn tại; `RadioGroup` dùng `role="radiogroup"` + `aria-labelledby` trỏ `<legend>` (bắt buộc, vì đặt role lên `<fieldset>` làm mất liên kết legend ngầm định).
- **`cn()`**: đặt `className` của caller **sau** class mặc định nên không bị ghi đè, và không có merge thông minh nào âm thầm gỡ class.
- **CD làm đúng 2 điểm khó**: `actions/checkout` có `ref: head_sha` → build đúng commit đã qua CI, không phải HEAD của nhánh mặc định (bẫy phổ biến nhất của `workflow_run`); tên image chắc chắn lowercase nhờ `${owner,,}` (bash parameter expansion, shell mặc định của `ubuntu-latest`).
- **Không tìm thấy defect** trong: React hook misuse, stale closure, timer leak, setState sau unmount, `aria-describedby` treo, forwardRef làm mất ref của caller, `.dockerignore` phá Dockerfile khác.

## Hai điểm reviewer nêu mà mình không đồng ý / đã có bằng chứng khác

1. **"YAML `cd-web.yml` chưa verify bằng công cụ"** — reviewer không parse được vì môi trường nó thiếu
   PyYAML. Mình **đã** parse bằng `js-yaml` (có trong `node_modules`), trước và sau khi sửa, và cả
   `ci.yml`. Xem `evidence.md` §8. AC-13 phần này **đạt**.
2. **Dockerfile vẫn chưa reproducible tuyệt đối** (`node:22-alpine` là tag mutable, không pin digest;
   CI test trên Node 20 nhưng image chạy Node 22). Reviewer tự xếp minor và không tie được vào failure
   cụ thể — **đồng ý**, nhưng đây là vấn đề chung của cả 4 Dockerfile trong `deploy/docker/`, không
   riêng web. Thuộc task hạ tầng riêng, không nhét vào EVF-1801.
