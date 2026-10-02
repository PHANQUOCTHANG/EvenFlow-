# EVF-1803/1804 (phần component miền) — đồng hồ theo giờ server + panel hàng chờ — spec & plan

| | |
|---|---|
| Jira | Phần component dùng chung của EVF-1803 (UI phòng chờ) và EVF-1804 (chọn vé + đồng hồ giữ ghế) |
| Tier | **Standard** |
| Base | `feature/EVF-1801-route-groups` (xếp tầng) |
| Branch | `feature/EVF-1803-queue-countdown` |
| Nguồn | `docs/01-nghiep-vu.md` BR-Q1..Q7, BR-O2 (**nguồn chuẩn**); `docs/08-stitch-ui-ux-handoff.md` §11.3, §11.4, §5 |

**Không phụ thuộc backend.** Mọi giá trị server-owned (`expires_at`, `rank`, `eta`, `poll_after_ms`) nhận
qua prop. Không gọi API, không suy diễn endpoint. Vì vậy slice này làm được ngay dù 6/8 service Go vẫn stub.

---

## 1. Phạm vi

### Trong phạm vi

| File | Nội dung |
|---|---|
| `src/lib/format.ts` | `formatClock` (ms → `mm:ss` / `h:mm:ss`), `formatVnd` (`1250000` → `1.250.000 đ`) |
| `src/lib/server-time.ts` | Tính và áp offset giữa đồng hồ client và đồng hồ server |
| `src/hooks/use-server-countdown.ts` | Logic đếm ngược theo giờ server |
| `src/components/ui/server-expiry-countdown.tsx` | Component đồng hồ, 3 variant |
| `src/components/queue/queue-status-panel.tsx` | Panel trạng thái hàng chờ, 8 trạng thái |

### Ngoài phạm vi

| Việc | Lý do |
|---|---|
| Gọi API / SSE / polling thật | Cần endpoint và OpenAPI, chưa có. `src/lib/queue-client.ts` đã có sẵn polling client, nối vào ở task sau. |
| `EventCard`, `TicketTierCard`, `OrderSummary`, `AssistantPanel` | Handoff §11.2, §11.6, §11.7 — slice sau, dễ hơn và độc lập |
| Page thật trong `(queue)` / `(checkout)` | Cần API contract |
| `SeatPicker` | Sự kiện trong dự án bán theo hạng vé + quota, chưa có sơ đồ ghế trong ERD |

## 2. Business rule phải tuân thủ — đây là phần dễ làm sai nhất

**2.1 BR-O2 — client KHÔNG tự tính thời hạn.** "Giữ ghế đúng 10:00 kể từ lúc hold thành công. Đồng hồ
đếm ngược lấy `expires_at` từ server; **client không tự tính**."

Hệ quả kỹ thuật bắt buộc: mỗi lần tick phải tính lại `expiresAt - now()`, **không** được giảm dần một
biến đếm. Nếu giảm dần thì khi tab bị background (browser tiết lưu `setInterval` xuống 1 lần/phút) đồng
hồ sẽ **chạy chậm hơn thực tế** và hiển thị cho người dùng nhiều thời gian hơn số họ thật sự còn —
đúng loại lỗi làm khách mất vé mà tưởng còn thời gian. Component cũng **không** được nhận `durationMs`;
chỉ nhận mốc `expiresAt` tuyệt đối.

**2.2 BR-Q7 — HAI đồng hồ khác nhau, không bao giờ gộp.** Suất admit có TTL **15 phút**; giữ ghế
**10 phút**. Handoff §11.4: "Make the timer label explicit so the two checkout timers cannot be
confused." Vì vậy `variant` là **bắt buộc**, không có default, và mỗi variant có nhãn riêng cố định
trong component — caller không tự đặt nhãn cho 2 đồng hồ checkout.

**2.3 BR-Q1 — LOBBY không có số thứ tự.** "Mọi người vào trước `sale_start_at` được gom vào LOBBY
**không có số thứ tự**. Đúng T0 hệ thống xáo trộn ngẫu nhiên rồi mới cấp rank. Vào sớm 2 tiếng hay 2
giây đều có cơ hội như nhau."

