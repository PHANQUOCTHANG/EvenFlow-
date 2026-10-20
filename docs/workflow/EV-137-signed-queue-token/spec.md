# EV-137 — Queue token ký HMAC gắn người + thiết bị + sự kiện (Spec)

| Mục | Giá trị |
|---|---|
| Jira | EV-137 (mã backlog **EVF-50**) — 3 SP — Sprint A, việc #4 (phần 2/2, sau EV-113) |
| Kế hoạch | [11-ke-hoach-luong-mua-ve.md](../../11-ke-hoach-luong-mua-ve.md) — việc 3/5 |
| Branch | `feature/EV-137-signed-queue-token`, chồng trên `feature/EV-113-device-registration` |
| Quy tắc | BR-Q3 (một người một chỗ), BR-Q7 (suất admit) |

## 1. Hiện trạng

Queue token là 32 ký tự hex ngẫu nhiên. Ticketing chỉ kiểm token có nằm trong ZSET `admitted`, **không kiểm token thuộc
về ai**. Một token đã được admit có thể chia cho nhiều tài khoản/thiết bị (kể cả bán lại), và status/stream cho bất kỳ ai
biết token đọc vị trí của người khác.

## 2. Thiết kế

**Định dạng** (`libs/go/authx/queue.go`): `q1.<base64url(payload)>.<base64url(HMAC-SHA256)>`; payload `{eid, sub, did, jti,
iat, exp}`. HMAC có tiền tố domain riêng (không lẫn với QR vé hay access token dùng chung secret). Xác minh bằng
`hmac.Equal`. `QueueClaims.Matches(eventID, subject, deviceID)`: đúng sự kiện + đúng người + (nếu token gắn thiết bị) đúng
thiết bị đang gọi.

**Redis không đổi:** thành viên của hàng chờ, `lobby`, `admitted` vẫn là `jti` (chuỗi hex 32 ký tự), nên script Lua, controller,
reconciler và harness oversell giữ nguyên. Token ký chỉ là lớp bọc quanh `jti`.

| Nơi | Hành vi |
|---|---|
| Waitingroom `join` | Sau khi vào hàng (idempotent theo identity, BR-Q3 → cùng `jti`), ký token mới `{eid, sub, did, jti}` hạn 24 giờ (= TTL phiên hàng). Join lại từ thiết bị khác trả cùng chỗ, token gắn thiết bị mới. Cần `X-Device-Id` (403 `/problems/device-required`) trừ khi `QUEUE_REQUIRE_DEVICE=false` |
| Waitingroom `status` / `stream` | Xác minh chữ ký, hạn, và `Matches(path eventID, X-Identity-Id, X-Device-Id)` **trước khi** chạm Redis. 401 `queue-token-invalid` / `queue-token-expired` (client join lại), 403 `queue-token-mismatch`, 400 thiếu token |
| Ticketing `POST …/holds` | Xác minh như trên rồi hỏi `IsAdmitted(eid, jti)`. Token của người/thiết bị/sự kiện khác → 403, không chạy use case. Đọc danh tính bằng `PrincipalFromHeaders` (401 nếu thiếu) |
| Secret | `QUEUE_TOKEN_SECRET` (≥ 16 byte) dùng chung hai service. Thiếu: dev dùng secret mặc định + cảnh báo; `APP_ENV=production` → không khởi động. Handler thiếu secret → 503 (fail-closed) |

## 3. Tiêu chí nghiệm thu

| AC | Nội dung |
|---|---|
| AC-1 | Token ký đúng; sửa payload/chữ ký, khác secret, token hex cũ, access token hay token rỗng đều bị từ chối; hết hạn có lỗi riêng |
| AC-2 | Join cấp token ký mang đúng eid/sub/did/jti; Redis chỉ giữ `jti` |
| AC-3 | Chia sẻ token: người khác, thiết bị khác, sự kiện khác → 403 ở status, stream **và** tạo hold; Redis/use case không bị chạm |
| AC-4 | Join lại từ thiết bị khác vẫn là cùng một chỗ (BR-Q3), token mới gắn thiết bị mới |
| AC-5 | Join thiếu thiết bị bị từ chối khi bắt buộc |
| AC-6 | Thiếu secret → fail-closed; production thiếu secret → không khởi động |

## 4. Triển khai và giới hạn

- **Token cũ mất hiệu lực** ngay khi deploy: khách đang trong hàng phải join lại (giữ nguyên chỗ nhờ BR-Q3, nhưng web phải gọi
  `join` lại khi nhận `queue-token-invalid`/`expired`). **Deploy ngoài giờ mở bán.**
- Web: đăng nhập gửi `device_fingerprint` (đã làm, `lib/device.ts`); chưa xử lý `device-required`, `queue-token-*`.
- Token vẫn là bearer: ai chiếm được cả access token lẫn queue token vẫn dùng được. Ràng buộc thiết bị chỉ nâng chi phí chia sẻ.
- k6 `opening-spike.js` đã cập nhật (đặt header danh tính như gateway, join lấy token ký). Chưa chạy lại tải.
- Chưa chạy lại cổng oversell 10.000 goroutine: harness gọi thẳng use case với `jti` nên không đi qua lớp mới.
