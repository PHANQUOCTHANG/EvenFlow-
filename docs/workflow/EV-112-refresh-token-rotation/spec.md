# EV-112 — Access token 15 phút và refresh token xoay vòng, thu hồi được (Spec)

| Mục | Giá trị |
|---|---|
| Jira | EV-112 (mã backlog **EVF-12**) — 5 SP — Sprint A, việc #1 |
| Kế hoạch tổng | [10-ke-hoach-hoan-thien-du-an.md](../../10-ke-hoach-hoan-thien-du-an.md) — Sprint A, làn bảo mật (#1 → #2 → #3 → #9) |
| Branch | `feature/EV-112-refresh-token-rotation` (từ `develop` @ `f2154d8`) |
| Phụ thuộc ngược | Gateway (EV-NEW-1) xác minh access token bằng `authx.VerifyAccessToken` của slice này |

## 1. Bài toán

Identity cấp một JWT sống **24 giờ**, không có refresh token, không có cách thu hồi. Token lộ (XSS, máy dùng chung,
log) dùng được cả ngày; đổi mật khẩu cũng không đuổi được kẻ đang giữ token. Gateway sắp xác minh JWT trên mọi
request nên định dạng token phải chốt trước.

## 2. Hiện trạng trước slice (`develop` @ `f2154d8`)

| # | Vấn đề | Bằng chứng |
|---|---|---|
| H1 | Token sống 24 giờ | `domain/jwt.go`: `exp = now + 24h` |
| H2 | Không có refresh, không thu hồi | Không có bảng hay endpoint nào |
| H3 | JWT chỉ có `sub/role/iat/exp`: không `iss`, `aud`, `typ`. Mọi JWT HS256 ký cùng secret đều được chấp nhận | `rbac_middleware.go` chỉ kiểm `SigningMethodHMAC` (nhận cả HS384/HS512) |
| H4 | Đặt lại mật khẩu không đụng tới phiên đang mở | `forgot_password.go` |

## 3. Thiết kế

**Access token** (`libs/go/authx/access.go`, dùng chung identity ↔ gateway): JWT HS256, 15 phút,
`iss=eventflow-identity`, `aud=eventflow-api`, `typ=access`, `sub`, `role`, `jti`, `iat`, `exp`. Xác minh chỉ nhận
HS256, bắt buộc `exp`, lệch đồng hồ ±30 giây. Hết hạn trả lỗi riêng (`ErrExpiredAccessToken`) để client biết nên
refresh thay vì đăng nhập lại.

**Refresh token**: 256 bit ngẫu nhiên (base64url), DB chỉ giữ SHA-256. Bảng `refresh_tokens`
(migration `0003_refresh_tokens.sql`) có `family_id`: mỗi lần đăng nhập mở một họ.

| Thao tác | Hành vi |
|---|---|
| Đăng nhập (mật khẩu, Google, VNeID) | Tạo họ mới, token sống 30 ngày |
| `POST /api/v1/auth/refresh` | Trong một transaction, `SELECT … FOR UPDATE` theo hash → thu hồi token cũ (`rotated`, `replaced_by`) → chèn token mới **cùng họ, giữ hạn của họ** (refresh không kéo dài phiên quá 30 ngày). Role đọc lại từ DB |
| Dùng lại token đã xoay | Thu hồi **cả họ** (`reuse_detected`), commit, ghi `audit_logs` `REFRESH_TOKEN_REUSE`, trả 401. Chủ thật và kẻ trộm đều phải đăng nhập lại |
| `POST /api/v1/auth/logout` | Thu hồi họ chứa token. Token lạ vẫn 204 (không cho dò token) |
| `POST /api/v1/auth/logout-all` (cần access token) | Thu hồi mọi họ của identity |
| Đặt lại mật khẩu thành công | Thu hồi mọi họ (`password_reset`) |

Phản hồi đăng nhập/refresh: `access_token`, `token_type: "Bearer"`, `expires_in` (giây), `refresh_token`,
`refresh_expires_in`. Giữ trường `token` (= `access_token`) để web hiện tại không vỡ.

**Đánh đổi có chủ đích:** thu hồi chặn refresh ngay lập tức, nhưng access token đã cấp vẫn dùng được tới khi hết hạn
(≤ 15 phút). Gateway không phải tra DB/Redis mỗi request; nếu cần thu hồi tức thì (khoá tài khoản) thì thêm danh sách
chặn `jti` ở gateway sau.

## 4. Tiêu chí nghiệm thu

| AC | Nội dung |
|---|---|
| AC-1 | Access token sống đúng 15 phút, có `iss/aud/typ/jti`; JWT kiểu cũ (24 giờ, không `iss`) bị từ chối |
| AC-2 | Xác minh chỉ nhận HS256 đúng secret; `alg=none`, HS512, sửa payload, sai `typ` đều bị từ chối |
| AC-3 | Refresh cấp cặp token mới, thu hồi token cũ, không kéo dài hạn của phiên |
| AC-4 | Dùng lại token đã xoay → thu hồi cả họ + audit; đúng **dưới đồng thời** (N request cùng token → đúng 1 thắng) |
| AC-5 | Logout thu hồi đúng phiên đó, không đụng phiên thiết bị khác; logout-all và đặt lại mật khẩu thu hồi mọi phiên |
| AC-6 | DB chỉ lưu hash; refresh token hết hạn (30 ngày) hoặc không tồn tại → 401 |
| AC-7 | Refresh đọc lại role từ DB (đổi quyền có hiệu lực ở lần refresh kế tiếp) |

## 5. Ràng buộc và việc để lại

- **Số migration:** dùng `0003` vì `0002_outbox_relay.sql` thuộc PR #55 (đang draft). Goose mặc định báo lỗi
  "missing migration" nếu `0002` vào **sau** `0003` trên DB đã chạy. Thứ tự merge an toàn: PR #55 trước, hoặc đổi
  số khi rebase.
- **Phụ thuộc:** identity giờ `require libs/go` (cho `authx`). Build độc lập (`GOWORK=off`) cần nâng
  `google.golang.org/api` v0.81 → v0.288 và `genproto`/`compute` để hết lỗi "ambiguous import" với grpc mới mà
  libs/go kéo theo. `idtoken.Validate` không đổi chữ ký.
- **Web (làn frontend):** `apps/web/src/lib/api/auth.ts` mới đọc `token`. Cần lưu `refresh_token`, tự gọi
  `/auth/refresh` khi nhận 401 hết hạn (single-flight giữa các tab để không tự kích hoạt phát hiện tái sử dụng), gọi
  `/auth/logout` khi đăng xuất. Không làm trong slice này.
- **Dọn bảng:** chưa có job xoá token hết hạn/đã thu hồi quá 30 ngày; gộp vào việc dọn dẹp chung (docs/09 GĐ 2).
