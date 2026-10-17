# EV-152 — Biên bản Code Review độc lập (Adversarial Review)

- **Người thực hiện**: AI Senior Auditor / EvenFlow Tech Lead
- **Thời điểm**: 2026-10-17
- **Phạm vi thẩm định**: Toàn bộ Git Diff của Task EV-152 (EVF-72) — Payment Inbox & Replay Mechanism
- **Trạng thái**: **PASS (Đủ điều kiện merge vào develop)**

---

## 1. Rà soát 4 Quy tắc Bất biến của EvenFlow

### Quy tắc 1: Tuyệt đối không sửa DB ngoài Transaction
- [x] **ĐẠT**.
- Mọi thao tác đổi trạng thái đơn hàng `orders` sang `PAID`, ghi nhận bản ghi `payments` -> `SUCCEEDED` và phát sinh sự kiện vào `outbox_events` (`order.paid`) trong tiến trình replay đều được thực thi thông qua `ProcessPaymentSuccessTx` bên trong một Transaction an toàn (`tx, err := r.pool.Begin(ctx)`).
- Đơn hàng được khóa chống race condition bằng `SELECT ... FOR UPDATE`.
- Việc lưu và cập nhật `payment_inbox` sử dụng ràng buộc duy nhất `UNIQUE (provider, provider_txn_id)` kết hợp `ON CONFLICT DO UPDATE` đảm bảo tính toàn vẹn dữ liệu và an toàn trước các luồng ghi đồng thời.

### Quy tắc 2: Không gọi API bên thứ ba (Sync Call) chặn luồng HTTP
- [x] **ĐẠT**.
- Endpoint webhook `/v1/payments/webhook/{provider}` xử lý xác minh chữ ký cục bộ (HMAC-SHA512 / RSA), kiểm tra replay window và ghi nhận DB/inbox với thời gian phản hồi microsecond, tuyệt đối không gọi đồng bộ ra ngoài.
- Tiến trình replay chạy hoàn toàn bất đồng bộ trong background goroutine (`time.NewTicker(30 * time.Second)`) hoặc qua admin endpoint nội bộ, có context timeout và shutdown êm ái (`graceful shutdown`).
- Việc phát vé QR và gửi email xác nhận được đẩy bất đồng bộ qua `outbox_events` để outbox relay chuyển sang RabbitMQ.

### Quy tắc 3: Không bao giờ phá vỡ oversell protection & concurrency integrity
- [x] **ĐẠT**.
- Khi một webhook đến sớm được replay, hàm `ProcessPaymentSuccessTx` kiểm tra:
  - Nếu `(provider, provider_txn_id)` đã SUCCEEDED -> trả về `AlreadyDone: true`.
  - Nếu đơn hàng đã `PAID` từ trước (do luồng khác hoặc webhook khác) -> trả về `AlreadyDone: true`.
  - Không bao giờ phát thêm outbox event thứ hai cho cùng một đơn hàng, không kích hoạt trừ trùng kho vé hay làm sai lệch số ghế (BR-O6).
- Đã được chứng minh bằng kiểm thử đồng thời 20 workers replay song song (`TestReplayPaymentInbox_ConcurrentReplay_Safe`) và chạy kiểm tra xung đột dữ liệu Go Race Detector (`go test -race`) đạt 0 data race.

### Quy tắc 4: Structured JSON Logging & Không rò rỉ PII
- [x] **ĐẠT**.
- Toàn bộ các tầng (use case, handler, worker) sử dụng `log/slog` dạng JSON có cấu trúc.
- Mọi log ghi nhận đều gắn kèm các trường ngữ cảnh chuẩn: `provider`, `provider_txn_id`, `order_id`, `inbox_id`, `trace_id`, `age_seconds`, `attempts`.
- Tuyệt đối không log thông tin nhạy cảm của khách hàng (PII) hay khóa bí mật của cổng thanh toán.

---

## 2. Các phát hiện và Đánh giá Chi tiết (Findings)

### BLOCKER
- **Không có**.

### MAJOR
- **Không có**.

### MINOR & OPTIMIZATION
- **Cơ chế giới hạn Max Attempts**: Bản ghi không khớp đơn (ví dụ do đơn hàng hủy hoặc dữ liệu rác) được giới hạn tối đa `maxAttempts = 5`. Mỗi lần thử thất bại tăng `attempts + 1`. Khi đạt ngưỡng sẽ dừng retry tự động, tránh chiếm dụng tài nguyên hệ thống.
- **Xử lý đa dạng định dạng `amount_cents`**: Trong payload JSON của webhook, số tiền có thể được truyền dưới dạng `int64`, `float64`, `int` hoặc `string`. Use case đã được thiết kế linh hoạt giải nén an toàn tránh panic.
- **Graceful Shutdown**: Background worker trong `main.go` được quản lý bằng `workerCtx` và `workerCancel()`, đóng kết nối ticker và tác vụ đang xử lý êm ái khi nhận tín hiệu SIGINT/SIGTERM.

---

## 3. Kết luận của Auditor
Mã nguồn của Task EV-152 tuân thủ chặt chẽ kiến trúc Hexagonal Go, đáp ứng đầy đủ tất cả các Acceptance Criteria (AC-1 đến AC-6), đạt coverage > 76% ở các tầng cốt lõi và 0 warning/error. Đủ tiêu chuẩn để đóng gói commit và mở PR vào nhánh `develop`.
