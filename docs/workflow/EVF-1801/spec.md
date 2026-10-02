# EVF-1801 — Design system + component UI dùng chung + CD frontend (spec)

| | |
|---|---|
| Jira | EVF-1801 (Epic 1800 — E9 Frontend), Sprint 0, Priority Highest, 5 SP — **cộng thêm CD frontend do người dùng yêu cầu** |
| Tier | **Standard** (feature mới, không đụng DB/auth/payment/delete). Phần CD có chạm `.github/workflows` + registry → xem mục 6. |
| Base | `fa8ca69` (develop sau khi merge origin/main) |
| Branch | `feature/EVF-1801-design-system` |
| Nguồn design | `apps/web/doc/design/stitch/md/DESIGN.md`, `docs/08-stitch-ui-ux-handoff.md` (§6 token table, §11.5 component list), 37 ảnh mockup |
| Cập nhật | 2026-10-02 — token đã chốt, không còn chặn |

---

## 1. Bối cảnh & hiện trạng

`apps/web` phục vụ Buyer, Organizer, Moderator, Ops. Hiện trạng (branch base `fa8ca69`):

- `src/app/layout.tsx` — root layout tối giản, `lang="vi"`
- `src/app/page.tsx` — placeholder, **dùng inline style**
- `src/app/globals.css` — 10 dòng, màu hard-code `#fafaf9` / `#0c0a09`
- `src/lib/queue-client.ts` + test (11 test)
- Vitest + RTL đã cấu hình, coverage threshold lines/functions/statements 70%, branches 60%
- **Chưa có**: Tailwind, `components/`, `hooks/`, `styles/`, token, route group
- `next.config.mjs` đã bật `output: "standalone"` → image nhỏ, khởi động nhanh (cần cho pre-warm trước giờ mở bán)

CI hiện có (`.github/workflows/ci.yml`, teammate thêm ~2026-09-27) đã bao phủ web khá đủ:

| Job | Nội dung | AC liên quan |
|---|---|---|
| `lint-web` | `npm run lint`, `npm run typecheck` | AC-12 (đã có sẵn) |
| `test-web` | `npm run test:coverage` (áp ngưỡng vitest) | AC-10 |
| `build` | `npm ci && npm run build` | AC-11 |
| `trivy.yml` → `scan` matrix `web` | build `web.Dockerfile` + Trivy CRITICAL | AC-13 |

**Không có bất kỳ workflow CD nào** — không gì publish image hay deploy. Đây là phần người dùng yêu cầu thêm.

## 2. Phạm vi

### Trong phạm vi

**A. Nền design system**
1. Cài & cấu hình Tailwind CSS v4.3.3 (`tailwindcss` + `@tailwindcss/postcss`), cấu hình CSS-first qua `@theme` — **không** tạo `tailwind.config.js`
2. `src/styles/tokens.css`: token duy nhất cho màu (light + dark), typography, spacing, radius, shadow, focus ring
3. Font Inter qua `next/font/google` với `tnum` (tabular numbers) bật cho dữ liệu định lượng
4. Dark mode: `data-theme` trên `<html>`, toggle thủ công, tôn trọng `prefers-color-scheme`, inline script chống FOUC
5. Helper `cn()` gộp className (tự viết, không thêm dependency)

**B. Component library theo handoff §11.5**

