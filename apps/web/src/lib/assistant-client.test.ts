import { describe, it, expect, beforeEach } from "vitest";
import {
  DEFAULT_FAQ_ITEMS,
  checkOutOfScope,
  getStoredQuota,
  incrementStoredQuota,
  resetStoredQuota,
  sendAssistantMessage,
  streamJobResponse,
  MAX_ASSISTANT_QUOTA,
  CSKH_INFO,
} from "./assistant-client";

describe("assistant-client (EV-185, EVF-115)", () => {
  const eventId = "ev-test-123";

  beforeEach(() => {
    window.sessionStorage.clear();
    resetStoredQuota(eventId);
  });

  describe("1. Danh sách FAQ & Trả lời tức thì 0 token (AC-2)", () => {
    it("có đầy đủ các câu hỏi thường gặp trọng tâm theo nghiệp vụ", () => {
      expect(DEFAULT_FAQ_ITEMS.length).toBeGreaterThanOrEqual(5);

      const holdFaq = DEFAULT_FAQ_ITEMS.find((f) => f.id === "faq-hold-timer");
      expect(holdFaq).toBeDefined();
      expect(holdFaq?.answer).toContain("10:00");
      expect(holdFaq?.answer).toContain("expires_at");

      const connectionFaq = DEFAULT_FAQ_ITEMS.find((f) => f.id === "faq-connection-loss");
      expect(connectionFaq).toBeDefined();
      expect(connectionFaq?.answer).toContain("60 giây");
    });

    it("khi click FAQ chip thì trả về ngay câu trả lời mẫu mà không tiêu tốn quota", async () => {
      const quotaBefore = getStoredQuota(eventId);

      const res = await sendAssistantMessage({
        eventId,
        question: "Thời gian giữ vé là bao lâu?",
        mode: "NORMAL",
        faqId: "faq-hold-timer",
      });

      expect(res.accepted).toBe(true);
      expect(res.immediateAnswer).toContain("10:00 phút");
      expect(res.isFaq).toBe(true);

      const quotaAfter = getStoredQuota(eventId);
      expect(quotaAfter.used).toBe(quotaBefore.used);
    });
  });

  describe("2. Kiểm tra câu hỏi ngoài phạm vi (BR-A4, AC-6)", () => {
    it("từ chối yêu cầu giữ vé trước hoặc can thiệp vị trí hàng đợi", () => {
      const refusal = checkOutOfScope("Làm sao để giữ vé trước cho tôi?");
      expect(refusal).not.toBeNull();
      expect(refusal).toContain("không thể can thiệp thứ tự");
    });

    it("từ chối tiết lộ con số tồn kho chính xác", () => {
      const refusal = checkOutOfScope("Kho còn chính xác bao nhiêu vé VIP?");
      expect(refusal).not.toBeNull();
      expect(refusal).toContain("không tiết lộ số lượng tồn kho chính xác");
    });

    it("từ chối các câu hỏi lạc đề ngoài phạm vi bán vé", () => {
      const refusal = checkOutOfScope("Hôm nay thời tiết thế nào?");
      expect(refusal).not.toBeNull();
      expect(refusal).toContain("chỉ hỗ trợ thông tin về sự kiện");
    });

    it("cho phép các câu hỏi hợp lệ trong phạm vi", () => {
      const refusal = checkOutOfScope("Sự kiện có cho phép đổi thông tin vé không?");
      expect(refusal).toBeNull();
    });
  });

  describe("3. Quản lý hạn mức 10 câu / 15 phút (BR-A2, AC-5)", () => {
    it("theo dõi và tăng số lượt hỏi đúng đắn", () => {
      const q0 = getStoredQuota(eventId);
      expect(q0.used).toBe(0);
      expect(q0.max).toBe(MAX_ASSISTANT_QUOTA);

      const q1 = incrementStoredQuota(eventId);
      expect(q1.used).toBe(1);

      const q2 = incrementStoredQuota(eventId);
      expect(q2.used).toBe(2);
    });

    it("tự động chặn gửi câu hỏi khi đã chạm ngưỡng 10 câu", async () => {
      for (let i = 0; i < MAX_ASSISTANT_QUOTA; i++) {
        incrementStoredQuota(eventId);
      }

      const res = await sendAssistantMessage({
        eventId,
        question: "Cho tôi hỏi về giá vé Standard",
        mode: "NORMAL",
      });

      expect(res.accepted).toBe(false);
      expect(res.rateLimited).toBe(true);
      expect(res.immediateAnswer).toContain("10 câu hỏi");
      expect(res.immediateAnswer).toContain("CSKH");
    });
  });

  describe("4. Xử lý các nấc suy biến hệ thống (AC-4)", () => {
    it("ở chế độ FAQ_ONLY: không báo lỗi kỹ thuật, chỉ trả lời FAQ hoặc gợi ý CSKH", async () => {
      const res = await sendAssistantMessage({
        eventId,
        question: "Cho tôi hỏi về thời gian mở cửa",
        mode: "FAQ_ONLY",
      });

      expect(res.accepted).toBe(true);
      expect(res.isFaq).toBe(true);
      expect(res.jobId).toBeUndefined();
      expect(res.notice).toContain("câu hỏi thường gặp");
      expect(res.immediateAnswer).not.toContain("429");
      expect(res.immediateAnswer).not.toContain("500");
    });

    it("ở chế độ SAVING: ưu tiên các câu hỏi thường gặp, hiển thị notice thân thiện", async () => {
      const res = await sendAssistantMessage({
        eventId,
        question: "Thời gian diễn ra sự kiện?",
        mode: "SAVING",
      });

      expect(res.accepted).toBe(true);
      expect(res.isFaq).toBe(true);
      expect(res.notice).toContain("ưu tiên các câu hỏi thường gặp");
    });

    it("ở chế độ OFF: từ chối xử lý và cung cấp thông điệp CSKH", async () => {
      const res = await sendAssistantMessage({
        eventId,
        question: "Xin chào",
        mode: "OFF",
      });

      expect(res.accepted).toBe(false);
      expect(res.notice).toContain("CSKH");
      expect(CSKH_INFO.hotline).toBe("1900 6868");
      expect(CSKH_INFO.email).toBe("support@evenflow.vn");
    });

    it("ở chế độ NORMAL với câu hỏi hợp lệ: trả 202 + jobId bất đồng bộ (BR-A1)", async () => {
      const res = await sendAssistantMessage({
        eventId,
        question: "Sự kiện có cho mang theo nước uống không?",
        mode: "NORMAL",
      });

      expect(res.accepted).toBe(true);
      expect(res.jobId).toBeDefined();
      expect(res.jobId).toContain("job_");
    });
  });

  describe("5. Mô phỏng luồng Streaming (AC-3)", () => {
    it("stream các token tuần tự và gọi onComplete với đầy đủ nội dung", async () => {
      const chunks: string[] = [];
      let completedText = "";

      await streamJobResponse(
        "job_123",
        "Có những hạng vé VIP nào?",
        (chunk) => chunks.push(chunk),
        (fullText) => {
          completedText = fullText;
        }
      );

      expect(chunks.length).toBeGreaterThan(0);
      expect(completedText).toContain("hạng vé");
      expect(chunks.join("")).toBe(completedText);
    });
  });
});
