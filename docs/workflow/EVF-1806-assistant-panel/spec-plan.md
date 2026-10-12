# EVF-1806 — Panel trợ lý AI (`AssistantPanel`) — spec & plan

| | |
|---|---|
| Jira | EVF-1806 "Panel chat trợ lý — streaming, FAQ chips, trạng thái suy biến". AC gốc: *ở chế độ FAQ_ONLY hiện thông báo rõ ràng, không báo lỗi kỹ thuật.* |
| Tier | **Standard** |
| Base | `develop` (= `main` tại `c848afa`) |
| Branch | `feature/EVF-1806-assistant-panel` |
| Nguồn | `docs/01-nghiep-vu.md` BR-A1..A7 (**nguồn chuẩn**); `docs/02-kien-truc.md` §3 (4 nấc suy biến); `docs/08-stitch-ui-ux-handoff.md` §10.11, §11.7, §5; ảnh `apps/web/doc/design/stitch/component/image/Quy chuẩn Component AssistantPanel - EventFlow.png` |

**Không phụ thuộc backend, không cần Docker.** Mọi thứ server-owned (chế độ suy biến, hạn mức, tin nhắn,
kết quả job) nhận qua prop. Component không gọi API. Nối vào `ai.chat.q` qua endpoint 202 + `job_id`
(BR-A1, EVF-1615) ở task sau khi có OpenAPI.

---

## 1. Phạm vi

| File | Nội dung |
|---|---|
| `src/components/assistant/assistant-panel.tsx` | Panel trợ lý: header, ranh giới thẩm quyền, log tin nhắn, FAQ chips, ô nhập, 4 chế độ suy biến, hạn mức |
| `src/components/assistant/index.ts` | Barrel export |
| `src/components/showcase/domain-gallery.tsx` | Thêm section "Trợ lý AI" trình bày các trạng thái bằng dữ liệu mẫu |

### Ngoài phạm vi

| Việc | Lý do |
|---|---|
| Gọi API, polling `job_id`, SSE | Chưa có OpenAPI cho EVF-1615 |
| "Streaming" chữ chạy | Jira ghi "streaming" nhưng handoff §11.7 cấm *fake streaming tokens* và kiến trúc là job bất đồng bộ (BR-A1): câu trả lời về nguyên khối. Không có streaming thật thì không giả. **Báo lại Jira** để sửa tiêu đề story. |
| Token màu riêng cho AI (lavender `#4C44D8` trong ảnh thiết kế) | `tokens.css` là nguồn màu duy nhất (EVF-1801) và chưa có token này. Thêm màu là thay đổi design system, cần review riêng. Slice này phân biệt nội dung AI bằng **nhãn chữ + icon + kiểu viền**, đủ cho WCAG (không phân biệt chỉ bằng màu). Ghi nợ cho EVF-1801. |
| Lưu lịch sử chat (IndexedDB) | Ảnh thiết kế ghi "Local IndexedDB FAQ Cache" nhưng không có trong nghiệp vụ; bỏ. |

## 2. Business rule phải tuân thủ

**2.1 BR-A4 + handoff §10.11 — trợ lý KHÔNG có thẩm quyền.** "Cấm tuyệt đối: hứa giữ vé, đoán lượt bằng
con số không lấy từ hệ thống, tiết lộ tồn kho chính xác." Handoff: "clear source/context boundary
between authoritative queue status and AI explanation".

Hệ quả kỹ thuật: **component không có prop nào cho rank / ETA / tồn kho / trạng thái đơn**. Không có
chỗ để nhét số vào. Nếu backend có số thật (qua tool-call, BR-A5) thì nó nằm trong `text` của tin
nhắn trợ lý; UI không tự thêm số nào. Panel luôn có một khối ranh giới thẩm quyền nói rõ: trợ lý không
giữ vé, không đổi đơn, số liệu hàng chờ/tồn kho xem ở bảng trạng thái chính thức. Gốc panel mang
`data-authoritative="false"`.

**2.2 BR-A1 — không có gì đồng bộ.** Gửi câu hỏi là tạo job; trả lời tới sau. Tin nhắn người dùng có
`status: "pending"` cho tới khi có kết quả. Trong lúc **có** một tin pending, **không** cho gửi tiếp
(ô nhập + nút Gửi + chip FAQ đều bị chặn): một người một job tại một thời điểm, vừa chống double-submit
vừa đúng với `ai.chat.q` TTL 60s. Không có spinner "đang gõ…" giả streaming; chỉ có text "đang xử lý".

