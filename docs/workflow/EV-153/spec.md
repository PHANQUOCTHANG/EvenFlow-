# EV-153 (EVF-73) — Job Đối Soát: Đơn Quá 15 Phút Không Có Webhook → Hỏi Cổng Thanh Toán, Tự Chữa (Spec)

| Thuộc tính | Chi tiết |
|---|---|
| Jira Ticket | **EV-153** (Mã backlog: **EVF-73**) |
| Epic / Module | E6 – Thanh toán (`EV-149`) / Payment Reliability & Reconciliation |
| Story Points | **5 SP** — Priority: **Highest / P0** |
| Tier | **Critical / High-Reliability** (Chống treo đơn, giải phóng ghế tồn đọng, tự chữa dữ liệu tiền tệ & trạng thái vé) |
| Base Commit | `07aabf473be3015e7f19a8cc00417f089d929b28` |
| Branch | `feature/EV-153-stale-order-reconciliation` |
| Nghiệp vụ liên quan | **BR-O1** (Không oversell), **BR-O2** (Hạn giữ ghế), **BR-O6** (Webhook exactly-once & đối soát sau 15 phút), **BR-O7** (Trả kho idempotent: mỗi hold chỉ release 1 lần), **4 Quy tắc Bất biến** |

---

## 1. Bối cảnh & Hiện trạng

### Bối cảnh kỹ thuật
Trong hệ thống bán vé chịu tải cao EvenFlow, khi khách hàng bấm thanh toán, đơn hàng chuyển từ `HELD` sang `PAYMENT_PENDING`. Thông thường, cổng thanh toán sẽ gửi Webhook/IPN về server để xác nhận giao dịch thành công hoặc thất bại. Tuy nhiên, trong môi trường phân tán thực tế:
- Cổng thanh toán bên thứ ba gặp sự cố mạng, drop webhook hoặc retry thất bại.
- Khách hàng đóng trình duyệt hoặc quét mã QR thanh toán thành công nhưng webhook không bao giờ gửi đến hệ thống.
- Ngược lại, khách hàng từ bỏ thanh toán (abandoned checkout), dẫn tới đơn hàng bị kẹt vĩnh viễn ở trạng thái `PAYMENT_PENDING`. Nếu không có cơ chế thu hồi, số vé này sẽ bị giữ ảo, gây ra hiện tượng "hết vé giả" (false sold-out), vi phạm nghiêm trọng trải nghiệm người dùng và doanh thu sự kiện.

### Hiện trạng mã nguồn trước EV-153
- **EV-150**: Đã hoàn thành Gateway Adapters (Sandbox, VNPay, MoMo, Stripe). Riêng `SandboxGateway` đã chuẩn bị sẵn cờ `SandboxModeDelay` để giả lập tình huống webhook không tới.
- **EV-151**: Đã hoàn thành xử lý webhook exactly-once với xác thực chữ ký và chống replay.
- **EV-152**: Đã hoàn thành `payment_inbox` cho webhook đến sớm hơn đơn hàng và định kỳ replay.
- **Gaps còn thiếu (Trước EV-153)**:
  - Chưa có cơ chế quét các đơn `PAYMENT_PENDING` bị treo quá 15 phút.
  - Chưa có cổng giao tiếp truy vấn chủ động trạng thái giao dịch từ các cổng thanh toán (`QueryPayment` status API).
  - Chưa có use case đối soát chủ động và tự chữa:
    - Nếu cổng báo khách đã trả tiền thành công -> Chuyển `PAID`, upsert `payments` sang `SUCCEEDED`, phát sự kiện outbox `order.paid`.
    - Nếu cổng báo giao dịch thất bại / hủy / không tìm thấy sau 15 phút -> Chuyển `CANCELLED`, trả kho vé idempotent (`seat_holds` & `ticket_inventory`), cập nhật `payments` sang `FAILED`.
  - Chưa export metric Prometheus `payment_webhook_lag_seconds` theo yêu cầu của Jira ticket và kiến trúc quan sát (Observability Architecture - docs/02).

---

## 2. Tiêu chí nghiệm thu (Acceptance Criteria - AC)

