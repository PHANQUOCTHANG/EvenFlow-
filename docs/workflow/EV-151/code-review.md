# EV-151 — Biên bản Code Review độc lập (Adversarial Review)

- **Reviewer**: AI Senior Auditor & Security Architect
- **Task**: EV-151 (Mã backlog EVF-71) — Webhook exactly-once: UNIQUE provider_txn_id, xác minh chữ ký, chống replay
- **Trạng thái kiểm tra**: **APPROVED (Đạt toàn bộ tiêu chuẩn & 4 Quy tắc Bất biến)**

---

## 1. Kiểm tra 4 Quy tắc Bất biến của EvenFlow

- [x] **Quy tắc 1: Tuyệt đối không sửa DB ngoài Transaction**:
  - `PostgresRepository.ProcessPaymentSuccessTx`: Kiểm tra trạng thái giao dịch cũ, khóa dòng đơn hàng (`SELECT ... FOR UPDATE`), cập nhật `orders` sang `PAID`, upsert bản ghi `payments` sang `SUCCEEDED`, và ghi nhận sự kiện `order.paid` vào bảng `outbox_events` đều nằm trọn vẹn trong CÙNG MỘT `pgx.Tx` với `defer tx.Rollback(ctx)` và `tx.Commit(ctx)`.
  - `PostgresRepository.ProcessPaymentFailedTx`: Cập nhật trạng thái `FAILED` cho payments cũng nằm trong transaction riêng biệt an toàn.
- [x] **Quy tắc 2: Không gọi API bên thứ ba (Sync Call) chặn luồng HTTP**:
  - Endpoint webhook nhận callback từ các cổng thanh toán (VNPay, MoMo, Stripe, Sandbox) và thực hiện xác minh chữ ký hoàn toàn bằng mật mã học cục bộ (HMAC-SHA512, HMAC-SHA256, hash check) mà không gọi sync HTTP ra ngoài.
  - Phản hồi HTTP 200 OK ngay lập tức cho đối tác thanh toán sau khi commit transaction.
- [x] **Quy tắc 3: Không phá vỡ concurrency / oversell protection (Exactly-Once BR-O6)**:
  - Khóa cơ sở dữ liệu `UNIQUE (provider, provider_txn_id)` bảo đảm tính bất biến của mã giao dịch cổng.
  - Kiểm thử Concurrency với 100 luồng đồng thời gọi cùng một webhook (`TestProcessWebhook_ExactlyOnce_Concurrent100`):
    - Đúng 1 luồng chuyển trạng thái đơn hàng sang `PAID`.
    - Đúng 1 sự kiện `order.paid` được tạo trong bảng `outbox_events`.
    - 99 luồng còn lại nhận kết quả `AlreadyDone: true` và trả về `200 OK` (idempotent no-op).
  - Không phá vỡ bất kỳ quy tắc giữ vé hay quota tồn kho nào.
- [x] **Quy tắc 4: Structured JSON Logging & Bảo mật PII**:
  - Toàn bộ log trong `ProcessWebhookUseCase` và `Handler` đều có cấu trúc (`slog`), đi kèm `trace_id`, `provider`, `order_id`, `provider_txn_id`.
  - Tuyệt đối không log Secret Keys, Access Keys, Hash Secrets hay thông tin nhạy cảm PII của người mua.

---

## 2. Rà soát Lỗ hổng Kỹ thuật (Vulnerability & Security Audit)

### A. Tấn công Replay (Replay Attack)
- **Rủi ro**: Kẻ tấn công thu thập gói tin webhook hợp lệ trong quá khứ và gửi lại nhiều lần để kích hoạt đơn hoặc thay đổi trạng thái.
- **Biện pháp xử lý**:
  - Use case kiểm tra timestamp từ webhook payload đối chiếu với `Clock.Now()`.
  - Cửa sổ thời gian quy định chặt chẽ: không quá **5 phút (300 giây)** trong quá khứ và không lệch quá **1 phút (60 giây)** trong tương lai (bù trôi đồng hồ / clock skew).
  - Webhook vượt quá cửa sổ thời gian bị từ chối ngay lập tức với mã lỗi `400 Bad Request` (`ErrReplayDetected`).

### B. Giả mạo chữ ký (Signature Spoofing)
- **Rủi ro**: Kẻ tấn công tự gửi request webhook giả mạo với kết quả thành công nhằm nhận vé mà không trả tiền.
- **Biện pháp xử lý**:
  - Tất cả các adapter (`vnpay`, `momo`, `stripe`, `sandbox`) bắt buộc phải thẩm định chữ ký số cryptographic bằng secret key đã cấu hình an toàn trên máy chủ.
  - Request có chữ ký rỗng, sai định dạng, hoặc không khớp giá trị băm đều bị từ chối với mã lỗi `400 Bad Request` (`ErrInvalidSignature`).

### C. Đua dữ liệu (Race Condition) khi Webhook đến cùng lúc
- **Rủi ro**: Cổng thanh toán gửi đồng thời 2 webhook retry do timeout mạng.
- **Biện pháp xử lý**:
  - Kết hợp row lock `SELECT ... FOR UPDATE` trên bảng `orders` và ràng buộc duy nhất `UNIQUE (provider, provider_txn_id)` trên bảng `payments`.
  - Bất kỳ transaction đến sau đều phát hiện trạng thái đã hoàn tất hoặc đơn đã `PAID`, trả về `AlreadyDone = true` và HTTP 200 OK an toàn.

---

## 3. Các phát hiện (Findings)

### BLOCKER
- **Không có**: Toàn bộ 4 Quy tắc Bất biến và các Acceptance Criteria cốt lõi của Jira đều được tuân thủ nghiêm ngặt.

### MAJOR / MINOR
- **Minor (Đã xử lý)**:
  - Chuẩn hóa canonical `gofmt` cho toàn bộ các file Go trong `services/payment`.
  - Định dạng chuẩn hỗ trợ cả POST JSON, POST Form và GET query parameters (phù hợp với đặc thù IPN/redirect của VNPay).

---

## 4. Kết luận

Mã nguồn của task **EV-151 (Backlog EVF-71)** đã hoàn thành xuất sắc và sẵn sàng 100% để merge vào `develop`.
- Unit & Concurrency test: 40/40 test cases PASS.
- Static analysis: `gofmt` sạch 100%, `go vet` PASS 0 lỗi.
- Binary build: Server biên dịch thành công.