Hệ quả: ở trạng thái `LOBBY`, panel **không được** hiển thị rank, vị trí, phần trăm, hay ETA cá nhân —
kể cả khi caller truyền vào. Và phải nói rõ cho người dùng rằng vào sớm không có lợi thế, nếu không họ
sẽ tự suy ra điều ngược lại. Đây là cơ chế chống bot, không phải chi tiết trình bày.

**2.4 Không ETA tự bịa.** Handoff §5 nguyên tắc 3: ETA/rank/tồn kho chỉ hiển thị khi **có nguồn dữ
liệu**. Panel không tự tính ETA từ rank. Thiếu thì hiện "đang cập nhật", không hiện số.

**2.5 Không progress bar giả.** Handoff §11.3: "no fake progress". Chỉ vẽ tiến trình khi có **cả**
`rank` và `totalAhead`/`initialRank` từ server. Không có thì không vẽ.

**2.6 Mất kết nối ≠ mất chỗ.** `docs/01` dòng 48: rớt kết nối > 90s thì **vẫn giữ chỗ 5 phút** rồi mới
`DROPPED`. Trạng thái `RECONNECTING` phải nói rõ là chưa mất chỗ.

## 3. Phát hiện: handoff §11.3 thiếu trạng thái `DROPPED`

Handoff §11.3 liệt 7 variant: `LOBBY QUEUED ADMITTED EXPIRED SOLD_OUT RECONNECTING UNKNOWN`.

Nhưng state machine trong `docs/01-nghiep-vu.md` dòng 48 có thêm:
`QUEUED --rớt kết nối > 90s--> (giữ chỗ 5 phút) --quá hạn--> DROPPED`

`DROPPED` là trạng thái **khác** `EXPIRED`: `EXPIRED` là hết TTL admit (đã tới lượt mà không mua kịp),
`DROPPED` là mất kết nối quá lâu khi còn đang xếp hàng. Thông điệp cho người dùng khác nhau hoàn toàn.

`docs/01` là nguồn chuẩn nghiệp vụ, handoff tự nhận là "đề xuất UX/UI". Nên **làm 8 trạng thái** (7 của
handoff + `DROPPED`) và báo lại để handoff bổ sung.

## 4. AC

### AC-1 — `formatClock`
- **Given** `0` → `"00:00"`; `1000` → `"00:01"`; `59_000` → `"00:59"`; `60_000` → `"01:00"`; `600_000` → `"10:00"`
- **Given** `≥ 3_600_000` → dạng `h:mm:ss` (ví dụ `3_661_000` → `"1:01:01"`)
- **Given** số âm → `"00:00"`, **không** hiện dấu trừ (đã hết hạn thì hiển thị 0, không đếm lên)
- **Given** giá trị không phải số hữu hạn (`NaN`, `Infinity`) → `"00:00"`, không throw
- **Then** làm tròn **xuống** (`floor`): còn 1500ms phải hiện `00:01`, không phải `00:02` — không bao giờ hiện nhiều thời gian hơn thực tế

### AC-2 — `formatVnd`
- **Given** `1250000` → `"1.250.000 đ"` (dấu chấm phân cách nghìn, hậu tố `đ`, đúng DESIGN.md §Typography)
- **Given** `0` → `"0 đ"`; **Given** số âm → có dấu trừ đứng trước
- **Given** không phải số hữu hạn → trả `"—"`, không throw và **không** hiện `NaN đ`

### AC-3 — `server-time.ts`
- **Then** `serverOffsetMs(serverNowMs, clientNowMs)` = `serverNowMs - clientNowMs`
- **Then** `serverNow(offsetMs)` = `Date.now() + offsetMs`
- **Given** offset không truyền hoặc không hữu hạn → coi là `0` (tin đồng hồ client), **không** throw
- **Then** module **không** tự gọi API lấy giờ server — offset do caller cung cấp