### AC-1 — Quét đơn `PAYMENT_PENDING` quá 15 phút (Stale Order Scanning)
- **Điều kiện**: Đơn hàng có `status = 'PAYMENT_PENDING'` và thời gian tồn tại vượt quá ngưỡng quy định (mặc định 15 phút theo BR-O6: `now() - updated_at >= 15m`).
- **Hành vi**:
  - Truy vấn danh sách đơn treo có kèm thông tin thanh toán gần nhất (`payments` record nếu có).
  - Phân trang / batch limit an toàn (mặc định 50 đơn/lần quét) để tránh khóa database hay nghẽn bộ nhớ.
  - Loại trừ các đơn đã ở trạng thái kết thúc (`PAID`, `CANCELLED`, `EXPIRED`, `REFUNDED`).

### AC-2 — Truy vấn chủ động trạng thái từ Cổng thanh toán (Active Gateway Query)
- **Điều kiện**: Đối với mỗi đơn treo, hệ thống gọi cổng thanh toán tương ứng để xác minh trạng thái tiền tệ thực tế.
- **Hành vi**:
  - Mở rộng port `PaymentGateway` với phương thức `QueryPayment(ctx context.Context, req PaymentQueryRequest) (PaymentQueryResult, error)`.
  - Triển khai đầy đủ trên cả 4 cổng:
    - **Sandbox**: Tra cứu trạng thái giao dịch giả lập (hỗ trợ các mode Success, Fail, Delay).
    - **VNPay**: Hỗ trợ QueryDR (`vnp_Command=querydr`) với HMAC checksum và parsing kết quả giao dịch.
    - **MoMo**: Hỗ trợ Query Transaction Status API (`/v2/gateway/api/query`) với chữ ký số HMAC-SHA256.
    - **Stripe**: Hỗ trợ PaymentIntent / Checkout Session retrieve status.
  - Trả về cấu trúc chuẩn `PaymentQueryResult`: `Status` (`SUCCEEDED`, `FAILED`, `PENDING`), `ProviderTxnID`, `AmountCents`, `RawResponse`.
  - **Quy tắc 2**: Lệnh gọi API bên thứ ba được thực thi NGOÀI database transaction, có context timeout nghiêm ngặt (tối đa 5s/request).

### AC-3 — Tự chữa đơn thành công -> `PAID` (Self-Heal to PAID)
- **Điều kiện**: Cổng thanh toán xác nhận giao dịch đã thu tiền thành công (`SUCCEEDED`).
- **Hành vi**:
  - Thực thi trong **CÙNG MỘT DB TRANSACTION** (Quy tắc 1):
    - Đọc và khóa dòng đơn hàng `orders` bằng `FOR UPDATE`.
    - Kiểm tra nếu đơn chưa `PAID`: cập nhật `orders.status = 'PAID'`, `updated_at = now()`.
    - Upsert bản ghi `payments`: `status = 'SUCCEEDED'`, `provider_txn_id`, `raw_payload`.
    - Ghi nhận sự kiện `order.paid` vào bảng `outbox_events` với `trace_id` hợp lệ.
  - **Idempotency**: Nếu một webhook muộn vừa kịp commit sang `PAID`, đối soát ghi nhận `already_done = true` và không gây lỗi trùng lặp.
  - Ghi nhận thời gian trễ vào metric `payment_webhook_lag_seconds`.

