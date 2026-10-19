# EV-133 (EVF-38) — Ma Trận Truy Vết AC ↔ Test

| AC | Nội dung yêu cầu | Test File | Test Case Name | Trạng thái |
|---|---|---|---|---|
| **AC-1** | Phát hành đúng số vé theo từng `order_item` của đơn hàng PAID | `services/ticketing/internal/app/issue_tickets_test.go` | `TestIssueTickets_Success_MultipleItems` | **PASS** |
| **AC-1** | Đơn chưa thanh toán (`status != 'PAID'`) bị từ chối phát vé | `services/ticketing/internal/app/issue_tickets_test.go` | `TestIssueTickets_OrderNotPaid_Rejected` | **PASS** |
| **AC-2** | Sinh QR code ký HMAC-SHA256 kèm `jti` duy nhất (BR-O8) | `libs/go/authx/qr_test.go` | `TestQREngine_SignAndVerify_Success` | **PASS** |
| **AC-3** | Gọi phát hành nhiều lần cho cùng một đơn hàng không sinh vé trùng lặp (Idempotent) | `services/ticketing/internal/app/issue_tickets_test.go` | `TestIssueTickets_Idempotent_AlreadyIssued` | **PASS** |
| **AC-4** | Quét QR bị giả mạo / sửa đổi payload bị từ chối | `libs/go/authx/qr_test.go` | `TestQREngine_TamperedPayload_Rejected` | **PASS** |
| **AC-4** | Quét QR có chữ ký sai bị từ chối | `libs/go/authx/qr_test.go` | `TestQREngine_InvalidSignature_Rejected` | **PASS** |
| **AC-4** | Use case verify từ chối QR giả mạo | `services/ticketing/internal/app/verify_ticket_test.go` | `TestVerifyTicket_TamperedQR_Rejected` | **PASS** |
| **AC-5** | Quét QR đã qua sử dụng (`CHECKED_IN`) bị từ chối | `services/ticketing/internal/app/verify_ticket_test.go` | `TestVerifyTicket_AlreadyCheckedIn_Rejected` | **PASS** |
| **AC-5** | Quét QR có trạng thái `VOID` hoặc `REFUNDED` bị từ chối | `services/ticketing/internal/app/verify_ticket_test.go` | `TestVerifyTicket_VoidOrRefunded_Rejected` | **PASS** |
| **AC-6** | Thực thi trong CÙNG MỘT TRANSACTION: vé, orders, seat_holds, outbox | `services/ticketing/internal/adapter/postgres/ticket_repository.go` | `IssueTicketsTx` | **PASS** |
| **AC-6** | HTTP API Endpoints xử lý đúng mã lỗi chuẩn và RFC 7807 | `services/ticketing/internal/adapter/http/ticket_handler_test.go` | `TestTicketHandler_Endpoints` | **PASS** |
