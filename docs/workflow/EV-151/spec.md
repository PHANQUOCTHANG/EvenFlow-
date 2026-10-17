# EV-151 — Webhook exactly-once: UNIQUE provider_txn_id, xác minh chữ ký, chống replay (Spec)

| Thuộc tính | Chi tiết |
|---|---|
| Jira | **EV-151** (mã backlog **EVF-71**) — 8 SP — Priority: **P0 (Highest)** — Sprint 3 |
| Tier | **Critical / Financial Reliability** (Tầng xử lý giao dịch tài chính exactly-once) |
| Base Commit | `c241ea152fc1a728883f1155b9c0f0afc837a1a6` |
| Branch | `feature/EV-151-webhook-exactly-once` |
| Nghiệp vụ liên quan | **BR-O6** (UNIQUE index trên `provider_txn_id`, webhook lặp lại là no-op trả 200; webhook đến sớm lưu inbox replay sau), **BR-O2** (bảo vệ hạn giữ vé không kích hoạt đơn quá hạn), **BR-O5** (idempotency an toàn) |
| Phụ thuộc | **EVF-70 / EV-150** (Adapter cổng thanh toán VNPay/MoMo/Stripe + sandbox mock) ✅, **EVF-36** (Transactional outbox relay) ✅ |
| Khách hàng của task | **EVF-72 / EV-152** (Payment inbox cho webhook đến sớm), **EVF-114 / EV-184** (Frontend Payment Flow & Order Result polling PENDING -> PAID) |

---

## 1. Bối cảnh & Hiện trạng

- Ở task trước (**EV-150 / EVF-70**), hệ thống đã xây dựng khung Adapter thanh toán cho 4 cổng: VNPay, MoMo, Stripe và Sandbox Mock, đồng thời chuyển trạng thái đơn hàng từ `HELD` sang `PAYMENT_PENDING`.
- Cổng thanh toán bên thứ ba (VNPay, MoMo, Stripe) trả về kết quả thanh toán thực tế thông qua các cuộc gọi **Webhook / IPN (Instant Payment Notification)** bất đồng bộ trên đường truyền Internet công cộng.
- Thách thức kỹ thuật:
  1. **Mạng không tin cậy (Network Retries)**: Cổng thanh toán gửi lại webhook nhiều lần (retry 5 - 10 lần) khi mạng chập chờn. Nếu không xử lý đúng, hệ thống có thể kích hoạt đơn nhiều lần, phát vé trùng, hoặc gây lỗi trạng thái dữ liệu.
  2. **Tấn công Replay & Giả mạo (Spoofing & Replay Attacks)**: Hacker có thể bắt gói tin webhook hợp lệ trong quá khứ và gửi lại nhiều lần để chuộc lợi, hoặc tự tạo webhook giả mạo không có chữ ký số hợp lệ của đối tác.
  3. **Đua dữ liệu đồng thời (Concurrency Race Conditions)**: Cổng thanh toán có thể bắn 2 hoặc nhiều request webhook cho cùng một giao dịch song song nhau trong vài mili-giây.
- Mục tiêu của task **EV-151 / EVF-71**:
  - Xây dựng luồng tiếp nhận Webhook chuẩn hóa cho tất cả các cổng (`sandbox`, `vnpay`, `momo`, `stripe`).
  - Kiểm tra chữ ký cryptographic (`HMAC-SHA512`, `HMAC-SHA256`, `Stripe signature`) của từng đối tác.
  - Áp dụng cửa sổ thời gian chống tấn công Replay (Timestamp Replay Protection Window: ±5 phút).
  - Đảm bảo tính **Exactly-Once** thông qua ràng buộc cơ sở dữ liệu `UNIQUE (provider, provider_txn_id)`.
  - Thực thi cập nhật trạng thái `payments`, `orders` (sang `PAID`), và tạo sự kiện outbox `order.paid` trong **CÙNG MỘT TRANSACTION** (Quy tắc bất biến 1 & 2).

---

## 2. Tiêu chí nghiệm thu (Acceptance Criteria)

