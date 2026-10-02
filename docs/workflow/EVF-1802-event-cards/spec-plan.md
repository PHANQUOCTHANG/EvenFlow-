# EVF-1802/1804 (phần card + order summary) — spec & plan

| | |
|---|---|
| Jira | Phần component dùng chung của EVF-1802 (trang sự kiện) và EVF-1804 (chọn vé) |
| Tier | **Standard** |
| Base | `feature/EVF-1803-queue-countdown` (xếp tầng) |
| Branch | `feature/EVF-1802-event-cards` |
| Nguồn | `docs/01-nghiep-vu.md` BR-E1..E3, BR-O1, BR-O3..O5 (**nguồn chuẩn**); `docs/08-stitch-ui-ux-handoff.md` §11.2, §11.6, §3, §5 |

**Không phụ thuộc backend.** Mọi giá trị server-owned nhận qua prop.

---

## 1. Phạm vi

| File | Nội dung |
|---|---|
| `src/components/event/event-card.tsx` | Thẻ sự kiện cho trang công khai, 6 trạng thái + skeleton |
| `src/components/checkout/ticket-tier-card.tsx` | Thẻ hạng vé, chọn số lượng, các trạng thái chặn |
| `src/components/checkout/order-summary.tsx` | Tóm tắt đơn, sticky, đồng hồ giữ vé, 6 trạng thái hold |
| `src/lib/money.ts` | `formatMoney(amount, currency)` — xem §2.4 |

### Ngoài phạm vi

| Việc | Lý do |
|---|---|
| Gọi API tạo hold / thanh toán | Chưa có endpoint, chưa có OpenAPI |
| `SeatPicker` | Dự án bán theo hạng vé + quota, ERD không có sơ đồ ghế |
| `PaymentForm` | Không thiết kế provider thanh toán cụ thể (handoff §7) |
| Page thật trong `(marketing)` / `(checkout)` | Cần API contract |
| `AssistantPanel`, `DataTable`, `MetricCard` | Handoff §11.7–11.9, slice sau |

## 2. Business rule phải tuân thủ

**2.1 BR-O1 — không bao giờ khẳng định tồn kho chính xác.** "Redis lệch số **không thể** gây oversell —
chỉ có thể gây *báo hết vé sớm*, và job đối soát sẽ sửa lại." Cộng handoff §3: "số lượng vé public chỉ
nên là **snapshot/nhãn khái quát**, không query DB theo mỗi lượt xem."

Hệ quả: `availability` là **nhãn rời rạc**, không phải số. `"available" | "limited" | "sold_out"`.
Component **không** nhận và **không** hiển thị số vé còn lại, kể cả khi caller muốn — một con số sai
sớm 30 giây là lời hứa sai với khách, và handoff §6 cấm thẳng badge "còn 1 vé" nếu không có nguồn
chính xác. Đây là lý do nghiệp vụ, không phải lựa chọn trình bày.

**2.2 BR-O4 — giới hạn mua là của SERVER, không có default trong UI.** "Tối đa `max_per_order` (mặc
định 4) vé/đơn và `max_per_identity` (mặc định 4) vé/người/sự kiện, tính gộp trên mọi đơn đã PAID."

Hai con số, và cái thứ hai **phụ thuộc lịch sử mua** nên UI không thể tự tính. `TicketTierCard` vì vậy
nhận `maxSelectable` đã được server tính sẵn = `min(max_per_order, max_per_identity - đã mua)`.
**Không có giá trị mặc định 4 trong component**: "mặc định 4" là default của *hệ thống*, không phải
thứ UI được phép đoán. Thiếu prop → không cho chọn số lượng.

**2.3 BR-O3 — một khách chỉ có 1 hold đang hoạt động mỗi sự kiện.** "Tạo hold mới → hold cũ bị huỷ và
trả kho ngay." Hệ quả cho UI: khi đang có hold hoạt động, chọn hạng vé khác **phải** cảnh báo rằng
hold hiện tại sẽ bị huỷ. Không cảnh báo thì khách mất chỗ đang giữ mà không hiểu vì sao.

