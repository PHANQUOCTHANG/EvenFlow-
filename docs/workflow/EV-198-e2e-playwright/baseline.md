# EV-198 — Baseline trên code cũ (bước Test design)

Code: `develop` @ `51c4729` (branch `feature/EV-198-e2e-playwright`), **chưa sửa B1–B3**, chỉ thêm/sửa test.
Ngày chạy: 2026-10-13. Máy: Windows 11, Node 22, Chromium headless shell 131 (Playwright 1.49.1).

## 1. Thay đổi test so với `develop`

- Mới: `apps/web/src/lib/queue-token.test.ts` (16), `apps/web/src/lib/api-base.test.ts` (4).
- Thêm vào cuối file: `lib/hold-client.test.ts` (+3), `lib/payment-client.test.ts` (+4),
  `hooks/use-hold-timer.test.tsx` (+2), `components/checkout/checkout-view.test.tsx` (+1 hành vi, +3 `buildPaymentUrl`).
- **Sửa test cũ theo plan-review F1** (khoá token mới `ef:queue:token:`, đúng hợp đồng plan 1.1):
  - `hooks/use-hold-timer.test.tsx:9-10` — `queueTokenKey` từ `evenflow_queue_token_${eventId}` → `ef:queue:token:${eventId}`
    (ảnh hưởng 4 test: tạo hold thành công, sold_out, ambiguous, bấm nhiều lần).
  - `components/checkout/checkout-view.test.tsx:42-43` — `beforeEach` ghi `ef:queue:token:evt-test-1`
    (ảnh hưởng 2 test: gọi requestHold với Idempotency-Key, bấm nhiều lần nhanh).
  - Ghi bằng khoá thuần (không import `queue-token.ts`) để hai file vẫn nạp được trên code cũ.
- E2E: `tests/e2e/fixtures/api-mock.ts`, `tests/e2e/specs/buy-flow.spec.ts` (mới); `specs/smoke.spec.ts` bỏ `test.skip`;
  `playwright.config.ts` `retries: 0`, `trace: "retain-on-failure"`, `timeout: 60s`, `expect.timeout: 10s`;
  `package-lock.json` (`@playwright/test` 1.49.1); `tests/e2e/.gitignore` (bổ sung `node_modules/`, `blob-report/` cạnh `report/`, `test-results/` có sẵn).

## 2. Unit test — `apps/web`

Lệnh:

```bash
cd apps/web
npx vitest run
```

Tóm tắt kết quả:

```
Test Files  6 failed | 89 passed (95)
     Tests  14 failed | 1527 passed (1541)
```

- 2 file **không nạp được** (đúng dự kiến — module chưa tồn tại, 20 test không chạy):
  `src/lib/queue-token.test.ts` (`./queue-token`), `src/lib/api-base.test.ts` (`./api-base`).
- 14 test đỏ:

| File | Test | Lý do đỏ |
|---|---|---|
| `hooks/use-hold-timer.test.tsx` | `useHoldTimer — đọc token phòng chờ (EV-198, B1) › token phòng chờ lưu ở khoá ef:queue:token:{eventId} …` | B1: `expected 'NOT_ADMITTED' not to be 'NOT_ADMITTED'` — hook đọc khoá `evenflow_queue_token_` |
| idem | `tạo hold thành công…`, `chuyển sang sold_out…`, `chuyển sang ambiguous…`, `Bấm đặt nhiều lần không tạo hold trùng` | B1 (test cũ đã chuyển khoá, F1): không tìm thấy token → NOT_ADMITTED |
| `components/checkout/checkout-view.test.tsx` | `khi bấm 'Giữ vé và thanh toán', gọi requestHold…`, `bấm đặt nhiều lần nhanh trên UI…` | B1 (F1): `requestHold` không được gọi |
| idem | `CheckoutView — URL sang thanh toán … (EV-198, B3) › bấm 'Tiếp tục thanh toán' chuyển sang …` | B3: `expected null to be 'tier-vip'` — URL chỉ có `orderId`, `holdId` |
| idem | `buildPaymentUrl (EV-198, B3)` × 3 | B3: `buildPaymentUrl` chưa được export (TypeError khi gọi) |
| `lib/hold-client.test.ts` | `requestHold không truyền apiBaseUrl gọi ${NEXT_PUBLIC_API_BASE}/v1/events/{id}/holds` | B2: nhận `/v1/events/evt-tech-summit-2026/holds` (tương đối) |
| `lib/payment-client.test.ts` | `initiatePayment không truyền apiBaseUrl …`, `getOrderDetails không truyền apiBaseUrl …` | B2: URL tương đối |

- Test cũ: 1528 test trên `develop`; trừ 6 test cũ đã chuyển khoá (F1, đỏ có chủ đích) thì **1522 test cũ còn lại xanh**.
  1527 xanh = 1522 test cũ + 5 test mới dạng "bảo vệ" (apiBaseUrl tường minh thắng ×3, env trống giữ URL tương đối,
  prop queueToken ưu tiên).
- `stderr` "Not implemented: navigation (except hash changes)" đến từ `public-header.test.tsx` / `workspace-shell.test.tsx`
  (có sẵn, không liên quan).

