# EV-184 — Luồng thanh toán + trang kết quả (kể cả hold hết hạn giữa chừng) (Spec)

| Thuộc tính | Chi tiết |
|---|---|
| Jira | EV-184 (mã backlog **EVF-114**) — 5 SP — Priority: **P0** — Sprint 3 |
| Tier | **Critical** (Chạm trực tiếp luồng tiền, giữ kho, xác nhận đơn hàng và phát hành vé) |
| Base Commit | `b44e898` |
| Branch | `feature/EV-184-payment-flow-order-result` |
| Nghiệp vụ | BR-O2 (đồng hồ giữ ghế 10:00 theo `expires_at`, hết hạn trả kho ngay), BR-O5 (Idempotency-Key bắt buộc cho mọi API ghi), BR-O6 (webhook bất đồng bộ, chống duplicate), BR-O8 (vé phát hành QR) |
| Phụ thuộc | EVF-70 (Adapter cổng thanh toán VNPay/Momo/Stripe + sandbox mock) ✅, EVF-113 (Chọn vé + đồng hồ giữ ghế 10:00) ✅ |

## 1. Bối cảnh & Hiện trạng

- Task EV-183 (EVF-113) đã hoàn thành màn chọn vé `/checkout/[eventId]`: người dùng chọn hạng vé và số lượng, tạo hold vé thành công và kích hoạt đồng hồ giữ ghế 10:00 theo `expires_at` của server. Nút "Tiếp tục thanh toán" điều hướng tới `/checkout/[eventId]/payment?orderId=${hold.orderId}&holdId=${hold.holdId}`.
- Hiện tại, route thanh toán `/checkout/[eventId]/payment` và trang kết quả `/checkout/[eventId]/result` chưa được hiện thực.
- Thiết kế UI/UX theo handoff mục 10.9 và 10.10:
  - Trang thanh toán (10.9): Tóm tắt đơn, đồng hồ giữ ghế tiếp tục đếm ngược theo mốc server, lựa chọn cổng thanh toán, form người nhận vé, chống submit trùng lặp, xử lý hold hết hạn ngay tại màn thanh toán.
  - Trang kết quả (10.10): Xử lý 5 trạng thái backend-confirmed riêng biệt: `PENDING` (webhook chậm, đang xác nhận), `PAID` (vé điện tử, QR demo), `FAILED` (thất bại), `EXPIRED` (hold hết hạn giữa chừng), `CANCELLED`.
  - **Nguyên tắc bất biến**: Trình duyệt redirect quay về **không phải là bằng chứng** thanh toán thành công; client phải chờ hoặc poll trạng thái từ server.

## 2. Tiêu chí nghiệm thu (Acceptance Criteria)

- **AC-1 — Màn hình thanh toán & Đồng hồ giữ vé (BR-O2, Stitch 10.9)**:
  - Hiển thị đầy đủ thông tin đơn hàng: tên sự kiện, địa điểm, hạng vé, số lượng, đơn giá, tổng tiền.
  - Đồng hồ giữ vé `ServerExpiryCountdown` tiếp tục đếm ngược theo `expires_at` của server (lấy từ active hold hoặc query params) và offset đồng hồ server.
  - Cho phép người mua chọn phương thức thanh toán: MoMo, VNPay, Sandbox Mock (Thẻ quốc tế / QR).
  - Form nhập thông tin người nhận vé (Họ tên, Email, Số điện thoại) với nhãn trợ năng liên kết `<label for="...">`.
- **AC-2 — Idempotency-Key & Chống submit trùng (BR-O5)**:
  - Mọi thao tác gửi yêu cầu thanh toán (`POST /v1/events/{id}/payments`) đều bắt buộc đính kèm header `Idempotency-Key` (UUID v4).
  - Khi người dùng bấm "Thanh toán ngay", giao diện chuyển sang trạng thái `processing` / `redirecting`, vô hiệu hoá form và nút bấm để chặn double submit.
