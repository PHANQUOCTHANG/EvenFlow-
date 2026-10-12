# EV-183 — Chọn vé + đồng hồ giữ ghế 10:00 theo expires_at của server (Spec)

| Thuộc tính | Chi tiết |
|---|---|
| Jira | EV-183 (mã backlog **EVF-113**) — 8 SP — Priority: **P0** — Sprint 3 |
| Tier | **Critical** (Chạm trực tiếp luồng đặt vé, giữ kho, chống oversell và tính tiền) |
| Base Commit | `7aceaa8` |
| Branch | `feature/EV-183-ticket-selection-hold-timer` |
| Nghiệp vụ | BR-O1 (không lộ tồn kho, nhãn khái quát), BR-O2 (client không tự tính giờ, lấy `expires_at`), BR-O3 (1 hold/khách/sự kiện), BR-O4 (giới hạn mua do server), BR-O5 (Idempotency-Key cho mọi API ghi) |
| Phụ thuộc | EVF-32 (Backend create hold trong transaction) ✅, EVF-33 (Middleware Idempotency-Key) ✅, EVF-112 (UI phòng chờ dẫn tới checkout) ✅ |

## 1. Bối cảnh & Hiện trạng

- Task EV-182 (EVF-112) đã hoàn thành UI phòng chờ: khi trạng thái chuyển sang `ADMITTED`, client lưu `queue_token` vào `sessionStorage` (`evenflow_queue_token_${eventId}`) và tự động điều hướng tới `/checkout/[eventId]`.
- Hiện tại route `/checkout/[eventId]` chưa có `page.tsx` (chỉ mới có `(checkout)/layout.tsx`).
- Các component nền tảng đã sẵn sàng và có unit test đầy đủ:
  - `TicketTierCard` (`apps/web/src/components/checkout/ticket-tier-card.tsx`): hiển thị hạng vé, nhãn định tính (`available`, `limited`, `sold_out`), chọn số lượng tuân thủ `maxSelectable` (BR-O4) và cảnh báo `hasActiveHoldElsewhere` (BR-O3).
  - `OrderSummary` (`apps/web/src/components/checkout/order-summary.tsx`): tóm tắt đơn hàng, tích hợp `ServerExpiryCountdown` (variant `hold`), các trạng thái `idle`, `creating`, `active`, `expired`, `sold_out`, `ambiguous` (BR-O5).
  - `useServerCountdown` (`apps/web/src/hooks/use-server-countdown.ts`) & `ServerExpiryCountdown`: đếm ngược theo mốc tuyệt đối của server, không bị trôi khi tab ẩn.
  - `useServerTimeSync` (`apps/web/src/hooks/use-server-time-sync.ts`): đồng bộ offset đồng hồ client/server qua `/api/time`.
- Backend endpoint:
  - `POST /v1/events/{eventID}/holds`:
    - Headers bắt buộc: `Idempotency-Key` (BR-O5), `X-Queue-Token`, `X-Identity-Id`.
    - Body: `{ "ticket_type_id": string, "quantity": number }`.
    - Phản hồi: `200 OK` trả về `HoldView`: `{ "order_id": string, "hold_id": string, "expires_at": string }`.
    - Lỗi: `400` (thiếu idempotency key/body), `403` (chưa admit), `409` (hết vé), `422` (vượt giới hạn mua), `503` (hệ thống bận).

## 2. Tiêu chí nghiệm thu (Acceptance Criteria)

- **AC-1 — Màn chọn hạng vé & số lượng (BR-O1, BR-O4)**:
  - Hiển thị danh sách các hạng vé của sự kiện lấy từ `event-service` (`TicketTierSnapshot`).
  - Tồn kho chỉ thể hiện bằng nhãn định tính (`AVAILABLE`, `FEW_LEFT`, `SOLD_OUT`), tuyệt đối không phơi bày số lượng cụ thể (BR-O1).
  - Giới hạn số lượng chọn tối đa tuân thủ `maxSelectable` (mặc định cho phép cấu hình từ server, kiểm soát không cho chọn vượt quá giới hạn).
- **AC-2 — Đồng hồ giữ ghế 10:00 theo `expires_at` của server (BR-O2)**:
  - Client không tự tính thời gian giữ ghế: đồng hồ đếm ngược dựa trên trường `expires_at` (ISO-8601 có múi giờ hoặc timestamp) do server trả về trong phản hồi tạo hold.
  - Đổi giờ máy khách (chỉnh đồng hồ hệ điều hành tới/lùi) **không kéo dài** được thời gian giữ ghế: sử dụng `useServerTimeSync` đo offset hoặc mốc tuyệt đối.
  - Khi tab bị background hoặc ẩn đi, thời gian còn lại được tính lại từ mốc, không giảm dần theo số nhịp interval.
