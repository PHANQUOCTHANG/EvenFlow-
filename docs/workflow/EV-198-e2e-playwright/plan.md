# EV-198 — Kế hoạch thực thi (Plan)

Đầu vào: `spec.md` (AC-1..AC-12). Base: `develop` @ `51c4729`. Baseline unit: 1528 test / 93 file.

## 1. Hợp đồng chốt trước (test viết theo các hợp đồng này)

### 1.1 `apps/web/src/lib/queue-token.ts` (mới) — sửa B1

```ts
/** Khoá sessionStorage DUY NHẤT cho queue token của một sự kiện. */
export function queueTokenKey(eventId: string): string;      // => `ef:queue:token:${eventId}`
export function readQueueToken(eventId: string): string | null; // null khi SSR / storage ném lỗi / không có
export function saveQueueToken(eventId: string, token: string): void; // nuốt lỗi storage
export function clearQueueToken(eventId: string): void;           // nuốt lỗi storage
```

- Giữ giá trị khoá hiện tại của phòng chờ (`ef:queue:token:`) để token của khách đang ở trong hàng không mất
  khi deploy.
- `components/queue/waiting-room.tsx` dùng `readQueueToken` / `saveQueueToken` / `clearQueueToken` thay cho
  `tokenKey` cục bộ. `hooks/use-hold-timer.ts` dùng `readQueueToken`, xoá `QUEUE_STORAGE_PREFIX`.

### 1.2 `apps/web/src/lib/api-base.ts` (mới) — sửa B2

```ts
/** Gốc URL của API backend. Đọc `process.env.NEXT_PUBLIC_API_BASE` (Next inline lúc build), mặc định "". */
export function apiBase(): string;
```

- `lib/hold-client.ts` `requestHold`: `apiBaseUrl` mặc định = `apiBase()` (thay `""`).
- `lib/payment-client.ts` `initiatePayment` và `getOrderDetails`: `apiBaseUrl` mặc định = `apiBase()`.
- Truyền tường minh `apiBaseUrl` vẫn thắng (test cũ dựa vào điều này).
- Không đụng `queue-client.ts` / `queue-status.ts` / `use-queue-status.ts` (đã đọc env đúng).
- Hệ quả cần nêu trong review: nhánh mock fallback của `initiatePayment` (`!apiBaseUrl && NODE_ENV !== "production"`)
  không còn kích hoạt khi env được đặt. Nhánh `method === "sandbox"` khi lỗi mạng giữ nguyên.

### 1.3 `components/checkout/checkout-view.tsx` — sửa B3

URL sang thanh toán thêm `tierId` và `quantity` của hold đang active:
`/checkout/{eventId}/payment?orderId=…&holdId=…&tierId=…&quantity=…` (mọi giá trị qua `encodeURIComponent`
hoặc `URLSearchParams`). `payment/page.tsx` đã đọc `tierId` / `quantity` — không sửa.

### 1.4 E2E (`tests/e2e`)

- Biến môi trường: `BASE_URL` (web, mặc định `http://localhost:3000`), `E2E_API_BASE` (API giả, mặc định
  `http://localhost:8080` để chạy được cả trong nightly compose). Web phải được build với
  `NEXT_PUBLIC_API_BASE` = `E2E_API_BASE`.
- `tests/e2e/fixtures/api-mock.ts` (mới): fixture Playwright mở rộng `test`, cung cấp `api` với:
  - đăng ký handler theo method + path cho origin `E2E_API_BASE`;
  - ghi lại mọi request (method, url, headers, body JSON) để assert;
  - request tới `E2E_API_BASE` không có handler, hoặc request `/v1/**` tới origin `BASE_URL`, được ghi vào
    danh sách vi phạm và **làm fail test ở afterEach** với URL cụ thể (AC-7). Route trả 599 kèm thông báo để
    UI không treo.
