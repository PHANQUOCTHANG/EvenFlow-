# EV-181 — Trang sự kiện ISR + đồng hồ đếm ngược theo giờ server (Spec)

| Thuộc tính | Chi tiết |
|---|---|
| Jira | EV-181 — 5 SP — Priority: P0 — Sprint 2 |
| Tier | **Critical** (Chạm trực tiếp luồng scale Tầng 1 & Tầng 2: ISR tĩnh + Jitter client 0..5s tại T0) |
| Base Commit | `c848afa` |
| Branch | `feature/EV-181-event-page-isr` |
| Nguồn quy chuẩn | `docs/01-nghiep-vu.md` BR-A4, BR-Q1, BR-Q5, BR-O2; `docs/02-kien-truc.md` Tầng 1 & 2; `docs/08-stitch-ui-ux-handoff.md` §4, §11.2 |

---

## 1. Bối cảnh & Hiện trạng

- **Vấn đề cần giải quyết:** Tại thời điểm mở bán sự kiện HOT (T0), hàng trăm nghìn người cùng truy cập. Nếu trang sự kiện chạm database hoặc nếu đồng hồ đếm ngược dựa vào giờ máy khách (client clock):
  1. Giờ máy khách bị lệch (chạy nhanh hoặc chậm vài phút) sẽ khiến người dùng vào phòng chờ sai thời điểm hoặc bị lỡ đợt xáo trộn ngẫu nhiên.
  2. Hàng trăm nghìn client cùng bắn request tại đúng T0 giây 0 sẽ gây bão xung đột (300k rps).
  3. Hiển thị số lượng vé chính xác sẽ kích thích bot vét vé và gây rò rỉ dữ liệu tồn kho theo thời gian thực.
- **Giải pháp EvenFlow (Tầng 1 & Tầng 2):**
  - Trang sự kiện công khai được render tĩnh bằng **Next.js ISR (Incremental Static Regeneration)** phục vụ từ Edge/CDN, không query DB trực tiếp lúc cao điểm (BR-A4).
  - Tồn kho vé chỉ hiển thị dạng khoảng ("Còn vé", "Sắp hết", "Hết vé").
  - Đồng hồ đếm ngược đồng bộ độ lệch (offset) qua endpoint `/api/time` có cache edge 1s (`s-maxage=1`), tính mốc tuyệt đối `target - serverNow(offset)`.
  - Áp dụng **Jitter phía client ngẫu nhiên 0..5s** tại T0 (`lib/jitter.ts` - EVF-61) trước khi chuyển hướng sang `/waiting/[eventId]` để làm phẳng đỉnh 300k rps thành ~60k rps mà không gây thiệt thòi cho người dùng nhờ cơ chế lottery/xáo trộn của phòng chờ.

---

## 2. Tiêu chí nghiệm thu (Acceptance Criteria)

- **AC-1 (Server Time Synchronization):**
  - Endpoint `/api/time` trả về thời gian server dạng `{ server_time: number, rfc3339: string }` kèm header `Cache-Control: public, s-maxage=1, stale-while-revalidate=5`.
  - Hook `useServerTimeSync` tự động gọi lấy giờ server một lần khi mount, đo độ trễ mạng (Round-Trip Time / 2) để tính `offsetMs = (server_time + rtt/2) - clientNowMs`.
  - Máy khách bị lệch giờ (dù sớm hay trễ 1 giờ) vẫn đếm ngược chuẩn xác tới đúng thời điểm mở bán của server.

- **AC-2 (Client-side Jitter 0..5s tại T0 - BR-Q5 / EVF-61):**
  - Module `lib/jitter.ts` cung cấp hàm `calculateJitterMs(seed?: string, maxJitterMs?: number)` tạo độ trễ ngẫu nhiên phân bố đều trong khoảng $[0, 5000]\text{ms}$.
  - Khi đồng hồ chạm mốc T0 hoặc người dùng bấm nút "Vào phòng chờ" lúc vừa mở bán, áp dụng độ trễ jitter kèm trạng thái trực quan ("Đang sắp xếp lượt vào phòng chờ...").
  - Đảm bảo tải request phân bố đều trên 5 giây, triệt tiêu đột biến tức thời.