**2.3 BR-A2 — hạn mức 10 câu / 15 phút, vượt → FAQ + CSKH.** `quotaRemaining === 0` ⇒ **chặn gửi câu
hỏi mở**, nhưng **chip FAQ vẫn bấm được** (vì FAQ không tốn Gemini, và BR-A2 nói vượt hạn mức thì trả
FAQ). Hiện thông báo kèm gợi ý kênh CSKH. Con số hạn mức chỉ hiện khi **cả** `quotaRemaining` và
`quotaLimit` là số nguyên hợp lệ; thiếu hoặc sai thì không hiện số (handoff §5 nguyên tắc 3).

**2.4 BR-A6 + docs/02 §3 — 4 nấc suy biến, UI hành xử khác nhau:**

| `mode` | Câu hỏi mở | FAQ chips | Thông báo |
|---|---|---|---|
| `NORMAL` | cho | cho | không |
| `SAVING` | **chặn** (docs/02: "Tắt câu hỏi mở, chỉ FAQ + cache") | cho | "đang ở chế độ tiết kiệm, chỉ trả lời câu hỏi thường gặp" |
| `FAQ_ONLY` | chặn | cho | "chỉ trả lời câu hỏi thường gặp" |
| `OFF` | — | — | ẩn toàn bộ hội thoại, chỉ còn một thông báo "tạm không khả dụng" + kênh CSKH (docs/02: "Ẩn trợ lý, hiện kênh CSKH") |

`mode` **bắt buộc, không default**: default `NORMAL` nghĩa là khi caller quên truyền, UI mở cửa gọi
Gemini đúng lúc hệ thống đang cạn ngân sách.

**2.5 AC gốc của EVF-1806 — không báo lỗi kỹ thuật.** Ở `SAVING`/`FAQ_ONLY`/`OFF` và ở trạng thái tin
nhắn `failed`, copy **không được** chứa các từ: `lỗi`, `error`, `429`, `rate limit`, `Gemini`,
`timeout`, `server`, `quota` (tiếng Anh), `token`. Khách trong phòng chờ không cần biết Gemini là gì;
họ cần biết còn hỏi được gì và hỏi ở đâu.

**2.6 BR-A7 — nội dung tin nhắn là dữ liệu.** `text` render dưới dạng **text thuần** (React escape),
không `dangerouslySetInnerHTML`, không parse markdown. Một câu trả lời chứa `<script>` hay `[link](…)`
phải hiện nguyên văn.

**2.7 Handoff §11.7 — phân biệt trả lời ngoài phạm vi và trả lời dự phòng.** `kind` của tin trợ lý:
`"ai"` (Gemini), `"faq"` (template, 0 token), `"out_of_scope"` (BR-A4 từ chối lịch sự),
`"fallback"` (hệ thống suy biến trả FAQ thay vì trả lời thật). Mỗi `kind` có nhãn chữ riêng để người
dùng biết mình đang đọc gì; `kind` thiếu ⇒ coi như `"ai"`.

## 3. Hợp đồng (contract) — chốt trước khi viết test và code

```ts
// src/components/assistant/assistant-panel.tsx
export type AssistantMode = "NORMAL" | "SAVING" | "FAQ_ONLY" | "OFF"; // giữ đúng tên docs/02 §3
export type AssistantAnswerKind = "ai" | "faq" | "out_of_scope" | "fallback";
export type AssistantMessageStatus = "pending" | "done" | "failed";

export interface AssistantMessage {
  id: string;
  role: "user" | "assistant";
  text: string;
  /** Chỉ có nghĩa với role "assistant". Thiếu ⇒ "ai". */
  kind?: AssistantAnswerKind;
  /** Chỉ có nghĩa với role "user". Thiếu ⇒ "done". pending = job đang chạy (BR-A1), failed = job lỗi/quá TTL. */
  status?: AssistantMessageStatus;
  /** Epoch ms hoặc ISO-8601 có múi giờ. Thiếu ⇒ không hiện giờ. */
  sentAt?: number | string;
}

export interface AssistantFaq { id: string; label: string }

export interface AssistantPanelProps {
  mode: AssistantMode;                 // BẮT BUỘC
  messages: AssistantMessage[];
  faqs?: AssistantFaq[];
  quotaRemaining?: number;             // BR-A2
  quotaLimit?: number;
  onSend?: (text: string) => void;     // text đã trim
  onSelectFaq?: (faqId: string) => void;
  onRetry?: (messageId: string) => void;
  onClose?: () => void;                // có ⇒ hiện nút đóng + Esc đóng
  supportHref?: string;                // kênh CSKH; thiếu ⇒ câu chữ chung, không có link
  className?: string;
}

export function AssistantPanel(props: AssistantPanelProps): JSX.Element;
```

