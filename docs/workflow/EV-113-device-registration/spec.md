# EV-113 — API đăng ký thiết bị, liên kết identity ↔ device (Spec)

| Mục | Giá trị |
|---|---|
| Jira | EV-113 (mã backlog **EVF-13**) — 3 SP — Sprint A, việc #4 (phần 1/2, trước EV-137) |
| Kế hoạch | [11-ke-hoach-luong-mua-ve.md](../../11-ke-hoach-luong-mua-ve.md) — việc 2/5 |
| Branch | `feature/EV-113-device-registration`, chồng trên `feature/EV-111-otp-verified-gate` |

## 1. Bài toán

Bảng `devices` có sẵn nhưng không API nào ghi vào. EV-137 cần queue token gắn **người + thiết bị + sự kiện**, và antibot
(R4) cần đếm số identity trên một fingerprint. Cần: đăng ký thiết bị, gắn thiết bị vào phiên, đưa thiết bị tới các service.

## 2. Thiết kế

- **Lưu:** `devices(identity_id, fingerprint, last_ip, last_seen)`. `fingerprint` chỉ lưu **HMAC** (`pii.Protector.Hash`),
  không lưu nguyên văn; `idx_devices_fingerprint` vẫn cho phép đếm identity trên một thiết bị. Đăng ký là upsert theo
  `(identity_id, fingerprint)`: cập nhật `last_seen`, `last_ip` (IP hỏng/rỗng không ghi đè).
- **Giới hạn:** tối đa `MaxDevicesPerIdentity = 5` thiết bị mỗi identity. Thiết bị mới khi đã đủ → 429
  `/problems/too-many-devices`; thiết bị đã có luôn đăng nhập được. Kiểm trong transaction dưới
  `pg_advisory_xact_lock` theo identity nên đăng nhập đồng thời không vượt giới hạn.
- **API:**
  - `POST /api/v1/auth/devices` (Bearer) `{fingerprint}` → `{device_id}`, idempotent. Fingerprint 8–256 ký tự, ngoài ra 400
    `/problems/invalid-fingerprint`.
  - `POST /auth/login` nhận `device_fingerprint` (tuỳ chọn): đăng ký thiết bị **trước khi** cấp token; lỗi thiết bị → không cấp token.
  - `POST /auth/refresh` nhận `device_fingerprint` (tuỳ chọn): gắn hoặc đổi thiết bị của phiên. Không gửi thì phiên giữ thiết bị cũ.
    Cách để client Google/VNeID (không đi qua `login`) gắn thiết bị.
- **Phiên ↔ thiết bị:** cột `refresh_tokens.device_id` (migration `0005`), giữ nguyên khi xoay. Access token mang claim
  `did`; gateway đặt `X-Device-Id` từ claim (xoá giá trị client gửi). Principal có `DeviceID`, từ chối `X-Device-Id` không phải UUID.
- **IP:** lấy từ `X-Forwarded-For` do gateway đặt (gateway xoá giá trị client gửi).

## 3. Tiêu chí nghiệm thu

| AC | Nội dung |
|---|---|
| AC-1 | Đăng ký idempotent; DB chỉ có HMAC 64 hex của fingerprint; `last_ip` cập nhật |
| AC-2 | Tối đa 5 thiết bị/identity, đúng dưới đồng thời (20 đăng ký song song → đúng 5) |
| AC-3 | `login`/`refresh` gắn thiết bị; lỗi thiết bị khi login thì không cấp token (refresh: xem §4); token mang `did` |
| AC-4 | Thiết bị giữ nguyên qua xoay refresh; đổi được bằng refresh kèm fingerprint khác |
| AC-5 | Gateway chuyển `X-Device-Id` từ claim, không tin header client |
| AC-6 | Fingerprint hỏng → 400, quá nhiều thiết bị → 429, cần Bearer |

## 4. Giới hạn

- Refresh kèm fingerprint mới khi đã đủ 5 thiết bị trả 429 **sau khi** token cũ đã bị xoay (rotate nằm trong cùng
  luồng), nên client phải đăng nhập lại. Chấp nhận được vì đây là tình huống bất thường; nếu cần chặt hơn thì đăng ký thiết bị
  trước khi xoay.
- Fingerprint là tín hiệu client tự khai, không phải bằng chứng; giá trị của nó là làm tín hiệu cho antibot và ràng buộc queue token.
- Google/VNeID chưa nhận fingerprint trực tiếp (gắn qua `refresh`).
- Chưa có API liệt kê / gỡ thiết bị.
