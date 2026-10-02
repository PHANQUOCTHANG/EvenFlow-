# Trang showcase design system — spec

| | |
|---|---|
| Mục đích | Trang công khai phô được những gì đã dựng, dùng làm **link nộp báo cáo tiến độ**, tài liệu design system cho đồng đội, và bằng chứng trực quan cho môn Kiểm thử |
| Tier | **Standard** |
| Base | `develop` sau khi merge 4 branch (`944b1d0`) |
| Branch | `feature/showcase-page` |
| Host | Vercel nối thẳng GitHub, Root Directory `apps/web` |

---

## 1. Bối cảnh

Repo đã có **49 component** và **1016 test**, nhưng `apps/web` chỉ có **đúng một page** (`(marketing)/page.tsx`) là thẻ placeholder nhỏ. Deploy lên Vercel bây giờ thì người mở link thấy gần như trống — phản ánh sai tiến độ theo hướng bất lợi.

Trang showcase sửa đúng khoảng cách đó: không phải tính năng cho người mua vé, mà là **bề mặt trình bày** cho những gì đã có.

## 2. Ràng buộc phải giữ

**2.1 Mọi số liệu phải được gắn nhãn là dữ liệu mẫu.** Handoff mục 8 "DATA AND PROTOTYPE RULES":
*"Label all numbers, attendee counts, ticket counts, prices, ETAs, charts, and account details as
sample/demo data. Do not present prototype data as live truth."*

Trang này đầy số: giá vé, số thứ tự hàng đợi, ETA, đồng hồ đếm ngược. **Không được** để người xem
tưởng là dữ liệu thật của một sự kiện đang bán. Phải có nhãn ở đầu trang và ở từng khối có số.

**2.2 Không được đóng băng mốc thời gian vào HTML.** Mọi route ở đây prerender tĩnh. Nếu tính
`Date.now() + 10 phút` ngay trong thân component thì mốc của **thời điểm build** bị nướng vào HTML —
người mở trang hôm sau sẽ thấy mọi đồng hồ đã hết hạn. Đây đúng là lỗi đã sửa ở `useServerCountdown`;
trang showcase không được tái tạo nó ở tầng trên. Mốc demo phải sinh **sau khi mount**.

**2.3 Không hard-code màu.** Mọi màu qua token, như mọi component khác.

**2.4 Không giả vờ là trang thật.** Trang không được có nút dẫn tới luồng mua vé không tồn tại.
Mọi hành động trong showcase chỉ để minh hoạ trạng thái.

## 3. Phạm vi

| Khối | Nội dung |
|---|---|
| Token | Bảng màu (light/dark), thang typography, radius, spacing |
| Primitive | Button 4 variant × 3 size + disabled/loading; Input 4 type; Select; Checkbox; Radio; Alert 4 variant; Toast; Badge 10 variant; Spinner 3 size; Card |
| Hàng chờ | `QueueStatusPanel` đủ **8** trạng thái |
| Đồng hồ | `ServerExpiryCountdown` cả **3** variant, có cả trạng thái cảnh báo và hết hạn |
| Sự kiện | `EventCard` đủ **6** trạng thái + skeleton |
| Checkout | `TicketTierCard` các trạng thái; `OrderSummary` đủ **6** trạng thái hold |

### Ngoài phạm vi
- Page thật cho `(queue)` `(checkout)` `(organizer)` `(ops)` — cần API contract
- Gọi API, dữ liệu thật

## 4. AC

### AC-1 — Nhãn dữ liệu mẫu
- **Then** đầu trang có cảnh báo rõ rằng **toàn bộ** số liệu là dữ liệu mẫu, không phải sự kiện thật
- **Then** cảnh báo đó nằm trong vùng đọc được bởi screen reader, không chỉ là màu/icon

### AC-2 — Không đóng băng thời gian
- **Then** không component nào trong trang tính mốc thời hạn trong thân render; mốc demo sinh sau khi mount
- **Then** trước khi mount, đồng hồ hiển thị trạng thái "chưa biết" (`--:--`), **không** phải `00:00` và **không** phải "đã hết hạn"
- **Bằng chứng**: render tĩnh trang (không chạy effect) không được chứa chuỗi của trạng thái hết hạn

### AC-3 — Phủ đủ trạng thái
- **Then** `QueueStatusPanel` xuất hiện đủ 8 trạng thái; `EventCard` đủ 6; `OrderSummary` đủ 6; `ServerExpiryCountdown` đủ 3 variant
- **Then** `Badge` xuất hiện đủ 10 variant

### AC-4 — Không màu hard-code
- **Then** grep `#hex` / `rgb(` / `hsl(` trong file của trang: rỗng

### AC-5 — Điều hướng
- **Then** trang chủ có link tới showcase; showcase có link về trang chủ
- **Then** trang nằm trong route group `(marketing)` nên dùng `PublicShell`, có đúng một `<main>`

### AC-6 — Gates
- **Then** lint, typecheck, test:coverage, build pass; 1016 test hiện có không hồi quy

## 5. Rủi ro

| Rủi ro | Giảm thiểu |
|---|---|
| Mốc thời gian bị nướng vào HTML lúc build | AC-2 có test trên render tĩnh |
| Người chấm tưởng là dữ liệu thật | AC-1 bắt có nhãn ở đầu trang |
| Trang nặng, kéo First Load JS | Chấp nhận: đây là trang demo, không nằm trên đường găng của giờ mở bán |
