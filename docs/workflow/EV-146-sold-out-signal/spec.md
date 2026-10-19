# EV-146 — Báo hết vé cho khách đang chờ (Spec)

| Mục | Giá trị |
|---|---|
| Jira | EV-146 (mã backlog **EVF-46**) — 3 SP — Sprint A, việc #8 |
| Kế hoạch | [11-ke-hoach-luong-mua-ve.md](../../11-ke-hoach-luong-mua-ve.md) — việc 5/5 |
| Branch | `feature/EV-146-sold-out-signal`, chồng trên `feature/EV-137-signed-queue-token` |
| Quy tắc | BR-Q6: hết vé thì mọi người QUEUED nhận `SOLD_OUT`, không đẩy thêm ai vào checkout |

## 1. Hiện trạng

`Queue.MarkSoldOut` (đặt `meta.sold_out` trong Redis) có sẵn nhưng không ai gọi. Controller và `admit.lua` đã đọc cờ này, nhưng nó không
bao giờ được đặt: hết vé rồi mà phòng chờ vẫn tiếp tục thả người vào checkout, status/SSE không bao giờ trả `SOLD_OUT`, người mới vẫn
được xếp hàng vô vọng.

## 2. Thiết kế

**Ai quyết định hết vé: ticketing** (chủ sở hữu tồn kho), qua `SoldOutWatcher` trong worker (mỗi giây, các sự kiện trong
`ACTIVE_EVENT_IDS`). Hợp đồng giữa hai service là hai cờ trong hash `wr:{ev}:meta` của phòng chờ:

| Cờ | Điều kiện đặt | Hiệu lực |
|---|---|---|
| `stock_zero` | Tổng kho Redis = 0 (đã nạp) | `admit.lua` ngừng thả người. Khách **không** thấy thay đổi (còn hold có thể hết hạn trả vé) |
| `sold_out` | Kho = 0 **và** không còn hold nào, liên tiếp 3 lần quan sát | `status` trả `SOLD_OUT` cho người **chưa** được vào (QUEUED/LOBBY); `join` của người mới trả `SOLD_OUT` không cấp token, không ghi danh; `admit` ngừng; SSE gửi trạng thái rồi đóng |

- Cờ tự gỡ ngay vòng sau khi kho có lại (hoàn tiền, hold hết hạn) — người trong hàng quay lại `QUEUED`, không mất chỗ.
- Người đã `ADMITTED` giữ nguyên trạng thái để hoàn tất đơn (ví dụ vé cuối vừa được giữ).
- **Không dùng "giữ vé thất bại" làm bằng chứng:** `hold.lua` chỉ quét tối đa 3 bucket nên `ErrSoldOut` có thể xảy ra khi bucket khác còn vé.
  Tín hiệu dựa trên **tổng** kho.
- **Kho chưa nạp ≠ hết vé:** `Stock` trả `Seeded=false` nếu hạng vé nào chưa có key trong Redis; watcher không ghi gì (tránh báo
  `SOLD_OUT` cho sự kiện chưa nạp kho hay khi Redis vừa bị xoá). Đọc kho và đếm hold trong cùng một MULTI.
- Khởi động lại worker ghi trạng thái rỗng một lần để xoá cờ cũ.

**Web:** `join` nhận `SOLD_OUT` (không token) → hiện panel "Đã hết vé", không poll. Token bị status từ chối 401 (token hex cũ trước EV-137 hay
hết hạn) → tự `join` lại đúng một lần (BR-Q3 giữ chỗ), lần hai vẫn lỗi thì hiện màn lỗi.

## 3. Tiêu chí nghiệm thu

| AC | Nội dung |
|---|---|
| AC-1 | Kho 0 + còn hold: ngừng admit, khách không đổi trạng thái. Kho 0 + không hold ổn định: `SOLD_OUT` cho người QUEUED/LOBBY |
| AC-2 | Người đã `ADMITTED` không bị đổi trạng thái; token lạ vẫn `UNKNOWN` |
| AC-3 | Khách mới lúc hết vé hẳn: `SOLD_OUT`, không token, không vào hàng; người đã có chỗ join lại vẫn nhận chỗ cũ (BR-Q3) |
| AC-4 | Cờ gỡ ngay khi kho có lại; người trong hàng quay về `QUEUED` |
| AC-5 | Kho chưa nạp, lỗi đọc Redis, lỗi ghi: không báo hết vé sai; ghi lỗi được thử lại vòng sau |
| AC-6 | Chống nhiễu: cần 3 lần quan sát liên tiếp; hold xuất hiện lại thì đếm lại |
| AC-7 | Web hiện màn hết vé và không poll; tự vào lại hàng khi token bị từ chối |

## 4. Giới hạn và việc để lại

- **Trạng thái `SOLD_OUT` của sự kiện ở event-svc** (domain đã có `MarkSoldOut`) chưa nối: cần outbox → consumer ở event, để Sprint B.
- Waitlist sau hết vé (BR-Q6 "mời vào waitlist") để Sprint D; panel chỉ hiện nút khi caller có chỗ mời.
- Watcher chạy trong worker nên cần `ACTIVE_EVENT_IDS` đúng (cùng danh sách với sweeper/seeder). Chạy nhiều replica worker vẫn an toàn (ghi idempotent), chỉ tốn thêm Redis.
- Độ trễ báo hết ≈ 3–4 giây sau vé cuối (3 lần quan sát × 1 giây + nhịp poll của client 30 giây ở trạng thái xa; SSE nhận ngay).
- Web vẫn chưa gửi `Authorization` cho join/status/SSE (việc nối web ↔ gateway, EV-104/198).
