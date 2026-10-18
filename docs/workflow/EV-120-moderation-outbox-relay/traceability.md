# EV-120 (bugfix) — Truy vết AC → test

Viết tắt: `A:` = `services/event/internal/app/handle_ai_verdict_test.go`, `H:` = `services/event/internal/adapter/http/handler_test.go`,
`W:` = `services/event/internal/worker/outbox_relay_test.go`, `WI:` = `…/worker/outbox_relay_integration_test.go`,
`PI:` = `…/adapter/postgres/event_repo_integration_test.go`, `T:` = `services/ticketing/internal/worker/outbox_relay_test.go`,
`TI:` = `…/ticketing/internal/worker/outbox_relay_integration_test.go`. `I` = Postgres / RabbitMQ thật (`-tags=integration`).

| AC | Test |
|---|---|
| AC-1 routing key `moderation`, payload đúng hợp đồng | A: `TestSubmitForReview_GhiJobKiemDuyetCungTransaction` · H: `TestHTTP_AddTier_And_Submit_BR_E1` · W: `TestDefaultRoutes_ModerationDiAiDirect` · WI (I): `TestTichHop_NopDuyet_JobToiHangKiemDuyet` |
| AC-2 một transaction, không nuốt lỗi | A: `TestSubmitForReview_GhiLoiThiTraLoiVaKhongCoJob` · PI (I): `TestTichHop_UpdateWithOutbox_HongGiuaChungThiKhongGhiGi` |
| AC-3 relay event: lọc, confirm, đánh dấu, property | WI (I): `TestTichHop_NopDuyet_JobToiHangKiemDuyet` · W: `TestNewOutboxRelay_CauHinhSaiThiLoi`, `TestDefaultOutboxRelayConfig_ChiLayDongCuaEvent` |
| AC-4 key không có tuyến không chặn dòng sau | WI (I): `TestTichHop_RoutingKeyLa_KhongChanDongSau` |
| AC-5 relay ticketing chỉ lấy `ticketing.*` (kể cả khi payment dùng chung aggregate `order`) | TI (I): `TestOutboxRelay_Integration_KhongChamDongCuaServiceKhac` · T: `TestDefaultOutboxRelayConfig_ChiLayDongCuaTicketing` |
| AC-6 không đói khi nhiều dòng service khác đứng trước | WI (I): `TestTichHop_NopDuyet_JobToiHangKiemDuyet` (12 dòng `order` > Batch 10) |
| AC-7 không phá EVF-39 | `TestNoOversell_UnderExtremeConcurrency`, `TestNoOversell_WithConcurrentExpiry` ×20 (evidence §2) |
