# EV-112 — Truy vết AC → test

Unit: `libs/go/authx/access_test.go` (`A:`), `services/identity/internal/app/session_test.go` (`S:`),
`internal/adapter/http/session_handler_test.go` (`H:`), `internal/adapter/http/rbac_middleware_test.go` (`M:`).
Tích hợp (Postgres 16 thật, `-tags=integration`): `internal/adapter/postgres/refresh_token_repo_integration_test.go` (`I:`).

| AC | Test |
|---|---|
| AC-1 access 15 phút, từ chối JWT cũ | A: `KyRoiXacMinhDuocDungClaims`, `HetHanTraLoiRieng`, `TuChoiTokenGiaMao/jwt cu khong iss` · S: `DangNhapCapAccess15PhutVaRefresh30Ngay` · M: `TuChoiJWTCu24Gio`, `Authenticate_Middleware` |
| AC-2 chỉ HS256 đúng secret | A: `TuChoiTokenGiaMao` (khác secret, alg none, HS512, typ khác, sửa payload, chuỗi rác), `SecretNganBiTuChoi`, `ThieuClaimBatBuocKhongKy` |
| AC-3 xoay vòng, giữ hạn phiên | S: `RefreshXoayVongVaGiuHanCuaPhien` · I: `RotateGiuHoVaHanVaGhiReplacedBy` · H: `RefreshEndpoint_TraCapTokenMoi` |
| AC-4 tái sử dụng → thu hồi họ, đúng dưới đồng thời | S: `DungLaiTokenCuThuHoiCaHo` (kèm audit) · I: `RotateDongThoiChiMotRequestThang` (20 goroutine), `LogoutVaLogoutAll` (token đã logout) · H: `RefreshEndpoint_MaLoi/tai su dung` |
| AC-5 logout / logout-all / đặt lại mật khẩu | S: `LogoutThuHoiPhienNayKhongDungPhienKhac`, `LogoutAllThuHoiMoiPhien`, `ForgotPassword_DatLaiMatKhauThuHoiMoiPhien` · I: `LogoutVaLogoutAll` · H: `LogoutEndpoint_Luon204`, `LogoutAllEndpoint_CanAccessToken` |
| AC-6 chỉ lưu hash; hết hạn/lạ → 401 | S: `DangNhapCapAccess15Phut…` (không có token thô trong repo), `HetHanHoacTokenLaBiTuChoi` · I: `HetHanKhongXoayVaKhongThuHoiHo` · H: `RefreshEndpoint_MaLoi` |
| AC-7 role đọc lại từ DB | S: `RefreshDocLaiRoleTuDB`, `IdentityBiXoaThiRefreshThatBai` |
