# EV-152 (EVF-72) — Payment Inbox Cho Webhook Đến Sớm & Replay (Spec)

| Thuộc tính | Chi tiết |
|---|---|
| Jira Ticket | **EV-152** (Backlog code: **EVF-72**) |
| Epic / Module | Payment Reliability & Webhook Processing |
| Story Points | **5 SP** — Priority: **High / P1** |
| Tier | **Critical / High-Reliability** (Chống mất mát thanh toán, đảm bảo tính toàn vẹn dữ liệu đơn hàng và vé) |
| Base Commit | `cf8b84648faf3e715caa8524c6727d0e8d90451b` |
| Branch | `feature/EV-152-payment-inbox-replay` |
| Nghiệp vụ liên quan | **BR-O6** (webhook đến trước khi đơn tồn tại -> lưu `payment_inbox` và replay sau; UNIQUE `provider_txn_id`, webhook lặp là no-op 200), **BR-O2** (giữ ghế và hạn giữ đơn), **BR-O5** (idempotency) |

---

## 1. Bối cảnh & Hiện trạng

### Bối cảnh kỹ thuật
Trong hệ thống thương mại điện tử và bán vé tải cao EvenFlow, khi khách hàng tiến hành thanh toán qua các cổng thanh toán bên thứ ba (VNPay, MoMo, Stripe, Sandbox), cổng thanh toán sẽ gửi Webhook/IPN về server. Trong các kịch bản mạng phân tán, tải cao hoặc race condition:
- Khách hàng quét mã QR hoặc hoàn tất thanh toán ở phía cổng cực nhanh trước khi tiến trình tạo đơn (Order creation) hoàn tất commit vào database hoặc khi có độ trễ bất đồng bộ.
- Webhook từ cổng thanh toán có thể đến hệ thống trước khi bản ghi `orders` tồn tại trong cơ sở dữ liệu.

### Hiện trạng mã nguồn trước EV-152
- Task EV-150 đã hoàn thành các Gateway Adapters (Sandbox, VNPay, MoMo, Stripe).
- Task EV-151 đã hoàn thành cơ chế Webhook exactly-once với xác thực chữ ký số, chống replay attack trong cửa sổ thời gian 5 phút, và cập nhật atomic DB transaction khi đơn hàng đã tồn tại.
- Tuy nhiên, trong `services/payment/internal/app/process_webhook.go` và `services/payment/internal/adapter/postgres/repository.go`:
  - Khi đơn hàng chưa có trong DB (`orders`), hàm trả về `domain.ErrOrderNotFound`.
  - HTTP Handler chuyển lỗi thành `404 Not Found`.
  - Cổng thanh toán coi 404 là lỗi và sẽ retry dồn dập hoặc bỏ cuộc, gây nguy cơ đơn bị treo hoặc khách đã mất tiền mà hệ thống không ghi nhận được.
- Bảng cơ sở dữ liệu `payment_inbox` đã được định nghĩa sẵn trong `migrations/0001_core.sql`:
  ```sql
  CREATE TABLE payment_inbox (
      id              BIGSERIAL PRIMARY KEY,
      provider        TEXT NOT NULL,
      provider_txn_id TEXT NOT NULL,
      payload         JSONB NOT NULL,
      received_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
      processed_at    TIMESTAMPTZ,
      attempts        INT NOT NULL DEFAULT 0,
      UNIQUE (provider, provider_txn_id)
  );
  ```
- Tuy nhiên chưa có code tầng domain, port, repository hay use case nào ghi nhận vào `payment_inbox` cũng như chưa có cơ chế replay tự động định kỳ.

---

## 2. Tiêu chí nghiệm thu (Acceptance Criteria - AC)

### AC-1 — Webhook đến sớm được lưu vào `payment_inbox` an toàn (BR-O6)
- **Điều kiện**: Webhook gửi đến có chữ ký hợp lệ và timestamp trong cửa sổ cho phép, nhưng `order_id` chưa tồn tại trong cơ sở dữ liệu (`ErrOrderNotFound`).
- **Hành vi**:
  - Không ném lỗi 404 / 500 ra HTTP response.
  - Tự động lưu toàn bộ dữ liệu webhook vào bảng `payment_inbox` với `provider`, `provider_txn_id`, `payload` (JSONB), `received_at = now()`, `processed_at = NULL`, `attempts = 0`.
  - Trả về HTTP 200 OK với body có cấu trúc JSON rõ ràng thông báo webhook đã được tiếp nhận an toàn vào inbox (`stored_in_inbox: true`, `message: "webhook received and stored in payment_inbox for delayed replay"`).
  - Cổng thanh toán nhận mã 200 sẽ không spam retry bão hòa hệ thống.

### AC-2 — Lưu Inbox Idempotent & Chống Trùng Lặp (Idempotent Storage)
- **Điều kiện**: Cùng một webhook đến sớm được bắn nhiều lần liên tiếp khi đơn hàng vẫn chưa xuất hiện.
- **Hành vi**:
  - Tận dụng khóa duy nhất `UNIQUE (provider, provider_txn_id)` của `payment_inbox`.
  - Câu lệnh chèn dùng `ON CONFLICT (provider, provider_txn_id) DO NOTHING` (hoặc cập nhật nhẹ không lỗi).
  - Không sinh exception/lỗi duplicate key, trả về kết quả thành công idempotent.

