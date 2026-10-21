# EV-198 — Truy vết AC → test

Unit: `ticketing/internal/app/issue_tickets_cleanup_test.go` (`U:`). Tích hợp Postgres: `ticketing/internal/adapter/postgres/repository_integration_test.go` (`PI:`).
Script: `deploy/scripts/demo.sh` (`DEMO`), `deploy/scripts/smoke.sh` (`SMOKE`). Bằng chứng chạy: evidence.md.

| AC | Test |
|---|---|
| AC-1 | DEMO: 33 bước, diễn tập 1 (104 s) và 2 (101 s) từ volume trống, chạy lặp (102 s) |
| AC-2 | SMOKE: 20 bước (bước 3 kiểm `jti`, bước 5 có header danh tính) |
| AC-3 | 3 lần `down -v` + `up -d`: exit 0, chỉ `migrate=exited`, 0 restart (ghi trong evidence.md) |
| AC-4 | U: `DonHoldRedisSauKhiPhatVe`, `DonHoldRedisCaKhiDonDaIssued` · PI: `ListOrderHoldIDs` · DEMO bước 9 (khách đang chờ nhận `SOLD_OUT`) |
| AC-5 | U: `LoiDonRedisKhongLamHongPhatVe`, `KhongCauHinhDonDepThiBoQua` · gate `Gate.Consume` chỉ `HDEL`/`ZREM`, không đụng `inv:*` (`TestGate_Consume`) |
