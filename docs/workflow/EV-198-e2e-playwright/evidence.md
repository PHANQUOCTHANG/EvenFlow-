# EV-198 — Bằng chứng xác minh độc lập (verify)

| Mục | Giá trị |
|---|---|
| Thời điểm | 2026-10-13 |
| Người xác minh | agent độc lập, chỉ đọc (không sửa source/test/workflow/fixture) |
| Branch | `feature/EV-198-e2e-playwright` — working tree **chưa commit** |
| Base | `develop` @ `51c47290065d8603f9e9f300c96105028917743b` |
| Máy | Windows 11, Node 22, Next.js 15.5.27, `@playwright/test` 1.49.1 (Chromium) |
| Đầu vào | `spec.md` (AC-1..AC-12), `plan.md` (§1–§6), `plan-review.md`, `code-review.md`, `challenge.md`, `traceability.md`, `baseline.md` |

## Kết luận: **FAIL (chỉ do hồ sơ)** — mọi gate code/test đều PASS trên máy

- Code và test: lint, typecheck, Vitest (1564/1564), build, E2E 6/6 và 120/120 (`--repeat-each=20 --retries=0`),
  gate vệ sinh — **tất cả xanh**.
- Lý do FAIL: bước 6 của verify ("mọi finding đã sửa hoặc đã ghi nợ") **chưa đạt** — có 6 mục nợ đã được spec /
  plan-review / code-review / challenge yêu cầu ghi lại nhưng không có ở `traceability.md` hay `docs/kiem-thu/README.md`
  (xem §6). Ngoài ra `docs/kiem-thu/README.md` khẳng định "Đang chạy mỗi PR … 20 lần liên tiếp" trong khi workflow
  chưa từng chạy trên CI (§5). Cả hai là sửa tài liệu, không cần đổi code.
- AC-8 (phần CI) và AC-9: **chờ chạy CI** — không thể chứng minh trước khi push.
- AC Jira "chạy trên compose trong CI": **CHƯA ĐẠT** (đã ghi đúng ở `traceability.md` §4).

## 1. Gate `apps/web`

| Lệnh | Exit | Kết quả |
|---|---|---|
| `npm run lint` | 0 | 0 error, 21 warning — toàn bộ ở `components/auth/{ForgotPasswordForm,LoginForm,OTPVerification,RegisterForm}.tsx` (không thuộc diff) |
| `npm run typecheck` (`tsc --noEmit`) | 0 | 0 lỗi |
| `npx vitest run` | 0 | `Test Files 95 passed (95)`, `Tests 1564 passed (1564)`, 24.8s |
| `NEXT_PUBLIC_API_BASE=http://localhost:8080 npm run build` | 0 | build production OK, `output: standalone` |

- So baseline: `develop` 1528 test / 93 file. Kỳ vọng `baseline.md`: 1528 + 13 + 20 = 1561; thêm 3 test kiểm mã nguồn
  `api-base.ts` (sau code-review F-5/C4) → **1564**, khớp. 0 fail.
- Đã kiểm `localhost:8080` được inline vào bundle client: có trong
  `.next/static/chunks/app/(checkout)/checkout/[eventId]/{page,payment/page,result/page}-*.js`.
- Đỏ trên code cũ (AC-11): theo `baseline.md` — 14 test đỏ + 2 file không nạp được trên `51c4729` chưa sửa.
  Verify không chạy lại code cũ; `challenge.md` M1–M5 cũng chứng minh bằng đột biến.

## 2. E2E trên build production standalone

Chuẩn bị (đúng như `web.Dockerfile` / workflow):

```bash
cd apps/web
# server.js nằm ở .next/standalone/apps/web/ (tracing root = gốc repo vì có package-lock.json ở gốc)
cp -r .next/static .next/standalone/apps/web/.next/static
cp -r public       .next/standalone/apps/web/public
cd .next/standalone/apps/web && HOSTNAME=127.0.0.1 PORT=3000 node server.js   # nền
curl -sf http://127.0.0.1:3000/api/healthz   # {"status":"ok"} sau ~2s ("Ready in 2.5s")
```

