# EV-186 — Console Organizer: tạo sự kiện, trạng thái duyệt, doanh số realtime (Spec)

| Thuộc tính | Chi tiết |
|---|---|
| Jira | EV-186 (mã backlog **EVF-116**) — 5 SP — Priority: **P1** — Sprint 6 |
| Tier | **Standard / SaaS Workspace** (Giao diện điều khiển cho Ban tổ chức sự kiện) |
| Base Commit | `0108c5f` |
| Branch | `feature/EV-186-organizer-console` |
| Nghiệp vụ | **BR-E1** (chỉ SCHEDULED khi có ≥ 1 hạng vé `quota > 0` và `price >= 0`), **BR-E2** (`sale_start_at` cách thời điểm duyệt ≥ 30 phút), **BR-E3** (sau `ON_SALE` cấm giảm quota, chỉ được tăng), **BR-E4** (AI scan 4 trục: nội dung cấm, sai lệch/lừa đảo, bản quyền hình ảnh, thông tin thiếu; rủi ro < 0.3 AUTO_APPROVED, 0.3–0.8 NEEDS_HUMAN, >= 0.8 AUTO_REJECTED), **BR-E5** (ghi audit_log kèm lý do, Moderator có thể lật ngược) |
| Phụ thuộc | EVF-20 (CRUD sự kiện + hạng vé, state machine), EVF-24 (Hàng đợi việc cho Moderator: duyệt / từ chối / lật ngược) |
| Thiết kế UI | `docs/08-stitch-ui-ux-handoff.md` Mục 10.12 (Organizer dashboard/event list), 10.13 (Create/edit event wizard), 10.14 (Event moderation status & sales analytics), 11.8 (Data table), 11.9 (Metric card & chart) |

---

## 1. Bối cảnh & Hiện trạng

- Ban tổ chức (Organizer) là chủ sở hữu sự kiện, chịu trách nhiệm nhập thông tin sự kiện, thiết lập các hạng vé, cấu hình thời gian mở bán (`sale_start_at`) và theo dõi doanh số thực tế khi sự kiện diễn ra.
- Hiện trạng mã nguồn:
  - Route group `(organizer)` mới chỉ có `layout.tsx` và `layout.test.tsx`, chưa có các trang giao diện con (`events`, `events/new`, `events/[eventId]/sales`, `events/[eventId]/review-status`).
  - Cấu hình navigation `ORGANIZER_NAV` trong `apps/web/src/components/layout/nav-config.ts` đang đặt `ready: false` cho `/organizer/events` và `/organizer/events/new`.
  - Nghiệp vụ kiểm duyệt AI 4 trục (BR-E4) và các quy tắc cấu hình vé (BR-E1, BR-E2, BR-E3) cần một giao diện trực quan, rõ ràng, minh bạch thông tin kiểm duyệt và lý do từ chối.
  - Ban tổ chức cần màn hình theo dõi doanh thu và lượng vé bán ra cập nhật realtime (≤ 5s) trong suốt đợt mở bán mà không gây tải giả lập sai lệch.

---

## 2. Tiêu chí nghiệm thu (Acceptance Criteria)

- **AC-1 — Màn hình Danh sách Sự kiện của Ban tổ chức (`/organizer/events` - Stitch 10.12)**:
  - Hiển thị danh sách các sự kiện thuộc quyền sở hữu của Ban tổ chức.
  - Hỗ trợ lọc theo trạng thái vòng đời (`DRAFT`, `PENDING_REVIEW`, `SCHEDULED`, `ON_SALE`, `SOLD_OUT`, `CLOSED`, `REJECTED`, `CANCELLED`) và tìm kiếm theo tên.
  - Mỗi sự kiện hiển thị đầy đủ: Tên sự kiện, địa điểm, thời gian bắt đầu, huy hiệu trạng thái (Status badge có icon và nhãn tiếng Việt), tóm tắt doanh số / số vé bán ra.
  - Cung cấp các hành động nhanh: Tạo sự kiện mới (`/organizer/events/new`), Xem doanh số realtime (`/organizer/events/[eventId]/sales`), Xem trạng thái kiểm duyệt (`/organizer/events/[eventId]/review-status`), Chỉnh sửa (`/organizer/events/[eventId]/edit`).
  - Hỗ trợ trạng thái Empty state khi chưa có sự kiện và trạng thái lọc không có kết quả.