Thuộc tính DOM để test và style bám vào, **không đổi tên**:

| Vị trí | Thuộc tính |
|---|---|
| gốc | `data-slot="assistant-panel"`, `data-mode={mode}`, `data-authoritative="false"`, `role="region"`, `aria-label="Trợ lý sự kiện"` |
| nhãn AI | text nhìn thấy chứa `AI hỗ trợ` |
| khối ranh giới | `data-slot="authority-notice"`, `role="note"` |
| log | `role="log"`, `aria-live="polite"`, `aria-label="Hội thoại với trợ lý"` |
| mỗi tin | `data-slot="message"`, `data-role={role}`, `data-kind` (tin trợ lý), `data-status` (tin người dùng) |
| nhóm FAQ | `role="group"`, `aria-label="Câu hỏi thường gặp"`; mỗi chip là `<button type="button">` |
| ô nhập | `<textarea>` có `<label>` nhìn thấy "Câu hỏi của bạn" |
| nút gửi | `<button type="submit">` tên "Gửi" |
| nút đóng | `<button>` tên "Đóng trợ lý" |
| nút gửi lại | `<button>` tên "Gửi lại" trong tin `failed` |
| hạn mức | `data-slot="quota"` |

## 4. AC

### AC-1 — Ranh giới thẩm quyền, không có chỗ cho số liệu
- **Then** ở mọi `mode` trừ `OFF`, có khối `authority-notice` nói rõ trợ lý **không giữ vé**, **không thay đổi đơn hàng**, và số liệu hàng chờ/tồn kho xem ở **bảng trạng thái chính thức** (regex: `/không.*giữ vé/i`, `/không.*đơn/i`, `/bảng trạng thái|trạng thái chính thức/i`)
- **Then** gốc có `data-authoritative="false"` và nhãn nhìn thấy `AI hỗ trợ`
- **Then** `AssistantPanelProps` **không** có key `rank`, `eta`, `etaSeconds`, `available`, `stock`, `inventory`, `orderStatus` (kiểm bằng type-level test `// @ts-expect-error` khi truyền `rank`)
- **Given** `messages = []`, **Then** panel vẫn render được (trạng thái chào mừng), **không** throw, **không** có tin nhắn nào trong log

### AC-2 — Render tin nhắn, text là dữ liệu (BR-A7)
- **Given** tin `{ role: "user", text: "Còn vé không?" }` và `{ role: "assistant", text: "Xin xem bảng trạng thái." }`, **Then** cả hai xuất hiện trong `role="log"`, đúng thứ tự mảng, với `data-role` tương ứng
- **Given** `text` = `<b>đậm</b> [link](http://x) <script>alert(1)</script>`, **Then** chuỗi đó xuất hiện **nguyên văn** dưới dạng text, DOM **không** có phần tử `b`, `a`, `script` mới
- **Given** tin trợ lý với `kind` lần lượt `ai | faq | out_of_scope | fallback`, **Then** mỗi tin có `data-kind` đúng và **nhãn chữ khác nhau** cho 4 kind (4 nhãn phân biệt được, không trùng)
- **Given** tin trợ lý **không** có `kind`, **Then** `data-kind="ai"`
- **Given** `sentAt` hợp lệ, **Then** hiện giờ `HH:mm`; `sentAt` thiếu hoặc không parse được ⇒ **không** hiện giờ và không throw
- **Then** log có `aria-live="polite"` (tin mới được đọc, không ngắt lời); **không** có `aria-live="assertive"` ở bất kỳ tin nào

