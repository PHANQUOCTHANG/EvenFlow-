# EV-198 — Code review (độc lập)

| Mục | Giá trị |
|---|---|
| Phạm vi | Working tree chưa commit trên `feature/EV-198-e2e-playwright` so với `develop` @ `51c4729` (`git diff` + file untracked) |
| Đầu vào | `spec.md` (AC-1..AC-12), `plan.md` (§1–§5, §6 F1–F15) |
| Đã chạy | `npx vitest run` 6 file bị ảnh hưởng → **66/66 pass**; `npx tsc --noEmit` trong `apps/web` → **0 lỗi**; chạy thử regex hygiene của workflow trên `tests/e2e` (xem F-1) |
| Không chạy | `next build`, Playwright (theo yêu cầu: không chạy build dài) |

## Verdict: **FAIL** (1 blocker, phần còn lại là minor/info)

Code sửa B1/B2/B3 đúng spec, unit test và E2E đều có sức bắt lỗi thật. Lý do FAIL duy nhất: bước
hygiene trong `e2e-web.yml` **luôn đỏ** trên chính code của slice này, nên AC-9 (CI chạy cho mỗi PR)
không thể xanh. Sửa F-1 (một dòng regex hoặc hai dòng comment) là đủ để chuyển sang PASS.

---

## Findings

### F-1 — BLOCKER — Regex hygiene của workflow khớp với comment hợp lệ → job luôn fail

- Bằng chứng: `.github/workflows/e2e-web.yml:48`
  `grep -rnE 'waitForTimeout|\b(test|it|describe)\.(skip|only|todo|fixme)\b' tests/e2e --include='*.ts' --exclude-dir=node_modules`
- Repro (chạy tại gốc repo, kết quả thực tế):
  ```
  tests/e2e/fixtures/api-mock.ts:254: *  quan sat xong trang thai trung gian. Thay cho waitForTimeout (AC-8). */
  tests/e2e/specs/buy-flow.spec.ts:11: *   -> calculateJitterMs = 0 -> sleepWithJitter resolve ngay. Khong dung page.clock / waitForTimeout.
  grep rc=0
  ```
  `grep` trả 0 → nhánh `if` chạy `exit 1` → job `e2e-web` đỏ trước khi build, trên mọi PR/push.
- Đề xuất: chỉ bắt **lời gọi**, không bắt chữ trong comment, và phủ thêm biến thể `describe.serial/parallel`:
  ```
  grep -rnE '\.waitForTimeout\(|\b(test|it|describe)(\.(describe|serial|parallel))*\.(skip|only|todo|fixme)\(' \
    tests/e2e --include='*.ts' --include='*.js' --include='*.mjs' --exclude-dir=node_modules
  ```
  (Đã chạy thử: trên `tests/e2e` hiện tại → `rc=1` (không khớp); trên mẫu thử khớp `test.skip(`, `test.describe.skip(`, `test.describe.fixme(`, `it.todo(`,
  `test.describe.parallel.only(`, `test.describe.serial.skip(`, `page.waitForTimeout(`; không khớp hai
  comment trên, `test.fail(`, `test.step(`.)
  Cách tối thiểu khác: đổi chữ trong hai comment. Nên làm cả hai và chạy lại grep trước khi push.

### F-2 — MINOR — Regex hiện tại bỏ sót `test.describe.parallel.only` / `test.describe.serial.skip` và file `.js/.mjs`

- Bằng chứng: `.github/workflows/e2e-web.yml:48`. Câu hỏi "có bỏ sót `test.describe.skip` không?":
  **không bỏ sót** — `\bdescribe\.skip\b` khớp vì `.` trước `describe` là ranh giới từ (đã chạy thử).
  Nhưng `test.describe.parallel.only(` / `test.describe.serial.skip(` không khớp (`parallel`/`serial`
  không nằm trong nhóm), và `--include='*.ts'` bỏ qua `.js/.mjs/.tsx`.
- Giảm nhẹ: `.only` vẫn bị `forbidOnly: !!process.env.CI` (`tests/e2e/playwright.config.ts:16`) chặn.
- Đề xuất: dùng regex ở F-1.

### F-3 — MINOR — Test "vi phạm làm FAIL test ở teardown" dùng `test.fail` nên xanh với **bất kỳ** lỗi nào

- Bằng chứng: `tests/e2e/specs/buy-flow.spec.ts:322-329`. `test.fail(true, …)` coi test là pass nếu
  nó fail vì bất cứ lý do gì (`page.goto("/")` lỗi, `expect(api.violations.length).toBe(1)` sai…),
  không nhất thiết vì `assertNoViolations()` ở teardown.
