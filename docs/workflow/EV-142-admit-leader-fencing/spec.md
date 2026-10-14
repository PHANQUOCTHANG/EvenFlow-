# EV-142 — Admit controller: bầu leader có fencing và đo sức khoẻ thật (Spec)

| Mục | Giá trị |
|---|---|
| Jira | EV-142 (mã backlog **EVF-55**, issue 1406) — 8 SP — Highest — Sprint 2 — nhãn `critical` |
| Kế hoạch tổng | [09-ke-hoach-hoan-thien-backend.md](../../09-ke-hoach-hoan-thien-backend.md) — Giai đoạn 5, hai việc "Bầu leader" và "Đo sức khoẻ thật" |
| Branch | `feature/EV-142-admit-leader-fencing` (từ `develop` @ `945259d`, sau PR #60) |
| Quy tắc nghiệp vụ | BR-Q5 và Q5.1–Q5.9 (mới) — [01-nghiep-vu.md](../../01-nghiep-vu.md) |
| Kiến trúc | [02-kien-truc.md](../../02-kien-truc.md) Tầng 5 |

## 1. Bài toán

Lúc mở bán có thể 300.000 người cùng bấm mua trong khi Postgres chỉ chịu an toàn khoảng 200 lượt giữ ghế mỗi
giây. Phòng chờ đặt một **van** phía trước: admit controller quyết định mỗi giây thả bao nhiêu người vào checkout
(`admit_rate`), điều chỉnh theo AIMD để **tải xuống Postgres gần như hằng số** dù hàng chờ có 30 nghìn hay 3 triệu
người. Đây là việc 2 trong 3 việc rủi ro nhất của dự án (docs/04).

## 2. Hiện trạng trước slice (đã kiểm trên `develop` @ `945259d`)

Không ai nhận task (không nhánh, PR, commit nào mang EV-142 / EVF-55; `libs/go/leader` rỗng). Khung từ giai đoạn 0
có vòng AIMD nhưng van đang "mù" và không có người cầm van duy nhất:

| # | Lỗ hổng | Hệ quả |
|---|---|---|
| H1 | `HealthProbe` là bản tĩnh `Health{}` luôn "khoẻ" | AIMD **chưa bao giờ hoạt động**: nhịp leo tới `MaxRate` rồi đứng yên bất kể Postgres ra sao → vòng xoáy sập (cascading failure) |
| H2 | Không bầu leader; chỉ đúng vì compose tình cờ chạy một bản | Hai pod (K8s 2 replica, rolling update, pod kẹt) cùng thả → **tốc độ gấp đôi**. `ZPOPMIN` chặn thả *trùng người*, không chặn *tốc độ* |
| H3 | Khoá đơn thuần không đủ | Leader cũ đứng hình (GC, throttle CPU) quá hạn lease rồi tỉnh lại, không biết mình đã mất khoá, vẫn thả. Kiểm "còn khoá không" trước khi ghi không cứu được vì có thể đứng hình *ngay sau* phép kiểm |
| H4 | Nhịp chỉ nằm trong RAM | Leader mới bắt đầu lại từ sàn 5/s: đang 400/s bỗng tụt, cần ~50 giây leo lại |
| H5 | Ghi đè của Ops chỉ nằm trong RAM | Ops bấm "tạm dừng khẩn cấp" (0) rồi leader đổi thì cờ mất, **leader mới tự mở van lại giữa sự cố** (lỗi an toàn) |
| H6 | Phản ứng liên tục với tín hiệu trễ | Người vừa vào mất 30 giây – 2 phút mới mua nên tải DB là hậu quả của quyết định trước đó: giảm, giảm, giảm rồi vọt lên → dao động răng cưa |

## 3. Tiêu chí nghiệm thu

Jira AC (4) và các AC kỹ thuật suy ra từ H1–H6:

| AC | Nội dung |
|---|---|
| AC-1 | Tiêm độ trễ giả vào checkout → `admit_rate` giảm trong ≤ 10 giây *(Jira)* |
| AC-2 | Phục hồi trong ≤ 60 giây sau khi hết độ trễ *(Jira)* |
| AC-3 | Không dao động quá 2 chu kỳ *(Jira)*; ở đây: không đổi chiều nào khi sức ép không đổi |
| AC-4 | Chỉ một leader mỗi sự kiện; leader cũ bị fencing chặn ghi *(Jira)* |
| AC-5 | Hai controller chạy song song thả không quá nhịp của **một** controller |
| AC-6 | Leader chết / tắt êm → instance khác lên với epoch lớn hơn và tiếp tục; không thả trùng, không bỏ quên |
| AC-7 | Leader cũ đứng hình: lệnh ghi đầu tiên bị `FENCED`, controller dừng ngay, không thả thêm ai, không đè nhịp leader mới |
| AC-8 | Leader mới tiếp tục từ nhịp đã lưu, không về sàn |
| AC-9 | Ghi đè của Ops (kể cả 0) sống sót qua đổi leader; bỏ ghi đè thì AIMD chạy lại; lệnh đặt trực tiếp trên controller không bị store gỡ nhầm |
| AC-10 | Không đo được sức khoẻ (không pod nào báo cáo, ảnh chụp cũ, Redis lỗi) coi như quá tải, không coi là khoẻ |
| AC-11 | Mất key epoch không làm epoch quay về số nhỏ (fencing không bị vô hiệu) |
| AC-12 | Nhịp, leader, epoch và lý do mỗi tick có metric; hai leader cùng lúc và không-có-leader có alert |

## 4. Thiết kế

### 4.1. `libs/go/leader` — lease + fencing token

Lease kiểu thuê có hạn trên Redis, mỗi thao tác là một Lua nguyên tử:

| Thao tác | Script | Ghi chú |
|---|---|---|
| Acquire | `SET lock id NX PX ttl`; epoch = `max(epoch + 1, TIME ms)` | Epoch **không có TTL**. Dùng đồng hồ Redis để epoch không quay lại số nhỏ khi mất key (AC-11) |
| Renew | `GET lock == id` thì `PEXPIRE` | Chỉ chủ khoá gia hạn |
| Release | `GET lock == id` thì `DEL` | Không xoá nhầm khoá người mới lên |

- TTL 5 giây, gia hạn mỗi 1 giây (cấu hình `LEADER_TTL_SECONDS`, `LEADER_RENEW_SECONDS`; bắt buộc `RenewEvery ≤ TTL/2`).
- Gia hạn trả `false` → thoái vị ngay và **không** nhả khoá (không còn của mình). Gia hạn *lỗi* thì chỉ thoái vị
  khi đã `≥ TTL − RenewEvery` kể từ lần thành công cuối (sau thời điểm đó người khác có thể đã lên).
- Giữa hai nhiệm kỳ luôn chờ một chu kỳ, tránh vòng lấy-nhả khoá nóng khi `fn` thất bại ngay.
- **Fencing nằm ở nơi ghi**: mỗi script ghi của controller nhận epoch và, ở dòng đầu, trả `FENCED` nếu
  `GET epoch ≠ epoch`. Kiểm và ghi cùng một lệnh nên không có khe hở (AC-4, AC-7).

### 4.2. `libs/go/loadsignal` — sức khoẻ thật

| Cách | Chọn? | Vì sao |
|---|---|---|
| Hỏi Prometheus | ✗ | Scrape 15 giây + cửa sổ `rate()` → tín hiệu trễ > 20 giây, trượt AC-1; biến hệ thống giám sát thành điểm chết của vòng điều khiển |
| Gọi `/metrics` từng pod | ✗ | Phải tự tìm pod, tự parse |
| **Pod ticketing tự ghi ảnh chụp lên Redis mỗi giây** | ✔ | Trễ ≤ 1 giây, dùng Redis sẵn có |

- `Recorder` (cửa sổ trượt 5 giây, reservoir 2048 mẫu/giây): p99, số yêu cầu, số lỗi. `Publisher` `HSET loadsignal:ticketing <pod>`.
- `Reader` gộp: p99 và pool lấy **pod tệ nhất**, tỷ lệ lỗi = tổng lỗi / tổng yêu cầu. Ảnh chụp cũ hơn `HEALTH_MAX_AGE_SECONDS`
  (3 giây) bị bỏ qua; cũ hơn 10× thì bị dọn. Không còn ảnh chụp nào → `ErrNoSignal` (AC-10).
- Ticketing chỉ tính yêu cầu **đã chạm tầng dưới**: bỏ `not_admitted`; chỉ `error` là lỗi (hết vé, vượt hạn mức là kết
  quả nghiệp vụ bình thường).
- Trước T0 chưa có yêu cầu: pod vẫn báo 0 yêu cầu, p99 0 → khoẻ, không phải "không biết".

### 4.3. Chống dao động (H6)

AIMD giữ nguyên (+10 khi khoẻ, ×0,7 khi có sức ép) và thêm **cửa sổ chờ** `DecreaseCooldown = 5` tick: sau một lần
giảm *thực sự* (không tính khi đã ở sàn), giữ nguyên nhịp 5 tick để tín hiệu trễ kịp phản ánh. Chấp nhận đánh đổi: từ
trần 500 xuống sàn 5 mất ~78 giây thay vì 13 giây. Lần giảm **đầu tiên** vẫn tức thì (500 → 350 trong 1 giây), nên AC-1
đạt; cooldown chỉ ngăn giảm chồng. `ADMIT_DECREASE_COOLDOWN=0` tắt nếu thực tế cần phản ứng gắt hơn.
Phục hồi từ sàn lên trần: 5 tick chờ + 50 tick (+10/tick) ≈ 55 giây ≤ 60 (AC-2), sát giới hạn — nếu trần tăng phải
tăng `StepUp` theo.

### 4.4. Trạng thái bền

`wr:{ev}:meta.admit_rate` (đã có, `status.lua` đọc để tính ETA) được `set_rate.lua` ghi có fencing; leader mới nạp
lại khi bắt đầu (AC-8). `override` / `override_rate` đọc mỗi tick (AC-9); phần ghi (`Queue.SetRateOverride`) để API
Ops của EV-147 gọi.

## 5. Phạm vi và ngoài phạm vi

**Trong phạm vi:** `libs/go/leader`, `libs/go/loadsignal`, fencing 3 script, `FencedQueue`, controller (restore,
override từ store, cooldown, thoát khi `ErrFenced`), probe, ghi tín hiệu ở ticketing, metric, alert, cấu hình, CI.

**Ngoài phạm vi (ghi lại):**

- **EV-147 (API và UI cho Ops):** chỉ lưu và tôn trọng ghi đè; API, UI, audit log là việc của EV-147.
- **Ngân sách nhịp chung nhiều sự kiện:** health tính chung nhưng mỗi sự kiện có controller và trần riêng
  (`MaxRate` 500) nên 3 sự kiện mở bán cùng lúc có thể cộng dồn tới 1.500. Cần việc riêng.
- **Lease trên một Redis:** failover Redis (replication bất đồng bộ) có thể cho vài giây hai leader. Fencing vẫn
  chặn leader cũ ngay khi chạm epoch mới; thiệt hại là nhịp tạm gấp đôi, không thả trùng người, không oversell.
  Chặt hơn phải dùng etcd/Consul (không đáng ở giai đoạn này).
- **Đồng hồ giữa các pod** (độ tuổi ảnh chụp so với đồng hồ controller): lệch vài giây sẽ làm tín hiệu bị coi là cũ
  và nhịp lùi về sàn — hướng an toàn; NTP giữ lệch ≪ 3 giây.
- **`seed.sh`** xoá mọi key `{EVENT_ID}` kể cả lease và epoch: leader mất khoá, thoái vị, lên lại với epoch mới. Vô hại
  nhờ epoch theo thời gian, nhưng nhớ khi viết script dọn dẹp khác.