| Lệnh (trong `tests/e2e`, `BASE_URL=http://localhost:3000 E2E_API_BASE=http://localhost:8080`) | Exit | Kết quả |
|---|---|---|
| `npx playwright test --retries=0` | 0 | **6 passed** (7.6s), 6 worker |
| `npx playwright test --repeat-each=20 --retries=0` | 0 | **120 passed** (41.4s), 16 worker, 0 failed, 0 flaky |

Kết quả lần chạy đơn:

```
ok 1 smoke.spec.ts:3:5 › trang chu tai duoc (1.2s)
ok 5 buy-flow.spec.ts:297:7 › AC-7 › ghi vi pham kem URL cho request /v1/** toi origin web va request API khong co handler (1.2s)
ok 6 buy-flow.spec.ts:322:7 › AC-7 › teardown cua fixture nem loi kem URL khi co request /v1/** toi origin web (1.2s)
ok 4 buy-flow.spec.ts:254:7 › AC-5: join tra 401 -> phong cho bao can dang nhap/OTP, khong chuyen trang (1.6s)
ok 3 buy-flow.spec.ts:276:7 › AC-6: hold tra 409 -> bao hang ve da het, khong sang trang thanh toan (5.4s)
ok 2 buy-flow.spec.ts:139:7 › AC-1..AC-4: xep hang -> duoc goi -> giu ve -> thanh toan -> nhan ve (6.1s)
6 passed (7.6s)
```

Dọn dẹp: dừng đúng PID 40080 (node, LISTEN `127.0.0.1:3000`), cổng 3000 trống sau đó; đã xoá `tests/e2e/test-results`
và `tests/e2e/report`.

Lưu ý: máy chạy 16 worker; CI `ubuntu-latest` ít lõi hơn → độ ổn định dưới CI cần run `repeat_each=20` thật (AC-8).

## 3. Gate vệ sinh (`.github/workflows/e2e-web.yml`)

Regex chép nguyên văn từ workflow:

```bash
grep -rnE '\.[[:space:]]*waitForTimeout[[:space:]]*\(|\b(test|it|describe)([[:space:]]*\.[[:space:]]*(describe|serial|parallel))*[[:space:]]*\.[[:space:]]*(skip|only|todo|fixme)[[:space:]]*\(|new Promise[^)]*setTimeout' \
  tests/e2e --include='*.ts' --include='*.js' --include='*.mjs' --exclude-dir=node_modules
```

- Trên `tests/e2e` hiện tại: **không khớp, exit 1** → step qua (không còn lỗi F-1/C1).
- Thử trên file mẫu (ngoài repo, scratchpad): bắt `test.skip("a")`, `test .only ("b")`, `test.describe.serial.skip("c")`,
  `page.waitForTimeout(100)`, `new Promise(r => setTimeout(r, 3000))`; không bắt comment, `test.fail(`, `test.step(`.
- **Lỗ còn lại (minor):** `new Promise((r) => setTimeout(r, 3000))` **không** bị bắt (`[^)]*` dừng ở `)` của `(r)`) —
  đây lại là dạng viết phổ biến nhất. Cũng không bắt sleep viết trên nhiều dòng. Ghi nợ (§6).
- `setTimeout` duy nhất trong `tests/e2e` là bộ hẹn giờ giới hạn 5s của teardown (`fixtures/api-mock.ts:273`), không phải
  chờ nghiệp vụ.

## 4. Phạm vi diff

`git status --short` (so với `51c4729`):

