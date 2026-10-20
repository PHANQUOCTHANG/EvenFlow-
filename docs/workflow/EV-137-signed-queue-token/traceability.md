# EV-137 — Truy vết AC → test

| AC | Test |
|---|---|
| AC-1 | `authx` `TestQueueToken_KyRoiXacMinh`, `HetHanTraLoiRieng`, `TuChoiTokenGiaMao` (9 ca), `KhongLanVoiLoaiTokenKhac`, `Matches`, `KyTuChoiThieuClaim` |
| AC-2 | `waitingroom/http` `TestJoin_CapQueueTokenKy` · integration `TestTichHop_QueueTokenKy_JoinStatusVaChiaSe` (Redis chỉ giữ jti) |
| AC-3 | `waitingroom/http` `TestStatusVaStream_XacMinhQueueToken` (8 ca × status + stream) · `ticketing/http` `TestCreateHold_QueueTokenSaiBiTuChoiTruocKhiChamUseCase` (8 ca) · integration `TestTichHop_QueueToken_KhongDungChoSuKienKhac`, `…AdmitTheoJTI` |
| AC-4 | integration `TestTichHop_QueueTokenKy_JoinStatusVaChiaSe` (alice laptop → phone: 1 chỗ, 2 token) |
| AC-5 | `waitingroom/http` `TestJoin_CanThietBiNeuBatBuoc` |
| AC-6 | `waitingroom/http` `TestJoin_ThieuSecretThiTuChoi` · `ticketing/http` `TestCreateHold_ThieuSecretThiTuChoi` · `authx` `TestQueueTokenSecretFromEnv` |
| Hợp lệ | `ticketing/http` `TestCreateHold_QueueTokenKyHopLeDuocChapNhan`, `TestCreateHold_ThieuDanhTinhLa401` · web `lib/device.test.ts` |