- Đánh giá sức bắt lỗi: nếu ai đó **xoá** `api.assertNoViolations()` khỏi fixture (`api-mock.ts:269`)
  thì test này pass bất ngờ → Playwright báo "expected to fail but passed" → đỏ. Nếu việc **ghi** vi phạm
  hỏng thì test AC-7 thứ nhất (`:297-320`) đỏ. Hai test bổ sung nhau nên AC-7 **được thực thi thật**;
  điểm yếu chỉ là test thứ hai không tự chứng minh lý do fail.
- Đề xuất (tuỳ chọn): kiểm tra lý do bằng `test.info()` trong một `afterEach` không khả thi với
  fixture teardown; đơn giản hơn là thêm `expect(api.violations[0]).toContain("/v1/events/evt-x/payments")`
  trước khi kết thúc thân test, để lỗi "khác" (sai số lượng/nội dung) bị loại bớt. Ghi chú trong comment
  rằng test này chỉ có ý nghĩa cùng test phía trên.

### F-4 — MINOR — Vi phạm phát sinh sau khi `use(api)` trả về bị bỏ qua (race ở teardown)

- Bằng chứng: `tests/e2e/fixtures/api-mock.ts:263-270`. `assertNoViolations()` chạy ngay sau `use()`,
  trước khi `context` (fixture phụ thuộc) đóng. Request đang bay hoặc request bắn ra trong khoảng đó
  (vd. polling/beacon muộn) được ghi vào `violations` **sau** khi đã assert → không làm fail test.
- Ảnh hưởng: chỉ làm lỏng AC-7 ở mép cuối test; các assert chính (`AC-2` gọi `assertNoViolations()`
  giữa chừng, `buy-flow.spec.ts:202`) không bị ảnh hưởng.
- Đề xuất (tuỳ chọn): trước khi assert, `await context.unrouteAll({ behavior: "wait" })` (Playwright ≥1.41)
  hoặc đóng các page của context rồi mới assert.

### F-5 — MINOR — Không có unit test nào giữ ràng buộc "inline được" của `apiBase()` (F7)

- Bằng chứng: `apps/web/src/lib/api-base.ts:9-11` đúng nguyên văn `return process.env.NEXT_PUBLIC_API_BASE ?? "";`
  (đúng F7). Nhưng `api-base.test.ts` và các test B2 của hold/payment đều `vi.resetModules()` + import lại,
  và Vitest chạy trên Node với `process.env` thật → nếu ai đó đổi sang
  `const { NEXT_PUBLIC_API_BASE } = process.env` (không được Next inline, ra `undefined` trên trình duyệt)
  thì toàn bộ unit test vẫn xanh.
- Giảm nhẹ: E2E bắt được hồi quy này — web build với env, request tương đối rơi vào route vi phạm
  `/v1/**` trên origin web (`api-mock.ts:134-142`) → test đỏ kèm URL. Vì E2E chạy trên mọi PR đụng
  `apps/web/**`, chấp nhận được.
- Đề xuất (tuỳ chọn): một test tĩnh đọc source `api-base.ts` và assert có đúng chuỗi
  `process.env.NEXT_PUBLIC_API_BASE`.

### F-6 — MINOR — `find … server.js | head -n 1` phụ thuộc thứ tự duyệt thư mục

- Bằng chứng: `.github/workflows/e2e-web.yml:72`. Build local hiện tại cho đúng **một** kết quả:
  `apps/web/.next/standalone/apps/web/server.js` (tracing root = gốc repo do có `package-lock.json` ở gốc,
  `next.config` để `outputFileTracingRoot` comment). Đúng như comment trong workflow. Nhưng nếu sau này
  có file `server.js` khác ngoài `node_modules` trong standalone (vd. chunk tên `server.js`), `find` không
  bảo đảm thứ tự.
- Đề xuất: chọn đường dẫn nông nhất, vd.
  `find .next/standalone -name server.js -not -path '*/node_modules/*' -not -path '*/.next/*' | awk '{print length, $0}' | sort -n | head -n1 | cut -d' ' -f2-`,
  hoặc thử lần lượt `.next/standalone/server.js` rồi `.next/standalone/apps/web/server.js`.

### F-7 — MINOR — `paths` filter: thay đổi ở gốc repo ảnh hưởng build không kích hoạt workflow; rủi ro nếu đặt làm required check

- Bằng chứng: `.github/workflows/e2e-web.yml:9-19`. `package-lock.json` gốc quyết định tracing root
  (F-6) nhưng không nằm trong `paths`. Nếu sau này `e2e-web` được đặt làm *required status check*,
  PR không đụng các path này sẽ treo "Expected — waiting" (GitHub không báo skipped cho workflow bị lọc path).
