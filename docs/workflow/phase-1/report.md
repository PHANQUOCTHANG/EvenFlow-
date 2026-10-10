# Giai đoạn 1 — Thư viện dùng chung

Kế hoạch: [../../09-ke-hoach-hoan-thien-backend.md](../../09-ke-hoach-hoan-thien-backend.md) mục "Giai đoạn 1".

## PR A — `go.sum` + `libs/go/httpx` (nhánh `feat/phase-1a-httpx`)

### Đã làm

| Việc | Thay đổi |
|---|---|
| Track `go.sum` | `libs/go`, `services/ticketing`, `services/waitingroom`. `go mod tidy` bổ sung `// indirect` còn thiếu (nợ #1 của backend-debt) |
| Dockerfile | Copy thêm `go.sum` của module còn lại: workspace gộp `go.sum` của mọi module nên thiếu là `missing go.sum entry` |
| CI | `cache-dependency-path` đổi từ `go.mod` sang `go.sum` (cache theo nội dung dependency) |
| `httpx` | `WriteProblem`/`WriteJSON`, `Chain`/`RequestID`/`AccessLog`/`Recover`, `NewServer`/`Run`/`Serve`, `Health` |
| Áp dụng | Xoá `problem()`/`writeJSON()` trùng ở 2 handler; 2 `cmd/server/main.go` dùng `httpx.Health` + `httpx.Run` |

### Quyết định đáng ghi

- `go.work.sum` **không** phát sinh: mỗi `go.sum` của module đã đủ. Không commit file này.
- Không đổi `go.mod` của 6 service rỗng (`gateway`, `identity`, ...) — `go mod tidy` chỉ đổi định dạng, là nhiễu.
- `AccessLog` chỉ ghi **mẫu route** (`POST /v1/events/{eventID}/holds`), không ghi URL thật, body hay header
  định danh — để log không chứa PII (EVF-15). Probe `/healthz`, `/readyz` ghi ở mức Debug.
- `statusWriter` giữ `http.Flusher`: handler SSE của phòng chờ kiểm `w.(http.Flusher)`, nếu middleware che mất
  thì mọi stream trả 500. Có test riêng và đã kiểm trên stack thật.
- `NewServer` cố ý **không** đặt `WriteTimeout` vì `/queue/stream` là SSE sống lâu.
- `ticketing` trước đây không có `IdleTimeout`; nay dùng chung 120 giây như `waitingroom`.

### Bằng chứng

- `gofmt`, `go vet`, `go test -race`: `httpx` 91.5%; ticketing app 100%, worker 93.2%; waitingroom app 98.3%.
- golangci-lint v2.12.2: 0 issues ở 3 module. Build `-mod=readonly` đạt cho cả 4 binary.
- Trivy `--severity CRITICAL --ignore-unfixed`: 0 ở 4 binary.
- `smoke.sh` 15/15 từ volume trống sau khi build lại image; test workflow 128 PASS.
- Kiểm tay trên stack: `X-Request-Id` hợp lệ được giữ, id sai bị thay; SSE `event: position` vẫn chảy qua chuỗi middleware.

### Chưa làm (cố ý)

- Middleware auth và rate limit — cần `authx`, thuộc giai đoạn 3.
- `libs/go/redisx` vẫn 0% coverage; cổng 70% chưa áp cho `libs/go` (dự kiến mở ở PR C).

## PR B — quan trắc `libs/go/otelx` (nhánh `feat/EVF-3-observability-otelx`)

Xếp chồng lên PR A (chưa merge lúc làm). Rebase lên `develop` khi A merge.

### Đã làm

| Việc | Thay đổi |
|---|---|
| `otelx` | `Init` (trace OTLP, no-op khi không có endpoint), `HTTP` (span + histogram theo mẫu route), `LogHandler` (gắn `trace_id`), `MetricsHandler`/`ServeMetrics`, `StartSpan`, carrier AMQP |
| Metric nghiệp vụ | Tên khớp `alerts.yml`: `ticketing_oversell_guard_rejections_total`, `ticketing_hold_duration_seconds{outcome}`, `ticketing_holds_released_total`, `ticketing_db_pool_usage`, `waitingroom_admit_rate{event}`, `waitingroom_queue_depth{event}` |
| Nối vào service | Metric đi qua port (`HoldMetrics`, `SweeperMetrics`, `AdmitMetrics`) với no-op mặc định → constructor và test cũ không đổi |
| Trace | Span con `redis.hold`, `redis.release`, `postgres.persist_hold`, `postgres.release_hold` |
| Hạ tầng | Controller `:9091` và worker `:9092` mở cổng `/metrics`; Prometheus có 2 job mới và **được mount `alerts.yml`** (trước đó compose không mount nên luật alert không được nạp) |

### Lỗi tìm thấy khi kiểm trên stack thật

- **Mọi route bị gắn nhãn `unmatched`.** `ServeMux` ghi `r.Pattern` lên request nó nhận; `RequestID` tạo bản sao
  (`WithContext`) nên `otelx.HTTP` đứng ngoài `RequestID` không bao giờ thấy pattern. Unit test gọi `HTTP` thẳng lên
  mux nên không bắt được. Sửa: `otelx.HTTP` đứng **ngay sau** `RequestID`; thêm test dùng đúng chuỗi middleware thật và
  ghi ràng buộc vào doc của `Chain` và `HTTP`.
- `/metrics` bị Prometheus scrape mỗi 5 giây làm ngập log INFO → hạ xuống Debug cùng probe sức khoẻ.

### Quyết định

- **Metric dùng thẳng `client_golang`**, không qua OTel metrics (tên đã chốt trong `alerts.yml`, ít dependency).
- **Dependency ghim ở bản còn build được với Go 1.25** (`otel v1.46.0`, `client_golang v1.24.1`): bản mới nhất đều đòi
  Go 1.26. Không nâng toolchain trong PR này.
- Nhãn `route` là mẫu route, không phải URL thật: tránh nổ cardinality và không đưa id vào metric/trace.
- Mặc định lấy mẫu 100% (`parentbased_always_on`). **Trước mở bán phải đặt `OTEL_TRACES_SAMPLER=parentbased_traceidratio`
  với tỉ lệ thấp** — nếu không, 300k rps sẽ tạo 300k span/giây.
- `go.work.sum` được commit (3 dòng) và copy vào image.

### Bằng chứng

- `gofmt`, `go vet`, `go test -race`: `otelx` 73.2%, `httpx` 92.0%; adapter metrics 100%; ticketing app 100%, worker 93.5%;
  waitingroom app 98.6%.
- golangci-lint v2.12.2: 0 issues ở 3 module. Build `-mod=readonly`: đạt 4 binary.
- Smoke 15/15 từ stack có Jaeger + Prometheus.
- Prometheus: 4/4 target của Go ở trạng thái up (`ai-worker` down vì không chạy trong stack này), 6 luật alert đã nạp.
- Jaeger: trace `POST /v1/events/{eventID}/holds` có span con `redis.hold` và `postgres.persist_hold`; log có `trace_id`.
- Trivy CRITICAL: 0 ở 4 binary.

### Nợ / chưa làm

- **Còn 5 lỗ hổng HIGH, chưa vá được trên Go 1.25:** `golang.org/x/net` (CVE-2026-78669, bản vá `v0.60.0` đòi Go 1.26) và 3 CVE
  stdlib (vá ở Go 1.26.9). Cổng CI chỉ chặn CRITICAL nên không đỏ. Nâng lên Go 1.26 là việc riêng, nên làm trước khi phát hành.
- Tiêu chí "một trace xuyên ≥ 3 service" của E1 chưa đạt: hai service hiện có chưa gọi nhau (cần gateway, giai đoạn 3).
- `ai-worker` chưa xuất metric trên cổng 8090 nên target `ai-worker` đang down.
- `/metrics` mở trên cổng dịch vụ: gateway (giai đoạn 3) không được route ra ngoài.
- Metric `ticketing_holds_released_total` xuất hiện ở cả `ticketing` (luôn 0) lẫn `ticketing-worker`; khi vẽ dashboard hãy `sum()` hoặc lọc theo job.
- `HealthProbe` thật cho controller (giai đoạn 5) sẽ đọc p99 từ histogram và pool từ gauge ở trên; chưa nối.