### AC-4 — `useServerCountdown` — tính lại từ mốc, không giảm dần
- **Given** `expiresAt` = now + 10 phút, **Then** `remainingMs` ≈ 600_000 và `expired` là `false`
- **Given** thời gian trôi 5 phút (fake timer), **Then** `remainingMs` ≈ 300_000
- **Given** `expiresAt` trong quá khứ, **Then** `remainingMs` là `0` và `expired` là `true` **ngay lần render đầu**, không cần chờ tick
- **Given** tab bị background và interval bị bỏ qua nhiều nhịp, **When** tick tiếp theo chạy, **Then** `remainingMs` khớp **thời gian thực đã trôi**, không phải số nhịp đã chạy. Kiểm bằng cách advance timer **ít nhịp** nhưng nhảy `Date.now()` **nhiều** — giá trị phải theo `Date.now()`.
- **Given** `offsetMs` được truyền, **Then** mốc so sánh là `Date.now() + offsetMs`
- **Then** interval được clear khi unmount, và **dừng** khi đã hết hạn (không chạy vô ích mãi)
- **Then** hook **không** nhận `durationMs` — chỉ nhận mốc tuyệt đối (BR-O2)

### AC-5 — `ServerExpiryCountdown`
- **Then** prop `variant` là **bắt buộc**, giá trị `"sale-start" | "admission" | "hold"`; không có default
- **Then** mỗi variant có nhãn cố định, khác nhau, nói rõ nó đếm cái gì — hai đồng hồ checkout không thể lẫn (BR-Q7)
- **Then** chữ số dùng tabular numbers (không nhảy bề rộng mỗi giây)
- **Given** `remainingMs` ≤ `warningThresholdMs` (prop, **không** có giá trị mặc định tự bịa), **Then** chuyển sang trạng thái cảnh báo
- **Then** trạng thái cảnh báo dùng `--color-warning-fg` cho **chữ**, `--color-accent` chỉ cho viền/nền — amber chỉ đạt 2.19:1 trên nền trắng (xem EVF-1801 spec §3.2)
- **Given** đã hết hạn, **Then** hiện trạng thái hết hạn kèm text rõ ràng, **không** đếm số âm
- **Then** có `aria-live` thông báo, nhưng **không** thông báo mỗi giây (spam screen reader): chỉ thông báo khi vào cảnh báo và khi hết hạn
- **Then** vùng số có `aria-hidden` hợp lý và có text thay thế cho screen reader dạng chữ ("Còn 9 phút 30 giây"), không đọc `09:30` theo từng ký tự
- **Then** tôn trọng `prefers-reduced-motion` (không nhấp nháy)

### AC-6 — `QueueStatusPanel` — 8 trạng thái
- **Then** phủ `LOBBY QUEUED ADMITTED EXPIRED SOLD_OUT RECONNECTING DROPPED UNKNOWN`
- **Then** mỗi trạng thái có `Badge` kèm **text**, không truyền tin chỉ bằng màu

### AC-7 — `LOBBY` tuyệt đối không có số thứ tự *(BR-Q1 — quan trọng nhất)*
- **Given** `state="LOBBY"` và caller **có** truyền `rank={42}` và `etaSeconds={300}`, **Then** panel **không** render `42`, **không** render ETA, **không** render phần trăm hay progress
- **Then** có text nói rõ vào sớm không tạo lợi thế
- **Bằng chứng**: test phải assert `queryByText("42")` là `null` ngay cả khi rank được truyền

### AC-8 — `QUEUED`
- **Given** `rank` có, **Then** hiển thị rank dùng tabular numbers
- **Given** `etaSeconds` **không** có, **Then** hiện "đang cập nhật", **không** tự tính ETA từ rank (BR-2.4)
- **Given** `etaSeconds` có, **Then** hiển thị ETA đó
- **Given** **thiếu** `totalAhead`, **Then** **không** render progress (`role="progressbar"` phải không tồn tại) — BR-2.5
- **Given** có **cả** `rank` và `totalAhead`, **Then** render `role="progressbar"` với `aria-valuenow`/`aria-valuemin`/`aria-valuemax` đúng

### AC-9 — `ADMITTED` / `EXPIRED` / `SOLD_OUT` / `RECONNECTING` / `DROPPED` / `UNKNOWN`
- **ADMITTED**: chỉ render đồng hồ khi có `admissionExpiresAt`; variant phải là `"admission"`, **không** phải `"hold"`
- **EXPIRED**: nói rõ phải xếp lại và **không** được ưu tiên (BR-Q7)
- **SOLD_OUT**: không hiển thị CTA vào checkout (BR-Q6)
- **RECONNECTING**: phải nói rõ **chưa mất chỗ** (BR-2.6)
- **DROPPED**: khác hẳn EXPIRED — mất chỗ vì mất kết nối quá lâu
- **UNKNOWN**: không bịa trạng thái, mời thử lại

