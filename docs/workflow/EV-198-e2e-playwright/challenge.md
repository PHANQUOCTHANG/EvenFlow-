# EV-198 — Challenge (phản biện độc lập)

| Mục | Giá trị |
|---|---|
| Phạm vi | Working tree chưa commit trên `feature/EV-198-e2e-playwright` so với `develop` @ `51c4729` |
| Ngày | 2026-10-13 |
| Cách làm | Đột biến (mutation) từng bản sửa B1/B2/B3 và fixture E2E, chạy lại unit + E2E trên build production; thử biên; chạy lặp 20 lần; soi workflow |
| **Kết luận** | **PASS** (không còn blocker; 1 blocker phát hiện lúc đầu đã được sửa trong khi challenge chạy — xem C1; còn 3 minor) |

Cách chạy E2E dùng trong challenge: `apps/web`: `NEXT_PUBLIC_API_BASE=http://localhost:8080 npm run build`, copy
`.next/static` + `public` vào `.next/standalone/apps/web/`, `HOSTNAME=127.0.0.1 PORT=3000 node server.js`;
`tests/e2e`: `BASE_URL=http://localhost:3000 E2E_API_BASE=http://localhost:8080 npx playwright test`.
Mọi đột biến được sao lưu trước và chép trả lại; đã kiểm `cmp` từng file và md5 của `git diff -- apps/web tests/e2e`
trùng với lúc bắt đầu.

## 1. Findings

### C1 — BLOCKER (đã được sửa trong lúc challenge, đã kiểm lại) — gate vệ sinh trong `e2e-web.yml` khớp chữ trong comment

- Bản workflow lúc bắt đầu challenge dùng `grep -rnE 'waitForTimeout|\b(test|it|describe)\.(skip|only|todo|fixme)\b'`.
- Repro (bản cũ):
  ```
  grep -rnE 'waitForTimeout|\b(test|it|describe)\.(skip|only|todo|fixme)\b' tests/e2e --include='*.ts' --exclude-dir=node_modules; echo $?
  ```
- Kết quả: khớp 2 dòng **comment** — `fixtures/api-mock.ts:254` ("Thay cho waitForTimeout") và
  `specs/buy-flow.spec.ts:11` ("Khong dung page.clock / waitForTimeout"), exit 0 → step `exit 1` → job
  `e2e-web` **đỏ trên mọi PR** dù test xanh.
- Trạng thái: workflow đã được đổi (trong lúc challenge) sang regex chỉ khớp lời gọi
  `'\.waitForTimeout\(|\b(test|it|describe)(\.(describe|serial|parallel))*\.(skip|only|todo|fixme)\('`.
  Chạy lại trên tree hiện tại: exit 1 (không khớp gì) → step qua. Đã kiểm regex mới vẫn bắt
  `test.skip("x")`, `page.waitForTimeout(100)`, `test.describe.only("x")`, `test.fixme("x")`. **Đã đóng.**

### C2 — MINOR — gate vệ sinh mới vẫn lách được

- Repro: ghi file thử (ngoài repo) gồm các dòng `test.skip ("x")`, `test .only("x")`,
  `await new Promise(r => setTimeout(r, 3000))`, rồi chạy đúng regex mới của workflow lên file đó.
- Kết quả: 3 dòng trên **không** bị bắt (chỉ `test.skip("x")`, `page.waitForTimeout(100)`,
  `test.describe.only("x")`, `test.fixme("x")` bị bắt).
- Ảnh hưởng thấp: `.only` đã có `forbidOnly: !!process.env.CI` chặn; `.skip` có khoảng trắng / sleep bằng
  `setTimeout` vẫn lọt. Gợi ý: cho phép `\s*` trước `(` và sau `.`, thêm `setTimeout\(` trong `tests/e2e`
  (hoặc dùng ESLint `playwright/no-skipped-test`, `playwright/no-wait-for-timeout`).

### C3 — MINOR — test "vi pham lam FAIL test o teardown" có thể xanh vì lý do sai

- Test dùng `test.fail(true, …)` nên **bất kỳ** lỗi nào trong thân test cũng thoả "expected to fail",
  không riêng lỗi ở teardown của fixture.
- Repro (đột biến): trong `fixtures/api-mock.ts` đổi predicate route số 2 thành
  `(url) => false && url.pathname.startsWith("/v1/")` (tắt bắt `/v1/**` tới origin web), rồi
  `npx playwright test -g "AC-7"`.
- Kết quả: test teardown vẫn **passed** (expect `api.violations.length === 1` trong thân fail → thoả
  `test.fail`), chỉ test anh em "ghi vi pham kem URL…" đỏ. Lưới an toàn vẫn còn nhờ test anh em, nhưng test
  teardown không tự chứng minh điều nó tuyên bố.
- Đối chứng: đột biến bỏ `api.assertNoViolations()` ở teardown → test này báo "Expected to fail, but passed"
  (đỏ) — tức là nó **có** bắt được trường hợp teardown bị gỡ.
- Gợi ý: thêm một kiểm tra trực tiếp `expect(() => apiMock.assertNoViolations()).toThrow(/\/v1\/events\/evt-x\/payments/)`
  trên một `ApiMock` có vi phạm, hoặc trong test teardown chỉ thao tác (không `expect` trong thân).

