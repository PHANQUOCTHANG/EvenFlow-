# EventFlow — UI/UX & Stitch Handoff

> Tài liệu handoff cho Product → UX → Design → Stitch → React/Next.js.  
> Bản thiết kế là đề xuất để tạo prototype; các business rule đã có trong dự án là nguồn chuẩn. Không coi số liệu demo hoặc trạng thái prototype là dữ liệu thật.

## 1. Cách sử dụng tài liệu này

1. Dán **Global Stitch Prompt** ở mục 8 vào Stitch để tạo nền tảng hình ảnh và bộ khung.
2. Tạo từng page bằng prompt tương ứng ở mục 10; luôn đính kèm Global Prompt làm context.
3. Tạo component độc lập bằng mục 11 nếu cần kiểm soát component hoặc trạng thái.
4. Dùng dữ liệu demo được ghi rõ là demo; không để Stitch tự tạo thêm quy tắc nghiệp vụ.
5. Review thiết kế theo checklist mục 12 trước khi handoff cho React/Next.js.

Các prompt chính được viết bằng tiếng Anh để dễ tái sử dụng trong công cụ thiết kế. Nội dung giao diện sản phẩm mặc định là tiếng Việt, trừ khi prompt page yêu cầu thêm bản English.

### Nhãn mức độ chắc chắn

- **Đã đặc tả:** có căn cứ trong tài liệu nghiệp vụ, kiến trúc hoặc backlog hiện tại.
- **Đề xuất UX/UI:** hướng thiết kế để tạo prototype; cần Product/Design xác nhận trước khi biến thành yêu cầu sản phẩm.
- **Chưa triển khai:** có trong định hướng/backlog nhưng giao diện hoặc service chưa được nối thành trải nghiệm end-to-end.

## 2. Product context

### Sản phẩm

EventFlow là nền tảng bán vé sự kiện trực tuyến được thiết kế cho đợt mở bán có lượng truy cập tăng đột biến. Hệ thống có phòng chờ ảo, checkout có giới hạn thời gian, chống bot, xử lý thanh toán bất đồng bộ và AI hỗ trợ khách/vận hành.

Đợt mở bán mẫu trong tài liệu: khoảng 300.000 người truy cập đồng thời để mua khoảng 20.000 vé. Đây là **mục tiêu thiết kế/kiểm thử được tài liệu nêu**, không phải bằng chứng hệ thống hiện đã đạt tải production.

### Giá trị sản phẩm cần truyền đạt qua UI

1. **Công bằng:** người vào trước giờ mở bán được đưa vào lobby; đến giờ mở bán lobby được xáo trộn để cấp thứ tự; người đến sau nối FIFO.
2. **Minh bạch:** trạng thái hàng chờ, vé, giữ chỗ và thanh toán phải dễ hiểu; không hứa một ETA hoặc số tồn kho thiếu căn cứ.
3. **An tâm:** giữ vé có thời hạn rõ ràng; trạng thái thanh toán pending khác với thành công; vé phát hành có thể xem lại.
4. **Chịu lỗi tốt:** mất kết nối không tự động đồng nghĩa mất vị trí; lỗi cần có hướng dẫn tiếp tục thay vì chỉ hiện mã kỹ thuật.
5. **Kiểm soát vận hành:** Organizer, Moderator và Ops có bề mặt thao tác riêng, quyền hạn và rủi ro khác Buyer.

### Người dùng và mục tiêu

| Vai trò | Mục tiêu chính | Các tác vụ UI trọng tâm |
|---|---|---|
| Buyer | Mua vé công bằng, an toàn, ít mơ hồ | Tìm sự kiện, vào queue, theo dõi lượt, giữ vé, thanh toán, xem vé |
| Organizer | Tạo và quản lý sự kiện | Cấu hình thông tin/vé/lịch, gửi duyệt, theo dõi trạng thái và báo cáo |
| Moderator | Ra quyết định kiểm duyệt có căn cứ | Xem nội dung/điểm AI/lý do, duyệt hoặc từ chối, xử lý khiếu nại |
| Ops | Bảo vệ hệ thống trong thời gian mở bán | Theo dõi queue/checkout/AI, điều chỉnh admit rate, bật suy biến hoặc tạm dừng |
| AI worker | Hỗ trợ tác vụ, không có quyền quyết định vượt policy | Trả lời có giới hạn, gợi ý kiểm duyệt/rule, tạo báo cáo; các quyết định nhạy cảm có người kiểm soát |

### Scope màn hình đề xuất

**MVP Buyer:** danh sách sự kiện → chi tiết sự kiện → đăng nhập/xác minh nếu cần → waiting room → chọn vé/giữ vé → checkout → kết quả và vé.

**MVP nội bộ:** đăng nhập theo vai trò → Organizer tạo/quản lý sự kiện → Moderator duyệt → Ops war room tối thiểu.

**Sau MVP / backlog:** báo cáo nâng cao, khiếu nại anti-bot, phê duyệt rule do AI đề xuất, check-in/door staff, i18n đầy đủ.

## 3. System context và giới hạn khi thiết kế

### Kiến trúc nhìn từ UI

```text
Next.js 15 / React (web buyer + organizer + ops)
        │
        ├── API Gateway (Go): xác thực, rate limit, load shedding, routing
        ├── Identity/Event services (Go)
        ├── Waiting Room (Go + Redis): join, status, SSE, admit
        ├── Ticketing (Go + Redis + PostgreSQL): hold, order, inventory
        ├── Payment (Go): payment state, webhook, reconcile
        ├── Anti-bot (Go): challenge / slow lane / block signals
        └── AI worker (Python + RabbitMQ + Redis): chat, moderation, reports
```

