import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import OrganizerEventsPage from "./page";

describe("OrganizerEventsPage route (/organizer/events - AC-1)", () => {
  it("render trang danh sách sự kiện của Ban tổ chức", async () => {
    const Component = await OrganizerEventsPage();
    render(Component);

    expect(screen.getByText("Sự kiện của tôi")).toBeInTheDocument();
    expect(screen.getByText("+ Tạo sự kiện mới")).toBeInTheDocument();
  });
});