**2.4 Không bịa tiền tệ.** DESIGN.md chốt định dạng `1.250.000 đ`, nhưng `formatVnd` hard-code hậu tố
`đ`. Nếu server trả `currency: "USD"` mà vẫn in `đ` thì đó là **sai số tiền**. `formatMoney` vì vậy:
- `currency === "VND"` → dùng `formatVnd`
- khác → in số đã nhóm + **mã tiền tệ**, không bịa ký hiệu
- `currency` thiếu → **không** đoán là VND; trả `"—"`

**2.5 Không bịa tổng.** Handoff §11.6: "Display fees/taxes only if supplied. Do not … invent totals."
`OrderSummary` tính `subtotal = Σ(unitAmount × quantity)`. Phí/thuế **chỉ** hiển thị khi được truyền;
không truyền thì `total === subtotal` và **không** render dòng phí bằng 0 (dòng "Phí: 0 đ" là khẳng
định rằng không có phí, mà ta không biết điều đó).

**2.6 Trạng thái "không rõ kết quả" là trạng thái thật.** Handoff §11.6 yêu cầu "ambiguous request
states", và BR-O5 bắt mọi API ghi dùng `Idempotency-Key` chính vì request có thể không biết kết quả.
Khi tạo hold mà không rõ thành công hay chưa, UI **không được** nói thành công, cũng **không được** nói
thất bại, và **không được** mời bấm lại bằng một request mới — bấm lại với key mới là nguy cơ tạo hold
trùng.

**2.7 Đồng hồ giữ vé không được reset khi rerender.** Dùng `ServerExpiryCountdown` variant `"hold"`
với `expiresAt` của server (BR-O2). Không tự tính 10 phút.

## 3. Phát hiện: handoff §11.2 thiếu 2 trạng thái sự kiện

Handoff §11.2 liệt 4 nhãn: `scheduled / on-sale / sold-out / cancelled`.

State machine ở `docs/01-nghiep-vu.md` dòng 22–23 có **6** trạng thái công khai:
`SCHEDULED → ON_SALE → SOLD_OUT | CLOSED → COMPLETED`, và nhánh `CANCELLED (hoàn tiền toàn bộ)`.

Thiếu `CLOSED` (hết giờ bán, vé có thể vẫn còn) và `COMPLETED` (sự kiện đã diễn ra). Hai cái này
**khác** `SOLD_OUT` và khác nhau: `CLOSED` là không bán nữa, `COMPLETED` là đã xong. Thông điệp cho
khách khác hẳn nhau.

`docs/01` là nguồn chuẩn, handoff tự nhận là đề xuất UX → làm **6 trạng thái** và báo lại.

Đây là lần thứ **ba** cùng một kiểu lệch (trước đó: handoff §11.3 thiếu `DROPPED`; `docs/03` §6 thiếu
route group cho Moderator). Nên coi là vấn đề hệ thống của bộ tài liệu, không phải ba sự cố rời rạc.

## 4. AC

### AC-1 — `formatMoney`
- `(1250000, "VND")` → `"1.250.000 đ"`
- `(1250000, "USD")` → số đã nhóm + `"USD"`, **không** chứa `"đ"`
- `currency` thiếu / rỗng → `"—"`; **không** mặc định VND
- amount không hữu hạn → `"—"`, không bao giờ `"NaN"`
- `(0, "VND")` → `"0 đ"`

