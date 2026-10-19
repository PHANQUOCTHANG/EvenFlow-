# EV-NEW-2 — API đơn hàng: xem đơn, huỷ hold, xem tồn kho (Spec)

| Mục | Giá trị |
|---|---|
| Jira | **Chưa có key** — EV-NEW-2 trong kế hoạch, 5 SP, Sprint A việc #6. Khi tạo issue, đổi tên nhánh/PR |
| Kế hoạch | [11-ke-hoach-luong-mua-ve.md](../../11-ke-hoach-luong-mua-ve.md) — việc 4/5, chuỗi B (độc lập EV-111/113/137) |
| Branch | `feature/order-api` (từ `develop` @ `1d4b358`) |

## 1. Hiện trạng

- Web (`payment-client.ts`) gọi `GET /v1/events/{id}/orders/{orderId}`; route này **chưa bao giờ tồn tại** ở backend, và gateway
  cũng không định tuyến. Trang kết quả thanh toán luôn rơi về bản cache.
- Khách không tự huỷ được hold; không API nào cho web biết hạng vé còn hay hết.
- **Lỗ hổng phát hiện thêm:** `GET /v1/orders/{id}/tickets` và `POST /v1/orders/{id}/issue-tickets` không kiểm chủ đơn, mà gateway
  cho mọi người dùng đã đăng nhập đi qua `/v1/orders/` → ai cũng đọc được vé (jti, chữ ký) hoặc kích phát hành vé của đơn bất kỳ.

## 2. Thiết kế (ticketing)

| Route | Quyền | Hành vi |
|---|---|---|
| `GET /v1/orders/{orderID}` | Chủ đơn | Đơn + hạng vé + sự kiện + thanh toán gần nhất + vé đã phát (khi `ISSUED`). Id sai / không tồn tại / của người khác → cùng 404. `Cache-Control: private, no-store`. `status` là bộ giá trị web dùng (`PENDING/PAID/FAILED/EXPIRED/CANCELLED`), `order_status` là trạng thái thô. Có cả `total_amount` (= `total_cents`, VND không có đơn vị lẻ) |
| `DELETE /v1/holds/{holdID}` | Chủ hold, **bắt buộc `Idempotency-Key`** (BR-O5) | Một transaction: khoá hold + đơn `FOR UPDATE`, trả kho Postgres, đơn → `CANCELLED`, ghi outbox `ticketing.hold.cancelled`. Sau đó trả kho Redis (idempotent) — Postgres trước, Redis sau, như sweeper. 204; hold đã huỷ/hết hạn → 204 không trả kho lần nữa; đơn `PAYMENT_PENDING`/đã thanh toán → 409 `/problems/hold-not-cancellable`; hold lạ / của người khác → 404 |
| `GET /v1/events/{eventID}/availability` | Công khai | Mỗi hạng vé: `AVAILABLE` / `FEW_LEFT` (≤ 10% sức chứa) / `SOLD_OUT`. **Không** lộ số vé chính xác. Đọc từ Redis (đường nóng), tổng hợp mỗi sự kiện trong 3 giây (một request xuống nguồn, các request còn lại dùng chung), `Cache-Control: public, max-age=2`; lỗi không được cache; bộ nhớ cache giới hạn 1024 sự kiện |
| `GET …/tickets`, `POST …/issue-tickets` | Chủ đơn | Thêm kiểm sở hữu (404 nếu không phải chủ). **Fail-closed:** chưa cấu hình kiểm quyền thì trả 503 |

Gateway: thêm `/v1/holds/` (đăng nhập) và `GET /v1/events/{eventID}/availability` (công khai).
Web: `getOrderDetails` gọi `GET /v1/orders/{id}` kèm `Authorization` (token ở `ef_token`); fixture E2E đổi theo.

## 3. Tiêu chí nghiệm thu

| AC | Nội dung |
|---|---|
| AC-1 | Chủ đơn xem được đơn đầy đủ; người khác, đơn lạ, id rác → cùng 404 |
| AC-2 | Trạng thái đơn dịch đúng sang bộ giá trị web, kể cả `FAILED` khi thanh toán gần nhất thất bại |
| AC-3 | Huỷ hold trả kho đúng **một** lần ở Postgres **và** Redis (không lệch), kể cả 20 yêu cầu huỷ đồng thời, huỷ lại, và huỷ hold đã bị sweeper trả |
| AC-4 | Không huỷ được hold của người khác hay đơn đang/đã thanh toán; kho không đổi |
| AC-5 | Sau khi huỷ, khách giữ vé lại được ngay |
| AC-6 | Tồn kho công khai đúng mức, không lộ số chính xác, gộp request, không cache lỗi, cache không phình |
| AC-7 | Route vé chỉ cho chủ đơn; thiếu cấu hình thì đóng |

## 4. Phát hiện khi làm và giới hạn

- **Id hold hai dạng:** Postgres trả UUID có gạch ngang, Redis lưu hex 32 ký tự. Lần đầu huỷ hold chỉ trả kho Postgres và để Redis
  lệch −3 (test tích hợp bắt được). Use case giờ chuẩn hoá id trước khi gọi Redis. Mọi code mới gọi `Gate` bằng id từ client phải làm tương tự.
- **`hold.lua` chỉ quét tối đa 3 bucket** nên `ErrSoldOut` lúc cuối không chứng minh kho Redis = 0. EV-146 phải dựa vào tổng kho
  (Redis/Postgres), không dựa vào một lần giữ vé thất bại.
- Đơn trả `issued_tickets` **không có `qrPayload`**: QR chỉ sinh được lúc phát hành (`issue-tickets`, nay chỉ chủ đơn gọi được). Cần API
  riêng cho chủ vé lấy lại QR (follow-up của EV-133).
- Web chưa gửi `Authorization` cho các lời gọi hold/payment (vẫn gửi `X-Identity-Id`, bị gateway xoá) — thuộc công việc nối web ↔ gateway (EV-104/198).
- Chưa có consumer cho `ticketing.hold.cancelled`; thông báo cho notification là việc sau.
- Nhánh này tách khỏi EV-137, nên `handler.go` của ticketing sẽ có xung đột nhỏ khi merge (cả hai thêm trường vào `Handler`).
