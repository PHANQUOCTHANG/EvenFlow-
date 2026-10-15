# EV-150 — Baseline (Trạng thái trước khi thay đổi)

Chụp lúc: 2026-10-15, commit `0345684d3592d335cb1292bb6e5e36ca2b9c8232` (nhánh `feature/EV-150-payment-gateway-adapters`, tách từ `develop`), working tree sạch.

## 1. Kết quả chạy test hiện tại
- Môi trường: Docker Engine 28.4.0 / Linux container trên Go 1.26 (`golang:1.26-alpine`), Go workspace `go.work`.
- Lệnh kiểm tra:
  ```bash
  docker run --rm -v evenflow-pkg-mod:/go/pkg/mod -v evenflow-go-build:/root/.cache/go-build -v "${PWD}:/app" -w /app golang:1.26-alpine go test ./services/payment/... ./libs/go/httpx/... ./libs/go/idem/...
  ```
- Kết quả:
  - `github.com/eventflow/eventflow/services/payment/cmd/server`: `[no test files]` (mã nguồn `services/payment` hiện tại mới chỉ là khung ban đầu với `/healthz`, chưa có unit test nào).
  - `github.com/eventflow/eventflow/libs/go/httpx`: `PASS` (0.319s).
  - `github.com/eventflow/eventflow/libs/go/idem`: `PASS` (0.012s).
- Nhận định: Toàn bộ test của các module hạ tầng liên quan (`httpx`, `idem`) đều xanh 100%. Mọi test fail sau này là lỗi hồi quy.

## 2. Các test case có thể bị ảnh hưởng
| File | Mục tiêu assert | Ảnh hưởng dự kiến |
|---|---|---|
| `libs/go/httpx/httpx_test.go` | RFC 7807 problem details, middleware | Không được làm gãy |
| `libs/go/idem/middleware_test.go` | Idempotency middleware (BR-O5) | Không được làm gãy |
| `apps/web/src/lib/payment-client.ts` | Hợp đồng API thanh toán client frontend | Phải tương thích chuẩn 100% |

## 3. Ngưỡng coverage yêu cầu
- `domain/` + `app/`: Lines >= 80%, Functions >= 80%, Branches >= 70%
- Toàn bộ unit tests chạy nhanh < 2 giây, 0 dependency mạng bên ngoài.
