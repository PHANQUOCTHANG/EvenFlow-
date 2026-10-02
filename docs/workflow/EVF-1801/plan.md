# EVF-1801 — Plan

Base `fa8ca69` · branch `feature/EVF-1801-design-system` · tier Standard. Đọc cùng `spec.md` (AC) và `baseline.md`.

## 1. Thứ tự thực hiện

| # | Bước | Lý do phải theo thứ tự này |
|---|---|---|
| 1 | Dependency + PostCSS + tokens.css + globals.css | Mọi component phụ thuộc token; không có token thì không viết được class |
| 2 | `lib/cn.ts` | Mọi component dùng để merge className |
| 3 | Theme (script chống FOUC, `useTheme`, `ThemeToggle`) + `layout.tsx` | Phải có `data-theme` trước khi test dark mode của component |
| 4 | Primitive không phụ thuộc gì: `Spinner`, `Badge`, `Card` | Button cần Spinner |
| 5 | `Button` | Cần Spinner (state loading) |
| 6 | Field layer: `FieldHelp`, `FieldError`, `useFieldIds` → `Input`, `Select`, `Checkbox`, `Radio` | 4 field component dùng chung logic id/aria |
| 7 | `Alert` → `Toast` + `ToastProvider` | Toast tái dùng phần hiển thị của Alert |
| 8 | `index.ts` barrel + refactor `page.tsx` | Cần toàn bộ component đã xong |
| 9 | Gates: lint, typecheck, test:coverage, build | Deterministic, chạy trước mọi review |
| 10 | CD: sửa `web.Dockerfile`, thêm `cd-web.yml`, `docker build` local | Độc lập với phần UI, làm sau để không chen vào gate UI |

## 2. File sẽ thay đổi

### Thêm mới

```
apps/web/postcss.config.mjs
apps/web/src/styles/tokens.css
apps/web/src/lib/cn.ts                       + cn.test.ts
apps/web/src/lib/theme.ts                    (hằng số + đọc/ghi localStorage an toàn)
apps/web/src/components/theme/theme-script.tsx
apps/web/src/components/theme/theme-toggle.tsx  + theme-toggle.test.tsx
apps/web/src/hooks/use-theme.ts              + use-theme.test.tsx
apps/web/src/components/ui/spinner.tsx       + spinner.test.tsx
apps/web/src/components/ui/badge.tsx         + badge.test.tsx
apps/web/src/components/ui/card.tsx          + card.test.tsx
apps/web/src/components/ui/button.tsx        + button.test.tsx
apps/web/src/components/ui/field.tsx         + field.test.tsx       (FieldHelp, FieldError, useFieldIds)
apps/web/src/components/ui/input.tsx         + input.test.tsx
apps/web/src/components/ui/select.tsx        + select.test.tsx
apps/web/src/components/ui/checkbox.tsx      + checkbox.test.tsx
apps/web/src/components/ui/radio.tsx         + radio.test.tsx
apps/web/src/components/ui/alert.tsx         + alert.test.tsx
apps/web/src/components/ui/toast.tsx         + toast.test.tsx
apps/web/src/components/ui/index.ts
.github/workflows/cd-web.yml
docs/workflow/EVF-1801/{traceability.md,evidence.md}
```

### Sửa

```
apps/web/package.json          + tailwindcss, @tailwindcss/postcss (devDependencies)
apps/web/package-lock.json     (npm install sinh ra)
apps/web/src/app/globals.css   bỏ màu hard-code, import tokens + tailwind
apps/web/src/app/layout.tsx    font Inter, ThemeScript, ToastProvider, class từ token
apps/web/src/app/page.tsx      bỏ inline style, dùng component mới — GIỮ <h1>EventFlow</h1>
deploy/docker/web.Dockerfile   npm ci + copy package-lock.json
```

### KHÔNG được sửa

```
apps/web/src/app/page.test.tsx        — baseline assert <h1>EventFlow</h1>, phải pass nguyên trạng
apps/web/src/lib/queue-client.ts(.test.ts) — không liên quan, phải giữ 11/11 pass
apps/web/vitest.config.ts             — không hạ ngưỡng coverage để test dễ pass
.github/workflows/ci.yml              — CI web đã đủ; CD đi file riêng
```

> Nếu bắt buộc phải đụng 1 trong 4 nhóm trên, **dừng và báo**, không tự sửa.