### Hợp đồng và sự thật nghiệp vụ đã được đặc tả

- Trang public event được định hướng ISR/CDN; số lượng vé public chỉ nên là snapshot/nhãn khái quát, không query DB theo mỗi lượt xem.
- Queue join cần identity đã xác thực OTP; join idempotent theo identity/event. Queue token không được tạo cảm giác có thể chia sẻ để nhân vị trí.
- `LOBBY` trước T0 không hiển thị thứ tự giả. Sau T0 có thứ tự; polling tuân theo `poll_after_ms` do server trả về. Near-turn có thể dùng SSE; các trạng thái không được giả lập như đã được backend xác nhận.
- Khi admit, quyền checkout có TTL 15 phút theo nghiệp vụ; hold vé 10 phút từ `expires_at` do server trả về. **Đây là hai bộ đếm/trạng thái khác nhau**, không gộp làm một timer.
- Mọi thao tác ghi checkout cần idempotency. Vé/đơn chỉ hiện PAID/ISSUED khi backend xác nhận; redirect từ cổng thanh toán chưa đủ chứng minh đã thanh toán.
- AI assistant gửi yêu cầu bất đồng bộ, có giới hạn 10 câu/15 phút; FAQ/degraded mode có thể trả lời thay Gemini. UI cần thể hiện “đang xử lý” hoặc “chế độ FAQ”, không chờ request vô hạn.
- Anti-bot có nhiều kết quả: allow, challenge, slow lane, block. Không nên tiết lộ scoring/rule nội bộ hoặc hướng dẫn cách né phát hiện.
- Ops có quyền thay đổi admit rate/degradation/pause; thao tác ảnh hưởng lớn cần xác nhận, hiển thị người thực hiện và kết quả từ server.
- Chưa có Flutter app trong repository. Bộ prompt này mô tả web responsive; chưa coi thiết kế là đặc tả native mobile.

### API / implementation hiện trạng cần lưu ý

- Backend waiting room đã có REST handler cho join, status và stream; frontend có queue client polling. Đây chưa phải bằng chứng toàn bộ trải nghiệm Buyer đã hoàn chỉnh.
- Ticketing có endpoint tạo hold; các route còn lại trong buyer checkout cần xác minh theo OpenAPI/service trước khi code UI.
- Các màn Organizer, Moderator, Ops và nhiều service được backlog mô tả nhưng một phần chưa triển khai.
- Stitch nên tạo prototype dựa trên **UI state model**; không tự suy diễn endpoints, giá, tồn kho chính xác, thông tin thanh toán hoặc quyền truy cập.

## 4. Information architecture và luồng

### Public / Buyer

```text
/events
  └── /events/[slug]
       ├── chưa đăng nhập/OTP → /auth/sign-in → /auth/verify
       └── /waiting/[eventId]
             ├── LOBBY → QUEUED → ADMITTED
             ├── SOLD_OUT → kết thúc hoặc waitlist (chỉ nếu API hỗ trợ)
             └── /checkout/[eventId]
                   ├── chọn vé + tạo hold
                   ├── /checkout/payment/[orderId]
                   └── /orders/[orderId]
                         ├── pending / success / failure / expired
                         └── /tickets/[orderId]
```

### Organizer

```text
/organizer
  ├── /organizer/events
  ├── /organizer/events/new
  ├── /organizer/events/[eventId]/edit
  ├── /organizer/events/[eventId]/review-status
  └── /organizer/events/[eventId]/sales
```

### Moderator

```text
/moderator
  ├── /moderator/reviews
  ├── /moderator/reviews/[eventId]
  └── /moderator/appeals
```

### Ops

```text
/ops
  ├── /ops/events/[eventId] (war room)
  └── /ops/ai (AI health / degradation)
```

> Các route trên là **đề xuất thông tin kiến trúc frontend**, không khẳng định route/API đang tồn tại.

## 5. UX principles và state model

### Nguyên tắc trải nghiệm

1. **Status first:** đầu trang luôn trả lời “đang ở trạng thái nào?” trước khi đưa biểu đồ, dữ liệu phụ hoặc CTA.
2. **One primary action:** mỗi trạng thái có tối đa một CTA chính; hành động phụ phân cấp rõ.
3. **No fake certainty:** ETA, thứ hạng, tồn kho, thanh toán, kiểm duyệt đều chỉ hiển thị khi có nguồn dữ liệu. Nếu thiếu, ghi “đang cập nhật” hoặc không hiển thị.
4. **Explain, then ask:** trước CAPTCHA/challenge, giải thích cần xác minh thêm nhưng không lộ logic anti-bot.
5. **Resilient by default:** giữ session/token trong phạm vi bảo mật, có retry theo server/backoff; không bắt người dùng bấm refresh liên tục.
6. **Critical action friction:** xác nhận pause sale, đổi admit rate, reject event, refund hoặc block; tránh xác nhận cho hành động không phá huỷ.
7. **Accessible at high stress:** typography dễ đọc, tương phản AA, trạng thái không chỉ phân biệt bằng màu, focus/keyboard hoạt động.
8. **Avoid dark-pattern urgency:** timer chỉ xuất hiện khi có server `expires_at`; không dùng countdown giả cho sale hoặc inventory.

### Buyer UI state inventory