| Component | Variant / state bắt buộc |
|---|---|
| `Button` | `primary` / `secondary` / `tertiary` / `destructive`; size `sm` / `md` / `lg`; hover, focus-visible, pressed, disabled, loading |
| `Input` | type `text` / `email` / `password` / `otp`; label luôn hiện, helper text, error, disabled |
| `Select` | label, option, placeholder, error, disabled |
| `Checkbox` | label, checked / unchecked / indeterminate, error, disabled |
| `Radio` / `RadioGroup` | label nhóm, option, error, disabled |
| `FieldError` + `FieldHelp` | inline validation dùng chung cho mọi field |
| `Alert` | `info` / `success` / `warning` / `error`; title + body + action tuỳ chọn, dismiss tuỳ chọn |
| `Toast` + `ToastProvider` | 4 variant như Alert, auto-dismiss, hàng đợi nhiều toast, dismiss thủ công |
| `Badge` | trạng thái nghiệp vụ EventFlow: `lobby` `queued` `admitted` `holding` `paid` `pending` `expired` `soldout` `failed`; **bắt buộc có text, không chỉ màu** |
| `Spinner` | size `sm` / `md` / `lg`, `role="status"` |
| `Card` | header / body / footer, radius 16px, elevation cấp 1 |

Barrel export `src/components/ui/index.ts`. Test RTL cho từng component.

**C. Dọn dẹp**
- Refactor `page.tsx` bỏ inline style, dùng token + component mới (chứng minh system hoạt động). **Giữ nguyên `<h1>EventFlow</h1>`** vì `page.test.tsx` assert nó.
- `globals.css` bỏ màu hard-code, import tokens

**D. CD frontend (người dùng yêu cầu thêm)**
- Workflow mới `.github/workflows/cd-web.yml`: build `deploy/docker/web.Dockerfile` → push `ghcr.io/<owner>/eventflow-web` khi push vào `main`; tag `sha-<short>` + `latest`; chạy sau khi CI xanh
- Sửa bug trong `deploy/docker/web.Dockerfile`: hiện copy `package.json` rồi `npm install` **không có lockfile** → build không reproducible. Đổi sang copy cả `package-lock.json` + dùng `npm ci`.

### Ngoài phạm vi (cố ý)

| Việc | Lý do |
|---|---|
| Route groups `(marketing) (queue) (checkout) (organizer) (ops)` | Thuộc "Phạm vi công việc" của EVF-1801 nhưng không nằm trong AC gốc; người dùng chốt scope là component library. **EVF-1801 vì vậy chưa đóng hoàn toàn.** |
| Client API sinh từ OpenAPI | Chặn bởi backend: 6/8 service Go vẫn là stub, chưa có spec OpenAPI |
| Domain component (QueueStatusPanel, ServerExpiryCountdown, EventCard, OrderSummary, AssistantPanel, DataTable, MetricCard, ConfirmationDialog) | Handoff §11.1–11.4, §11.6–11.12 → thuộc EVF-1802/1803/1804, cần API contract |
| Deploy thật lên môi trường chạy (k8s/compose trên server) | Chưa có cluster/host. CD dừng ở publish image có tag — bước deploy là việc riêng khi có hạ tầng. |

## 3. Design token — ĐÃ CHỐT

### 3.1 Xung đột trong tài liệu design và cách xử lý

Tài liệu design không tự nhất quán. Đã đối chiếu 2 nguồn và chốt như sau:

| # | Xung đột | Quyết định |
|---|---|---|
| 1 | `DESIGN.md` YAML frontmatter là palette Material auto-gen của Stitch (`primary: #030e24`, `secondary: #4c44d8`, `surface: #f8f9ff`); phần prose bên dưới và handoff §6 lại ghi palette khác (`#18243A`, `#5B55E7`, `#F5F7FA`) | **Lấy prose + handoff §6**. Hai nguồn độc lập đồng thuận, có gán vai trò semantic rõ ràng. **Bỏ YAML frontmatter.** |
| 2 | Border: `#E4E7EC` (DESIGN.md) vs `#DCE2EA` (handoff §6); thêm `#D0D5DD` (DESIGN.md, border của control/elevation 2) | `--color-border: #DCE2EA`, `--color-border-strong: #D0D5DD` |
| 3 | Radius panel: 16px (DESIGN.md) vs 20px (handoff §6) | Control 10px, card 16px (cả 2 nguồn khớp), panel 20px (theo handoff — chỉ dùng cho hero/large panel) |
| 4 | **Không có palette dark ở bất kỳ nguồn nào**, toàn bộ design là light-only; nhưng AC gốc yêu cầu dark mode | Người dùng chốt: **dẫn xuất palette dark**, giữ vai trò semantic, kiểm tương phản, đánh dấu là giá trị đề xuất chờ design xác nhận |

