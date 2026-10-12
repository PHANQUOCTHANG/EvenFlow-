import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { describe, it, expect, beforeEach, vi } from "vitest";
import { ChatPanel } from "./chat-panel";
import { DEFAULT_FAQ_ITEMS, resetStoredQuota } from "@/lib/assistant-client";

describe("ChatPanel component (EV-185, AC-1..AC-6)", () => {
  const eventId = "ev-chat-panel-test";

  beforeEach(() => {
    window.sessionStorage.clear();
    resetStoredQuota(eventId);
  });

  it("render welcome state, nhãn 'AI hỗ trợ' và tiêu đề khi mở", () => {
    const handleClose = vi.fn();
    render(
      <ChatPanel
        eventId={eventId}
        eventTitle="Live Concert Anh Trai"
        isOpen={true}
        onClose={handleClose}
      />
    );

    expect(screen.getByText("Trợ lý EventFlow")).toBeDefined();
    expect(screen.getAllByText("AI hỗ trợ").length).toBeGreaterThanOrEqual(1);
    expect(screen.getByText("Live Concert Anh Trai")).toBeDefined();
    expect(screen.getByRole("region", { name: "Bảng điều khiển trợ lý AI" })).toBeDefined();
    expect(screen.getByText(/Xin chào! Tôi là Trợ lý EvenFlow/)).toBeDefined();
  });

  it("không render gì khi isOpen là false", () => {
    const handleClose = vi.fn();
    render(
      <ChatPanel
        eventId={eventId}
        isOpen={false}
        onClose={handleClose}
      />
    );

    expect(screen.queryByText("Trợ lý EventFlow")).toBeNull();
  });

  it("gọi onClose khi bấm nút đóng hoặc bấm phím Escape", () => {
    const handleClose = vi.fn();
    render(
      <ChatPanel
        eventId={eventId}
        isOpen={true}
        onClose={handleClose}
      />
    );

    const closeBtn = screen.getByRole("button", { name: "Đóng bảng trợ lý" });
    fireEvent.click(closeBtn);
    expect(handleClose).toHaveBeenCalledTimes(1);

    fireEvent.keyDown(window, { key: "Escape" });
    expect(handleClose).toHaveBeenCalledTimes(2);
  });

  it("ở chế độ FAQ_ONLY: hiển thị thông báo rõ ràng, không báo lỗi kỹ thuật và vô hiệu hóa ô input", () => {
    const handleClose = vi.fn();
    render(
      <ChatPanel
        eventId={eventId}
        isOpen={true}
        onClose={handleClose}
        initialMode="FAQ_ONLY"
      />
    );

    expect(
      screen.getAllByText(/Trợ lý đang ở chế độ câu hỏi thường gặp. Nếu cần thêm hỗ trợ, vui lòng liên hệ CSKH./).length
    ).toBeGreaterThanOrEqual(1);

    // Tuyệt đối không chứa mã lỗi
    expect(screen.queryByText(/429/)).toBeNull();
    expect(screen.queryByText(/500/)).toBeNull();

    const input = screen.getByPlaceholderText(/Đang ở chế độ FAQ/);
    expect(input).toBeDisabled();
  });

  it("ở chế độ OFF: ẩn trợ lý hoàn toàn và hiển thị thông tin kênh CSKH", () => {
    const handleClose = vi.fn();
    render(
      <ChatPanel
        eventId={eventId}
        isOpen={true}
        onClose={handleClose}
        initialMode="OFF"
      />
    );

    expect(screen.getByText("Trợ lý AI đang tạm ngưng")).toBeDefined();
    expect(screen.getByText(/1900 6868/)).toBeDefined();
    expect(screen.getByText(/support@evenflow.vn/)).toBeDefined();

    // Ẩn hoàn toàn form gửi tin nhắn
    expect(screen.queryByLabelText("Nội dung câu hỏi")).toBeNull();
  });

  it("ở chế độ SAVING: vô hiệu hóa ô input tự do, cho phép chọn FAQ chips", () => {
    const handleClose = vi.fn();
    render(
      <ChatPanel
        eventId={eventId}
        isOpen={true}
        onClose={handleClose}
        initialMode="SAVING"
      />
    );

    expect(
      screen.getAllByText(/Trợ lý đang ưu tiên các câu hỏi thường gặp/).length
    ).toBeGreaterThanOrEqual(1);
    const input = screen.getByPlaceholderText(/Đang ở chế độ FAQ/);
    expect(input).toBeDisabled();

    // FAQ chips vẫn hoạt động
    const faqChip = screen.getByText(DEFAULT_FAQ_ITEMS[0].question);
    expect(faqChip).toBeDefined();
  });

  it("click FAQ chip sẽ đẩy câu hỏi và câu trả lời vào luồng chat", async () => {
    const handleClose = vi.fn();
    render(
      <ChatPanel
        eventId={eventId}
        isOpen={true}
        onClose={handleClose}
      />
    );

    const firstFaq = DEFAULT_FAQ_ITEMS[0];
    const chipBtn = screen.getByText(firstFaq.question);
    fireEvent.click(chipBtn);

    await waitFor(() => {
      // 1 chip trong suggestions + 1 bubble tin nhắn
      expect(screen.getAllByText(firstFaq.question).length).toBeGreaterThanOrEqual(2);
      expect(screen.getByText(firstFaq.answer)).toBeDefined();
    });
  });

  it("gửi câu hỏi tự do qua form input", async () => {
    const handleClose = vi.fn();
    render(
      <ChatPanel
        eventId={eventId}
        isOpen={true}
        onClose={handleClose}
      />
    );

    const input = screen.getByPlaceholderText(/Đặt câu hỏi về sự kiện/);
    const submitBtn = screen.getByRole("button", { name: "Gửi" });

    fireEvent.change(input, { target: { value: "Tôi cần hỏi thông tin đổi vé" } });
    fireEvent.click(submitBtn);

    await waitFor(() => {
      expect(screen.getByTestId(/message-user-/)).toBeDefined();
      expect(screen.getByText("Tôi cần hỏi thông tin đổi vé")).toBeDefined();
    });
  });
});
