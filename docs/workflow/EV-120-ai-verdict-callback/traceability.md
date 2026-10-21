# EV-120 (bugfix) — Truy vết AC → test

`services/ai-worker/tests/`: `C:` = `test_config.py`, `M:` = `test_main.py`, `D:` = `test_compose_config.py`.

| AC | Test |
|---|---|
| AC-1 callback đi thẳng event service | M: `test_ConsumerKiemDuyetGoiEventApiBase_KhongQuaGateway` · C: `test_CallbackKiemDuyet_MacDinhDiThangEventService` |
| AC-2 SystemTools vẫn qua gateway | C: `test_CallbackKiemDuyet_MacDinhDiThangEventService` · D: `test_SystemToolsVanDiQuaGateway` |
| AC-3 cấu hình hỏng không khởi động | C: `test_EventApiBaseSaiBiTuChoi` (`""`, không scheme, `ftp://`, không host) |
| AC-4 compose khớp service event | D: `test_CallbackKiemDuyetTroThangEventService_DungCong` |
| AC-5 chạy thật | evidence §2 |
