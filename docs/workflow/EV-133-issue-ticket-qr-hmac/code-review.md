# EV-133 (EVF-38) — Biên Bản Code Review Độc Lập

- Reviewer: AI Senior Auditor & Security Architect
- Ngày review: 2026-10-19
- Trạng thái kiểm tra: **PASS (Không có Blocker)**

---

## 1. Kiểm tra 4 Quy tắc Bất biến của EvenFlow

### Quy tắc 1: Tuyệt đối không sửa DB ngoài Transaction
- **Đánh giá**: **PASS 100%**
- **Chi tiết**: Toàn bộ quy trình phát hành vé trong `IssueTicketsTx`:
  - Khóa dòng đơn hàng bằng `SELECT ... FOR UPDATE` để chống race condition.
  - Kiểm tra trạng thái đơn hàng (`PAID` / `ISSUED`).
  - Chèn danh sách vé vào bảng `tickets`.
  - Cập nhật `seat_holds.consumed_at = now()` để chốt giữ chỗ vĩnh viễn (BR-O7).
  - Cập nhật `orders.status = 'ISSUED'`.
  - Ghi nhận sự kiện `ticketing.ticket.issued` vào bảng `outbox_events`.
  - Tất cả được bao bọc trong một `pgx.Tx` duy nhất, rollback tự động nếu bất kỳ bước nào gặp sự cố.

### Quy tắc 2: Không gọi API bên thứ ba (Sync Call) chặn luồng HTTP
- **Đánh giá**: **PASS 100%**
- **Chi tiết**:
  - Quá trình tạo mã QR và ký HMAC-SHA256 được thực hiện 100% bằng CPU cục bộ thông qua thư viện chuẩn Go `crypto/hmac` và `crypto/sha256`, không phụ thuộc vào bất kỳ mạng hay dịch vụ bên ngoài nào.
  - Việc gửi email/SMS có đính kèm vé được chuyển giao cho Notification Service (EVF-75) thông qua hàng đợi RabbitMQ (bằng Transactional Outbox), API trả về ngay sau khi commit DB.

### Quy tắc 3: Chống Oversell & Bảo toàn kho vé
- **Đánh giá**: **PASS 100%**
- **Chi tiết**:
  - Không can thiệp hay làm giảm ràng buộc của Redis Lua gate hoặc Postgres `CHECK (available >= 0)`.
  - Khi thanh toán thành công và phát vé, `seat_holds` được đánh dấu `consumed_at`. Hold sweeper chỉ quét những hold có `released_at IS NULL AND consumed_at IS NULL`, do đó không xảy ra hiện tượng hoàn trả nhầm số vé đã bán.

### Quy tắc 4: Structured JSON Logging
- **Đánh giá**: **PASS 100%**
- **Chi tiết**:
  - Mọi bản ghi log qua `slog` đều mang đầy đủ ngữ cảnh `trace_id`, `order_id`, `ticket_count`, `jti`.
  - Tuyệt đối không log HMAC Secret hay thông tin cá nhân khách hàng (PII).

---

## 2. Kiểm tra An ninh Bảo mật (Security Audit)

- **Timing Attack Resistance**: Hàm `authx.QREngine.Verify` sử dụng `subtle.ConstantTimeCompare` để so sánh chữ ký HMAC, ngăn chặn hoàn toàn tấn công kênh phụ đo thời gian (Timing Attacks).
- **Replay Attack Resistance**: Mỗi vé mang một `jti` ngẫu nhiên 128-bit duy nhất (`UNIQUE` constraint trong Postgres). Tại cổng soát vé, trạng thái vé được kiểm tra; nếu vé đã ở trạng thái `CHECKED_IN`, hệ thống từ chối lập tức với lỗi `ErrTicketAlreadyUsed`.
- **Tampering Prevention**: Bất kỳ sự thay đổi nào đối với thông tin vé (đổi `ticket_id`, đổi hạng vé `ticket_type_id`, sửa `exp`) đều làm sai lệch chữ ký HMAC và bị từ chối với lỗi `ErrTamperedQR`.

---

## 3. Các phát hiện (Findings)

### BLOCKER
- Không có.

### MAJOR
- Không có.

### MINOR
- Cần lưu ý cấu hình biến môi trường `HMAC_QR_SECRET` trong production phải là chuỗi ngẫu nhiên có độ dài tối thiểu 32 bytes (đã có giá trị mặc định cho local dev).
