# EventFlow — Tài liệu Kiểm thử phần mềm

Thư mục này gom tài liệu kiểm thử để nộp cho môn **Kiểm thử phần mềm**. Dự án đồng thời là bài
Project 1, nên artifact kỹ thuật nằm ở `docs/workflow/<task>/`, còn đây là nơi tổng hợp lại theo
góc nhìn kiểm thử.

> **Định dạng tạm thời.** Chủ nhiệm đồ án sẽ đưa **mẫu (template)** của môn học sau. Mọi file ở
> đây đang để ở dạng đơn giản, dễ nắn lại — **chưa** dựng cấu trúc phức tạp để tránh phải làm lại
> từ đầu khi có mẫu. Khi có mẫu thì chuyển toàn bộ nội dung sang đúng mẫu đó.

---

## 1. Phạm vi hiện tại

Kiểm thử đang tập trung vào **frontend `apps/web`**. Backend Go và Python chưa có nhiều để kiểm:
6/8 service Go vẫn là stub health-check 31 dòng, `ai-worker` còn TODO ở phần consumer.

| Tầng | Công cụ | Trạng thái |
|---|---|---|
| Unit + Component (frontend) | Vitest 2.1.8 + React Testing Library + jsdom | **Đang chạy, 1086 test** |
| E2E | Playwright (`tests/e2e`) | Khung đã dựng, chưa có kịch bản thật |
| Unit (Go) | `go test` | CI có job `test-go`, code phần lớn là stub |
| Unit (Python) | pytest | CI có job `test-python`, chưa có test |
| Integration | testcontainers | `make test-integration`, cần Docker |
| Cổng không oversell | `make test-oversell` (TestNoOversell ×200) | Có workflow riêng `oversell-gate.yml` |
| Quét lỗ hổng image | Trivy, chặn mức CRITICAL | `trivy.yml` |
| **Hiện vật build** | `gates.sh` check C4.3 + C4.5 (grep `.next/server/**`) | **Đang chạy** — xem §7.1 |
| **Container thật** | `docker build` + `docker kill -s SIGTERM` | **Đã chạy** — xem §7.2 |
| Kubernetes | `kubectl --dry-run=server`, probe qua kubelet | **Chưa chạy** — chưa bật Kubernetes trong Docker Desktop |

## 2. Cách chạy

```bash
cd apps/web
npm run test            # chạy test, không áp ngưỡng coverage
npm run test:coverage   # chạy test + áp ngưỡng trong vitest.config.ts
npm run lint
npm run typecheck
npm run build
```

Ngưỡng coverage cấu hình tại `apps/web/vitest.config.ts`: lines / functions / statements **70%**,
branches **60%**.

> **Cảnh báo khi đọc số coverage.** `coverage.include` đang là `src/**/*.{ts,tsx}` và **không**
> loại trừ `*.test.*`, nên **file test cũng được tính như mã nguồn**. File test gần như luôn 100%
> covered theo định nghĩa, nên con số tổng bị đẩy lên và ngưỡng 70% thực chất dễ hơn ý định ban
> đầu. Con số vẫn tái lập được, nhưng **không nên dùng nó làm bằng chứng chất lượng**. Đã có ticket
> riêng để thêm `src/**/*.{test,spec}.{ts,tsx}` vào `coverage.exclude`.

## 3. Thống kê hiện tại

Chạy ngày **2026-10-05**. **1086 test / 44 file.** Số đo bằng
`npx vitest run --reporter=json`, không chép tay.

