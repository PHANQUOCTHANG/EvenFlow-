# EV-133 (EVF-38) — Phát Hành Vé + QR Ký HMAC Có JTI (Spec)

| Thuộc tính | Chi tiết |
|---|---|
| Jira Ticket | **EV-133** (Mã backlog: **EVF-38**) |
| Epic / Module | E6 – Thanh toán và phát vé (`EV-149`) / Ticketing Service |
| Story Points | **5 SP** — Priority: **High / P1** |
| Tier | **Critical / Security & Data Integrity** (Chống làm giả vé, chống vào cửa lậu, đảm bảo mỗi đơn phát đúng vé) |
| Base Commit | `c555d4eb63f23cac8b978abb1aa561f8c4300068` |
| Branch | `feature/EV-133-issue-ticket-qr-hmac` |
| Nghiệp vụ liên quan | **BR-O8** (Vé phát hành gắn QR ký HMAC kèm `jti` chống sao chép; check-in đổi `ISSUED -> CHECKED_IN` đúng một lần), **BR-O7** (Consume seat holds khi hoàn tất), **4 Quy tắc Bất biến** |

---

## 1. Bối cảnh & Hiện trạng

### Bối cảnh nghiệp vụ
Trong hệ thống EvenFlow, sau khi khách hàng hoàn tất thanh toán (đơn hàng chuyển sang trạng thái `PAID` thông qua webhook thanh toán hoặc job đối soát tự chữa):
- Hệ thống cần phát hành vé điện tử cho khách. Mỗi vé tương ứng với 1 chỗ ngồi/suất vào cửa theo các hạng vé (`ticket_types`) và số lượng ghi trong `order_items`.
- Vé vào cửa cần có mã QR động/tĩnh được ký số bằng thuật toán mã hóa đối xứng HMAC-SHA256 với khóa bí mật do ban tổ chức/hệ thống quản lý.
- Mã QR chứa định danh duy nhất `jti` (JWT ID / unique token ID) và thông tin vé để nhân viên soát vé (scanner) tại cổng sự kiện có thể xác thực tính hợp lệ trực tuyến ngay cả trong điều kiện mạng cao điểm:
  - Nếu mã QR bị kẻ gian chỉnh sửa thông tin (thay đổi `ticket_id`, đổi hạng vé VIP sang thường, sửa hạn dùng) -> chữ ký HMAC sai -> từ chối lập tức.
  - Nếu mã QR bị chụp ảnh màn hình chia sẻ cho người khác cùng quét vào cổng -> người đầu tiên quét vé sẽ hợp lệ, người thứ 2 quét sẽ bị từ chối do `jti` đã được sử dụng (Replay Attack).

### Hiện trạng mã nguồn trước EV-133
1. Bảng `tickets` đã được tạo sẵn trong `migrations/0001_core.sql`:
   ```sql
   CREATE TABLE tickets (
       id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
       order_id       UUID NOT NULL REFERENCES orders(id),
       ticket_type_id UUID NOT NULL REFERENCES ticket_types(id),
       jti            TEXT NOT NULL UNIQUE,        -- BR-O8: chong sao chep QR
       qr_sig         TEXT NOT NULL,
       status         TEXT NOT NULL DEFAULT 'ISSUED'
                      CHECK (status IN ('ISSUED','CHECKED_IN','VOID','REFUNDED')),
       checked_in_at  TIMESTAMPTZ,
       created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
   );
   ```
2. Thư mục `libs/go/authx` chưa được khởi tạo.
3. Trong `services/ticketing`:
   - Chưa có entity `Ticket`, use case phát hành vé (`IssueTicketsUseCase`), use case xác thực mã QR (`VerifyTicketUseCase`).
   - Chưa có adapter repository thao tác với bảng `tickets`.
   - Chưa có HTTP API endpoint để phát hành / lấy vé của đơn hàng / xác minh QR vé.
   - Chưa có consumer hoặc outbox event phát ra thông báo vé đã phát hành (`ticketing.ticket.issued`) để dịch vụ thông báo (Notification Service EVF-75) gửi email/SMS cho khách.

---

## 2. Tiêu chí nghiệm thu (Acceptance Criteria - AC)

### AC-1 — Phát hành đúng số vé theo từng `order_item` của đơn hàng PAID
- **Điều kiện**: Đơn hàng có `status = 'PAID'` (hoặc `ISSUED` khi kiểm tra lại).
- **Hành vi**:
  - Đọc toàn bộ danh sách `order_items` thuộc về `order_id`.
  - Với mỗi item có `quantity = N`, sinh chính xác $N$ bản ghi vé tương ứng với `ticket_type_id` đó.
  - Tổng số vé sinh ra bằng đúng $\sum quantity$ của đơn hàng.
  - Đơn chưa thanh toán (`status != 'PAID'`) không được phép phát vé và trả về lỗi `ErrOrderNotPaid`.

### AC-2 — Sinh QR code ký HMAC-SHA256 kèm `jti` duy nhất (BR-O8)
- **Điều kiện**: Mỗi vé được tạo ra trong hệ thống.
- **Hành vi**:
  - Sinh mã `jti` duy nhất ngẫu nhiên (UUID v4 hoặc crypto-secure random hex).
  - Đóng gói payload vé: `ticket_id`, `order_id`, `event_id`, `ticket_type_id`, `jti`, `iat`, `exp`.
  - Ký số payload bằng HMAC-SHA256 sử dụng khóa bí mật cấu hình hệ thống (`HMAC_SECRET`).
  - Lưu chữ ký `qr_sig` và `jti` vào bảng `tickets`.
  - Cung cấp chuỗi định dạng QR tiêu chuẩn (payload có cấu trúc kèm chữ ký phân tách, e.g. `<base64-payload>.<hmac-signature>`) để in ấn hoặc hiển thị lên ứng dụng khách.

