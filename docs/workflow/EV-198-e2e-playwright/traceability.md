# EV-198 — Truy vết AC → test

Bản đầu viết ở bước Test design (kết quả trên code cũ: `baseline.md`). Cột trạng thái đã được bước xác minh độc lập
cập nhật trên code mới — số liệu thô ở `evidence.md`.

Đường dẫn: unit test nằm trong `apps/web/src/...`; E2E nằm trong `tests/e2e/...`.

## 1. Tiêu chí nghiệm thu

| AC | Nội dung | File test | Tên test / bước | Trạng thái |
|---|---|---|---|---|
| AC-1 | Luồng chính xanh: trang sự kiện → phòng chờ (QUEUED, rank) → ADMITTED → checkout → giữ vé có đồng hồ → thanh toán → kết quả đã thanh toán + mã vé | `tests/e2e/specs/buy-flow.spec.ts` | `AC-1..AC-4: xep hang -> duoc goi -> giu ve -> thanh toan -> nhan ve` (bước `AC-1: trang su kien -> phong cho …`, `AC-1/AC-2: chon All-Access x2, giu ve …`, `AC-1/AC-3: dien thong tin, thanh toan -> trang ket qua …`) | **ĐẠT (máy)** — verify 2026-10-13: standalone build, 1 lần 6/6, `--repeat-each=20` 120/120 (`evidence.md`) |
| AC-2 | Hold: `POST {API_BASE}/v1/events/evt-tech-summit-2026/holds`, `X-Queue-Token` = token join trả về, `Idempotency-Key` khác rỗng, body `{ticket_type_id:"tier-all-access", quantity:2}`; không có `/v1/...` lên origin web | `tests/e2e/specs/buy-flow.spec.ts` | bước `AC-2: hold dung token, dung dia chi, dung body` + teardown fixture (AC-7) | **ĐẠT (máy)** — như AC-1 (`evidence.md`) |
| AC-3 | Payments: `POST {API_BASE}/v1/events/…/payments` có `Idempotency-Key`, `order_id`/`hold_id` = giá trị hold, `amount` = 5.000.000 | `tests/e2e/specs/buy-flow.spec.ts` | bước `AC-3: request thanh toan dung don`; redirect phải có dấu `e2e=paid-ok` (chống nhánh sandbox giả, plan-review F3) | **ĐẠT (máy)** — như AC-1 (`evidence.md`) |
| AC-4 | Trang thanh toán: "All-Access Executive Pass", "2 vé", "5.000.000 đ" | `tests/e2e/specs/buy-flow.spec.ts` | bước `AC-4: trang thanh toan hien dung hang ve, so luong, tong tien` | **ĐẠT (máy)** — như AC-1 (`evidence.md`) |
| AC-5 | Join 401 → `alert` cần đăng nhập/OTP, không chuyển trang | `tests/e2e/specs/buy-flow.spec.ts` | `AC-5: join tra 401 -> phong cho bao can dang nhap/OTP, khong chuyen trang` | **ĐẠT (máy)** — 20/20 (`evidence.md`) |
| AC-6 | Hold 409 → báo hết vé, không sang thanh toán | `tests/e2e/specs/buy-flow.spec.ts` | `AC-6: hold tra 409 -> bao hang ve da het, khong sang trang thanh toan` | **ĐẠT (máy)** — 20/20 (`evidence.md`) |
| AC-7 | Giả lập không che lỗi: request `/v1/**` lên origin web, request API_BASE không handler, `/v1/**` lên origin khác, `pageerror`, console CORS → fail kèm URL | `tests/e2e/fixtures/api-mock.ts` (teardown `finalizeApiMock`: `context.unrouteAll({ behavior: "wait" })` có giới hạn 5s, rồi `assertNoViolations`), `tests/e2e/specs/buy-flow.spec.ts` | `AC-7: gia lap khong che loi › ghi vi pham kem URL cho request /v1/** toi origin web va request API khong co handler`; `AC-7: gia lap khong che loi › teardown cua fixture nem loi kem URL khi co request /v1/** toi origin web` (thay test `test.fail` cũ theo code-review C3/F-3: kiểm request bị chặn 599, vi phạm ghi đúng URL, gọi trực tiếp `finalizeApiMock` và kiểm lỗi ném ra chứa URL; đã thử đột biến: tắt route web-origin → đỏ (404 ≠ 599), bỏ `assertNoViolations` → đỏ) | **ĐẠT (máy)** — 2 test × 20 = 40/40 (`evidence.md`) |
| AC-8 | Ổn định: `--repeat-each=20 --retries=0`, không `waitForTimeout`, jitter tất định | `tests/e2e/specs/buy-flow.spec.ts`, `tests/e2e/playwright.config.ts` | `Math.random = () => 0` qua `addInitScript`; lần poll ADMITTED bị "cổng" giữ tới khi test thấy QUEUED; chờ hydrate bằng `__reactProps$`; `retries: 0` | **Máy: ĐẠT** (120/120, `--retries=0`, gate vệ sinh không khớp) — **CI: chờ chạy CI** (`workflow_dispatch repeat_each=20`) |
| AC-9 | Workflow CI mới | `.github/workflows/e2e-web.yml` (bước Implementation) | — | **chờ chạy CI** — workflow có, chưa push nên chưa có run |
| AC-10 | Không còn `test.skip` luồng đầy đủ; test trang chủ giữ nguyên | `tests/e2e/specs/smoke.spec.ts` | `trang chu tai duoc` (đã xoá `test.skip`) | **ĐẠT** — `trang chu tai duoc` xanh; gate vệ sinh không khớp (`evidence.md`) |
| AC-11 | Unit test B1–B3 đỏ trên code cũ, xanh trên code mới; ≥ 1528 + mới, 0 fail | xem mục 2 | — | **ĐẠT** — 95 file / 1564 test, 0 fail; lint/typecheck/build rc=0; đỏ trên code cũ: `baseline.md` |
| AC-12 | Ghi nhận trung thực | file này + `docs/kiem-thu/README.md` (bước Docs) | — | **ĐẠT một phần** — compose CHƯA ĐẠT đã ghi (mục 4), README đã cập nhật; còn thiếu ghi nợ (xem `evidence.md` §6) |

