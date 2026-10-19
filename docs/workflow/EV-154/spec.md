# EV-154 (EVF-74) — Luồng Hoàn Tiền Khi Sự Kiện Bị Hủy (Spec)

| Thuộc tính | Chi tiết |
|---|---|
| Jira Ticket | **EV-154** (Mã backlog: **EVF-74**) |
| Epic / Module | E6 – Thanh toán (`EV-149`) / Payment Reliability & Refunds |
| Story Points | **5 SP** — Priority: **P2 / Sprint 6** |
| Tier | **Critical / Financial Integrity & Data Reliability** (Bảo đảm hoàn tiền chính xác, không hoàn trùng, tự động giải phóng hạn mức vé khi hủy sự kiện) |
| Base Commit | `f2154d829e80df2d795864a4a28e94547baff0b4` |
| Branch | `feature/EV-154-refund-cancelled-event` |
| Nghiệp vụ liên quan | **BR-E1** (Vòng đời sự kiện: `CANCELLED`), **BR-O4** (Hạn mức mua: `REFUNDED` trả lại hạn mức), **BR-O5** (Idempotency), **BR-O6** (Exactly-once & Chống replay), **BR-O7** (Tính Idempotent), **4 Quy tắc Bất biến** |

---

## 1. Bối cảnh & Hiện trạng

