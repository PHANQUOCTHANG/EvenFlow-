# EV-196 — Dashboard Grafana có sẵn và Prometheus scrape đủ service (Spec)

| Mục | Giá trị |
|---|---|
| Jira | EV-196 (dashboard Grafana cho metric nghiệp vụ; phần alert đã có ở `alerts.yml`) |
| Kế hoạch | [12-ke-hoach-demo-backend.md](../../12-ke-hoach-demo-backend.md) — việc tùy chọn |
| Branch | `feature/EV-196-grafana-dashboard`, chồng trên `feature/EV-103-tracing-gateway-identity-event` |

## 1. Hiện trạng

Grafana trong compose khởi động rỗng: không datasource, không dashboard, nên trình bày phải dựng tay. Prometheus có 10 job nhưng 4 job (`event`, `gateway`,
`identity`, `notification`) luôn `down`: các service này chưa có `/metrics`, và riêng gateway chủ động chặn `/metrics` ở cổng công khai (đúng, không đổi).

## 2. Thiết kế

- `GET /metrics` thêm cho event, identity, notification (chỉ nằm trong mạng nội bộ; gateway chỉ định tuyến các tiền tố `/api/...` nên không lộ ra ngoài).
- Gateway phục vụ metrics ở cổng riêng `METRICS_ADDR` (mặc định `:9094`) qua `otelx.ServeMetrics`; `prometheus.yml` trỏ `gateway:9094`. Cổng chính vẫn chặn `/metrics`.
- Provisioning Grafana (`deploy/observability/grafana/provisioning`): datasource Prometheus (uid `prometheus`, mặc định) và provider nạp thư mục dashboard.
- Dashboard `eventflow-overview` (14 panel, làm mới 5 s, 15 phút gần nhất), sinh JSON ổn định, chỉ dùng metric có thật:
  - **Tổng quan:** số service sống · oversell guard bị kích (phải 0) · tồn kho lệch Redis/Postgres · số người đang xếp hàng · nhịp admit.
  - **Phòng chờ:** độ dài hàng chờ và nhịp admit theo sự kiện · số leader của controller (1 là đúng; 2 split-brain; 0 không ai thả).
  - **HTTP:** request/giây, p99, lỗi 5xx và 429 theo service.
  - **Ticketing:** hold trả kho/giây, p50/p99 xử lý giữ ghế theo kết quả, pool Postgres.

## 3. Tiêu chí nghiệm thu

| AC | Nội dung |
|---|---|
| AC-1 | Sau `make up`, Grafana (http://localhost:3001) có sẵn datasource và dashboard "EventFlow - Tong quan", không thao tác tay |
| AC-2 | Cả 10 job Prometheus `up` |
| AC-3 | Mỗi panel trả dữ liệu (không lỗi, không rỗng) sau `make demo` |
| AC-4 | `/metrics` không truy cập được qua cổng công khai của gateway |

## 4. Còn hở

- Dashboard chưa có panel AI (`ai_queue_lag_seconds`, `ai_dlq_depth`, `ai_gemini_429_total`...) dù ai-worker đã có metric; alert của chúng đã có trong `alerts.yml`.
- Chưa có panel doanh thu / vé bán / tỉ lệ thanh toán: hệ thống chưa phát metric nghiệp vụ này.
- Series theo sự kiện (hàng chờ, nhịp admit) giữ lại sự kiện đã rời danh sách đến khi controller khởi động lại; vô hại nhưng làm chú giải dài sau nhiều lần chạy demo.
