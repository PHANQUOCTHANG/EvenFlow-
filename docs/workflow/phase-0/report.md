# Giai đoạn 0 — nối những gì đã viết cho chạy được

Nhánh `feat/phase-0-wiring`. Mục tiêu: `up → migrate → seed → smoke` cho ra một luồng thật
xếp hàng → admit → giữ ghế → hết hạn → trả kho, qua PgBouncer như production.

## Đã làm

| Việc | Thay đổi |
|---|---|
| Seed | `deploy/scripts/seed.sh` — idempotent, `RESET=1` xoá sạch một sự kiện (Postgres + Redis), ghi `sale_start_ms` |
| Nạp tồn kho | `InventorySeeder` (worker): Postgres → Redis, `HSETNX` nên không ghi đè số đang bị hold trừ |
| Worker trong compose | `ticketing.Dockerfile` build thêm `/worker`; service `ticketing-worker` |
| Admit controller | `cmd/controller`, `internal/config` (validate env lúc khởi động), `Queue.SaleStart`; service `waitingroom-controller` |
| Smoke | `deploy/scripts/smoke.sh` — 15 bước, kiểm cả hai tầng tồn kho |
| Sửa lỗi tích hợp | PgBouncer không đăng nhập được Postgres 16 (`AUTH_TYPE=scram-sha-256`) — chưa từng chạy được trước đây |

## Bằng chứng

- `gofmt`, `go vet` (+ tag `bugrepro`, `integration`), `go test -race`: ticketing app 100%, worker 93.2%;
  waitingroom app 98.3%, config 100%, domain 100%.
- `golangci-lint` v2.12.2: 0 issues ở `libs/go`, `ticketing`, `waitingroom`.
- Trivy `--severity CRITICAL --ignore-unfixed`: 0 ở 4 binary (server/worker/server/controller).
- `smoke.sh` xanh 3 lần liên tiếp; `seed.sh` chạy lại không đổi tồn kho (64 bucket, 20.000 vé).
- pgx 5.9.2 chạy ổn qua PgBouncer `transaction` ở chế độ mặc định — nghi vấn #9 của
  `backend-debt/integration-report.md` không tái hiện, không cần đổi `QueryExecMode`.

## Nợ còn lại (cố ý, ghi để không bị quên)

1. **`HealthProbe` của controller là bản tĩnh** (luôn khoẻ) — rate leo thẳng tới `ADMIT_MAX_RATE`.
   Compose đặt trần 50/s. Thay bằng số đo thật ở EVF-55 / EVF-3.
2. **Chưa bầu leader** (EVF-55): chỉ chạy đúng 1 bản controller. Chạy 2 bản thì nhịp admit gấp đôi
   (không admit trùng người nhờ `ZPOPMIN`).
3. **T0 do `seed.sh` ghi vào Redis** — event-svc (EVF-20) phải ghi thay.
4. **`ACTIVE_EVENT_IDS` là danh sách tay**; sự kiện tạo sau khi worker/controller khởi động sẽ không được quét.
5. **k6 `opening-spike.js` chưa giữ ghế được**: gửi `X-Identity-Id: loadtest-…` không phải UUID nên
   `PersistHold` lỗi. Cần seed identity hàng loạt hoặc chờ identity-svc (giai đoạn 3).
6. **Web chưa gọi được backend**: `NEXT_PUBLIC_API_BASE=http://localhost:8080` nhưng chưa có gateway.
7. `seed.sh` đặt lại `sale_start_at = now()` mỗi lần chạy trên sự kiện đã có.
8. `edoburu/pgbouncer:latest` chưa pin phiên bản.
9. Seeder là bản tối thiểu của EVF-37: chỉ điền chỗ thiếu, **chưa** phát hiện/sửa lệch và chưa có metric `inventory_drift`.
