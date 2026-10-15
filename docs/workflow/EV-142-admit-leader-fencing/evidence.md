# EV-142 — Bằng chứng

Ngày chạy: 2026-10-14. Máy: Windows 11, Docker Desktop 28.4.0, Go 1.26.9 (`GOTOOLCHAIN=local`).
Nhánh `feature/EV-142-admit-leader-fencing`, base `develop` @ `945259d`.

**Chưa có bằng chứng CI (Linux):** nhánh chưa được đẩy. Mọi con số là chạy cục bộ.

## 1. Kết quả

| Kiểm | Kết quả |
|---|---|
| Unit `libs/go/leader`, `libs/go/loadsignal` (`-race -count=5`) | PASS |
| Unit `services/waitingroom` (`-race`) | PASS — 5 package |
| Unit `services/ticketing` (`-race`) | PASS |
| Tích hợp `libs/go/leader`, Redis 7 thật (`TestTichHop*`) | PASS — 6 test |
| Tích hợp `libs/go/loadsignal`, Redis 7 thật | PASS — 3 test |
| Tích hợp `services/waitingroom/test/leader`, Redis 7 thật, `-race` | PASS — 9 test, lặp 3 lần không flaky |
| `go vet` (có và không `-tags=integration`) cả 3 module | PASS |
| `GOWORK=off go build -mod=readonly ./...` cả 3 module (sau `go mod tidy` waitingroom) | PASS |
| Coverage `domain/` + `app/` của waitingroom (cổng 70%) | 98,3% (`domain` 100%, `app` 98,1%) |
| `.github/scripts/tests/test-workflows.sh` | 137/137 |
| `smoke.sh` từ volume trống, stack đầy đủ có controller và ticketing mới | **20/20**, chạy 2 lần (trước và sau đổi epoch sang đồng hồ) |

## 2. Chạy thật: hai replica và `kill -9`

Trên stack compose thật, kịch bản AC-6 và AC-8:

| Bước | Quan sát |
|---|---|
| Tín hiệu sức khoẻ | Hash `loadsignal:ticketing` có ảnh chụp của pod ticketing, cập nhật mỗi giây |
| Lease | `wr:{ev}:leader` = instance đang giữ, `PTTL` ≈ 4,7 s |
| Epoch | `wr:{ev}:leader_epoch` = `1791988421561` (đồng hồ ms) |
| Nhịp | `admit_rate` leo lên 50 (trần local) khi tín hiệu khoẻ |
| Thêm replica thứ hai | Replica 2 **không** lên làm leader (0 lần), replica 1 giữ cả hai sự kiện |
| `docker kill` leader (không nhả khoá) | Leader mới lên sau **5,2 giây** (≈ TTL), epoch `…503962` > `…421561` |
| Nhịp sau đổi leader | Log `tiep tuc nhip admit cua leader truoc rate=50`; `admit_rate` giữ nguyên 50, không về sàn 5 |

## 3. Tái hiện lỗi trước khi sửa (H1, H2)

Đọc mã `develop` @ `945259d` cho thấy bằng chứng cấu trúc thay vì chạy lại:

- `staticHealth.Health` trả `app.Health{}` → `healthy()` luôn đúng → `adjust(true)` mỗi tick → nhịp chỉ đi lên tới `MaxRate`.
- `admit.lua` không có điều kiện nào ngoài `sold_out`; không có leader lock nên hai controller cùng `ZPOPMIN`.

Hai test mới thể hiện hậu quả và đều đỏ khi cơ chế bị gỡ (§4): `HaiController_ChiMotLeaderThaNguoi` (nhịp 50/s,
3 giây: một leader thả 145, hai leader sẽ ~290) và `LeaderCuDungHinh_ControllerDungNgayKhongThaThem`.

## 4. Đột biến

Áp tạm, chạy bộ tích hợp, rồi gỡ. Không commit.

| # | Đột biến | Kết quả | Test đỏ |
|---|---|---|---|
| M1 | Bỏ kiểm epoch trong `admit.lua` | Đỏ | `Fencing_AdmitCuBiChan`, `Fencing_ChuaCoLeaderThiMoiEpochBiChan` |
| M2 | Khoá cho phép nhiều leader (`if cur then return 0 end` bị xoá) | Đỏ | `HaiController_ChiMotLeaderThaNguoi`, `LeaderTatEm_NguoiKhacTiepQuan` |
| M3 | Controller không khôi phục nhịp cũ khi lên leader | **Sống sót ở bản đầu → siết test → Đỏ** | `DoiLeader_TiepTucNhipVaGiuTamDungCuaOps` ("nhịp đầu tiên của leader mới = 6, thấp hơn nhịp cũ 155") |
| M4 | Không đọc ghi đè của Ops từ store | Đỏ | `DoiLeader_TiepTucNhipVaGiuTamDungCuaOps` |
| M5 | Bỏ qua `ErrFenced`, tiếp tục vòng admit | Đỏ | `LeaderCuDungHinh_ControllerDungNgayKhongThaThem` |

Ghi chú về M3: bản đầu của test kiểm "nhịp ≥ nhịp cũ" bằng `eventually` 3 giây; không khôi phục thì B vẫn leo lại
tới đó trong ~2,9 giây nên đột biến qua. Test được sửa để kiểm **lần ghi đầu tiên** của leader mới (StepUp 1/tick),
sau đó M3 bị bắt ổn định.

## 5. Lỗ hổng phát hiện khi chạy smoke, đã sửa

Log controller lúc chạy `smoke.sh` cho thấy leader thoái vị rồi lên lại với **cùng epoch 1**: `seed.sh` xoá mọi key
`{EVENT_ID}`, kể cả khoá leader và epoch. Bản đầu cấp epoch bằng `INCR` nên mất key là epoch quay về 1, và một
leader cũ mang epoch 1 sẽ lại được nơi ghi chấp nhận, tức fencing bị vô hiệu đúng lúc cần. Đã đổi thành
`max(epoch + 1, TIME ms)` (spec §4.1) và thêm test `MatKeyEpochKhongLamEpochQuayLai`.

## 6. Đánh đổi và giới hạn đã biết

- **Cooldown làm giảm chậm hơn khi quá tải kéo dài:** lần giảm đầu vẫn tức thì (500 → 350 trong 1 giây), nhưng từ
  trần xuống sàn mất ~78 giây thay vì 13 giây. Chủ ý (spec §4.3), cấu hình được bằng `ADMIT_DECREASE_COOLDOWN`.
- **Phục hồi 55 giây** từ sàn lên trần 500 (+10/tick), sát giới hạn 60 giây của Jira; tăng trần thì tăng `StepUp`.
- **Mặc định `ADMIT_HEALTH_MODE=signal`:** khi chạy controller mà không có ticketing, nhịp bị giữ ở sàn. Muốn leo
  trần ở local không có ticketing thì đặt `static` (có cảnh báo to trong log).
- **Docker Desktop Windows:** `TestNoOversell_UnderExtremeConcurrency` vẫn chập chờn timeout Redis 200 ms trên máy này,
  như `develop` (ghi ở evidence EV-130). Không thuộc slice này; phán quyết là CI Linux.
- Ngoài phạm vi: ngân sách nhịp chung nhiều sự kiện, API/UI Ops (EV-147), failover Redis có thể cho vài giây hai
  leader. Xem spec §5.
