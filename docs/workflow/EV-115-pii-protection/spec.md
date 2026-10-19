# EV-115 — Mã hoá số điện thoại, lọc PII khỏi log, bỏ log DB URL (Spec)

| Mục | Giá trị |
|---|---|
| Jira | EV-115 (mã backlog **EVF-15**) — 3 SP — Sprint A, việc #9 |
| Kế hoạch tổng | [10-ke-hoach-hoan-thien-du-an.md](../../10-ke-hoach-hoan-thien-du-an.md) — làn bảo mật #1 → #2 → #3 → **#9** |
| Branch | `feature/EV-115-pii-protection`, chồng trên `feature/EV-114-event-route-authorization` (cùng sửa `identity/cmd/server/main.go` với EV-112) |

## 1. Hiện trạng (`develop` @ `f2154d8`)

| # | Vấn đề | Bằng chứng |
|---|---|---|
| P1 | Log khởi động in `DATABASE_URL` **kèm mật khẩu** | `main.go`: `log.Info("ket noi database", "url", cfg.DatabaseURL)` |
| P2 | Số điện thoại lưu **nguyên văn** trong cột `phone_hash`; `phone_enc` không dùng | `register.go`: `identity.PhoneHash = &input.Identifier` |
| P3 | `fmt.Printf` in email/SĐT và **mã OTP** ra stdout | `register.go`, `forgot_password.go`: `[MOCK] SENDING OTP: %s to %s` |
| P4 | OTP sinh bằng `math/rand` gieo theo thời gian — đoán lại được | `rand.Seed(time.Now().UnixNano())` |
| P5 | Không có lớp chặn PII nào cho slog | — |

## 2. Thiết kế

**`libs/go/pii`** (dùng chung):

- `Protector`: AES-256-GCM (nonce ngẫu nhiên, byte version cho xoay khoá, AAD gắn bản mã với đúng dòng
  `identities.phone_enc:<id>`) và HMAC-SHA256 (tra cứu + `UNIQUE`). Hai khoá base64 32 byte, phải khác nhau.
  HMAC có khoá thay vì SHA-256 trần: không ai brute-force ~10⁹ số di động từ bản DB bị lộ.
- `NormalizePhoneVN`: `09…`, `84…`, `+84 …` về một dạng `84xxxxxxxxx`.
- `NewRedactingHandler`: bọc `slog.Handler`. Khoá chứa `password/secret/token/otp/authorization/cookie/dsn/
  database_url/…` → `[REDACTED]`; khoá `email/phone/identifier/cccd/…` → che một phần; mọi chuỗi và lỗi được
  quét email, SĐT, JWT, số CCCD 12 chữ số, mật khẩu trong URL (`://user:***@`). Là lưới an toàn, không thay cho
  việc không log PII.

**Identity:**

| Việc | Cách làm |
|---|---|
| Lưu SĐT | Use case chỉ đặt `Identity.Phone` (đã chuẩn hoá, chỉ trong bộ nhớ); repository ghi `phone_hash` = HMAC, `phone_enc` = AES-GCM |
| Tra cứu | Định danh là SĐT → so `phone_hash` với HMAC của dạng chuẩn hoá; còn lại so `email` |
| Dữ liệu cũ | Khởi động chạy `ProtectLegacyPhones`: dòng có `phone_hash` khác 64 ký tự (SĐT thô) → HMAC + mã hoá. Idempotent. Hai dòng cùng một số ở hai dạng → dòng sau đụng `UNIQUE`, **bỏ qua và báo** (`skipped_duplicates`) |
| Log | Logger mặc định đi qua bộ lọc; bỏ dòng log `DATABASE_URL`; `fmt.Printf` → `slog` với email/SĐT đã che |
| OTP không có kênh gửi | Không bao giờ vào log. Dev (`APP_ENV` ≠ production): in ra stderr qua `DevOTPSink`, định danh đã che. Production: không in; SĐT nhận OTP qua Telegram bot như cũ |
| Khoá | `PII_ENC_KEY`, `PII_HASH_KEY` (base64 32 byte). Dev thiếu → khoá mặc định cố định + cảnh báo. `APP_ENV=production` thiếu khoá hoặc `JWT_SECRET` → dừng khởi động |
| OTP | `crypto/rand` thay `math/rand` |

## 3. Tiêu chí nghiệm thu

| AC | Nội dung |
|---|---|
| AC-1 | DB không còn SĐT thô: `phone_hash` là HMAC 64 hex, `phone_enc` giải mã đúng, bản mã chép sang dòng khác không giải mã được |
| AC-2 | Đăng nhập/đặt lại mật khẩu bằng `09…`, `84…`, `+84 …` cùng ra một tài khoản; đăng ký lại cùng số khác dạng → trùng |
| AC-3 | Dòng cũ được chuyển khi khởi động, idempotent, bản trùng bị bỏ qua có báo |
| AC-4 | Log không chứa mật khẩu DB, token, OTP, email/SĐT đầy đủ, JWT, CCCD — kể cả trong thông điệp lỗi và group |
| AC-5 | OTP không vào log; chỉ kênh dev nhận, production không in |
| AC-6 | Production thiếu khoá → không khởi động |
| AC-7 | OTP sinh từ nguồn ngẫu nhiên mật mã |

## 4. Giới hạn và việc để lại

- **Bản trùng còn SĐT thô:** dòng bị bỏ qua (cùng một số ở hai dạng — dữ liệu cũ cho phép điều đó) cần người gộp tài
  khoản; log khởi động báo số lượng.
- **Xoay khoá:** định dạng có byte version nhưng mới có một khoá; xoay khoá cần thêm khoá cũ để giải mã và job mã hoá
  lại.
- **Service khác** (event, ticketing, payment…) chưa bọc logger bằng `pii.NewRedactingHandler`; nên làm cùng EV-103 khi
  đổi sang `otelx`.
- `phone_enc` mới được ghi, chưa có chỗ đọc (gửi SMS là Sprint D).
- Redis vẫn dùng định danh thô làm khoá OTP/rate limit (`otp:<email|sđt>`), TTL 5 phút; ngoài phạm vi.
