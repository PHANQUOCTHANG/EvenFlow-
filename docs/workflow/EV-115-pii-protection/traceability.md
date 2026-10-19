# EV-115 — Truy vết AC → test

Unit: `libs/go/pii/pii_test.go` (`P:`), `services/identity/internal/app/pii_test.go` (`A:`),
`internal/app/register_test.go` (`R:`), `internal/config/config_test.go` (`C:`).
Tích hợp (Postgres 16 thật, `-tags=integration`): `internal/adapter/postgres/identity_repo_pii_integration_test.go` (`I:`).

| AC | Test |
|---|---|
| AC-1 không còn SĐT thô | I: `KhongLuuSoDienThoaiTho` · P: `MaHoaGiaiMaDungAAD`, `TuChoiBanMaSaiNguCanh` (sai AAD, sửa bản mã, sai khoá, quá ngắn), `HashCoDinhVaPhuThuocKhoa`, `KhoaHong` · R: `successful registration with phone` |
| AC-2 tra cứu nhiều dạng | I: `KhongLuuSoDienThoaiTho` (3 dạng + đăng ký trùng) · P: `NormalizePhoneVN` · A: `SoDienThoaiKhongHopLeBiTuChoi` |
| AC-3 chuyển dữ liệu cũ | I: `ChuyenDuLieuCuVaIdempotent` |
| AC-4 log sạch | P: `ChePIIVaBiMatTrongLog` (DSN, mật khẩu, token, OTP, JWT, email, SĐT, CCCD, lỗi, group), `WithAttrsCungDuocChe`, `Mask` · A: `EmailOTPLogDaCheEmail` |
| AC-5 OTP không vào log | A: `OTPKhongVaoLogChiDiKenhDev`, `KhongCoKenhDevThiKhongInOTP`, `ForgotPassword_OTPKhongVaoLog` |
| AC-6 production bắt buộc khoá | C: `ProductionBatBuocKhoa`, `DevDungKhoaPIIMacDinhHopLe` |
| AC-7 OTP crypto/rand | Kiểm bằng review + staticcheck (hết cảnh báo `SA1019 rand.Seed`) |
