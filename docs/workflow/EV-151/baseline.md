# EV-151 — Baseline (Trạng thái trước khi thay đổi)

Chụp lúc: 2026-10-17, commit `c241ea152fc1a728883f1155b9c0f0afc837a1a6` (nhánh `feature/EV-151-webhook-exactly-once`, tách từ `develop`), working tree sạch.

## 1. Kết quả chạy test hiện tại
- Môi trường: Docker Engine 28.4.0 / Linux container trên Go 1.26 (`golang:1.26-alpine`), Go workspace `go.work`.
- Lệnh kiểm tra:
  ```bash
  docker run --rm -v evenflow-pkg-mod:/go/pkg/mod -v evenflow-go-build:/root/.cache/go-build -v "${PWD}:/app" -w /app golang:1.26-alpine go test ./services/payment/... ./libs/go/httpx/... ./libs/go/idem/...
  ```
- Kết quả chạy thực tế:
  - `github.com/eventflow/eventflow/services/payment/internal/adapter/gateway`: `PASS` (11 test cases)
  - `github.com/eventflow/eventflow/services/payment/internal/adapter/http`: `PASS` (7 test cases)
  - `github.com/eventflow/eventflow/services/payment/internal/adapter/postgres`: `PASS` (1 test case)
  - `github.com/eventflow/eventflow/services/payment/internal/app`: `PASS` (10 test cases)
  - `github.com/eventflow/eventflow/services/payment/internal/domain`: `PASS` (5 test cases)
  - `github.com/eventflow/eventflow/libs/go/httpx`: `PASS`
  - `github.com/eventflow/eventflow/libs/go/idem`: `PASS`
- Tổng số test hiện có trong `services/payment`: **27 passed, 0 failed**.
- Nhận định: Toàn bộ 27 test cases của task EV-150 và các module hạ tầng dùng chung (`httpx`, `idem`) đều xanh 100%. Mọi test fail sau này là lỗi hồi quy.

## 2. Các test case có thể bị ảnh hưởng
| File | Mục tiêu assert | Ảnh hưởng dự kiến |
|---|---|---|
| `services/payment/internal/adapter/gateway/*_test.go` | Xử lý verify callback và initiate payment của 4 cổng | Không được làm gãy |
| `services/payment/internal/adapter/http/handler_test.go` | Route POST `/v1/events/{id}/payments`, GET methods | Không được làm gãy |
| `services/payment/internal/app/initiate_payment_test.go` | Luồng khởi tạo thanh toán và kiểm tra hold expired | Không được làm gãy |
| `services/payment/internal/domain/payment_test.go` | Validation domain và che giấu PII | Không được làm gãy |
| `libs/go/httpx/httpx_test.go` | Chuẩn hóa RFC 7807 problem details | Không được làm gãy |

## 3. Ngưỡng coverage yêu cầu
- `domain/` + `app/`: Lines >= 80%, Functions >= 80%, Branches >= 70%
- `adapter/http/`: Lines >= 75%
- Concurrency test: 100 webhook đồng thời xử lý đúng 1 đơn `PAID`, 0 duplicate outbox event.
- Thời gian chạy toàn bộ test suite < 3 giây, 0 dependency mạng bên ngoài.