- **AC-1 — Webhook Exactly-Once & Chống Duplicate (BR-O6, Tiêu chí cốt lõi của Jira)**:
  - Khi nhận webhook lần đầu tiên với giao dịch thành công:
    - Chuyển trạng thái đơn hàng sang `PAID`.
    - Ghi nhận `payments` với trạng thái `SUCCEEDED` và lưu `provider_txn_id`.
    - Ghi sự kiện `order.paid` vào bảng `outbox_events`.
    - Trả về mã phản hồi `200 OK`.
  - **Bắn lại cùng webhook 100 lần (tuần tự hoặc đồng thời 100 luồng)**:
    - Đúng duy nhất 1 đơn hàng chuyển `PAID`.
    - Đúng duy nhất 1 sự kiện `order.paid` được sinh trong `outbox_events`.
    - Toàn bộ 100 request đều nhận được phản hồi thành công `200 OK` (idempotent no-op), không sinh ra lỗi `500` hay ngoại lệ crash hệ thống.

- **AC-2 — Xác thực chữ ký đối tác (Cryptographic Signature Verification)**:
  - Mọi webhook đến đều phải được xác thực chữ ký số bằng Secret Key cấu hình trước:
    - **VNPay**: Kiểm tra `vnp_SecureHash` bằng thuật toán HMAC-SHA512 qua danh sách tham số chuẩn đã sort alphabet (loại trừ hash params).
    - **MoMo**: Kiểm tra `signature` bằng thuật toán HMAC-SHA256 theo định dạng chữ ký chuẩn của MoMo IPN.
    - **Stripe**: Kiểm tra `Stripe-Signature` (hoặc signature params) theo tiêu chuẩn webhook Stripe.
    - **Sandbox**: Kiểm tra `provider_txn_id` và token chữ ký mock (nếu có).
  - Nếu chữ ký rỗng, giả mạo, hoặc không khớp:
    - Từ chối ngay lập tức với mã lỗi `400 Bad Request` kèm thông báo `"chữ ký không hợp lệ"`.
    - Không cập nhật DB, ghi log cảnh báo an ninh.

- **AC-3 — Cửa sổ thời gian chống Replay Attack (Replay Protection Window)**:
  - Trích xuất timestamp từ webhook payload/query hoặc header:
    - MoMo: `responseTime` (epoch millisecond).
    - VNPay: `vnp_PayDate` (`yyyyMMddHHmmss`) hoặc `timestamp`.
    - Stripe: `created` (epoch second) hoặc header timestamp.
    - Sandbox: `timestamp` (RFC3339 hoặc epoch).
  - Kiểm tra độ lệch thời gian so với `Clock.Now()`:
    - Cửa sổ hợp lệ: không quá **5 phút (300 giây)** trong quá khứ và không lệch quá **1 phút (60 giây)** trong tương lai (bù trôi đồng hồ / clock skew).
    - Nếu timestamp vượt quá cửa sổ thời gian: Từ chối với mã lỗi `400 Bad Request` (`ErrReplayDetected`), không xử lý đơn.

- **AC-4 — Tính nguyên tử trong DB Transaction & Transactional Outbox (Quy tắc bất biến 1)**:
  - Tất cả các thao tác thay đổi trạng thái phải nằm trong CÙNG MỘT Postgres Transaction:
    1. Ghi nhận/cập nhật bản ghi trong bảng `payments` (`status = 'SUCCEEDED'` hoặc `'FAILED'`).
    2. Cập nhật `orders`: `status = 'PAID'`, `updated_at = now()` với điều kiện `orders.status IN ('PAYMENT_PENDING', 'HELD')`.
    3. Ghi sự kiện vào bảng `outbox_events`:
       - `aggregate_type`: `'order'`
       - `aggregate_id`: `order_id` (UUID)
       - `routing_key`: `'order.paid'`
       - `payload`: JSON đầy đủ `order_id`, `event_id`, `identity_id`, `payment_id`, `provider`, `provider_txn_id`, `amount_cents`, `currency`, `paid_at`.
       - `trace_id`: Trace ID từ context.
  - Nếu bất kỳ bước nào lỗi hoặc vi phạm ràng buộc: Transaction tự động rollback toàn bộ, không để lại dữ liệu rác.