- **AC-3 — Idempotency-Key & Chống tạo hold trùng (BR-O5)**:
  - Mọi thao tác gửi yêu cầu tạo hold đều bắt buộc sinh và đính kèm `Idempotency-Key` (UUID v4) trong request header.
  - Khi người dùng bấm nút đặt giữ vé, giao diện chuyển sang trạng thái `creating`, vô hiệu hoá nút bấm để ngăn chặn double-click / rapid clicks.
  - Nếu kết nối mạng gián đoạn và client retry, phải tái sử dụng cùng một `Idempotency-Key` cho cùng phiên đặt vé đó để server trả về hold cũ thay vì tạo hold mới.
  - Nếu request gặp sự cố mạng hoặc lỗi không xác định kết quả, UI chuyển sang trạng thái `ambiguous` (cảnh báo khách không gửi lại yêu cầu để tránh giữ trùng kho).
- **AC-4 — Quản lý vòng đời Hold (BR-O2, BR-O3, BR-O5)**:
  - `idle`: Khách chưa bấm giữ vé, nút chính hiển thị "Giữ vé và thanh toán".
  - `creating`: Đang gửi request lên backend, hiển thị spinner và khóa tương tác.
  - `active`: Đã giữ vé thành công; hiển thị đồng hồ đếm ngược `ServerExpiryCountdown` (variant `hold`); nút chính chuyển thành "Tiếp tục thanh toán" dẫn tới luồng thanh toán `/checkout/[eventId]/payment`.
  - `expired`: Khi đồng hồ đếm về 0, giao diện tự động chuyển sang trạng thái `expired`, hiển thị thông báo "Đã hết thời gian giữ vé" và đổi nút thành "Chọn lại vé", huỷ bỏ hold cục bộ.
  - `sold_out`: Nếu backend trả 409 (hết vé), chuyển trạng thái sang `sold_out` và không cho phép thanh toán hạng vé đó.
- **AC-5 — Cảnh báo và xử lý hold hiện có (BR-O3)**:
  - Một khách chỉ có 1 hold hoạt động cho mỗi sự kiện.
  - Khi đã có hold active, nếu khách chọn xem hoặc chuyển sang hạng vé khác, hiển thị cảnh báo `hasActiveHoldElsewhere`: "Mỗi khách chỉ giữ được một lượt vé... Hoàn tất thanh toán phần đang giữ, hoặc đợi nó hết hạn, rồi mới chọn được hạng vé khác."
- **AC-6 (Negative) — Kiểm tra quyền vào lượt (Admitted Guard / Queue Token)**:
  - Nếu người dùng truy cập trực tiếp `/checkout/[eventId]` mà không có `queue_token` trong `sessionStorage`, hoặc backend trả `403 Forbidden` (`ErrNotAdmitted`), hiển thị banner thông báo chưa được cấp lượt và điều hướng / cung cấp nút quay lại phòng chờ `/waiting/[eventId]`.

## 3. Ranh giới (Scope)

- **In Scope**:
  - `apps/web/src/lib/hold-client.ts`: Module client gọi API `POST /v1/events/{id}/holds` kèm header `Idempotency-Key`, `X-Queue-Token`, `X-Identity-Id`, xử lý response 200, 403, 409, 422, ambiguous.
  - `apps/web/src/hooks/use-hold-timer.ts`: Hook quản lý trạng thái hold, tích hợp countdown đếm ngược theo `expires_at` và `offsetMs`, chống đổi giờ máy khách.
  - `apps/web/src/components/checkout/checkout-view.tsx`: Màn hình giao diện chọn vé hoàn chỉnh kết hợp `TicketTierCard`, `OrderSummary`, cảnh báo phòng chờ.
  - `apps/web/src/app/(checkout)/checkout/[eventId]/page.tsx`: Next.js page route cho `/checkout/[eventId]`.
  - Toàn bộ unit tests: `hold-client.test.ts`, `use-hold-timer.test.tsx`, `checkout-view.test.tsx`, `page.test.tsx`.
- **Out of Scope**:
  - Tích hợp cổng thanh toán VNPay/Momo/Stripe và trang kết quả đơn hàng (thuộc task tiếp theo EV-184 / EVF-114).
  - Sửa đổi backend Go (`services/ticketing`).
  - Sửa đổi các component đã được test kỹ ở EVF-1802 (`ticket-tier-card.tsx`, `order-summary.tsx`) ngoại trừ trường hợp cần mở rộng type tương thích ngược (không làm gãy test baseline).