| Feature | Trạng thái tối thiểu |
|---|---|
| Event | scheduled / countdown / on sale / sold out / closed / cancelled / data unavailable |
| Auth | sign-in / OTP input / resend cooldown / invalid code / rate-limited |
| Queue | lobby / queued / reconnecting / admitted / expired / sold out / challenge / unavailable |
| Ticket selection | loading / available / limit reached / sold out / hold creating / hold active / hold failed |
| Payment | ready / redirecting / waiting confirmation / success / failed / expired / reconciliation pending |
| Assistant | idle / FAQ result / queued / responding / degraded FAQ / quota reached / failed |

### Internal dashboard state inventory

Mọi dashboard cần có loading, empty, stale data, partial outage, permission denied, action pending, action success/failure. Biểu đồ phải ghi đơn vị, thời gian, nguồn hoặc trạng thái freshness; không dùng mock real-time animation để giả số liệu thật.

## 6. Visual direction — design proposal

Đây là hướng thẩm mỹ đề xuất để Stitch bắt đầu; cần được Senior Designer/Product xác nhận trước khi áp dụng làm design system chính thức.

### Brand expression

- **Buyer storefront:** đáng tin, hiện đại, giàu năng lượng sự kiện nhưng không giống sàn flash-sale.
- **Queue/checkout:** bình tĩnh, có trật tự, thông tin ưu tiên hơn trang trí.
- **Organizer/Moderator/Ops:** công cụ SaaS chuyên nghiệp, dense vừa phải, ưu tiên khả năng quét nhanh và kiểm soát rủi ro.
- Tránh: glassmorphism dày, neon toàn trang, gradient nền lòe loẹt, carousel tự chạy, skeleton dài không cần thiết, biểu đồ không có đơn vị, badge “còn 1 vé” nếu không có nguồn dữ liệu chính xác.

### Token khởi tạo (đề xuất; Stitch có thể dùng làm design seed)

| Token | Giá trị đề xuất | Cách dùng |
|---|---|---|
| `color.brand.900` | `#18243A` | Navy cho header, text nhấn, chrome nội bộ |
| `color.brand.700` | `#34496B` | Secondary brand |
| `color.action.600` | `#5B55E7` | CTA chính, focus, link quan trọng |
| `color.action.100` | `#EEEDFF` | Surface nhấn nhẹ |
| `color.accent.500` | `#E6A23C` | Notice/điểm nhấn sự kiện, dùng tiết chế |
| `color.success.700` | `#18794E` | Success |
| `color.warning.700` | `#9A5B00` | Warning |
| `color.danger.700` | `#B42318` | Error/destructive |
| `color.text.primary` | `#182230` | Nội dung chính |
| `color.text.secondary` | `#526071` | Nội dung phụ |
| `color.surface` | `#FFFFFF` | Card/dialog |
| `color.canvas` | `#F5F7FA` | Nền app |
| `color.border` | `#DCE2EA` | Border/divider |
| `radius.control` | `10px` | Button/input |
| `radius.card` | `16px` | Card |
| `radius.panel` | `20px` | Hero/large panel |
| `shadow.card` | subtle, low contrast | Card nổi nhẹ |

**Contrast note:** màu token là giá trị gợi ý; cần kiểm tra WCAG AA thực tế trên text size/state trước khi chốt. Trạng thái phải có icon/label, không chỉ màu.

### Typography, spacing, layout

- Sans-serif: Inter, Geist Sans hoặc font sans hiện có trong sản phẩm; tối đa 2 family.
- Body 16px / line-height thoáng; dashboard có thể dùng 14px cho metadata nhưng không thu nhỏ dữ liệu quan trọng.
- Hệ spacing bội 4/8; content width public 1120–1200px; dashboard desktop dùng sidebar 240–264px và topbar.
- Mobile: 360px minimum viewport; không để CTA checkout hoặc queue status bị đẩy sâu dưới fold. Tablet 768px; desktop 1280px trở lên.
- Public pages: max width, nhiều khoảng trắng, hero ảnh sự kiện có overlay đủ contrast.
- Internal dashboards: grid 12 cột desktop; data tables chuyển thành stacked rows/card ở màn nhỏ; không ép bảng ngang nếu thông tin có thể tái cấu trúc.

### Component visual rules

- Button: primary, secondary, tertiary, destructive; có hover/focus/pressed/disabled/loading.
- Input: label luôn nhìn thấy, helper/error text gắn đúng field, không dùng placeholder thay label.
- Card: phân cấp heading → content → action; click target không mơ hồ.
- Badge: text + icon nếu trạng thái quan trọng.
- Timer: monospaced/tabular numbers; thông báo bằng text khi sắp hết hạn; hỗ trợ reduced motion.
- Chart: title, unit, range selector, tooltip, empty/stale state, accessible table summary.
- Dialog: title mô tả hậu quả, CTA có động từ cụ thể, destructive action không đặt cạnh primary thường.

## 7. Handoff / technical design constraints