### AC-3 — Replay tự động thành công khi đơn hàng xuất hiện (BR-O6 Replay)
- **Điều kiện**: Đơn hàng sau đó đã được tạo thành công trong DB (trạng thái `HELD` hoặc `PAYMENT_PENDING`).
- **Hành vi**:
  - Use case `ReplayPaymentInboxUseCase` quét các bản ghi chưa xử lý (`processed_at IS NULL` và `attempts < max_attempts`).
  - Thực thi lại logic xử lý thanh toán thông qua `ProcessPaymentSuccessTx`:
    - Chuyển `orders` -> `PAID`.
    - Ghi bản ghi `payments` -> `SUCCEEDED`.
    - Ghi nhận sự kiện `order.paid` vào `outbox_events`.
    - Tất cả nằm trong CÙNG MỘT TRANSACTION an toàn (Quy tắc bất biến 1).
  - Đánh dấu bản ghi trong `payment_inbox`: `processed_at = now()`.

### AC-4 — Replay Idempotent & Không Sinh Lỗi / Không Oversell
- **Điều kiện**: Bản ghi inbox được kích hoạt replay nhiều lần, hoặc đơn hàng đã chuyển sang `PAID` bởi một kênh khác.
- **Hành vi**:
  - Replay nhận biết đơn đã `PAID` (`AlreadyDone = true`).
  - Cập nhật `processed_at = now()` trong `payment_inbox` để không tiếp tục quét lại.
  - Tuyệt đối không sinh thêm outbox event thừa, không trừ trùng số vé, đảm bảo exactly-once (BR-O6).

### AC-5 — Backoff và Giới hạn số lần thử (Max Attempts Guard)
- **Điều kiện**: Đơn hàng vì một lý do nào đó không bao giờ xuất hiện (ví dụ dữ liệu rác hoặc người dùng hủy đơn ở bước trước).
- **Hành vi**:
  - Mỗi lần job replay quét mà đơn vẫn chưa xuất hiện (`ErrOrderNotFound`), trường `attempts` tăng thêm 1 và ghi nhận `last_attempt_at`.
  - Khi `attempts >= max_attempts` (mặc định cấu hình = 5), job replay dừng việc tiếp tục thử lại định kỳ để tránh tốn tài nguyên CPU / DB.

### AC-6 — Phát hiện và Cảnh báo bản ghi tồn quá lâu (Stale Inbox Alert & Metrics)
- **Điều kiện**: Các bản ghi trong `payment_inbox` có `processed_at IS NULL` và `now() - received_at > stale_threshold` (mặc định 15 phút theo quy định nghiệp vụ đối soát BR-O6).
- **Hành vi**:
  - Use case kiểm tra và phát hiện danh sách các bản ghi tồn quá lâu.
  - Ghi structured log mức `WARN`/`ERROR` chứa đầy đủ: `inbox_id`, `provider`, `provider_txn_id`, `age_seconds`, `attempts`.
  - Cung cấp phương thức báo cáo / hook để tích hợp metric giám sát (ví dụ metric `payment_inbox_stale_records_count` hoặc lag seconds).

---

## 3. Ranh giới (Scope In / Scope Out)

### In Scope
- Thêm Entity & Struct trong Domain: `PaymentInboxItem`, `ReplayInboxResult`, `StaleInboxRecord`.
- Mở rộng Port `PaymentRepository`:
  - `SavePaymentInbox(ctx, item)`
  - `GetPendingPaymentInbox(ctx, limit, maxAttempts)`
  - `MarkPaymentInboxProcessed(ctx, id)`
  - `IncrementPaymentInboxAttempts(ctx, id)`
  - `GetStalePaymentInbox(ctx, olderThan, limit)`
- Cập nhật `ProcessWebhookUseCase`: Khi gặp `ErrOrderNotFound`, tự động lưu vào inbox và trả kết quả thành công với cờ `StoredInInbox: true`.
- Hiện thực Use Case mới: `ReplayPaymentInboxUseCase` thực hiện quét, thử lại, cập nhật trạng thái và cảnh báo bản ghi tồn quá lâu.
- Hiện thực trong `PostgresRepository` (truy vấn SQL chuẩn với Postgres) và `MemoryRepository` (phục vụ unit test và chế độ chạy local offline).
- Cập nhật HTTP Handler: Phản hồi JSON `stored_in_inbox: true` khi webhook lưu vào inbox.
- Bổ sung background worker / ticker runner trong server (hoặc HTTP/job hook) có khả năng định kỳ kích hoạt replay.
- Viết Unit Test và Concurrency Test toàn diện cho toàn bộ các kịch bản trên.

### Out of Scope
- Sửa schema DB (bảng `payment_inbox` đã có sẵn trong `0001_core.sql`).
- Chức năng đối soát chủ động sang cổng thanh toán (Active Reconciliation query - thuộc task tiếp theo **EVF-73 / EV-153**).
- Thay đổi cấu trúc phát vé QR (thuộc **EVF-38**).
