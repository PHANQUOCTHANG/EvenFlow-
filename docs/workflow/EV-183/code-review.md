# EV-183 — Biên bản Code Review độc lập (Adversarial Review)

- **Reviewer**: Senior Tech Lead & Security Reviewer
- **Mã Task**: EV-183 (EVF-113) — Chọn vé + đồng hồ giữ ghế 10:00 theo expires_at của server
- **Trạng thái kiểm tra**: **PASS** (Đủ điều kiện tạo PR vào develop)

---

## 1. Kiểm tra 4 Quy tắc Bất biến (EvenFlow Core Invariants)

- [x] **1. Tuyệt đối không sửa DB ngoài Transaction**:
  - *Đánh giá*: Task thuộc phạm vi frontend Next.js 15 (`apps/web`). Mọi tương tác kho vé đi qua HTTP API (`POST /v1/events/{id}/holds`) của Ticketing service — nơi đã được bảo vệ bằng Postgres Transaction và CHECK constraint.
- [x] **2. Không gọi API bên thứ ba (Sync Call) chặn luồng HTTP**:
  - *Đánh giá*: Phía client sử dụng async fetch, bắt timeout/lỗi mạng để chuyển trạng thái an toàn `ambiguous` theo BR-O5, không chặn thread.
- [x] **3. Không bao giờ bypass kiểm tra oversell / tính toàn vẹn tồn kho**:
  - *Đánh giá*:
    - **BR-O1**: Không hiển thị số lượng tồn kho chính xác, chỉ dùng nhãn định tính (`AVAILABLE`, `FEW_LEFT`, `SOLD_OUT`).
    - **BR-O2**: Đồng hồ giữ ghế lấy `expires_at` tuyệt đối từ server, kết hợp `offsetMs` chuẩn hóa từ server; đổi giờ máy khách không thể kéo dài thời gian giữ vé.
    - **BR-O3**: Đang có hold ở hạng vé khác thì cảnh báo rõ ràng và không cho tạo hold đè.
    - **BR-O4**: Tôn trọng `maxSelectable` do server chỉ định, không hardcode default sai lệch.
    - **BR-O5**: Bắt buộc sinh và gửi `Idempotency-Key` (UUIDv4) cho mọi request ghi; khoá nút bấm (`creating`) chống double-click; mạng chập chờn chuyển sang `ambiguous`.
- [x] **4. Structured JSON Logging / Bảo mật PII & Secret**:
  - *Đánh giá*: Không log thông tin nhạy cảm, token phòng chờ `queue_token` được lưu trữ an toàn trong `sessionStorage` và gửi qua header HTTP, không in ra màn hình hoặc console.

---

## 2. Rà soát Chi tiết Kỹ thuật & Khả năng Tiếp cận (A11y)

### A. Race Conditions & Double-Submit Protection
- `useHoldTimer` chặn ngay từ đầu hàm `createHold` nếu `holdState === "creating"`.
- `OrderSummary` disable nút bấm và hiển thị spinner khi `creating`.
- `hold-client` sinh UUIDv4 đảm bảo tính đơn nhất cho mỗi lượt ghi.

### B. Accessibility (WCAG 2.1 AA)
- Phân chia landmark rõ ràng: `<header>`, `<section aria-label="Danh sách hạng vé">`, `<aside>`, `<section aria-label="Tóm tắt đơn hàng">`.
- Các thông báo lỗi và cảnh báo sử dụng `role="alert"` hoặc `role="status"` tương thích tốt với Screen Readers.
- Nút bấm và input có đầy đủ `aria-label`, thẻ giá có `tabular-nums` và format chuẩn tiền tệ.

---

## 3. Các phát hiện (Findings)

### BLOCKER
- **Không có.**

### MAJOR
- **Đã xử lý**: Cảnh báo unused variables trong `hold-client.ts`, `hold-client.test.ts`, `checkout-view.tsx` được Gate ESLint phát hiện đã được khắc phục triệt để.

### MINOR
- **Ghi nhận**: Khi hold hết hạn (`expired`), nút bấm tự động đổi thành "Chọn lại vé" và gọi `resetHold()`, cho phép người dùng bắt đầu lại quy trình chọn vé mà không cần tải lại toàn bộ trang.

---

## 4. Kết luận
Toàn bộ 6 file hồ sơ workflow, 4 file mã nguồn/test mới cùng 1223 test cases đã vượt qua 100% các Gate kiểm tra tự động. Đủ điều kiện đóng gói commit theo Conventional Commits.
