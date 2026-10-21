# EV-135 — Truy vết AC → test

Unit: `ticketing/internal/app/check_in_ticket_test.go` (`U:`), `internal/adapter/http/check_in_test.go` (`H:`), `gateway/internal/proxy/gateway_test.go` (`G:`).
Tích hợp Postgres: `ticketing/internal/adapter/postgres/repository_integration_test.go` (`PI:`). Script: `deploy/scripts/demo.sh` bước 5.

| AC | Test |
|---|---|
| AC-1 | U: `ThanhCongDoiTrangThaiVaGhiGio` · H: `MotLanDungMotLan` · demo: check-in lần 1 `CHECKED_IN`, lần 2 409 |
| AC-2 | U: `QuetDongThoiChiMotMayThanhCong` (`-race`) · PI: `UpdateTicketStatus_CheckInDungMotLan` (Postgres 16 thật) |
| AC-3 | H: `ChiOps` · G: `RouteTheoRole` (buyer 403, ops 200, buyer vẫn `verify` được) · demo: khách tự check-in 403 |
| AC-4 | U: `KiemQRLoiThiKhongDoiTrangThai` · H: `QRGiaVaBodyHong` |
