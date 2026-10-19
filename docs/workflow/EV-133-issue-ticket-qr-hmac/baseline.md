# EV-133 (EVF-38) — Baseline (Trạng thái trước khi thay đổi)

Chụp lúc: 2026-10-19, commit `c555d4eb63f23cac8b978abb1aa561f8c4300068` (nhánh `develop`), working tree sạch (ngoại trừ tài liệu quy trình vibe coding `docs/09-Quy_trinh_vibe_coding.md`).
Nhánh làm việc: `feature/EV-133-issue-ticket-qr-hmac`.

---

## 1. Kết quả chạy test hiện tại

### A. Test các thư viện dùng chung (`libs/go`)
- Lệnh: `docker run --rm -v "${PWD}:/app" -w /app golang:1.26-alpine go test ./libs/go/mq ./libs/go/idem ./libs/go/loadsignal ./libs/go/httpx`
- Kết quả: **PASS 100%**
  - `libs/go/mq`: ok (1.993s)
  - `libs/go/idem`: ok (0.015s)
  - `libs/go/loadsignal`: ok (0.010s)
  - `libs/go/httpx`: ok (0.339s)

### B. Test dịch vụ Vé (`services/ticketing`)
- Lệnh: `docker run --rm -v "${PWD}:/app" -w /app golang:1.26-alpine go test -v ./services/ticketing/internal/domain/... ./services/ticketing/internal/app/...`
- Kết quả: **PASS 100% (28/28 tests passed, 0 failed)**
  - `services/ticketing/internal/domain`: ok (0.010s)
  - `services/ticketing/internal/app`: ok (0.011s)
  - Toàn bộ testsuite giữ ghế (CreateHold), BR-O1 (chống oversell), BR-O3 (một hold hoạt động), BR-O4 (hạn mức mua cá nhân / đơn) đều xanh.

### C. Test dịch vụ Thanh toán liên quan (`services/payment`)
- Lệnh: `docker run --rm -v "${PWD}:/app" -w /app golang:1.26-alpine go test ./services/payment/internal/domain/... ./services/payment/internal/adapter/gateway/...`
- Kết quả: **PASS 100%**
  - `services/payment/internal/domain`: ok (0.007s)
  - `services/payment/internal/adapter/gateway`: ok (0.010s)

---

## 2. Nhận định Baseline
- Toàn bộ test hiện có đều xanh.
- Mọi test fail sau này là lỗi hồi quy (regression) và không được chấp nhận.
- Bảng `tickets` đã có schema trong `migrations/0001_core.sql` với các ràng buộc `UNIQUE (jti)`, `status IN ('ISSUED','CHECKED_IN','VOID','REFUNDED')`.
- Chưa có thư viện `libs/go/authx` để ký và xác thực QR HMAC-SHA256 kèm `jti`.
- Chưa có logic phát hành vé (Ticket Issuance) trong `services/ticketing`.

---

## 3. Các test case có thể bị ảnh hưởng
| File | Mục tiêu assert | Ảnh hưởng dự kiến |
|---|---|---|
| `services/ticketing/internal/app/create_hold_test.go` | Luồng giữ ghế, kiểm tra tồn kho | Không được làm gãy |
| `services/ticketing/internal/worker/outbox_relay.go` | Relay sự kiện outbox qua RabbitMQ | Bổ sung routing_key phát vé không làm xung đột relay |
| `libs/go/mq/topology.go` | Topology RabbitMQ | Mở rộng binding nếu cần thiết mà không sửa đổi queues cũ |

---

## 4. Ngưỡng coverage yêu cầu
- Lines: >= 80%
- Functions: >= 80%
- Branches: >= 70%
- 4 Quy tắc Bất biến: 100% tuân thủ
