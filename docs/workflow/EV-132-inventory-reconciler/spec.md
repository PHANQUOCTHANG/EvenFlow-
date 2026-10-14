# EV-132 — Job đối soát tồn kho Redis ↔ Postgres (Spec)

| Mục | Giá trị |
|---|---|
| Jira | EV-132 (mã backlog **EVF-37**, issue 1308) — 5 SP — Highest — Sprint 1 |
| Kế hoạch tổng | [09-ke-hoach-hoan-thien-backend.md](../../09-ke-hoach-hoan-thien-backend.md) — Giai đoạn 2, việc "Đối soát tồn kho" |
| Branch | `feature/EV-132-inventory-reconciler` (từ `develop` @ `d868c8a`, sau PR #57) |
| Quy tắc nghiệp vụ | BR-O1 (không oversell), BR-O7 (trả kho idempotent) — [01-nghiep-vu.md](../../01-nghiep-vu.md) |

## 1. Bối cảnh

Jira AC: (1) cố ý làm lệch Redis → job tự sửa trong ≤ 20 giây; (2) metric `inventory_drift` về 0 sau khi sửa.

Trước slice này chỉ có `InventorySeeder` (bản tối thiểu của giai đoạn 0): nó chỉ điền vào bucket **chưa có**
(`HSETNX`), nên không bao giờ sửa một bucket đã có nhưng sai. Redis lệch (failover, mất dữ liệu, timeout 200 ms làm
mất một lần trả kho — xem EV-134 `evidence.md` §1) thì khách bị báo "hết vé" sai cho tới khi hold hết hạn hoặc
Redis được nạp lại. Metric `ticketing_inventory_drift` đã có trong `alerts.yml` và `docs/02` nhưng **chưa ai xuất**,
nên cảnh báo `InventoryDrift` không bao giờ kêu.

Việc nhận: chưa có nhánh hay PR nào cho việc này tại thời điểm khảo sát (chỉ PR #55 outbox đang mở ở backend).

## 2. Vấn đề cốt lõi: hold đang bay

Redis trừ kho **trước**, Postgres commit **sau** (và khi trả kho thì ngược lại: Postgres trước, Redis sau). Ở bất kỳ
thời điểm nào, dưới tải, luôn có hold khiến hai số *hợp lệ nhưng khác nhau*. So `Redis == Postgres` rồi sửa sẽ đè lên
chính các hold đó: Redis phồng lên, hoặc hold ở Postgres không còn dấu ở Redis.

Lời giải: dùng hash `hold:{ev}:h` của Redis (đã lưu `tt`, `bucket`, `qty` của từng hold) làm sổ cái:

```
kỳ_vọng(bucket) = Postgres.available − Σ qty của hold ĐANG NẰM trong hash Redis
                  mà Postgres chưa ghi, hoặc đã trả kho
lệch(bucket)    = Redis(bucket) − kỳ_vọng(bucket)
```

Hold có ở cả hai tầng (active hoặc consumed) thì cả hai cùng đã trừ, không ảnh hưởng. Hai trường hợp còn lại là
"đang bay" và "chờ release" — đều **không phải lệch**.

## 3. Tiêu chí nghiệm thu

| AC | Nội dung |
|---|---|
| AC-1 | Cố ý làm lệch Redis (thiếu hoặc phồng, một hoặc nhiều bucket) → job tự sửa về khớp Postgres trong ≤ 20 giây |
| AC-2 | `ticketing_inventory_drift{ticket_type}` phản ánh lệch chưa giải thích được và về **0** sau khi sửa |
| AC-3 | Hold đang bay (Redis đã trừ, Postgres chưa commit) và hold chờ release **không bị coi là lệch**, không bị sửa |
| AC-4 | Nhiễu thoáng qua (đọc Redis và Postgres lệch nhau vài ms) không bao giờ thành phép sửa |
| AC-5 | Sửa không đè lên hold đang trừ kho đồng thời (dùng delta, không SET) và không bao giờ đẩy tồn kho Redis xuống âm |
| AC-6 | Hai instance worker chạy song song không sửa gấp đôi |
| AC-7 | Redis bị xoá hết key tồn kho được nạp lại đúng theo Postgres |
| AC-8 | Lỗi ở một sự kiện, hoặc Redis/Postgres lỗi, không gây phép sửa sai và không chặn sự kiện khác |

## 4. Thiết kế

- **Chu kỳ 10 giây**, mỗi sự kiện trong `ACTIVE_EVENT_IDS`, chạy trong `ticketing-worker` cùng sweeper và seeder.
- **Khoá theo sự kiện** (`SET recon:{ev}:lock NX PX 8000`): hai instance không cùng đối soát một chu kỳ.
- **Đọc Redis trước, Postgres sau**; ảnh chụp Redis (tồn kho + hash hold) là một `MULTI` nên nguyên tử.
- **Xác nhận trước khi sửa:** lệch phải thấy cùng dấu ở hai lượt liên tiếp; lượt xác nhận chạy sau 2 giây ngay trong
  chu kỳ (giữ khoá). Số sửa là giá trị **nhỏ hơn** trong hai lần; phần thừa nếu có để lượt sau.
- **Sửa bằng delta** (`reconcile.lua`: `HGET` + cộng + kẹp ≥ 0 + `HSET`, một script nguyên tử).
- Metric: `ticketing_inventory_drift{ticket_type}` (tổng |lệch| theo bucket, **không có dấu** để lệch ngược dấu giữa
  hai bucket không triệt tiêu nhau) và `ticketing_inventory_drift_corrected_total{direction=refill|drain}`.
- Seeder **giữ nguyên**: nó nạp nhanh (5 giây) sau khi mất dữ liệu hoàn toàn; job này lo phần còn lại.

## 5. Ngoài phạm vi

- **Dựng lại chỉ mục hết hạn** cho hold đang active ở Postgres mà Redis đã mất (ZSET `hold:{ev}:z`): sau khi Redis
  mất dữ liệu, các hold đó không bao giờ được sweeper trả. Cần quyết định về thứ tự consume ở giai đoạn 6.
- Tự release hold mồ côi (Redis có, Postgres không, quá hạn): sweeper đã dọn khi TTL hết; release sớm hơn có nguy
  cơ làm hold đang bay mất chỉ mục hết hạn.
- Thay `ACTIVE_EVENT_IDS` bằng danh sách từ event service (giai đoạn 4).
- Thêm `InventoryDrift == 0` vào `TestNoOversell_WithConcurrentExpiry` (đề xuất của EV-134, cần trưởng nhóm duyệt).
