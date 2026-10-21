# EV-131 / EV-133 / EV-155 — Truy vết AC → test

Unit: `libs/go/mq/topology_test.go` (`T:`), `libs/go/mq/consumer_test.go` (`C:`), `payment/internal/worker/outbox_relay_test.go` (`P:`),
`ticketing/internal/worker/order_paid_consumer_test.go` (`O:`), `notification/internal/adapter/rabbitmq/consumer_test.go` (`N:`).
Tích hợp: `libs/go/mq/integration_test.go` (`MQ:`, RabbitMQ thật), `payment/internal/worker/outbox_relay_integration_test.go` (`PI:`, Postgres thật).
E2E trên compose: `E2E` (xem evidence.md).

| AC | Test |
|---|---|
| AC-1 | O: `PhatVeTheoOrderID` · E2E bước 10–11 (đơn `ISSUED` sau ≤ 3s, không gọi `issue-tickets`) |
| AC-2 | PI: `ChiPhatDongCuaPayment`, `BrokerLoiGiuThuTuVaThuLai` · P: `PhuMoiRoutingKeyCuaPayment` |
| AC-3 | O: `GiaoLapLaAck` · E2E bước 14 (gọi lại `issue-tickets`: `already_issued=true`, DB 1 vé) |
| AC-4 | MQ: `TopologySuKien_RetryVeDungHangGiuKeyGoc` · C: `RoutingKeyGoc_GiuQuaRetry` · T: `Consumer_RetryQuayVeDungHangGoc` · O: `LoiTamThoiDuocRetry` · N: `TraCuuLoiThiRetry…` |
| AC-5 | O: `LoiVinhVienVaoDLQ` · N: `…KhongTimThayThiDLQ` |
| AC-6 | MQ: `TopologySuKien_…` (publish `ticketing.hold.created` thành công, sink nhận) · T: `Sink_MoiExchangeSuKienCoHangNhanMoiKey` |
| AC-7 | N: `TraEmailKhiMessageKhongMangSan` (payload thật của ticketing và payment), `CoEmailVaTenThiKhongTraCuu` · E2E bước 12 |
| AC-8 | E2E bước 14 (verify 200, QR sửa 1 ký tự → 400 `/problems/tampered-qr`) |
| Đồng bộ | T: `DefinitionsJSON_KhopTopologyCode` (definitions.json khớp code), MQ: `DeclareTopologyThatHaiLan` |
