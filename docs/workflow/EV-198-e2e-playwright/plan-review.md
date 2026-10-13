# EV-198 — Review kế hoạch (plan-review)

Người review: agent độc lập, context sạch. Đầu vào: `spec.md`, `plan.md`, và các file nguồn mà plan/spec trích dẫn
(đối chiếu trên `feature/EV-198-e2e-playwright` = `develop` @ `51c4729`).

## Kết luận: **PASS WITH CHANGES**

Hướng đi đúng và các hợp đồng chung (`queue-token.ts`, `api-base.ts`, URL thanh toán) đủ để sửa B1–B3. Mọi AC
(AC-1..AC-12) đều có bước tương ứng trong plan. Tuy nhiên có 6 điểm **major** phải sửa trước khi giao test design:
plan bỏ sót test unit cũ đang "khoá" lỗi B1, chưa nói gì về CORS cho response giả lập (khác origin 3000 → 8080, kèm
`credentials: "include"`), chưa chặn được việc nhánh `sandbox` che lỗi, chưa chốt hợp đồng response của từng handler
giả lập (kể cả GET order và mã vé), và AC-8 "trong CI" chưa có cách kiểm chứng.

Không có blocker.

## Đối chiếu nhanh các khẳng định trong spec/plan

| Khẳng định | Kết quả |
|---|---|
| `waiting-room.tsx:22` khoá `ef:queue:token:` | Đúng (dùng ở 45, 57, 129, 156 — cả 4 chỗ phải thay) |
| `use-hold-timer.ts:36` khoá `evenflow_queue_token_` | Đúng |
| `hold-client.ts:108`, `payment-client.ts:140,245` mặc định `""` | Đúng |
| `checkout-view.tsx:51-55` không truyền `apiBaseUrl`, `:89` URL thiếu tier/qty | Đúng |
| `payment/page.tsx:46-47` đọc `tierId`/`quantity` | Lệch dòng: thực tế là **44-45** (nội dung đúng) |
| `use-queue-status.ts:158` ngưỡng SSE `<= 3000` | Đúng (`SSE_THRESHOLD_MS = 3_000` ở dòng 60) |
| Vitest không đặt `NEXT_PUBLIC_API_BASE` | Đúng: `vitest.config.ts` không nạp env, `.env.example` không có biến này → test URL tương đối `hold-client.test.ts:81` vẫn xanh |
| `undefined` truyền xuống vẫn kích hoạt default | Đúng: `use-hold-timer` / `use-payment-session` / `use-order-status` truyền `apiBaseUrl` = `undefined` → default `apiBase()` có hiệu lực |
| Nightly compose có API_BASE `http://localhost:8080` | Đúng: `deploy/compose/docker-compose.yml:219` truyền build arg |
| `ci.yml` pin action | Dùng **tag** (`actions/checkout@v4`, `actions/setup-node@v4`), không dùng SHA; Node `"22"` (`ci.yml:31`); **không có** `upload-artifact` trong repo |

## Phát hiện

### F1 — major — Test unit cũ đang mã hoá lỗi B1, sẽ đỏ sau khi sửa

**Bằng chứng:** `apps/web/src/hooks/use-hold-timer.test.tsx:9` (`evenflow_queue_token_${eventId}`),
`apps/web/src/components/checkout/checkout-view.test.tsx:42` (`sessionStorage.setItem("evenflow_queue_token_evt-test-1", …)`).
Sau khi `use-hold-timer` chuyển sang `readQueueToken` (khoá `ef:queue:token:`), các test này sẽ nhận `NOT_ADMITTED`
và fail. Bảng rủi ro của plan chỉ nhắc test hold/payment về URL, không nhắc chỗ này; bước 2 lại cấm implementer sửa test.

**Thay đổi plan:** Ở §2 bước 1, giao rõ cho agent test design: cập nhật 2 file trên dùng `queueTokenKey(eventId)`
(hoặc literal `ef:queue:token:`) và ghi vào `baseline.md` là "test cũ khoá lỗi, được sửa có chủ đích". Thêm vào bảng
rủi ro §4. Gate AC-11 phải nêu rõ con số: 1528 cũ (sau khi sửa 2 file này) + test mới, 0 fail.

