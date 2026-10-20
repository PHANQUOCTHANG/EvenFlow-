# EV-NEW-2 — Truy vết AC → test

Unit: `ticketing/internal/domain/availability_test.go` (`D:`), `internal/app/orders_test.go` (`A:`),
`internal/adapter/http/order_handler_test.go` (`H:`), gateway `gateway_test.go` (`G:`), web `payment-client.test.ts` (`W:`).
Tích hợp (Postgres 16 + Redis 7 thật, `-tags=integration`): `ticketing/test/concurrency/order_api_test.go` (`I:`).

| AC | Test |
|---|---|
| AC-1 | I: `GetOrderChiChoChuDon` · H: `GetOrder_MaLoi` (chưa đăng nhập, của người khác, không tồn tại, lỗi hạ tầng không lộ nội bộ) |
| AC-2 | H: `GetOrder_TraDonVaDichTrangThaiChoWeb` (8 trạng thái), `GetOrder_VeDaPhatHanhDungKeyCuaWeb` · I: `GetOrderThanhToanVaVe` · A: `GetOrder_ChiDocVeKhiDonDaPhatHanh` |
| AC-3 | I: `HuyHoldTraKhoDungMotLan` (kho PG = quota, drift Redis = 0), `HuyHoldDongThoiChiTraKhoMotLan` (20 goroutine), `HuyHoldDaHetHanKhongTraKhoLanHai` · A: `CancelHold_*` (5 ca, gồm đổi id sang dạng Redis) |
| AC-4 | I: `KhongHuyDuocHoldCuaNguoiKhacHoacDonDaThanhToan` · H: `CancelHold` (409, 404, thiếu `Idempotency-Key`, 401) |
| AC-5 | I: `HuyHoldTraKhoDungMotLan` (giữ lại ngay sau khi huỷ) |
| AC-6 | D: `LevelOf` · A: `GetAvailability_*` · H: `Availability_CongKhaiVaGomRequest`, `…LoiKhongDuocCache`, `…CacheKhongPhinhVoHan` · I: `TonKhoCongKhai` |
| AC-7 | H: `TicketRoutes_ChiChuDonDuocDocVaPhatHanh` (5 ca, gồm fail-closed) |
| Định tuyến | G: `DinhTuyenDungService` (+ `/v1/orders/o1`, `/v1/holds/h1`, `/availability`) |
| Web | W: `getOrderDetails … /v1/orders/{orderId}`, `gửi Authorization khi đã đăng nhập` |