### 3.2 Phát hiện accessibility — amber không dùng được làm text

Đã tính tương phản WCAG cho từng token trên nền trắng `#FFFFFF`:

| Token | Giá trị | Tỉ lệ trên trắng | Kết luận |
|---|---|---|---|
| `text.primary` | `#182230` | ~15.9:1 | Pass AAA |
| `text.secondary` | `#526071` | **6.42:1** | Pass AA |
| `action.600` | `#5B55E7` | **5.39:1** | Pass AA (text + nền nút với chữ trắng) |
| `success.700` | `#18794E` | **5.41:1** | Pass AA |
| `warning.700` | `#9A5B00` | **5.43:1** | Pass AA |
| `danger.700` | `#B42318` | **6.57:1** | Pass AA |
| `accent.500` | `#E6A23C` | **2.19:1** | **FAIL — không được dùng làm màu chữ** |

`DESIGN.md` mô tả widget countdown "chuyển viền **và nhãn** sang Warm Amber `#E6A23C`" khi còn dưới 2 phút. **Nhãn chữ amber trên nền trắng chỉ đạt 2.19:1, vi phạm WCAG AA** mà chính tài liệu tự cam kết tuân thủ. Xử lý: `--color-accent` chỉ dùng cho viền/nền/icon; chữ cảnh báo dùng `--color-warning-fg` (`#9A5B00`, 5.43:1). Sẽ báo lại cho Design.

### 3.3 Token light (từ handoff §6 + DESIGN.md prose)

```
--color-bg: #F5F7FA            canvas ứng dụng
--color-surface: #FFFFFF       card / dialog
--color-surface-subtle: #EEEDFF  nền nhấn nhẹ (action.100)
--color-fg: #182230            text chính
--color-fg-muted: #526071      text phụ
--color-border: #DCE2EA
--color-border-strong: #D0D5DD
--color-brand: #18243A         navy: header, chrome nội bộ
--color-brand-soft: #34496B
--color-primary: #5B55E7       CTA chính, focus, link trọng yếu
--color-primary-hover: #4A44D4 tối 8%
--color-primary-fg: #FFFFFF
--color-accent: #E6A23C        viền/nền/icon — KHÔNG dùng làm chữ
--color-success: #18794E
--color-warning-fg: #9A5B00
--color-danger: #B42318
--focus-ring: 0 0 0 3px rgba(91, 85, 231, 0.2)
```

### 3.4 Token dark — ĐỀ XUẤT, chờ Design xác nhận

Dẫn xuất từ palette light, giữ nguyên vai trò semantic. Mọi giá trị đã kiểm tương phản trên nền `#0B1220`:

```
--color-bg: #0B1220
--color-surface: #121C2E
--color-surface-subtle: #221F4A
--color-fg: #E8EDF5            15.9:1 trên bg — pass AAA
--color-fg-muted: #A3B1C4      8.59:1 trên bg, 7.83:1 trên surface — pass AA
--color-border: #27354A
--color-border-strong: #3A4B66
--color-brand: #121C2E
--color-primary: #8B87F5       indigo sáng hơn để nổi trên nền tối
--color-primary-fg: #0B1220    5.98:1 — chữ TỐI trên nền indigo sáng (chữ trắng chỉ 3.05:1, fail)
--color-accent: #F0B355        9.8:1 — viền/nền/icon
--color-warning-fg: #F0B355
--color-success: #45C78A
--color-danger: #F97066        6.55:1
```

