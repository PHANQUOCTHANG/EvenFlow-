# EV-182 — UI phòng chờ: vị trí, ETA, tiến trình, polling thích nghi + SSE (Spec)

| Thuộc tính | Chi tiết |
|---|---|
| Jira | EV-182 (mã backlog **EVF-112**) — 8 SP — Priority: P0 — Sprint 2 |
| Tier | **Critical** (chạm trực tiếp tải hệ thống: 300.000 khách cùng poll) |
| Base Commit | `5eeafdc` |
| Branch | `feature/EV-182-waiting-room-ui` |
| Nghiệp vụ | BR-Q1 (LOBBY không có rank), BR-Q4 (nhịp poll do server quyết), BR-Q7 (mất kết nối ≠ mất chỗ, TTL suất mua 15') |
| Phụ thuộc | EVF-54 (`GET /queue/status` + ETag) ✅ có trong `services/waitingroom`, EVF-57 (SSE `/queue/stream`) ✅ có, EVF-110 (shell + design system) ✅ có |

## 1. Bối cảnh & Hiện trạng

- `apps/web/src/lib/queue-client.ts` đã có `joinQueue` (jitter) và `watchQueue` (async generator polling).
  `watchQueue` không phơi bày ETag/connection state cho UI và không có SSE.
- `QueueStatusPanel` (EVF-1803) đã có copy chuẩn cho LOBBY / EXPIRED / SOLD_OUT / UNKNOWN / RECONNECTING.
- `EventActionPanel` (EV-181) sau jitter điều hướng tới `/waiting/[eventId]` — **route này chưa tồn tại** (404).
- Backend `services/waitingroom/internal/adapter/http/handler.go`:
  - `GET /v1/events/{id}/queue/status` — header `X-Queue-Token`, `If-None-Match`; trả 304 + `X-Poll-After-Ms`, 503 khi xả tải.
  - `GET /v1/events/{id}/queue/stream` — SSE `event: position`, **bắt buộc header `X-Queue-Token`**
    → không dùng được `EventSource` gốc (không gửi được header) ⇒ phải đọc SSE qua `fetch` + `ReadableStream`.
  - Server trả `rank = -1` ở LOBBY, `eta_seconds = -1` khi chưa ước lượng được.

## 2. Tiêu chí nghiệm thu (Acceptance Criteria)

Ba AC gốc của Jira là AC-1..AC-3; AC-4..AC-8 là các tiêu chí chi tiết hoá phạm vi công việc.

- **AC-1 — Tôn trọng `poll_after_ms` (BR-Q4)**: Lần poll kế tiếp chỉ được gửi sau đúng `poll_after_ms` của phản hồi 200,
  hoặc `X-Poll-After-Ms` của phản hồi 304, hoặc `Retry-After` của phản hồi 503. Gửi `If-None-Match` với ETag đã nhận.
  Giá trị không hợp lệ (thiếu, 0, âm, NaN) **không bao giờ** làm client poll nhanh hơn: rơi về 10s; giá trị dưới 1s bị kẹp lên 1s.
- **AC-2 — Reconnect có backoff, không bão request khi mất mạng**: Lỗi mạng/5xx → lùi theo cấp số nhân 1s, 2s, 4s… trần 30s,
  cộng jitter 0..500ms. Sự kiện `online` của trình duyệt **không** kích hoạt request tức thì. SSE dùng cùng chính sách backoff,
  quá số lần thử thì rơi về polling (không mở lại SSE ngay lập tức).
- **AC-3 — Tab ẩn không tăng tần suất poll**: Khi `document.visibilityState = "hidden"` hoặc khi tab hiện lại,
  client **không** gửi thêm request ngoài lịch đã hẹn (không refetch on focus/visibility).
- **AC-4 — SSE cho nhóm gần lượt + fallback polling**: Khi server cấp `poll_after_ms ≤ 3000` (nhóm gần lượt), client nâng cấp lên
  SSE `/queue/stream`. Stream đóng/lỗi → quay lại polling theo nhịp server gần nhất (không poll ngay).
- **AC-5 — Hiển thị vị trí/ETA/tiến trình trung thực**: `QueuePosition` hiển thị rank do server cấp; ETA chỉ khi server cung cấp
  (`eta_seconds ≥ 0`), nếu không ghi "đang cập nhật"; `ProgressRing` chỉ vẽ khi có mẫu số hợp lệ (rank lúc vào hàng), không bao giờ
  nhảy ngược; LOBBY không hiển thị rank (BR-Q1).
- **AC-6 — Chuyển sang chọn vé khi tới lượt**: Khi state = `ADMITTED`, `AdmitBanner` thông báo (role alert) + đồng hồ suất mua
  theo `expires_at` server, và tự điều hướng **một lần** tới `/checkout/[eventId]`; luôn có nút thủ công làm phương án dự phòng.
- **AC-7 — Trạng thái kết nối & lỗi**: Hiển thị "Đang cập nhật trực tiếp / Đang kết nối lại / Mất kết nối" và nhắc khách
  "chưa mất chỗ" khi đang reconnect. 401/403/404 từ status là lỗi cuối — dừng poll, không retry vô hạn.
- **AC-8 (Negative) — Join phòng chờ**: Route `/waiting/[eventId]` tái sử dụng `queue_token` trong `sessionStorage`, nếu chưa có thì gọi
  `joinQueue` **không jitter** (jitter đã chạy ở trang sự kiện). 401 → yêu cầu đăng nhập/OTP; 403 → cần xác minh thêm; lỗi khác →
  retry backoff tối đa 5 lần rồi hiện nút "Thử lại". Không hiển thị `queue_token` ra UI.

## 3. Ranh giới (Scope)

- **In Scope**:
  - `lib/backoff.ts`, `lib/queue-status.ts` (fetch một lần + chuẩn hoá payload), `lib/sse.ts` (SSE qua fetch, reconnect backoff).
  - `hooks/use-queue-status.ts`.
  - Component `ProgressRing`, `QueuePosition`, `AdmitBanner`, container `WaitingRoom`.
  - Route `app/(queue)/waiting/[eventId]/page.tsx`.
- **Out of Scope**:
  - Logic Redis/Lua xếp hàng, admit controller (backend Go — EVF-51/54/57).
  - Trang chọn vé `/checkout/[eventId]` (EVF-113) — task này chỉ điều hướng tới.
  - AI assistant drawer, waitlist API, CAPTCHA challenge UI.
  - Sửa `queue-client.ts` / `QueueStatusPanel` (giữ nguyên, chỉ tái sử dụng).
