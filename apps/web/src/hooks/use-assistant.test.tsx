import { renderHook, act } from "@testing-library/react";
import { describe, it, expect, beforeEach } from "vitest";
import { useAssistant } from "./use-assistant";
import { DEFAULT_FAQ_ITEMS, resetStoredQuota } from "@/lib/assistant-client";

describe("useAssistant hook (EV-185, AC-1..AC-5)", () => {
  const eventId = "ev-hook-test";

  beforeEach(() => {
    window.sessionStorage.clear();
    resetStoredQuota(eventId);
  });

  it("khởi tạo state mặc định với tin nhắn chào mừng và quota ban đầu", () => {
    const { result } = renderHook(() =>
      useAssistant({ eventId, enableStreaming: false })
    );

    expect(result.current.mode).toBe("NORMAL");
    expect(result.current.messages.length).toBe(1);
    expect(result.current.messages[0].role).toBe("assistant");
    expect(result.current.messages[0].content).toContain("Trợ lý EvenFlow");
    expect(result.current.quota.used).toBe(0);
    expect(result.current.quota.max).toBe(10);
    expect(result.current.isPending).toBe(false);
  });

  it("askFaq thêm câu hỏi và câu trả lời FAQ ngay lập tức mà không tiêu hao quota", async () => {
    const { result } = renderHook(() =>
      useAssistant({ eventId, enableStreaming: false })
    );

    const faq = DEFAULT_FAQ_ITEMS[0];
    await act(async () => {
      await result.current.askFaq(faq);
    });

    expect(result.current.messages.length).toBe(3); // 1 welcome + 1 user + 1 assistant
    expect(result.current.messages[1].role).toBe("user");
    expect(result.current.messages[1].content).toBe(faq.question);

    expect(result.current.messages[2].role).toBe("assistant");
    expect(result.current.messages[2].content).toBe(faq.answer);
    expect(result.current.messages[2].isFaq).toBe(true);

    // Không tiêu hao quota
    expect(result.current.quota.used).toBe(0);
  });

  it("sendMessage gửi câu hỏi mở và tăng quota đã dùng", async () => {
    const { result } = renderHook(() =>
      useAssistant({ eventId, enableStreaming: false })
    );

    await act(async () => {
      await result.current.sendMessage("Cho tôi hỏi về quy định mang đồ ăn");
    });

    // Quota tăng lên 1
    expect(result.current.quota.used).toBe(1);
    expect(result.current.messages.length).toBeGreaterThanOrEqual(3);
    const lastMsg = result.current.messages[result.current.messages.length - 1];
    expect(lastMsg.role).toBe("assistant");
    expect(lastMsg.status).toBe("complete");
  });

  it("chuyển sang nấc FAQ_ONLY: phản hồi bằng FAQ template không báo lỗi kỹ thuật", async () => {
    const { result } = renderHook(() =>
      useAssistant({ eventId, initialMode: "FAQ_ONLY", enableStreaming: false })
    );

    await act(async () => {
      await result.current.sendMessage("Thời gian giữ vé là bao lâu?");
    });

    const lastMsg = result.current.messages[result.current.messages.length - 1];
    expect(lastMsg.isFaq).toBe(true);
    expect(lastMsg.notice).toContain("câu hỏi thường gặp");
  });

  it("chuyển nấc suy biến bằng setMode chèn system message tương ứng", () => {
    const { result } = renderHook(() =>
      useAssistant({ eventId, enableStreaming: false })
    );

    act(() => {
      result.current.setMode("SAVING");
    });

    expect(result.current.mode).toBe("SAVING");
    const sysMsg = result.current.messages[result.current.messages.length - 1];
    expect(sysMsg.role).toBe("system");
    expect(sysMsg.content).toContain("ưu tiên các câu hỏi thường gặp");
  });

  it("khoá quota khi người dùng hỏi vượt quá 10 câu", async () => {
    const { result } = renderHook(() =>
      useAssistant({ eventId, enableStreaming: false })
    );

    // Đẩy quota lên 10
    for (let i = 0; i < 10; i++) {
      await act(async () => {
        await result.current.sendMessage(`Câu hỏi số ${i + 1}`);
      });
    }

    expect(result.current.quota.used).toBe(10);

    // Câu hỏi thứ 11
    await act(async () => {
      await result.current.sendMessage("Câu hỏi thứ 11 ngoài hạn mức");
    });

    const lastMsg = result.current.messages[result.current.messages.length - 1];
    expect(lastMsg.content).toContain("10 câu hỏi");
    expect(lastMsg.content).toContain("CSKH");
  });

  it("clearHistory và resetQuota dọn dẹp lại hội thoại", async () => {
    const { result } = renderHook(() =>
      useAssistant({ eventId, enableStreaming: false })
    );

    await act(async () => {
      await result.current.sendMessage("Hỏi thử 1 câu");
    });

    expect(result.current.messages.length).toBeGreaterThan(1);

    act(() => {
      result.current.clearHistory();
      result.current.resetQuota();
    });

    expect(result.current.messages.length).toBe(1);
    expect(result.current.quota.used).toBe(0);
  });
});