### AC-3 — Đảm bảo tính lũy kế & Không cấp trùng vé khi nhận lệnh lặp (Idempotent Issuance)
- **Điều kiện**: Gọi phát hành vé nhiều lần cho cùng một đơn hàng `order_id` (ví dụ do consumer retry hoặc webhook lặp).
- **Hành vi**:
  - Khóa dòng đơn hàng `orders` bằng `FOR UPDATE` trong giao dịch database.
  - Nếu đơn hàng đã có vé trong bảng `tickets` (hoặc đơn đã ở trạng thái `ISSUED`):
    - Không sinh thêm vé mới.
    - Trả về danh sách vé đã phát hành trước đó với cờ `already_issued = true`.
    - Đảm bảo `UNIQUE(jti)` và tính toàn vẹn 1-1 giữa vé và đơn hàng.

### AC-4 — Từ chối vé có mã QR giả mạo (Tampered QR Rejection)
- **Điều kiện**: Quét mã QR có payload bị sửa đổi dữ liệu (sai `ticket_id`, sai `event_id`, sửa `exp`) hoặc chữ ký HMAC không khớp với khóa bí mật.
- **Hành vi**:
  - Thuật toán xác minh chữ ký phát hiện chữ ký không hợp lệ bằng so sánh thời gian không đổi (`subtle.ConstantTimeCompare`).
  - Trả về kết quả từ chối ngay lập tức với lỗi `ErrInvalidSignature` / `ErrTamperedQR`.
  - Ghi log cảnh báo bảo mật (`WARN`) có kèm IP/trace_id (không lộ secret).

### AC-5 — Từ chối mã QR dùng lại lần 2 (Replay Attack Rejection)
- **Điều kiện**: Quét mã QR có chữ ký hợp lệ nhưng trạng thái vé trong cơ sở dữ liệu đã là `CHECKED_IN`, `VOID` hoặc `REFUNDED`.
- **Hành vi**:
  - Tra cứu theo `jti` và `ticket_id`.
  - Phát hiện vé đã được check-in lúc `checked_in_at` trước đó.
  - Từ chối vào cửa với lỗi `ErrTicketAlreadyUsed`.
  - Nếu vé ở trạng thái `VOID` hoặc `REFUNDED` -> từ chối với lỗi tương ứng.

### AC-6 — Tuân thủ 4 Quy tắc Bất biến của EvenFlow
1. **Transaction an toàn**:
   - Quá trình đọc đơn hàng, kiểm tra trạng thái, lưu các bản ghi `tickets`, cập nhật `seat_holds.consumed_at = now()`, cập nhật `orders.status = 'ISSUED'` và ghi nhận sự kiện `ticketing.ticket.issued` vào `outbox_events` phải nằm trong **CÙNG MỘT TRANSACTION DATABASE** (Quy tắc 1).
2. **Không gọi API bên ngoài chặn luồng**:
   - Việc sinh mã QR và ký HMAC hoàn toàn chạy local trong CPU qua thư viện chuẩn `crypto/hmac`, không gọi HTTP bên thứ ba (Quy tắc 2).
   - Sự kiện gửi email/SMS có vé được đẩy qua RabbitMQ via Outbox Relay (Quy tắc 2).
3. **Chống Oversell & Bảo toàn kho**:
   - Chuyển `seat_holds` sang `consumed_at` an toàn, không hoàn trả kho thừa, không để hold sweeper thu hồi nhầm (Quy tắc 3).
4. **Structured JSON Logging**:
   - Ghi log đầy đủ `trace_id`, `order_id`, `event_id`, số lượng vé đã phát hành. Tuyệt đối không log HMAC Secret hay thông tin cá nhân khách hàng (Quy tắc 4).

---

## 3. Ranh giới (Scope In / Scope Out)

### In Scope
- Thư viện `libs/go/authx`: Module ký và xác thực QR HMAC-SHA256 độc lập, unit test đầy đủ.
- Domain `services/ticketing/internal/domain`:
  - Entity `Ticket`, `TicketWithQR`, `TicketStatus`.
  - Các lỗi nghiệp vụ: `ErrInvalidTicketStatus`, `ErrTamperedQR`, `ErrTicketAlreadyUsed`, `ErrTicketExpired`, `ErrOrderNotPaid`.
- Port `services/ticketing/internal/port`:
  - Interface `TicketRepository` (lấy đơn, chốt cấp vé trong transaction, tra cứu theo JTI).
  - Interface `QRSigner` (ký & xác minh token QR).
- App `services/ticketing/internal/app`:
  - `IssueTicketsUseCase`: Điều phối nghiệp vụ phát hành vé, idempotency, consume hold, ghi outbox.
  - `VerifyTicketUseCase`: Điều phối xác thực chữ ký và kiểm tra trạng thái vé.
- Adapter `services/ticketing/internal/adapter`:
  - Postgres Repository: Thao tác bảng `tickets`, `orders`, `seat_holds`, `outbox_events`.
  - HTTP Handlers:
    - `POST /v1/orders/{order_id}/issue-tickets`
    - `GET /v1/orders/{order_id}/tickets`
    - `POST /v1/tickets/verify`
- Unit test & Integration test cho toàn bộ các thành phần trên.

### Out of Scope
- Phần cứng máy quét mã vạch (Barcode Scanner Hardware) hoặc ứng dụng camera di động của nhân viên soát vé (việc của app mobile).
- Gửi email SMTP / SMS thực tế (thuộc dịch vụ `notification-svc` - Jira EVF-75, chỉ cần outbox event `ticketing.ticket.issued`).