- Web target: **Next.js App Router + React + TypeScript**. Stitch là công cụ tạo thiết kế/prototype, không phải nguồn contract nghiệp vụ.
- Giữ nguyên semantic state name khi mapping UI: `LOBBY`, `QUEUED`, `ADMITTED`, `EXPIRED`, `SOLD_OUT`; order/payment state phải lấy từ API enum thực tế trước khi implement.
- UI queue dùng `poll_after_ms`, ETag/304, `Retry-After` và SSE nếu service contract cho phép; không hardcode polling mỗi giây.
- Countdown hold đọc `expires_at` từ server. Dùng server offset/time API khi có; không kéo dài hold vì tab background hoặc client clock.
- Mọi mutation có loading, duplicate-submit prevention, idempotency key theo contract, error recovery. Không tự retry mutation bằng key mới khi kết quả chưa rõ.
- Dùng design tokens và component variants; không lặp CSS riêng theo từng page cho cùng một control.
- Charts/tables không hiển thị dữ liệu thật giả; prompt ghi rõ mọi số liệu trong Stitch là “sample/demo”.
- Không thiết kế provider payment cụ thể, OTP provider, account recovery hoặc waitlist API nếu chưa được xác nhận.
- Không đưa identity, email, phone, queue token, QR signature, raw anti-bot score hoặc PII vào ảnh mockup/public screen.

## 8. Global Stitch Prompt (paste once as the project brief)

```text
Design a cohesive responsive web product called EventFlow, a high-scale event ticketing platform with a fair virtual waiting room, time-limited ticket holds, asynchronous payment confirmation, AI assistance, and separate Organizer, Moderator, and Operations workspaces.

PRODUCT CONTEXT
The system is designed for sudden ticket-sale traffic spikes. Its product promises are fairness, transparent status, no overselling, resilient queue behavior, and trustworthy payment state. The repository currently contains a Next.js 15 / React web shell and partial Go/Python backend capabilities; treat this as a design prototype, not a claim that every workflow or API is already implemented.

USERS
1) Buyer: discovers events, joins a queue, selects and temporarily holds tickets, pays, and retrieves issued tickets.
2) Organizer: creates events and ticket tiers, submits them for review, and monitors sales.
3) Moderator: reviews AI-assisted event moderation and handles appeals.
4) Ops: monitors a live sale and can control admission rate, pause/resume admission, and change AI degradation mode.

NON-NEGOTIABLE DOMAIN RULES
- Before sale start, queue entrants are in a LOBBY without a personal rank. At sale start, the lobby is randomized; later entrants join FIFO. Never imply that arriving earlier within the lobby guarantees a better rank.
- Queue position, ETA, polling interval, admission, inventory, payment state, and hold expiry are server-owned. Never invent a precise value or imply a client-side estimate is authoritative.
- Use adaptive polling based on the server-provided poll_after_ms. SSE may be used for near-turn updates. A network disconnect does not automatically mean the buyer lost their place.
- An admitted checkout slot has a 15-minute TTL. A ticket hold has a separate 10-minute TTL based on server expires_at. Do not combine these timers.
- A payment redirect/return is not proof of payment. Show pending confirmation until the backend confirms success.
- AI chat is asynchronous, quota-limited, and may degrade to FAQ. The assistant must not promise tickets, invent queue positions/ETAs, or disclose exact inventory without trusted live data.
- Anti-bot may require a generic additional verification, but never reveal internal risk scores or evasion rules.
- Destructive or high-impact Ops/Moderator actions require a clear confirmation and an explicit consequence.

VISUAL DIRECTION
Use a premium, trustworthy event-commerce visual language. The buyer storefront can feel energetic through photography and restrained warm accents; queue and checkout screens should feel calm and operationally clear. Internal consoles should look like polished enterprise SaaS: dense enough for work, but not visually noisy.
Suggested initial palette: deep navy #18243A, action indigo #5B55E7, pale indigo #EEEDFF, warm amber #E6A23C, canvas #F5F7FA, white surfaces, primary text #182230, secondary text #526071, success #18794E, warning #9A5B00, danger #B42318. Treat these as proposed design tokens and verify WCAG AA contrast.
Use a clean sans-serif such as Inter or Geist Sans, a 4/8px spacing rhythm, 10px control radius, 16px card radius, subtle borders and low-elevation shadows. Avoid excessive gradients, glassmorphism, neon, auto-rotating carousels, fake urgency, and decorative charts.

RESPONSIVE AND ACCESSIBILITY
Design desktop first for 1440px, then show responsive behavior at 1024px, 768px, and 390px. Keep queue status and primary actions visible on mobile. Support keyboard navigation, visible focus, WCAG AA contrast, reduced motion, readable status text, and icons/labels in addition to color. Do not use placeholder text as the only field label.

DESIGN SYSTEM
Create reusable button, input, select, checkbox, radio, badge, alert, card, modal, dropdown, tabs, table, pagination, toast, skeleton, empty state, error state, queue status panel, countdown, chart, and navigation patterns. Include default, hover, focus, pressed, disabled, loading, success, warning, and error states where relevant. Keep UI copy in Vietnamese by default; use realistic concise Vietnamese content.

DATA AND PROTOTYPE RULES
Label all numbers, attendee counts, ticket counts, prices, ETAs, charts, and account details as sample/demo data. Do not present prototype data as live truth. Do not invent new business rules, APIs, payment providers, refund behavior, waitlist capability, account recovery, or permissions. If an API-owned value is unavailable, show a clear loading/stale/unknown state rather than fabricate it.

OUTPUT
Produce a coherent multi-page design system and high-fidelity screens with reusable components, named states, responsive layouts, and clear annotations. For every page, show the main state plus the important loading, empty, error, and edge states. Maintain shared navigation, spacing, typography, colors, and interaction patterns across Buyer and internal workspaces.
```

## 9. Prompt template for each page

Copy this template and append one page-specific prompt from section 10.