## 3. Hợp đồng API component (chốt trước khi viết test)

Mọi component: `forwardRef`, spread `...rest` xuống phần tử gốc, `className` merge qua `cn()` (không ghi đè), export cả component và type props.

```ts
// lib/cn.ts
export function cn(...parts: Array<string | false | null | undefined>): string

// lib/theme.ts
export type Theme = "light" | "dark"
export const THEME_STORAGE_KEY = "eventflow-theme"
export function readStoredTheme(): Theme | null      // bọc try/catch, lỗi -> null
export function storeTheme(t: Theme): void           // bọc try/catch, lỗi -> no-op
export function systemTheme(): Theme                 // matchMedia, không có -> "light"

// hooks/use-theme.ts
export function useTheme(): {
  theme: Theme
  setTheme: (t: Theme) => void
  toggle: () => void
}
// đọc data-theme hiện tại trên <html> khi mount; set thì ghi cả attribute và localStorage

// components/theme/theme-script.tsx
export function ThemeScript(): JSX.Element           // <script dangerouslySetInnerHTML> chạy trước hydration
// components/theme/theme-toggle.tsx
export function ThemeToggle(props: { className?: string }): JSX.Element
// <button aria-label="Chuyển giao diện sáng/tối"> aria-pressed = theme === "dark"

// ui/spinner.tsx
export type SpinnerSize = "sm" | "md" | "lg"
export interface SpinnerProps extends React.SVGAttributes<SVGSVGElement> {
  size?: SpinnerSize        // default "md"
  label?: string            // default "Đang tải"
}
// <svg role="status" aria-label={label}>

// ui/badge.tsx
export type BadgeVariant =
  | "lobby" | "queued" | "admitted" | "holding"
  | "paid" | "pending" | "expired" | "soldout" | "failed" | "neutral"
export interface BadgeProps extends React.HTMLAttributes<HTMLSpanElement> {
  variant?: BadgeVariant    // default "neutral"
  children: React.ReactNode // BẮT BUỘC — badge không bao giờ chỉ có màu
}

// ui/card.tsx
export interface CardProps extends React.HTMLAttributes<HTMLDivElement> {
  header?: React.ReactNode
  footer?: React.ReactNode
}
// header render trong <div data-slot="header">, footer trong <div data-slot="footer">,
// children trong <div data-slot="body">; slot vắng thì KHÔNG render thẻ rỗng

// ui/button.tsx
export type ButtonVariant = "primary" | "secondary" | "tertiary" | "destructive"
export type ButtonSize = "sm" | "md" | "lg"
export interface ButtonProps extends React.ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: ButtonVariant   // default "primary"
  size?: ButtonSize         // default "md"
  loading?: boolean         // default false
}
// type default "button"; loading -> disabled + aria-busy="true" + <Spinner/>;
// children LUÔN render (kể cả loading) để không layout shift;
// disabled||loading -> onClick không được gọi

// ui/field.tsx
export interface FieldIds { inputId: string; helpId: string; errorId: string }
export function useFieldIds(providedId?: string): FieldIds
export function describedBy(ids: FieldIds, opts: { hasHelp: boolean; hasError: boolean }): string | undefined
export function FieldLabel(props: { htmlFor: string; children: React.ReactNode; required?: boolean }): JSX.Element
// CHOT (bo sung 2026-10-02, truoc do con de ngo): dau hieu bat buoc la mot <span aria-hidden="true">*</span>
// nam NGOAI <label>, la sibling cua no. textContent cua <label> PHAI dung bang ten field, khong kem "*".
// Ly do: neu "*" nam trong <label> thi label text thanh "Ten field *", lam vo moi truy van theo
// label (getByLabelText) tren field required — da lam do 2 test cua input/select.
export function FieldHelp(props: { id: string; children: React.ReactNode }): JSX.Element
export function FieldError(props: { id: string; children: React.ReactNode }): JSX.Element
// FieldError: role="alert"

// ui/input.tsx
export type InputType = "text" | "email" | "password" | "otp"
export interface InputProps extends Omit<React.InputHTMLAttributes<HTMLInputElement>, "type"> {
  label: string             // BẮT BUỘC — không dùng placeholder thay label
  type?: InputType          // default "text"
  error?: string
  helpText?: string
  otpLength?: number        // default 6, chỉ dùng khi type="otp"
}
// error -> aria-invalid="true" + aria-describedby trỏ errorId; không error -> KHÔNG có aria-invalid
// type="otp" -> inputMode="numeric" autoComplete="one-time-code" maxLength=otpLength, class tabular
// type="password" -> nút toggle aria-label "Hiện mật khẩu"/"Ẩn mật khẩu" + aria-pressed,
//   bấm thì input type đổi text <-> password

// ui/select.tsx
export interface SelectOption { value: string; label: string; disabled?: boolean }
export interface SelectProps extends React.SelectHTMLAttributes<HTMLSelectElement> {
  label: string
  options: SelectOption[]
  placeholder?: string      // render <option value="" disabled> — KHÔNG thay cho label
  error?: string
  helpText?: string
}

// ui/checkbox.tsx
export interface CheckboxProps extends Omit<React.InputHTMLAttributes<HTMLInputElement>, "type"> {
  label: string
  indeterminate?: boolean    // set qua ref .indeterminate, KHÔNG phải attribute
  error?: string
  helpText?: string
}

// ui/radio.tsx
export interface RadioOption { value: string; label: string; disabled?: boolean }
export interface RadioGroupProps {
  name: string
  legend: string
  options: RadioOption[]
  value?: string
  onValueChange?: (v: string) => void
  error?: string
  helpText?: string
  className?: string
}
// <fieldset role="radiogroup" aria-labelledby={legendId}>, mọi radio cùng name

// ui/alert.tsx
export type AlertVariant = "info" | "success" | "warning" | "error"
export interface AlertProps extends React.HTMLAttributes<HTMLDivElement> {
  variant?: AlertVariant    // default "info"
  title?: string
  onDismiss?: () => void    // có -> render nút aria-label="Đóng thông báo"
}
// role: "alert" cho warning|error, "status" cho info|success

// ui/toast.tsx
export interface ToastOptions {
  variant?: AlertVariant    // default "info"
  title: string
  description?: string
  duration?: number         // ms, default 5000; <= 0 thì không auto-dismiss
}
export function ToastProvider(props: { children: React.ReactNode }): JSX.Element
export function useToast(): { toast: (o: ToastOptions) => string; dismiss: (id: string) => void }
// container aria-live="polite"; nhiều toast xếp hàng, không ghi đè;
// useToast ngoài ToastProvider -> throw Error("useToast phải dùng trong ToastProvider")
```

