# EV-113 — Truy vết AC → test

| AC | Test |
|---|---|
| AC-1 | `postgres` (integration) `TestDeviceRepo_IdempotentVaChiLuuHash` · `app` `TestDeviceService_KiemTraFingerprint` |
| AC-2 | `TestDeviceRepo_GioiHanDungDuoiDongThoi` (20 goroutine, max 5 → 5 OK / 15 từ chối / 5 dòng) |
| AC-3 | `app` `TestLogin_GanThietBiTruocKhiCapToken`, `TestSession_DangNhapGanThietBiVaXoayGiuNguyen` · `http` `TestRefreshEndpoint_ChuyenFingerprintVaMaLoiThietBi` · `authx` `TestAccessToken_ThietBiDiTheoToken` |
| AC-4 | `app` `TestSession_RefreshGanHoacDoiThietBi`, `TestSession_RefreshLoiThietBiKhongTieuTokenMoi` · `postgres` `TestRefreshTokenRepo_ThietBiTheoPhien` |
| AC-5 | `gateway` `TestGateway_ChuyenThietBiTuToken` · `authx` `TestPrincipal_ThietBiChiTuGatewayVaPhaiLaUUID` |
| AC-6 | `http` `TestRegisterDevice_ThanhCongVaMaLoi`, `TestRegisterDevice_CanDangNhap`, `TestRegisterDevice_IPLayTuXForwardedFor` |
