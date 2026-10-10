/**
 * Assistant Client & FAQ Repository cho EvenFlow Buyer Assistant (EV-185, EVF-115).
 * Tuân thủ các quy tắc cốt lõi:
 * - BR-A1: Không request nào chờ đồng bộ Gemini; API trả 202 Accepted + job_id; kết quả trả qua SSE.
 * - BR-A2: Quota tối đa 10 câu / 15 phút mỗi khách. Vượt -> trả FAQ + gợi ý CSKH.
 * - BR-A4: Giới hạn phạm vi nghiêm ngặt: không hứa giữ vé, không đoán lượt bịa số, không tiết lộ tồn kho thật.
 * - BR-A7: 4 nấc suy biến NORMAL / SAVING / FAQ_ONLY / OFF.
 */

export type DegradationMode = "NORMAL" | "SAVING" | "FAQ_ONLY" | "OFF";

export interface FaqItem {
  id: string;
  question: string;
  answer: string;
  category: "queue" | "hold" | "payment" | "policy";
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
  faqId?: string;
  jobId?: string;
  notice?: string;
}

export interface AssistantQuota {
  used: number;
  max: number;
  resetAt: number; // timestamp ms
}

export const MAX_ASSISTANT_QUOTA = 10;
export const QUOTA_WINDOW_MS = 15 * 60 * 1000; // 15 phút

export const CSKH_INFO = {
  hotline: "1900 6868",
  email: "support@evenflow.vn",
  workingHours: "08:00 - 22:00 hàng ngày",
};

export const DEGRADATION_NOTICES: Record<DegradationMode, string | null> = {
  NORMAL: null,
  SAVING: "Trợ lý đang ưu tiên các câu hỏi thường gặp để phục vụ nhanh hơn.",
  FAQ_ONLY:
    "Trợ lý đang ở chế độ câu hỏi thường gặp. Nếu cần thêm hỗ trợ, vui lòng liên hệ CSKH.",
  OFF: "Trợ lý tạm thời không khả dụng. Vui lòng liên hệ CSKH.",
};

export const DEFAULT_FAQ_ITEMS: FaqItem[] = [
  {
    id: "faq-hold-timer",
    question: "Thời gian giữ vé là bao lâu?",
    answer:
      "Sau khi chọn vé, bạn có 10:00 phút để hoàn tất thanh toán tính theo mốc expires_at của server. Hết thời gian này, vé sẽ được hoàn trả lại kho theo quy chế công bằng.",
    category: "hold",
  },
  {
    id: "faq-connection-loss",
    question: "Tôi có bị mất chỗ nếu rớt mạng không?",
    answer:
      "Không. Vị trí của bạn được lưu an toàn trên server. Bạn có tối đa 60 giây để kết nối lại mà không bị mất chỗ trong hàng đợi.",
    category: "queue",
  },
  {
    id: "faq-admitted",
    question: "Làm sao biết khi nào tới lượt mua vé?",
    answer:
      "Khi đến lượt (trạng thái ADMITTED), hệ thống sẽ tự động chuyển bạn vào màn hình chọn vé và giữ chỗ. Vui lòng giữ nguyên tab trình duyệt này.",
    category: "queue",
  },
  {
    id: "faq-payment-methods",
    question: "Có những phương thức thanh toán nào?",
    answer:
      "EvenFlow hỗ trợ thanh toán qua VNPay, MoMo và Thẻ thanh toán quốc tế (Sandbox Mock). Mọi giao dịch đều được đính kèm Idempotency-Key để chống trùng lặp.",
    category: "payment",
  },
  {
    id: "faq-refund-policy",
    question: "Vé đã mua có được hoàn tiền không?",
    answer:
      "Chính sách hoàn vé tuân theo quy định riêng của từng Ban tổ chức sự kiện. Bạn vui lòng kiểm tra kỹ chi tiết trước khi xác nhận đơn hàng.",
    category: "policy",
  },
  {
    id: "faq-batch-admit",
    question: "Tại sao vị trí hàng đợi nhảy theo từng đợt?",
    answer:
      "Hệ thống điều phối tiếp nhận khách theo từng đợt (batch admit) để bảo vệ kho vé không bị nghẽn và đảm bảo 0 oversell, do đó số thứ tự sẽ giảm theo từng nhịp của máy chủ.",
    category: "queue",
  },
];