> Khác biệt quan trọng so với light: ở dark, chữ trên nút primary là **màu tối** `#0B1220`, không phải trắng — vì chữ trắng trên `#8B87F5` chỉ đạt 3.05:1 (fail AA). Mỗi theme có `--color-primary-fg` riêng.

### 3.5 Typography (DESIGN.md, giữ nguyên)

Font **Inter** duy nhất, bật `font-feature-settings: "tnum"` cho countdown, số thứ tự hàng đợi, tiền VNĐ, số vé.

Scale: `headline-xl` 36/44 w700 ls-0.02em · `headline-lg` 28/36 w600 · `headline-md` 20/28 w600 · `headline-sm` 16/24 w600 · `body-lg` 16/24 w400 · `body-md` 14/20 w400 · `body-sm` 12/18 w400 · `label-lg` 14/20 w600 · `label-md` 12/16 w600 · `label-sm` 11/14 w600 ls+0.02em · `numeric-metric` 32/38 w700 · `numeric-timer` 20/24 w700 ls+0.04em

### 3.6 Spacing, radius, elevation

Spacing nhịp 4/8: `xs` 4px · `sm` 8px · `md` 16px · `lg` 24px · `xl` 32px. Gutter 1rem (mobile 0.75rem), margin 1.5rem (mobile 1rem).

Radius: `control` 10px (button, input, select, chip) · `card` 16px · `panel` 20px · `pill` 9999px (chỉ status badge).

Elevation: cấp 1 card `0 1px 3px rgba(16,24,40,0.05)` + viền 1px · cấp 2 floating `0 4px 12px -2px rgba(16,24,40,0.08)` · cấp 3 modal `0 12px 32px -4px rgba(16,24,40,0.14)`.

## 4. Tiêu chí nghiệm thu (AC)

### AC-1 — Token tập trung một nơi *(AC gốc Jira #1)*
- **Given** toàn bộ `apps/web/src`, **When** grep màu hard-code (`#[0-9a-fA-F]{3,8}`, `rgb(`, `hsl(`) trong `src/components/**` và `src/app/**`, **Then** không có kết quả; màu chỉ định nghĩa trong `src/styles/tokens.css`
- **Bằng chứng**: output grep lưu vào `evidence.md`

### AC-2 — Dark mode đồng bộ toàn trang *(AC gốc Jira #2)*
- **Given** `<html data-theme="dark">`, **When** render bất kỳ page/component, **Then** mọi token màu lấy giá trị dark
- **Given** `prefers-color-scheme: dark` và người dùng chưa chọn thủ công, **When** load, **Then** theme = dark
- **Given** người dùng chọn theme thủ công, **When** reload, **Then** lựa chọn được giữ (localStorage) và **không FOUC** — bắt buộc inline script chạy trước hydration
- **Given** `localStorage` bị chặn / ném lỗi (private mode), **When** load, **Then** trang vẫn render theo `prefers-color-scheme`, không crash
- **Given** theme dark, **Then** chữ trên nút primary dùng `--color-primary-fg` = màu tối (xem 3.4)

### AC-3 — Button
- **Given** `variant` ∈ {primary, secondary, tertiary, destructive} × `size` ∈ {sm, md, lg}, **When** render, **Then** áp đúng class
- **Given** `disabled` hoặc `loading`, **When** click, **Then** `onClick` **không** được gọi
- **Given** `loading`, **Then** có Spinner, `aria-busy="true"`, **kích thước không đổi** (chống layout shift — DESIGN.md yêu cầu)
- **Then** mặc định `type="button"`, forwardRef tới `<button>`, `className` truyền vào được merge không bị ghi đè
- **Then** chiều cao: `md` = 44px desktop / 48px mobile (DESIGN.md §Components.1)