Kỳ vọng sau khi sửa: 1528 + 13 (mới trong file cũ) + 20 (2 file mới) = **1561 test, 0 fail**.

## 3. E2E — Playwright trên build production của code cũ

Lệnh:

```bash
cd apps/web
NEXT_PUBLIC_API_BASE=http://localhost:8080 npm run build
npm run start            # nền, port 3000 (cảnh báo "next start does not work with output: standalone" — vẫn chạy)
cd ../../tests/e2e
npm install              # tạo package-lock.json
npx playwright install chromium
npx playwright test
```

Ghi chú build: `npm` (Git Bash trên Windows) in `Segmentation fault` **sau khi** `next build` đã xong (có `.next/BUILD_ID`,
`localhost:8080` đã được inline trong chunk `waiting/[eventId]`) — lỗi thoát của npm trên máy này, không phải lỗi build.

Kết quả:

```
Running 6 tests using 6 workers
  ok  smoke.spec.ts › trang chu tai duoc
  ok  buy-flow.spec.ts › AC-7 › ghi vi pham kem URL cho request /v1/** toi origin web va request API khong co handler
  x   buy-flow.spec.ts › AC-7 › vi pham lam FAIL test o teardown (test nay phai fail)   [expected failure -> tính là pass]
  ok  buy-flow.spec.ts › AC-5: join tra 401 -> phong cho bao can dang nhap/OTP, khong chuyen trang
  x   buy-flow.spec.ts › AC-1..AC-4: xep hang -> duoc goi -> giu ve -> thanh toan -> nhan ve      (15.2s)
  x   buy-flow.spec.ts › AC-6: hold tra 409 -> bao hang ve da het, khong sang trang thanh toan   (15.3s)
  2 failed
  4 passed (17.4s)
```

Chạy lại lần 2: kết quả giống hệt (2 failed, 4 passed, 16.9s).

### Điểm đỏ từng test

| Test | Bước đỏ | Thông báo | Nguyên nhân |
|---|---|---|---|
| AC-1..AC-4 | `AC-1/AC-2: chon All-Access x2, giu ve -> dang giu ve co dong ho` (`buy-flow.spec.ts:191`) | `Timed out 10000ms waiting for expect(getByText('Thời gian giữ vé còn').first()).toBeVisible()` | **B1**: ảnh chụp cho thấy banner "Chưa được cấp quyền mua vé (BR-Q4)"; request hold **không bao giờ được gửi** (không có request nào tới `/holds`). Các bước trước xanh: trang sự kiện → phòng chờ thấy rank 42 → ADMITTED → tự chuyển `/checkout/evt-tech-summit-2026`, chọn All-Access × 2 (tóm tắt 5.000.000 đ) |
| AC-6 | `expect(soldOut).toBeVisible()` (`buy-flow.spec.ts:287`) | `Timed out 10000ms waiting for … getByRole('alert').filter({ hasText: 'Phần vé bạn chọn vừa được người khác mua hết' })` | **B1**: báo NOT_ADMITTED thay vì hết vé; hold không được gửi |

### Thăm dò thêm (chỉ để kiểm chứng test, đã xoá, không nằm trong repo)

Để biết test có đỏ đúng chỗ ở B2/B3 và các bước sau có đúng selector hay không, đã chạy một bản sao tạm của
`buy-flow.spec.ts` (thư mục `tests/e2e/probe`, đã xoá) với các "vá giả" ở phía trình duyệt:

1. Ghi thêm token vào khoá cũ `evenflow_queue_token_…` (giả lập đã sửa B1). Bước này **không chạy riêng**; suy luận từ
   code: hold sẽ đi tới **origin web** `http://localhost:3000/v1/events/evt-tech-summit-2026/holds` → fixture ghi vi phạm,
   trả 599 → `requestHold` ném `UNKNOWN` (599 không thuộc 502/503/504) → không có đồng hồ giữ vé, test đỏ ở bước giữ vé
   và teardown AC-7 liệt kê URL (B2).
2. Kết hợp (1) với route chuyển `/v1/**` của origin web vào handler giả (giả lập đã sửa B1 + B2): đỏ ở bước
   `AC-4: trang thanh toan hien dung hang ve…` — `getByText('All-Access Executive Pass')` không thấy; trang thanh toán hiện
   **Standard Pass × 1** (**B3**).
3. Thêm `page.goto(...payment?…&tierId=tier-all-access&quantity=2)` (giả lập đã sửa B3): **toàn bộ AC-1..AC-4 xanh**
   (trang thanh toán đúng, redirect có `e2e=paid-ok`, trang kết quả "Đã thanh toán thành công" và hiện đúng
   `E2E-TKT-0001`, `E2E-TKT-0002` — plan-review F4 không cần lách). AC-6 xanh 3/3.
   Bản thăm dò chạy `--repeat-each=10`: 40/40 test liên quan xanh (chỉ test AC-7 số 1 đỏ — đúng dự kiến vì bản thăm dò
   cố ý chuyển hướng `/v1/**` của web).

Server `next start` (PID 26976) đã được dừng sau khi chạy; không đụng tiến trình node khác.
