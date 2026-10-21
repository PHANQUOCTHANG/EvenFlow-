# EV-118 — Sự kiện mới tự vào bán, tài khoản demo, danh sách công khai (Spec)

| Mục | Giá trị |
|---|---|
| Jira | EV-118 (lịch mở bán; phần scheduler), kèm phần backend của EV-122 (danh sách công khai) |
| Kế hoạch | [12-ke-hoach-demo-backend.md](../../12-ke-hoach-demo-backend.md) — bước 3 (G5, G6, G9) |
| Branch | `feature/EV-118-dynamic-active-events`, chồng trên `feature/EV-131-post-payment-chain` |
| Quy tắc | BR-E2 (T0 ≥ lúc duyệt + 30 phút), BR-Q1/Q2 (LOBBY trước T0, lottery tại T0) |

## 1. Hiện trạng

- Admit controller, sweeper, seeder, reconciler, soldout watcher chỉ phục vụ `ACTIVE_EVENT_IDS` cố định; `sale_start_ms` trong
  `wr:{ev}:meta` chỉ do `seed.sh` ghi. Sự kiện organizer tạo qua API được duyệt nhưng **không bao giờ bán được**.
- Không có scheduler: `SCHEDULED → ON_SALE` phải do Ops gọi `POST /publish` đúng lúc.
- BR-E2 cứng 30 phút: demo trực tiếp "tạo → duyệt → mở bán" phải chờ hơn nửa tiếng.
- Tài khoản mẫu có `password_hash = '!'`: không đăng nhập được; không có moderator/ops.
- `GET /api/v1/events` là danh sách "của tôi" (khách 401, buyer 403): trang chủ không có nguồn dữ liệu.
- Phát hiện khi chạy thật: ai-worker gửi kết quả kiểm duyệt tới `gateway:8080/internal/...`, gateway chặn `/internal/*` → kết quả AI
  không bao giờ tới event-service.
- `seed.sh RESET=1` lỗi khoá ngoại khi sự kiện đã có thanh toán/vé (`payments`, `tickets` tham chiếu `orders` không cascade).

## 2. Thiết kế

**Hợp đồng `libs/go/activeevents`:** SET Redis `wr:active_events`. Ticketing-worker ghi (thay cả tập trong một MULTI), controller đọc.

**ticketing-worker — `EventDiscovery`** (mỗi 5 giây):
- Postgres: sự kiện `ON_SALE`, `SOLD_OUT` (hold hết hạn có thể trả vé lại) và `SCHEDULED` mở bán trong 24 giờ tới.
- Cập nhật `ActiveEvents` (= `ACTIVE_EVENT_IDS` ∪ tìm thấy) mà sweeper, seeder, reconciler, soldout watcher đọc **mỗi vòng**
  (`Config.Events`); ghi tập sang Redis.
- Ghi `sale_start_ms` khi chưa có, hoặc khi T0 đang lưu **vẫn ở tương lai** (đổi lịch trước giờ mở). T0 đã qua không bao giờ sửa:
  lottery đã chạy theo nó.
- Postgres lỗi: giữ danh sách cũ, không ghi Redis (một lần mất kết nối không được gỡ sự kiện đang bán).

**waitingroom-controller — `EventSupervisor`** (mỗi `EVENTS_REFRESH_SECONDS`, mặc định 5):
- Danh sách = Redis ∪ `ACTIVE_EVENT_IDS` (nay tuỳ chọn). Mỗi sự kiện đúng một goroutine `runEvent` (tranh lease + fencing như EV-142).
- Sự kiện mới: bật trong một chu kỳ. Rời danh sách: huỷ context → leader nhả khoá. Goroutine tự thoát vì lỗi: chạy lại vòng sau.
- Sự kiện vẫn trong danh sách giữ nguyên goroutine (không mất leader). Đọc Redis lỗi: giữ nguyên mọi sự kiện đang chạy.

**event-service**
- `SaleOpener` (mỗi giây): `UPDATE events SET status='ON_SALE' WHERE status='SCHEDULED' AND sale_start_at <= now() RETURNING id`.
  Một lệnh có điều kiện nên nhiều instance chạy cùng lúc vẫn chỉ mở mỗi sự kiện một lần. Lottery/admit không phụ thuộc trạng thái
  này (chạy theo `sale_start_ms`); `ON_SALE` phục vụ trang sự kiện và danh sách.
- BR-E2 cấu hình được: `MIN_SALE_LEAD_SECONDS` (mặc định 30 phút, cảnh báo khi ngắn hơn). Compose đặt 60 giây cho demo.
- `GET /api/v1/events/public?limit=n`: công khai, chỉ `SCHEDULED`/`ON_SALE`/`SOLD_OUT`, gần mở bán nhất trước, `limit` mặc định 50,
  trần 100, `Cache-Control: public, max-age=10`. Gateway khai báo route công khai riêng.

**ai-worker:** `MODERATION_API_BASE` (compose: `http://event:8084`) cho callback kiểm duyệt; tool tra cứu vẫn đi qua gateway.

**seed.sh:** 4 tài khoản demo mật khẩu `Demo@123456` (hash argon2id sinh bằng hasher của identity), ghi đè khi chạy lại; `RESET=1`
xoá `tickets`, `payments`, `sale_reports` theo đúng thứ tự khoá ngoại.

## 3. Tiêu chí nghiệm thu

| AC | Nội dung |
|---|---|
| AC-1 | Sự kiện organizer tạo + được duyệt: trong ≤ 10 giây có trong `wr:active_events`, có `sale_start_ms`, kho đã nạp Redis; không sửa env, không restart |
| AC-2 | Khách join trước T0 vào `LOBBY`; tại T0 lottery chạy, khách được `ADMITTED`, giữ ghế được |
| AC-3 | Đến T0 sự kiện tự sang `ON_SALE`; chạy lặp không đổi thêm |
| AC-4 | Sự kiện huỷ/kết thúc rời danh sách; controller dừng điều khiển; sự kiện cố định không bao giờ rời |
| AC-5 | Postgres/Redis lỗi không gỡ sự kiện đang chạy |
| AC-6 | T0 đã qua không bị ghi đè; T0 tương lai được cập nhật khi đổi lịch |
| AC-7 | 4 tài khoản demo đăng nhập được; sai mật khẩu 401 |
| AC-8 | Danh sách công khai không lộ DRAFT/PENDING_REVIEW/REJECTED/CANCELLED; "của tôi" vẫn cần đăng nhập; limit bị kẹp |
| AC-9 | BR-E2 mặc định 30 phút; chỉ đổi khi đặt `MIN_SALE_LEAD_SECONDS` |
| AC-10 | Kết quả kiểm duyệt AI tới được event-service |

## 4. Ngoài phạm vi / còn hở

- Sự kiện huỷ rời danh sách ngay, nên hold còn sống của nó không được sweeper trả kho (luồng hoàn tiền EV-154 lo đơn đã trả tiền).
- `SystemTools` của ai-worker gọi `GET /v1/events/{id}` — route không tồn tại (đúng là `/api/v1/events/{id}`); tool trả lỗi, trợ lý trả lời fallback.
- identity trả 500 (không phải 401) khi `password_hash` không phải argon2id hợp lệ (vd `'!'` của tài khoản k6).
- Trần nhịp admit vẫn tính riêng từng sự kiện; nhiều sự kiện mở cùng lúc cộng dồn (giới hạn có từ EV-142).