### AC-4 — Tự chữa đơn thất bại -> `CANCELLED` & Trả kho vé Idempotent (Self-Heal to CANCELLED & Release Hold)
- **Điều kiện**: Cổng thanh toán xác nhận giao dịch thất bại, khách hủy, hoặc đơn quá 15 phút mà không tìm thấy giao dịch trên cổng (chưa từng thanh toán).
- **Hành vi**:
  - Thực thi trong **CÙNG MỘT DB TRANSACTION** (Quy tắc 1):
    - Đọc và khóa dòng đơn hàng `orders` bằng `FOR UPDATE`.
    - Cập nhật `orders.status = 'CANCELLED'`, `updated_at = now()` (chỉ cập nhật khi đang là `PAYMENT_PENDING`).
    - **Trả kho vé idempotent (BR-O7 & BR-O1)**:
      ```sql
      UPDATE seat_holds
         SET released_at = now()
       WHERE order_id = $1 AND released_at IS NULL AND consumed_at IS NULL
      RETURNING ticket_type_id, bucket, quantity;
      ```
      Với mỗi hold được giải phóng, hoàn trả số lượng vé về `ticket_inventory`:
      ```sql
      UPDATE ticket_inventory
         SET available = available + $quantity
       WHERE ticket_type_id = $ticket_type_id AND bucket = $bucket;
      ```
    - Cập nhật `payments.status = 'FAILED'` (nếu có bản ghi).
    - Ghi sự kiện `hold.released` vào bảng `outbox_events` để đồng bộ kho Redis và thông báo hàng chờ.
  - Đảm bảo ghế chỉ được giải phóng đúng 1 lần duy nhất (`WHERE released_at IS NULL`).

### AC-5 — Xử lý an toàn khi giao dịch vẫn `PENDING` hoặc lỗi mạng (Transient / In-Flight Handling)
- **Điều kiện**: Cổng thanh toán phản hồi giao dịch vẫn đang trong quá trình thanh toán (`PENDING`) và đơn chưa vượt quá ngưỡng hard-timeout, HOẶC gặp lỗi kết nối mạng tạm thời (`ErrGatewayCommunication`).
- **Hành vi**:
  - Không vội hủy đơn hoặc đổi trạng thái.
  - Ghi log cảnh báo cấu trúc JSON.
  - Bỏ qua đơn này để chu kỳ đối soát tiếp theo tái kiểm tra.

### AC-6 — Metric `payment_webhook_lag_seconds` & Observability
- **Điều kiện**: Hệ thống đo lường độ trễ từ lúc đơn vào `PAYMENT_PENDING` tới khi đối soát phát hiện / tự chữa.
- **Hành vi**:
  - Xuất metric Prometheus:
    - `payment_webhook_lag_seconds`: Histogram/Gauge đo số giây trễ của webhook (giúp phát hiện cổng thanh toán bị nghẽn IPN/webhook).
    - `payment_reconciliation_total`: Counter đếm số đơn được đối soát xử lý theo từng nhãn `outcome` (`healed_paid`, `healed_cancelled`, `still_pending`, `skipped`, `error`).
    - `payment_reconciliation_duration_seconds`: Histogram đo thời gian chạy của job đối soát.
  - Metric xuất hiện trên endpoint `/metrics` thông qua `libs/go/otelx`.

### AC-7 — Background Job Worker & API Trigger
- **Hành vi**:
  - Chạy background goroutine định kỳ (ticker mặc định 1 phút) trong `cmd/server/main.go`, hỗ trợ graceful shutdown qua context.
  - Cung cấp HTTP route nội bộ `POST /internal/reconcile` (hoặc cấu hình) cho phép trigger tức thì theo yêu cầu hoặc từ cronjob bên ngoài, trả về báo cáo JSON chi tiết.

---

## 3. Ranh giới (Scope)

### In Scope
- Port `PaymentGateway`: Thêm hàm `QueryPayment`.
- Adapters Gateway: Triển khai `QueryPayment` cho Sandbox, VNPay, MoMo, Stripe.
- Port `PaymentRepository`: Thêm các hàm `GetStalePendingOrders`, `ProcessOrderCancellationTx`.
- Postgres Repository & Memory Repository: Hiện thực đầy đủ các hàm trên tuân thủ transaction an toàn và BR-O7.
- Use Case: `ReconcileStaleOrdersUseCase` trong `internal/app`.
- Metrics: Bộ đo lường Prometheus bao gồm `payment_webhook_lag_seconds`.
- HTTP Adapter: Endpoint trigger nội bộ và expose metrics.
- Background Worker: Tích hợp vào `cmd/server/main.go`.
- Unit tests & Concurrency integration tests cho toàn bộ use case và repository.

### Out of Scope
- Luồng hoàn tiền tự động khi organizer hủy sự kiện (thuộc task tiếp theo **EVF-74 / EV-154**).
- Gửi email/SMS thông báo vé (thuộc task **EVF-75 / EV-155**).
