# EV-154 (EVF-74) — Baseline (Trạng thái trước khi thay đổi)

Chụp lúc: 2026-10-19 11:12 UTC+7, commit gốc `f2154d829e80df2d795864a4a28e94547baff0b4` (nhánh `develop` sau khi merge PR #72 cho task EV-133), working tree sạch.

## 1. Kết quả chạy test hiện tại
- Lệnh thực thi:
  ```bash
  docker run --rm -v "${PWD}:/app" -w /app golang:1.26-alpine go test -count=1 -v ./services/payment/...
  ```
- Kết quả: **70 passed, 0 failed** trên toàn bộ các packages của `services/payment`:
  - `internal/adapter/gateway`: 8 test passed (100% pass)
  - `internal/adapter/http`: 22 test passed (100% pass)
  - `internal/adapter/metrics`: 2 test passed (100% pass)
  - `internal/adapter/postgres`: 5 test passed (100% pass)
  - `internal/app`: 25 test passed (100% pass, bao gồm initiate, webhook exactly-once, replay inbox, stale reconciliation)
  - `internal/domain`: 8 test passed (100% pass)
- Thư viện phụ trợ (`libs/go/...`):
  - `libs/go/authx`: PASS
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
| `services/payment/internal/app/process_webhook_test.go` | Luồng xử lý webhook exactly-once | Không được làm gãy, các đơn đã hoàn tiền (REFUNDED) không được phép ghi đè thanh toán |
| `services/payment/internal/app/reconcile_orders_test.go` | Luồng đối soát đơn treo | Không được làm gãy, đơn đã bị huỷ sự kiện/hoàn tiền cần được bỏ qua an toàn |
| `services/payment/internal/adapter/http/handler_test.go` | Route POST/GET webhook, xử lý duplicate và error responses | Mở rộng thêm endpoint hoàn tiền theo sự kiện `/v1/events/{eventID}/refund` |
| `services/payment/internal/adapter/postgres/repository_test.go` & `memory_repository_test.go` | Giao dịch DB atomicity, lock `orders FOR UPDATE` | Bổ sung phương thức truy vấn đơn hàng đã trả tiền theo sự kiện (`GetPaidOrdersByEvent`), cập nhật trạng thái `REFUNDING`, và xử lý hoàn tiền atomic (`ProcessRefundSuccessTx`) |

## 3. Ngưỡng coverage yêu cầu
- Lines: >= 70%
- Functions/Methods: >= 70%
- Statements: >= 70%
- Đảm bảo 100% tuân thủ 4 Quy tắc Bất biến của EvenFlow.
