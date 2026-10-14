# EV-130 — Bằng chứng

Ngày chạy: 2026-10-14. Máy: Windows 11, Docker Desktop 28.4.0, Go 1.26.9 (`GOTOOLCHAIN=local`).
Nhánh `feature/EV-130-purchase-limit`, base `develop` @ `f443e26`.

**Chưa có bằng chứng CI (Linux):** nhánh chưa được đẩy. Mọi con số là chạy cục bộ.

## 1. Kết quả

| Kiểm | Kết quả |
|---|---|
| Unit `services/ticketing` (`go test -race ./...`) | PASS |
| Unit `libs/go/httpx` (`-race`) | PASS |
| Tích hợp `-run TestPurchaseLimit`, Postgres 16 + Redis 7 thật | PASS — 4 test (bảng 8 trạng thái + đúng biên, thiếu hạn mức, định nghĩa đếm, thanh toán xen giữa) |
| Tích hợp `TestReconciler*`, `TestNoOversell_WithConcurrentExpiry` | PASS |
| Tích hợp `TestNoOversell_UnderExtremeConcurrency` | **Không ổn định trên máy này, giống hệt `develop`** — xem §3 |
| `go vet` (có và không `-tags=integration`) | PASS |
| `GOWORK=off go build -mod=readonly ./...` | PASS |
| Coverage `domain/` + `app/` (cổng 70%) | 98,5% (`app` 100%, `domain` 80%) |
| `gofmt` trên file đã sửa (bỏ CRLF của checkout Windows) | sạch |

Stress test thanh toán xen giữa (8 tab, 5 giây): 4 đơn được trả tiền, ~1.600 lần bị chặn vì hạn mức, **4 vé chiếm
hạn mức / hạn mức 4**, 0 lỗi ngoài dự kiến.

## 2. Tái hiện lỗi trước khi sửa

Test viết trước, chạy trên code `develop` (chỉ thêm trường `MaxPerIdentity` vào port để biên dịch):

```
--- FAIL: TestCreateHold_BRO4_DangGiuGheBamMuaLaiThiNhanLaiHoldCu
    dang giu ghe bam mua lai phai nhan lai hold cu (BR-O3), nhan loi: vuot gioi han moi nguoi
--- FAIL: TestCreateHold_BRO4_TruyenHanMucXuongTransaction
    PersistHold phai nhan max_per_identity = 6, nhan 0
--- FAIL: TestCreateHold_BRO4_KhongTraCuuDuocHoldThiBaoLoi
```

## 3. Cổng oversell 10.000 goroutine trên Docker Desktop Windows

Lỗi duy nhất là **timeout Redis 200 ms** ở `IsAdmitted` (`kiem tra admit: i/o timeout`) và `chay script hold` —
trước khi chạm Postgres. Bất biến 1–3 (đúng 100 vé, tồn kho 0, Redis khớp Postgres) **đạt ở mọi lần**; test chỉ dừng
ở bất biến 4 (không request nào lỗi hệ thống).

So sánh cùng điều kiện, `develop` gốc chạy trong worktree riêng:

| Bản | Số request timeout qua các lần chạy |
|---|---|
| `develop` @ `f443e26` | 2895 · 464 · 1264 · PASS · 6161 |
| EV-130 | 445 · 1399 · 1744 · 4006 |

Hai phân phối như nhau: nhiễu của Docker Desktop vừa khởi động, không do EV-130 (thay đổi chỉ thêm một truy vấn
Postgres sau cổng Redis). Phán quyết cuối là CI Linux.

## 4. Đột biến

Áp tạm lên bản đã sửa, chạy unit + tích hợp `TestPurchaseLimit`, rồi gỡ. Không commit.

| # | Đột biến | Kết quả | Test đỏ |
|---|---|---|---|
| M1 | Tắt lớp chốt trong transaction | Đỏ 3/3 | `LopChotTrongTransaction…` (PAID, ISSUED, REFUNDING, đúng biên), `ThanhToanXenGiuaKhongVuotHanMuc` — **khách giữ 5–6 vé / hạn mức 4** |
| M1b | M1 + lớp nhanh đếm cả `HELD` (= hành vi `develop`) | Đỏ 3/3 | `ThanhToanXenGiuaKhongVuotHanMuc` — **5 vé / hạn mức 4**; `DemDungTheoDinhNghiaBRO4` |
| M2 | Bỏ nhánh trả hold cũ trong lớp nhanh | Đỏ | `DangGiuGheBamMuaLaiThiNhanLaiHoldCu` |

Ghi chú: bản đầu của stress test **xanh với M1** — cửa sổ TOCTOU thật (~1 ms) ngắn hơn một lần thanh toán. Test được
sửa để nới cửa sổ (`slowCountRepo`, chờ 20 ms sau khi đếm) và sau đó bắt M1 ổn định. Bản đầu của test bảng trạng thái
cũng dựa vào việc bucket 0 vừa khít 4 vé; đã chuyển sang mỗi trường hợp một bucket.

`MaxPerIdentity <= 0` (fail-closed) có test riêng `ThieuHanMucThiTuChoi`.
