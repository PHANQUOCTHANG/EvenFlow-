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
