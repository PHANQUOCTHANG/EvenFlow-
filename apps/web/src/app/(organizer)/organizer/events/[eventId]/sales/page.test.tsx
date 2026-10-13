import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import OrganizerSalesPage from "./page";

describe("OrganizerSalesPage route (/organizer/events/[eventId]/sales - AC-4)", () => {
  it("render trang dashboard doanh số realtime cho sự kiện hợp lệ", async () => {
    const Component = await OrganizerSalesPage({
      params: Promise.resolve({ eventId: "evt-tech-summit-2026" }),
    });
    render(Component);

    expect(screen.getByText("Vietnam Tech Summit 2026 — Kỷ Nguyên AI & Cloud Scale")).toBeInTheDocument();
    expect(screen.getByText(/Realtime \(≤ 5s\)/)).toBeInTheDocument();
    expect(screen.getByText("Tổng doanh thu")).toBeInTheDocument();
  });

  it("render thẻ không tìm thấy khi mã sự kiện không tồn tại", async () => {
    const Component = await OrganizerSalesPage({
      params: Promise.resolve({ eventId: "non-existent-event" }),
    });
    render(Component);

    expect(screen.getByText("Không tìm thấy sự kiện")).toBeInTheDocument();
  });
});