## 2. Unit test (Vitest) theo lỗi

| Lỗi / hợp đồng | File | Test | Trên code cũ |
|---|---|---|---|
| Hợp đồng `queue-token.ts` (plan 1.1) | `apps/web/src/lib/queue-token.test.ts` | 16 test: khoá `ef:queue:token:{id}`, read/save/clear, storage ném lỗi, SSR | không nạp được module (chưa có file) |
| Hợp đồng `api-base.ts` (plan 1.2) | `apps/web/src/lib/api-base.test.ts` | 4 test: env đặt / không đặt (`vi.stubEnv`, nạp lại module) + 3 test chặn hồi quy plan-review F7 (code-review C4/F-5): đọc mã nguồn `api-base.ts` (bỏ comment), phải có nguyên văn `process.env.NEXT_PUBLIC_API_BASE`, không destructure `process.env`, không `process.env[...]` | không nạp được module |
| B1 | `apps/web/src/hooks/use-hold-timer.test.tsx` | `useHoldTimer — đọc token phòng chờ (EV-198, B1) › token phòng chờ lưu ở khoá ef:queue:token:{eventId} được createHold gửi trong X-Queue-Token` | ĐỎ (NOT_ADMITTED) |
| B1 (bảo vệ) | idem | `… › queueToken truyền qua prop vẫn được ưu tiên hơn token trong storage` | xanh |
| B1 — test cũ chuyển sang khoá mới (plan-review F1) | `use-hold-timer.test.tsx` (4 test dùng `queueTokenKey`), `components/checkout/checkout-view.test.tsx` (`beforeEach`) | `tạo hold thành công…`, `chuyển sang sold_out…`, `chuyển sang ambiguous…`, `Bấm đặt nhiều lần không tạo hold trùng`; `khi bấm 'Giữ vé và thanh toán'…`, `bấm đặt nhiều lần nhanh trên UI…` | ĐỎ (đọc khoá cũ `evenflow_queue_token_`) |
| B2 hold | `apps/web/src/lib/hold-client.test.ts` | `requestHold không truyền apiBaseUrl gọi ${NEXT_PUBLIC_API_BASE}/v1/events/{id}/holds` | ĐỎ (URL tương đối) |
| B2 hold (bảo vệ) | idem | `apiBaseUrl truyền tường minh vẫn thắng biến môi trường`; `không đặt NEXT_PUBLIC_API_BASE thì vẫn gọi URL tương đối như cũ` | xanh |
| B2 payment | `apps/web/src/lib/payment-client.test.ts` | `initiatePayment không truyền apiBaseUrl gọi …/payments`; `getOrderDetails không truyền apiBaseUrl gọi …/orders/{orderId}` | ĐỎ |
| B2 payment (bảo vệ) | idem | `initiatePayment: apiBaseUrl truyền tường minh vẫn thắng…`; `getOrderDetails: apiBaseUrl truyền tường minh vẫn thắng…` | xanh |
| B3 (hợp đồng `buildPaymentUrl`, plan-review F13) | `apps/web/src/components/checkout/checkout-view.test.tsx` | `buildPaymentUrl (EV-198, B3)` × 3 | ĐỎ (chưa export) |
| B3 hành vi | idem | `CheckoutView — URL sang thanh toán mang tierId và quantity của hold (EV-198, B3) › bấm 'Tiếp tục thanh toán' chuyển sang /checkout/{id}/payment kèm …` | ĐỎ (thiếu `tierId`) |