### AC-10 — Freshness & live region
- **Then** nếu có `lastUpdatedAt` thì hiển thị; không có thì không hiện gì (không bịa "vừa cập nhật")
- **Then** panel có `aria-live="polite"` để đổi trạng thái được thông báo
- **Then** `connectionState` (`"live" | "reconnecting" | "offline"`) hiển thị bằng **text**, không chỉ icon

### AC-11 — Gates
- `npm run lint`, `npm run typecheck`, `npm run build`, `npm run test:coverage` pass; không hồi quy 522 test hiện có

## 5. Hợp đồng API

```ts
// lib/format.ts
export function formatClock(ms: number): string
export function formatVnd(amount: number): string

// lib/server-time.ts
export function serverOffsetMs(serverNowMs: number, clientNowMs: number): number
export function serverNow(offsetMs?: number): number

// hooks/use-server-countdown.ts
export interface ServerCountdown { remainingMs: number; expired: boolean }
export function useServerCountdown(expiresAt: number | string | null | undefined, offsetMs?: number): ServerCountdown
// expiresAt null/undefined -> remainingMs 0, expired false (chua co moc, KHONG phai da het han)
// KHONG nhan durationMs (BR-O2)

// components/ui/server-expiry-countdown.tsx
export type CountdownVariant = "sale-start" | "admission" | "hold"
export interface ServerExpiryCountdownProps {
  variant: CountdownVariant            // BAT BUOC, khong default
  expiresAt: number | string
  offsetMs?: number
  warningThresholdMs?: number          // khong co default tu bia; khong truyen = khong co canh bao
  className?: string
}

// components/queue/queue-status-panel.tsx
export type QueueState =
  | "LOBBY" | "QUEUED" | "ADMITTED" | "EXPIRED"
  | "SOLD_OUT" | "RECONNECTING" | "DROPPED" | "UNKNOWN"
export type ConnectionState = "live" | "reconnecting" | "offline"
export interface QueueStatusPanelProps {
  state: QueueState
  rank?: number                 // rank HIEN TAI; BI BO QUA khi state = LOBBY (BR-Q1)
  initialRank?: number          // rank LUC VAO = mau so co dinh; thieu -> KHONG ve progress
  etaSeconds?: number           // thieu -> "dang cap nhat", KHONG tu tinh
  admissionExpiresAt?: number | string
  admissionWarningThresholdMs?: number   // khong truyen -> khong canh bao
  onJoinWaitlist?: () => void            // BR-Q6; khong truyen -> khong nhac waitlist
  lastUpdatedAt?: number | string
  connectionState?: ConnectionState
  offsetMs?: number
  className?: string
}
// Progress: valuemin 0, valuemax = initialRank, valuenow = clamp(initialRank - rank, 0, initialRank)
```

> **Sửa hợp đồng (2026-10-02) — `totalAhead` đổi thành `initialRank`.** Bản đầu dùng
> `totalAhead` làm mẫu số. Agent test chỉ ra đây là **lỗi thiết kế**, và đúng: nếu `totalAhead`
> là "số người đang ở phía trước" thì nó **giảm dần**, lấy làm mẫu số thì thanh tiến trình nhảy
> ngược; tệ hơn, `rank` và "số người phía trước" chênh nhau đúng 1, nên công thức
> `totalAhead - rank` luôn ra khoảng 0 → thanh tiến trình **luôn đứng im ở 0**. Hiện thực đầu
> tiên của mình mắc đúng lỗi này. Mẫu số phải là một giá trị **cố định**: rank lúc người dùng
> vào hàng. Khi đó `initialRank - rank` chỉ tăng, thanh tiến trình đơn điệu.

## 6. Rủi ro