| Nhóm | File | Số test |
|---|---|---|
| **Thư viện** | `lib/format.test.ts` | 66 |
| | `lib/server-time.test.ts` | 39 |
| | `lib/money.test.ts` | 24 |
| | `lib/readiness.test.ts` | 19 |
| | `lib/queue-client.test.ts` | 11 |
| | `lib/theme.test.ts` | 11 |
| | `lib/cn.test.ts` | 7 |
| **Hook** | `hooks/use-server-countdown.test.tsx` | 48 |
| | `hooks/use-theme.test.tsx` | 13 |
| **Component miền** | `components/queue/queue-status-panel.test.tsx` | 112 |
| | `components/checkout/order-summary.test.tsx` | 74 |
| | `components/checkout/ticket-tier-card.test.tsx` | 49 |
| | `components/event/event-card.test.tsx` | 39 |
| **Component dùng chung** | `components/ui/server-expiry-countdown.test.tsx` | 43 |
| | `components/ui/button.test.tsx` | 32 |
| | `components/ui/input.test.tsx` | 23 |
| | `components/ui/checkbox.test.tsx` | 20 |
| | `components/ui/radio.test.tsx` | 18 |
| | `components/ui/spinner.test.tsx` | 17 |
| | `components/ui/toast.test.tsx` | 17 |
| | `components/ui/field.test.tsx` | 16 |
| | `components/ui/badge.test.tsx` | 15 |
| | `components/ui/alert.test.tsx` | 14 |
| | `components/ui/select.test.tsx` | 14 |
| | `components/ui/card.test.tsx` | 11 |
| **Bố cục** | `components/layout/nav-config.test.ts` | 45 |
| | `components/layout/workspace-shell.test.tsx` | 44 |
| | `components/layout/public-header.test.tsx` | 36 |
| | `components/layout/nav-link.test.tsx` | 30 |
| | `components/layout/container.test.tsx` | 13 |
| | `components/layout/public-shell.test.tsx` | 12 |
| | `components/layout/skip-link.test.tsx` | 11 |
| | `components/layout/public-footer.test.tsx` | 7 |
| **Theme** | `components/theme/theme-script.test.tsx` | 13 |
| | `components/theme/theme-toggle.test.tsx` | 7 |
| **Route group** | `app/(ops)/layout.test.tsx` | 15 |
| | `app/(organizer)/layout.test.tsx` | 14 |
| | `app/(marketing)/layout.test.tsx` | 13 |
| | `app/(checkout)/layout.test.tsx` | 11 |
| | `app/(queue)/layout.test.tsx` | 11 |
| | `app/(marketing)/page.test.tsx` | 1 |
| **Health endpoint** | `app/api/readyz/route.test.ts` | 19 |
| | `app/api/healthz/route.test.ts` | 13 |
| **Tắt êm (graceful shutdown)** | `instrumentation.test.ts` | 19 |

Tổng theo nhóm: thư viện 177, hook 61, component miền 274, component dùng chung 240, bố cục 198,
theme 20, route group 65, health endpoint 32, tắt êm 19 — cộng lại đúng **1086**.

## 4. Quy trình sinh ra bộ test này

Theo `multi_agent_coding_workflow.pdf` của nhóm. Điểm quan trọng với môn kiểm thử:

**Người viết test KHÔNG phải người viết mã.** Mỗi task có một tác tử riêng viết test từ
acceptance criteria, **không được đọc mã hiện thực**. Nếu người viết mã cũng viết test thì test chỉ
mô tả lại mã, không phát hiện được sai lệch so với yêu cầu.

**Mỗi AC phải có ít nhất một test hoặc một bằng chứng chạy được**, ghi trong ma trận truy vết của
từng task.

**Không được sửa, xoá, skip test để cho qua.** Khi test đỏ, phải xác định test sai hay mã sai rồi
mới xử lý, và ghi lại kết luận. Trong `apps/web` hiện **không có** `.skip` / `.only` / `it.todo` nào.
Toàn repo có **đúng một** chỗ: `tests/e2e/specs/smoke.spec.ts:9` là `test.skip` cho kịch bản E2E mua
vé **chưa viết** (chờ trang phòng chờ / checkout), không phải test đỏ bị tắt đi. Gate C4.2 soi cả
`tests/e2e/**` nhưng chỉ chặn dòng **mới thêm**, nên dòng có sẵn này không làm gate đỏ — và một
`.skip` mới thêm ở đó thì bị chặn. (Bản trước của câu này ghi "toàn bộ repo không có" là **sai**;
phát hiện ở bước xác minh độc lập P6.)

## 5. Ma trận truy vết theo từng tính năng

| Tính năng | Ma trận truy vết | Đặc tả + AC |
|---|---|---|
| Design system, token, dark mode | `docs/workflow/EVF-1801/traceability.md` | `docs/workflow/EVF-1801/spec.md` |
| Route group + app shell | `docs/workflow/EVF-1801-route-groups/traceability.md` | `.../spec-plan.md` |
| Đồng hồ theo giờ server + panel hàng chờ | `docs/workflow/EVF-1803-queue-countdown/traceability.md` | `.../spec-plan.md` |
| Thẻ sự kiện, hạng vé, tóm tắt đơn | `docs/workflow/EVF-1802-event-cards/traceability.md` | `.../spec-plan.md` |
| Health endpoint + tắt êm + deploy k8s | `docs/workflow/deploy-k8s-web/traceability.md` | `.../spec.md`, `.../contracts.md` |

Riêng slice deploy còn có `docs/workflow/deploy-k8s-web/integration-report.md` (bằng chứng thô của
từng lần chạy, kể cả các lần **chưa** chạy được) và `runbook.md` (§6 liệt kê từng thứ chưa verify
được và vì sao). Hai file đó dùng được làm ví dụ về **báo cáo kiểm thử trung thực**: ghi cả phần
không đạt thay vì chỉ ghi phần xanh.

Mỗi task còn có `code-review.md` (hoặc mục tương đương trong `spec-plan.md`) ghi từng lỗi được
phát hiện kèm mức độ, vị trí `file:line` và cách xử lý — dùng được làm hồ sơ lỗi cho môn học.

