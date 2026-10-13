import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import NewEventPage from "./page";

const pushMock = vi.fn();

vi.mock("next/navigation", () => ({
  useRouter: () => ({
    push: pushMock,
  }),
}));

describe("NewEventPage route (/organizer/events/new - AC-2)", () => {
  it("render stepper wizard tạo sự kiện mới", () => {
    render(<NewEventPage />);
    expect(screen.getByText("Tạo sự kiện mới")).toBeInTheDocument();
    expect(screen.getByLabelText(/Tên sự kiện/)).toBeInTheDocument();
  });
});