### C4 — MINOR — unit test không giữ được yêu cầu "viết nguyên văn" của `apiBase()`

- Repro (đột biến): `api-base.ts` → `const { NEXT_PUBLIC_API_BASE } = process.env; return NEXT_PUBLIC_API_BASE ?? "";`
  rồi `npx vitest run src/lib src/hooks src/components/checkout src/components/queue`.
- Kết quả: **691/691 xanh** (Vitest đọc `process.env` thật nên không phân biệt). Chỉ E2E bắt được (sau build lại):
  AC-1..4 và AC-6 đỏ với vi phạm `POST http://localhost:3000/v1/events/evt-tech-summit-2026/holds — request /v1/** toi origin cua web`.
- Chấp nhận được vì `e2e-web.yml` chạy mỗi PR đổi `apps/web/**`; ghi lại để không ai xoá E2E mà nghĩ unit đã đủ.
  (Có thể thêm 1 test grep nguồn / ESLint rule `no-restricted-syntax` cho destructure `process.env`.)

## 2. Đã thử phá nhưng KHÔNG phá được (bằng chứng)

| # | Đột biến / thử | Unit (Vitest) | E2E (build production) |
|---|---|---|---|
| M1 | `use-hold-timer.ts` đọc lại khoá cũ `evenflow_queue_token_{id}` (B1) | 7 test đỏ (gồm test B1 mới) | AC-1..4, AC-6 đỏ (timeout chờ trạng thái giữ vé / thông báo hết vé) |
| M2 | `waiting-room.tsx` ghi token bằng khoá khác (B1) | 1 đỏ (`waiting-room.test.tsx` "lưu sessionStorage") | không chạy (unit đã đỏ) |
| M3 | `hold-client.ts` `apiBaseUrl = ""` (B2) | 1 đỏ (test B2 hold) | AC-1..4, AC-6 đỏ, vi phạm nêu rõ `POST http://localhost:3000/v1/…/holds` |
| M4 | `payment-client.ts` `initiatePayment` `apiBaseUrl = ""` (B2) | 1 đỏ (test B2 payment) | không chạy (unit đã đỏ) |
| M5 | `buildPaymentUrl` bỏ `tierId` + `quantity` (B3) | 3 đỏ | AC-1..4 đỏ ở bước AC-4 |
| M6 | `apiBase()` destructure `process.env` | 0 đỏ (→ C4) | AC-1..4, AC-6 đỏ, vi phạm nêu URL |
| M7 | `queue-token.ts` đổi `PREFIX` | 8 đỏ (khoá phải giữ `ef:queue:token:`) | — |
| M8 | Build `NEXT_PUBLIC_API_BASE=http://localhost:8080`, chạy `E2E_API_BASE=http://localhost:8081` | — | AC-1 đỏ: `POST http://localhost:8080/…/queue/join — request /v1/** toi origin khong phai E2E_API_BASE` |
| M9 | Fixture: bỏ `api.assertNoViolations()` ở teardown | — | test teardown đỏ ("Expected to fail, but passed") |
| M10 | Fixture: request API không handler trả 404 lặng lẽ | — | test AC-7 "ghi vi pham…" đỏ |
| F1 | Độ ổn định: `--repeat-each=20 --retries=0 --workers=8` | — | **120/120 passed** (1.1 phút) |
| F2 | Console error/warning trong AC-1 + AC-6 (spec thử tạm, đã xoá) | — | chỉ 1 dòng: `409 (Conflict)` của AC-6 — đúng kịch bản, không có lỗi hydrate/CORS |
| E1 | `buildPaymentUrl` với giá trị chứa `&`, `?`, `#`, `/` | có test, xanh | — |
| E2 | `quantity` NaN/0 trên URL thanh toán | `payment/page.tsx` dùng `parseInt` + `quantity > 0 ? quantity : 1` → không lộ NaN | — |
| E3 | sessionStorage ném lỗi / SSR | `queue-token.test.ts` phủ get/set/remove ném lỗi + `window` undefined, xanh | — |
| W1 | `getOrderDetails` gọi qua `useOrderStatus` với `apiBaseUrl` undefined | default param áp dụng → `apiBase()`; E2E bắt GET order đúng origin | — |
| W2 | `WEB_SERVER_DIR` qua `$GITHUB_ENV`, `/api/healthz` tồn tại, server bind 127.0.0.1 nhưng `BASE_URL=localhost` | — | cùng cấu hình chạy xanh trên máy (Chromium tự rơi về IPv4) |

## 3. Ghi chú ngoài phạm vi (không tính finding)

- `payment/page.tsx` khi `tierId` không khớp vẫn lặng lẽ rơi về `event.tiers[0]` — có từ trước, spec không đụng.
- `amount` thanh toán do client tính rồi gửi lên — backend thật phải tự tính lại (nợ backend, đã ngoài phạm vi).
- Khi sessionStorage bị chặn hoàn toàn, token chỉ sống trong state của phòng chờ nên sang trang checkout vẫn
  "chưa được cấp quyền" — giới hạn thiết kế có từ trước, không phải hồi quy.
