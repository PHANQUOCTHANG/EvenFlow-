# EV-171 — Truy vết AC → test

`U:` = `services/ai-worker/tests/test_assistant_api.py`, `I:` = `tests/integration/test_assistant_integration.py`
(Redis + RabbitMQ thật), `M:` = `tests/test_main.py`.

| AC | Test |
|---|---|
| AC-1 202 < 50 ms, không gọi Gemini | I: `Post202Duoi50ms_p95` · U: `Post_202VaXepHangDungHopDongChatJob` (chỉ publish, không có Gemini trong đường đi) |
| AC-2 vượt quota → FAQ + CSKH | U: `Post_VuotQuota_TraFaqVaCSKH_KhongXepHang` · I: `Quota_10QuaCau11BiChan` |
| AC-3 quota nguyên tử, cửa sổ trượt | I: `Quota_10QuaCau11BiChan` (kèm TTL), `Quota_CuaSoTruot`, `Quota_50YeuCauSongSong_Chi10Qua` |
| AC-4 GET + quyền chủ job | U: `Get_ChuaXong_202Pending`, `Get_Xong_200KemKetQua`, `Get_KhongPhaiChu_404`, `Get_KhongTonTai_404` · I: `DauCuoi_PostRoiGetRoiStream` |
| AC-5 SSE: trước / sau kết quả, quá hạn | I: `DauCuoi_PostRoiGetRoiStream` (sau), `Stream_MoTruocKhiCoKetQua` (trước, có heartbeat), `Stream_QuaHan_TraLoiDuPhong` · U: `Stream_KhongPhaiChu_404` |
| AC-6 nấc OFF | U: `Post_NacOFF_KhongXepHangKhongTruQuota` |
| AC-7 401 / 422 / 400 RFC 7807 | U: `Post_ThieuIdentity_401`, `Post_CauHoiKhongHopLe_422`, `Post_BodyHong_LoiRFC7807`, `Post_BrokerSap_503KhongLoLoi` |
| AC-8 metric, log không có câu hỏi | `/metrics` trên compose (evidence §2) · M: `ApiTroLyDuocGanVaoApp` · I: `Publisher_MessageDungProperty` |