| Rủi ro | Giảm thiểu |
|---|---|
| Đếm ngược giảm dần thay vì tính lại từ mốc → hiện nhiều thời gian hơn thực tế khi tab background | AC-4 có test riêng: advance **ít nhịp** timer nhưng nhảy `Date.now()` **nhiều** |
| Làm tròn lên → hiện `10:00` khi chỉ còn 9:59.5 | AC-1 bắt `floor` |
| Lỡ hiển thị rank ở LOBBY → phá cơ chế chống bot của BR-Q1 | AC-7 test assert rank **không** xuất hiện dù được truyền |
| Tự tính ETA từ rank cho "đẹp" | AC-8 cấm; không có ETA thì hiện "đang cập nhật" |
| `aria-live` thông báo mỗi giây làm screen reader không dùng được | AC-5 chỉ cho thông báo khi vào cảnh báo và khi hết hạn |
| Amber làm màu chữ | AC-5 bắt dùng `--color-warning-fg` |

## 7. Điểm dừng báo người dùng

- Nếu phát hiện thêm trạng thái nghiệp vụ nào trong `docs/01` mà handoff thiếu → báo, **không** tự thêm vào UI mà không ghi lại
- Nếu `warningThresholdMs` cần một giá trị mặc định để UI dùng được → **dừng và hỏi**, vì ngưỡng cảnh báo là policy nghiệp vụ (`DESIGN.md` gợi ý 2 phút cho hold, nhưng đó là tài liệu design chứ không phải BR)

### 3.1 Phát hiện thêm: DESIGN.md hướng dẫn hiện rank + progress ở LOBBY, trái BR-Q1

`apps/web/doc/design/stitch/md/DESIGN.md` mục Components.3 ghi:

> **Lobby Progress Card:** Hiển thị vị trí người dùng trong hàng đợi (Queue Position) với cỡ chữ
> `numeric-metric`, nhãn "Số thứ tự của bạn". Bên dưới là thanh tiến trình (progress bar) màu
> Action Indigo, kèm ước tính thời gian chờ trung bình…

Đây là chỉ dẫn **hiện số thứ tự và thanh tiến trình ở LOBBY** — trái trực diện BR-Q1, và chính là
lỗi mà AC-7 tồn tại để chặn. Code làm theo `docs/01`, nhưng **tài liệu design vẫn đang hướng dẫn
người làm tiếp đi sai**, nên phải báo lại y như đã báo việc handoff §11.3 thiếu `DROPPED`.

Phát hiện bởi code review độc lập (m9). Nguy hiểm hơn việc thiếu `DROPPED`: thiếu một trạng thái
thì sẽ có người nhận ra, còn làm theo một chỉ dẫn sai thì phá đúng cơ chế chống bot mà không ai thấy.

---

## 8. Lệch khỏi plan trong lúc làm

### 8.1 Hai hàm public không có trong hợp đồng §5 ban đầu

`spellDuration` (`lib/format.ts`) và `toEpochMs` (`lib/server-time.ts`) được thêm trong lúc hiện thực
nhưng §5 không khai. Code review (M4) chỉ ra hậu quả: `spellDuration` sinh **toàn bộ** nội dung mà
người dùng screen reader nghe được về đồng hồ giữ vé, mà `grep -c spellDuration format.test.ts` = **0**
— không một test trực tiếp nào. Kịch bản: ai đó bỏ phần giây thì "còn 1 phút 59 giây" đọc thành "còn
1 phút", sai 59 giây trên đồng hồ giữ ghế, và cả 717 test vẫn xanh. Đã bổ sung cả hai vào hợp đồng và
có test bảng trực tiếp.

### 8.2 `ServerCountdown` thêm field `ready` sau code review

Không có trong §5 ban đầu. Lý do ở §10 (M1).

### 8.3 File ngoài bảng §1

`src/components/ui/index.ts` (thêm 5 dòng export) và 5 file test — §1 chỉ liệt 5 file nguồn.

### 8.4 Những thứ cố ý KHÔNG làm, và lý do

