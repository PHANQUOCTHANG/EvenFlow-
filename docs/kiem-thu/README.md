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
| Unit + Component (frontend) | Vitest 2.1.8 + React Testing Library + jsdom | **Đang chạy, 996 test** |
| E2E | Playwright (`tests/e2e`) | Khung đã dựng, chưa có kịch bản thật |
| Unit (Go) | `go test` | CI có job `test-go`, code phần lớn là stub |
| Unit (Python) | pytest | CI có job `test-python`, chưa có test |
| Integration | testcontainers | `make test-integration`, cần Docker |
| Cổng không oversell | `make test-oversell` (TestNoOversell ×200) | Có workflow riêng `oversell-gate.yml` |
| Quét lỗ hổng image | Trivy, chặn mức CRITICAL | `trivy.yml` |

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

Chạy ngày 2026-10-02. **996 test / 40 file.**

| Nhóm | File | Số test |
|---|---|---|
| **Thư viện** | `lib/format.test.ts` | 66 |
| | `lib/server-time.test.ts` | 39 |
| | `lib/money.test.ts` | 19 |
| | `lib/queue-client.test.ts` | 11 |
| | `lib/theme.test.ts` | 11 |
| | `lib/cn.test.ts` | 7 |
| **Hook** | `hooks/use-server-countdown.test.tsx` | 48 |
| | `hooks/use-theme.test.tsx` | 13 |
| **Component miền** | `components/queue/queue-status-panel.test.tsx` | 112 |
| | `components/checkout/order-summary.test.tsx` | 67 |
| | `components/checkout/ticket-tier-card.test.tsx` | 45 |
| | `components/event/event-card.test.tsx` | 35 |
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

## 4. Quy trình sinh ra bộ test này

Theo `multi_agent_coding_workflow.pdf` của nhóm. Điểm quan trọng với môn kiểm thử:

**Người viết test KHÔNG phải người viết mã.** Mỗi task có một tác tử riêng viết test từ
acceptance criteria, **không được đọc mã hiện thực**. Nếu người viết mã cũng viết test thì test chỉ
mô tả lại mã, không phát hiện được sai lệch so với yêu cầu.

**Mỗi AC phải có ít nhất một test hoặc một bằng chứng chạy được**, ghi trong ma trận truy vết của
từng task.

**Không được sửa, xoá, skip test để cho qua.** Khi test đỏ, phải xác định test sai hay mã sai rồi
mới xử lý, và ghi lại kết luận. Toàn bộ repo hiện **không có** `.skip` / `.only` / `it.todo` nào.

## 5. Ma trận truy vết theo từng tính năng

| Tính năng | Ma trận truy vết | Đặc tả + AC |
|---|---|---|
| Design system, token, dark mode | `docs/workflow/EVF-1801/traceability.md` | `docs/workflow/EVF-1801/spec.md` |
| Route group + app shell | `docs/workflow/EVF-1801-route-groups/traceability.md` | `.../spec-plan.md` |
| Đồng hồ theo giờ server + panel hàng chờ | `docs/workflow/EVF-1803-queue-countdown/traceability.md` | `.../spec-plan.md` |
| Thẻ sự kiện, hạng vé, tóm tắt đơn | `docs/workflow/EVF-1802-event-cards/traceability.md` | `.../spec-plan.md` |

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
| `docker build`, `make test` | Máy dev chưa chạy Docker; không có `make` và Go toolchain | CI (`trivy.yml` build image, `ci.yml` chạy `test-go`/`test-python`) |

## 7. Việc còn phải làm cho môn kiểm thử

- [ ] Nhận **mẫu tài liệu** của môn học rồi chuyển toàn bộ nội dung sang đúng mẫu
- [ ] Viết test plan tổng theo mẫu (phạm vi, chiến lược, tiêu chí vào/ra, rủi ro)
- [ ] Bảng test case dạng nộp được (ID, tiền điều kiện, bước, kết quả mong đợi, kết quả thực tế)
- [ ] Kịch bản E2E thật trong `tests/e2e` cho luồng mua vé
- [ ] Test cho backend Go/Python khi các service hết là stub
- [ ] Sửa `coverage.exclude` rồi báo lại số coverage **thật** của mã nguồn