- Đề xuất: thêm `package-lock.json` gốc vào `paths`; nếu muốn required check thì dùng mẫu
  `dorny/paths-filter` như `ci.yml:45` thay vì `on.paths`. Không chặn merge.

### F-8 — INFO — Đổi hành vi nhánh mock-fallback của `initiatePayment` (đã nêu trong plan §1.2)

- Bằng chứng: `apps/web/src/lib/payment-client.ts:142,225`.
- Phân tích: nhánh `!apiBaseUrl && NODE_ENV !== "production"` chỉ chạy khi `fetch` **reject** (lỗi mạng/CORS).
  Trước đây trên trình duyệt `apiBaseUrl = ""` → gọi tương đối vào Next → 404 → ném `UNKNOWN`, nên nhánh
  này hầu như chỉ chạy khi offline. Sau sửa:
  - Build **có** `NEXT_PUBLIC_API_BASE` (compose, CI E2E): lỗi mạng với `vnpay`/`momo` → `NETWORK_ERROR`
    (không còn "thành công giả") — đây là hành vi mong muốn.
  - Build **không** đặt env (Vitest, image `cd-web` khi repo variable rỗng): y như cũ.
  - `method === "sandbox"` + lỗi mạng/CORS vẫn **giả thành công** (nợ có sẵn). E2E đã chặn việc nhánh này
    che lỗi bằng marker `e2e=paid-ok` (`buy-flow.spec.ts:154,229`) và bắt console CORS (`api-mock.ts:157-162`). Đạt F3.
- Đề xuất: ghi nợ "sandbox fallback che lỗi mạng" vào `traceability.md` nếu chưa có.

### F-9 — INFO — `waiting-room.tsx`: lỗi storage giờ bị nuốt thay vì hiện lỗi

- Bằng chứng: `apps/web/src/components/queue/waiting-room.tsx:44,56` + `apps/web/src/lib/queue-token.ts`.
  Trước: `sessionStorage.getItem/setItem` ném (storage bị chặn) → `catch` → "Đã xảy ra lỗi mạng…".
  Sau: đọc trả `null` → join lại; ghi thất bại im lặng, token vẫn trong state → phòng chờ chạy tiếp, nhưng
  sang trang checkout `readQueueToken` trả `null` → "Bạn chưa được cấp quyền mua vé".
- Đánh giá: trường hợp hiếm (storage bị chặn), hành vi mới không tệ hơn; đúng hợp đồng plan §1.1 ("nuốt lỗi storage").
  Không cần sửa.

---

## Kiểm tra theo từng câu hỏi review