- **AC-2 — Form Tạo / Sửa Sự kiện dạng Stepper Wizard (`/organizer/events/new` - Stitch 10.13, BR-E1, BR-E2, BR-E3)**:
  - Wizard 4 bước trực quan:
    1. **Thông tin cơ bản**: Tên sự kiện, slug, mô tả, địa điểm, thời gian diễn ra (`starts_at`).
    2. **Hạng vé & Quota**: Thêm / sửa / xoá hạng vé (Tên hạng vé, giá bán `price >= 0`, số lượng `quota > 0`, mô tả ngắn).
    3. **Lịch mở bán**: Thiết lập `sale_start_at` (thời điểm mở bán T0).
    4. **Xem lại & Gửi duyệt**: Tóm tắt tổng thể cấu hình sự kiện, cảnh báo chính sách và nút "Gửi kiểm duyệt" hoặc "Lưu nháp".
  - Kiểm tra tính hợp lệ chặt chẽ theo Business Rules:
    - **BR-E1**: Bắt buộc có ít nhất 1 hạng vé với `quota > 0` và `price >= 0` để gửi duyệt.
    - **BR-E2**: Thời điểm `sale_start_at` phải cách thời điểm hiện tại/duyệt tối thiểu 30 phút để hệ thống kịp pre-warm hạ tầng.
    - **BR-E3**: Đối với sự kiện đã ở trạng thái `ON_SALE`, không cho phép giảm `quota` của hạng vé; chỉ cho phép tăng `quota`.
  - Sau khi nộp thành công: Chuyển sự kiện sang trạng thái `PENDING_REVIEW` và điều hướng về trang kiểm duyệt `/organizer/events/[eventId]/review-status`.

- **AC-3 — Bảng Trạng thái Kiểm duyệt & Lý do AI / Moderator (`/organizer/events/[eventId]/review-status` - Stitch 10.14, BR-E4, BR-E5)**:
  - Hiển thị trạng thái kiểm duyệt rõ ràng:
    - `DRAFT`: Chưa gửi duyệt, kèm nút tiếp tục chỉnh sửa.
    - `PENDING_REVIEW`: Đang trong tiến trình quét tự động hoặc chờ Moderator xử lý.
    - `SCHEDULED`: Đã được duyệt thành công (Auto-approved hoặc Moderator duyệt), sẵn sàng mở bán đúng T0.
    - `REJECTED`: Bị từ chối kiểm duyệt (Auto-rejected hoặc Moderator từ chối).
  - Bảng phân tích chi tiết kết quả AI scan 4 trục (BR-E4):
    1. *Nội dung cấm (Prohibited content)*
    2. *Sai lệch / Lừa đảo (Misleading & Scam)*
    3. *Bản quyền hình ảnh (Image copyright)*
    4. *Thông tin thiếu (Missing information)*
  - Hiển thị điểm rủi ro tổng hợp và phân loại:
    - `< 0.3`: `AUTO_APPROVED` (Duyệt tự động)
    - `0.3 – 0.8`: `NEEDS_HUMAN` (Cần Moderator rà soát thủ công)
    - `>= 0.8`: `AUTO_REJECTED` (Từ chối tự động)
  - Khi bị từ chối: Hiển thị minh bạch lý do từ chối (Supporting reasons), ghi chú audit log và nút "Chỉnh sửa & Nộp lại" (Resubmit) để Organizer khắc phục vi phạm.