```text
Use the EventFlow Global Stitch Prompt as the source of truth. Design the following page as part of the same product and reuse its design tokens and components.

Page: [PAGE NAME]
Primary user and task: [USER + JOB TO BE DONE]
Entry point: [HOW THE USER ARRIVES]
Primary success outcome: [WHAT SUCCESS LOOKS LIKE]
Data available to this screen: [ONLY KNOWN SERVER/API DATA]
Required states: default, loading, empty, error, stale/connection lost, and the page-specific states listed below.
Primary action: [ONE ACTION]
Secondary actions: [ACTIONS]
Responsive requirements: desktop 1440px, tablet 768px, mobile 390px.
Accessibility: keyboard, visible focus, WCAG AA, reduced motion, semantic status announcements.

Do not invent API fields, permissions, inventory, ETA, payment success, or business policy. Mark every prototype value as demo. Return the high-fidelity screen, reusable components, responsive layout, and an interaction/state annotation.
```

## 10. Page-specific Stitch prompts

### A. Buyer pages

#### 10.1 Event discovery / event listing

```text
Design the EventFlow public event discovery page in Vietnamese.
User: a ticket buyer browsing upcoming live events.
Show a clean public header with EventFlow logo, event search, category filters, date filter, and sign-in entry. Create a clear page title, featured event area, and responsive event-card grid. Each event card may show event artwork, title, venue/city, event date, sale status, and a “View event” action. Use only broad availability labels such as “Sắp mở bán”, “Đang mở bán”, or “Đã hết vé”; do not show exact remaining inventory.
Include loading skeletons, no-results state with reset filters, unavailable-data state, and mobile filter drawer. Keep the page content cache/CDN-friendly: no personalized queue state or rapidly changing exact inventory on this page. All event content is demo data.
```

#### 10.2 Event detail / sale landing

```text
Design a public event detail page for a high-demand concert. Create an editorial hero with event image, event name, performer, venue, date/time, and a concise event summary. Below it, show event information, ticket-tier overview with prices and broad availability status, sale opening time, purchase limits if provided, and clear terms/help links.
Use a server-synchronized countdown only when a trusted sale start time is available. Primary action should change by event state: “Nhắc tôi” only if a reminder feature is confirmed; “Vào phòng chờ” for eligible upcoming sale; “Đang mở bán”/join queue during sale; disabled or explanatory sold-out/cancelled state otherwise. Do not invent a waitlist or reminder API. Never promise access to a ticket.
Include share action, loading, missing event, sold out, cancelled, and not-yet-scheduled variants. Responsive layout should keep event title, sale state, and primary action easy to find on mobile.
```

#### 10.3 Sign-in

```text
Design a focused EventFlow sign-in page or modal in Vietnamese. Support the identity credential fields that the product team confirms; do not assume passwordless, social login, or a particular provider. Include visible labels, password visibility if password entry is used, validation, pending submit, invalid credentials, rate-limit state, and a link to registration only if registration is in scope.
Explain briefly that account verification is required before joining a queue. Do not show a queue token or sensitive identity data. Keep event context visible as a small return-to-event panel when sign-in was initiated from a sale.
```

#### 10.4 OTP verification

```text
Design the OTP verification step required before joining the waiting room. Use a clear six-digit code input pattern only as a proposed interaction (allow Product to change code length), show the destination in masked form only if the backend provides it, and provide a resend countdown only when server policy supports it.
Include invalid/expired code, resend pending, rate-limited, network error, and verified states. Do not invent SMS/email provider, resend policy, or recovery route. Main action: verify and return to the originating event/queue flow.
```

#### 10.5 Waiting room — lobby before sale start

```text
Design the EventFlow waiting-room pre-sale LOBBY state. This is a critical fairness state: explicitly say the buyer is in the lobby and that a queue position has not been assigned yet. Do not show rank, progress percentage, personal ETA, or imply that joining earlier within the lobby improves the chance of a better position. At sale start, the eligible lobby is randomized by the system.
Show event name and sale time, a calm “You are in the waiting room” status, a simple explanation, a connection/last-updated indicator, safe guidance about keeping the session available, and an accessible FAQ assistant entry. Do not encourage aggressive refreshing. Include browser/network reconnect state and an optional “leave” action only if the product confirms its consequence.
Create responsive desktop/mobile layouts and explicit states for loading, lobby active, connection reconnecting, event time unavailable, and event postponed/cancelled.
```

#### 10.6 Waiting room — queued

```text
Design the EventFlow QUEUED waiting-room state after a server rank has been assigned. Prioritize the server-provided rank, queue status, ETA only when supplied as a trusted value, and next update interval. Explain that the queue updates automatically and that refreshing is unnecessary. Include connection health and the “polling / live update” state without exposing technical details.
The screen must support rank updated, no ETA available, stale status, 503 with Retry-After, reconnecting, and sold-out outcomes. Do not render an invented progress percentage unless the server provides a defined denominator. Avoid fake precision and urgency.
Place the AI assistant in a secondary panel/drawer with FAQ chips such as “Tôi cần làm gì khi đang chờ?” and “Điều gì xảy ra khi đến lượt?”. The assistant must be visually distinct from official queue status.
```

#### 10.7 Waiting room — admitted / your turn

```text
Design the ADMITTED state as a clear transition from waiting to checkout. Show a prominent “Đến lượt bạn” status, a concise explanation, and a primary “Tiếp tục chọn vé” action. If and only if the server provides admitted expires_at, show the separate 15-minute admission window countdown; label it clearly as time allowed to enter checkout, not the ticket hold timer.
Include expired admission and sold-out states. Never imply admission guarantees a ticket. Preserve event context and explain the next step. Provide a visible but non-alarming notice that checkout availability depends on remaining inventory.
```

