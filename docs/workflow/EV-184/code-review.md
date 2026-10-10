# EV-184 — Biên bản Code Review độc lập (Adversarial Review)

- Reviewer: AI Senior Auditor
- Trạng thái kiểm tra: **PASS (Không có blocker, không có vi phạm quy tắc)**

## 1. Kiểm tra 4 Quy tắc Bất biến

- [x] **Không thao tác DB ngoài transaction**: Tầng frontend Next.js chỉ giao tiếp qua REST API client, không thao tác DB trực tiếp.
- [x] **Không gọi sync API bên thứ ba**: Luồng thanh toán và webhook được xử lý bất đồng bộ theo BR-O6. Client không coi browser redirect là thành công mà poll trạng thái xác nhận từ server.
- [x] **Không phá vỡ concurrency / oversell protection**:
  - BR-O2: Đồng hồ giữ ghế tiếp tục đếm theo mốc `expires_at` của server. Khi hết hạn, giao diện lập tức khóa thanh toán và dọn dẹp active hold trong `sessionStorage`.
  - BR-O5: Bắt buộc đính kèm `Idempotency-Key` (UUID v4) trong header `initiatePayment()`. Có cơ chế chống double submit (`isSubmittingRef` + disable form & button).
- [x] **Structured JSON log & Bảo mật PII**:
  - Mã QR hiển thị trên vé điện tử sử dụng visual SVG demo, không chứa token xác thực hay chữ ký mật.
  - Các trường thông tin người nhận vé (Email, Phone) tuân thủ validation và không bị phơi bày ra bên ngoài.

## 2. Kiểm tra Trợ năng (Accessibility) & Edge Cases

- [x] **WCAG AA Compliance**:
  - Form nhập thông tin người nhận vé dùng `<Input>` với nhãn liên kết `<label for="...">`, không dùng placeholder thay label.
  - Chọn phương thức thanh toán dùng `<RadioGroup>` với `<fieldset role="radiogroup">` và `<legend>`.
  - Đồng hồ đếm ngược có `aria-live="polite"` và text thay thế cho screen reader.
- [x] **Xử lý tình huống Webhook chậm (AC-4)**:
  - Khi cổng thanh toán chưa phản hồi, trang kết quả giữ nguyên trạng thái `PENDING` ("Đang xác nhận"), tiếp tục polling với backoff và cung cấp nút kiểm tra thủ công. Tuyệt đối không báo lỗi sai lệch.
- [x] **Xử lý tình huống Hold hết hạn giữa lúc thanh toán (AC-3)**:
  - Giao diện tự động khoá nút, báo rõ lý do vé đã trả kho, và dẫn nút quay về chọn lại vé.

## 3. Các phát hiện (Findings)

### BLOCKER
- Không có.

### MAJOR
- Đã khắc phục: Loại bỏ unused variable `isPendingWebhook` và `Spinner` để pass 100% ESLint zero-warning.
- Đã khắc phục: Sử dụng variant `"paid"` chuẩn cho `Badge` component để pass TypeScript typecheck.

### MINOR
- Không có.
