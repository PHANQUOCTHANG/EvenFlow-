# EV-142 — Truy vết AC → test

Viết tắt: `L:` = `libs/go/leader`, `S:` = `libs/go/loadsignal`, `A:` = `services/waitingroom/internal/app`
(`admit_controller_leader_test.go`, `admit_controller_test.go`), `C:` = `internal/config`, `M:` = `internal/adapter/metrics`,
`P:` = `internal/adapter/signal`, `T:` = `services/ticketing/cmd/server`, `W:` = `services/waitingroom/test/leader`.
`I` = tích hợp trên Redis thật (`-tags=integration`).

| AC | Test |
|---|---|
| AC-1 giảm ≤ 10 giây | A: `Adjust_TieuChiNghiemThu_GiamTrong10sPhucHoiTrong60s`, `Tick_BackpressureDecreasesRate` |
| AC-2 phục hồi ≤ 60 giây | A: `Adjust_TieuChiNghiemThu_GiamTrong10sPhucHoiTrong60s`, `Adjust_ODuoiSan_KhongBatCuaSo`, `Tick_RecoversAfterBackpressureClears` |
| AC-3 không dao động | A: `Adjust_KhongDaoDongKhiSucEpKhongDoi`, `Adjust_SauMoiLanGiamThiGiuMotCuaSo`, `Tick_RateAlwaysWithinBoundsUnderMixedSignals` · C: `AdmitConfig_DecreaseCooldown` |
| AC-4 một leader, leader cũ bị chặn | L (I): `LuaKhoaVaEpoch`, `ChiMotLeaderTaiMoiThoiDiem`, `LeaderCuDungHinhBiFencingChan` · W (I): `Fencing_AdmitCuBiChan`, `Fencing_ChuaCoLeaderThiMoiEpochBiChan`, `Fencing_ShuffleCuBiChan_MoiChiMotLan`, `Fencing_NhipCuBiChanVaDocLaiDuoc` · L: `Run_KhongLaLeaderThiKhongChayViec`, `Run_MatKhoaThiHuyViecVaKhongNhaKhoaCuaNguoiKhac` |
| AC-5 hai controller | W (I): `HaiController_ChiMotLeaderThaNguoi` |
| AC-6 leader chết / tắt êm | L (I): `LeaderChetThiNguoiKhacLenSauTTL`, `TatEmThiFailoverNhanh` · L: `Run_TatEm_NhaKhoaDeFailoverNgay`, `Run_NhanEpochTangDanMoiNhiemKy`, `Run_LoiGiaHan_ThoangQuaThiGiu_KeoDaiThiThoaiVi` · W (I): `LeaderTatEm_NguoiKhacTiepQuan` |
| AC-7 leader cũ đứng hình | W (I): `LeaderCuDungHinh_ControllerDungNgayKhongThaThem` · A: `Run_BiFencingKhiAdmit_ThoatNgayVaKhongThaThem`, `Run_BiFencingKhiGhiNhip_CungThoaiVi`, `Run_BiFencingKhiShuffle_KhongTiepTucVaoVongAdmit`, `Run_LoiThuong_VanChayTiep` |
| AC-8 tiếp tục nhịp | A: `Restore_TiepTucNhipCuCuaLeaderTruoc` · W (I): `DoiLeader_TiepTucNhipVaGiuTamDungCuaOps` |
| AC-9 ghi đè của Ops | A: `SyncOverride_TamDungKhanCapSongSotQuaLeaderMoi`, `SyncOverride_OpsBoGhiDe_TraQuyenChoAIMD`, `SyncOverride_StoreKhongGoGhiDeTrongRAM`, `SyncOverride_LoiDoc_GiuNguyenTrangThai`, `SyncOverride_GiaTriGhiDeBiChanTaiTran` · W (I): `Override_DatVaBo`, `DoiLeader_TiepTucNhipVaGiuTamDungCuaOps` |
| AC-10 không đo được = sức ép | P: `Probe_KhongCoTinHieu_LaLoiChuKhongPhaiKhoe`, `Probe_PodBaoCaoKhongYeuCau_LaKhoe` · A: `Tick_UnknownHealthIsTreatedAsBackpressure` · S (I): `AnhChupCuBiBoQua_KhongPodThiErrNoSignal`, `GopPodTeNhat`, `PublisherToiReader` · S: `Recorder_*` · T: `SignalMetrics_ChiTinhYeuCauChamTangDuoi` · C: `HealthConfig` |
| AC-11 mất key epoch | L (I): `MatKeyEpochKhongLamEpochQuayLai` |
| AC-12 quan sát | M: `Prom_LeaderVaAdjust` · A: `Adjust_XuatLyDoMoiTick` · `deploy/observability/alerts.yml` (`AdmitLeaderSplit`, `AdmitNoLeader`) · C: `LeaderConfig` |