### AC-4 — Input (text / email / password / otp)
- **Given** có `label`, **Then** `<label htmlFor>` khớp `id` input; không truyền `id` thì id sinh tự động vẫn khớp
- **Then** **không bao giờ** dùng placeholder thay label (handoff §6 Component visual rules)
- **Given** có `error`, **Then** input có `aria-invalid="true"`, `aria-describedby` trỏ tới phần tử lỗi, phần tử lỗi có `role="alert"`
- **Given** có `helpText` và không có `error`, **Then** `aria-describedby` trỏ tới help; **không** có `aria-invalid`
- **Given** `type="otp"`, **Then** `inputMode="numeric"`, `autoComplete="one-time-code"`, `maxLength` cấu hình được, dùng tabular numbers
- **Given** `type="password"`, **Then** có nút hiện/ẩn với `aria-label` và `aria-pressed`

### AC-5 — Select / Checkbox / Radio
- **Select**: label liên kết, option render từ prop, placeholder là `<option disabled>` không phải label, error như AC-4
- **Checkbox**: label click được, `indeterminate` set qua ref (không phải attribute), error, disabled
- **RadioGroup**: `role="radiogroup"` + `aria-labelledby` trỏ legend; các radio chung `name`; chỉ 1 checked
- Cả 3: focus-visible ring dùng token

### AC-6 — Alert / Toast
- **Alert**: variant `info|success|warning|error`; `error` và `warning` có `role="alert"`, `info`/`success` có `role="status"`; dismiss được nếu truyền `onDismiss`, nút dismiss có `aria-label`
- **Toast**: `ToastProvider` + hook `useToast`; gọi `toast({variant, title})` thì toast xuất hiện; auto-dismiss sau `duration` (dùng fake timer trong test); nhiều toast xếp hàng không ghi đè nhau; container có `aria-live="polite"`
- **Given** không bọc `ToastProvider`, **When** gọi `useToast`, **Then** ném lỗi rõ ràng chứ không fail im lặng

### AC-7 — Badge
- **Then** variant phủ đủ **9 trạng thái nghiệp vụ**: `lobby` `queued` `admitted` `holding` `paid` `pending` `expired` `soldout` `failed`, **cộng `neutral` làm giá trị mặc định** (tổng 10) — chốt 2026-10-02 sau khi test design phát hiện spec ghi 9 còn plan ghi 10
- **Then** **mọi** badge render text; **không** truyền tin chỉ bằng màu (handoff §5 nguyên tắc 7, §6 Component visual rules)
- **Then** radius là pill (ngoại lệ duy nhất dùng full-radius theo DESIGN.md §Shapes)

### AC-8 — Card / Spinner
- **Card**: `header` / `footer` tuỳ chọn, chỉ render slot khi được truyền; radius 16px; elevation cấp 1
- **Spinner**: `role="status"`, `aria-label` mặc định ghi đè được, size sm/md/lg, tôn trọng `prefers-reduced-motion`

### AC-9 — Accessibility & focus
- **Given** điều hướng bàn phím, **When** focus Button/Input/Select/Checkbox/Radio, **Then** có focus ring dùng token qua `:focus-visible` (không dùng `outline: none` trơ)
- **Then** tương phản ≥ 4.5:1 cho text thường ở **cả** light và dark — số liệu tính sẵn ở 3.2/3.4, ghi lại trong `evidence.md`
- **Then** `--color-accent` không xuất hiện ở bất kỳ thuộc tính `color` nào (chỉ border/background/fill) — kiểm bằng grep

### AC-10 — Test & coverage
- **Then** mọi component trong `components/ui` có ≥ 1 test file, toàn bộ pass, không `skip` / `only`
- **Then** `npm run test:coverage` pass ngưỡng: lines/functions/statements ≥ 70%, branches ≥ 60%
- **Then** `src/lib/queue-client.test.ts` vẫn **11/11 pass**, `src/app/page.test.tsx` vẫn pass **mà không bị sửa**

### AC-11 — Build
- **Given** repo sạch, **When** `npm run build` trong `apps/web`, **Then** build thành công; CSS output chứa utility class đã dùng (xác nhận Tailwind v4 nhận diện được nội dung)

