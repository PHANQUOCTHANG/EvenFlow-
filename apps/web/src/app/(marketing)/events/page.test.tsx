import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import EventsListPage from "./page";

describe("EventsListPage (EVF-111)", () => {
  it("render tieu de chinh va danh sach the su kien", async () => {
    const jsx = await EventsListPage();
    render(jsx);

    expect(
      screen.getByRole("heading", { level: 1, name: /Sự kiện nổi bật/i }),
    ).toBeInTheDocument();

    expect(
      screen.getByText(/EvenFlow Grand Concert 2026/i),
    ).toBeInTheDocument();
    expect(
      screen.getByText(/Vietnam Tech Summit 2026/i),
    ).toBeInTheDocument();
  });
});
