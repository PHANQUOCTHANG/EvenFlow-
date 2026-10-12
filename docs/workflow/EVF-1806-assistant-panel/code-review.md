# EVF-1806 — Code review (bước 7, reviewer độc lập)

Phạm vi: `apps/web/src/components/assistant/{assistant-panel.tsx,index.ts,assistant-panel.test.tsx}`,
`apps/web/src/components/showcase/domain-gallery.test.tsx`, diff `domain-gallery.tsx`.
Đối chiếu: `spec-plan.md` §2.1–2.7, §3, AC-1..AC-10.

Đã chạy: `npx vitest run src/components/assistant src/components/showcase/domain-gallery.test.tsx`
→ 2 file, 160/160 pass (152 + 8). Không chạy lint/typecheck/build (việc của bước verify).

## Tổng quan đạt

- §2.1 / AC-1: props không có rank/eta/tồn kho (chốt bằng type-level test + `@ts-expect-error`); `authority-notice` `role="note"` ở mọi mode trừ OFF; gốc `data-authoritative="false"`, nhãn "AI hỗ trợ".
- §2.4: `mode` bắt buộc, không default (`assistant-panel.tsx:50`, destructure không có giá trị mặc định). OFF ẩn log/textarea/FAQ (`:207-213`). SAVING/FAQ_ONLY chặn câu hỏi mở, chip vẫn bấm được (`:145-148`).
- §2.5: đã quét toàn bộ chuỗi trong component, gồm aria-label/title (`Trợ lý sự kiện`, `Hội thoại với trợ lý`, `Câu hỏi thường gặp`, `Đóng trợ lý`, `KIND_LABEL`, `MODE_NOTICE`, copy pending/failed/hết lượt, `SupportHint`, icon `Alert` "i" có aria-hidden). Không có `lỗi|error|429|rate limit|Gemini|timeout|server|quota|token`. "AI" không thuộc danh sách cấm.
- §2.6 / BR-A7: `{message.text}` render thuần trong `<p>` (`:357`), không `dangerouslySetInnerHTML`, không parse markdown.
- §2.3 / AC-7: `isValidQuota` = `Number.isInteger && >= 0` (`:92-94`), loại được NaN/Infinity/2.5/-1; quota chỉ hiện khi cả hai hợp lệ; `quotaRemaining === 0` một mình đã đủ chặn.
- §2.2 / AC-4: pending chặn textarea + Gửi + mọi chip; không có `role="progressbar"`.
- AC-8: thông báo dùng `Alert variant="info"` → `role="status"` (`ui/alert.tsx` `roleFor`), không có `role="alert"`.
- AC-9: Escape gắn `onKeyDown` ở gốc `<section>` (`:186`), không gắn `document`.
- AC-10: `domain-gallery.tsx` diff chỉ thêm import + một `<Section>` mới + hằng demo; không sửa section cũ. Có panel cho cả 4 mode.
- Key list dùng `message.id` / `faq.id` / `demo.label` — ổn định. Textarea là controlled (`value={draft}`), trim trước khi gửi, xoá sau khi gửi.
- Giờ `HH:mm` chỉ render sau mount (tránh hydration mismatch), dùng `toEpochMs` dùng chung (từ chối ISO không có múi giờ) — nhất quán với `lib/server-time`.

## Findings

### F-1 — MAJOR — Nút "Gửi lại" bỏ qua mọi điều kiện chặn câu hỏi mở (BR-A1, BR-A2, BR-A6)

**Evidence:** `assistant-panel.tsx:367-371` — nút "Gửi lại" chỉ phụ thuộc vào `onRetry`, không phụ thuộc
`hasPending`, `mode`, hay `quotaExhausted`. Retry là gửi lại một câu hỏi mở (tạo job `ai.chat.q` mới,
tốn Gemini), nhưng:

- Repro 1 (BR-A1, §2.2 "một người một job tại một thời điểm"): `messages=[FAILED_MSG, PENDING_MSG]`, `onRetry` có → textarea/Gửi/chip đều disabled nhưng "Gửi lại" vẫn bấm được → tạo job thứ hai trong khi job đầu còn chạy. Bấm liên tục cũng là double-submit.
- Repro 2 (BR-A6, §2.4): `mode="SAVING"` hoặc `"FAQ_ONLY"` + một tin `failed` → "Gửi lại" bấm được, tức là mở cửa câu hỏi tự do đúng lúc hệ thống đang tiết kiệm ngân sách.
- Repro 3 (BR-A2): `quotaRemaining=0` + tin `failed` → "Gửi lại" bấm được dù đã hết lượt.

Spec §2.2 nói rõ "**không** cho gửi tiếp (ô nhập + nút Gửi + chip FAQ đều bị chặn)" và §2.4 "Câu hỏi mở: chặn"; AC-5 không nêu tương tác này nên test không bắt được.

**Fix:** tách `const openQuestionGate = mode === "NORMAL" && !hasPending && !quotaExhausted;`, dùng cho cả `openQuestionsAllowed` (`&& Boolean(onSend)`) và truyền `retryAllowed={openQuestionGate}` vào `MessageItem` → `<Button disabled={!retryAllowed}>` (giữ nút hiển thị để trạng thái thất bại vẫn rõ). Thêm test: pending + failed → "Gửi lại" disabled, bấm không gọi `onRetry`; SAVING/FAQ_ONLY/quota 0 + failed → disabled. (Nếu product muốn retry vẫn được phép ở SAVING thì phải ghi rõ vào spec; mặc định đọc spec là chặn.)