## 3. Hợp đồng endpoint giả lập (plan-review F5)

Bảng đầy đủ nằm ở đầu `tests/e2e/fixtures/api-mock.ts`. Tóm tắt:

| Endpoint (origin `E2E_API_BASE`) | Phản hồi |
|---|---|
| `POST /v1/events/{id}/queue/join` 200 | `{state:"QUEUED", queue_token, rank:42, is_new:true, poll_after_ms:3500}` |
| `POST …/queue/join` 401 | `{title:"Unauthorized", detail}` |
| `GET …/queue/status` lần 1 | 200 `{state:"QUEUED", rank:42, eta_seconds:90, poll_after_ms:3500}` + `ETag` |
| `GET …/queue/status` lần 2 | 200 `{state:"ADMITTED", rank:0, eta_seconds:0, poll_after_ms:3500, expires_at: epoch ms + 15'}` (giữ bởi cổng tới khi test thấy QUEUED) |
| `GET …/queue/stream` | không được gọi (polling, 3500 > 3000) → vi phạm nếu bị gọi |
| `POST …/holds` 201 | `{order_id, hold_id, expires_at}` — `expires_at` = lúc nhận request + 10', ISO |
| `POST …/holds` 409 | `{title:"Conflict", detail}` |
| `POST …/payments` 200 | `{payment_id, order_id, status:"PENDING", redirect_url: return_url + "&e2e=paid-ok"}` |
| `GET …/orders/{orderId}` 200 | `{…, status:"PAID", issued_tickets:[{ticketId, ticketCode:"E2E-TKT-0001", tierName, attendeeName, qrPayload}, …]}` |

Mọi phản hồi (kể cả 599 vi phạm) có `Access-Control-Allow-Origin` = origin của web, `Allow-Credentials: true`,
`Allow-Headers` (Content-Type, Idempotency-Key, X-Queue-Token, X-Identity-Id, If-None-Match, Accept),
`Expose-Headers: ETag, X-Poll-After-Ms, Retry-After`.

