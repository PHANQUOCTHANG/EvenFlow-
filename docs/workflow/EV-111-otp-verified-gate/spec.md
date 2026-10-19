# EV-111 — Ghi nhận đã verify OTP; phòng chờ từ chối người chưa verify (Spec)

| Mục | Giá trị |
|---|---|
| Jira | EV-111 (mã backlog **EVF-11**) — 3 SP — Sprint A, việc #5 |
| Kế hoạch | [11-ke-hoach-luong-mua-ve.md](../../11-ke-hoach-luong-mua-ve.md) — làn luồng mua, việc 1/5 |
| Branch | `feature/EV-111-otp-verified-gate` (từ `develop` @ `1d4b358`) |
| Quy tắc | BR-Q3: token xếp hàng gắn với identity **đã xác thực OTP** |

## 1. Hiện trạng

Cột `identities.otp_verified_at` có sẵn nhưng **không code nào ghi**; phòng chờ chỉ kiểm có `X-Identity-Id`. Tạo tài khoản ảo
là vào hàng chờ được ngay. Phòng chờ còn tin header `X-Identity-Id` thô (fallback "cho local dev").

## 2. Thiết kế

- **Identity ghi `otp_verified_at`** khi tạo tài khoản: đăng ký OTP email/SĐT; Google (và từ nay **từ chối** Google
  token có `email_verified=false`); VNeID (định danh quốc gia). VNeID còn thiếu `created_at` — đã gán.
- **Access token mang claim `ver`** = `otp_verified_at IS NOT NULL`, đọc lại từ DB ở mỗi lần refresh. Token cấp trước
  thay đổi này không có `ver` → coi là chưa xác thực tới khi refresh (≤ 15 phút).
- **Gateway** đặt header `X-Identity-Verified: true|false` từ claim; header này nằm trong danh sách bị xoá nếu client gửi.
- **Phòng chờ** đọc danh tính bằng `authx.PrincipalFromHeaders` (bỏ fallback header thô). Thiếu danh tính → 401;
  chưa verify → 403 `/problems/otp-required`, **không ghi danh** vào hàng.
- **Migration `0004`**: backfill `otp_verified_at = created_at` cho tài khoản cũ (mọi đường tạo tài khoản đều đã qua
  OTP/IdP). Down là no-op vì không phân biệt được dòng đã backfill.

## 3. Tiêu chí nghiệm thu

| AC | Nội dung |
|---|---|
| AC-1 | Mọi đường tạo tài khoản (OTP, Google, VNeID) ghi `otp_verified_at`; Google email chưa xác thực bị từ chối |
| AC-2 | Access token mang `ver`; gateway chuyển thành header và không tin header client gửi |
| AC-3 | `join` thiếu danh tính → 401; chưa verify → 403 `otp-required`; đã verify → 200 |
| AC-4 | Tài khoản cũ được backfill |

## 4. Giới hạn

- Ticketing vẫn đọc `X-Identity-Id` và chưa kiểm `ver`; hold đã cần queue token admit, nên cổng chặn nằm ở `join`.
  EV-137 sẽ ràng buộc token với người gọi.
- Web: bắt `/problems/otp-required` để chuyển sang màn xác thực.
