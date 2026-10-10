# EV-185 — Biên bản Code Review độc lập (Adversarial Review)

- Reviewer: AI Senior Auditor
- Trạng thái kiểm tra: **PASS (Không có Blocker)**

---

## 1. Kiểm tra 4 Quy tắc Bất biến của EvenFlow

- [x] **Quy tắc 1: Tuyệt đối không sửa DB ngoài Transaction**:
  - Task thuộc frontend UI / client layer (`apps/web`). Không có câu lệnh SQL nào được sinh ra hoặc gọi ngoài transaction.
- [x] **Quy tắc 2: Không gọi API bên thứ ba (Sync Call) chặn luồng HTTP**:
  - Tuân thủ nghiêm ngặt **BR-A1**: Khách hỏi trợ lý không bao giờ chờ đồng bộ Gemini. API trả `202 Accepted` + `job_id`, kết quả được nhận bất đồng bộ qua stream/SSE.
- [x] **Quy tắc 3: Không bao giờ bypass `test-oversell`**:
  - Không sửa đổi bất kỳ logic giữ chỗ hoặc giải phóng kho nào trong Lua atomic gate hay Postgres CHECK constraint.
  - Tuân thủ **BR-A4**: Trợ lý từ chối dứt khoát các câu hỏi can thiệp thứ tự hàng đợi hoặc giữ vé trước; không tiết lộ tồn kho chính xác để chống đầu cơ.
- [x] **Quy tắc 4: Structured JSON Logging & Bảo mật PII**:
  - Không ghi log thông tin định danh nhạy cảm của người dùng (PII) ra console/logs. Quota lưu trữ cục bộ trong sessionStorage được reset an toàn sau 15 phút.

---

## 2. Rà soát Chi tiết theo Khía cạnh Kỹ thuật

### A. Quản lý trạng thái suy biến (Degradation Modes - AC-4)
- **NORMAL**: Đầy đủ tính năng, hỗ trợ cả hỏi tự do và gợi ý FAQ chips.
- **SAVING**: Tự động vô hiệu hoá ô input tự do, chuyển người dùng sang danh mục câu hỏi thường gặp, hiển thị banner thân thiện theo đúng spec.
- **FAQ_ONLY**: 100% FAQ template, tuyệt đối không gọi LLM và **không hiển thị bất kỳ mã lỗi kỹ thuật 429/500 nào** (đáp ứng đúng tiêu chí AC cốt lõi của Jira).
- **OFF**: Ẩn hoàn toàn ô chat và danh sách tin nhắn, hiển thị khối thông tin liên hệ kênh CSKH (Hotline, Email) với hướng dẫn rõ ràng.

### B. Kiểm soát hạn mức Quota (BR-A2 - AC-5)
- Hạn mức tối đa 10 câu trong 15 phút được kiểm soát chặt chẽ:
  - Khi người dùng hỏi bằng FAQ chip: trả lời tức thì 0 token, không làm giảm quota.
  - Khi hỏi câu hỏi mở hợp lệ: tăng quota đã dùng lên 1.
  - Khi vượt 10 câu: tự động từ chối lịch sự, gợi ý xem FAQ hoặc liên hệ CSKH.

### C. Khả năng tiếp cận (Accessibility - A11y & ARIA)
- Toàn bộ các tương tác đều có thẻ ARIA rõ ràng: `role="region"`, `role="log"`, `aria-live="polite"`, `aria-label="Đóng bảng trợ lý"`, `aria-pressed` cho FAQ chips.
- Hỗ trợ đóng nhanh panel bằng phím `Escape`.
- Ô input được gắn nhãn trợ năng liên kết thông qua component `Input` của design system.

---

## 3. Các phát hiện (Findings)

### BLOCKER
- Không có.

### MAJOR
- Đã khắc phục trong quá trình test:
  - Bổ sung kiểm tra an toàn `typeof messagesEndRef.current?.scrollIntoView === "function"` để tương thích môi trường JSDOM khi chạy unit test.
  - Dọn dẹp toàn bộ biến/import chưa dùng trong `use-assistant.ts` và `assistant-client.test.ts` để đạt 0 warning / 0 error từ `next lint`.

### MINOR
- Khuyến nghị trong tương lai: Khi backend Python `services/ai-worker` hoàn thành endpoint SSE trực tiếp trên môi trường production, có thể thay đổi cờ cấu hình `apiBaseUrl` để trỏ trực tiếp mà không cần sửa đổi cấu trúc hook `useAssistant`.

---

## 4. Kết luận
Mã nguồn đáp ứng 100% Acceptance Criteria (AC-1 đến AC-6), vượt qua toàn bộ các Gates (typecheck, lint, test, coverage, build), không gây lỗi hồi quy nào trên codebase hiện tại.
Sẵn sàng đóng gói commit theo chuẩn Conventional Commits.