### Bối cảnh nghiệp vụ
Theo quy định tại [docs/01-nghiep-vu.md](file:///c:/Users/ADMIN/OneDrive/Documents/GitHub/EvenFlow-/docs/01-nghiep-vu.md) (Mục 2.1 & 2.3):
- Khi Ban tổ chức (Organizer) hủy một sự kiện (`status = CANCELLED`), sự kiện dừng mọi hoạt động bán vé.
- Toàn bộ các vé đã phát hành (`ISSUED`) và các đơn hàng đã thanh toán thành công (`PAID` hoặc `ISSUED`) thuộc sự kiện đó phải được hoàn tiền toàn bộ:
  ```
  ISSUED --organizer huy su kien--> REFUNDING --> REFUNDED
  ```
- Khách hàng cần được hoàn tiền tự động về tài khoản/thẻ ngân hàng đã dùng khi thanh toán mà không cần phải khiếu nại thủ công.
- Sự kiện outbox `payment.refunded` phải được phát hành để dịch vụ thông báo (`notification-svc`) gửi email/SMS xác nhận hoàn tiền cho khách hàng.

### Hiện trạng mã nguồn trước EV-154
- **EV-150**: Hoàn thành cổng thanh toán (Sandbox, VNPay, MoMo, Stripe) với các hàm khởi tạo giao dịch (`InitiatePayment`) và kiểm tra callback (`VerifyCallback`).
- **EV-151**: Hoàn thành webhook exactly-once, chống replay.
- **EV-152**: Hoàn thành `payment_inbox` cho webhook đến sớm và replay.
- **EV-153**: Hoàn thành đối soát chủ động và tự chữa cho đơn treo quá 15 phút (`QueryPayment` và `ReconcileStaleOrdersUseCase`).
- **EV-133**: Hoàn thành phát hành vé (`ISSUED`) và QR HMAC.
- **Gaps còn thiếu (Trước EV-154)**:
  - Cổng giao tiếp `PaymentGateway` chưa hỗ trợ phương thức hoàn tiền `RefundPayment(ctx, req)`.
  - Chưa có adapter gọi API hoàn tiền cho cả 4 nhà cung cấp (`SandboxGateway`, `VNPayGateway`, `MoMoGateway`, `StripeGateway`).
  - Chưa có usecase xử lý hoàn tiền theo lô cho sự kiện bị hủy (`RefundCancelledEventUseCase`).
  - Chưa có hàm cập nhật trạng thái đơn hàng sang `REFUNDING` và hoàn tất sang `REFUNDED` đồng bộ với bảng `tickets`, `payments` và `orders` trong cùng một Database Transaction (Quy tắc bất biến 1).
  - Chưa có cơ chế phát sự kiện outbox `payment.refunded` để thông báo cho khách hàng.
  - Chưa có consumer tiếp nhận message `event.cancelled` từ RabbitMQ cũng như endpoint HTTP phục vụ Ops/Trigger kiểm thử.

---

## 2. Tiêu chí nghiệm thu (Acceptance Criteria - AC)

### AC-1 — Quét toàn bộ đơn hàng cần hoàn tiền khi sự kiện bị hủy (Cancelled Event Order Scanning)
- **Điều kiện**: Nhận tín hiệu sự kiện bị hủy qua message `event.cancelled` hoặc endpoint HTTP kích hoạt hoàn tiền.
- **Hành vi**:
  - Truy vấn tất cả các đơn hàng thuộc `event_id` đang ở các trạng thái có thể hoàn tiền: `PAID`, `ISSUED`, hoặc `REFUNDING`.
  - Bỏ qua các đơn ở trạng thái kết thúc không cần hoàn: `CANCELLED` (chưa thanh toán), `EXPIRED`, hoặc `REFUNDED` (đã hoàn tiền trước đó).
  - Phân trang hoặc chia lô (batching) an toàn (mặc định 50 đơn/lô) để tránh quá tải bộ nhớ và nghẽn kết nối database.

### AC-2 — Chuyển trạng thái sang `REFUNDING` và gọi cổng thanh toán ngoài Transaction (Quy tắc 2)
- **Điều kiện**: Mỗi đơn hàng hợp lệ được chuẩn bị hoàn tiền.
- **Hành vi**:
  - Cập nhật trạng thái `orders.status = 'REFUNDING'` để đánh dấu giao dịch đang được xử lý hoàn tiền (chống xử lý trùng lặp bởi các worker khác).
  - Gọi phương thức `RefundPayment` của `PaymentGateway` tương ứng với nhà cung cấp ban đầu của đơn hàng.
  - **Quy tắc 2**: Toàn bộ tương tác mạng với bên thứ ba (Sandbox, VNPay, MoMo, Stripe) được thực hiện NGOÀI database transaction, có context timeout nghiêm ngặt (mặc định 10s/request).

### AC-3 — Hỗ trợ đầy đủ API hoàn tiền trên cả 4 Cổng thanh toán (Gateway Refund Adapters)
- **Sandbox**:
  - Giả lập hoàn tiền thành công ngay lập tức ở chế độ chuẩn (`SandboxModeSuccess`, `SandboxModeDelay`).
  - Giả lập hoàn tiền thất bại khi ở chế độ `SandboxModeFail` để phục vụ kiểm thử resilience.
- **VNPay**:
  - Xây dựng request hoàn tiền theo chuẩn VNPay Refund API (`vnp_Command=refund`, `vnp_TransactionType=02` hoàn toàn phần, `vnp_CreateBy=admin`).
  - Tạo mã kiểm tra tính toàn vẹn (checksum) và parse response code (`00` là thành công).
- **MoMo**:
  - Xây dựng request hoàn tiền theo MoMo Refund API (`/v2/gateway/api/refund`) với `transId`, `amount`, chữ ký HMAC-SHA256.
  - Parse `resultCode = 0` là thành công.
- **Stripe**:
  - Tạo refund request tới Stripe API (`/v1/refunds`) với `payment_intent` hoặc `charge` và `amount`.
  - Parse trạng thái `status = succeeded`.

### AC-4 — Cập nhật trạng thái Atomic trong Transaction Database (Quy tắc 1 & BR-O4)
- **Điều kiện**: Cổng thanh toán xác nhận hoàn tiền thành công (`REFUNDED`).
- **Hành vi**:
  - Trong **CÙNG MỘT TRANSACTION DATABASE**:
    1. Cập nhật `orders.status = 'REFUNDED'`, `updated_at = now()`.
    2. Cập nhật `payments.status = 'REFUNDED'`, lưu mã `refund_txn_id` và thông tin chi tiết vào `raw_payload`.
    3. Cập nhật `tickets.status = 'REFUNDED'` cho toàn bộ vé liên kết với đơn hàng này.
    4. Ghi nhận sự kiện vào `outbox_events` với `aggregate_type = 'payment'`, `routing_key = 'payment.refunded'` chứa thông tin: `order_id`, `event_id`, `identity_id`, `amount_cents`, `currency`, `refund_txn_id`, `refunded_at`.
  - **BR-O4 (Phục hồi hạn mức)**: Khi đơn hàng chuyển sang `REFUNDED`, hạn mức mua vé của khách hàng trên sự kiện đó lập tức được hoàn trả (bởi vì `SettledOrderStatuses` chỉ chiếm hạn mức khi ở `PAID`, `ISSUED`, `REFUNDING`).

### AC-5 — Tính Idempotent tuyệt đối & Chống hoàn tiền trùng lặp (Idempotent Refund & Anti-Replay)
- **Điều kiện**: Khi có retry từ message queue, hoặc người dùng/ops kích hoạt lại lệnh hoàn tiền cho sự kiện.
- **Hành vi**:
  - Nếu đơn hàng đã ở trạng thái `REFUNDED`: Hệ thống bỏ qua an toàn, ghi nhận `AlreadyDone = true`, không gọi lại cổng thanh toán.
  - Mỗi yêu cầu hoàn tiền gửi tới cổng thanh toán đều gắn kèm mã định danh giao dịch duy nhất (`RefundID` / `Idempotency-Key`) để cổng thanh toán bên thứ ba từ chối trừ tiền lần hai.

### AC-6 — Xử lý lỗi linh hoạt và cho phép Retry (Failure Resilience)
- **Điều kiện**: Cổng thanh toán trả về lỗi tạm thời (timeout mạng, 5xx).
- **Hành vi**:
  - Đơn hàng giữ nguyên ở trạng thái `REFUNDING`, ghi nhận lỗi trong log có cấu trúc.
  - Không rollback các đơn hàng khác đã hoàn tiền thành công trong cùng lô (mỗi đơn hàng được cách ly độc lập).
  - Cung cấp kết quả tổng hợp: `TotalOrders`, `RefundedCount`, `FailedCount`, `SkippedCount`, `Duration`.

### AC-7 — Tích hợp HTTP REST API & Event Consumer (Endpoints & Integration)
- **HTTP Endpoint**:
  - `POST /v1/events/{eventID}/refund`: Endpoint cho phép Ops hoặc ban quản trị kích hoạt hoàn tiền theo sự kiện (bắt buộc `Idempotency-Key` theo BR-O5).
  - Trả về JSON chuẩn với số lượng đơn đã hoàn, số tiền và thời gian thực thi.
- **Event Consumer**:
  - Sẵn sàng tích hợp consumer lắng nghe routing key `event.cancelled` từ RabbitMQ để tự động kích hoạt usecase hoàn tiền.

---

## 3. Ranh giới (Scope)

### In Scope
- Mở rộng port `PaymentGateway` và triển khai `RefundPayment` trên 4 gateway adapters (Sandbox, VNPay, MoMo, Stripe).
- Bổ sung các phương thức repository: `GetPaidOrdersByEvent`, `MarkOrderRefundingTx`, `ProcessRefundSuccessTx`, `ProcessRefundFailedTx` trên cả `PostgresRepository` và `MemoryRepository`.
- Xây dựng use case `RefundCancelledEventUseCase` với đầy đủ unit tests (TDD).
- Endpoint HTTP `POST /v1/events/{eventID}/refund` trong `Handler`.
- Ghi nhận outbox event `payment.refunded`.

### Out of Scope
- Gửi email/SMS trực tiếp từ payment-svc (vi phạm Quy tắc 2: việc gửi email/SMS thuộc trách nhiệm của `notification-svc` thông qua việc tiêu thụ outbox event `payment.refunded`).
- Can thiệp vào luồng kiểm duyệt sự kiện hay đổi trạng thái sự kiện trong DB `events` (thuộc `event-svc`).
