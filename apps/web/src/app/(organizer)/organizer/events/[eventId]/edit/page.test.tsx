import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import EditEventPage from "./page";

const pushMock = vi.fn();

vi.mock("next/navigation", () => ({
  useRouter: () => ({
    push: pushMock,
  }),
}));

describe("EditEventPage route (/organizer/events/[eventId]/edit - AC-2)", () => {
  it("render form chỉnh sửa sự kiện đã có", async () => {
    const Component = await EditEventPage({
      params: Promise.resolve({ eventId: "evt-tech-summit-2026" }),
    });
    render(Component);

    expect(
      screen.getByText(/Chỉnh sửa: Vietnam Tech Summit 2026 — Kỷ Nguyên AI & Cloud Scale/),
    ).toBeInTheDocument();
  });
});
