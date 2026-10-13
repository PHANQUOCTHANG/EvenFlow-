# EV-132 — Bằng chứng

Ngày chạy: 2026-10-13. Máy: Windows 11, Docker Desktop 28.4.0, Go 1.26.9 (`GOTOOLCHAIN=local`).
Nhánh `feature/EV-132-inventory-reconciler`, base `develop` @ `d868c8a`.

**Chưa có bằng chứng CI (Linux):** nhánh chưa được đẩy. Mọi con số là chạy cục bộ.

## 1. Kết quả

| Kiểm | Kết quả |
|---|---|
| Unit `internal/worker` (`-race -count=3`) | PASS — 22 test reconciler, fake xác định |
| Tích hợp `test/concurrency`, `-run TestReconciler`, Postgres 16 + Redis 7 thật | PASS — 8 test |
| `go vet` (có và không `-tags=integration`), `go test -race ./...` ở `services/ticketing` | PASS |
| Coverage `domain/` + `app/` (cổng 70%) / `internal/worker` | 98,1% / 91,3% |
| `GOWORK=off go build -mod=readonly ./...` | PASS |
| `smoke.sh` từ volume trống | **20/20** (18 cũ + bước 9: làm lệch Redis, chờ tự sửa) |

Dưới tải mô phỏng bằng 20 hold thật rồi lệch thêm 6 vé: Redis được sửa đúng 6, 20 hold và tổng
`available + đang giữ = quota` không đổi.

## 2. Thời gian tự sửa (AC-1: ≤ 20 giây)

Trên stack Docker, lệch −3 ở một bucket, thời gian từ lúc làm lệch tới lúc khớp Postgres:

| Cấu hình | Các lần đo |
|---|---|
| Chỉ xác nhận ở chu kỳ sau (bản đầu) | 17,1 s · 19,0 s · 19,6 s — đạt nhưng sát giới hạn: cận trên theo thiết kế đúng bằng 2 chu kỳ |
| Thêm lượt xác nhận sau 2 s trong cùng chu kỳ (bản nộp) | 10,3 s · 5,4 s · 3,6 s · 10,3 s |

Cận trên của bản nộp: tối đa 10 s để chu kỳ kế tiếp thấy + 2 s xác nhận ≈ 12 s.

## 3. Đột biến

Áp tạm lên bản đã commit, chạy unit + tích hợp `TestReconciler`, rồi `git checkout`. Không commit.

| # | Đột biến | Kết quả | Test đỏ (rút gọn) |
|---|---|---|---|
| R1 | Sửa ngay lần thấy đầu tiên | Đỏ | `NhieuThoangQua…`, `DoiDau…`, `HaiLanLechKhacNhau…`, +6 |
| R2 | Không trừ hold đang bay khỏi kho kỳ vọng | Đỏ | `HoldDangBay_KhongPhaiLech`, `HoldDangBay_KhongBiCoiLaLech`, +3 |
| R3 | Coi hold đã trả ở Postgres như còn giữ | Đỏ | `PostgresDaTraRedisChuaKip_*` (unit + tích hợp) |
| R4 | Không đổi hex32 → UUID khi tra trạng thái hold | Đỏ | `VoiHoldThat_ChiSuaDoLechInject_KhongPhaHold` (chỉ tích hợp bắt được) |
| R5 | Bỏ khoá theo sự kiện | Đỏ | `HaiInstanceSongSong_KhongSuaGapDoi`, `KhoaDoiSoatDoInstanceKhac_BoQua` |
| R6 | Đảo chiều sửa | Đỏ | 11 test |
| R7 | Sửa theo lần đo mới nhất thay vì giá trị nhỏ hơn | Đỏ | `LechTangDan_ChiSuaGiaTriNhoHon…` (xem ghi chú) |
| R8 | Script không kẹp tồn kho ≥ 0 | Đỏ | `ScriptKhongDeTonKhoXuongAm` |
| R9 | Không bao giờ chạy lượt xác nhận | Đỏ | `LuotXacNhan_SuaNgayTrongMotChuKy`, `MotChuKyVoiLuotXacNhan…`, `LuotXacNhan_HuyCtxThiThoat` |
| R10 | Luôn chờ xác nhận dù không lệch | Đỏ (treo tới timeout) | `LuotXacNhan_KhongCoLechThiKhongDocLanHai` |