### AC-12 — Lint + typecheck trong CI *(AC gốc Jira #3)*
- **Then** `npm run lint` và `npm run typecheck` pass 0 error; đã có trong job `lint-web` của `ci.yml`
- **Then** `make test` từ thư mục gốc pass

### AC-13 — CD frontend
- **Then** `.github/workflows/cd-web.yml` tồn tại, trigger khi push `main` (và `workflow_dispatch`), **không** chạy trên PR từ fork
- **Then** job build image từ `deploy/docker/web.Dockerfile` và push `ghcr.io/<owner>/eventflow-web` với tag `sha-<short>` + `latest`, dùng `permissions: packages: write` + `GITHUB_TOKEN` (không secret ngoài)
- **Then** workflow có `concurrency` group để push liên tiếp không chạy chồng
- **Then** `deploy/docker/web.Dockerfile` copy `package-lock.json` và dùng `npm ci` thay `npm install`; `docker build -f deploy/docker/web.Dockerfile .` thành công tại local
- **Then** YAML hợp lệ (parse được bằng công cụ, không chỉ đọc mắt)

## 5. Rủi ro

| Rủi ro | Ảnh hưởng | Giảm thiểu |
|---|---|---|
| Tailwind v4 cấu hình CSS-first (`@theme`), khác hẳn v3 | Làm theo tài liệu v3 → token không vào Tailwind | `@import "tailwindcss"` + `@theme`, không tạo `tailwind.config.js`; verify bằng CSS output thật |
| FOUC khi dark mode dùng `localStorage` | Nhấp nháy sai màu — lỗi thấy bằng mắt, không test nào bắt nếu không viết riêng | Inline script trong `<head>` trước hydration + test riêng |
| Thêm ~12 component mới | Kéo coverage xuống dưới 70% → **đỏ CI của cả team** | Test viết cùng lúc với component; chạy `test:coverage` trước khi push, không chỉ `npm test` |
| Palette dark là tự dẫn xuất | Design có thể bác bỏ | Token 1 file duy nhất → đổi giá trị không phải sửa component; đánh dấu rõ là đề xuất |
| Amber `#E6A23C` fail contrast (3.2) | Nếu làm theo nguyên văn DESIGN.md là vi phạm WCAG | Tách `--color-accent` (viền/nền) khỏi `--color-warning-fg` (chữ); grep chặn ở AC-9 |
| CD push image lên GHCR | Tạo tài nguyên public ngoài repo | Chỉ trigger trên `main`, dùng `GITHUB_TOKEN`, không secret ngoài; không có bước deploy lên môi trường chạy |
| Teammate force-push / merge song song | Conflict `package.json`, `package-lock.json`, `ci.yml` | `git fetch` + rebase lên `origin/develop` trước khi mở PR |

## 6. Điểm cần người dùng xác nhận

1. **Cắt scope**: route groups + OpenAPI client không nằm trong PR này → **EVF-1801 chưa đóng hoàn toàn**. Đã nêu, chờ xác nhận khi review PR.
2. **Palette dark là giá trị mình đề xuất**, không phải từ Design. Cần Design xác nhận.
3. **Amber fail contrast** — cần báo lại Design để sửa tài liệu (mục 3.2).
4. **GHCR image sẽ public** theo mặc định của repo. Nếu muốn private thì đổi setting package sau khi push lần đầu.

## 7. Definition of Done

- Mọi AC có bằng chứng pass trong `traceability.md`
- Không còn blocker/major mở
- `npm run lint`, `npm run typecheck`, `npm run test:coverage`, `npm run build`, `make test`, `docker build -f deploy/docker/web.Dockerfile .` pass — kèm output thô trong `evidence.md`
- Không hồi quy so với `baseline.md` (12/12 test cũ vẫn pass), không test nào bị xoá/skip/làm yếu
- Diff chỉ nằm trong file nêu ở `plan.md`, hoặc có giải thích