| Việc | Lý do không làm |
|---|---|
| Icon dạng inline SVG | Dùng ký tự `⏱` / `⚠` với `aria-hidden`. Thoả yêu cầu "icon ngữ nghĩa" của DESIGN.md mà không thêm dependency icon nào. Đổi sang SVG thì phải sửa test (đang khớp theo lớp ký tự). |
| Ngưỡng thoát cho copy "Đang xác nhận" | Nếu server không bao giờ gửi `EXPIRED` về thì khách mắc ở copy chuyển tiếp vô hạn. Cần một policy (bao lâu thì mời tải lại) — là quyết định nghiệp vụ, không tự chọn. Ticket riêng. |
| `reconnectDeadlineAt` cho cửa sổ giữ chỗ 5 phút | Cần variant đồng hồ thứ 4 mà handoff §11.4 chỉ định nghĩa 3. Không tự phát minh. Ticket riêng, **nên ưu tiên**: `RECONNECTING` nói "chưa mất chỗ" mà không nói còn bao lâu là thiếu đúng con số khách cần nhất trong 5 phút đó. |
| `warningThresholdMs` thành bắt buộc `number \| null` | Code review (m1) đề nghị, và lập luận đúng: `variant` đã bắt buộc chính vì sợ default sai âm thầm, rồi lại để ngưỡng cảnh báo optional âm thầm — caller quên prop thì đồng hồ 10 phút chạy tới 0 không bao giờ cảnh báo, và không typecheck/test/lint nào bắt. Hoãn vì bắt buộc ở đây sẽ lan sang `QueueStatusPanelProps` và mọi caller. Ticket riêng. |
| `poll_after_ms` dạng "cập nhật lại sau N giây" | Polling ngoài phạm vi slice (§1). Nhưng copy hiện tại **đã hứa** "trang tự cập nhật" / "không cần tải lại" — nếu panel được cắm vào page trước khi `queue-client.ts` được nối thì lời hứa đó sai. Là điều kiện bàn giao, ghi vào traceability. |

## 9. Evidence

Chạy sau lần sửa cuối (không phải output cũ chép lại).

```
$ cd apps/web

$ npm run typecheck
> tsc --noEmit
(khong output = 0 error)

$ npm run lint
> next lint
✔ No ESLint warnings or errors

$ npm run test:coverage
exit=0
 Test Files  36 passed (36)
      Tests  830 passed (830)
```

830 test, 0 fail, 0 skip. Phân rã: 308 test của slice này + 522 test đã có từ trước → **không hồi
quy** (mốc 522 khớp đúng tổng của branch `feature/EVF-1801-route-groups`).

Code review độc lập **tự chạy lại cả 4 gate** và xác nhận số liệu khớp, kể cả việc `717 − 195 = 522`
ở lần chạy trước.

> **Lưu ý về con số coverage.** `vitest.config.ts` đặt `coverage.include = ["src/**/*.{ts,tsx}"]` và
> không loại `*.test.*`, nên file test được tính như source và con số tổng bị đẩy lên. Con số tái lập
> được nhưng **không phải coverage của source**, nên không dùng nó làm bằng chứng chất lượng. Coverage
> source thật của 5 file mới: 100% trừ `format.ts` branches, chính là khoảng trống `spellDuration` mà
> M4 chỉ ra và đã được đóng. Lỗi config có từ slice trước, đã có ticket riêng.

## 10. Code review độc lập (bước 7) và cách xử lý

Reviewer: agent riêng, context sạch. **Kết luận: không có blocker**, 4 major, 13 minor. Nó xác nhận
phần logic đồng hồ thực sự tính lại từ mốc chứ không chỉ nói vậy trong comment, và **BR-Q1 sạch tuyệt
đối** — đã soi từng nhánh điều kiện cộng cả đường rò qua `aria-*`, `data-*` và live region.

