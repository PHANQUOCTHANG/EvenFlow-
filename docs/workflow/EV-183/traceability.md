# EV-183 — Ma trận truy vết AC ↔ Test

Nguồn nghiệp vụ: `docs/01-nghiep-vu.md` BR-O1..BR-O5, BR-Q4, BR-Q7; Jira backlog EV-183 (EVF-113).

| AC | Nội dung yêu cầu | Test File | Test Case Name | Trạng thái |
|---|---|---|---|---|
| **AC-1** | Màn chọn vé & số lượng: hiển thị các hạng vé, nhãn định tính không lộ số lượng tồn kho (BR-O1, BR-O4) | `apps/web/src/components/checkout/checkout-view.test.tsx` | `render danh sách hạng vé và nhãn định tính không lộ số tồn kho (BR-O1)` | **PASS** |
| **AC-1** | Giới hạn số lượng mua do server quy định, cho phép tăng/giảm số lượng và cập nhật tóm tắt | `apps/web/src/components/checkout/checkout-view.test.tsx` | `cho phép tăng số lượng và cập nhật tóm tắt đơn hàng` | **PASS** |
| **AC-2** | Đồng hồ giữ ghế 10:00 theo `expires_at` của server (BR-O2): client không tự tính, khôi phục từ storage nếu còn hạn | `apps/web/src/hooks/use-hold-timer.test.tsx` | `khôi phục active hold từ sessionStorage nếu còn hạn (BR-O2)` | **PASS** |
| **AC-2** | Đổi giờ máy khách không kéo dài được thời gian giữ ghế: tự động chuyển sang expired khi quá hạn | `apps/web/src/hooks/use-hold-timer.test.tsx` | `đánh dấu expired nếu hold trong sessionStorage đã quá hạn` | **PASS** |
| **AC-2** | **Đổi giờ máy khách không kéo dài được thời gian giữ ghế**: offsetMs bù trừ giờ lệch, tính đúng thời gian thực của server | `apps/web/src/hooks/use-hold-timer.test.tsx` | `Đổi giờ máy khách không kéo dài được thời gian giữ ghế (BR-O2)` | **PASS** |
| **AC-3** | Bắt buộc Idempotency-Key UUIDv4 cho mọi thao tác ghi (BR-O5) | `apps/web/src/lib/hold-client.test.ts` | `generateIdempotencyKey > sinh chuỗi uuid hợp lệ` | **PASS** |
| **AC-3** | Tự sinh Idempotency-Key và đính kèm header khi gửi request tạo hold | `apps/web/src/lib/hold-client.test.ts` | `tự sinh Idempotency-Key nếu caller không truyền (BR-O5)` | **PASS** |
| **AC-3** | Bấm đặt nhiều lần không tạo hold trùng: gửi requestHold với Idempotency-Key | `apps/web/src/components/checkout/checkout-view.test.tsx` | `khi bấm 'Giữ vé và thanh toán', gọi requestHold với Idempotency-Key (BR-O5)` | **PASS** |
| **AC-3** | **Bấm đặt nhiều lần không tạo hold trùng (Hook)**: chặn rapid click đồng thời qua isCreatingRef, chỉ gọi requestHold đúng 1 lần | `apps/web/src/hooks/use-hold-timer.test.tsx` | `Bấm đặt nhiều lần không tạo hold trùng (BR-O5)` | **PASS** |
| **AC-3** | **Bấm đặt nhiều lần không tạo hold trùng (UI)**: nút bấm vô hiệu hoá và chặn click dồn dập trên giao diện | `apps/web/src/components/checkout/checkout-view.test.tsx` | `bấm đặt nhiều lần nhanh trên UI không tạo hold trùng (BR-O5)` | **PASS** |
| **AC-4** | Vòng đời Hold: tạo hold thành công chuyển sang active và lưu storage | `apps/web/src/hooks/use-hold-timer.test.tsx` | `tạo hold thành công: chuyển sang active và lưu storage (BR-O2, BR-O5)` | **PASS** |
| **AC-4** | Hết vé (409 Conflict) chuyển trạng thái sang sold_out | `apps/web/src/hooks/use-hold-timer.test.tsx` | `chuyển sang sold_out khi nhận lỗi 409 từ server` | **PASS** |
| **AC-4** | Lỗi mạng / timeout chuyển trạng thái sang ambiguous (BR-O5) | `apps/web/src/hooks/use-hold-timer.test.tsx` | `chuyển sang ambiguous khi nhận lỗi mạng / không rõ kết quả (BR-O5)` | **PASS** |
| **AC-4** | Nút 'Tiếp tục thanh toán' điều hướng / gọi callback thanh toán | `apps/web/src/components/checkout/checkout-view.test.tsx` | `khi hold active, bấm 'Tiếp tục thanh toán' gọi onProceedToPayment callback` | **PASS** |
| **AC-4** | Reset hold giải phóng state về idle và xoá storage | `apps/web/src/hooks/use-hold-timer.test.tsx` | `resetHold giải phóng state về idle và xoá storage` | **PASS** |
| **AC-5** | Cảnh báo khi có active hold ở hạng khác (BR-O3) | `apps/web/src/components/checkout/checkout-view.test.tsx` | `khi có active hold, hiển thị cảnh báo ở hạng vé khác (BR-O3)` | **PASS** |
| **AC-6** | Kiểm tra quyền vào lượt: từ chối tạo hold khi không có queue token | `apps/web/src/hooks/use-hold-timer.test.tsx` | `từ chối tạo hold nếu không có queueToken (chưa qua phòng chờ)` | **PASS** |
| **AC-6** | Hiển thị cảnh báo chưa admit và nút quay lại phòng chờ (AC-6, BR-Q4) | `apps/web/src/components/checkout/checkout-view.test.tsx` | `hiển thị cảnh báo chưa admit và nút quay lại phòng chờ khi không có token (AC-6)` | **PASS** |
| **Route** | Route page `/checkout/[eventId]` render CheckoutView component | `apps/web/src/app/(checkout)/checkout/[eventId]/page.test.tsx` | `render CheckoutView component khi tìm thấy sự kiện` | **PASS** |
| **Route** | Route page gọi notFound khi không tìm thấy sự kiện | `apps/web/src/app/(checkout)/checkout/[eventId]/page.test.tsx` | `gọi notFound khi không tìm thấy sự kiện` | **PASS** |
| **Route** | Metadata sinh title chính xác và noindex | `apps/web/src/app/(checkout)/checkout/[eventId]/page.test.tsx` | `generateMetadata tạo title chính xác và noindex` | **PASS** |
