# EV-198 — Bằng chứng

Ngày chạy: 2026-10-21. Windows 11, Docker Desktop 28.4.0, Go 1.26.9, bash (Git for Windows), Python 3.13. Base: `feature/EV-118-dynamic-active-events`.
**Chưa có bằng chứng CI (Linux):** mọi con số là chạy cục bộ; `e2e-nightly.yml` chưa chạy trên runner.

| Kiểm | Kết quả |
|---|---|
| `go vet` + `-tags=integration`, unit `-race` module ticketing | PASS |
| Tích hợp ticketing `adapter/postgres` (gồm `ListOrderHoldIDs`) | PASS |
| `bash -n` `demo.sh`, `smoke.sh` | PASS |
| `docker compose config -q` | PASS |

## Diễn tập từ volume trống (`down -v` → `up -d` → `seed.sh` → `demo.sh`)

| Lần | `up -d` | `seed` | Container restart | `smoke.sh` | `demo.sh` |
|---|---:|---:|---|---|---|
| 1 | 25 s | 4 s | 0 | — | 33/33, 104 s |
| 2 | 32 s | 4 s | 0 | 20/20 | 33/33, 101 s |
| 2 (chạy lại, cùng stack) | — | — | — | — | 33/33, 102 s |

Thời gian từng bước của lần 2: đăng ký + OTP 1 s · tạo sự kiện + duyệt 2 s · chờ giờ mở bán 76 s (DEMO_LEAD 75) · giữ ghế → thanh toán → vé →
email 3 s · kiểm QR 1 s · khách 2 huỷ giữ ghế 4 s · bảo mật <1 s · trợ lý AI 1 s · hết vé 12 s.

Khởi động sạch 3 lần liên tiếp sau khi sửa healthcheck: `up -d` exit 0 (25 s, 37 s, 35 s), chỉ `migrate=exited`.
Log toàn stack sau diễn tập: 0 lần `unnamed prepared statement`.

## Lần diễn tập hỏng đã dẫn tới các sửa lỗi (giữ lại để minh bạch)

1. Lần dựng đầu sau thêm Mailpit/gateway: `up -d` thoát lỗi `service "migrate" didn't complete successfully` → healthcheck Postgres.
2. Lần diễn tập kế: kiểm duyệt AI không phản hồi trong 30 s; log event-service `unnamed prepared statement does not exist`;
   ai-worker retry sau 30 s nhận 422 (đã quá BR-E2) rồi vào DLQ → `lib/pq` qua PgBouncer.
3. Bước hết vé: cờ `SOLD_OUT` không bật vì còn 1 hold đã thanh toán trong Redis (`HLEN hold:{ev}:h = 1`) → `WithHoldCleanup`.
4. Bước hết vé, lần khác: `429` ở `queue/status` (poll 1 s/lần) và `join` trả `SOLD_OUT` không có token → sửa script.
