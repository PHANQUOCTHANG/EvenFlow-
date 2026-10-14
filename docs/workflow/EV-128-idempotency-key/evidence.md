# EV-128 — Bằng chứng

Ngày chạy: 2026-10-13. Máy: Windows 11, Docker Desktop 28.4.0, Go 1.26.9 (tải về thư mục tạm, `GOTOOLCHAIN=local`).
Nhánh `feature/EV-128-idempotency-key`, base `develop` @ `f0f368b`.

**Chưa có bằng chứng CI (Linux)** cho slice này: nhánh chưa được đẩy lên. Mọi con số là chạy cục bộ.

## 1. Test

| Bộ test | Kết quả |
|---|---|
| `libs/go/idem` unit (`-race -count=10`) | PASS — 17 test, store giả có đồng hồ giả |
| `libs/go/idem` tích hợp, Postgres 16 thật (`-race -tags=integration -count=10`) | PASS — 10 test × 10 lần |
| `services/ticketing/internal/adapter/http` | PASS — 4 test mới (thiếu key, phát lại, 409 nghiệp vụ được nhớ, 503 không bị khoá) |
| `go test -race ./...` ở `libs/go` và `services/ticketing` | PASS |
| `apps/web` `hold-client` + `use-hold-timer` | PASS 26; `tsc --noEmit` và `eslint` sạch |
| `test-workflows.sh` / `test-run-and-report-tests.sh` | 137 PASS / 40 PASS |
| `smoke.sh` từ volume trống, qua PgBouncer | **18/18** (15 cũ + 3 bước idempotency) |

Test đồng thời chạy 50 goroutine cùng key trên Postgres thật, handler ngủ 300 ms: handler chạy đúng 1 lần, 1 request
nhận 200, 49 còn lại 409 hoặc phát lại.

## 2. Đột biến

Áp tạm lên bản đã commit, chạy `go test -tags=integration ./idem/`, rồi `git checkout`. Không commit.

| # | Đột biến | Kết quả | Test đỏ |
|---|---|---|---|
| M1 | Bỏ so sánh hash ở `Begin` | Đỏ | `KhacBodyHoacKhacNguoi_Tra422`, `DinhDanhKhongPhaiUUID…` |
| M2 | Bỏ định danh khỏi hash | Đỏ | `CungKhoaKhacNguoi_Tra422VaKhongLoResponse`, `KhacBodyHoacKhacNguoi_Tra422` |
| M3 | Lưu cả response 5xx | Đỏ | `Loi5xx_XoaKhoaDeThuLaiThatSu`, `Loi5xxVaPanic_XoaKhoa` |
| M4 | `Complete` không kiểm quyền sở hữu | Đỏ | `QuaLease_TiepQuanVaChuCuKhongGhiDe` + 4 test khác |
| M5 | Ghi kết quả bằng ctx của request | Đỏ | `ClientNgatGiuaChung_VanLuuResponse`, `ClientNgat_VanLuuResponse` |
| M6 | Không bao giờ tiếp quản sau lease | Đỏ | `QuaLease_TiepQuanVaChuCuKhongGhiDe`, `HetHan_…` |
| M7 | Không bao giờ giao lại key hết hạn | Đỏ | `HetHan_KhoaDuocDungLaiVoiNoiDungKhac` |
| M8 | Panic không xoá key | Đỏ | `Panic_XoaKhoaVaDePanicDiTiep`, `Loi5xxVaPanic_XoaKhoa` |

Không đột biến nào sống sót. M1 chỉ bị bắt ở test tích hợp vì store giả của unit test tự so hash — đúng thiết kế:
logic SQL phải được kiểm trên Postgres thật.

## 3. Độ trễ `POST /holds` (stack Docker Desktop, tuần tự, 300 request)

| Chế độ | p50 | p99 |
|---|---|---|
| Key mới mỗi lần (thêm `Begin` + `Complete`) | 26,2 ms | 43,8 ms |
| Cùng key (phát lại, một lần đọc) | 23,6 ms | 36,6 ms |

**Không có mốc "trước khi thêm `idem`"** nên không kết luận được phần phí; đây chỉ là số tuyệt đối trên máy dev,
một client tuần tự. Phí dưới tải 10.000 đồng thời chưa đo (cổng oversell gọi thẳng use case nên không đi qua
middleware). Cần đo bằng k6 `opening-spike.js` trước khi mở bán.

## 4. Điểm cần biết

- **Frontend:** `hold-client.ts` coi mọi 409 là "hết vé". Đã sửa kèm slice: 409 có `type` =
  `urn:eventflow:problem:idempotency-in-progress` thành `AMBIGUOUS`. Backend và web phải triển khai cùng nhau.
- **`libs/go/go.mod` trên `develop` thiếu hai dòng** `testcontainers-go/modules/postgres` và `modules/redis` mà
  `testx` import; trong workspace vẫn build được nhờ `services/ticketing/go.mod`, nhưng `GOWORK=off` thì không.
  `go mod tidy` bổ sung và nâng `go-redis` `v9.7.0 → v9.7.3`; giữ trong slice này.
- `pgx` ghim `v5.9.2` cùng bản với ticketing (và PR #55) để tránh xung đột `go.sum`.
- Tiếp quản sau lease có thể chạy lại handler. An toàn với hold nhờ BR-O3; API có tác dụng phụ không idempotent
  (thanh toán, giai đoạn 6) phải tự có khoá phía cổng thanh toán.
- Định danh hiện là `X-Identity-Id` client tự khai; chưa là ranh giới bảo mật cho tới giai đoạn 3.
- `gofmt -l` báo file chỉ do working copy Windows checkout CRLF (index là LF), như đã ghi ở EV-134.

## 5. Việc còn lại

| Việc | Ai | Trạng thái |
|---|---|---|
| Đẩy nhánh, mở PR vào `develop`, xem job `integration` trên Linux | Người có quyền | Chờ — chưa push theo quy ước nhóm |
| Đo phí `idem` dưới tải bằng k6 | — | Chưa làm |
| Dọn key hết hạn | Giai đoạn 2 | Hoãn |
| Rebase nếu PR #55 (outbox) merge trước: cùng đụng `libs/go/go.mod`, `go.sum` | Người merge sau | Chờ |