/**
 * Kiểm tra câu hỏi có thuộc phạm vi bị từ chối theo BR-A4 hay không.
 */
export function checkOutOfScope(query: string): string | null {
  const lower = query.toLowerCase().trim();

  // 1. Đòi giữ vé trước, can thiệp thứ tự hàng đợi
  if (
    lower.includes("giữ vé trước") ||
    lower.includes("chen hàng") ||
    lower.includes("ưu tiên") ||
    lower.includes("nhảy cóc") ||
    lower.includes("chỉnh rank")
  ) {
    return "Trợ lý không thể can thiệp thứ tự xếp hàng hoặc giữ trước vé. Vị trí của bạn được điều phối hoàn toàn tự động và minh bạch bởi máy chủ.";
  }

  // 2. Hỏi con số tồn kho chính xác
  if (
    lower.includes("còn bao nhiêu vé") ||
    lower.includes("tồn kho chính xác") ||
    lower.includes("số vé còn lại") ||
    lower.includes("chính xác bao nhiêu vé")
  ) {
    return "Để đảm bảo công bằng và chống đầu cơ, trợ lý không tiết lộ số lượng tồn kho chính xác. Tình trạng vé chỉ được thể hiện định tính (Còn vé / Sắp hết / Hết vé) trên trang sự kiện.";
  }

  // 3. Đoán trước kết quả / chắc chắn mua được không
  if (
    lower.includes("chắc chắn mua được không") ||
    lower.includes("tôi có mua được không") ||
    lower.includes("đoán xem")
  ) {
    return "Cơ hội mua vé phụ thuộc vào số lượng vé khả dụng tại thời điểm bạn tới lượt. Bạn vui lòng theo dõi chỉ số ước lượng ETA trên bảng vị trí chính thức.";
  }

  // 4. Lạc đề ngoài phạm vi bán vé / sự kiện
  if (
    lower.includes("thời tiết") ||
    lower.includes("nấu ăn") ||
    lower.includes("lập trình") ||
    lower.includes("chứng khoán")
  ) {
    return "Trợ lý EvenFlow chỉ hỗ trợ thông tin về sự kiện, hướng dẫn quy trình phòng chờ và chính sách bán vé. Rất tiếc tôi không thể giải đáp các chủ đề khác.";
  }

  return null;
}

const quotaStorageKey = (eventId: string) => `ef:assistant:quota:${eventId}`;

export function getStoredQuota(eventId: string): AssistantQuota {
  if (typeof window === "undefined") {
    return { used: 0, max: MAX_ASSISTANT_QUOTA, resetAt: Date.now() + QUOTA_WINDOW_MS };
  }

  try {
    const raw = window.sessionStorage.getItem(quotaStorageKey(eventId));
    if (!raw) {
      const initial: AssistantQuota = {
        used: 0,
        max: MAX_ASSISTANT_QUOTA,
        resetAt: Date.now() + QUOTA_WINDOW_MS,
      };
      window.sessionStorage.setItem(quotaStorageKey(eventId), JSON.stringify(initial));
      return initial;
    }

    const parsed = JSON.parse(raw) as AssistantQuota;
    // Kiểm tra hết hạn cửa sổ 15 phút thì reset
    if (Date.now() >= parsed.resetAt) {
      const resetQuota: AssistantQuota = {
        used: 0,
        max: MAX_ASSISTANT_QUOTA,
        resetAt: Date.now() + QUOTA_WINDOW_MS,
      };
      window.sessionStorage.setItem(quotaStorageKey(eventId), JSON.stringify(resetQuota));
      return resetQuota;
    }
    return parsed;
  } catch {
    return { used: 0, max: MAX_ASSISTANT_QUOTA, resetAt: Date.now() + QUOTA_WINDOW_MS };
  }
}

