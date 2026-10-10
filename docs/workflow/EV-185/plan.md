# EV-185 — Kế hoạch thực thi (Plan)

## 1. Thứ tự thực hiện

| Bước | Nội dung | Lý do / Ràng buộc |
|---|---|---|
| **1** | Hiện thực `apps/web/src/lib/assistant-client.ts` + `assistant-client.test.ts` | Tầng API client: types, FAQ repository, mock/live endpoint handler, stream simulator/parser, quota tracking (BR-A1, BR-A2) |
| **2** | Hiện thực `apps/web/src/components/assistant/faq-chips.tsx` + `faq-chips.test.tsx` | Component hiển thị các chip câu hỏi thường gặp, accessibility keyboard & ARIA roles |
| **3** | Hiện thực `apps/web/src/components/assistant/streaming-message.tsx` + `streaming-message.test.tsx` | Component hiển thị tin nhắn, visual ranh giới thông tin, typing indicator cursor khi streaming |
| **4** | Hiện thực `apps/web/src/hooks/use-assistant.ts` + `use-assistant.test.tsx` | Hook quản lý hội thoại, nấc suy biến (`NORMAL`/`SAVING`/`FAQ_ONLY`/`OFF`), quota 10 câu/15p và streaming |
| **5** | Hiện thực `apps/web/src/components/assistant/chat-panel.tsx` + `chat-panel.test.tsx` + `index.ts` | Giao diện panel hoàn chỉnh: banner nấc suy biến, thông tin CSKH khi OFF, danh sách tin nhắn, FAQ chips, form nhập |
| **6** | Tích hợp vào `apps/web/src/components/queue/waiting-room.tsx` + update `waiting-room.test.tsx` | Đưa nút mở trợ lý và drawer/panel vào phòng chờ theo user story, đảm bảo 100% test cũ không bị gãy |
| **7** | Chạy Gate tự động (typecheck, lint, test, build) & thu thập `evidence.md`, `traceability.md` | Xác minh 100% test pass, đạt ngưỡng coverage |
| **8** | Adversarial Review (`code-review.md`) & Commit theo Conventional Commits | Đảm bảo 4 quy tắc bất biến và chuẩn chỉ mã nguồn |

---

## 2. Danh sách File thay đổi

### Thêm mới
- `apps/web/src/lib/assistant-client.ts`
- `apps/web/src/lib/assistant-client.test.ts`
- `apps/web/src/components/assistant/faq-chips.tsx`
- `apps/web/src/components/assistant/faq-chips.test.tsx`
- `apps/web/src/components/assistant/streaming-message.tsx`
- `apps/web/src/components/assistant/streaming-message.test.tsx`
- `apps/web/src/hooks/use-assistant.ts`
- `apps/web/src/hooks/use-assistant.test.tsx`
- `apps/web/src/components/assistant/chat-panel.tsx`
- `apps/web/src/components/assistant/chat-panel.test.tsx`
- `apps/web/src/components/assistant/index.ts`
- `docs/workflow/EV-185/baseline.md`
- `docs/workflow/EV-185/spec.md`
- `docs/workflow/EV-185/plan.md`
- `docs/workflow/EV-185/traceability.md`
- `docs/workflow/EV-185/evidence.md`
- `docs/workflow/EV-185/code-review.md`

### Sửa đổi (nếu cần thiết)
- `apps/web/src/components/queue/waiting-room.tsx` (tích hợp nút mở và drawer ChatPanel)
- `apps/web/src/components/queue/waiting-room.test.tsx` (kiểm tra nút mở trợ lý)

### TUYỆT ĐỐI CẤM SỬA
- `apps/web/src/components/queue/queue-position.test.tsx` (baseline test)
- `apps/web/src/components/queue/admit-banner.test.tsx` (baseline test)
- `apps/web/src/hooks/use-queue-status.test.tsx` (baseline test)
- `apps/web/src/lib/sse.test.ts` (baseline test)
- `vitest.config.ts`, `package.json` (không hạ ngưỡng coverage)
- Bất kỳ file backend nào trong `services/ai-worker` hay Go services

---

## 3. Hợp đồng API / Type Signatures

### 3.1 `apps/web/src/lib/assistant-client.ts`
```ts
export type DegradationMode = "NORMAL" | "SAVING" | "FAQ_ONLY" | "OFF";

export interface FaqItem {
  id: string;
  question: string;
  answer: string;
  category?: "ticket" | "queue" | "payment" | "policy";
}

export type MessageRole = "user" | "assistant" | "system";
export type MessageStatus = "sending" | "pending" | "streaming" | "complete" | "error";

export interface AssistantMessage {
  id: string;
  role: MessageRole;
  content: string;
  createdAt: string;
  status: MessageStatus;
  isFaq?: boolean;
  jobId?: string;
  faqId?: string;
}

export interface AssistantQuota {
  used: number;
  max: number; // 10 câu
  resetWindowMinutes: number; // 15 phút
}

export interface SendQuestionResponse {
  jobId?: string;
  immediateAnswer?: string;
  mode: DegradationMode;
  status: "accepted" | "answered_immediately" | "rate_limited" | "rejected";
  message?: string;
}
```

### 3.2 `apps/web/src/hooks/use-assistant.ts`
```ts
export interface UseAssistantOptions {
  eventId: string;
  initialMode?: DegradationMode;
  apiBaseUrl?: string;
  enableStreamMock?: boolean;
}

export interface UseAssistantReturn {
  messages: AssistantMessage[];
  mode: DegradationMode;
  quota: AssistantQuota;
  isPending: boolean;
  isStreaming: boolean;
  error: string | null;
  sendMessage: (text: string) => Promise<void>;
  askFaq: (faq: FaqItem) => Promise<void>;
  setMode: (mode: DegradationMode) => void;
  resetQuota: () => void;
  clearHistory: () => void;
}
```