#### 10.8 Ticket selection and active hold

```text
Design the buyer ticket selection / hold page for an admitted user. Show ticket tiers, unit prices, quantity selector, purchase limit from event configuration, order summary, and a clear create-hold action. Do not expose exact live stock unless an authoritative API explicitly supplies it. Make sold-out/limit-reached states explicit.
After successful hold, switch to a distinct active-hold state with server expires_at countdown (10 minutes per current business specification), selected tier/quantity, amount, and clear next action to payment. Explain that the hold is temporary. Do not start or reset the timer on client navigation; it is anchored to the server hold expiry.
Include creating hold, hold success, hold conflict/sold out, validation error, network outcome unknown, hold expired, and recovery states. Prevent duplicate submit visually. If the outcome is ambiguous, instruct the user to check order status rather than creating a new hold blindly.
```

#### 10.9 Checkout / payment handoff

```text
Design the payment handoff page for a held ticket order. Show order summary, total, hold expiry from server, and a secure payment action. Do not assume a specific payment provider, card form, QR payment, or payment method. The page should clearly distinguish “Ready to pay”, “Redirecting”, and “Waiting for payment confirmation”.
On return from a payment provider, never show success based only on the browser redirect. Show a pending-confirmation state and a safe refresh/status-check action if supported. Include payment failed, hold expired during payment, temporary provider unavailable, and reconciliation pending states. Avoid instructing the buyer to pay again while the first result is unknown.
```

#### 10.10 Order result and ticket wallet

```text
Design an order result page that supports separate backend-confirmed states: payment pending, payment succeeded/ticket issued, payment failed, order expired, cancelled, and reconciliation pending. Use distinct hierarchy and copy for each state. Only show downloadable/viewable ticket QR when the backend confirms ticket issuance; use a placeholder visual and label it demo in Stitch. Never show real QR signatures or sensitive tokens.
For success, show event details, order reference, issued ticket count, and next action to view tickets. For pending, explain that confirmation can take time and the buyer should not pay again yet. For failure/expired, show only recovery actions supported by product policy; do not promise automatic refund or retry.
Include printable/mobile ticket presentation as a separate wallet detail state, with safe QR placeholder, event/time/venue, ticket status, and accessible fallback reference.
```

#### 10.11 Buyer AI assistant panel

```text
Design a reusable EventFlow buyer assistant drawer/panel, primarily used in the waiting room. Include welcome state, suggested FAQ chips, user/assistant message styling, pending async answer state, and a clear source/context boundary between authoritative queue status and AI explanation.
Support FAQ answer, queued/processing, degraded FAQ-only mode, rate-limit/quota reached, out-of-scope refusal, and service unavailable. The assistant must not promise tickets, invent ETA/rank/inventory, or claim to have changed an order. For dynamic data, show only backend-confirmed values; otherwise point to the official status panel. Use concise Vietnamese copy and show a subtle “AI hỗ trợ” label.
```

### B. Organizer pages

#### 10.12 Organizer dashboard and event list

```text
Design the EventFlow Organizer workspace. Use a persistent professional SaaS sidebar/topbar and clear account/workspace identity. The dashboard should summarize the organizer's own events by status, upcoming sale schedule, and high-level sales indicators only when available. The event list needs search/filter by status/date, sortable columns or responsive cards, and a clear “Tạo sự kiện” action.
Use explicit event lifecycle labels: draft, pending review, scheduled, on sale, sold out, closed/completed, rejected, cancelled. Do not let the UI imply the organizer can reduce ticket quota after sale starts. Include loading, empty first-use, stale metrics, permission denied, and API error states. All figures are sample.
```

#### 10.13 Create/edit event wizard

```text
Design a multi-step event creation/edit wizard for an Organizer: event basics, date/time/venue, ticket tiers and quota, sale schedule, review and submit. Provide a stepper with saved progress, field-level validation, draft save, and a review summary before submission.
Rules: scheduling requires at least one valid ticket tier with quota > 0 and price >= 0; sale start must be at least 30 minutes after approval; after ON_SALE, quota cannot be decreased. Present these as inline constraints, not surprise errors. AI moderation may route to auto-approved, needs-human, or rejected states; do not imply AI decisions are final where moderator override exists.
Do not invent image storage constraints, currency choices, or unconfirmed fields. Include unsaved-changes confirmation, save error, submit pending, and submit success states. Ensure complex forms work on tablet/mobile.
```

#### 10.14 Event moderation status and sales analytics

```text
Design an Organizer event detail workspace with tabs for overview, review status, ticket tiers, and sales/reporting. The moderation panel should show status, submitted time, AI result/reasons only as allowed by product policy, and human decision when available. Provide a clear distinction between “under review” and “approved”.
The analytics tab may include sales over time, ticket-tier breakdown, order funnel, and post-sale report sections (commercial, conversion, technical, fraud, experience, recommendations). Every chart needs units, time range, freshness, and sample-data label. Do not imply real-time precision if data is stale. Avoid suggesting the organizer can override moderation or Ops controls.
```

### C. Moderator pages

#### 10.15 Moderation review queue

```text
Design an internal Moderator review queue for events needing human review. Use a scannable table with event, organizer, submitted time, review status, priority, and age/SLA only if configured. Include filters, search, pagination, loading, empty queue, stale data, and error states.
Keep AI risk signals discreet and explainable; do not use an opaque score as the only basis for a decision. Ensure the page does not expose buyer PII unnecessarily. Each row opens an event review detail. All records are demo.
```

