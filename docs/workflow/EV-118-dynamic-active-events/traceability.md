# EV-118 — Truy vết AC → test

Unit: `ticketing/internal/worker/event_discovery_test.go` (`D:`), `waitingroom/internal/app/event_supervisor_test.go` (`S:`),
`event/internal/worker/sale_opener_test.go` (`O:`), `event/internal/domain/event_lead_test.go` (`L:`),
`event/internal/adapter/http/public_events_contract_test.go` (`P:`, từ PR #85),
`gateway/internal/proxy/gateway_test.go` (`G:`), `ai-worker/tests/test_config.py` (`A:`).
Tích hợp: `ticketing/internal/adapter/{postgres,redis}/active_events_integration_test.go` (`TI:`),
`event/internal/adapter/postgres/event_repo_integration_test.go` (`EI:`), `libs/go/activeevents/integration_test.go` (`AI:`).
Kịch bản trên compose: `ORG` (organizer → khách mua), xem evidence.md.

| AC | Test |
|---|---|
| AC-1 | D: `ThemSuKienMoiCongVoiDanhSachCoDinh`, `Seeder_DungDanhSachDongMoiVong` · TI: `ListSellableEvents_LocTheoTrangThaiVaKhoangNhinTruoc`, `PublishActiveEvents_GhiTapVaT0` · AI: `ReplaceThayTrongTapVaReadSapXep` · S: `ThemVaBotSuKienTheoDanhSach` · ORG bước 6 (2 giây) |
| AC-2 | ORG bước 7–9 (`LOBBY` → lottery 1 người → `ADMITTED` → hold 200) · hồi quy `waitingroom/test/leader`, `test/soldout` |
| AC-3 | EI: `OpenDueSales_ChiMoSuKienDenGio` · O: `QuetNgayVaTheoChuKyKeCaKhiLoi` · ORG bước 8 (`ON_SALE`) |
| AC-4 | D: `SuKienHetBanRoiDanhSach` · S: `ThemVaBotSuKienTheoDanhSach`, `RunDungThiDungHet`, `GoroutineThoatThiChayLai` · ORG: huỷ → rời `wr:active_events`, log "dung dieu khien" |
| AC-5 | D: `PostgresLoiGiuDanhSachCuKhongGhiRedis`, `RedisLoiVanCapNhatDanhSachTrongWorker` · S: `DocDanhSachLoiGiuNguyen` |
| AC-6 | TI: `PublishActiveEvents_GhiTapVaT0` (chưa có / tương lai / đã qua / không T0) |
| AC-7 | ORG đăng nhập 3 vai · kiểm tay 4 tài khoản 200, sai mật khẩu 401 |
| AC-8 | P: `PublicEvents_KhongCanTokenVaChiTraTrangThaiCongKhai` · `event/internal/adapter/postgres/list_public_integration_test.go` (PR #85) · G: `DinhTuyenDungService` |
| AC-9 | L: `KhoangBRE2CauHinhDuoc` |
| AC-10 | A: `KiemDuyetGoiThangEventServiceKhiCauHinh` · ORG bước 3 (`AUTO_APPROVED` sau 1 giây) |
