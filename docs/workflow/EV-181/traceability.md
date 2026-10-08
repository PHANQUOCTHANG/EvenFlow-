# EV-181 — Ma trận truy vết AC ↔ Test (Traceability)

| AC | Nội dung yêu cầu | Test File | Test Case Name | Trạng thái |
|---|---|---|---|---|
| **AC-1** | Endpoint `/api/time` trả về epoch ms và RFC3339 có edge cache header | `src/app/api/time/route.test.ts` | `GET /api/time > trả về server_time và rfc3339 hợp lệ với Cache-Control s-maxage=1` | PASS |
| **AC-1** | Hook `useServerTimeSync` tính đúng độ lệch offsetMs có bù trừ RTT | `src/hooks/use-server-time-sync.test.tsx` | `useServerTimeSync > tính toán offsetMs chuẩn xác từ server response` | PASS |
| **AC-1** | Đếm ngược không bị ảnh hưởng bởi đồng hồ máy khách lệch | `src/hooks/use-server-time-sync.test.tsx` | `useServerTimeSync > lệch giờ client không làm sai lệch serverNow()` | PASS |
| **AC-2** | Jitter tạo độ trễ ngẫu nhiên phân bố trong 0..5000ms | `src/lib/jitter.test.ts` | `calculateJitterMs > phân bố ngẫu nhiên trong khoảng [0, 5000]ms` | PASS |
| **AC-2** | `sleepWithJitter` có callback cập nhật tiến trình | `src/lib/jitter.test.ts` | `sleepWithJitter > gọi callback tiến trình và kết thúc đúng hạn` | PASS |
| **AC-3** | Trang `events/[slug]` render tĩnh với ISR cấu hình revalidate | `src/app/(marketing)/events/[slug]/page.test.tsx` | `EventDetailPage > render thông tin sự kiện đầy đủ` | PASS |
| **AC-3** | Slug không tồn tại trả về 404 notFound() | `src/app/(marketing)/events/[slug]/page.test.tsx` | `EventDetailPage > slug không tồn tại kích hoạt notFound()` | PASS |
| **AC-4** | Tồn kho vé chỉ hiển thị dạng khoảng (AVAILABLE/FEW_LEFT/SOLD_OUT) | `src/components/event/ticket-tier-list.test.tsx` | `TicketTierList > hiển thị nhãn khoảng vé, không lộ số lượng tồn kho` | PASS |
| **AC-5** | Thông báo LOBBY xáo trộn ngẫu nhiên hiển thị rõ ràng | `src/components/event/lobby-notice.test.tsx` | `LobbyNotice > hiển thị cảnh báo không có số thứ tự vào sớm` | PASS |
| **AC-6** | Nút hành động áp dụng jitter trước khi vào phòng chờ | `src/components/event/event-action-panel.test.tsx` | `EventActionPanel > áp dụng jitter 0..5s và chuyển hướng tới /waiting/[id]` | PASS |
| **AC-6** | Nút mua vé bị vô hiệu khi sự kiện SOLD_OUT hoặc CANCELLED | `src/components/event/event-action-panel.test.tsx` | `EventActionPanel > disable nút khi hết vé hoặc đã huỷ` | PASS |
| **AC-7** | Accessible attributes (aria-live, aria-busy, keyboard focus) | `src/components/event/event-action-panel.test.tsx` | `EventActionPanel > cung cấp aria-busy và thông báo trực quan khi đang chờ jitter` | PASS |