### F-2 — MAJOR — Enter trong lúc gõ IME gửi câu hỏi chưa hoàn chỉnh

**Evidence:** `assistant-panel.tsx:164-169` — `handleTextareaKeyDown` không kiểm `event.nativeEvent.isComposing`
(hay `keyCode === 229`). Khách hàng là người Việt: bộ gõ Telex/VNI trên macOS, iOS, Android, và một số
cấu hình bộ gõ trên Windows dùng composition (marked text). Nhấn Enter để chốt chữ đang gõ sẽ (a) `preventDefault`
mất thao tác chốt chữ, (b) gọi `submit()` với `draft` chưa có ký tự cuối, gửi câu hỏi cụt và tiêu một lượt
trong 10 lượt / 15 phút (BR-A2). Repo chưa có xử lý nào cho việc này (grep `isComposing` không có kết quả).

**Fix:**
```ts
if (event.key === "Enter" && !event.shiftKey && !event.nativeEvent.isComposing && event.keyCode !== 229) {
```
Thêm test: `fireEvent.keyDown(input, { key: "Enter", isComposing: true })` (hoặc `keyCode: 229`) → không gọi `onSend`, nội dung giữ nguyên.

### F-3 — MINOR — Escape đóng panel cả khi đang huỷ composition IME hoặc khi phần tử con đã xử lý Escape

**Evidence:** `assistant-panel.tsx:172-177` — không kiểm `isComposing` / `event.defaultPrevented`. Nhấn
Esc để huỷ chữ đang gõ dở sẽ đóng luôn panel; `stopPropagation()` cũng chặn Escape lên các listener ở
`document` (ví dụ dialog chứa panel) — đây có thể là ý đồ, nhưng nên ghi chú.
**Fix:** `if (event.key !== "Escape" || !onClose || event.defaultPrevented || event.nativeEvent.isComposing) return;`

### F-4 — MINOR — Copy hướng dẫn không khớp trạng thái thực

**Evidence:**
- `:240` lời chào "…hoặc đặt câu hỏi về quy định mua vé" vẫn hiện ở SAVING/FAQ_ONLY và khi hết lượt, lúc ô nhập đã bị khoá.
- `:301` "Enter để gửi, Shift+Enter để xuống dòng." hiện cả khi textarea disabled.
- `MODE_NOTICE.SAVING/FAQ_ONLY.body` (`:80`, `:84`) và thông báo hết lượt (`:274`) nói "các câu hỏi thường gặp bên dưới / vẫn dùng được" kể cả khi `faqs` rỗng hoặc không có `onSelectFaq` (chip bị disabled vì `faqAllowed` cần `onSelectFaq`) → hứa một thứ không có trên màn hình (handoff §5: không bịa).

**Fix:** điều kiện hoá câu chữ theo `openQuestionsAllowed` và `faqs?.length && onSelectFaq`; khi không có FAQ thì chỉ nhắc kênh CSKH.

### F-5 — MINOR — Quota hiển thị được cặp số vô lý

**Evidence:** `:140` chấp nhận `quotaRemaining > quotaLimit` (vd. `12/10`) và `quotaLimit = 0` (`3/0`). Spec chỉ đòi số nguyên ≥ 0 nên không vi phạm AC-7, nhưng tinh thần "không bịa số" (§2.3) gợi ý thêm `quotaLimit > 0 && quotaRemaining <= quotaLimit`.
**Fix:** bổ sung điều kiện trên vào `showQuota` (không ảnh hưởng `quotaExhausted`), thêm 2 case test.

### F-6 — MINOR — Một số test yếu hơn tên gọi

- `assistant-panel.test.tsx:1419-1428` tên "khong lam mat noi dung dang go" nhưng không assert `textarea.value === "Đang gõ"`.
- `:1487-1490` selector `[onclick]` không bao giờ khớp trong React (handler không thành attribute) → phần này gần như vacuous; phần `role=` vẫn có giá trị.
- Không có test cho tương tác retry với pending/SAVING/quota (xem F-1) và IME (F-2).
- Test `sentAt` rác in 4 dòng `console.warn` từ `server-time` ra stderr; nên `vi.spyOn(console, "warn").mockImplementation(() => {})` và assert được gọi để log CI sạch.

### F-7 — MINOR (style) — Hằng demo đặt dưới component

**Evidence:** `domain-gallery.tsx` diff: `noop`, `DEMO_FAQS`, `DEMO_ASKED`, `AssistantDemo`, `ASSISTANT_DEMOS` khai báo **sau** `DomainGallery`, trong khi các hằng khác của file (`QUEUE_STATES`, `EVENT_STATES`, `HOLD_STATES`…) ở trên đầu. Chạy đúng (chỉ đọc lúc render) nhưng lệch quy ước file. `supportHref="#"` trong showcase bấm sẽ nhảy lên đầu trang — có thể bỏ để demo nhánh "không có link".
**Fix:** dời lên cạnh các hằng demo khác.

### Nit (không tính severity)
- `<section role="region">` thừa role (section có aria-label đã là region) — giữ được vì spec §3 chốt `role="region"`.
- `epoch as number` (`:351`) — có thể tính `isoTime` cùng chỗ với `time` để bỏ cast.

## Verdict

**FAIL** — 2 major (F-1 retry bỏ qua chặn job/suy biến/hết lượt; F-2 Enter khi đang gõ IME tiếng Việt).
Các AC-1..AC-10 còn lại đều được cài đặt đúng và có test thực chất; sửa F-1, F-2 kèm test là đủ để PASS.