- `tests/e2e/specs/buy-flow.spec.ts` (mới): AC-1..AC-6.
  - Jitter trang sự kiện (AC-8): `page.addInitScript` cố định `Math.random` chỉ trong phạm vi cần thiết
    hoặc cách tất định tương đương; không dùng `waitForTimeout` để chờ nghiệp vụ.
  - Status: lần đầu `QUEUED` (rank > 0), sau đó `ADMITTED`. Hook nâng lên SSE khi `poll_after_ms <= 3000`
    (`hooks/use-queue-status.ts:158`): hoặc trả `poll_after_ms` > 3000 với ít nhất 1 lần poll, hoặc giả lập
    luôn `GET …/queue/stream` (`text/event-stream`). Người viết test chọn và ghi lý do.
  - `/api/time` của web là thật (không giả lập).
- `tests/e2e/specs/smoke.spec.ts`: xoá `test.skip` luồng đầy đủ (đã thay bằng `buy-flow.spec.ts`), giữ test trang chủ.
- `tests/e2e/playwright.config.ts`: `retries: 0` (AC-8). Thêm `package-lock.json` để CI dùng `npm ci`.

### 1.5 CI — `.github/workflows/e2e-web.yml` (mới)

- Trigger: `pull_request` và `push` vào `develop`, `main`, `paths`: `apps/web/**`, `tests/e2e/**`,
  `.github/workflows/e2e-web.yml`; thêm `workflow_dispatch`.
- `permissions: contents: read`; action pin giống `ci.yml` (cùng version/SHA đang dùng ở repo).
- Node 22; `apps/web`: `npm ci`, `NEXT_PUBLIC_API_BASE=http://localhost:8080 npm run build`, `npm run start`
  nền; chờ `http://localhost:3000` sẵn sàng (có timeout); `tests/e2e`: `npm ci`, cài chromium, `npx playwright test`.
- Upload `tests/e2e/report` khi fail. `timeout-minutes` hợp lý (≤ 20).
- Không sửa `ci.yml`, `e2e-nightly.yml`.

## 2. Thứ tự

1. Test design (agent riêng): unit test B1–B3 (Vitest, cạnh source) + toàn bộ `tests/e2e/**` + `traceability.md`
   (bản đầu) + `baseline.md` (chạy trên code cũ: unit mới đỏ ở đâu, E2E đỏ ở bước nào).
2. Implementation: `queue-token.ts`, `api-base.ts`, sửa 5 file nguồn (waiting-room, use-hold-timer, hold-client,
   payment-client, checkout-view), workflow CI. Không sửa test do bước 1 viết; test sai → báo lại.
3. Gates: `apps/web` lint/typecheck/test/build; E2E chạy trên build production; `--repeat-each=20 --retries=0`.
4. Code review, challenge, verify (agent độc lập) → `code-review.md`, `challenge.md`, `evidence.md`.
5. Docs: `traceability.md` hoàn chỉnh, `docs/kiem-thu/README.md`.

## 3. File được phép đổi

`apps/web/src/lib/{queue-token,api-base}.ts` (+ `.test.ts`), `apps/web/src/components/queue/waiting-room.tsx`,
`apps/web/src/hooks/use-hold-timer.ts`, `apps/web/src/lib/{hold-client,payment-client}.ts`,
`apps/web/src/components/checkout/checkout-view.tsx`, test Vitest cạnh các file đó, `tests/e2e/**`,
`.github/workflows/e2e-web.yml`, `docs/workflow/EV-198-e2e-playwright/**`, `docs/kiem-thu/README.md`.

## 4. Rủi ro

| Rủi ro | Giảm thiểu |
|---|---|
| Đổi mặc định `apiBaseUrl` làm hỏng test unit cũ của hold/payment (đang dựa vào URL tương đối) | Vitest không đặt `NEXT_PUBLIC_API_BASE` → `apiBase()` = `""`, hành vi test cũ giữ nguyên; gate so với baseline |
| E2E flaky do jitter, timer, SSE | Cố định nguồn ngẫu nhiên; chờ bằng `expect(...).toBeVisible()` / `waitForURL`; chạy 20 lần |
| `page.route` không bắt được request do service worker / prefetch | Không có service worker; chỉ bắt `fetch` từ client; vi phạm được ghi và fail rõ ràng |
| Đổi khoá token làm khách đang trong hàng mất token khi deploy | Giữ nguyên khoá cũ của phòng chờ |
| Workflow mới tốn phút CI | Lọc `paths`; chỉ chromium; build một lần |