### F2 — major — Chưa có CORS cho response giả lập

**Bằng chứng:** web ở `http://localhost:3000`, API giả ở `http://localhost:8080` → request khác origin. Join, status
và stream dùng `credentials: "include"` (`lib/queue-client.ts:60`, `lib/queue-status.ts:121`, `lib/sse.ts:156`). Hold
gửi header tuỳ biến (`X-Queue-Token`, `X-Identity-Id`, `Idempotency-Key` — `lib/hold-client.ts:124-129`). Status đọc
`ETag`, `X-Poll-After-Ms`, `Retry-After` (`lib/queue-status.ts` ~126-137). Chromium vẫn áp CORS lên response do
`route.fulfill` trả về: thiếu `Access-Control-Allow-Origin` thì fetch ném `TypeError`. Với `credentials: "include"`, giá trị
phải là đúng origin (không được dùng `*`) và phải kèm `Access-Control-Allow-Credentials: true`. Header không nằm trong
danh sách expose thì `res.headers.get()` trả `null`.

**Thay đổi plan:** Ở §1.4, fixture `api-mock.ts` phải tự gắn vào **mọi** response (kể cả response 599 báo vi phạm)
các header: `Access-Control-Allow-Origin: <BASE_URL origin>`, `Access-Control-Allow-Credentials: true`,
`Access-Control-Expose-Headers: ETag, X-Poll-After-Ms, Retry-After`. Request `OPTIONS` (preflight) phải được trả 204
kèm `Allow-Methods`/`Allow-Headers`, hoặc ghi rõ đã kiểm trong `baseline.md` rằng Playwright Chromium tự xử lý
preflight. Request `OPTIONS` không được tính là vi phạm AC-7.

### F3 — major — Nhánh `sandbox` biến mọi lỗi mạng của thanh toán thành "thành công"

**Bằng chứng:** phương thức mặc định của form là `"sandbox"` (`hooks/use-payment-session.ts:85`), và
`payment-client.ts:223` trả `mock_pay_…` rồi redirect sang trang kết quả khi `method === "sandbox"` gặp lỗi mạng. Lỗi
CORS (F2), request bị abort, hay request đi nhầm origin đều là lỗi mạng. Với `NODE_ENV=production` thì nhánh
`!apiBaseUrl` đã tắt sẵn, nên plan §1.2 chỉ mới nêu hệ quả của B2. Rủi ro thật nằm ở nhánh `sandbox`, mà nhánh này
plan giữ nguyên. Khi đó AC-3 vẫn có thể "thấy" request trong log của mock (route handler đã chạy) trong khi trình
duyệt bỏ response, và luồng vẫn chạy tiếp như thể mọi thứ ổn.

**Thay đổi plan:** Trong `buy-flow.spec.ts`, handler payments phải trả `redirect_url` có dấu hiệu riêng (ví dụ
`/checkout/evt-tech-summit-2026/result?orderId=…&via=mock-server`), và test assert URL cuối chứa dấu hiệu đó. Như vậy
đường fallback (dùng `effectiveReturnUrl`, không có dấu hiệu) sẽ bị bắt. Ngoài ra, fixture lắng nghe `page.on("pageerror")`
và `console` error dạng CORS/`Failed to fetch`, rồi đưa vào danh sách vi phạm. Thêm dòng này vào bảng rủi ro §4.

### F4 — major — "Mã vé do server trả về" có thể bị UI tự bịa; hợp đồng `issued_tickets` lệch kiểu