- **AC-3 (Trang sự kiện ISR `events/[slug]`):**
  - Route: `apps/web/src/app/(marketing)/events/[slug]/page.tsx`.
  - Áp dụng `export const revalidate = 60` (hoặc ISR revalidate theo cấu hình) và cung cấp `generateStaticParams` cho các slug dựng sẵn.
  - Xử lý `notFound()` (HTTP 404) chuẩn cho các slug không tồn tại.
  - Có đầy đủ Metadata SEO (title, description, openGraph, twitter card).

- **AC-4 (Hiển thị khoảng vé - BR-A4):**
  - Danh sách các hạng vé (Ticket Tiers) chỉ hiển thị trạng thái tồn kho ước lượng:
    - `AVAILABLE` ("Còn nhiều vé")
    - `FEW_LEFT` ("Sắp hết vé")
    - `SOLD_OUT` ("Hết vé")
  - Tuyệt đối không render số lượng tồn kho cụ thể (ví dụ "còn 12 vé").

- **AC-5 (Minh bạch luật phòng chờ LOBBY - BR-Q1):**
  - Khi sự kiện ở trạng thái `SCHEDULED` (chưa tới T0): hiển thị đồng hồ đếm ngược `ServerExpiryCountdown` (variant `sale-start`).
  - Giao diện phải có thông báo giải thích rõ ràng cho khách: *"Hệ thống sử dụng phòng chờ ngẫu nhiên tại thời điểm mở bán. Mọi người tham gia trước giờ mở bán đều có cơ hội như nhau, vào sớm không tạo ưu thế."*

- **AC-6 (Nút điều hướng phòng chờ thích ứng trạng thái):**
  - `SCHEDULED`: Nút hiển thị "Chưa mở bán" (disabled) hoặc "Vào phòng chờ sớm (LOBBY)" nếu sự kiện cho phép vào lobby trước giờ G.
  - `ON_SALE`: Nút hiển thị "Vào phòng chờ / Mua vé", kích hoạt và chuyển tiếp tới `/waiting/[eventId]` sau khi qua jitter.
  - `SOLD_OUT` / `CLOSED` / `COMPLETED` / `CANCELLED`: Vô hiệu hóa nút mua vé kèm nhãn trạng thái tương ứng.

- **AC-7 (Accessibility & Responsive UX):**
  - Hỗ trợ đầy đủ phím bấm (Keyboard navigable), nhãn cho trình đọc màn hình (`aria-live`, `aria-busy` khi đang chờ jitter).
  - Tương thích hoàn toàn giao diện sáng/tối (Dark Mode) và responsive từ mobile (375px) đến desktop (1200px+).

---

## 3. Ranh giới (Scope)

### In Scope
1. Module `src/lib/jitter.ts` và unit test `src/lib/jitter.test.ts`.
2. API endpoint `src/app/api/time/route.ts` và unit test `src/app/api/time/route.test.ts`.
3. Hook `src/hooks/use-server-time-sync.ts` và test `src/hooks/use-server-time-sync.test.tsx`.
4. Mock service / client dữ liệu sự kiện snapshot `src/lib/event-service.ts`.
5. Component giao diện sự kiện:
   - `src/components/event/event-hero.tsx` (Hero banner, countdown, action button with jitter)
   - `src/components/event/ticket-tier-list.tsx` (Danh sách hạng vé theo khoảng BR-A4)
   - `src/components/event/lobby-notice.tsx` (Thông báo quy chế LOBBY BR-Q1)
6. Trang ISR `src/app/(marketing)/events/[slug]/page.tsx` và trang danh sách `src/app/(marketing)/events/page.tsx`.
7. Cập nhật `src/components/layout/nav-config.ts` kích hoạt link `/events`.
8. Kiểm thử đơn vị toàn diện với Vitest đạt coverage $\ge 70\%$.

### Out of Scope
1. Logic xếp hàng Redis lua join / ticket hold (đã thuộc service backend Go hoặc EVF-51 / EVF-112).
2. Tích hợp thanh toán cổng ngân hàng VNPay / MoMo (thuộc EVF-114).
3. Đăng nhập / xác thực OAuth (thuộc Identity service).
