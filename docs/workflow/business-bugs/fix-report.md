# Sửa bug nghiệp vụ tìm thấy bởi test domain/app

Nhánh `fix/business-bugs`, xếp chồng lên `fix/backend-debt` (`d762ee5`). Ba bug do test của WP-C/WP-D
tìm ra (`docs/workflow/backend-debt/integration-report.md` §3). Quyết định của người dùng
(2026-10-08): **sửa data race**, **sửa override giữ tới khi Ops bỏ**, **sửa tài liệu BR-O3 theo code**.

Cách làm: test repro đã có (sau build tag `bugrepro`) ⇒ TDD: chứng minh đỏ trên mã cũ, sửa, chứng minh
xanh, rồi đưa test ra chạy mặc định để bug quay lại là CI đỏ ngay.

## 1. Data race trong `CreateHold` (ticketing) — ĐÃ SỬA

- **Lỗi:** mọi request dùng chung một `*math/rand.Rand` (`create_hold.go`, field `rng`). Kiểu đó không
  an toàn đồng thời; `Execute` chạy song song từ HTTP handler đúng lúc mở bán.
- **Sửa:** bỏ field `rng`, dùng `rand.IntN` cấp gói của `math/rand/v2` (an toàn đồng thời, tự seed).
- **Test:** `TestCreateHold_NhieuKhachGiuGheDongThoi_KhongDuocDataRace` chuyển từ
  `create_hold_bug_test.go` (bugrepro) sang `create_hold_concurrency_test.go` (chạy mặc định). CI
  `test-go` chạy `-race` nên race quay lại sẽ đỏ.

## 2. BR-Q5 manual override không được giữ (waitingroom) — ĐÃ SỬA

- **Lỗi:** `SetRateOverride` chỉ ghi biến rate; tick kế tiếp chạy AIMD từ đó. Ops đặt 0/s để dừng khẩn
  cấp ⇒ 3 tick sau vẫn thả 60 người.
- **Sửa:** cờ `override atomic.Bool`. Đang ghi đè ⇒ `tick` không đo health, không chạy AIMD; rate của
  Ops được công bố và dùng nguyên vẹn. Thêm `ClearRateOverride()` để trả quyền cho AIMD (tick sau tiếp
  tục từ rate hiện tại, kéo về `[MinRate, MaxRate]` — sau khi tạm dừng 0/s thì bắt đầu lại từ sàn).
  `adjust` dùng `CompareAndSwap` và kiểm cờ lần nữa, nên một lần ghi đè rơi đúng giữa lúc AIMD đọc và
  ghi không bị AIMD đè lên (khe race logic mà review P5 của slice nợ đã chỉ ra).
- **Test:** hai test repro chuyển sang `admit_controller_override_test.go` (mặc định), thêm
  `TestClearRateOverride_HandsControlBackToAIMD` (2 ca). Xoá `admit_controller_bug_test.go` (rỗng sau
  khi chuyển — không mất test nào).
- **Test cũ phải chỉnh bước dựng:** 4 test (`TestTick_BackpressureDecreasesRate`,
  `…SustainedBackpressureFloorsAtMinRate…`, `…UnknownHealthIsTreatedAsBackpressure`,
  `…RecoversAfterBackpressureClears`) dùng `SetRateOverride` để **đặt rate xuất phát** rồi kỳ vọng AIMD
  chạy — tức bước dựng dựa vào đúng ngữ nghĩa lỗi. Thêm `ClearRateOverride()` ngay sau bước dựng.
  `git diff` của file: **chỉ thêm dòng**, không assert nào đổi.
- **Vẫn chưa có endpoint Ops** gọi `SetRateOverride` / `ClearRateOverride` — ngoài phạm vi.

## 3. Bằng chứng (golang:1.25 Debian, `GOPROXY=off`, bản copy LF)

```
MA CU (HEAD) + test moi:
  ticketing  TestCreateHold_NhieuKhachGiuGheDongThoi_KhongDuocDataRace  --- FAIL, 11 x WARNING: DATA RACE
  waitingroom TestSetRateOverride_IsRespectedOnNextTicks               --- FAIL
  waitingroom TestSetRateOverride_ZeroPausesAdmissionOnNextTicks       --- FAIL
    (lan chay TRUOC khi them ClearRateOverride vao buoc dung cua 4 test cu; sau do ban cu
     khong con bien dich duoc vi test cu goi ham chua ton tai o ma cu)
MA MOI + test moi:
  ticketing   race test     ok
  waitingroom override tests ok
  ticketing   vet + vet -tags bugrepro + go test -race   rc 0, coverage 97.5%
  waitingroom vet + vet -tags bugrepro + go test -race   rc 0, coverage 98.6%
  waitingroom app -race x20                              rc 0
  golangci-lint v2.12.2: ticketing 0, waitingroom 0
```

Ghi chú môi trường: lần chạy đầu dùng `golang:1.25-alpine` cho ra "build failed" ở **cả** mã cũ lẫn mới
— DNS trong container hỏng nên `apk` không cài được `gcc` (cần cho `-race`). Kết quả đó bị bỏ; chuyển
sang `golang:1.25` (Debian, có sẵn gcc) với `GOPROXY=off`.

## 4. BR-O3 — CHƯA LÀM, chờ xác nhận lại

Quyết định là "sửa tài liệu theo code". Khi rà mọi chỗ nhắc BR-O3 thì thấy **frontend đang hiện thực
theo tài liệu cũ**: `apps/web/src/components/checkout/ticket-tier-card.tsx:41` và test của nó
(`ticket-tier-card.test.tsx:737` "dang co hold o hang khac: phai canh bao") cảnh báo khách rằng **hold cũ
sẽ bị huỷ** khi chọn hạng vé khác; spec EVF-1802 (`docs/workflow/EVF-1802-event-cards/spec-plan.md` AC-6)
và `docs/jira-import.csv` cũng theo tài liệu cũ. Trong khi backend:

- `create_hold.go` nhánh `HoldAlreadyHolding` trả lại **hold cũ nguyên vẹn** (hạng vé + số lượng cũ),
  bất kể lần bấm mới chọn hạng nào;
- **không có endpoint huỷ hold** (chỉ sweeper trả kho khi hết hạn).

Nếu chỉ sửa tài liệu: khách chọn hạng khác thấy "hold cũ sẽ bị huỷ" nhưng nhận lại hold cũ, và **không
có cách đổi hạng** trong 10 phút. Ba cách xử lý thay đổi phạm vi rất khác nhau — xem câu hỏi cho người
dùng.