**Bằng chứng:** `components/checkout/order-result-view.tsx:124-128`: khi `issuedTickets` rỗng, UI tự sinh
`EF-${orderId.slice(-6).toUpperCase()}-${idx+1}`. `lib/payment-client.ts:271` gán `issuedTickets: data.issued_tickets || []`
mà **không** map snake_case sang camelCase, nên view đọc `ticket.ticketCode` (`:138`). Nếu mock trả `ticket_code`, UI hiện
mã rỗng. Nếu mock trả mảng rỗng, UI hiện mã tự bịa, và một assertion lỏng kiểu "thấy mã vé" vẫn xanh. Plan §1.4 không
nhắc gì tới handler `GET …/orders/{orderId}`, dù AC-1 cần nó.

**Thay đổi plan:** Chốt hợp đồng GET order trong plan: `status: "PAID"`, `issued_tickets: [{ ticketId, ticketCode,
tierName, … }]` (camelCase **theo đúng những gì frontend đang đọc**), với `ticketCode` có dạng không thể trùng mẫu `EF-…`
(ví dụ `SRV-E2E-0001`). Test assert đúng chuỗi đó. Ghi nợ vào `traceability.md`: `issued_tickets` không được map
snake→camel, nên backend thật phải thống nhất lại hợp đồng này.

### F5 — major — Chưa chốt hợp đồng response cho từng handler giả lập

**Bằng chứng:** §1.4 chỉ mô tả cơ chế của fixture, không có bảng endpoint → response. Một số chi tiết ảnh hưởng trực
tiếp tới độ ổn định:
- Hold: `expires_at` phải được tính **lúc nhận request** (`now + 10 phút`). Nếu dùng chuỗi cố định, hold sẽ hết hạn sau
  ngày đó. Hook đếm theo `serverNow(offsetMs)` lấy từ `/api/time` thật (`use-hold-timer.ts:69-70`, `useServerCountdown`).
- Hold: response bắt buộc có `order_id` và `hold_id` (`hold-client.ts:150-152`), vì AC-3 so sánh với các giá trị này.
- Join: `queue_token`, `rank`, `is_new`, `poll_after_ms` (`queue-client.ts:24-29`).
- Status: `state`, `rank`, `poll_after_ms`, và header `ETag` nếu có (nếu có ETag thì lần poll sau sẽ gửi
  `If-None-Match`, nên handler phải trả 200 chứ không phải 304 khi muốn chuyển sang ADMITTED).
- Payments: `payment_id`, `order_id`, `redirect_url` (xem F3), `status`.

**Thay đổi plan:** Thêm vào §1.4 bảng "Endpoint giả lập → method → response → header", gồm join, status, (stream nếu
dùng), holds (201 / 409 cho AC-6), payments, GET order, và join 401 cho AC-5.

### F6 — major — AC-8 "trong CI" và AC-9 chưa có cách kiểm chứng

**Bằng chứng:** Plan §1.5 cho workflow PR chạy `npx playwright test` **một lần**. §2 bước 3 chỉ chạy
`--repeat-each=20` ở máy. AC-8 lại ghi "trên máy **và trong CI**". AC-9 cần bằng chứng là workflow thật sự chạy trên PR.

**Thay đổi plan:** Thêm input `workflow_dispatch` tên `repeat_each` (mặc định `1`) và truyền vào
`--repeat-each=${{ inputs.repeat_each || 1 }} --retries=0`. Chạy tay một lần với `20` trên branch, rồi ghi link run cùng
tổng số test và số fail vào `evidence.md`. Với AC-9, ghi link run do sự kiện `pull_request` kích hoạt. Thêm gate bằng
grep: `tests/e2e` không chứa `waitForTimeout` (hoặc mỗi chỗ có phải kèm lý do không phải chờ nghiệp vụ) và không chứa
`test.skip`, `.only`, `.todo` (AC-10).

### F7 — minor — `api-base.ts`: ràng buộc cách truy cập `process.env`

**Bằng chứng:** Next chỉ inline đúng biểu thức thành viên literal `process.env.NEXT_PUBLIC_API_BASE`. Destructuring
(`const { NEXT_PUBLIC_API_BASE } = process.env`), truy cập động `process.env[key]` hay đi qua biến trung gian đều **không**
được inline, và trên client sẽ ra `undefined` → `""`, tức là B2 quay lại. Ngoài ra, nếu đọc env ở mức module (như
`queue-client.ts:31`) thì `vi.stubEnv` trong unit test B2 không có tác dụng.