## 5. Rollback

Revert merge commit. Không có migration, không đổi dữ liệu lưu trữ (khoá sessionStorage giữ nguyên).

## 6. Sửa sau plan review vòng 1 (`plan-review.md`: PASS WITH CHANGES)

| Finding | Thay đổi vào kế hoạch |
|---|---|
| F1 | Test design cập nhật 2 test đang seed khoá cũ (`use-hold-timer.test.tsx:9`, `checkout-view.test.tsx:42`) sang hợp đồng mới, ghi vào `baseline.md`. Implementation vẫn không sửa test. |
| F2 | Fixture trả lời preflight `OPTIONS` và thêm CORS vào **mọi** response (kể cả 599): `Allow-Origin` = đúng origin `BASE_URL`, `Allow-Credentials: true`, `Allow-Headers` gồm `Content-Type, Idempotency-Key, X-Queue-Token, X-Identity-Id, If-None-Match, Accept`, `Expose-Headers: ETag, X-Poll-After-Ms, Retry-After`. |
| F3 | Mock payments trả `redirect_url` có marker riêng; test assert trình duyệt tới đúng URL đó. `pageerror` và console nhắc CORS bị tính là vi phạm. |
| F4 | Hợp đồng GET order: `status: "PAID"`, `issued_tickets[].ticketCode` = giá trị riêng (vd. `E2E-TKT-0001`); test assert đúng giá trị đó. Thiếu mapping snake_case → camelCase của `issued_tickets` ghi là nợ. |
| F5 | Bảng hợp đồng endpoint → response nằm trong `traceability.md` / `api-mock.ts`; hold `expires_at` tính lúc request (now + 10 phút), có `order_id`, `hold_id`. |
| F6 | `e2e-web.yml` có input `workflow_dispatch` `repeat_each`; chạy một lần với 20 và ghi link run vào `evidence.md`, cùng link run của PR. Gate grep `tests/e2e/**` chặn `waitForTimeout` / `.skip` / `.only` / `.todo`. |
| F7 | `apiBase()` viết đúng nguyên văn `return process.env.NEXT_PUBLIC_API_BASE ?? "";` trong thân hàm (không destructure, không hằng module). Unit test dùng `vi.stubEnv` + `vi.unstubAllEnvs`. |
| F8 | Jitter: `page.addInitScript(() => { Math.random = () => 0; })`. Không dùng `page.clock`. |
| F9 | Dùng polling (`poll_after_ms` 3500), không SSE; gọi `/queue/stream` là vi phạm. |
| F10 | Route bắt mọi `**/v1/**` ở origin khác → vi phạm (phát hiện lệch giữa `NEXT_PUBLIC_API_BASE` lúc build và `E2E_API_BASE`). |
| F11 | `retries: 0`, `trace: "retain-on-failure"`; upload `test-results/` và `report/`. Nightly cũng chạy `retries: 0` (cùng config). |
| F12 | Action pin theo tag như `ci.yml` (`actions/upload-artifact@v4`); cache npm cho cả hai lockfile; chờ sẵn sàng bằng `/api/healthz`. `output: "standalone"` → chạy `node .next/standalone/server.js` (copy `public` và `.next/static` như `web.Dockerfile`) thay vì `next start`. |
| F13 | Thêm hợp đồng: `export function buildPaymentUrl(eventId, hold: { orderId, holdId, ticketTypeId, quantity }): string` trong `checkout-view.tsx`, trả URL tương đối có `orderId`, `holdId`, `tierId`, `quantity`. Unit test gọi trực tiếp. |
| F14 | File được phép đổi thêm: `apps/web/src/hooks/use-hold-timer.test.tsx`, `apps/web/src/components/checkout/checkout-view.test.tsx`, `apps/web/src/lib/{hold-client,payment-client}.test.ts`. Lockfile `tests/e2e` sinh với `@playwright/test@1.49.1`. |
| F15 | Sửa tham chiếu trong spec thành `payment/page.tsx:44-45`. |
