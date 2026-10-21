# EV-196 — Bằng chứng

Ngày chạy: 2026-10-21. Windows 11, Docker Desktop 28.4.0, Grafana/Prometheus bản `latest` của compose. **Chưa có bằng chứng CI (Linux).**

| Kiểm | Kết quả |
|---|---|
| Prometheus targets | Trước: event, gateway, identity, notification `down`. Sau: 10/10 `up` |
| Grafana provisioning | Datasource `Prometheus` (uid `prometheus`) và dashboard `eventflow-overview` tự nạp |
| Truy vấn 14 panel qua `/api/ds/query` sau `make demo` | 14/14 có dữ liệu, 0 lỗi (hàng chờ/nhịp admit 11 series, leader 22, request/giây 6 service) |
| Unit event (route `/metrics`), `go vet` các module sửa | PASS |
| `make demo` | 37/37, 96 s |