**Thay đổi plan:** Ghi vào hợp đồng §1.2 rằng thân hàm phải là đúng `return process.env.NEXT_PUBLIC_API_BASE ?? "";`,
đọc lúc gọi hàm. Unit test dùng `vi.stubEnv` cùng `vi.unstubAllEnvs()` trong `afterEach`. (`api-base` chỉ được import từ
module client; server bundle cũng được inline lúc build nên không ảnh hưởng gì.)

### F8 — minor — Jitter: chốt cách làm cụ thể

**Bằng chứng:** `lib/jitter.ts:30` dùng `Math.random`, với `DEFAULT_MIN_JITTER_MS = 0`. `event-action-panel.tsx:54` gọi
`sleepWithJitter` mà không truyền `randomFn`. Idempotency-Key dùng `crypto.randomUUID` (`hold-client.ts:51-53`; `localhost`
là secure context), nên không phụ thuộc `Math.random`.

**Thay đổi plan:** Thay cụm "chỉ trong phạm vi cần thiết hoặc cách tất định tương đương" bằng
`page.addInitScript(() => { Math.random = () => 0; })`, cho jitter bằng 0ms. Ghi lại hệ quả: `backoffDelay` cũng thành
tất định, điều này chấp nhận được. Không dùng `page.clock`, vì nó làm lệch `useServerTimeSync` / countdown.

### F9 — minor — Status: nên chọn polling, không chọn SSE

**Bằng chứng:** `lib/sse.ts` tự đọc stream bằng `fetch`. `route.fulfill` trả toàn bộ body một lần rồi đóng, nên
`conn.done` = `ended` và hook rơi về polling sau `lastPollAfterMs` (`use-queue-status.ts` ~178-190). Hành vi này đúng
nhưng phụ thuộc vào cách reconnect/backoff, khó tất định hơn.

**Thay đổi plan:** Chốt dùng `poll_after_ms = 3500` (lớn hơn ngưỡng 3000) cho lần QUEUED và chuyển sang ADMITTED ở lần
poll kế tiếp. Không đăng ký handler stream, để mọi request `/queue/stream` thành vi phạm AC-7, đúng ý đồ. Đặt timeout
cho `waitForURL` (≥ 10s) cao hơn nhịp poll.

### F10 — minor — Không có catch-all cho origin thứ ba / API_BASE build lệch

**Bằng chứng:** Plan §1.4 chỉ ghi lại vi phạm ở hai origin (`E2E_API_BASE` và web `/v1/**`). Nếu web được build với
`NEXT_PUBLIC_API_BASE` khác `E2E_API_BASE`, request đi tới origin thứ ba mà không route nào bắt. Kết quả là lỗi mạng,
và với thanh toán thì bị nhánh sandbox nuốt (F3).

**Thay đổi plan:** Fixture đăng ký thêm route `**/v1/**` cho mọi origin, coi là vi phạm nếu origin không phải
`E2E_API_BASE`. Thông báo vi phạm phải gợi ý nguyên nhân "web build với NEXT_PUBLIC_API_BASE khác E2E_API_BASE".

### F11 — minor — Cấu hình Playwright khi `retries: 0`

**Bằng chứng:** `tests/e2e/playwright.config.ts`: `trace: "on-first-retry"`. Khi `retries: 0` thì trace không bao giờ
được ghi. Đổi `retries` cũng ảnh hưởng nightly (`e2e-nightly.yml` → `make test-e2e`).

**Thay đổi plan:** Đổi sang `trace: "retain-on-failure"`. Ghi rõ nightly cũng chuyển sang `retries: 0` (có chủ đích,
spec cho phép giữ nguyên file nightly). Workflow mới upload cả `test-results/` (chứa trace) cùng `report/`.

