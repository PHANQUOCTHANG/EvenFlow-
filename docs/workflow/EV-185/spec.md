# EV-185 — Panel chat trợ lý: streaming, FAQ chips, trạng thái suy biến (Spec)

| Thuộc tính | Chi tiết |
|---|---|
| Jira | EV-185 (mã backlog **EVF-115**) — 5 SP — Priority: **P1** — Sprint 5 |
| Tier | **Standard / High-concurrency UI** (Giao diện trợ lý AI cho khách mua vé trong phòng chờ) |
| Base Commit | `f12e04d` |
| Branch | `feature/EV-185-assistant-chat-panel` |
| Nghiệp vụ | **BR-A1** (không request nào chờ đồng bộ Gemini, 202 + job_id, kết quả qua SSE), **BR-A2** (quota 10 câu / 15 phút, vượt → FAQ + gợi ý CSKH), **BR-A4** (giới hạn phạm vi: chỉ giải thích chính sách/sự kiện/hàng chờ, cấm hứa giữ vé, cấm đoán lượt bịa số, cấm lộ tồn kho thật), **BR-A7** (4 nấc suy biến NORMAL / SAVING / FAQ_ONLY / OFF) |
| Phụ thuộc | EVF-94 (API trợ lý phía user: 202 + job_id, SSE), EVF-87 (4 nấc suy biến NORMAL / SAVING / FAQ_ONLY / OFF + công tắc cho Ops) |
| Thiết kế UI | `docs/08-stitch-ui-ux-handoff.md` Mục 10.11 (Buyer AI assistant panel) |

---

## 1. Bối cảnh & Hiện trạng

- Khách hàng khi chờ trong phòng chờ (`/waiting/[eventId]`) có nhu cầu đặt các câu hỏi về thời gian mở bán, cách thức xếp hàng, thời hạn giữ vé 10:00, phương thức thanh toán hoặc chính sách hoàn vé.
- Ở các đợt mở bán chịu tải lớn (lên tới 300.000 rps), việc để người dùng gọi trực tiếp tới LLM đồng bộ sẽ gây quá tải sập hệ thống (429 rate limit, lag hàng đợi).
- Do đó, kiến trúc EvenFlow quy định:
  - Phía backend AI-worker hỗ trợ 4 nấc suy biến: `NORMAL`, `SAVING`, `FAQ_ONLY`, `OFF`.
  - Phía giao diện phòng chờ cần một panel trợ lý (`ChatPanel`, `StreamingMessage`, `FaqChips`) cùng hook điều phối `use-assistant`.
  - Giao diện phải phân định ranh giới rõ ràng: thông tin số thứ tự, ETA hàng đợi là dữ liệu chính thức từ authoritative status panel; trợ lý AI chỉ đóng vai trò giải thích thông tin và chính sách, tuyệt đối không bịa số.

---

## 2. Tiêu chí nghiệm thu (Acceptance Criteria)

- **AC-1 — Cấu trúc Panel trợ lý & Tích hợp phòng chờ (Stitch 10.11)**:
  - Panel trợ lý (`ChatPanel`) có thể mở/đóng linh hoạt (drawer hoặc collapsible floating panel) ngay trong trang phòng chờ `/waiting/[eventId]`.
  - Có welcome state thân thiện, nhãn nhận diện tinh tế "AI hỗ trợ", nút thu nhỏ / đóng tiện lợi.
  - Phân định rõ ràng ngữ cảnh: ghi chú minh bạch rằng trợ lý giải đáp chính sách và thông tin, không thay đổi trạng thái hàng đợi.
  - Hỗ trợ đầy đủ phím bấm tiếp cận (keyboard accessibility) và nhãn ARIA (`role="region"`, `aria-label`, `role="log"`).

- **AC-2 — Chip câu hỏi thường gặp (FaqChips - BR-A4, 0 token)**:
  - Cung cấp danh mục các chip câu hỏi thường gặp:
    - *"Thời gian giữ vé là bao lâu?"* (Giải thích mốc 10:00 theo `expires_at` của server)
    - *"Tôi có bị mất chỗ nếu rớt mạng không?"* (Giải thích BR-Q7: mất kết nối không mất lượt)
    - *"Có những hình thức thanh toán nào?"* (VNPay, MoMo, Thẻ quốc tế)
    - *"Làm sao biết khi nào tới lượt mua vé?"* (Hệ thống tự động chuyển trang khi ADMITTED)
    - *"Vé đã mua có được hoàn trả không?"* (Chính sách ban tổ chức)
  - Khi người dùng nhấp vào chip FAQ: trả lời ngay tức thì từ bộ template chuẩn hóa, không tiêu hao quota token câu hỏi tự do.

