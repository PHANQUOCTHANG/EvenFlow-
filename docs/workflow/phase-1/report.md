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