## 4. AC của Jira chưa đạt

| AC Jira | Trạng thái | Lý do | Điều kiện để đạt |
|---|---|---|---|
| "Chạy trên compose trong CI" | **CHƯA ĐẠT** | Trên compose không có gateway :8080 (stub), không CORS, join đòi `X-Identity-Id`; id hạng vé frontend không phải UUID seed; payment-svc chỉ có `/healthz`; không có GET order và không ai ghi bảng `tickets` (spec mục 1) | Gateway/proxy :8080 + CORS + identity; seed hạng vé khớp frontend; payment-svc thật; route GET order + phát hành vé. Khi đó chạy cùng `buy-flow.spec.ts` với backend thật (bỏ fixture giả lập) |


## 5. Cập nhật sau review (code-review / challenge)

- C3/F-3: test AC-7 teardown viết lại, không dùng `test.fail` (xem dòng AC-7).
- F-4: teardown fixture chờ handler route đang chạy (`unrouteAll({ behavior: "wait" })`, tối đa 5s rồi `ignoreErrors`) trước khi kiểm vi phạm.
- C4/F-5: `apps/web/src/lib/api-base.test.ts` thêm 3 test kiểm mã nguồn (F7).
- Kết quả: `npx vitest run src/lib` 18 file / 311 test xanh; E2E trên standalone build (`HOSTNAME=127.0.0.1 PORT=3000 node server.js`) `--repeat-each=5 --retries=0`: 30/30 xanh.

## 6. Nợ kỹ thuật ghi nhận (không sửa trong slice này)

| # | Nợ | Nguồn | Vì sao chưa sửa / điều kiện để sửa |
|---|---|---|---|
| D-1 | `requestHold` mặc định `identityId = "guest-user"` (`lib/hold-client.ts`); ticketing thật cần UUID (`migrations/0001_core.sql` cột `identity_id`) | spec §4 | Cần identity thật trên đường mua vé (gateway gắn `X-Identity-Id`). Giả lập không kiểm header này. |
| D-2 | Trang thanh toán truyền `expiresAt={undefined}` (`app/(checkout)/checkout/[eventId]/payment/page.tsx`): không có đồng hồ giữ vé ở bước thanh toán | spec §4 | Cần truyền `expires_at` của hold qua URL hoặc đọc `getActiveHold`; ngoài phạm vi 3 lỗi nối trang. |
| D-3 | `getOrderDetails` chuyển `issued_tickets` nguyên dạng, view đọc `ticketCode` (camelCase); `OrderResultView` tự sinh mã `EF-…` khi danh sách rỗng | plan-review F4 | Hợp đồng order backend chưa có. Giả lập trả `ticketCode` và E2E kiểm đúng mã; backend thật trả snake_case sẽ lộ lỗi. |
| D-4 | Thanh toán phương thức `sandbox` biến lỗi mạng thành "thành công giả" (`lib/payment-client.ts`, nhánh `catch`) | code-review F-8 | Hành vi có từ EV-184. E2E chặn bằng marker `e2e=paid-ok` trên `redirect_url` + bắt lỗi CORS trên console. |
| D-5 | `e2e-web.yml` lọc bằng `paths`: nếu sau này đặt làm required check, PR không đụng các path đó sẽ treo chờ | code-review F-7 | Chưa là required check. Khi cần: chuyển sang job `changes` dùng `dorny/paths-filter` như `ci.yml`. |
| D-6 | Gate grep chỉ bắt sleep dạng `new Promise … setTimeout` trên **cùng một dòng** | challenge C2 / verify | Đã mở rộng regex (`new Promise[^;]*setTimeout`); dạng nhiều dòng vẫn lọt. Review code là lớp chặn cuối. |
| D-7 | AC Jira "chạy trên compose" | Jira | Xem §4. |