### AC-3 — Gửi câu hỏi mở (BR-A1)
- **Given** `mode="NORMAL"`, gõ `"  Mở bán lúc mấy giờ?  "` rồi bấm **Gửi**, **Then** `onSend` được gọi **đúng 1 lần** với `"Mở bán lúc mấy giờ?"` (đã trim) và ô nhập được xoá trống
- **Given** nhấn **Enter** (không Shift) trong textarea, **Then** gửi như bấm nút; **Given** **Shift+Enter**, **Then** **không** gửi (xuống dòng)
- **Given** ô nhập rỗng hoặc chỉ khoảng trắng, **Then** bấm Gửi/Enter **không** gọi `onSend`
- **Given** không truyền `onSend`, **Then** ô nhập và nút Gửi bị `disabled`, không throw
- **Then** textarea có `<label>` nhìn thấy, **không** dùng placeholder làm label (handoff §6)
- **AC-3b (bổ sung vòng review 1)** **Given** IME đang ghép chữ (`isComposing` hoặc `keyCode 229`, Telex/VNI), **Then** Enter **không** gửi và giữ nguyên nội dung; chuỗi chỉ gồm ký tự không bề rộng (`U+200B`…) coi như rỗng

### AC-4 — Một job một lúc (BR-A1)
- **Given** có **bất kỳ** tin `status: "pending"`, **Then** textarea `disabled`, nút Gửi `disabled`, **mọi** chip FAQ `disabled`; bấm Gửi/Enter/chip **không** gọi `onSend`/`onSelectFaq`
- **Then** tin pending hiện text "đang xử lý" (regex `/đang xử lý/i`) và `data-status="pending"`; **không** có spinner giả streaming (không có phần tử `role="progressbar"`)
- **Given** không còn tin pending, **Then** gửi lại được

### AC-5 — Tin thất bại và gửi lại
- **Given** tin người dùng `status: "failed"` và có `onRetry`, **Then** tin đó có nút **Gửi lại**; bấm ⇒ `onRetry(message.id)` đúng 1 lần
- **Given** không có `onRetry`, **Then** không có nút Gửi lại, vẫn hiện trạng thái thất bại bằng chữ
- **Then** copy của trạng thái failed **không** chứa từ kỹ thuật (xem §2.5)
- **AC-5b (bổ sung vòng review 1)** Gửi lại = một câu hỏi mở mới (tạo job). **Given** đang có tin pending, hoặc `mode` ≠ `NORMAL`, hoặc `quotaRemaining ≤ 0`, **Then** nút Gửi lại `disabled` và bấm **không** gọi `onRetry`. Không đòi `onSend`.

### AC-6 — FAQ chips
- **Given** `faqs` có 3 mục, **Then** nhóm `role="group"` tên "Câu hỏi thường gặp" có đúng 3 `button` với `label` tương ứng; bấm chip thứ 2 ⇒ `onSelectFaq(faqs[1].id)` đúng 1 lần
- **Given** `faqs` rỗng hoặc thiếu, **Then** **không** render nhóm (không có group rỗng)
- **Given** `mode="FAQ_ONLY"` hoặc `"SAVING"`, **Then** chip **vẫn bấm được** (BR-A6: FAQ không tốn Gemini)
- **Given** `quotaRemaining = 0`, **Then** chip **vẫn bấm được** (BR-A2)

### AC-7 — Hạn mức (BR-A2)
- **Given** `quotaRemaining=3, quotaLimit=10`, **Then** `data-slot="quota"` hiện "3/10" (regex `/3\s*\/\s*10/`) kèm chữ "lượt"
- **Given** chỉ có một trong hai, hoặc giá trị không phải số nguyên ≥ 0 (`-1`, `2.5`, `NaN`), **Then** **không** render `quota` (không bịa số)
- **Given** `quotaRemaining = 0` và `mode="NORMAL"`, **Then** textarea + nút Gửi `disabled`; có thông báo (regex `/hết lượt|đã dùng hết/i`) và nhắc kênh CSKH (`/hỗ trợ|CSKH|chăm sóc/i`); nếu có `supportHref` ⇒ có `<a href={supportHref}>`, không có ⇒ không có `<a>` nào tới CSKH
- **Then** thông báo hết lượt **không** chứa từ kỹ thuật (§2.5)
- **AC-7b (bổ sung vòng review 1)** Chỉ hiện số khi `quotaLimit > 0` và `quotaRemaining ≤ quotaLimit` (không hiện "11/10", "0/0"). `quotaRemaining` âm ⇒ coi như hết lượt (fail closed).

