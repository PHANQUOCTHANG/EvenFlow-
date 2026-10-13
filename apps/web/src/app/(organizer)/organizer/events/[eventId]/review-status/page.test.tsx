import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import ReviewStatusPage from "./page";

describe("ReviewStatusPage route (/organizer/events/[eventId]/review-status - AC-3)", () => {
  it("render trạng thái kiểm duyệt của sự kiện", async () => {
    const Component = await ReviewStatusPage({
      params: Promise.resolve({ eventId: "evt-tech-summit-2026" }),
    });
    render(Component);

    expect(screen.getByText("Trạng thái kiểm duyệt")).toBeInTheDocument();
    expect(screen.getByText("Vietnam Tech Summit 2026 — Kỷ Nguyên AI & Cloud Scale")).toBeInTheDocument();
    expect(screen.getByText("✅ Đã duyệt — Sẵn sàng mở bán")).toBeInTheDocument();
  });

  it("render thẻ không tìm thấy khi mã sự kiện không tồn tại", async () => {
    const Component = await ReviewStatusPage({
      params: Promise.resolve({ eventId: "non-existent-event" }),
    });
    render(Component);

    expect(screen.getByText("Không tìm thấy sự kiện")).toBeInTheDocument();
  });
});
