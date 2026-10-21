# EV-198 — Script demo end-to-end, sửa smoke, diễn tập (Spec)

| Mục | Giá trị |
|---|---|
| Jira | EV-198 (E2E trên compose thật), kèm sửa lỗi phát hiện khi diễn tập |
| Kế hoạch | [12-ke-hoach-demo-backend.md](../../12-ke-hoach-demo-backend.md) — bước 4 (G7) |
| Branch | `feature/EV-198-demo-script`, chồng trên `feature/EV-118-dynamic-active-events` |

## 1. Phạm vi

1. **`deploy/scripts/demo.sh` + `make demo`**: 33 bước, mọi request qua gateway; đọc OTP và email vé từ Mailpit; tự tạo sự kiện và
   người mua mới mỗi lần nên chạy lặp được. `DEMO_LEAD` (mặc định 75 giây) và `DEMO_SOLDOUT` (mặc định 1) điều chỉnh.
2. **Sửa `smoke.sh`** (G7): gửi `X-Identity-Role/Verified/Device-Id` (smoke gọi thẳng service nên đóng vai gateway); kiểm "giữ chỗ" bằng
   `jti` thay vì so chuỗi token (token ký lại mỗi lần join, EV-137).
3. **`tests/demo/eventflow.http`**: cùng luồng để bấm tay (REST Client; thời gian mở bán dùng biến động `{{$datetime}}`).
4. **CI**: `e2e-nightly.yml` chạy thêm `make seed` và `make demo` sau smoke.
5. **Kịch bản trình bày** [docs/demo/kich-ban.md](../../demo/kich-ban.md).

## 2. Lỗi phát hiện khi diễn tập và đã sửa

| Lỗi | Biểu hiện | Sửa |
|---|---|---|
| Healthcheck Postgres qua unix socket | Khởi động lần đầu: `migrate` thoát lỗi `connection refused` vì `pg_isready` báo "healthy" khi server tạm của entrypoint (chỉ nghe socket) còn chạy; chập chờn nên các lần trước may mắn qua | `pg_isready -h 127.0.0.1` (qua TCP) |
| `lib/pq` + PgBouncer chế độ transaction | event-service: `pq: unnamed prepared statement does not exist`; kết quả kiểm duyệt AI bị mất, job vào DLQ sau retry, sự kiện kẹt `PENDING_REVIEW` | identity, event nối thẳng Postgres trong compose. `binary_parameters=yes` không dùng được: cột `jsonb` của outbox nhận `[]byte` sẽ gửi dạng nhị phân sai. ticketing/payment/notification (pgx) vẫn qua PgBouncer |
| Hold đã thanh toán nằm lại Redis tới khi hết hạn (10 phút) | Cờ `SOLD_OUT` (EV-146) đếm cả hold đã trả tiền nên trễ tới 10 phút sau lần mua cuối: khách đang chờ vẫn thấy `QUEUED`. `Gate.Consume` có sẵn nhưng không nơi nào gọi | `IssueTicketsUseCase.WithHoldCleanup`: sau khi phát vé (kể cả khi đơn đã `ISSUED` từ trước) xoá hold khỏi Redis **không trả kho**; lỗi chỉ ghi log. `Repository.ListOrderHoldIDs` |
| Poll `queue/status` bị `429` | Gateway giới hạn 0,5 lần/giây/người (BR-Q4); script poll mỗi giây | Poll mỗi 2,5 giây |
| `SOLD_OUT` ở `join` không có token | Khi cờ đã bật trước lúc join, trả `SOLD_OUT` không cấp token (đúng thiết kế) | Script nhận cả hai nhánh |

## 3. Tiêu chí nghiệm thu

| AC | Nội dung |
|---|---|
| AC-1 | Từ volume trống: `make up && make migrate && make seed && make demo` đạt, 2 lần liên tiếp từ trạng thái trống và lặp lại trên cùng stack |
| AC-2 | `make smoke` đạt 20/20 sau các thay đổi của bước 1–3 |
| AC-3 | Khởi động từ volume trống ≥ 3 lần: mọi container chạy, không `migrate` lỗi, không service nào restart |
| AC-4 | Phát vé xong thì hold rời Redis, khách đang chờ nhận `SOLD_OUT` trong vài giây sau khi kho về 0 |
| AC-5 | Phát vé không bao giờ trả kho Redis (chỉ `Consume`); lỗi dọn Redis không làm hỏng phát vé |

## 4. Còn hở

- `SOLD_OUT` trên `availability` cho sự kiện **chưa nạp kho Redis** (vài giây đầu sau khi sự kiện mới vào bán) là chủ ý cũ ("chưa nạp = tin phía an toàn", có test): script chờ `AVAILABLE`. Trang web nên hiển thị "đang chuẩn bị" thay vì "hết vé" trong khoảng này.
- Production vẫn cần giải pháp cho `lib/pq` qua PgBouncer: chuyển identity/event sang `pgx/stdlib`, hoặc dùng PgBouncer chế độ session cho hai service này.
- Bước "giữ ghế hết hạn" không có trong `make demo` (TTL 10 phút theo BR-O2); `make smoke` phủ bằng TTL 5 giây.
- Trace Jaeger mới có waitingroom, ticketing, ticketing-worker (EV-103 chưa làm).