- **AC-3 — Xử lý Hold hết hạn ngay giữa lúc thanh toán (AC cốt lõi - BR-O2)**:
  - Nếu đồng hồ giữ ghế đếm về 0 khi người dùng đang ở trang thanh toán (hoặc khi quay lại từ cổng thanh toán mà hold đã hết hạn):
    - Khóa ngay nút thanh toán, hiển thị banner cảnh báo: "Đã hết thời gian giữ vé. Số vé của bạn đã được hoàn trả lại kho theo quy định."
    - Tự động dọn dẹp active hold trong `sessionStorage`.
    - Cung cấp nút CTA "Chọn lại vé" điều hướng về `/checkout/[eventId]`.
- **AC-4 — Trang kết quả đơn hàng & Xử lý Webhook chậm (AC cốt lõi - BR-O6, Stitch 10.10)**:
  - Route: `/checkout/[eventId]/result`.
  - Khi chưa có kết quả cuối cùng từ webhook cổng thanh toán: hiển thị trạng thái `PENDING` ("Đang xác nhận thanh toán").
  - Thực hiện polling định kỳ với backoff thích nghi để lấy trạng thái mới nhất từ server (`GET /v1/events/{eventId}/orders/{orderId}`).
  - Tuyệt đối không coi redirect là thanh toán thành công và không báo lỗi kỹ thuật khi webhook đến chậm.
- **AC-5 — Trạng thái Thành công (PAID / ISSUED) & Vé điện tử (BR-O8, Stitch 10.10)**:
  - Khi server xác nhận đơn hàng đã `PAID`: giao diện chuyển sang trạng thái thành công với thông điệp rõ ràng.
  - Hiển thị mã đơn hàng, ngày thanh toán, phương thức đã chọn, danh sách vé điện tử kèm mã QR demo an toàn (ghi chú DEMO, không lộ thông tin nhạy cảm).
  - Cung cấp nút "In / Tải vé" và "Về trang chủ".
- **AC-6 — Trạng thái Thất bại & Đơn quá hạn (FAILED / EXPIRED)**:
  - `FAILED`: Hiển thị lý do thất bại (từ chối giao dịch, số dư không đủ), nút thử lại thanh toán nếu hold còn hạn hoặc nút chọn lại vé nếu hold đã hết hạn.
  - `EXPIRED`: Hiển thị rõ đơn hàng đã bị huỷ do quá hạn giữ vé, cung cấp nút quay về chọn vé.

## 3. Ranh giới (Scope)

- **In Scope**:
  - `apps/web/src/lib/payment-client.ts`: Module client gọi API thanh toán, tạo payment session, poll trạng thái đơn hàng, mock sandbox adapter.
  - `apps/web/src/hooks/use-payment-session.ts`: Hook điều phối khởi tạo thanh toán, kiểm soát hold expiry, form validation và trạng thái submit.
  - `apps/web/src/hooks/use-order-status.ts`: Hook polling trạng thái đơn hàng (adaptive backoff) cho trang kết quả.
  - `apps/web/src/components/checkout/payment-handoff-view.tsx`: Màn hình giao diện thanh toán hoàn chỉnh.
  - `apps/web/src/components/checkout/order-result-view.tsx`: Màn hình giao diện kết quả đơn hàng hoàn chỉnh (5 trạng thái).
  - `apps/web/src/app/(checkout)/checkout/[eventId]/payment/page.tsx`: Next.js route page cho màn thanh toán.
  - `apps/web/src/app/(checkout)/checkout/[eventId]/result/page.tsx`: Next.js route page cho màn kết quả đơn hàng.
  - Toàn bộ unit tests tương ứng với độ bao phủ cao (Coverage >= 70%).
- **Out of Scope**:
  - Cấu hình hạ tầng ngân hàng production thật (VNPay/MoMo credentials thật).
  - Sửa đổi backend Go core ticketing (`services/ticketing`).