### F12 — minor — Workflow: pin action và `next start` với standalone

**Bằng chứng:** `ci.yml` pin bằng tag `@v4`, không bằng SHA, và không dùng `upload-artifact`.
`apps/web/next.config.mjs:10` đặt `output: "standalone"`, nên `next start` in cảnh báo "does not work with output:
standalone". Lệnh vẫn phục vụ được trang, nhưng khác cách chạy trên compose.

**Thay đổi plan:** Ghi "pin theo tag giống `ci.yml`: `actions/checkout@v4`, `actions/setup-node@v4` (Node `"22"`,
`cache-dependency-path` gồm `apps/web/package-lock.json` và `tests/e2e/package-lock.json`), `actions/upload-artifact@v4`".
Chốt dùng `npm run start` và chấp nhận cảnh báo, hoặc chạy `node .next/standalone/server.js` sau khi copy `.next/static`
và `public`. Ghi lý do chọn. Bước chờ sẵn sàng nên poll `http://localhost:3000/api/healthz`.

### F13 — minor — Unit test B3 phải chạy được trên jsdom

**Bằng chứng:** `checkout-view.tsx:89` gán `window.location.href`, mà jsdom không hỗ trợ điều hướng (báo
"Not implemented: navigation"). Test cũ né chỗ này bằng `onProceedToPayment` (`checkout-view.test.tsx:181-187`).

**Thay đổi plan:** Ở §1.3, tách hàm thuần `buildPaymentUrl(eventId, hold)` (export từ `checkout-view.tsx` hoặc
`lib/`) để unit test B3 assert chuỗi URL. Cách khác là chỉ định test phải stub `window.location` bằng
`Object.defineProperty`. Nếu tách sang `lib/` thì bổ sung file đó vào §3.

### F14 — minor — Danh sách file được phép đổi

**Bằng chứng / thay đổi:** §3 nên nêu tên tường minh, thay vì gộp vào "test cạnh các file đó":
`hooks/use-hold-timer.test.tsx`, `components/checkout/checkout-view.test.tsx`, `components/queue/waiting-room.test.tsx`
(F1), `lib/hold-client.test.ts`, `lib/payment-client.test.ts`. Nếu tách helper ở F13 thì thêm file đó. Không cần sửa
`e2e-nightly.yml` hay `Makefile`. Ghi chú: `tests/e2e/package-lock.json` phải được sinh bằng đúng `@playwright/test@1.49.1`
đang pin, và nightly vẫn dùng `npm install` nên không bị ảnh hưởng.

### F15 — minor — Sai số dòng trong spec

**Bằng chứng:** spec §2 B3 trích `payment/page.tsx:46-47`, thực tế là `:44-45`.
**Thay đổi:** Sửa trích dẫn trong spec, hoặc ghi chú lại trong `traceability.md`.

## Độ phủ AC

| AC | Phủ trong plan | Ghi chú |
|---|---|---|
| AC-1 | §1.4 buy-flow | Cần F3, F4, F5 để không xanh giả |
| AC-2 | §1.1, §1.2, fixture ghi request | Cần F2 (CORS) |
| AC-3 | §1.2, §1.3 | Cần F3 |
| AC-4 | §1.3 | OK |
| AC-5 | buy-flow | OK (`waiting-room.tsx` render `role="alert"` khi 401) |
| AC-6 | buy-flow | OK (409 → SOLD_OUT ở `hold-client.ts`) |
| AC-7 | fixture | Cần F2 (599 kèm CORS), F10 |
| AC-8 | §1.4, §2 bước 3 | Cần F6, F8, F9 |
| AC-9 | §1.5 | Cần F6 (bằng chứng run), F12 |
| AC-10 | §1.4 smoke | Cần gate grep (F6) |
| AC-11 | §2 bước 1–3 | Cần F1, F7, F13 |
| AC-12 | §2 bước 5 | Bổ sung nợ từ F4 (`issued_tickets`) và F3 (fallback sandbox) |