export function incrementStoredQuota(eventId: string): AssistantQuota {
  const current = getStoredQuota(eventId);
  const updated: AssistantQuota = {
    ...current,
    used: current.used + 1,
  };
  if (typeof window !== "undefined") {
    try {
      window.sessionStorage.setItem(quotaStorageKey(eventId), JSON.stringify(updated));
    } catch {
      // bỏ qua lỗi storage
    }
  }
  return updated;
}

export function resetStoredQuota(eventId: string): AssistantQuota {
  const reset: AssistantQuota = {
    used: 0,
    max: MAX_ASSISTANT_QUOTA,
    resetAt: Date.now() + QUOTA_WINDOW_MS,
  };
  if (typeof window !== "undefined") {
    try {
      window.sessionStorage.setItem(quotaStorageKey(eventId), JSON.stringify(reset));
    } catch {
      // bỏ qua lỗi storage
    }
  }
  return reset;
}

export interface SendQuestionParams {
  eventId: string;
  question: string;
  mode: DegradationMode;
  faqId?: string;
}

export interface SendQuestionResult {
  accepted: boolean;
  jobId?: string;
  immediateAnswer?: string;
  isFaq?: boolean;
  outOfScopeRefusal?: boolean;
  rateLimited?: boolean;
  notice?: string;
}

/**
 * Xử lý gửi câu hỏi tới trợ lý:
 * - Trả lời ngay 0 token nếu là FAQ hoặc bị từ chối ngoài phạm vi.
 * - Trả lời ngay nếu chạm nấc suy biến FAQ_ONLY / SAVING / OFF.
 * - Gửi 202 Accepted + jobId nếu là câu hỏi mở ở chế độ NORMAL.
 */
