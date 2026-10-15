# EV-150 — Adapter cổng thanh toán (VNPay/Momo/Stripe) + sandbox mock (Spec)

| Thuộc tính | Chi tiết |
|---|---|
| Jira | **EV-150** (mã backlog **EVF-70**) — 8 SP — Priority: **P0 (Highest)** — Sprint 3 |
| Tier | **Critical / Financial Gateway** (Tầng cổng thanh toán cốt lõi, bảo toàn tài chính) |
| Base Commit | `0345684d3592d335cb1292bb6e5e36ca2b9c8232` |
| Branch | `feature/EV-150-payment-gateway-adapters` |
| Nghiệp vụ liên quan | **BR-O2** (giữ ghế 10:00 theo `expires_at` của server, hết hạn từ chối thanh toán), **BR-O3** (1 hold hoạt động/identity/event), **BR-O5** (bắt buộc `Idempotency-Key` cho API ghi), **BR-O6** (webhook exactly-once, provider_txn_id UNIQUE) |
| Phụ thuộc | **EVF-32** (Ghi hold vào Postgres trong transaction, chọn bucket ngẫu nhiên) ✅ |
| Khách hàng của task | **EVF-71 / EV-151** (Webhook exactly-once), **EVF-114 / EV-184** (Frontend Payment Flow & Order Result) |

---

## 1. Bối cảnh & Hiện trạng

- Sau khi người dùng được Admit từ phòng chờ ảo và hoàn tất việc chọn vé (tạo hold thành công ở trạng thái `HELD`), bước tiếp theo là tiến hành thanh toán vé để chuyển hóa thành vé chính thức (`PAID` → `ISSUED`).
- Hiện trạng:
  - `services/payment` mới chỉ có khung rỗng (`cmd/server/main.go`) với endpoint `/healthz`.
  - Frontend `apps/web/src/lib/payment-client.ts` đã định nghĩa hợp đồng gọi API `POST /v1/events/{eventId}/payments` kèm `Idempotency-Key` và hỗ trợ các phương thức `vnpay`, `momo`, `stripe`, `sandbox`.
  - Cần xây dựng kiến trúc Hexagonal hoàn chỉnh cho `services/payment`, cung cấp Port trừu tượng `PaymentGateway` và các Adapter cụ thể cho VNPay, MoMo, Stripe, cùng Sandbox Mock để môi trường phát triển local hoạt động độc lập 100% không phụ thuộc dịch vụ ngoài.

---

## 2. Tiêu chí nghiệm thu (Acceptance Criteria)

- **AC-1 — Port `PaymentGateway` & Abstract Interfaces (DIP)**:
  - Định nghĩa interface `PaymentGateway` tại tầng `port`, không phụ thuộc vào framework HTTP/SQL cụ thể:
    - `InitiatePayment(ctx, req) (PaymentResult, error)`: Khởi tạo giao dịch thanh toán, sinh redirect URL hoặc checkout session.
    - `VerifyCallback(ctx, callbackParams) (CallbackResult, error)`: Kiểm tra tính toàn vẹn và chữ ký của phản hồi từ cổng.
    - `Provider() Provider`: Trả về định danh nhà cung cấp (`sandbox`, `vnpay`, `momo`, `stripe`).
  - Hỗ trợ `PaymentRegistry` / `GatewayFactory` cho phép quản lý và tra cứu gateway theo `Provider`.

- **AC-2 — Adapter VNPay (Sandbox / Production ready)**:
  - Sinh URL chuyển hướng thanh toán VNPay chuẩn:
    - Thuật toán chữ ký HMAC-SHA512 (`vnp_SecureHash`).
    - Các tham số tiêu chuẩn: `vnp_Version`, `vnp_Command=pay`, `vnp_TmnCode`, `vnp_Amount` (nhân 100 theo quy định VNPay), `vnp_CurrCode=VND`, `vnp_TxnRef`, `vnp_OrderInfo`, `vnp_ReturnUrl`, `vnp_IpAddr`, `vnp_CreateDate`.
  - Sắp xếp tham số theo thứ tự alphabet trước khi ký hash.
  - Hàm thẩm định chữ ký callback / return URL từ VNPay.

- **AC-3 — Adapter MoMo (Sandbox / Production ready)**:
  - Sinh payload thanh toán MoMo:
    - Thuật toán chữ ký HMAC-SHA256 (`signature`).
    - Các tham số: `partnerCode`, `orderId`, `requestId`, `amount`, `orderInfo`, `redirectUrl`, `ipnUrl`, `extraData`, `requestType=captureWallet`.
  - Thẩm định chữ ký callback / redirect từ MoMo.

- **AC-4 — Adapter Stripe (Sandbox / Production ready)**:
  - Hỗ trợ tạo checkout session URL cho thẻ tín dụng quốc tế (Visa/Mastercard).
  - Chuyển đổi tiền tệ chính xác (VND sang đơn vị chuẩn hoặc USD cents).
  - Mockable HTTP client để kiểm thử tự động không cần gọi Stripe API thật.