### AC-8 — 4 nấc suy biến (BR-A6, docs/02 §3, AC gốc Jira)
- **Given** `mode="NORMAL"`, **Then** **không** có thông báo suy biến; gửi được
- **Given** `mode="SAVING"`, **Then** textarea + Gửi `disabled`; có thông báo nhắc **chỉ trả lời câu hỏi thường gặp** (`/câu hỏi thường gặp/i`); chip bấm được
- **Given** `mode="FAQ_ONLY"`, **Then** như SAVING; thông báo của `SAVING` và `FAQ_ONLY` được phép khác nhau nhưng cả hai **không** chứa từ kỹ thuật (§2.5) — đây là AC gốc của story
- **Given** `mode="OFF"`, **Then** **không** có `role="log"`, **không** có textarea, **không** có nhóm FAQ; có đúng một thông báo "tạm không khả dụng" (`/tạm.*không.*khả dụng|tạm ngưng/i`) kèm nhắc kênh CSKH; `data-mode="OFF"`; **không** chứa từ kỹ thuật
- **Then** thông báo suy biến dùng `role="status"` (không phải `alert`): đây là thông tin, không phải sự cố của người dùng
- **Then** gốc luôn có `data-mode` đúng bằng `mode`

### AC-9 — Đóng panel và bàn phím
- **Given** có `onClose`, **Then** có nút "Đóng trợ lý"; bấm ⇒ `onClose` 1 lần; nhấn **Escape** khi focus đang ở trong panel (ví dụ trong textarea) ⇒ `onClose` 1 lần
- **Given** không có `onClose`, **Then** không có nút đóng; Escape **không** throw
- **Then** mọi control tương tác là phần tử native (`button`, `textarea`, `a`), có tên truy cập được qua `getByRole`
- **AC-9b (bổ sung vòng review 1)** Escape trong lúc IME ghép chữ hoặc đã bị `preventDefault` **không** đóng panel

### AC-11 — Dữ liệu biên (bổ sung vòng review 1, challenge C-1/C-7/C-10)
- **Given** `sentAt` là số ngoài miền `Date` (`1e20`), **Then** không throw, không hiện giờ (thuộc AC-2)
- **Given** `supportHref` không phải `http(s):`, `mailto:`, `tel:`, đường dẫn `/…` hoặc `#…` (ví dụ `javascript:`, `data:`, `//host`), **Then** không render `<a>`, chỉ hiện câu chữ chung
- **Given** `kind` lạ lúc runtime, **Then** coi như `"ai"` (có nhãn)
- Copy khớp màn hình: gợi ý "Enter để gửi" chỉ hiện khi ô nhập mở; thông báo SAVING/FAQ_ONLY không hứa "bên dưới"

### AC-10 — Showcase
- **Given** render `DomainGallery`, **Then** có section tiêu đề chứa "Trợ lý" và có **ít nhất** một `AssistantPanel` cho mỗi `mode` trong 4 nấc (`[data-slot="assistant-panel"][data-mode=…]`)
- **Then** test hiện có của showcase (nếu có) vẫn pass

## 5. Plan

1. **Test trước** (agent riêng, chỉ đọc spec này): `src/components/assistant/assistant-panel.test.tsx`
   phủ AC-1..AC-9; AC-10 vào `src/components/showcase/domain-gallery.test.tsx` (tạo mới nếu chưa có).
   Baseline: 1086/1086 pass trên `develop` (`docs/kiem-thu/README.md` §3).
2. **Implement** `assistant-panel.tsx` + `index.ts`, rồi thêm section vào `domain-gallery.tsx`.
3. **Gates**: `npm run lint`, `npm run typecheck`, `npm test`, `npm run build`. Diff chỉ gồm 5 file
   trên + `docs/workflow/EVF-1806-assistant-panel/*` + `docs/kiem-thu/README.md`.
4. **Code review + Challenge** bằng 2 agent context sạch (chỉ spec + diff).
5. **Verification** bằng agent chỉ đọc: chạy lại gates, điền `traceability.md`, lưu raw output.
6. Cập nhật `docs/kiem-thu/README.md` §3 và §5 với số liệu thật.

**Rủi ro:** (a) copy tiếng Việt dễ lọt từ "lỗi" — test §2.5 quét bằng regex trên toàn `textContent`
của panel ở từng trạng thái; (b) Escape handler gắn ở gốc panel bằng `onKeyDown`, không gắn `document`
để không nuốt Escape của dialog khác; (c) `domain-gallery.tsx` là file chung của showcase — chỉ **thêm**
section, không sửa section cũ. **Rollback:** revert 1 merge commit; không có migration, không đổi
contract nào đã có.