export async function sendAssistantMessage(
  params: SendQuestionParams
): Promise<SendQuestionResult> {
  const { eventId, question, mode, faqId } = params;

  // 1. Chế độ OFF: ẩn trợ lý hoàn toàn
  if (mode === "OFF") {
    return {
      accepted: false,
      notice: DEGRADATION_NOTICES.OFF ?? undefined,
    };
  }

  // 2. Nếu là bấm FAQ chip: trả ngay lập tức 0 token (AC-2)
  if (faqId) {
    const faq = DEFAULT_FAQ_ITEMS.find((f) => f.id === faqId);
    if (faq) {
      return {
        accepted: true,
        immediateAnswer: faq.answer,
        isFaq: true,
      };
    }
  }

  // 3. Chế độ FAQ_ONLY: tuyệt đối không gọi LLM, chỉ trả lời bằng FAQ hoặc hướng dẫn CSKH (AC-4)
  if (mode === "FAQ_ONLY") {
    // Khớp FAQ mẫu nếu câu hỏi tương tự
    const matched = DEFAULT_FAQ_ITEMS.find((f) =>
      question.toLowerCase().includes(f.question.toLowerCase().slice(0, 10))
    );
    if (matched) {
      return {
        accepted: true,
        immediateAnswer: matched.answer,
        isFaq: true,
        notice: DEGRADATION_NOTICES.FAQ_ONLY ?? undefined,
      };
    }

    return {
      accepted: true,
      immediateAnswer:
        "Trợ lý đang ở chế độ câu hỏi thường gặp. Bạn vui lòng chọn các chủ đề gợi ý phía dưới hoặc liên hệ bộ phận CSKH để được hỗ trợ chi tiết.",
      isFaq: true,
      notice: DEGRADATION_NOTICES.FAQ_ONLY ?? undefined,
    };
  }

  // 4. Chế độ SAVING: chỉ ưu tiên FAQ
  if (mode === "SAVING") {
    const matched = DEFAULT_FAQ_ITEMS.find((f) =>
      question.toLowerCase().includes(f.question.toLowerCase().slice(0, 10))
    );
    if (matched) {
      return {
        accepted: true,
        immediateAnswer: matched.answer,
        isFaq: true,
        notice: DEGRADATION_NOTICES.SAVING ?? undefined,
      };
    }

    return {
      accepted: true,
      immediateAnswer:
        "Hệ thống đang chịu tải cao, trợ lý tạm thời ưu tiên giải đáp các câu hỏi thường gặp. Bạn vui lòng chọn một trong các câu hỏi gợi ý bên dưới.",
      isFaq: true,
      notice: DEGRADATION_NOTICES.SAVING ?? undefined,
    };
  }

  // 5. Kiểm tra hạn mức 10 câu / 15 phút (BR-A2, AC-5)
  const quota = getStoredQuota(eventId);
  if (quota.used >= quota.max) {
    return {
      accepted: false,
      rateLimited: true,
      immediateAnswer:
        "Bạn đã đạt hạn mức 10 câu hỏi trong vòng 15 phút. Vui lòng tham khảo danh mục câu hỏi thường gặp (FAQ) hoặc liên hệ kênh CSKH nếu cần hỗ trợ khẩn cấp.",
      notice: "Đã đạt hạn mức câu hỏi cho phiên làm việc này.",
    };
  }

  // 6. Kiểm tra câu hỏi ngoài phạm vi (BR-A4, AC-6)
  const refusal = checkOutOfScope(question);
  if (refusal) {
    // Không tính vào quota khi bị từ chối hợp lệ
    return {
      accepted: true,
      immediateAnswer: refusal,
      outOfScopeRefusal: true,
    };
  }

  // 7. Câu hỏi hợp lệ: Tiêu thụ 1 quota câu hỏi mở và cấp jobId bất đồng bộ (BR-A1)
  incrementStoredQuota(eventId);
  const jobId = `job_${Date.now()}_${Math.random().toString(36).slice(2, 8)}`;

  return {
    accepted: true,
    jobId,
    notice: undefined,
  };
}

/**
 * Trình mô phỏng stream trả kết quả cho job (hoặc kết nối SSE trong tương lai).
 */
export async function streamJobResponse(
  jobId: string,
  question: string,
  onChunk: (chunk: string) => void,
  onComplete: (fullText: string) => void
): Promise<void> {
  // Sinh câu trả lời logic phù hợp với câu hỏi
  let answer =
    "Cảm ơn bạn đã đặt câu hỏi. Hàng đợi EvenFlow đang được điều phối tự động theo cơ chế Token Bucket. Bạn vui lòng giữ kết nối tab trình duyệt để hệ thống tự động chuyển bạn vào trang chọn vé ngay khi tới lượt.";

  if (question.toLowerCase().includes("vé vip") || question.toLowerCase().includes("hạng vé")) {
    answer =
      "Các hạng vé khả dụng (Standard, VIP, Early Bird) sẽ hiển thị chi tiết kèm giá vé niêm yết ngay khi bạn được tiếp nhận vào màn hình chọn vé.";
  } else if (question.toLowerCase().includes("thanh toán")) {
    answer =
      "Sau khi chọn vé, bạn có 10 phút để thanh toán qua cổng VNPay, MoMo hoặc Thẻ quốc tế. Quá 10 phút, phiên giữ vé sẽ hết hạn.";
  }

  // Mô phỏng từng chunk stream
  const words = answer.split(" ");
  let accumulated = "";

  for (let i = 0; i < words.length; i++) {
    const chunk = (i === 0 ? "" : " ") + words[i];
    accumulated += chunk;
    onChunk(chunk);
    // Nhịp streaming mượt mà ~25ms
    await new Promise((resolve) => setTimeout(resolve, 25));
  }

  onComplete(accumulated);
}
