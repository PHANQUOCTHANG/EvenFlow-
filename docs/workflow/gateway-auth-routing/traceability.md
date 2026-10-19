# EV-NEW-1 (gateway) — Truy vết AC → test

Unit: `services/gateway/internal/proxy/gateway_test.go` (`G:`, gateway thật + upstream giả bằng `httptest`),
`internal/ratelimit/limiter_test.go` (`R:`), `internal/config/config_test.go` (`C:`),
`libs/go/authx/principal_test.go` (`P:`).

| AC | Test |
|---|---|
| AC-1 xoá danh tính giả | G: `XoaDanhTinhGiaMaoTrenMoiRoute` (route công khai + cần đăng nhập, X-Forwarded-For giả) · P: `StripXoaMoiHeaderGiaDanh`, `SetGhiDeGiaTriCuaClient`, `FromHeadersTuChoiGiaTriSai` |
| AC-2 401 khi token không hợp lệ | G: `RouteCanDangNhapTuChoiTokenKhongHopLe` (6 ca, kiểm upstream không bị chạm), `RouteCongKhaiGanDanhTinhNeuTokenHopLe` |
| AC-3 chặn nội bộ | G: `ChanDuongDanNoiBo` (`/internal/*`, `/metrics`, `/admin/*`, path lạ, `..`) |
| AC-4 role | G: `RouteTheoRole` |
| AC-5 định tuyến | G: `DinhTuyenDungService` (18 endpoint web đang gọi) |
| AC-6 rate limit | G: `GioiHanTocDo` · R: `ChoDotBienRoiChan`, `NapLaiTheoThoiGian`, `KhoaDocLap`, `DonBucketNhanRoi` · C: `DocBienMoiTruong`, `TuChoiCauHinhHong` |
| AC-7 CORS | G: `CORS` |
| AC-8 SSE / 502 / 413 | G: `SSEDuocDayNgay`, `UpstreamSapTra502`, `GioiHanKichThuocBody`, `SecurityHeaders` |
| Cấu hình | C: `MacDinhChoDev`, `ProductionBatBuocJWTSecret` |