```
 M apps/web/src/components/checkout/checkout-view{.tsx,.test.tsx}
 M apps/web/src/components/queue/waiting-room.tsx
 M apps/web/src/hooks/use-hold-timer{.ts,.test.tsx}
 M apps/web/src/lib/hold-client{.ts,.test.ts}
 M apps/web/src/lib/payment-client{.ts,.test.ts}
 M docs/kiem-thu/README.md
 M tests/e2e/{.gitignore,playwright.config.ts,specs/smoke.spec.ts}
?? .github/workflows/e2e-web.yml
?? apps/web/src/lib/{api-base,queue-token}{.ts,.test.ts}
?? docs/workflow/EV-198-e2e-playwright/
?? tests/e2e/{fixtures/,package-lock.json,specs/buy-flow.spec.ts}
```

`git diff --stat`: 13 file tracked, +469 / −55.

Đối chiếu plan §3 + §6 F14 + workflow + docs: **mọi file đều nằm trong danh sách được phép**. `ci.yml` và
`e2e-nightly.yml` không đổi (`git diff --quiet` = 0). `tests/e2e/package-lock.json` khoá `@playwright/test` 1.49.1, khớp
`package.json` (F14). Không có file ngoài phạm vi.

## 5. Từng AC

| AC | Bằng chứng | Kết quả |
|---|---|---|
| AC-1 | `buy-flow.spec.ts:139` (bước AC-1, AC-1/AC-2, AC-1/AC-3): QUEUED + rank 42 → cổng mở → ADMITTED → `/checkout/…` → "Thời gian giữ vé còn" → kết quả "Đã thanh toán thành công" + `E2E-TKT-0001/0002` | ĐẠT (máy, 21/21 kể cả lần đơn) |
| AC-2 | bước `AC-2`: đúng 1 POST holds, `x-queue-token` = `QUEUE_TOKEN` của join, `idempotency-key` khác rỗng, body `{ticket_type_id:"tier-all-access",quantity:2}`, `assertNoViolations()` giữa chừng | ĐẠT (máy) |
| AC-3 | bước `AC-3`: `idempotency-key`, `order_id`/`hold_id` = giá trị hold, `amount` 5_000_000; redirect phải có `e2e=paid-ok` | ĐẠT (máy) |
| AC-4 | bước `AC-4`: tên hạng, "2 vé", "5.000.000 đ", nút "Thanh toán 5.000.000 đ" | ĐẠT (máy) |
| AC-5 | `buy-flow.spec.ts:254`: alert "Bạn cần đăng nhập và xác thực OTP", ở lại `/waiting/…`, cấm poll/stream | ĐẠT (máy) |
| AC-6 | `buy-flow.spec.ts:276`: alert hết vé trong "Tóm tắt đơn hàng", không có nút "Tiếp tục thanh toán", cấm payments | ĐẠT (máy) |
| AC-7 | `buy-flow.spec.ts:297,322` + `fixtures/api-mock.ts` (`finalizeApiMock`): 599 + vi phạm kèm URL; teardown ném lỗi chứa URL | ĐẠT (máy) |
| AC-8 | 120/120 `--repeat-each=20 --retries=0`; `retries: 0` trong config; gate vệ sinh không khớp; `Math.random = () => 0` | Máy: ĐẠT. **CI: chờ chạy CI** (cần run `workflow_dispatch` `repeat_each=20`, ghi link) |
| AC-9 | `.github/workflows/e2e-web.yml`: trigger PR/push `develop`,`main` + paths, build với API_BASE, standalone server, `retries: 0` (config), upload report khi fail, không Docker | **chờ chạy CI** — chưa push, chưa có run nào |
| AC-10 | `smoke.spec.ts`: `test.skip` đã xoá, `trang chu tai duoc` xanh; grep vệ sinh không khớp | ĐẠT |
| AC-11 | Vitest 1564/1564; lint/typecheck/build exit 0; đỏ trên code cũ theo `baseline.md` | ĐẠT |
| AC-12 | `traceability.md` §4 ghi "chạy trên compose" CHƯA ĐẠT + lý do + điều kiện; README cập nhật dòng E2E + bảng số | **ĐẠT một phần**: README dòng E2E ghi "**Đang chạy mỗi PR** … 20 lần liên tiếp không flaky" — chưa đúng cho tới khi CI chạy; nên sửa thành "workflow đã thêm, chờ run đầu tiên" |
| Jira "chạy trên compose trong CI" | Không có backend thật (gateway :8080, CORS, identity, payment, order, phát hành vé) | **CHƯA ĐẠT** — lý do và điều kiện ở `traceability.md` §4 |

