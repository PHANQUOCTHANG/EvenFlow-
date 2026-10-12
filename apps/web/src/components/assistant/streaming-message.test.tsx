import { render, screen } from "@testing-library/react";
import { describe, it, expect } from "vitest";
import { StreamingMessage } from "./streaming-message";
import type { AssistantMessage } from "@/lib/assistant-client";

describe("StreamingMessage component (EV-185, AC-1, AC-3)", () => {
  it("render tin nhắn người dùng đúng định dạng", () => {
    const userMsg: AssistantMessage = {
      id: "u1",
      role: "user",
      content: "Thời gian giữ vé là bao lâu?",
      createdAt: new Date().toISOString(),
      status: "complete",
    };

    render(<StreamingMessage message={userMsg} />);

    expect(screen.getByText("Bạn")).toBeDefined();
    expect(screen.getByText("Thời gian giữ vé là bao lâu?")).toBeDefined();
    expect(screen.queryByText("AI hỗ trợ")).toBeNull();
  });

  it("render tin nhắn trợ lý với nhãn 'AI hỗ trợ' hoặc 'FAQ'", () => {
    const aiMsg: AssistantMessage = {
      id: "a1",
      role: "assistant",
      content: "Thời gian giữ vé là 10 phút.",
      createdAt: new Date().toISOString(),
      status: "complete",
      isFaq: false,
    };

    render(<StreamingMessage message={aiMsg} isLatestAssistant />);

    expect(screen.getByText("Trợ lý EvenFlow")).toBeDefined();
    expect(screen.getByText("AI hỗ trợ")).toBeDefined();
    expect(screen.getByText("Thời gian giữ vé là 10 phút.")).toBeDefined();
    expect(
      screen.getByText("Nguồn: Quy chế EvenFlow & thông tin xác nhận từ máy chủ.")
    ).toBeDefined();
  });

  it("render trạng thái pending với spinner/status", () => {
    const pendingMsg: AssistantMessage = {
      id: "a2",
      role: "assistant",
      content: "",
      createdAt: new Date().toISOString(),
      status: "pending",
    };

    render(<StreamingMessage message={pendingMsg} />);

    expect(screen.getByRole("status", { name: "Đang xử lý câu hỏi..." })).toBeDefined();
    expect(screen.getByText("Đang xử lý câu trả lời...")).toBeDefined();
  });

  it("render tin nhắn system thông báo", () => {
    const sysMsg: AssistantMessage = {
      id: "s1",
      role: "system",
      content: "Hệ thống đang ở chế độ câu hỏi thường gặp.",
      createdAt: new Date().toISOString(),
      status: "complete",
    };

    render(<StreamingMessage message={sysMsg} />);

    expect(screen.getByRole("status")).toBeDefined();
    expect(screen.getByText("Hệ thống đang ở chế độ câu hỏi thường gặp.")).toBeDefined();
  });
});