Ghi chú R7: lần chạy đầu **sống sót** vì kịch bản duy nhất của test là chiều giảm (−8 rồi −7), nơi `abs(now)` và
`min` cho cùng kết quả. Đã thêm kịch bản chiều tăng (−7 rồi −8) và R7 đỏ.

R4 chỉ bị bắt ở test tích hợp: unit test dùng fake nên không thấy ranh giới id. Đây là lý do test tích hợp dùng
`app.CreateHold` thật chứ không dựng hold bằng tay.

## 4. Phát hiện khi làm

- **`HoldView.HoldID` trả cho client là UUID có gạch** (lấy từ Postgres `id::text`), trong khi Redis lưu id dạng hex
  32 ký tự. `Gate.Release(ctx, ev, view.HoldID)` vì thế là **NOOP âm thầm**. Hiện chưa có đường nào gọi như vậy
  (sweeper dùng id từ ZSET), nhưng API huỷ hold ở giai đoạn 2 sẽ dính. Nên chuẩn hoá một dạng id.
- `hold.lua` chỉ quét 3 trong 32 bucket, nên quota nhỏ (≈3 vé/bucket) báo hết sớm khi đặt `qty ≥ 2`. Đúng thiết kế
  (đã ghi trong EV-134 spec), nhưng test mới phải dùng quota 3.200 để không đỏ vì lý do này.
- Hash hold của Redis đủ làm sổ cái cho đối soát, nhưng **mất hash nghĩa là mất chỉ mục hết hạn** của hold đang
  active ở Postgres (xem "Ngoài phạm vi" trong spec).

## 5. Điểm cần biết

- Metric `ticketing_inventory_drift` không có dấu (tổng |lệch|): hai bucket lệch ngược dấu không triệt tiêu nhau,
  nhưng cũng không cho biết hướng; hướng nằm ở `ticketing_inventory_drift_corrected_total{direction}`.
- Trong lúc một lượt đang chờ xác nhận (2 s), khoá giữ 8 s; chu kỳ 10 s nên không chồng chu kỳ.
- Sửa sai do nhiễu hiếm gặp tự hiện ra thành lệch ngược dấu ở chu kỳ sau và được sửa lại; sai số bị chặn bởi
  một lần sửa, và hướng "Redis phồng" vẫn bị Postgres chặn (không oversell).
- Worker log WARN mỗi lần sửa: mong đợi **không có** trong vận hành bình thường; xuất hiện đều đặn là dấu hiệu
  Redis đang mất kho (timeout 200 ms dưới đợt cao điểm, xem EV-134).
- `gofmt -l` báo file do working copy Windows checkout CRLF (index là LF), như đã ghi ở EV-134.

## 6. Việc còn lại

| Việc | Ai | Trạng thái |
|---|---|---|
| Đẩy nhánh, mở PR vào `develop`, xem job `integration` trên Linux | Người có quyền | Chờ — chưa push theo quy ước nhóm |
| Chạy `opening-spike.js` (k6) với job bật, xem `inventory_drift` và số lần sửa | — | Chưa làm |
| Dựng lại chỉ mục hết hạn cho hold active mà Redis đã mất | Giai đoạn 6 (thứ tự consume) | Hoãn |
| Chuẩn hoá dạng id hold giữa Redis và Postgres | Giai đoạn 2 (huỷ hold) | Đề xuất |
| Thêm `InventoryDrift == 0` vào test hết hạn của oversell gate | Trưởng nhóm duyệt | Đề xuất từ EV-134 |
