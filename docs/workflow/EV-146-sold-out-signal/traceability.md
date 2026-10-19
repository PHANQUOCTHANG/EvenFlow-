# EV-146 — Truy vết AC → test

Unit: `ticketing/internal/worker/sold_out_watcher_test.go` (`W:`), `waitingroom/internal/adapter/http/handler_test.go` (`H:`),
`internal/domain/position_test.go` (`D:`), web `waiting-room.test.tsx` (`X:`).
Tích hợp Redis thật: `waitingroom/test/soldout/soldout_test.go` (`S:`, Lua join/status/admit), `ticketing/test/concurrency/sold_out_test.go` (`T:`, watcher + Gate + hold thật).

| AC | Test |
|---|---|
| AC-1 | W: `Kho0ConHoldChiNgungThaNguoi`, `HetHanChiSauNhieuLanLienTiepKhongHold` · S: `StatusDoiSangSoldOutChoNguoiChuaVao`, `LobbyCungNhanSoldOut`, `ControllerNgungThaKhiKhoBang0` (cả `sold_out` và `stock_zero`), `StockZeroKhongDoiTrangThaiKhach` · T: `VongDoiTheoKhoVaHold` |
| AC-2 | S: `NguoiDaAdmitVanHoanTatDon`, `StatusDoiSangSoldOut…` (token lạ → UNKNOWN) |
| AC-3 | S: `JoinMoiKhongGhiDanhNhungNguoiCuConCho` · H: `Join_HetVeHanKhongCapToken` |
| AC-4 | W: `KhoCoLaiGoCoNgay` · S: `StatusDoiSangSoldOut…` (gỡ cờ → QUEUED) · T: `VongDoiTheoKhoVaHold` (hold trả kho, hoàn tiền) |
| AC-5 | W: `ChuaNapKhoThiKhongKetLuanHetVe`, `LoiGhiDuocThuLaiVongSau`, `LoiDocKhoKhongGhiCo`, `CheckAllKhongDungLaiKhiMotSuKienLoi`, `SuKienKhongCoHangVeThiBoQua`, `DanhSachHangVeDuocGiuTrongKhoangLamMoi` · T: `ChuaNapKhoKhongBaoHetVe` |
| AC-6 | W: `HetHanChiSauNhieuLanLienTiepKhongHold`, `HoldXuatHienLamLaiBoDem` · T: `HoldThatLamHetKho` |
| AC-7 | X: `EV-146: join lúc hết vé hẳn…`, `EV-137: token cũ bị status từ chối 401…`, `…vẫn 401 thì hiện màn lỗi` · D: `PollInterval_SoldOut…` |
