# EV-159 — Truy vết AC → test

`P:` = `services/ai-worker/tests/mq/test_protocol.py`, `C:` = `tests/test_classify_real_errors.py`,
`R:` = `tests/mq/test_runtime.py`, `I:` = `tests/integration/test_consumers_integration.py` (broker thật),
`A:` = `deploy/observability/alerts_test.yml` (`promtool test rules`).

| AC | Test |
|---|---|
| AC-1 lỗi tạm thời tự phục hồi | P: `is_permanent_LoiTamThoiThiThuLai[408/425/429/5xx]`, `is_permanent_ConLaiLaTam` · C: `httpx_HTTPStatusError[429/503]`, `genai_ClientError[429]`, `genai_ServerError_LaTam`, `httpx_MatKetNoi_LaTam`, `LimiterVa429_LaTam` · I: `LoiTam_TuPhucHoiQuaHangTre` |
| AC-2 lỗi vĩnh viễn vào DLQ ngay và bắn alert | I: `KiemDuyet_Callback404_VaoDLQNgayVaDoSauDLQ` · P: `decide_LoiVinhVienTuThuVienVaoDLQNgay` · A: `AiDeadLetterQueueNotEmpty` |
| AC-3 bảng phân loại | P: `is_permanent_Http4xx`, `is_permanent_BodyHongLaVinhVien`, `is_permanent_CodeKhongPhaiSoThiBoQua`, `is_permanent_LoiBocTrongCause` · C: toàn bộ |
| AC-4 metric DLQ + header | R: `DLQ_DemTheoLyDo_VinhVien`, `DLQ_DemTheoLyDo_HetLuot`, `DLQ_ChuyenTiepLoi_KhongDem`, `DLQ_GhiLoaiLoiVaoHeader`, `Sampler_DoSauDLQ` · I: `KiemDuyet_Callback404_…` |
| AC-5 alert | A: 3 case (`AiDeadLetterQueueNotEmpty` im ở 30s / kêu ở 105s; `AiDeadLetterSpike` kêu với 20/phút, im với 1/phút) |
| AC-6 CI kiểm alert | `ci.yml` `validate-deploy` + filter `deploy/observability/**`; `test-workflows.sh` 137/137 |
