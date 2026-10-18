# EV-153 (EVF-73) — Baseline (Trạng thái trước khi thay đổi)

Chụp lúc: 2026-10-18 12:53 UTC+7, commit gốc `07aabf473be3015e7f19a8cc00417f089d929b28` (nhánh `develop` sau khi merge PR #65 cho task EV-152), working tree sạch.

## 1. Kết quả chạy test hiện tại
- Lệnh thực thi:
  ```bash
  docker run --rm -v evenflow-pkg-mod:/go/pkg/mod -v evenflow-go-build:/root/.cache/go-build -v "${PWD}:/app" -w /app golang:1.26-alpine go test -count=1 -v ./services/payment/...
  ```
- Kết quả: **67 passed, 0 failed** trên toàn bộ các packages của `services/payment`:
  - `internal/adapter/gateway`: 12 test passed (100% pass)
  - `internal/adapter/http`: 14 test passed (100% pass)
  - `internal/adapter/postgres`: 3 test passed (100% pass)
  - `internal/app`: 32 test passed (100% pass, bao gồm initiate, webhook exactly-once, replay inbox)
  - `internal/domain`: 6 test passed (100% pass)
- Thư viện phụ trợ (`libs/go/...`):
  - `libs/go/httpx`: PASS
  - `libs/go/idem`: PASS
  - `libs/go/leader`: PASS
  - `libs/go/loadsignal`: PASS
  - `libs/go/mq`: PASS
  - `libs/go/otelx`: PASS
  - `libs/go/testx`: PASS
- Nhận định: Toàn bộ test hiện có đều xanh 100%. Mọi test fail sau này là lỗi hồi quy (regression) và không được chấp nhận.

## 2. Các test case có thể bị ảnh hưởng
| File | Mục tiêu assert | Ảnh hưởng dự kiến |
|---|---|---|
| `services/payment/internal/app/process_webhook_test.go` | Luồng xử lý webhook exactly-once, chữ ký, replay timestamp | Không được làm gãy, cần tương thích khi đơn đã được tự chữa bởi job đối soát |
| `services/payment/internal/app/replay_inbox_test.go` | Replay payment_inbox khi webhook đến sớm | Không được làm gãy, phối hợp với job đối soát để tránh trùng lặp |
| `services/payment/internal/adapter/http/handler_test.go` | Route POST/GET webhook, xử lý duplicate và error responses | Mở rộng endpoint đối soát `/internal/reconcile` và `/metrics` |
| `services/payment/internal/adapter/postgres/repository_test.go` & `memory_repository_test.go` | Giao dịch DB atomicity, lock `orders FOR UPDATE` | Bổ sung phương thức truy vấn đơn treo (`GetStalePendingOrders`) và hủy đơn an toàn kèm trả kho (`ProcessOrderCancellationTx`) |

## 3. Ngưỡng coverage yêu cầu
- Lines: >= 70%
- Functions/Methods: >= 70%
- Statements: >= 70%
- Đảm bảo 100% tuân thủ 4 Quy tắc Bất biến của EvenFlow.