#### 10.16 Event review detail and decision

```text
Design a Moderator event review detail screen. Show event content and media, ticket/sale configuration, AI moderation result across the configured review dimensions (prohibited content, misleading claims, image rights, missing information), supporting reasons, and audit history. Keep original content distinct from AI-generated analysis.
Provide explicit Approve and Reject actions with required rationale where policy requires it, plus a way to override an AI decision. Confirm destructive/reject actions and show pending/result feedback. Do not let the AI act as the human decision-maker. Include loading, content unavailable, already reviewed/locked, and permission denied states.
```

#### 10.17 Anti-bot appeal review

```text
Design a Moderator anti-bot appeal screen showing the affected event, appeal reference, user-provided explanation, decision status, and explainable rule evidence permitted by policy. Minimize personal data. Do not expose exact thresholds or operational details that would enable evasion.
Provide review, uphold, or clear-block actions only if those permissions are confirmed. Require a reason for a decision and write an audit trail. Include empty, pending evidence, stale decision, duplicate appeal, and action failure states.
```

### D. Ops pages

#### 10.18 Live-sale war room

```text
Design an EventFlow Ops war room for one live event. Desktop-first, high information density with calm hierarchy. Show sale status, queue depth, current admit rate, checkout throughput, p99 latency, DB pool pressure, error rate, inventory/reconciliation health, payment webhook health, AI queue lag, and last-updated time only when these metrics are supplied. Every metric must include unit, time window, and freshness.
Include event selector, incident/status banner, trend charts, and a compact action panel. Keep read-only monitoring distinct from control actions. Do not show fabricated green health states or animate demo metrics as if live. Build stale/partial telemetry, service degraded, no active sale, and permission denied states.
```

#### 10.19 Admission controls and incident actions

```text
Design the Ops control panel for viewing/overriding admit_rate, pausing/resuming admission, and handling an emergency. Display current automatic/manual mode, current value and unit, last change, actor, and reason only if available. A manual override should show its effective status and expiration/clear behavior only if the backend defines them.
For pause/resume or high-impact rate changes, use a confirmation dialog with event name, consequence, current state, proposed value, and reason field. Never silently submit. Include pending action, success, validation failure, stale state conflict, and permission denied. Do not invent safe rate limits or backend controls.
```

#### 10.20 AI health and degradation controls

```text
Design an Ops AI health page showing queue lag by job type, provider health, limiter usage, event budget, cache hit rate, and current degradation mode only where telemetry exists. Show modes as NORMAL, SAVING, FAQ_ONLY, and OFF, with plain-language impact on buyer experience.
Changing mode requires explicit confirmation and audit context. Distinguish provider/API health from application status. Do not expose prompts, secrets, user conversation content, or raw PII in this console. Include partial metrics, stale data, provider rate-limited, and no permission states. All metrics are demo.
```

## 11. Reusable component prompts

### 11.1 Public navigation and internal app shell

```text
Create two shared responsive shells for EventFlow: (1) public buyer header/footer and (2) authenticated internal workspace with role-aware sidebar and topbar. Do not invent permission links; annotate role-specific nav as configurable. Include mobile menu, active route, focus/keyboard behavior, account menu, and compact states. Reuse the global tokens.
```

### 11.2 Event card and ticket tier card

```text
Create responsive EventCard and TicketTierCard components. EventCard supports scheduled/on-sale/sold-out/cancelled labels, artwork placeholder, event details, and one primary link. TicketTierCard supports name, price/currency supplied by data, broad availability if supplied, quantity selection when enabled, and disabled/sold-out/limit states. Never invent exact stock. Provide loading skeleton and accessible focus state.
```

### 11.3 Queue status panel / position visualization

```text
Create a reusable queue status panel with variants LOBBY, QUEUED, ADMITTED, EXPIRED, SOLD_OUT, RECONNECTING, and UNKNOWN. Lobby has no rank or personal ETA. Queued displays server rank and only server-provided ETA. Admitted displays admission expiry only if available. Include last updated, connection status, accessible live announcements, and no fake progress. Provide desktop and mobile layout.
```

### 11.4 Countdown component

```text
Create a reusable server-expiry countdown component with variants sale-start countdown, admitted-slot expiry, and ticket-hold expiry. Make the timer label explicit so the two checkout timers cannot be confused. Accept server timestamp/offset as input; do not imply the component owns or extends expiry. Include warning threshold as configurable (do not invent policy), expired state, paused/background tab behavior annotation, reduced motion, and screen-reader-friendly text.
```

### 11.5 Button, form fields, alerts, status badges

```text
Create a small accessible component library: primary/secondary/tertiary/destructive buttons; text/email/password/OTP fields; select/checkbox/radio; inline validation; alert banner; toast; status badge. Include hover, focus-visible, pressed, disabled, loading, success, warning, and error. Use labels and associated helper text. Status badges require text/icon, not color alone. Use Vietnamese copy examples.
```

### 11.6 Checkout order summary

```text
Create a sticky responsive order summary for ticket checkout with ticket tier, quantity, unit amount, total, server-backed hold expiry, and a primary next action. Display fees/taxes only if supplied. Include hold creating, active, expired, sold-out, and ambiguous request states. Do not reset timer on rerender or invent totals. On mobile convert to a compact expandable summary without hiding the primary action.
```

### 11.7 AI assistant