## 4. Chi tiết kỹ thuật cần đúng

**Tailwind v4** — `globals.css`:
```css
@import "tailwindcss";
@import "../styles/tokens.css";
```
`postcss.config.mjs`: `{ plugins: { "@tailwindcss/postcss": {} } }`. **Không** tạo `tailwind.config.js`.
Token expose cho Tailwind bằng `@theme` trong `tokens.css` → sinh utility `bg-surface`, `text-fg-muted`, `rounded-control`…

**Dark mode** — `tokens.css`:
```css
:root            { /* light */ }
@media (prefers-color-scheme: dark) {
  :root:not([data-theme="light"]) { /* dark */ }
}
:root[data-theme="dark"] { /* dark */ }
```
`ThemeScript` đặt trong `<head>`, đọc `localStorage` trong try/catch, set `document.documentElement.dataset.theme`. Chạy sync trước paint.

**jsdom** — không implement `matchMedia`. `vitest.setup.ts` phải stub nó, nếu không mọi test chạm theme sẽ throw.

**Reduced motion** — Spinner: `@media (prefers-reduced-motion: reduce) { animation: none }`.

## 5. Rollback

Toàn bộ nằm trên branch riêng, chưa merge. Rollback = xoá branch. Không migration, không thay đổi dữ liệu, không thay đổi API. CD chỉ publish image mới có tag riêng, không ghi đè image đang chạy (chưa có môi trường chạy nào).

## 6. Điểm dừng báo người dùng

- `test:coverage` không đạt ngưỡng sau khi đã viết test cho mọi component → báo, **không** hạ ngưỡng trong `vitest.config.ts`
- Phải sửa `page.test.tsx` hoặc `queue-client.test.ts` → dừng, báo
- `docker build` fail vì lý do ngoài Dockerfile (mạng, base image) → báo, không đổi base image
