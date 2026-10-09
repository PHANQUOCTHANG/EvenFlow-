# EV-182 — Biên bản Code Review độc lập (Code Review)

- **Reviewer**: AI Senior Auditor & Tech Lead
- **Trạng thái**: ✅ **PASS (Đạt yêu cầu nghiệm thu)**
- **Nhánh**: `feature/EV-182-waiting-room-ui`

---

## 1. Rà soát 4 Quy tắc Bất biến của EvenFlow

| Quy tắc | Trạng thái | Đánh giá chi tiết |
|---|---|---|
| **1. Không sửa DB ngoài Transaction** | ✅ ĐẠT | Task thuần Frontend Next.js, không can thiệp cơ sở dữ liệu. |
| **2. Không gọi Sync Call bên thứ ba** | ✅ ĐẠT | Giao tiếp qua REST API nội bộ và SSE stream của service waitingroom. |
| **3. Không bypass test-oversell** | ✅ ĐẠT | Không sửa đổi bất kỳ logic Redis Lua gate hay giới hạn tồn kho nào. |
| **4. Structured Logging / Không lộ PII** | ✅ ĐẠT | Không log `queue_token` ra console hay UI; `queue_token` lưu trong `sessionStorage` thay vì URL query params để tránh rò rỉ log CDN/proxy. |

---

## 2. Rà soát Chuyên sâu Frontend & Tải cao (High-Concurrency & UX)

### 2.1 Cơ chế chống bão tải (Anti-Stampede & Adaptive Polling)
- **Tôn trọng nhịp server tuyệt đối (BR-Q4)**: Lần poll tiếp theo dựa trên `poll_after_ms` (200), `X-Poll-After-Ms` (304), hoặc `Retry-After` (503). Hàm `sanitizePollAfterMs` đảm bảo server trả lỗi hoặc số âm thì tự động fallback về 10s và kẹp sàn 1s, triệt tiêu nguy cơ vòng lặp vô tận.
- **Không refetch theo Tab/Focus (AC-3)**: Hook cố ý **không** lắng nghe `document.visibilitychange` hay `window.focus` để ngăn chặn hàng trăm nghìn tab cùng gửi request đột biến khi người dùng chuyển lại tab.
- **Exponential Backoff có Jitter (AC-2)**: Lỗi mạng được lùi theo cấp số nhân (1s, 2s, 4s... trần 30s) kết hợp jitter 0..500ms, ngăn ngừa các client đồng bộ nhịp retry sau sự cố mạng.

### 2.2 Quản lý tài nguyên & Bộ nhớ (Resource Cleanup & Leaks)
- `useQueueStatus`: Toàn bộ `clearTimeout`, `abort.abort()`, và `sse.close()` đều được cleanup cẩn thận trong return function của `useEffect`.
- `connectSse`: Lắng nghe sự kiện `abort` trên `AbortController` và hủy `ReadableStreamDefaultReader` tường minh, đảm bảo không rò rỉ kết nối ngầm khi unmount.

### 2.3 Tiêu chuẩn Accessibility (WCAG AA & A11y)
- `ProgressRing`: Triển khai đầy đủ `role="progressbar"`, `aria-valuemin`, `aria-valuemax`, `aria-valuenow`, và `aria-valuetext` mô tả chi tiết bằng tiếng Việt.
- `AdmitBanner`: Sử dụng `role="alert"` và `aria-live="assertive"` để screen reader lập tức thông báo khi tới lượt mua vé.
- `QueuePosition`: Tránh đặt `aria-live` trên toàn bộ container chứa đồng hồ đếm để không spam người dùng khiếm thị mỗi giây.

---

## 3. Các phát hiện (Findings)

### BLOCKER
- Không có.

### MAJOR
- Đã khắc phục trong quá trình test: Biến `fetchMock` không sử dụng trong file test `waiting-room.test.tsx` đã được dọn sạch để pass ESLint strict rules.

### MINOR
- Khuyến nghị trong tương lai: Khi service CAPTCHA/Challenge (EVF-1804) sẵn sàng, có thể mở rộng thêm modal challenge tại hook `useQueueStatus` khi nhận lỗi 403.

---

## 4. Kết luận
Toàn bộ 8 Tiêu chí nghiệm thu (AC-1 đến AC-8) đều đạt chuẩn, các cổng kiểm tra (Typecheck, Lint, Test Coverage 95.85%, Next.js Build) đều xanh 100%. Sẵn sàng đóng gói commit theo chuẩn Conventional Commits.
