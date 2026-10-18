# EV-157 — Truy vết AC → test

Viết tắt (trong `services/ai-worker/tests`): `P:` = `mq/test_protocol.py`, `T:` = `mq/test_topology.py`,
`R:` = `mq/test_runtime.py`, `C:` = `test_config.py`, `G:` = `test_gemini_client.py`, `H:` = `test_chat_handler.py`,
`S:` = `test_results.py`, `L:` = `test_logx.py`, `M:` = `test_main.py`,
`I:` = `integration/test_consumers_integration.py` (RabbitMQ 3.13 + Redis 7 thật, `pytest -m integration`).

| AC | Test |
|---|---|
| AC-1 report không làm chat trễ > 5 giây *(Jira)* | I: `ReportNang_KhongLamChatTreQua5Giay[async]`, `ReportNang_KhongLamChatTreQua5Giay[thread]` |
| AC-2 `ai_queue_lag_seconds{queue}` *(Jira)* | R: `Lag_LayTuTimestampAMQP`, `Lag_KhongCoTimestampThiDungEnqueuedAtTrongBody` · I: `LagMetric_BonQueueVaVeKhongKhiTrong` |
| AC-3 pool riêng, prefetch = N, N handler song song | I: `PoolChaySongSongDungSoConsumer`, `ReportNang_*` · M: `BonPoolDungSoConsumer`, `PoolBangKhongVanCoSpecDeRuntimeBoQua` |
| AC-4 khai báo topology, lệch thì dừng | T: `load_ai_topology_ChiLayPhanAI`, `GiuNguyenThamSoQueue`, `ThieuFileThiLoi`, `KhongCoPhanAIThiLoi`, `declare_ThuTuExchangeQueueBinding`, `declare_LoiBocTenDoiTuong` · I: `KhaiBaoTopology_HaiLanVanOK`, `KhaiBaoTopology_LechThamSoThiLoi` · C: `TopologyMacDinh_TrongRepoTroToiDeploy`, `TopologyMacDinh_TrongImageKhongVoLucImport` |
| AC-5 giao thức retry/DLQ khớp Go | P: `attempt_of_KhopGo`, `decide_BangQuyetDinh`, `decide_LoiConCuaPermanentCungVaoDLQ`, `forward_headers_*` · R: `ThanhCong_Ack`, `LoiTamLanDau_ChuyenSangRetryVoiAttempt2`, `LoiTamLanThu3_VaoDLQGiuAttemptVaLyDo`, `LoiVinhVien_VaoDLQNgayLanDau`, `ChuyenTiepLoi_NackRequeueKhongAck` · I: `LoiTam_TuPhucHoiQuaHangTre`, `HetLuot_VaoDLQGiuBodyVaLyDo`, `LoiVinhVien_VaoDLQNgay`, `HeaderAttemptKieuGo_DuocDocDung` · H: `BodyHong_LaLoiVinhVien` |
| AC-6 kết quả chat vào Redis | S: `GhiTruocRoiMoiPublish`, `TiengVietGiuNguyen` · I: `Chat_DauCuoi_KetQuaVaoRedis` |
| AC-7 Gemini thật, 429, token, embed | G: `Generate_TraTokenThatVaGioiHanDauRa`, `Generate_ThieuUsageThiTokenLaNone`, `Generate_QuaTimeoutThiLoi`, `Generate_429ThanhRateLimitedVaTangCounter`, `Generate_Loi4xxKhacKhongPhaiRateLimited`, `Embed_That`, `Embed_MockTatDinhVaBoDau`, `ThatMaThieuKey_TuChoi` · H: `429_ThanhRetryKhongTraFallbackChoKhach`, `LimiterChuaChoPhep_Retry`, `GeminiLoiKhac_KhachVanNhanFallback_KhongHoanToken`, `KhongBietTokenThat_KhongHoan`, `CoTokenThat_HoanPhanThua`, `LanCuoiLimiterVanTuChoi_TraLoiDuPhongVaAck`, `LanCuoi429_CungTraLoiDuPhong`, `ChuaToiLanCuoi_VanRetry` |
| AC-8 tắt êm | R: `DangTat_LoiThiTraLaiHangKhongChuyenTiep`, `DangTat_ChuaXuLyThiTraLaiHangKhongGoiHandler` · I: `TatEm_ViecDangChayXongPhanConLaiQuayLaiHang`, `TatEm_QuaHanThiHuyVaMessageQuayLai` |
| AC-9 anomaly / report vào DLQ có lý do | M: `ChuaCoHandler_VaoDLQVoiLyDo[anomaly/report]` · compose (evidence §3) |
| AC-10 cấu hình fail fast | C: `MacDinhHopLe`, `GiaTriSaiBiTuChoi[*]`, `AnomalyVaReportDuocTat`, `GeminiThatMaThieuKeyThiTuChoi` |
| AC-11 log JSON có `trace_id`, không PII | L: `trace_id_from_traceparent`, `trace_id_from_HongHoacThieu`, `DongLogLaJsonCoNguCanhMessage`, `NgoaiNguCanh_KhongCoTruongMessage`, `KhongBaoGioGhiCauHoi`, `LoiCoStackTrace`, `configure_LogUvicornCungLaJson`, `LogConfigUvicorn_DungFormatterJson`, `KhongGhiColorMessageCuaUvicorn` · R: `Delivery_MangAttemptVaPayload` |
| AC-12 mọi lần gọi Gemini thật qua limiter (cả kiểm duyệt) | `test_guarded.py`: `ChuaDuocPhep_KhongGoiGeminiVaKhongHoan`, `DatChoTruocRoiHoanPhanThua`, `429_HoanHetVaNemLai`, `LoiKhac_KhongHoan`, `KhongBietTokenThat_KhongHoan`, `KiemDuyetThat_DiQuaLimiter`, `KiemDuyetThat_LimiterTuChoi_KhongGoiGemini`, `KiemDuyet_ChuaLanCuoi_LimiterTuChoiThiRetry`, `KiemDuyet_LanCuoiVanKhongGoiDuoc_ChuyenNguoiDuyet`, `KiemDuyet_LanCuoi429_CungChuyenNguoiDuyet`, `KiemDuyetMock_VanChayKhongCanLimiter` |