| Hạng mục | Kết luận |
|---|---|
| **B1** (khoá token) | Đúng. `lib/queue-token.ts` một khoá `ef:queue:token:{id}` (giữ khoá cũ của phòng chờ, không mất token khi deploy). `waiting-room.tsx` và `use-hold-timer.ts:90` đều đi qua module; `QUEUE_STORAGE_PREFIX` đã xoá. Chuỗi `evenflow_queue_token_` chỉ còn trong comment giải thích ở `queue-token.ts:4` (grep). |
| **B2** (URL tuyệt đối) | Đúng. `hold-client.ts:110`, `payment-client.ts:142,247` mặc định `apiBase()`. Các hook truyền `apiBaseUrl` **undefined** khi không có prop (`use-hold-timer.ts:112`, `use-payment-session.ts:134`, `use-order-status.ts:53`) → tham số mặc định vẫn kích hoạt (JS: `undefined` dùng default). Truyền tường minh vẫn thắng (có test). |
| **Next inline `NEXT_PUBLIC_API_BASE`** | `apiBase()` viết nguyên văn `process.env.NEXT_PUBLIC_API_BASE` → DefinePlugin của Next thay ở **cả** bundle client và server lúc build. Không server component nào import giá trị từ `hold-client`/`payment-client` (chỉ `import type` trong `order-result-view.tsx`, `payment-handoff-view.tsx` — đều client). Mọi lời gọi nằm trong hook/handler chạy trên trình duyệt nên `page.route` bắt được. Lưu ý: giá trị cố định lúc build — image build với env rỗng sẽ gọi tương đối (đã có cảnh báo ở `cd-web.yml:63-73`). |
| **B3** (`buildPaymentUrl`) | Đúng. `checkout-view.tsx:29-40` dùng `URLSearchParams` + `encodeURIComponent(eventId)`, trả URL tương đối có `orderId`, `holdId`, `tierId`, `quantity`. `payment/page.tsx:42-45` đọc `query.tierId` (so khớp `t.id`) và `parseInt(query.quantity, 10)` → parse đúng. Hạng đầu tiên của sự kiện test là `tier-standard-pass` (900.000) ≠ All-Access, nên nếu B3 quay lại thì AC-4 E2E (tên hạng, "2 vé", "5.000.000 đ") đỏ. |
| **SSR safety `queue-token.ts`** | An toàn: `typeof window === "undefined"` → `null`; truy cập `window.sessionStorage` bọc `try` (SecurityError); mọi thao tác get/set/remove bọc `try`. Chỉ được gọi trong effect/handler. Có test SSR (`vi.stubGlobal("window", undefined)`). |
| **Phạm vi diff** | Tất cả file thay đổi nằm trong plan §3 + §6 F14 (`lib/{api-base,queue-token}.ts(+test)`, `waiting-room.tsx`, `use-hold-timer.ts(+test)`, `{hold,payment}-client.ts(+test)`, `checkout-view.tsx(+test)`, `tests/e2e/**`, `.github/workflows/e2e-web.yml`, `docs/workflow/EV-198-e2e-playwright/**`). **Không** có file ngoài phạm vi. `ci.yml`, `e2e-nightly.yml` không đổi. `docs/kiem-thu/README.md` chưa cập nhật (bước 5 của plan — chưa tới hạn, nhưng AC-12 cần trước khi đóng). |
| **Unit test có đỏ trên code cũ?** | Có, không tautology: B1 `use-hold-timer.test.tsx` (code cũ đọc khoá `evenflow_queue_token_` → `NOT_ADMITTED`, `success=false`); B2 hold/payment (code cũ gọi `/v1/...` tương đối ≠ `http://localhost:8080/v1/...`); B3 `checkout-view.test.tsx` (code cũ thiếu `tierId`/`quantity`; `buildPaymentUrl` không tồn tại → file không biên dịch). Test `queue-token`/`api-base` là test hợp đồng cho module mới. Hạn chế: xem F-5. |
| **E2E có bắt lỗi thật?** | Có. AC-2 so `x-queue-token` với đúng `QUEUE_TOKEN` của join (bắt B1), body hold chính xác; request tương đối bị route vi phạm (bắt B2); AC-4 assert UI trang thanh toán (bắt B3); AC-1/3 dùng marker `e2e=paid-ok` + mã vé riêng `E2E-TKT-000x` + `status: "PAID"` nên nhánh sandbox giả / cache không thể làm xanh giả. Chờ bằng `createGate` + `expect`/`waitForURL`, không `waitForTimeout`; jitter cố định `Math.random = () => 0`; polling 3500ms > ngưỡng SSE, stream bị `forbid`. |
| **AC-7 được thực thi thế nào** | Vi phạm (API không handler, `/v1/**` sai origin, `pageerror`, console CORS, handler ném lỗi) được ghi vào `violations`, request trả 599 có CORS; fixture `api` gọi `expect(violations).toEqual([])` **sau `use()`** → lỗi ở teardown fixture làm test fail với danh sách URL trong message. Áp dụng cho mọi test dùng `api` (cả 5 test trong `buy-flow.spec.ts`; `smoke.spec.ts` không dùng fixture — đúng, nó không gọi API). Hạn chế: F-3, F-4. |
| **Workflow** | `paths` đúng 3 nhóm theo AC-9 (+F-7). `GITHUB_ENV` ghi đường dẫn tuyệt đối `$GITHUB_WORKSPACE/apps/web/<dir>` → bước sau dùng `${{ env.WEB_SERVER_DIR }}` làm `working-directory` hợp lệ. Copy `.next/static` + `public` giống `web.Dockerfile:40-42`. Readiness: `curl -sf /api/healthz` 30×2s (route có `force-dynamic`), in log khi fail. Process `nohup … &` sống qua các bước sau. `repeat_each` qua biến env `REPEAT_EACH` → không có injection; mặc định `'1'`. `retries: 0` nằm trong config. Artifact `tests/e2e/report` + `tests/e2e/test-results` (khớp `outputFolder: "report"` và `trace: retain-on-failure`). `npx playwright install --with-deps chromium` sau `npm ci` (lockfile `@playwright/test@1.49.1` khớp `package.json`). Action pin theo tag giống `ci.yml` (`checkout@v4`, `setup-node@v4`). **Lỗi duy nhất: F-1.** `HOSTNAME=127.0.0.1` vs `BASE_URL=http://localhost:3000`: Chromium thử cả IPv4 nên OK; origin trang = `http://localhost:3000` khớp `Access-Control-Allow-Origin`. |

## Việc cần làm để PASS

1. Sửa F-1 (bắt buộc), chạy lại lệnh grep tại gốc repo và xác nhận `rc=1` (không khớp).
2. (Khuyến nghị) F-2 dùng chung regex mới; F-6 chọn `server.js` tất định.
3. Các mục minor/info còn lại có thể ghi nợ trong `traceability.md`.
