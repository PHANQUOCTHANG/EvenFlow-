# EV-188 — Accessibility (WCAG AA) + i18n vi/en (Spec)

| Mục | Giá trị |
|---|---|
| Jira | EV-188 (mã backlog **EVF-118**, issue 1809) — 3 SP — Sprint 5 |
| Branch | `feature/EV-188-a11y-i18n`, **xếp trên** `feature/EV-187-ops-war-room` (→ EV-198 → `develop` @ `51c4729`) |
| Tier | **Standard** (10 bước), frontend `apps/web` + `tests/e2e` |
| Quyết định phạm vi (người dùng, 2026-10-13) | Làm a11y trước (axe-core trong Playwright, sửa lỗi); i18n vi/en **gọn** để hạn chế đụng code của nhóm |

## 1. Bối cảnh

Jira AC: (1) axe-core 0 lỗi nghiêm trọng, chạy trong CI; (2) chuyển ngôn ngữ không reload trang.
Phạm vi Jira: rà WCAG AA các màn hình Buyer; tách chuỗi ra file ngôn ngữ vi/en.

Hiện trạng (khảo sát trên nhánh nền):
- Chưa có `axe-core` / `@axe-core/playwright` ở đâu. Hạ tầng E2E (EV-198) đã có: build production, giả lập backend
  (`tests/e2e/fixtures/api-mock.ts`), workflow `e2e-web.yml` chạy mỗi PR.
- Chưa có thư viện i18n; `<html lang="vi">` cứng trong `app/layout.tsx:24`; chuỗi nằm rải trong từng file
  (~330 dòng ở luồng mua vé, ~600 nếu tính auth + trợ lý).
- `/events` và `/events/[slug]` là ISR (`revalidate = 60`, `generateStaticParams`): gọi `cookies()` ở layout sẽ
  biến chúng thành dynamic — trái ý đồ phục vụ từ CDN.
- Formatter (`lib/format.ts`) cố ý không dùng ICU để server và client ra cùng chuỗi.

## 2. Tiêu chí nghiệm thu

### A. Accessibility

**AC-1 — Quét axe trên mọi màn hình Buyer.** Bộ Playwright mới quét bằng `@axe-core/playwright` với tag
`wcag2a`, `wcag2aa`, `wcag21a`, `wcag21aa`, ở **cả theme sáng và tối**, các trạng thái: trang chủ `/`; `/events`;
`/events/vietnam-tech-summit-2026`; `/waiting/evt-tech-summit-2026` khi QUEUED và khi join bị 401; `/checkout/…`
trước và sau khi giữ vé; trang thanh toán; trang kết quả (đơn đã thanh toán); `/auth/sign-in`.
Backend giả lập bằng fixture của EV-198.

**AC-2 — 0 vi phạm serious/critical.** Mọi vi phạm `impact` = `serious` hoặc `critical` làm test fail, thông báo
gồm rule id, selector và trang. Vi phạm `moderate`/`minor` được ghi vào báo cáo (attachment), không làm fail.
Không dùng `disableRules` / `exclude` để né vi phạm; nếu buộc phải loại trừ (vd. phần tử của bên thứ ba), phải
ghi lý do trong `traceability.md` và code-review chấp nhận.

**AC-3 — Sửa ở gốc.** Lỗi tìm được sửa trong component/token (không sửa riêng cho test). Mỗi loại lỗi sửa có ít
nhất một unit test (RTL) giữ lại, hoặc được chứng minh bằng chính bài quét axe nếu không kiểm được trong jsdom
(vd. tương phản màu).

**AC-4 — Chạy trong CI.** Bài quét nằm trong `tests/e2e/specs/`, chạy bằng workflow `e2e-web.yml` sẵn có (không sửa
`ci.yml`). Lặp `--repeat-each=5 --retries=0` không flaky.

### B. i18n

**AC-5 — Hạ tầng.** Từ điển có kiểu: `vi` là nguồn, `en` phải có **đủ khoá** (kiểm bằng TypeScript và một unit
test so khoá). Hook `useT()` / `useLocale()` qua `LocaleProvider` (client) đặt ở root layout.

**AC-6 — Chuyển ngôn ngữ không reload.** Nút chuyển ngôn ngữ trong header công khai (cạnh nút theme): bấm thì chữ
trên trang đổi ngay, `document.documentElement.lang` đổi theo, **không** điều hướng / reload (E2E kiểm bằng một
biến gắn trên `window` trước khi bấm vẫn còn sau khi bấm, và URL không đổi). Lựa chọn được nhớ qua reload.

**AC-7 — Không phá ISR, không lỗi hydration.** Không gọi `cookies()`/`headers()` ở root layout hay trang ISR;
`/events`, `/events/[slug]` vẫn là static/ISR trong output `next build`. Render đầu (server + client) luôn bằng
`vi`; locale đã lưu áp dụng sau khi mount. Không có lỗi hydration trên console (E2E).

**AC-8 — Phạm vi dịch.** Được dịch vi/en: khung chung (skip link, header công khai gồm nav + nút theme + nút ngôn
ngữ + nút menu, footer, `FocusShell`) và `EventActionPanel` (nút vào phòng chờ và các trạng thái). Mọi chuỗi còn
lại giữ tiếng Việt và được liệt kê là nợ theo từng file trong `traceability.md`.

**AC-9 — Nút ngôn ngữ truy cập được.** Là `<button>` có tên truy cập được bằng cả hai ngôn ngữ, cho biết ngôn ngữ
hiện tại (vd. `aria-pressed` hoặc nhãn "English / Tiếng Việt" có `lang` đúng trên chữ).

### C. Chung

**AC-10 — Không hồi quy.** `apps/web` lint/typecheck/test/build pass; test hiện có không bị sửa trừ khi khẳng định
đúng chuỗi đã chuyển vào từ điển (giữ nguyên giá trị tiếng Việt nên phần lớn không cần sửa).
E2E hiện có (smoke, buy-flow, ops-war-room) vẫn xanh.

## 3. Ngoài phạm vi

- Dịch toàn bộ luồng mua vé, auth, trợ lý AI, metadata trang (`generateMetadata`), dữ liệu mẫu sự kiện.
- Định dạng số/tiền/thời lượng theo locale (`formatVnd`, `spellDuration`) — ghi nợ.
- URL theo ngôn ngữ (`/en/...`), `hreflang`.
- Màn hình Ops/Organizer (không phải Buyer).
