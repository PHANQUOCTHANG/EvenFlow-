# EV-152 — Baseline (Trạng thái trước khi thay đổi)

Chụp lúc: 2026-10-17 19:53 UTC+7, commit gốc `cf8b84648faf3e715caa8524c6727d0e8d90451b` (nhánh `develop` sau khi merge PR #64 cho task EV-151), working tree sạch.

## 1. Kết quả chạy test hiện tại
- Lệnh thực thi:
  ```bash
  docker run --rm -v evenflow-pkg-mod:/go/pkg/mod -v evenflow-go-build:/root/.cache/go-build -v "${PWD}:/app" -w /app golang:1.26-alpine go test -count=1 -v ./services/payment/...
  ```
- Kết quả: **51 passed, 0 failed** trên toàn bộ 5 packages của `services/payment`:
  - `gateway`: 12 test passed (100% pass)
  - `http`: 14 test passed (100% pass)
  - `postgres`: 3 test passed (100% pass)
  - `app`: 16 test passed (100% pass)
  - `domain`: 6 test passed (100% pass)
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
| `services/payment/internal/app/process_webhook_test.go` | Luồng xử lý webhook exactly-once, chữ ký, replay timestamp | Không được làm gãy, cần mở rộng kịch bản khi đơn chưa tồn tại |
| `services/payment/internal/adapter/http/handler_test.go` | Route POST/GET webhook, xử lý duplicate và error responses | Cần tương thích backward, hỗ trợ phản hồi 200/202 khi lưu inbox |
| `services/payment/internal/adapter/postgres/repository_test.go` & `memory_repository_test.go` | Giao dịch DB atomicity, lock `orders FOR UPDATE` | Bổ sung các phương thức thao tác bảng `payment_inbox` |

## 3. Ngưỡng coverage yêu cầu
- Lines: >= 70%
- Functions/Methods: >= 70%
- Statements: >= 70%
- Đảm bảo 100% tuân thủ 4 Quy tắc Bất biến của EvenFlow.
