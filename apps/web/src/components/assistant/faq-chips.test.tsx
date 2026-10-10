import { render, screen, fireEvent } from "@testing-library/react";
import { describe, it, expect, vi } from "vitest";
import { FaqChips } from "./faq-chips";
import { DEFAULT_FAQ_ITEMS } from "@/lib/assistant-client";

describe("FaqChips component (EV-185, AC-2)", () => {
  it("render danh sách các chip FAQ theo hợp đồng", () => {
    const handleSelect = vi.fn();
    render(<FaqChips onSelectFaq={handleSelect} />);

    expect(screen.getByRole("region", { name: "Câu hỏi thường gặp" })).toBeDefined();

    for (const item of DEFAULT_FAQ_ITEMS) {
      expect(screen.getByText(item.question)).toBeDefined();
    }
  });

  it("click vào chip gọi onSelectFaq với đúng item", () => {
    const handleSelect = vi.fn();
    render(<FaqChips onSelectFaq={handleSelect} />);

    const firstChip = screen.getByText(DEFAULT_FAQ_ITEMS[0].question);
    fireEvent.click(firstChip);

    expect(handleSelect).toHaveBeenCalledTimes(1);
    expect(handleSelect).toHaveBeenCalledWith(DEFAULT_FAQ_ITEMS[0]);
  });

  it("khi disabled thì toàn bộ các chip bị vô hiệu hóa và không nhận click", () => {
    const handleSelect = vi.fn();
    render(<FaqChips onSelectFaq={handleSelect} disabled />);

    const buttons = screen.getAllByRole("button");
    for (const btn of buttons) {
      expect(btn).toBeDisabled();
    }

    fireEvent.click(buttons[0]);
    expect(handleSelect).not.toHaveBeenCalled();
  });

  it("đánh dấu aria-pressed khi selectedId khớp", () => {
    const handleSelect = vi.fn();
    const target = DEFAULT_FAQ_ITEMS[1];
    render(<FaqChips onSelectFaq={handleSelect} selectedId={target.id} />);

    const targetBtn = screen.getByRole("button", { name: target.question });
    expect(targetBtn.getAttribute("aria-pressed")).toBe("true");

    const otherBtn = screen.getByRole("button", { name: DEFAULT_FAQ_ITEMS[0].question });
    expect(otherBtn.getAttribute("aria-pressed")).toBe("false");
  });
});
