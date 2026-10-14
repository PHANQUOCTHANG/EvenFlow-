# EV-132 — Truy vết AC → test

Unit: `services/ticketing/internal/worker/inventory_reconciler_test.go` (tên viết tắt `U:`).
Tích hợp: `services/ticketing/test/concurrency/reconcile_test.go` (`I:`), chạy với `-tags=integration`.

| AC | Test |
|---|---|
| AC-1 sửa trong ≤ 20 giây | U: `RedisThieu_SuaSauHaiChuKy_MetricVe0`, `RedisPhong_BotDi`, `LuotXacNhan_SuaNgayTrongMotChuKy` · I: `LamLechRedis_TuSuaTrongHaiChuKy`, `MotChuKyVoiLuotXacNhan_SuaNgayKhongCanChuKySau` · `smoke.sh` bước 9 |
| AC-2 metric về 0 | U: `RedisThieu_SuaSauHaiChuKy_MetricVe0`, `LoiSua_GiuDoLechDeThuLai` · `prom_test.go`: `MetricDoiSoat_DungTenVaVeKhong` |
| AC-3 hold bay / chờ release không phải lệch | U: `HoldDangBay_KhongPhaiLech`, `HoldDaGhiPostgres_HaiTangCungTru`, `PostgresDaTraRedisChuaKip_KhongPhaiLech`, `HoldDaConsume_KhongPhaiLech`, `LechThatCongHoldDangBay_ChiSuaPhanLech` · I: `HoldDangBay_KhongBiCoiLaLech`, `PostgresDaTraRedisChuaKip_KhongBiCoiLaLech`, `VoiHoldThat_ChiSuaDoLechInject_KhongPhaHold` |
| AC-4 nhiễu không thành phép sửa | U: `NhieuThoangQua_KhongBaoGioDuocSua`, `DoiDau_KhongSua`, `LuotXacNhan_NhieuThoangQuaBienMat_KhongSua`, `HaiLanLechKhacNhau_SuaPhanChacChan`, `LechTangDan_ChiSuaGiaTriNhoHon…` |
| AC-5 delta, không âm | U: `RedisPhongKhongBaoGioXuongAm` · I: `ScriptKhongDeTonKhoXuongAm` |
| AC-6 hai instance | U: `KhoaDoiSoatDoInstanceKhac_BoQua` · I: `HaiInstanceSongSong_KhongSuaGapDoi` |
| AC-7 Redis bị xoá | I: `KhoRedisBiXoaHet_DuocNapLai` |
| AC-8 lỗi | U: `LoiDocRedis_KhongSuaVaTraLoi`, `LoiDocPostgres_KhongSua`, `LoiSua_GiuDoLechDeThuLai`, `SuKienKhongCoTonKho_KhongSuaGi`, `ReconcileAll_LoiMotSuKienKhongChanSuKienKhac`, `LuotXacNhan_HuyCtxThiThoat` |