## 6. Xử lý finding của review

| Finding | Trạng thái | Chỗ sửa / ghi nợ |
|---|---|---|
| code-review F-1 / challenge C1 (regex khớp comment) | ĐÃ SỬA | `e2e-web.yml` step "chan waitForTimeout…": chỉ khớp lời gọi; verify exit 1 |
| F-2 (lồng `describe.serial/parallel`, `.js/.mjs`) | ĐÃ SỬA | cùng regex + `--include='*.js' --include='*.mjs'` |
| F-3 / C3 (`test.fail` xanh vì lý do sai) | ĐÃ SỬA | `buy-flow.spec.ts:322` viết lại, gọi `finalizeApiMock` trực tiếp và kiểm message |
| F-4 (race teardown) | ĐÃ SỬA | `api-mock.ts` `finalizeApiMock`: `unrouteAll({behavior:"wait"})` giới hạn 5s |
| F-5 / C4 (`apiBase()` nguyên văn) | ĐÃ SỬA | `api-base.test.ts` +3 test đọc mã nguồn (7 test) |
| F-6 (`find | head`) | ĐÃ SỬA | `e2e-web.yml`: chọn `server.js` nông nhất bằng `awk -F/ … | sort -n` |
| F-7 (paths / required check) | SỬA MỘT PHẦN | đã thêm `package-lock.json` gốc vào `paths`; rủi ro "required check treo khi bị lọc path" **chưa ghi nợ** |
| F-8 (nhánh sandbox giả thành công khi lỗi mạng) | **CHƯA GHI NỢ** | code-review đề xuất ghi vào `traceability.md` — không có |
| F-9 (lỗi storage bị nuốt) | INFO, không cần sửa | — |
| C2 (gate vệ sinh lách được) | SỬA MỘT PHẦN | đã thêm khoảng trắng + `new Promise…setTimeout`; còn lọt `new Promise((r) => setTimeout(…))` (§3) — **chưa ghi nợ** |
| spec §4: `identityId = "guest-user"` | **CHƯA GHI NỢ** | spec nói "ghi nợ" — không thấy ở `traceability.md` / README |
| spec §4: trang thanh toán `expiresAt={undefined}` | **CHƯA GHI NỢ** | như trên |
| plan-review F4: thiếu mapping snake_case → camelCase `issued_tickets` | **CHƯA GHI NỢ** | plan §6 F4 nói "ghi là nợ" — không có (E2E giả lập trả sẵn camelCase, `buy-flow.spec.ts` comment) |
| challenge §3 (fallback `tiers[0]`, `amount` do client tính) | ngoài phạm vi, đã ghi trong `challenge.md` | — |

## 7. Việc cần làm để PASS

1. Thêm mục "Nợ kỹ thuật" vào `traceability.md` (hoặc README) gồm: F-7 required-check, F-8 sandbox fallback, C2 phần còn lọt,
   `guest-user`, `expiresAt={undefined}`, mapping `issued_tickets`.
2. Sửa dòng E2E trong `docs/kiem-thu/README.md`: chưa khẳng định "Đang chạy mỗi PR" cho tới khi có run CI xanh.
3. Sau khi push: ghi link run PR và link run `workflow_dispatch repeat_each=20` vào file này → đóng AC-8 (CI), AC-9.
4. (Không bắt buộc) dòng 1 và 3 của `traceability.md` vẫn ghi "bản đầu … chưa chạy trên code mới" — lỗi thời.
