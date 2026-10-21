# EV-135 — Check-in vé đúng một lần (Spec)

| Mục | Giá trị |
|---|---|
| Jira | EV-135 (API check-in ISSUED → CHECKED_IN đúng một lần) |
| Kế hoạch | [12-ke-hoach-demo-backend.md](../../12-ke-hoach-demo-backend.md) — việc tùy chọn G11 |
| Branch | `feature/EV-135-ticket-check-in`, chồng trên `feature/EV-198-demo-script` |
| Quy tắc | BR-O8: mỗi vé chỉ có giá trị vào cửa một lần |

## 1. Hiện trạng

`POST /v1/tickets/verify` chỉ kiểm chữ ký và trạng thái, không đổi trạng thái: quét bao nhiêu lần cũng "hợp lệ". `Repository.UpdateTicketStatus` (UPDATE có
điều kiện) có sẵn nhưng không ai gọi.

## 2. Thiết kế

`POST /v1/tickets/check-in` `{qr_payload}`:
1. Dùng lại `VerifyTicketUseCase` (chữ ký, hạn, JTI, khớp claims/DB, trạng thái). Lỗi kiểm trả nguyên: 400 `tampered-qr`, 410 `ticket-expired`, 404, 409
   `ticket-already-used`, 403 void/refunded. Các lỗi này **không** chạm trạng thái vé.
2. `UPDATE tickets SET status='CHECKED_IN', checked_in_at=$now WHERE id=$1 AND status='ISSUED'`. Tính "đúng một lần" nằm ở điều kiện `WHERE`, không ở bước kiểm
   trước: hai máy quét cùng lúc đều thấy `ISSUED` ở bước 1, nhưng chỉ một UPDATE đổi được dòng; máy còn lại nhận `ErrTicketAlreadyUsed` → 409.
3. Chỉ role `ops`: gateway giới hạn route (`Roles: ops`), ticketing kiểm lại header (lớp hai, phòng gọi thẳng service). `verify` vẫn mở cho mọi người dùng
   đăng nhập vì chỉ đọc.
4. Không cần `Idempotency-Key`: lần gọi lại tự nhiên trả 409 (không đổi gì thêm).

Bảng ánh xạ lỗi của `verify` được tách thành `writeVerifyResult` dùng chung cho cả hai route.

## 3. Tiêu chí nghiệm thu

| AC | Nội dung |
|---|---|
| AC-1 | Lần quét đầu: 200, vé `CHECKED_IN` kèm `checked_in_at`; lần hai: 409 |
| AC-2 | 50 máy quét đồng thời một vé: đúng 1 thành công, 49 nhận `ErrTicketAlreadyUsed`; vé khác không đổi |
| AC-3 | Chỉ ops: buyer/organizer/moderator/không danh tính bị 403 và vé không đổi trạng thái |
| AC-4 | QR giả / sai body: 400, không đổi trạng thái; lỗi kiểm QR không gọi UPDATE |
