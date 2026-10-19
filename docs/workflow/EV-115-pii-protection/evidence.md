# EV-115 — Bằng chứng

Ngày chạy: 2026-10-19. Máy: Windows 11, Docker Desktop 28.4.0, Go 1.26.9 (`GOTOOLCHAIN=local`).
Nhánh `feature/EV-115-pii-protection`, base `feature/EV-114-event-route-authorization`.

**Chưa có bằng chứng CI (Linux):** nhánh chưa được đẩy. Mọi con số là chạy cục bộ.

| Kiểm | Kết quả |
|---|---|
| Unit `libs/go/pii` | PASS |
| Unit `services/identity` (`go test -race ./...`) | PASS |
| Tích hợp identity (`-tags=integration`, Postgres 16 thật): `TestIdentityRepoPII*` + `TestRefreshTokenRepo*` | PASS — 6 test |
| `go vet` identity (có và không `-tags=integration`) | PASS |
| `golangci-lint` `libs/go/pii` | 0 issue |
| `golangci-lint` file identity đã sửa | Sạch; còn 2 cảnh báo có sẵn ở `main.go` (`defer db.Close()` + `os.Exit`) |
| Coverage `domain/` + `app/` identity (cổng 70%) | 80,9% |

CI: bước tích hợp identity trong `integration.yml` đếm cả `TestIdentityRepoPII` (fail nếu 0 test PASS).

## E2E cục bộ cả làn bảo mật (EV-112 + gateway + EV-114 + EV-115)

Postgres 16 + Redis 7 trong Docker (đã chạy migration `0001` + `0003`, chèn sẵn một dòng SĐT thô kiểu cũ), chạy thật
`identity`, `event`, `gateway` từ nhánh này; mọi request đi qua gateway `:18080`. Kết quả **25/25 PASS**:

| Nhóm | Kiểm |
|---|---|
| Đăng ký SĐT | request OTP / verify qua gateway; OTP chỉ ở kênh dev (stderr, định danh đã che), không có trong log |
| PII trong DB | Không còn SĐT thô (`phone_hash`, `phone_enc`); dòng cũ được mã hoá lúc khởi động (`converted: 1`) |
| Token | Đăng nhập bằng `+84…` (chuẩn hoá), `expires_in` ≈ 900; `/auth/me` qua gateway |
| Gateway | Không token → 401; giả `X-Identity-Id`/`X-Identity-Role: organizer` không token → 401; `/internal/…/ai-verdict` → 404; buyer + header giả `moderator` gọi approve → 403 |
| Phân quyền event | Đổi role trong DB → refresh trả role `organizer` → tạo sự kiện, `organizer_id` lấy từ token; khách không thấy DRAFT (404), chủ thấy (200); organizer tự duyệt → 403 + audit `ACCESS_DENIED` |
| Tái sử dụng refresh | Dùng lại token cũ → 401, token mới nhất của họ cũng 401, audit `REFRESH_TOKEN_REUSE` |
| Log | Không có mật khẩu DB, SĐT, refresh token trong log identity |
| Rate limit | 14 lần login liên tiếp từ một IP → có 429 |