## 6. Những gì KHÔNG kiểm được ở mức unit

Ghi lại để không ai tưởng là đã phủ:

| Hạng mục | Lý do | Cách kiểm thay thế |
|---|---|---|
| Ẩn/hiện theo breakpoint (menu mobile, sidebar) | jsdom không chạy CSS, không có viewport | Playwright với viewport thật |
| `prefers-reduced-motion` | `vitest.setup.ts` stub `matchMedia` luôn trả `matches: false` | Playwright `emulateMedia({ reducedMotion: "reduce" })` |
| Tỉ lệ tương phản màu, sticky, max-width | Thuộc tính CSS, jsdom không tính layout | Tính tay theo công thức WCAG (đã làm, lưu trong evidence của từng task) + kiểm bằng mắt |
| Tiết lưu `setInterval` ở tab background thật | Chỉ mô phỏng được bằng fake timer | Kiểm thủ công trên browser thật |
| `make test`, test Go/Python | Máy dev không có `make` và Go toolchain | CI (`ci.yml` chạy `test-go` / `test-python`) |
| Giao nhận **signal POSIX** (SIGTERM/SIGINT) | Windows không có signal POSIX: `child.kill('SIGTERM')` của Node thực chất gọi `TerminateProcess`, tiến trình chết ngay, không có handler nào chạy | Chạy container Linux rồi `docker kill -s SIGTERM` — **đã làm**, xem §7.2 |
| Hành vi của **mã sau khi bundle/minify** | Vitest không bundle: nó nạp đúng một module instance. Mọi lỗi chỉ xuất hiện khi webpack nhân bản module hoặc terser xoá nhánh đều **vô hình** với cả 1086 test | Grep marker trong `.next/server/**` — gate C4.5, xem §7.1 |
| Kubernetes: pod Ready, probe qua kubelet, HPA, Ingress, rolling update | Chưa bật Kubernetes trong Docker Desktop; Docker Desktop cũng không có metrics-server và ingress controller | `docs/workflow/deploy-k8s-web/runbook.md` §3–§5; phần chưa chạy liệt kê ở §6.2 của runbook đó |

## 7. Hai mức kiểm thử mà unit test không với tới

Phần này đáng ghi riêng cho môn học: đây là **hai lỗi thật** trong dự án mà toàn bộ bộ unit test —
1086 test, cộng lint, typecheck và `next build` — đều **không** phát hiện được, và cách bắt được chúng.

### 7.1 Lỗi chỉ tồn tại sau khi bundle: nhánh 503 bị xoá khỏi bản production

**Hiện tượng.** `/api/readyz` phải trả `503` khi ứng dụng đang tắt êm, để Kubernetes rút pod khỏi
danh sách nhận traffic. Cờ trạng thái lúc đầu là một `let` ở tầng module.

**Vì sao test không thấy.** Next đóng gói `instrumentation.ts` và **từng** route handler thành các
**bundle webpack riêng**, nên file giữ cờ bị nhân bản vào mỗi bundle — một `let` ở tầng module trở
thành **hai biến khác nhau**. Trong bundle của `/api/readyz` không có ai *ghi* cờ, nên terser chứng
minh được nó luôn `true` và **xoá hẳn nhánh 503** khỏi bản build. Trong bundle của
`instrumentation.ts` không có ai *đọc* cờ, nên `setReady(false)` thành dead code và cũng bị xoá.

Vitest **không** bundle — nó nạp đúng một module instance — nên cả 1086 test vẫn xanh, trong khi
production thì `/api/readyz` **luôn trả 200** và tính năng tắt êm không bao giờ quan sát được. Đây là
dương tính giả ở mức nguy hiểm nhất: test xanh, build xanh, tính năng không tồn tại.

**Cách bắt.** Gate `C4.5` trong `.github/scripts/gates.sh` grep chính file build ra:

| bundle | marker bắt buộc có | ý nghĩa nếu mất |
|---|---|---|
| `app/api/readyz/route.js` | `__eventflowWebReadiness__` | cờ không còn đọc từ `globalThis` ⇒ đã thành biến cục bộ của bundle |
| `app/api/readyz/route.js` | `not-ready`, `503` | nhánh 503 đã bị optimizer xoá |
| `instrumentation.js` | `__eventflowWebReadiness__` | `setReady(false)` đã bị xoá làm dead code |
| `instrumentation.js` | `SIGTERM`, `SIGINT` | không còn đăng ký handler ⇒ pod chết bằng exit 143/130 thay vì tắt êm |
| `app/api/healthz/route.js` | **không được có** `503` | logic readiness lọt vào liveness ⇒ k8s **restart** pod đang drain |

Hai bài học về **thiết kế chính cái check**:

- **Marker phải sống qua minify.** `__eventflowWebReadiness__` là một khoá **chuỗi** trên
  `globalThis`, và terser không đổi tên chuỗi. Ngược lại `grep -c setReady` **vô dụng** — tên hàm bị
  minify nên nó ra `0` ngay cả khi mã hoàn toàn đúng. Một check như vậy sẽ báo động giả rồi bị tắt đi.
- **Check chưa từng fail thì chưa phải bằng chứng.** Đã tiêm lại đúng hồi quy đó rồi chạy lại toàn bộ:
  lint, typecheck, **44 file / 1086 test**, `next build` đều xanh, bốn marker về `0`, và chỉ C4.5 fail
  với exit 1. Hoàn nguyên thì exit 0. Không có bước này thì C4.5 chỉ là một đoạn script *trông như*
  đang bảo vệ cái gì đó.

### 7.2 Lỗi chỉ xuất hiện dưới signal thật: tắt êm trên Windows là giả

**Hiện tượng.** Ban đầu AC "SIGTERM ⇒ readyz 503 ⇒ chờ 5s ⇒ exit 0" được kiểm bằng
`process.emit("SIGTERM")` ngay trong tiến trình.

**Vì sao chưa đủ.** `process.emit` chỉ gọi hàm handler, nó **không** đi qua đường giao nhận signal
của hệ điều hành. Và Windows **không có signal POSIX**: `child.kill('SIGTERM')` của Node thực chất
gọi `TerminateProcess`, tiến trình chết ngay, không handler nào chạy. Nên trên máy dev Windows thì
không có cách nào kiểm thật.

**Cách bắt.** Chạy container Linux rồi `docker kill -s SIGTERM`. Kết quả đo được:

| Quan sát | Kết quả |
|---|---|
| `/api/readyz` sau SIGTERM | 503 **dưới nửa giây** (77 ms lần đo đầu, ~316 ms khi P6 đo lại — xem `integration-report.md` §8) |
| `/api/healthz` suốt cửa sổ drain | **giữ 200** — liveness không được sập, nếu không kubelet sẽ restart pod giữa lúc drain |
| Thoát tiến trình | ExitCode **0** ở t=5129 ms |
| SIGTERM thứ hai ở t=2s, chạy dưới `--init` | **không** cắt ngắn drain |
| SIGINT | 503 trong ~1s, ExitCode 0 |

Ca cuối bảng là một lỗi riêng mà chỉ cách kiểm này mới lộ ra: `process.once` thay vì `process.on` làm
listener bị thao ngay sau lần gọi đầu, libuv trả signal về mặc định, nên **SIGTERM thứ hai** giết tiến
trình với exit 143 giữa cửa sổ drain. Lỗi này chỉ hiện khi node **không** phải PID 1 (`--init`, tini,
hay bất kỳ entrypoint wrapper) — tức một hồi quy ẩn, chỉ lộ khi đổi cách chạy container.

### 7.3 Kết luận dùng được cho báo cáo

Ba tầng, mỗi tầng thấy thứ tầng dưới không thấy:

| Tầng | Công cụ | Thấy được gì |
|---|---|---|
| Mã nguồn | Vitest (1086 test) | logic, hợp đồng hàm, hành vi component |
| Hiện vật build | `gates.sh` C4.3 + C4.5 | hệ quả của bundling/minify: nhánh bị xoá, module bị nhân bản, route bị prerender |
| Môi trường chạy | container + `docker kill -s`, `kubectl` | giao nhận signal, thứ tự tắt, probe qua kubelet |

Nguyên tắc chung lấy từ `multi_agent_coding_workflow.pdf`: **không tin báo cáo "đã xong", chỉ tin
bằng chứng kiểm chứng được độc lập** — và một check chống hồi quy thì bằng chứng của nó là **đã từng
fail đúng lúc cần fail**.

## 8. Việc còn phải làm cho môn kiểm thử

- [ ] Nhận **mẫu tài liệu** của môn học rồi chuyển toàn bộ nội dung sang đúng mẫu
- [ ] Viết test plan tổng theo mẫu (phạm vi, chiến lược, tiêu chí vào/ra, rủi ro)
- [ ] Bảng test case dạng nộp được (ID, tiền điều kiện, bước, kết quả mong đợi, kết quả thực tế)
- [ ] Kịch bản E2E thật trong `tests/e2e` cho luồng mua vé
- [ ] Test cho backend Go/Python khi các service hết là stub
- [ ] Sửa `coverage.exclude` rồi báo lại số coverage **thật** của mã nguồn
- [ ] Bật Kubernetes trong Docker Desktop rồi chạy nốt phần cluster (`runbook.md` §3–§5): đây là
      **ô duy nhất còn “chưa chạy”** trong bảng ở §1