- **AC-4 — Dashboard Doanh số Realtime Cập nhật trong ≤ 5s (`/organizer/events/[eventId]/sales` - Stitch 10.14, AC back-log)**:
  - Cập nhật số liệu tự động theo chu kỳ ≤ 5s (mặc định 4s, có nút bật/tắt cập nhật tự động và nút "Làm mới ngay").
  - Thẻ chỉ số tổng quan (Metric Cards - Stitch 11.9):
    - **Tổng doanh thu**: Số tiền thu được định dạng VND chuẩn (`formatVND`).
    - **Tổng vé đã bán**: Số vé bán ra kèm tỷ lệ lấp đầy tổng (% sold / total quota).
    - **Đơn hàng thành công**: Tổng số giao dịch mua vé hoàn tất (PAID).
    - **Vé đang giữ (Active Holds)**: Số vé đang được khách giữ trong giỏ 10:00.
  - Bảng chi tiết doanh số theo từng hạng vé:
    - Tên hạng vé, Giá vé, Số vé đã bán, Hạn mức quota, Tỷ lệ lấp đầy (Progress bar trực quan), Doanh thu theo hạng vé.
  - Huy hiệu thông tin: Nhãn trạng thái cập nhật ("Realtime (≤ 5s)"), mốc thời gian cập nhật gần nhất ("Vừa xong" hoặc thời gian cụ thể), thông báo minh bạch dữ liệu mẫu/chính thức.

- **AC-5 — Báo cáo Tổng kết sau bán (Post-sale Report - Stitch 10.14)**:
  - Tích hợp tab/phần Báo cáo tổng kết: Tóm tắt hiệu quả thương mại (Giá trị trung bình mỗi đơn - AOV, Tốc độ bán đỉnh điểm - Peak velocity), Phân bổ phương thức thanh toán (VNPay, MoMo, Thẻ quốc tế).

- **AC-6 — Kích hoạt Navigation & Workspace Shell**:
  - Cập nhật `ORGANIZER_NAV` trong `apps/web/src/components/layout/nav-config.ts` sang `ready: true` cho:
    - `/organizer/events` ("Sự kiện của tôi")
    - `/organizer/events/new` ("Tạo sự kiện")
  - Đảm bảo 100% test hiện có của `layout.test.tsx` và `nav-config.test.ts` đều PASS không bị hồi quy.

---

## 3. Ranh giới (Scope)

### In Scope
- Thư viện client `apps/web/src/lib/organizer-service.ts` quản lý dữ liệu sự kiện, kiểm tra quy tắc BR-E1, BR-E2, BR-E3, phân tích AI 4 trục BR-E4 và bộ đếm doanh số realtime.
- Hook `apps/web/src/hooks/use-organizer-sales.ts` quản lý polling / streaming dữ liệu doanh số realtime ≤ 5s.
- Các component UI:
  - `apps/web/src/components/organizer/event-list.tsx`
  - `apps/web/src/components/organizer/event-wizard.tsx`
  - `apps/web/src/components/organizer/moderation-status-card.tsx`
  - `apps/web/src/components/organizer/sales-dashboard.tsx`
  - `apps/web/src/components/organizer/index.ts`
- Các trang trong App Router `(organizer)`:
  - `apps/web/src/app/(organizer)/organizer/page.tsx` (redirect / overview)
  - `apps/web/src/app/(organizer)/organizer/events/page.tsx`
  - `apps/web/src/app/(organizer)/organizer/events/new/page.tsx`
  - `apps/web/src/app/(organizer)/organizer/events/[eventId]/sales/page.tsx`
  - `apps/web/src/app/(organizer)/organizer/events/[eventId]/review-status/page.tsx`
  - `apps/web/src/app/(organizer)/organizer/events/[eventId]/edit/page.tsx`
- Cập nhật `ORGANIZER_NAV` trong `apps/web/src/components/layout/nav-config.ts` sang `ready: true`.
- Viết 100% unit tests và integration tests cho toàn bộ thư viện, hook, components và pages mới.

### Out of Scope
- Không can thiệp DB Go hay API backend Go (mock client service chuẩn hợp đồng).
- Không can thiệp `services/ai-worker` Python.