- **AC-5 — Xử lý kết quả giao dịch Thất bại (Payment Failed)**:
  - Khi webhook thông báo giao dịch thất bại từ ngân hàng/ví (ví dụ MoMo `resultCode != 0`, VNPay `vnp_ResponseCode != "00"`):
    - Cập nhật `payments.status = 'FAILED'`.
    - Không cập nhật đơn hàng sang `PAID`, không phát sự kiện `order.paid`.
    - Ghi nhận `outbox_events` với routing key `order.payment_failed` (nếu cần) và trả về `200 OK` cho cổng thanh toán xác nhận đã nhận tin.

- **AC-6 — Endpoint REST Webhook chuyên biệt**:
  - Cung cấp route tiếp nhận Webhook:
    - `POST /v1/payments/webhook/{provider}` (hỗ trợ JSON body cho MoMo, Stripe, Sandbox).
    - `GET /v1/payments/webhook/{provider}` (hỗ trợ query parameters cho luồng redirect/IPN của VNPay).
  - Tự động chuẩn hóa provider và định tuyến tới đúng Gateway Adapter tương ứng.
  - Trả về mã phản hồi theo chuẩn từng cổng (ví dụ MoMo yêu cầu 204 hoặc 200 JSON `{"message": "success"}`).

- **AC-7 — Structured JSON Logging & Bảo mật PII (Quy tắc bất biến 4)**:
  - Toàn bộ log ghi lại có `trace_id`, `event_id`, `order_id`, `provider`, `provider_txn_id`.
  - Tuyệt đối không log thông tin nhạy cảm (secret keys, access keys, hash secrets, số thẻ).

---

## 3. Ranh giới (Scope)

- **In Scope**:
  - Mở rộng tầng `domain`: Định nghĩa các kiểu dữ liệu, lỗi nghiệp vụ (`ErrInvalidSignature`, `ErrReplayDetected`, `ErrDuplicateWebhook`,...) và entity kết quả webhook.
  - Mở rộng tầng `port`: Bổ sung interface xử lý webhook transaction `ProcessPaymentSuccessTx`, `ProcessPaymentFailedTx` trong `PaymentRepository`. Bổ sung kiểm tra timestamp chống replay.
  - Tầng `app`: Hiện thực Use Case `ProcessWebhookUseCase` điều phối xác thực chữ ký, kiểm tra replay window, kiểm tra duplicate, và kích hoạt DB transaction.
  - Tầng `adapter/gateway`: Bổ sung hàm kiểm tra chữ ký và trích xuất timestamp chống replay cho cả 4 cổng (`Sandbox`, `VNPay`, `MoMo`, `Stripe`).
  - Tầng `adapter/postgres`: Hiện thực hàm `ProcessPaymentSuccessTx` và `ProcessPaymentFailedTx` trên `PostgresRepository` và `MemoryRepository`.
  - Tầng `adapter/http`: Bổ sung handler `HandleWebhook` (`POST` và `GET` `/v1/payments/webhook/{provider}`).
  - Tầng `cmd/server/main.go`: Khởi tạo và liên kết các route webhook mới.
  - Bộ kiểm thử toàn diện: Unit tests cho domain, use cases, gateway adapters, HTTP handler, và kiểm thử Concurrency (100 request đồng thời kiểm chứng exactly-once).

- **Out of Scope**:
  - Cơ chế `payment_inbox` cho webhook đến trước khi đơn hàng tồn tại (thuộc task tiếp theo **EVF-72 / EV-152**).
  - Worker tiến trình quét outbox_events đẩy ra RabbitMQ (đã có ở **EVF-36**).
  - Job đối soát đơn quá 15 phút không nhận được webhook (thuộc task **EVF-73 / EV-153**).