| | Nội dung | Xử lý |
|---|---|---|
| **M1** | Hook đọc `Date.now()` ngay trong thân render. Mọi route là `○ (Static)` nên HTML được prerender **lúc build** → con số thời điểm build bị đóng băng vào HTML; khách mở trang ba ngày sau thấy "Đã hết thời gian giữ vé" ở first paint. `formatClockTime` dùng `getHours()` nên prerender ở UTC vs browser ICT lệch 7 tiếng → hydration mismatch. Reviewer chỉ ra repo **đã có** đúng pattern phòng lỗi này (`use-theme.ts` khởi tạo bằng hằng số rồi đọc giá trị phụ thuộc browser trong effect) và slice này đi ngược lại. | **Đã sửa.** Thêm `ready: boolean`; trước mount trả giá trị xác định và hiện `--:--` (không phải `00:00`, vì `00:00` trông như đã hết hạn). `lastUpdatedAt` chỉ render sau mount. Hook cũng chuyển sang tính trong lúc render, nhờ đó sửa luôn m6. |
| **M2** | `typeof x === "number"` cho `NaN`/`Infinity` đi qua. `rank={NaN}` → hiện chữ `NaN`, `aria-valuenow="NaN"`, và `width: "NaN%"` bị browser bỏ qua nên div trong có `h-full` không width → **thanh tiến trình hiện đầy 100%**, báo với khách "gần tới lượt" trong khi không biết gì. `etaSeconds={NaN}` → `spellDuration(NaN)` ra `"0 giây"` → "Thời gian ước tính: ~0 giây", đúng cái AC-8 cấm. Reviewer chỉ ra `format.ts` và `server-time.ts` đều dùng `Number.isFinite`, panel là chỗ **duy nhất** lệch khỏi chuẩn đó. | **Đã sửa:** `Number.isInteger(rank)`, `Number.isInteger(initialRank) && > 0`, `Number.isFinite(etaSeconds)`. |
| **M3** | `ADMITTED` + đồng hồ đã hết hạn cho hai thông điệp ngược nhau, và tình huống này **bắt buộc xảy ra**: BR-Q4 cho `poll_after_ms` tới 30s nên luôn có cửa sổ tới 30 giây giữa lúc đồng hồ về 0 và lúc server đẩy `EXPIRED` về. Trong đó panel vừa mời đi thanh toán vừa báo suất đã hết, không một chữ nói khách phải làm gì. | **Đã sửa:** panel tự biết đồng hồ đã hết và đổi sang copy chuyển tiếp ("Suất mua vừa hết hạn… chưa cần làm gì thêm"), đồng thời **bỏ render đồng hồ** để panel và đồng hồ không lệch nhau một nhịp. |
| **M4** | `spellDuration` / `toEpochMs` là public API trên luồng tiền, không AC, không dòng truy vết, không test trực tiếp. | **Đã sửa:** thêm vào hợp đồng §5, có test bảng trực tiếp. |

Minor đã sửa: **m2** (`aria-valuetext` — "958" một mình vô nghĩa với screen reader; dùng "đã tiến"
chứ không "đã gọi" vì BR-Q2 cho người vào sau T0 nối FIFO nên con số đó không chắc là số người thực
sự được gọi), **m3** (hết hạn đổi sang `role="alert"` + `assertive`: mất hold là sự kiện khách mất
vé, polite bị xếp sau mọi output đang đọc, ví dụ khách đang gõ số thẻ), **m4** (gộp nhãn — trước đó
screen reader đọc nhãn 2–3 lần), **m7** (`toEpochMs` từ chối chuỗi không có múi giờ: ECMA hiểu
`"2026-10-03"` là UTC còn `"2026-10-03T09:00:00"` là giờ địa phương, nên mốc mở bán lệch 7 tiếng),
**m8** (thêm icon ngữ nghĩa theo DESIGN.md), **m9** (báo lại xung đột DESIGN.md vs BR-Q1 — xem §3.1),
**m13** (viết §8 và §9 này).

Thêm một việc do agent test nêu sau đó: `toEpochMs` trả `null` im lặng biến **lỗi backend thành UI
đứng im** — khách thấy `--:--` vô thời hạn và không ai biết tại sao. Đã thêm `console.warn` ở
dev/CI. Đây là điểm agent test nói nó lo nhất.

Hoãn có ghi lại: **m1** (bắt buộc `warningThresholdMs`), **m5** (nhịp 250ms vẫn có thể hiện nhiều hơn
thời gian còn lại tới 249ms — phá đúng tính chất §6 tuyên bố, nhưng tác động thực tế gần như bằng 0),
**m10** (`SOLD_OUT` im lặng tuyệt đối về waitlist khi caller quên prop), **m11** (copy hứa trang tự
cập nhật trong khi polling chưa nối), **m12** (coverage bị thổi phồng), và 4 khoảng trống hợp đồng mà
agent test nêu: `ADMITTED` + `ready === false`, ngưỡng thoát cho copy "Đang xác nhận",
`spellDuration(0)`/`(NaN)` chưa chốt chuỗi, và hai bộ đếm trên cùng một mốc.
