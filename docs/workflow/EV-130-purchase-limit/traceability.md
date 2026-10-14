# EV-130 — Truy vết AC → test

Unit: `services/ticketing/internal/app/create_hold_bro4_test.go` (`U:`), `create_hold_test.go` (`U-cũ:`),
`get_purchase_limit_test.go` (`U-API:`), `internal/domain/purchase_limit_test.go` (`D:`),
`internal/adapter/http/purchase_limit_test.go` (`H:`), `libs/go/httpx/httpx_test.go` (`X:`).
Tích hợp: `services/ticketing/test/concurrency/purchase_limit_test.go` (`I:`), chạy với `-tags=integration`.

| AC | Test |
|---|---|
| AC-1 số lượng mỗi đơn | U-cũ: `TuChoiTruocKhiDongVaoTonKho` (5 ca BR-O4 số lượng), `BRO4_DungBienGioiHanVanDuocMua` |
| AC-2 hạn mức mỗi người | U: `VuotHanMucKhongCoHoldThiTuChoi` · U-cũ: 3 ca `max_per_identity` · I: `LopChotTrongTransactionTheoTrangThaiDon` (8 trạng thái + đúng biên), `DemDungTheoDinhNghiaBRO4` · D: `OrderStatusSets_DaChotLaTapConCuaChiemHanMuc` |
| AC-3 bấm lại nhận hold cũ | U: `DangGiuGheBamMuaLaiThiNhanLaiHoldCu`, `KhongTraCuuDuocHoldThiBaoLoi` |
| AC-4 đúng dưới đồng thời | I: `ThanhToanXenGiuaKhongVuotHanMuc` (8 tab + thanh toán xen giữa, cửa sổ nới 20 ms), `LopChotTrongTransactionTheoTrangThaiDon`, `ThieuHanMucThiTuChoi` · U: `TruyenHanMucXuongTransaction` |
| AC-5 trả kho, không tính Redis lệch | U: `LopChotTuChoiThiTraKhoVaBaoVuotHanMuc` · I: `LopChotTrongTransaction…` (tồn kho không đổi khi bị từ chối) |
| AC-6 hai `type` 422 | H: `CreateHold_BRO4_Hai422CoTypeRieng` · X: `WriteProblemType_GhiDungType` |
| AC-7 API hạn mức | H: `PurchaseLimit_TraHanMucChoDungKhach`, `KhongCanIdempotencyKey`, `LoiDuocDichDungMa`, `KhongCauHinhThiKhongCoRoute` · U-API: `TinhSoVeConChonDuoc`, `LoiNguonDuocTraNguyen` · D: `MaxSelectable_BRO4` · I: `DemDungTheoDinhNghiaBRO4` |
