# EV-196 — Truy vết AC → kiểm

Unit: `services/event/internal/adapter/http/public_events_test.go` (`E:` `Metrics_KhongCanDangNhap`); gateway: `internal/proxy/gateway_test.go` (`G:` `ChanDuongDanNoiBo`, sẵn có).
Kiểm trên compose: API Grafana và Prometheus (xem evidence.md).

| AC | Kiểm |
|---|---|
| AC-1 | `GET /api/search?query=EventFlow` và `/api/datasources` sau `up` mới (volume Grafana mới) |
| AC-2 | `GET :9090/api/v1/targets`: 10/10 `up` |
| AC-3 | Script gọi `/api/ds/query` cho từng truy vấn của 14 panel: 14/14 có series, 0 lỗi |
| AC-4 | G: `ChanDuongDanNoiBo` (`/metrics` → 404); E: `/metrics` của event trả 200 ở cổng nội bộ |