- **AC-3 — Streaming tin nhắn & Phản hồi bất đồng bộ (StreamingMessage, useAssistant - BR-A1)**:
  - Hỗ trợ hiển thị tin nhắn người dùng và tin nhắn phản hồi của trợ lý với visual phân biệt rõ ràng.
  - Khi gửi câu hỏi tự do: chuyển sang trạng thái đang xử lý (`status="pending"`), hiển thị con trỏ typing / streaming mượt mà (`status="streaming"`).
  - Tích hợp chuẩn SSE client để nhận các chunk trả về từ stream và ghép hoàn chỉnh thành tin nhắn cuối cùng (`status="complete"`).

- **AC-4 — Xử lý 4 nấc suy biến hệ thống (AC CỐT LÕI - EVF-87, BR-A7, Stitch 10.11)**:
  - `NORMAL`: Chế độ chuẩn, cho phép hỏi tự do lẫn bấm FAQ chips.
  - `SAVING`: Hàng đợi AI lag > 30s hoặc ngân sách > 80%. Tắt tính năng hỏi tự do, chỉ cho phép tương tác qua FAQ chips. Hiển thị thông báo thân thiện: *"Trợ lý đang ưu tiên các câu hỏi thường gặp để phục vụ nhanh hơn."*
  - `FAQ_ONLY`: Hàng đợi lag > 2 phút hoặc ngân sách cạn. 100% template FAQ, không gọi Gemini. Hiển thị thông báo rõ ràng: *"Trợ lý đang ở chế độ câu hỏi thường gặp. Nếu cần thêm hỗ trợ, vui lòng liên hệ CSKH."* **Tuyệt đối không báo lỗi kỹ thuật hay mã lỗi 429/500 cho người dùng.**
  - `OFF`: Gemini gặp sự cố toàn diện. **Ẩn hoàn toàn giao diện chat trợ lý**, thay bằng khối thông tin liên hệ kênh Chăm sóc khách hàng (Hotline, Email CSKH).

- **AC-5 — Kiểm soát hạn mức 10 câu / 15 phút (BR-A2)**:
  - Quản lý quota câu hỏi mở của mỗi khách (tối đa 10 câu / 15 phút).
  - Hiển thị số lượt hỏi còn lại khi người dùng tương tác.
  - Khi đã dùng hết 10 câu: hiển thị thông báo đã đạt hạn mức, tự động khoá ô nhập câu hỏi tự do và gợi ý chuyển sang danh mục FAQ hoặc liên hệ CSKH.

- **AC-6 — Từ chối ngoài phạm vi & An toàn thông tin (BR-A4)**:
  - Với các câu hỏi ngoài phạm vi (đòi giữ vé trước, hỏi tồn kho chính xác, câu hỏi không liên quan), trợ lý đưa ra câu trả lời từ chối lịch sự, hướng dẫn quay lại theo dõi bảng trạng thái chính thức.

---

## 3. Ranh giới (Scope)

### In Scope
- Thư viện client `apps/web/src/lib/assistant-client.ts` quản lý dữ liệu FAQ, quota và kết nối SSE.
- Hook `apps/web/src/hooks/use-assistant.ts` quản lý trạng thái chat, nấc suy biến, quota và streaming.
- Các component UI:
  - `apps/web/src/components/assistant/faq-chips.tsx`
  - `apps/web/src/components/assistant/streaming-message.tsx`
  - `apps/web/src/components/assistant/chat-panel.tsx`
  - `apps/web/src/components/assistant/index.ts`
- Tích hợp vào `apps/web/src/components/queue/waiting-room.tsx`.
- Viết 100% unit tests và integration tests cho các modules mới.

### Out of Scope
- Không can thiệp backend Python `services/ai-worker` hay thay đổi Go backend.
- Không sửa đổi các logic tính toán vị trí rank / eta trong phòng chờ.