- **AC-5 — Adapter Sandbox Mock Gateway (Chạy local 100%)**:
  - Hoạt động tức thì local mà không cần kết nối mạng hay API key thật.
  - Hỗ trợ các kịch bản kiểm thử:
    - *Auto-Success*: Sinh redirect URL tức thì với trạng thái thành công.
    - *Auto-Fail*: Mô phỏng giao dịch bị từ chối / lỗi tài khoản.
    - *Simulate-Delay*: Trả về URL mà không sinh webhook ngay, mô phỏng đơn hàng bị trễ để kiểm thử Job đối soát (EVF-73).

- **AC-6 — Luồng chuyển trạng thái `HELD` → `PAYMENT_PENDING` & Bảo vệ Hold (BR-O2, BR-O3, BR-O4)**:
  - Khi nhận yêu cầu thanh toán:
    1. Kiểm tra đơn hàng tồn tại và đang ở trạng thái `HELD`.
    2. Kiểm tra thời hạn giữ vé: `orders.expires_at > now()`. Nếu đã quá hạn giữ ghế (BR-O2), từ chối ngay lập tức và trả về mã lỗi `409 Conflict` (`ErrHoldExpired`).
    3. Kiểm tra số tiền `amount` khớp chính xác với `orders.total_cents` và đúng loại tiền tệ `currency`.
    4. Cập nhật trạng thái đơn hàng từ `HELD` sang `PAYMENT_PENDING` trong **Postgres DB Transaction an toàn** (Quy tắc bất biến 1).
    5. Ghi nhận bản ghi thanh toán vào bảng `payments` với trạng thái `PENDING`.
    6. Trả về kết quả khởi tạo cho client.

- **AC-7 — Cấu hình linh hoạt (Config-driven Gateway Selection)**:
  - Cho phép cấu hình cổng mặc định và danh sách các cổng được kích hoạt qua biến môi trường (`PAYMENT_DEFAULT_PROVIDER`, `PAYMENT_ENABLED_PROVIDERS`, `VNPAY_*`, `MOMO_*`, `STRIPE_*`, `SANDBOX_*`).
  - Nếu client không chỉ định method, tự động dùng default provider.
  - Nếu client yêu cầu provider bị vô hiệu hóa hoặc không tồn tại, trả về lỗi `400 Bad Request` rõ ràng.

- **AC-8 — REST API Handler & Idempotency-Key (BR-O5)**:
  - `POST /v1/events/{eventID}/payments`:
    - Bắt buộc header `Idempotency-Key` (BR-O5) được kiểm soát qua middleware `libs/go/idem`.
    - Body khớp hợp đồng `apps/web/src/lib/payment-client.ts`:
      `order_id`, `hold_id`, `amount`, `method`, `buyer_name`, `buyer_email`, `buyer_phone`, `return_url`.
    - Response: `payment_id`, `order_id`, `redirect_url`, `status`, `provider_txn_id`.
  - `GET /v1/payments/{paymentID}`: Truy vấn thông tin và trạng thái thanh toán.
  - `GET /v1/payments/methods`: Trả về danh sách cổng thanh toán đang hoạt động kèm tên hiển thị, icon và mô tả.

- **AC-9 — Tuân thủ 4 Quy tắc Bất biến & Độ tin cậy**:
  - Không sửa DB ngoài Transaction.
  - Không gọi API bên ngoài chặn luồng HTTP vô hạn (mọi network call có timeout 5s, circuit breaker/fallback an toàn).
  - Tuyệt đối không log thông tin nhạy cảm PII (số thẻ, OTP, secret key). Structured JSON log luôn có `trace_id`, `event_id`, `order_id`, `provider`.

---

## 3. Ranh giới (Scope)

- **In Scope**:
  - Toàn bộ layout Hexagonal cho `services/payment` (`domain`, `port`, `app`, `adapter/gateway`, `adapter/http`, `adapter/postgres`, `config`).
  - Tích hợp 4 cổng: Sandbox Mock, VNPay, MoMo, Stripe.
  - REST API `POST /v1/events/{eventID}/payments`, `GET /v1/payments/{paymentID}`, `GET /v1/payments/methods`.
  - Bộ Unit test đầy đủ cho Domain, Use Cases, Gateway Adapters, HTTP Handler.
  - Cập nhật `services/payment/cmd/server/main.go` khởi chạy toàn bộ server với các route trên.

- **Out of Scope**:
  - Xử lý Webhook callback bất đồng bộ từ các cổng (sẽ do task tiếp theo **EV-151 / EVF-71** thực hiện).
  - Background Job đối soát đơn treo quá 15 phút (thuộc task **EV-153 / EVF-73**).
  - Luồng hoàn tiền khi sự kiện bị hủy (thuộc task **EV-154 / EVF-74**).