```text
Create a reusable assistant panel with FAQ chips, async pending indicator, messages, retry state, FAQ-only/degraded notice, quota reached state, and out-of-scope response. Visually distinguish assistant content from official system status. Do not show fake streaming tokens or claim live access to queue/inventory. Include keyboard-accessible send and announce new messages to assistive technology.
```

### 11.8 Data table and filter toolbar

```text
Create a responsive internal data table pattern with search, filters, sort, pagination, selected-row actions only where allowed, loading skeleton, empty state, error state, and stale-data notice. At narrow widths, reflow rows into readable cards rather than forcing the whole page to scroll horizontally. Include table caption, accessible column headers, and demo data labeling.
```

### 11.9 Metric card and chart

```text
Create dashboard metric cards and charts with title, value, unit, time range, comparison only if data exists, last-updated timestamp, tooltip, accessible text summary, loading, empty, stale, and unavailable variants. Never use decorative live animation or color as the only signal. Label every sample value as demo.
```

### 11.10 Confirmation dialog / action audit

```text
Create a confirmation dialog for high-impact Moderator/Ops actions. Include precise action title, target event, current state, proposed change, consequence, optional reason, cancel, and action-specific submit label. Provide pending/success/failure feedback and keyboard focus management. Avoid generic “Are you sure?” copy.
```

### 11.11 Empty, loading, error, stale states

```text
Create a consistent state-pattern set for EventFlow: skeleton loading, first-use empty, no results, recoverable network error, permission denied, stale data, partial service outage, and action outcome unknown. Each state should explain what is known, what is not known, and one safe next action. Do not suggest refreshing repeatedly or retrying a payment whose result is unknown.
```

### 11.12 Ticket / QR visual

```text
Create a mobile-first issued ticket detail card with event name/date/venue, ticket tier, order reference, ticket status, and a neutral QR placeholder for prototype only. Never use a real QR, secret signature, or actual personal data. Include issued, checked-in, void/refunded states only when backend status exists; provide a text fallback reference and high-contrast presentation.
```

## 12. Design review / Stitch iteration checklist

### Product and behavior

- [ ] Lobby trước T0 không hiển thị rank/ETA giả hoặc nói vào sớm sẽ được ưu tiên.
- [ ] Queue rank, ETA, polling, admit và connection status phân biệt rõ server truth với thông tin không sẵn có.
- [ ] Admission TTL 15 phút không bị nhầm với hold TTL 10 phút.
- [ ] Payment pending không bị biến thành success chỉ vì redirect.
- [ ] Không khẳng định có vé, waitlist, reminder, refund hoặc payment provider nếu chưa có contract.
- [ ] AI panel không được hiểu nhầm là nguồn chính thức cho queue/inventory/order.
- [ ] Ops/Moderator control có quyền hạn, xác nhận và trạng thái kết quả rõ ràng.

### Visual, responsive, accessibility

- [ ] Cùng token, typography, button, form, badge, nav giữa mọi page.
- [ ] Có desktop/tablet/mobile; mobile không mất CTA hoặc thông tin trạng thái quan trọng.
- [ ] Contrast AA, focus-visible, keyboard flow, reduced motion; trạng thái không phân biệt chỉ bằng màu.
- [ ] Chart có đơn vị/range/freshness; demo data được gắn nhãn.
- [ ] Form lỗi gắn đúng field; placeholder không thay label.
- [ ] Timer/queue update được announce phù hợp, không làm screen reader đọc liên tục.

### Handoff sang React/Next.js

- [ ] Mỗi page prompt có route đề xuất, entry point, primary action, required states.
- [ ] Component có variants và props/data assumptions; không hardcode nghiệp vụ vào visual.
- [ ] API contract được xác minh cho từng state trước implementation.
- [ ] Có asset export names, image aspect ratio, alt-text notes, icon source và font note.
- [ ] Acceptance criteria gắn với backlog/story; ưu tiên buyer vertical slice trước.

## 13. Suggested implementation order

1. Chốt global tokens và public/internal shell.
2. Dựng Buyer vertical slice: Event detail → Lobby/Queue → Admitted → Hold → Payment pending/result.
3. Review accessibility, mobile behavior, reconnect/error states và mapping API.
4. Dựng Organizer create/review-status.
5. Dựng Moderator review queue/detail.
6. Dựng Ops war room/controls sau khi telemetry/control API ổn định.
7. Tạo bản native Flutter riêng chỉ khi mobile app được xác nhận là scope.

## 14. Repository references

- [README.md](../README.md) — mục tiêu, trạng thái scaffold và phần đã/còn thiếu.
- [00-tong-quan.md](./00-tong-quan.md) — SLO, guarantees, roadmap.
- [01-nghiep-vu.md](./01-nghiep-vu.md) — actor, state machine, business rules.
- [02-kien-truc.md](./02-kien-truc.md) — service boundaries, queue, scale, payment/AI architecture.
- [03-cau-truc-src.md](./03-cau-truc-src.md) — frontend/source layout đề xuất.
- [04-jira-backlog.md](./04-jira-backlog.md) — frontend stories EVF-110–118, API/service work.
- [05-Dac_ta_yeu_cau_cua_du_an.md](./05-Dac_ta_yeu_cau_cua_du_an.md) — SRS.
- [06-Thiet_ke_co_ban.md](./06-Thiet_ke_co_ban.md) — Use Case và ERD.
- [07-Quy_trinh_lam_viec_nhom.md](./07-Quy_trinh_lam_viec_nhom.md) — Git/PR/test workflow.