### AC-2 — `EventCard`: 6 trạng thái
- **Then** phủ `SCHEDULED ON_SALE SOLD_OUT CLOSED COMPLETED CANCELLED`
- **Then** mỗi trạng thái có `Badge` kèm **text**; `CLOSED` và `COMPLETED` có thông điệp **khác nhau** và khác `SOLD_OUT`
- **Then** `CANCELLED` nói rõ được hoàn tiền toàn bộ (docs/01 dòng 23)
- **Then** đúng **một** link chính (handoff §11.2 "one primary link"); thẻ không phải một link bọc cả khối (click target không mơ hồ)
- **Given** `href` không truyền, **Then** **không** render link (không link chết)
- **Then** có vùng ảnh; thiếu `imageUrl` thì render placeholder có `aria-hidden`, **không** render `<img>` với `src` rỗng
- **Then** `SOLD_OUT`/`CLOSED`/`COMPLETED`/`CANCELLED` giảm nhấn thị giác nhưng **vẫn hiển thị** (DESIGN.md: giữ bố cục, không ẩn thẻ)

### AC-3 — `EventCard`: skeleton
- **Given** `loading`, **Then** render skeleton có `aria-busy="true"`, **không** render tên/giá/link, và **không** hiện bất kỳ nhãn trạng thái nào
- **Then** skeleton tôn trọng `prefers-reduced-motion`

### AC-4 — `TicketTierCard`: giá và tồn kho
- **Then** giá qua `formatMoney`; `currency` do caller truyền, **không** mặc định
- **Then** `availability` chỉ nhận `"available" | "limited" | "sold_out"`; component **không** có prop số vé còn lại
- **Given** `availability="limited"`, **Then** nhãn là định tính ("Sắp hết"), **không** chứa con số
- **Given** `availability` thiếu, **Then** **không** khẳng định còn vé; hiện "đang cập nhật"

### AC-5 — `TicketTierCard`: chọn số lượng theo BR-O4
- **Given** `maxSelectable` thiếu, **Then** **không** render điều khiển chọn số lượng
- **Given** `maxSelectable={3}`, **Then** không chọn được quá 3; nút tăng bị disable ở 3
- **Then** không giảm xuống dưới 0; nút giảm disable ở 0
- **Given** `maxSelectable={0}`, **Then** không render điều khiển (hết suất mua của người này)
- **Given** `availability="sold_out"` hoặc `disabled`, **Then** không render điều khiển và không gọi `onQuantityChange`
- **Then** điều khiển có nhãn đọc được; giá trị hiện tại thông báo được cho screen reader
- **Then** mọi lần đổi gọi `onQuantityChange` với số mới

### AC-6 — `TicketTierCard`: chọn khi đang có hold khác (BR-O3)
- **Given** `hasActiveHoldElsewhere`, **Then** trước khi đổi lựa chọn phải có cảnh báo rằng hold hiện tại sẽ bị huỷ và trả kho
- **Then** cảnh báo là **text**, không chỉ màu

### AC-7 — `TicketTierCard`: trạng thái chọn
- **Given** `selected`, **Then** có `aria-pressed` hoặc `aria-selected` đúng trên điều khiển chọn — **không** chỉ đổi viền (DESIGN.md mô tả viền 2px, nhưng viền một mình không đến được screen reader)

### AC-8 — `OrderSummary`: dòng tiền
- **Then** mỗi dòng hiện tên hạng, số lượng, đơn giá, và tổng dòng
- **Then** `subtotal = Σ(unitAmount × quantity)`
- **Given** `fees` thiếu, **Then** **không** render dòng phí và `total === subtotal`
- **Given** `fees` có, **Then** render và `total = subtotal + fees`
- **Then** không render dòng thuế nào nếu không được truyền
- **Given** `items` rỗng, **Then** hiện trạng thái rỗng, **không** hiện `"0 đ"` như một tổng hợp lệ
- **Then** mọi số tiền qua `formatMoney` với cùng `currency`; trộn nhiều `currency` trong `items` → hiện lỗi, **không** cộng lẫn

### AC-9 — `OrderSummary`: 6 trạng thái hold
- **Then** phủ `idle | creating | active | expired | sold_out | ambiguous`
- **`creating`**: nút chính disable, có chỉ báo, **không** nói đã giữ được vé
- **`active`**: render `ServerExpiryCountdown` variant **`"hold"`** (không phải `"admission"`), `expiresAt` từ prop
- **`expired`**: nói rõ vé đã được trả kho và cần chọn lại; nút chính **không** còn là "thanh toán"
- **`sold_out`**: không mời thanh toán
- **`ambiguous`**: **không** nói thành công, **không** nói thất bại, **không** render nút tạo hold mới; chỉ mời chờ hệ thống xác nhận (BR-O5)

### AC-10 — `OrderSummary`: sticky & mobile
- **Then** trên mobile là dạng thu gọn mở rộng được, nhưng nút chính **luôn hiển thị** (handoff §11.6: "without hiding the primary action")
- **Then** vùng mở rộng có `aria-expanded` + `aria-controls` trỏ phần tử tồn tại

### AC-11 — Không hồi quy, gates
- `npm run lint`, `typecheck`, `build`, `test:coverage` pass; 830 test hiện có vẫn pass
- grep màu hard-code trong 3 file mới: rỗng

## 5. Hợp đồng API

```ts
// lib/money.ts
export function formatMoney(amount: number, currency: string | undefined | null): string

// components/event/event-card.tsx
export type EventPublicState =
  | "SCHEDULED" | "ON_SALE" | "SOLD_OUT" | "CLOSED" | "COMPLETED" | "CANCELLED"
export interface EventCardProps {
  state: EventPublicState
  title: string
  venue?: string
  startsAt?: number | string      // ISO co mui gio hoac epoch ms
  imageUrl?: string
  href?: string                   // khong co -> KHONG render link
  loading?: boolean
  className?: string
}

// components/checkout/ticket-tier-card.tsx
export type TierAvailability = "available" | "limited" | "sold_out"
export interface TicketTierCardProps {
  name: string
  unitAmount: number
  currency: string
  availability?: TierAvailability          // thieu -> "dang cap nhat"
  /** = min(max_per_order, max_per_identity - da mua), do SERVER tinh (BR-O4).
   *  Thieu -> KHONG render dieu khien chon so luong. KHONG co default. */
  maxSelectable?: number
  quantity?: number
  onQuantityChange?: (next: number) => void
  selected?: boolean
  disabled?: boolean
  /** BR-O3: dang co hold o hang ve khac -> phai canh bao hold cu se bi huy. */
  hasActiveHoldElsewhere?: boolean
  className?: string
}

// components/checkout/order-summary.tsx
export type HoldState = "idle" | "creating" | "active" | "expired" | "sold_out" | "ambiguous"
export interface OrderSummaryItem {
  tierName: string
  quantity: number
  unitAmount: number
  currency: string
}
export interface OrderSummaryProps {
  items: OrderSummaryItem[]
  holdState: HoldState
  holdExpiresAt?: number | string
  offsetMs?: number
  holdWarningThresholdMs?: number
  fees?: number                     // thieu -> KHONG render dong phi
  primaryActionLabel?: string
  onPrimaryAction?: () => void
  className?: string
}
```

## 6. Rủi ro

| Rủi ro | Giảm thiểu |
|---|---|
| Lỡ thêm prop số vé còn lại cho "đẹp" | AC-4 cấm; `TierAvailability` là union 3 nhãn |
| In `đ` cho tiền không phải VND | AC-1 có test riêng cho USD |
| Render "Phí: 0 đ" khi không biết có phí | AC-8 bắt không render dòng phí khi thiếu prop |
| `ambiguous` bị gộp vào `expired` hoặc có nút thử lại | AC-9 bắt không có nút tạo hold mới |
| Mặc định `maxSelectable = 4` | AC-5 bắt thiếu prop thì không render điều khiển |
| Trạng thái chọn chỉ thể hiện bằng viền | AC-7 bắt có `aria-pressed`/`aria-selected` |

## 7. Điểm dừng báo người dùng

- Nếu cần một trạng thái sự kiện công khai nào ngoài 6 cái của `docs/01` → dừng, hỏi
- Nếu `formatMoney` cần hỗ trợ tiền tệ thứ ba có ký hiệu riêng → dừng, hỏi; không tự thêm ký hiệu
